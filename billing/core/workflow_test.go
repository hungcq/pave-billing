package core

import (
	"testing"
	"time"

	"encore.app/billing/models"
	"encore.dev/types/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
)

// helper to build a minimal AppConfig for workflow activity options
func testCfg() *models.AppConfig {
	return &models.AppConfig{
		Temporal: models.TemporalConfig{
			ActivityStartToCloseTimeout: func() int { return 60 },
			ActivityRetryPolicy: models.ActivityRetryPolicy{
				InitialInterval:    func() int { return 1 },
				BackoffCoefficient: func() float64 { return 2.0 },
				MaximumInterval:    func() int { return 60 },
				MaximumAttempts:    func() int { return 3 },
			},
		},
		Billing: models.BillingConfig{},
	}
}

func TestBillWorkflow(t *testing.T) {
	t.Run("when_started_should_save_bill_then_wait_for_signals_or_period_end", func(t *testing.T) {
		s := testsuite.WorkflowTestSuite{}
		env := s.NewTestWorkflowEnvironment()

		cfg := testCfg()
		w := NewBillWorkflows(cfg)

		// Mock activities: SaveBill, CloseBill, AddLineItemToBill
		env.OnActivity((&BillingActivities{}).SaveBill, mock.Anything, mock.Anything).
			Return(nil).Once()

		billID := uuid.Must(uuid.NewV4())
		start := time.Now()
		env.SetStartTime(start)

		bill := &models.Bill{
			ID:         billID,
			CustomerID: "cust-1",
			Status:     models.BillStatusOpen,
			CreatedAt:  start,
			UpdatedAt:  start,
			// period end far in the future so workflow waits until we close via signal
			PeriodStart: start,
			PeriodEnd:   start.Add(24 * time.Hour),
		}

		// Schedule a close signal a bit later
		closedAt := start.Add(2 * time.Hour)
		env.OnActivity((&BillingActivities{}).CloseBill, mock.Anything, mock.Anything).
			Return(bill, nil).Once()

		env.RegisterDelayedCallback(func() {
			env.SignalWorkflow(CloseBillSignal, CloseBillSignalData{RequestedAt: closedAt})
		}, time.Minute)

		env.ExecuteWorkflow(w.CreateBill, BillWorkflowInput{Bill: bill})

		assert.True(t, env.IsWorkflowCompleted())
		assert.NoError(t, env.GetWorkflowError())
	})

	t.Run("when_add_line_item_signal_received_on_open_bill_should_persist_item", func(t *testing.T) {
		s := testsuite.WorkflowTestSuite{}
		env := s.NewTestWorkflowEnvironment()

		cfg := testCfg()
		w := NewBillWorkflows(cfg)

		// SaveBill always succeeds
		env.OnActivity((&BillingActivities{}).SaveBill, mock.Anything, mock.Anything).
			Return(nil).Once()

		// Expect AddLineItemToBill once for the signal
		env.OnActivity((&BillingActivities{}).AddLineItemToBill, mock.Anything, mock.Anything).
			Return(nil).Once()

		// Close at the end so workflow can complete
		env.OnActivity((&BillingActivities{}).CloseBill, mock.Anything, mock.Anything).
			Return(&models.Bill{}, nil).Once()

		start := time.Now()
		env.SetStartTime(start)

		bill := &models.Bill{
			ID:          uuid.Must(uuid.NewV4()),
			CustomerID:  "cust-2",
			Status:      models.BillStatusOpen,
			CreatedAt:   start,
			UpdatedAt:   start,
			PeriodStart: start,
			PeriodEnd:   start.Add(24 * time.Hour),
		}

		item := models.LineItem{
			ID:          uuid.Must(uuid.NewV4()),
			BillID:      bill.ID,
			Description: "Service X",
			Currency:    models.USD,
			Quantity:    decimal.NewFromFloat(2),
			UnitPrice:   decimal.NewFromFloat(10),
		}

		// Send add-line-item signal, then a close signal to finish
		env.RegisterDelayedCallback(func() {
			env.SignalWorkflow(AddLineItemSignal, LineItemSignalData{LineItem: item})
		}, time.Minute)
		env.RegisterDelayedCallback(func() {
			env.SignalWorkflow(CloseBillSignal, CloseBillSignalData{RequestedAt: start.Add(2 * time.Hour)})
		}, 2*time.Minute)

		env.ExecuteWorkflow(w.CreateBill, BillWorkflowInput{Bill: bill})

		assert.True(t, env.IsWorkflowCompleted())
		assert.NoError(t, env.GetWorkflowError())
	})

	t.Run("GetBill query should return current bill state", func(t *testing.T) {
		s := testsuite.WorkflowTestSuite{}
		env := s.NewTestWorkflowEnvironment()

		cfg := testCfg()
		w := NewBillWorkflows(cfg)

		env.OnActivity((&BillingActivities{}).SaveBill, mock.Anything, mock.Anything).
			Return(nil).Once()
		env.OnActivity((&BillingActivities{}).CloseBill, mock.Anything, mock.Anything).
			Return(&models.Bill{}, nil).Once()

		start := time.Now()
		env.SetStartTime(start)

		bill := &models.Bill{
			ID:          uuid.Must(uuid.NewV4()),
			CustomerID:  "cust-3",
			Status:      models.BillStatusOpen,
			CreatedAt:   start,
			UpdatedAt:   start,
			PeriodStart: start,
			PeriodEnd:   start.Add(24 * time.Hour),
		}

		var queried models.Bill
		// Query shortly after start
		env.RegisterDelayedCallback(func() {
			f, _ := env.QueryWorkflow(GetBillQuery)
			_ = f.Get(&queried)
			// then close so workflow completes
			env.SignalWorkflow(CloseBillSignal, CloseBillSignalData{RequestedAt: start.Add(1 * time.Hour)})
		}, time.Minute)

		env.ExecuteWorkflow(w.CreateBill, BillWorkflowInput{Bill: bill})

		assert.True(t, env.IsWorkflowCompleted())
		assert.NoError(t, env.GetWorkflowError())
		assert.Equal(t, bill.ID, queried.ID)
		assert.Equal(t, bill.CustomerID, queried.CustomerID)
	})
}
