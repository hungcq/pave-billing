package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	mocksCore "encore.app/billing/core/mocks"
	"encore.app/billing/ext_services/mocks"
	"encore.app/billing/models"
	"encore.app/billing/repository"
	"encore.dev/types/uuid"
	"github.com/golang/mock/gomock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

//go:generate mockgen -package=mocks -destination=mocks/temporal_client_mock.go go.temporal.io/sdk/client Client

func TestNewService(t *testing.T) {
	t.Run("should_create_service_with_all_dependencies", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		mockTemporalClient := mocksCore.NewMockClient(ctrl)
		fakeRepo := &repository.FakeRepo{}
		mockConversionService := mocks.NewMockExchangeRatesService(ctrl)

		cfg := &models.AppConfig{
			Billing: models.BillingConfig{
				Workflow: models.WorkflowConfig{},
			},
		}

		service := NewService(cfg, mockTemporalClient, fakeRepo, mockConversionService)

		assert.NotNil(t, service)
	})
}

func TestService_CreateBill(t *testing.T) {
	testCfg := &models.AppConfig{
		Billing: models.BillingConfig{
			Workflow: models.WorkflowConfig{
				WorkflowIDPrefix: func() string {
					return "test-prefix-"
				},
			},
		},
		Temporal: models.TemporalConfig{
			WorkflowExecutionTimeoutBuffer: func() int {
				return 10
			},
			TaskQueue: func() string {
				return "test-queue"
			},
		},
	}
	t.Run("when_request_is_valid", func(t *testing.T) {
		t.Run("should_create_bill_and_start_workflow", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockTemporalClient := mocksCore.NewMockClient(ctrl)
			fakeRepo := &repository.FakeRepo{}
			mockConversionService := mocks.NewMockExchangeRatesService(ctrl)
			mockTemporalClient.EXPECT().
				ExecuteWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(nil, nil)

			service := NewService(testCfg, mockTemporalClient, fakeRepo, mockConversionService)

			req := &models.CreateBillRequest{
				CustomerID:  "customer-123",
				PeriodStart: time.Now(),
				PeriodEnd:   time.Now().AddDate(0, 1, 0), // 1 month from now
			}

			bill, err := service.CreateBill(context.TODO(), req)

			assert.NoError(t, err)
			assert.NotNil(t, bill)
			assert.Equal(t, req.CustomerID, bill.CustomerID)
			assert.Equal(t, models.BillStatusOpen, bill.Status)
			assert.Equal(t, req.PeriodStart, bill.PeriodStart)
			assert.Equal(t, req.PeriodEnd, bill.PeriodEnd)
			assert.NotEmpty(t, bill.WorkflowID)
			assert.True(t, strings.HasPrefix(bill.WorkflowID, "test-prefix-"))
			assert.NotZero(t, bill.CreatedAt)
			assert.NotZero(t, bill.UpdatedAt)
		})
	})

	t.Run("when_temporal_client_fails", func(t *testing.T) {
		t.Run("should_return_error", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// Create a mock that fails
			mockTemporalClient := mocksCore.NewMockClient(ctrl)
			mockTemporalClient.EXPECT().ExecuteWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
				nil,
				errors.New("failed to start workflow"),
			)
			fakeRepo := &repository.FakeRepo{}
			mockConversionService := mocks.NewMockExchangeRatesService(ctrl)

			service := NewService(testCfg, mockTemporalClient, fakeRepo, mockConversionService)

			req := &models.CreateBillRequest{
				CustomerID:  "customer-123",
				PeriodStart: time.Now(),
				PeriodEnd:   time.Now().AddDate(0, 1, 0),
			}

			bill, err := service.CreateBill(context.TODO(), req)

			assert.Error(t, err)
			assert.Nil(t, bill)
			assert.Contains(t, err.Error(), "failed to start workflow")
		})
	})
}

type fakeEncodedValue struct {
	value any
}

func (f fakeEncodedValue) HasValue() bool {
	return true
}

func (f fakeEncodedValue) Get(valuePtr interface{}) error {
	rv := reflect.ValueOf(valuePtr)
	if rv.Kind() != reflect.Ptr {
		return errors.New("valuePtr must be a pointer")
	}

	// Assign the stored value into the pointer
	rv.Elem().Set(reflect.ValueOf(f.value))
	return nil
}

func TestService_GetBillByID(t *testing.T) {
	t.Run("when_bill_exists_in_workflow", func(t *testing.T) {
		t.Run("should_return_bill_from_workflow", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockTemporalClient := mocksCore.NewMockClient(ctrl)
			fakeRepo := &repository.FakeRepo{}
			mockConversionService := mocks.NewMockExchangeRatesService(ctrl)

			cfg := &models.AppConfig{
				Billing: models.BillingConfig{
					Workflow: models.WorkflowConfig{
						WorkflowIDPrefix: func() string {
							return "test-prefix-"
						},
					},
				},
				Temporal: models.TemporalConfig{},
			}

			service := NewService(cfg, mockTemporalClient, fakeRepo, mockConversionService)

			billID := uuid.Must(uuid.NewV4())
			workflowID := "test-prefix-" + billID.String()

			// Create a bill in the fake repo
			bill := models.Bill{
				ID:         billID,
				CustomerID: "customer-123",
				Status:     models.BillStatusOpen,
				WorkflowID: workflowID,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
				LineItems: []*models.LineItem{
					{
						ID:          uuid.Must(uuid.NewV4()),
						BillID:      billID,
						Description: "Test service",
						Currency:    models.USD,
						Quantity:    decimal.NewFromFloat(1.0),
						UnitPrice:   decimal.NewFromFloat(10.00),
					},
				},
			}

			// Mock the conversion service to return rates
			mockConversionService.EXPECT().GetRates(gomock.Any()).Return(&models.RatesData{
				Rates: map[string]float64{
					"USD": 1.0,
				},
				UpdatedAt: time.Now(),
			}, nil).AnyTimes()
			mockTemporalClient.EXPECT().
				QueryWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(fakeEncodedValue{value: bill}, nil).AnyTimes()

			retrievedBill, err := service.GetBillByID(context.TODO(), billID)

			assert.NoError(t, err)
			assert.NotNil(t, retrievedBill)
			assert.Equal(t, billID, retrievedBill.ID)
			assert.Equal(t, "customer-123", retrievedBill.CustomerID)
			assert.Len(t, retrievedBill.LineItems, 1)
		})
	})

	t.Run("when_bill_not_found_in_workflow", func(t *testing.T) {
		t.Run("should_fallback_to_database", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockTemporalClient := mocksCore.NewMockClient(ctrl)
			fakeRepo := &repository.FakeRepo{}
			mockConversionService := mocks.NewMockExchangeRatesService(ctrl)

			cfg := &models.AppConfig{
				Billing: models.BillingConfig{
					Workflow: models.WorkflowConfig{
						WorkflowIDPrefix: func() string {
							return "test-prefix-"
						},
					},
				},
				Temporal: models.TemporalConfig{},
			}

			service := NewService(cfg, mockTemporalClient, fakeRepo, mockConversionService)

			billID := uuid.Must(uuid.NewV4())

			// Mock the conversion service to return rates
			mockConversionService.EXPECT().GetRates(gomock.Any()).Return(&models.RatesData{
				Rates: map[string]float64{
					"USD": 1.0,
				},
				UpdatedAt: time.Now(),
			}, nil).AnyTimes()
			mockTemporalClient.EXPECT().
				QueryWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(fakeEncodedValue{value: nil}, errors.New("not found"))

			retrievedBill, err := service.GetBillByID(context.TODO(), billID)

			assert.Error(t, err)
			assert.Nil(t, retrievedBill)
			assert.Equal(t, models.ErrBillNotFound, err)
		})
	})
}

func TestService_AddLineItemToBill(t *testing.T) {
	testCfg := &models.AppConfig{
		Billing: models.BillingConfig{
			Workflow: models.WorkflowConfig{
				WorkflowIDPrefix: func() string {
					return "test-prefix-"
				},
			},
		},
	}
	t.Run("when_bill_is_open", func(t *testing.T) {
		t.Run("should_add_line_item", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockTemporalClient := mocksCore.NewMockClient(ctrl)
			fakeRepo := &repository.FakeRepo{}
			mockConversionService := mocks.NewMockExchangeRatesService(ctrl)

			service := NewService(testCfg, mockTemporalClient, fakeRepo, mockConversionService)

			billID := uuid.Must(uuid.NewV4())
			workflowID := "test-prefix-" + billID.String()

			// Create a bill in the fake repo
			bill := models.Bill{
				ID:         billID,
				CustomerID: "customer-123",
				Status:     models.BillStatusOpen,
				WorkflowID: workflowID,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			req := &models.AddLineItemRequest{
				Description: "Test service",
				Currency:    models.USD,
				Quantity:    decimal.NewFromFloat(2.0),
				UnitPrice:   decimal.NewFromFloat(10.50),
			}

			// Mock the conversion service to return rates
			mockConversionService.EXPECT().GetRates(gomock.Any()).Return(&models.RatesData{
				Rates: map[string]float64{
					"USD": 1.0,
				},
				UpdatedAt: time.Now(),
			}, nil).AnyTimes()
			mockTemporalClient.EXPECT().
				SignalWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTemporalClient.EXPECT().
				QueryWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(fakeEncodedValue{value: bill}, nil).AnyTimes()

			updatedBill, err := service.AddLineItemToBill(context.TODO(), billID, req)

			assert.NoError(t, err)
			assert.NotNil(t, updatedBill)
			assert.Equal(t, billID, updatedBill.ID)
		})
	})

	t.Run("when_bill_is_closed", func(t *testing.T) {
		t.Run("should_return_error", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockTemporalClient := mocksCore.NewMockClient(ctrl)
			fakeRepo := &repository.FakeRepo{}
			mockConversionService := mocks.NewMockExchangeRatesService(ctrl)
			mockConversionService.EXPECT().GetRates(gomock.Any()).Return(&models.RatesData{
				Rates: map[string]float64{
					"USD": 1.0,
				},
				UpdatedAt: time.Now(),
			}, nil).AnyTimes()

			service := NewService(testCfg, mockTemporalClient, fakeRepo, mockConversionService)

			billID := uuid.Must(uuid.NewV4())
			workflowID := "test-prefix-" + billID.String()

			// Create a closed bill in the fake repo
			bill := models.Bill{
				ID:         billID,
				CustomerID: "customer-123",
				Status:     models.BillStatusClosed,
				WorkflowID: workflowID,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
				ClosedAt:   &[]time.Time{time.Now()}[0],
			}
			mockTemporalClient.EXPECT().
				QueryWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(fakeEncodedValue{value: bill}, nil)

			req := &models.AddLineItemRequest{
				Description: "Test service",
				Currency:    models.USD,
				Quantity:    decimal.NewFromFloat(2.0),
				UnitPrice:   decimal.NewFromFloat(10.50),
			}

			updatedBill, err := service.AddLineItemToBill(context.TODO(), billID, req)

			assert.Error(t, err)
			assert.Nil(t, updatedBill)
			assert.Equal(t, models.ErrBillClosed, err)
		})
	})
}

func TestService_CloseBill(t *testing.T) {
	testCfg := &models.AppConfig{
		Billing: models.BillingConfig{
			Workflow: models.WorkflowConfig{
				WorkflowIDPrefix: func() string {
					return "test-prefix-"
				},
			},
		},
	}
	t.Run("when_bill_is_open", func(t *testing.T) {
		t.Run("should_close_bill", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockTemporalClient := mocksCore.NewMockClient(ctrl)
			fakeRepo := &repository.FakeRepo{}
			mockConversionService := mocks.NewMockExchangeRatesService(ctrl)

			service := NewService(testCfg, mockTemporalClient, fakeRepo, mockConversionService)

			billID := uuid.Must(uuid.NewV4())
			workflowID := "test-prefix-" + billID.String()

			// Create a bill in the fake repo
			bill := models.Bill{
				ID:         billID,
				CustomerID: "customer-123",
				Status:     models.BillStatusOpen,
				WorkflowID: workflowID,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}

			// Mock the conversion service to return rates
			mockConversionService.EXPECT().GetRates(gomock.Any()).Return(&models.RatesData{
				Rates: map[string]float64{
					"USD": 1.0,
				},
				UpdatedAt: time.Now(),
			}, nil).AnyTimes()
			mockTemporalClient.EXPECT().SignalWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTemporalClient.EXPECT().
				QueryWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(fakeEncodedValue{value: bill}, nil).AnyTimes()

			closedBill, err := service.CloseBill(context.TODO(), billID)

			assert.NoError(t, err)
			assert.NotNil(t, closedBill)
			assert.Equal(t, models.BillStatusClosed, closedBill.Status)
			assert.NotNil(t, closedBill.ClosedAt)
		})
	})

	t.Run("when_bill_is_already_closed", func(t *testing.T) {
		t.Run("should_return_bill_without_changes", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			mockTemporalClient := mocksCore.NewMockClient(ctrl)
			fakeRepo := &repository.FakeRepo{}
			mockConversionService := mocks.NewMockExchangeRatesService(ctrl)

			service := NewService(testCfg, mockTemporalClient, fakeRepo, mockConversionService)

			billID := uuid.Must(uuid.NewV4())
			workflowID := "test-prefix-" + billID.String()
			closedAt := time.Now().Add(-time.Hour)

			// Create a closed bill in the fake repo
			bill := models.Bill{
				ID:         billID,
				CustomerID: "customer-123",
				Status:     models.BillStatusClosed,
				WorkflowID: workflowID,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
				ClosedAt:   &closedAt,
			}

			// Mock the conversion service to return rates
			mockConversionService.EXPECT().GetRates(gomock.Any()).Return(&models.RatesData{
				Rates: map[string]float64{
					"USD": 1.0,
				},
				UpdatedAt: time.Now(),
			}, nil)
			mockTemporalClient.EXPECT().
				QueryWorkflow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(fakeEncodedValue{value: bill}, nil)

			closedBill, err := service.CloseBill(context.TODO(), billID)

			assert.NoError(t, err)
			assert.NotNil(t, closedBill)
			assert.Equal(t, models.BillStatusClosed, closedBill.Status)
			assert.Equal(t, &closedAt, closedBill.ClosedAt) // Should not change
		})
	})
}
