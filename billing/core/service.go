package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"encore.app/billing/ext_services"
	"encore.app/billing/models"
	"encore.app/billing/repository"
	"encore.dev/rlog"
	"encore.dev/types/uuid"
	"go.temporal.io/sdk/client"
)

//go:generate mockgen -package=mocks -destination=mocks/service_mock.go . Service
type Service interface {
	CreateBill(ctx context.Context, req *models.CreateBillRequest) (*models.Bill, error)
	GetBillByID(ctx context.Context, id uuid.UUID) (*models.Bill, error)
	AddLineItemToBill(ctx context.Context, billId uuid.UUID, req *models.AddLineItemRequest) (*models.Bill, error)
	CloseBill(ctx context.Context, id uuid.UUID) (*models.Bill, error)
}

type service struct {
	repository        repository.Repository
	temporalClient    client.Client
	conversionService ext_services.ExchangeRatesService
	cfg               *models.AppConfig
}

func NewService(
	cfg *models.AppConfig, temporalClient client.Client, repository repository.Repository, conversionService ext_services.ExchangeRatesService,
) *service {
	log := rlog.With("module", "billing_core")
	log.Info("billing service initialized",
		"temporal_client_available", temporalClient != nil,
		"repository_available", repository != nil,
		"conversion_service_available", conversionService != nil)

	return &service{
		temporalClient:    temporalClient,
		repository:        repository,
		conversionService: conversionService,
		cfg:               cfg,
	}
}

func (s *service) CreateBill(ctx context.Context, req *models.CreateBillRequest) (*models.Bill, error) {
	log := rlog.With("module", "billing_core").With("customer_id", req.CustomerID)
	log.Info("creating new bill",
		"period_start", req.PeriodStart,
		"period_end", req.PeriodEnd)

	billID := uuid.Must(uuid.NewV4())
	workflowID := fmt.Sprintf("%s%s", s.cfg.Billing.Workflow.WorkflowIDPrefix(), billID.String())

	bill := &models.Bill{
		ID:          billID,
		CustomerID:  req.CustomerID,
		Status:      models.BillStatusOpen,
		PeriodStart: req.PeriodStart,
		PeriodEnd:   req.PeriodEnd,
		WorkflowID:  workflowID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	log = log.With("bill_id", billID.String()).With("workflow_id", workflowID)
	log.Info("bill created, starting workflow")

	// Calculate workflow timeout with configured buffer
	workflowTimeout := req.PeriodEnd.Sub(req.PeriodStart) + time.Duration(s.cfg.Temporal.WorkflowExecutionTimeoutBuffer())*time.Second

	workflowOptions := client.StartWorkflowOptions{
		ID:                       workflowID,
		TaskQueue:                s.cfg.Temporal.TaskQueue(),
		WorkflowExecutionTimeout: workflowTimeout,
	}

	if _, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, (&BillWorkflows{}).CreateBill, BillWorkflowInput{Bill: bill}); err != nil {
		log.Error("failed to start workflow", "error", err)
		return nil, fmt.Errorf("failed to start workflow: %w", err)
	}

	log.Info("workflow started successfully")
	return bill, nil
}

func (s *service) GetBillByID(ctx context.Context, id uuid.UUID) (*models.Bill, error) {
	log := rlog.With("module", "billing_core").With("bill_id", id.String())
	log.Info("retrieving bill by ID")

	// Try to get bill from workflow first
	workflowID := fmt.Sprintf("%s%s", s.cfg.Billing.Workflow.WorkflowIDPrefix(), id.String())
	resp, err := s.temporalClient.QueryWorkflow(
		ctx, workflowID, "", GetBillQuery,
	)
	if err == nil {
		log.Info("bill found in workflow, querying workflow state")
		bill := &models.Bill{}
		if err = resp.Get(bill); err == nil {
			log.Info("bill retrieved from workflow, calculating totals")
			if err = s.calculateSum(ctx, bill); err != nil {
				log.Error("failed to calculate bill totals", "error", err)
				return nil, err
			}
			log.Info("bill retrieved successfully from workflow")
			return bill, nil
		}
		log.Warn("failed to get bill from workflow response", "error", err)
	}

	// error when querying workflow or bill is closed
	log.Info("bill not found in workflow, querying database")
	bill, err := s.repository.GetBillByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Warn("bill not found in database", "bill_id", id.String())
			return nil, models.ErrBillNotFound
		}
		log.Error("database error when retrieving bill", "error", err)
		return nil, err
	}

	log.Info("bill found in database, calculating totals")
	if err = s.calculateSum(ctx, bill); err != nil {
		log.Error("failed to calculate bill totals", "error", err)
		return nil, err
	}

	log.Info("bill retrieved successfully from database")
	return bill, nil
}

func (s *service) AddLineItemToBill(ctx context.Context, billId uuid.UUID, req *models.AddLineItemRequest) (*models.Bill, error) {
	log := rlog.With("module", "billing_core").With("bill_id", billId.String())
	log.Info("adding line item to bill",
		"description", req.Description,
		"currency", req.Currency,
		"quantity", req.Quantity,
		"unit_price", req.UnitPrice)

	// Get bill to check if it exists and is open
	bill, err := s.GetBillByID(ctx, billId)
	if err != nil {
		log.Error("failed to get bill for closing", "error", err)
		return nil, err
	}

	if bill.IsClosed() {
		log.Warn("attempted to add line item to closed bill")
		return nil, models.ErrBillClosed
	}

	id, _ := uuid.NewV4()
	signal := LineItemSignalData{
		LineItem: models.LineItem{
			ID:          id,
			BillID:      billId,
			Description: req.Description,
			Currency:    req.Currency,
			Quantity:    req.Quantity,
			UnitPrice:   req.UnitPrice,
			CreatedAt:   time.Now(),
		},
	}

	log = log.With("workflow_id", bill.WorkflowID)
	log.Info("sending line item signal to workflow")

	err = s.temporalClient.SignalWorkflow(ctx, bill.WorkflowID, "", AddLineItemSignal, signal)
	if err != nil {
		log.Error("failed to send signal to workflow", "error", err)
		return nil, fmt.Errorf("failed to send signal to workflow: %w", err)
	}

	bill, err = s.GetBillByID(ctx, billId)
	if err != nil {
		log.Error("failed to get bill after sending close signal", "error", err)
		return nil, err
	}
	bill.AddLineItem(signal.LineItem)

	log.Info("line item signal sent successfully")
	return bill, nil
}

func (s *service) CloseBill(ctx context.Context, id uuid.UUID) (*models.Bill, error) {
	log := rlog.With("module", "billing_core").With("bill_id", id.String())
	log.Info("closing bill")

	// Get bill to check if it exists and is open
	bill, err := s.GetBillByID(ctx, id)
	if err != nil {
		log.Error("failed to get bill for closing", "error", err)
		return nil, err
	}

	if bill.IsClosed() {
		log.Info("bill is already closed")
		return bill, nil
	}

	now := time.Now()
	// Send close signal to workflow
	signal := CloseBillSignalData{
		RequestedAt: now,
	}

	log = log.With("workflow_id", bill.WorkflowID)
	log.Info("sending close signal to workflow")

	err = s.temporalClient.SignalWorkflow(ctx, bill.WorkflowID, "", CloseBillSignal, signal)
	if err != nil {
		log.Error("failed to send close signal to workflow", "error", err)
		return nil, fmt.Errorf("failed to send close signal to workflow: %w", err)
	}

	// Get bill to check if it exists and is open
	bill, err = s.GetBillByID(ctx, id)
	if err != nil {
		log.Error("failed to get bill after sending close signal", "error", err)
		return nil, err
	}
	bill.Status = models.BillStatusClosed
	bill.ClosedAt = &now

	log.Info("bill closed successfully", "closed_at", now)
	return bill, nil
}

func (s *service) calculateSum(ctx context.Context, bill *models.Bill) error {
	log := rlog.With("module", "billing_core").With("bill_id", bill.ID.String())
	log.Info("calculating bill totals", "line_items_count", len(bill.LineItems))

	rates, err := s.conversionService.GetRates(ctx)
	if err != nil {
		log.Error("failed to get exchange rates", "error", err)
		return err
	}
	log.Info("exchange rates retrieved successfully")

	if err = bill.CalculateSum(rates); err != nil {
		log.Error("failed to calculate bill totals", "error", err)
		return err
	}

	log.Info("bill totals calculation completed successfully")
	return nil
}
