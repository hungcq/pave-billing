package core

import (
	"context"
	"time"

	"encore.app/billing/models"
	"encore.app/billing/repository"
	"encore.dev/rlog"
	"encore.dev/types/uuid"
)

func NewBillingActivities(repository repository.Repository) *BillingActivities {
	return &BillingActivities{
		repository: repository,
	}
}

type BillingActivities struct {
	repository repository.Repository
}

// SaveBill update bill status to "open" after the workflow has been started
func (a *BillingActivities) SaveBill(ctx context.Context, input *models.Bill) error {
	logger := rlog.With("module", "billing_activities")
	logger.Info("Saving bill", "bill_id", input.ID)

	err := a.repository.CreateBill(ctx, input)
	if err != nil {
		logger.Error("Failed to save bill", "error", err)
		return err
	}

	logger.Info("Save bill successfully", "bill_id", input.ID)
	return nil
}

type CloseBillInput struct {
	BillID   uuid.UUID `json:"bill_id"`
	ClosedAt time.Time `json:"closed_at"`
}

// CloseBill closes a bill and sets its final total
func (a *BillingActivities) CloseBill(ctx context.Context, input CloseBillInput) (*models.Bill, error) {
	logger := rlog.With("module", "billing_activities")
	logger.Info("Closing bill", "bill_id", input.BillID)
	err := a.repository.CloseBill(ctx, input.BillID, input.ClosedAt)
	if err != nil {
		logger.Error("Failed to close bill", "error", err)
		return nil, err
	}

	bill, err := a.repository.GetBillByID(ctx, input.BillID)
	if err != nil {
		logger.Error("Failed to get bill", "error", err)
		return nil, err
	}

	logger.Info("Bill closed successfully", "bill_id", input.BillID)
	return bill, nil
}

// AddLineItemToBill persists a line item and updates bill total in a single transaction
func (a *BillingActivities) AddLineItemToBill(ctx context.Context, lineItem models.LineItem) error {
	logger := rlog.With("module", "billing_activities")
	logger.Info("Persisting line item",
		"line_item_id", lineItem.ID,
		"bill_id", lineItem.BillID)

	err := a.repository.AddLineItemToBill(ctx, &lineItem)
	if err != nil {
		logger.Error("Failed to persist line item and update bill total", "error", err)
		return err
	}

	logger.Info("Line item persisted successfully",
		"line_item_id", lineItem.ID,
		"bill_id", lineItem.BillID)
	return nil
}
