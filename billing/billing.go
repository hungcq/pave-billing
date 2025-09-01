package billing

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"encore.app/billing/core"
	exchangerates "encore.app/billing/ext_services"
	"encore.app/billing/models"
	"encore.app/billing/repository"
	"encore.dev/config"
	"encore.dev/rlog"
	"encore.dev/storage/cache"
	"encore.dev/storage/sqldb"
	"encore.dev/types/uuid"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

//encore:service
type Handler struct {
	service        core.Service
	temporalClient client.Client
	worker         worker.Worker
}

var db = sqldb.NewDatabase("billing", sqldb.DatabaseConfig{
	Migrations: "./migrations",
})

var cacheCluster = cache.NewCluster("billing", cache.ClusterConfig{
	EvictionPolicy: cache.AllKeysLRU,
})

// Load loads the application configuration
var cfg = config.Load[*models.AppConfig]()

var secrets struct {
	TemporalApiKey string
}

// Use configured cache TTL for exchange rates
var exchangeRatesKV = cache.NewStructKeyspace[string, models.RatesData](cacheCluster, cache.KeyspaceConfig{
	KeyPattern:    "billing" + "/:key",
	DefaultExpiry: cache.ExpireIn(time.Duration(cfg.ExternalServices.ExchangeRates.TTL()) * time.Second),
})

func initHandler() (*Handler, error) {
	log := rlog.With("module", "billing_handler")
	log.Info("initializing billing handler")

	// Use configured Temporal host port
	temporalClient, err := client.Dial(client.Options{
		HostPort:          cfg.Temporal.Address(),
		Namespace:         cfg.Temporal.Namespace(),
		Logger:            rlog.With("module", "temporal_worker"),
		ConnectionOptions: client.ConnectionOptions{TLS: &tls.Config{}},
		Credentials:       client.NewAPIKeyStaticCredentials(secrets.TemporalApiKey),
	})
	if err != nil {
		log.Error("failed to create temporal client", "error", err)
		return nil, fmt.Errorf("failed to create temporal client: %w", err)
	}
	log.Info("temporal client created successfully")

	repo := repository.NewSQLRepository(db)
	log.Info("SQL repository initialized")

	conversionService := exchangerates.NewConversionService(cfg, exchangeRatesKV)
	log.Info("conversion service initialized")

	billingService := core.NewService(cfg, temporalClient, repo, conversionService)
	log.Info("billing core service initialized")

	// Use configured task queue
	w := worker.New(temporalClient, cfg.Temporal.TaskQueue(), worker.Options{})
	log.Info("temporal worker created", "task_queue", cfg.Temporal.TaskQueue())

	billingWorkflows := core.NewBillWorkflows(cfg)
	w.RegisterWorkflow(billingWorkflows.CreateBill)
	log.Info("bill workflow registered")

	activities := core.NewBillingActivities(repo)
	w.RegisterActivity(activities.SaveBill)
	w.RegisterActivity(activities.AddLineItemToBill)
	w.RegisterActivity(activities.CloseBill)
	log.Info("temporal activities registered",
		"activities", []string{"SaveBill", "AddLineItemToBill", "CloseBill"})

	err = w.Start()
	if err != nil {
		log.Error("worker failed to start", "error", err)
		fmt.Printf("Worker failed: %v\n", err)
		w.Stop()
		return nil, fmt.Errorf("failed to start temporal worker: %w", err)
	}
	log.Info("temporal worker started successfully")

	log.Info("billing handler initialization completed")
	return &Handler{
		service:        billingService,
		temporalClient: temporalClient,
		worker:         w,
	}, nil
}

// Shutdown gracefully shuts down the service
func (h *Handler) Shutdown(force context.Context) {
	log := rlog.With("module", "billing_handler")
	log.Info("shutting down billing handler")

	h.worker.Stop()
	log.Info("temporal worker stopped")

	h.temporalClient.Close()
	log.Info("temporal client closed")

	log.Info("billing handler shutdown completed")
}

// CreateBill creates a new bill and starts the billing workflow
//
//encore:api public method=POST path=/bills
func (h *Handler) CreateBill(ctx context.Context, req *models.CreateBillRequest) (*models.BillResponse, error) {
	log := rlog.With("module", "billing_handler").With("http_method", "POST").With("http_path", "/bills").With("customer_id", req.CustomerID)
	log.Info("creating new bill via HTTP API")

	// Validate request
	if err := ValidateCreateBillRequest(req); err != nil {
		log.Error("request validation failed", "error", err)
		return nil, err
	}
	log.Info("request validation passed")

	bill, err := h.service.CreateBill(ctx, req)
	if err != nil {
		log.Error("failed to create bill", "error", err)
		return nil, err
	}

	return &models.BillResponse{Data: bill}, nil
}

// AddLineItem adds a line item to an existing bill
//
//encore:api public method=POST path=/bills/:billId/line-items
func (h *Handler) AddLineItem(
	ctx context.Context, billId uuid.UUID, req *models.AddLineItemRequest,
) (*models.BillResponse, error) {
	log := rlog.With("module", "billing_handler").With("http_method", "POST").With("http_path", fmt.Sprintf("/bills/%s/line-items", billId)).With("bill_id", billId.String())
	log.Info("adding line item via HTTP API",
		"description", req.Description,
		"currency", req.Currency,
		"quantity", req.Quantity,
		"unit_price", req.UnitPrice)

	// Validate request
	if err := ValidateAddLineItemRequest(req); err != nil {
		log.Error("request validation failed", "error", err)
		return nil, err
	}
	log.Info("request validation passed")

	bill, err := h.service.AddLineItemToBill(ctx, billId, req)
	if err != nil {
		log.Error("failed to add line item", "error", err)
		return nil, err
	}

	return &models.BillResponse{Data: bill}, nil
}

// CloseBill closes an active bill
//
//encore:api public method=POST path=/bills/:bill_id/close
func (h *Handler) CloseBill(ctx context.Context, bill_id uuid.UUID) (*models.GetBillResponse, error) {
	log := rlog.With("module", "billing_handler").With("http_method", "POST").With("http_path", fmt.Sprintf("/bills/%s/close", bill_id)).With("bill_id", bill_id.String())
	log.Info("closing bill via HTTP API")

	bill, err := h.service.CloseBill(ctx, bill_id)
	if err != nil {
		log.Error("failed to close bill", "error", err)
		return nil, fmt.Errorf("failed to close bill: %w", err)
	}

	return &models.GetBillResponse{Data: bill}, nil
}

// GetBill retrieves a bill by ID with its line items
//
//encore:api public method=GET path=/bills/:bill_id
func (h *Handler) GetBill(ctx context.Context, bill_id uuid.UUID) (*models.GetBillResponse, error) {
	log := rlog.With("module", "billing_handler").With("http_method", "GET").With("http_path", fmt.Sprintf("/bills/%s", bill_id)).With("bill_id", bill_id.String())
	log.Info("retrieving bill via HTTP API")

	bill, err := h.service.GetBillByID(ctx, bill_id)
	if err != nil {
		log.Error("failed to retrieve bill", "error", err)
		return nil, err
	}

	return &models.GetBillResponse{Data: bill}, nil
}
