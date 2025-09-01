package core

import (
	"time"

	"encore.app/billing/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	AddLineItemSignal = "AddLineItemSignal"

	CloseBillSignal = "CloseBillSignal"
	GetBillQuery    = "GetBillQuery"
)

// BillWorkflowInput represents the input for starting a bill workflow
type BillWorkflowInput struct {
	Bill *models.Bill `json:"bill"`
}

type LineItemSignalData struct {
	LineItem models.LineItem `json:"line_item"`
}

type CloseBillSignalData struct {
	RequestedAt time.Time `json:"requested_at"`
}

type BillWorkflows struct {
	cfg *models.AppConfig
}

func NewBillWorkflows(cfg *models.AppConfig) *BillWorkflows {
	return &BillWorkflows{cfg: cfg}
}

func (w *BillWorkflows) CreateBill(ctx workflow.Context, input BillWorkflowInput) error {
	logger := workflow.GetLogger(ctx)

	bill := input.Bill
	logger.Info("Starting bill workflow", "bill_id", bill.ID)

	// Get configuration for activity options
	activityCtx := workflow.WithActivityOptions(ctx, getDefaultActivityOptions(w.cfg))
	if err := workflow.ExecuteActivity(
		activityCtx, (&BillingActivities{}).SaveBill, bill,
	).Get(ctx, nil); err != nil {
		return err
	}

	// Signal channels
	addLineItemCh := workflow.GetSignalChannel(ctx, AddLineItemSignal)
	closeBillCh := workflow.GetSignalChannel(ctx, CloseBillSignal)
	if err := workflow.SetQueryHandler(ctx, GetBillQuery, func() (*models.Bill, error) {
		return bill, nil
	}); err != nil {
		return err
	}

	// Timer until period end
	duration := bill.PeriodEnd.Sub(workflow.Now(ctx))
	if duration < 0 {
		duration = 0
	}
	periodEndTimer := workflow.NewTimer(ctx, duration)

	selector := workflow.NewSelector(ctx)

	selector.AddReceive(addLineItemCh, func(c workflow.ReceiveChannel, more bool) {
		var signal LineItemSignalData
		c.Receive(ctx, &signal)
		logger.Info("Received add line item signal", "line_item_id", signal.LineItem.ID)

		success := bill.AddLineItem(signal.LineItem)

		if success { // the bill is not closed
			addItemCtx := workflow.WithActivityOptions(ctx, getDefaultActivityOptions(w.cfg))
			err := workflow.ExecuteActivity(addItemCtx, (&BillingActivities{}).AddLineItemToBill, signal.LineItem).
				Get(addItemCtx, nil)
			if err != nil {
				logger.Error("Failed to persist line item", "error", err)
			}
		} else {
			logger.Warn("Bill is closed, ignoring line item signal")
		}
	})

	selector.AddReceive(closeBillCh, func(c workflow.ReceiveChannel, more bool) {
		var signal CloseBillSignalData
		c.Receive(ctx, &signal)
		logger.Info("Received close bill signal, closing bill")
		closeBill(ctx, bill, signal.RequestedAt, w.cfg)
	})

	selector.AddFuture(periodEndTimer, func(f workflow.Future) {
		logger.Info("Billing period ended, automatically closing bill")
		closeBill(ctx, bill, workflow.Now(ctx), w.cfg)
	})

	for !bill.IsClosed() {
		selector.Select(ctx)
	}

	logger.Info("Bill workflow completed", "bill_id", bill.ID)
	return nil
}

// getDefaultActivityOptions returns activity options based on configuration
func getDefaultActivityOptions(cfg *models.AppConfig) workflow.ActivityOptions {
	return workflow.ActivityOptions{
		StartToCloseTimeout: time.Duration(cfg.Temporal.ActivityStartToCloseTimeout()) * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Duration(cfg.Temporal.ActivityRetryPolicy.InitialInterval()) * time.Second,
			BackoffCoefficient: cfg.Temporal.ActivityRetryPolicy.BackoffCoefficient(),
			MaximumInterval:    time.Duration(cfg.Temporal.ActivityRetryPolicy.MaximumInterval()) * time.Second,
			MaximumAttempts:    int32(cfg.Temporal.ActivityRetryPolicy.MaximumAttempts()),
		},
	}
}

func closeBill(ctx workflow.Context, bill *models.Bill, requestedAt time.Time, cfg *models.AppConfig) {
	success := bill.Close(requestedAt)
	if !success {
		workflow.GetLogger(ctx).Warn("Bill is already closed, ignoring close bill signal")
		return
	}

	activityCtx := workflow.WithActivityOptions(ctx, getDefaultActivityOptions(cfg))
	err := workflow.ExecuteActivity(activityCtx, (&BillingActivities{}).CloseBill, CloseBillInput{
		BillID:   bill.ID,
		ClosedAt: requestedAt,
	}).Get(ctx, nil)

	if err != nil {
		workflow.GetLogger(ctx).Error("Failed to close bill", "error", err)
	}
}
