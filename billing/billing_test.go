package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"encore.app/billing/core/mocks"
	"encore.app/billing/models"
	"encore.dev/beta/errs"
	"encore.dev/types/uuid"
	"github.com/golang/mock/gomock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestCreateBill(t *testing.T) {
	t.Run("when_request_is_invalid_should_return_error", func(t *testing.T) {
		req := &models.CreateBillRequest{
			CustomerID:  "", // Invalid: empty customer ID
			PeriodStart: time.Now(),
			PeriodEnd:   time.Now().AddDate(0, 1, 0),
		}
		handler := &Handler{}
		response, err := handler.CreateBill(context.TODO(), req)

		assert.Error(t, err)
		assert.Nil(t, response)
		// Verify validation error
		var validationErr *errs.Error
		assert.ErrorAs(t, err, &validationErr)
		assert.Equal(t, errs.InvalidArgument, validationErr.Code)
		assert.Contains(t, validationErr.Message, "customer_id is required")
	})
	t.Run("when_request_is_valid", func(t *testing.T) {
		req := &models.CreateBillRequest{
			CustomerID:  "customer-123",
			PeriodStart: time.Now(),
			PeriodEnd:   time.Now().AddDate(0, 1, 0), // 1 month from now
		}

		t.Run("should_create_bill", func(t *testing.T) {
			mockSvc := mocks.NewMockService(gomock.NewController(t))
			handler := &Handler{service: mockSvc}

			defer handler.CreateBill(context.TODO(), req)

			mockSvc.EXPECT().CreateBill(gomock.Any(), req)
		})
		t.Run("when_service_returns_success", func(t *testing.T) {
			t.Run("should_return_bill", func(t *testing.T) {
				mockSvc := mocks.NewMockService(gomock.NewController(t))
				handler := &Handler{service: mockSvc}
				returnedBill := &models.Bill{
					ID:          uuid.Must(uuid.NewV4()),
					CustomerID:  req.CustomerID,
					Status:      models.BillStatusOpen,
					PeriodStart: req.PeriodStart,
					PeriodEnd:   req.PeriodEnd,
					CreatedAt:   time.Now(),
					UpdatedAt:   time.Now(),
				}
				mockSvc.EXPECT().CreateBill(gomock.Any(), req).Return(returnedBill, nil)

				res, err := handler.CreateBill(context.TODO(), req)

				assert.Nil(t, err)
				assert.Equal(t, &models.BillResponse{
					Data: returnedBill,
				}, res)
			})
		})
		t.Run("when_service_returns_error", func(t *testing.T) {
			mockSvc := mocks.NewMockService(gomock.NewController(t))
			handler := &Handler{service: mockSvc}
			mockSvc.EXPECT().CreateBill(gomock.Any(), req).Return(nil, errors.New("some error"))

			res, err := handler.CreateBill(context.TODO(), req)

			assert.Error(t, err)
			assert.Nil(t, res)
		})
	})
}

func TestAddLineItem(t *testing.T) {
	t.Run("when_request_is_invalid_should_return_error", func(t *testing.T) {
		billID := uuid.Must(uuid.NewV4())
		req := &models.AddLineItemRequest{
			Description: "", // Invalid: empty description
			Currency:    models.USD,
			Quantity:    decimal.NewFromFloat(2.0),
			UnitPrice:   decimal.NewFromFloat(10.50),
		}
		handler := &Handler{}
		response, err := handler.AddLineItem(context.TODO(), billID, req)

		assert.Error(t, err)
		assert.Nil(t, response)
		// Verify validation error
		var validationErr *errs.Error
		assert.ErrorAs(t, err, &validationErr)
		assert.Equal(t, errs.InvalidArgument, validationErr.Code)
		assert.Contains(t, validationErr.Message, "description is required")
	})

	t.Run("when_request_is_valid", func(t *testing.T) {
		billID := uuid.Must(uuid.NewV4())
		req := &models.AddLineItemRequest{
			Description: "Test service",
			Currency:    models.USD,
			Quantity:    decimal.NewFromFloat(2.0),
			UnitPrice:   decimal.NewFromFloat(10.50),
		}

		t.Run("should_add_line_item", func(t *testing.T) {
			mockSvc := mocks.NewMockService(gomock.NewController(t))
			handler := &Handler{service: mockSvc}

			defer handler.AddLineItem(context.TODO(), billID, req)

			mockSvc.EXPECT().AddLineItemToBill(gomock.Any(), billID, req)
		})

		t.Run("when_service_returns_success", func(t *testing.T) {
			t.Run("should_return_updated_bill", func(t *testing.T) {
				mockSvc := mocks.NewMockService(gomock.NewController(t))
				handler := &Handler{service: mockSvc}
				returnedBill := &models.Bill{
					ID:        billID,
					Status:    models.BillStatusOpen,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					LineItems: []*models.LineItem{
						{
							ID:          uuid.Must(uuid.NewV4()),
							BillID:      billID,
							Description: req.Description,
							Currency:    req.Currency,
							Quantity:    req.Quantity,
							UnitPrice:   req.UnitPrice,
						},
					},
				}
				mockSvc.EXPECT().AddLineItemToBill(gomock.Any(), billID, req).Return(returnedBill, nil)

				res, err := handler.AddLineItem(context.TODO(), billID, req)

				assert.Nil(t, err)
				assert.Equal(t, &models.BillResponse{
					Data: returnedBill,
				}, res)
			})
		})

		t.Run("when_service_returns_error", func(t *testing.T) {
			mockSvc := mocks.NewMockService(gomock.NewController(t))
			handler := &Handler{service: mockSvc}
			mockSvc.EXPECT().AddLineItemToBill(gomock.Any(), billID, req).Return(nil, errors.New("some error"))

			res, err := handler.AddLineItem(context.TODO(), billID, req)

			assert.Error(t, err)
			assert.Nil(t, res)
		})
	})
}

func TestCloseBill(t *testing.T) {
	t.Run("when_bill_id_is_valid", func(t *testing.T) {
		billID := uuid.Must(uuid.NewV4())

		t.Run("should_close_bill", func(t *testing.T) {
			mockSvc := mocks.NewMockService(gomock.NewController(t))
			handler := &Handler{service: mockSvc}

			defer handler.CloseBill(context.TODO(), billID)

			mockSvc.EXPECT().CloseBill(gomock.Any(), billID)
		})

		t.Run("when_service_returns_success", func(t *testing.T) {
			t.Run("should_return_closed_bill", func(t *testing.T) {
				mockSvc := mocks.NewMockService(gomock.NewController(t))
				handler := &Handler{service: mockSvc}
				closedAt := time.Now()
				returnedBill := &models.Bill{
					ID:        billID,
					Status:    models.BillStatusClosed,
					ClosedAt:  &closedAt,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				mockSvc.EXPECT().CloseBill(gomock.Any(), billID).Return(returnedBill, nil)

				res, err := handler.CloseBill(context.TODO(), billID)

				assert.Nil(t, err)
				assert.Equal(t, &models.GetBillResponse{
					Data: returnedBill,
				}, res)
			})
		})

		t.Run("when_service_returns_error", func(t *testing.T) {
			mockSvc := mocks.NewMockService(gomock.NewController(t))
			handler := &Handler{service: mockSvc}
			mockSvc.EXPECT().CloseBill(gomock.Any(), billID).Return(nil, errors.New("some error"))

			res, err := handler.CloseBill(context.TODO(), billID)

			assert.Error(t, err)
			assert.Nil(t, res)
		})
	})
}

func TestGetBill(t *testing.T) {
	t.Run("when_bill_id_is_valid", func(t *testing.T) {
		billID := uuid.Must(uuid.NewV4())

		t.Run("should_get_bill", func(t *testing.T) {
			mockSvc := mocks.NewMockService(gomock.NewController(t))
			handler := &Handler{service: mockSvc}

			defer handler.GetBill(context.TODO(), billID)

			mockSvc.EXPECT().GetBillByID(gomock.Any(), billID)
		})

		t.Run("when_service_returns_success", func(t *testing.T) {
			t.Run("should_return_bill", func(t *testing.T) {
				mockSvc := mocks.NewMockService(gomock.NewController(t))
				handler := &Handler{service: mockSvc}
				returnedBill := &models.Bill{
					ID:        billID,
					Status:    models.BillStatusOpen,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
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
					Total: &models.Total{
						ByCurrency: map[models.Currency]decimal.Decimal{
							models.USD: decimal.NewFromFloat(10.00),
						},
					},
				}
				mockSvc.EXPECT().GetBillByID(gomock.Any(), billID).Return(returnedBill, nil)

				res, err := handler.GetBill(context.TODO(), billID)

				assert.Nil(t, err)
				assert.Equal(t, &models.GetBillResponse{
					Data: returnedBill,
				}, res)
			})
		})

		t.Run("when_service_returns_error", func(t *testing.T) {
			mockSvc := mocks.NewMockService(gomock.NewController(t))
			handler := &Handler{service: mockSvc}
			mockSvc.EXPECT().GetBillByID(gomock.Any(), billID).Return(nil, errors.New("some error"))

			res, err := handler.GetBill(context.TODO(), billID)

			assert.Error(t, err)
			assert.Nil(t, res)
		})
	})
}

func TestValidation_InvalidPeriod(t *testing.T) {
	req := &models.CreateBillRequest{
		CustomerID:  "customer-123",
		PeriodStart: time.Now().AddDate(0, 1, 0), // Start in the future
		PeriodEnd:   time.Now(),                  // End in the past (invalid)
	}
	handler := &Handler{}
	response, err := handler.CreateBill(context.TODO(), req)

	assert.Error(t, err)
	assert.Nil(t, response)

	// Verify validation error
	var validationErr *errs.Error
	assert.ErrorAs(t, err, &validationErr)
	assert.Equal(t, errs.InvalidArgument, validationErr.Code)
}

func TestValidation_InvalidCurrency(t *testing.T) {
	billID := uuid.Must(uuid.NewV4())
	req := &models.AddLineItemRequest{
		Description: "Test service",
		Currency:    "INVALID", // Invalid currency
		Quantity:    decimal.NewFromFloat(1.0),
		UnitPrice:   decimal.NewFromFloat(10.00),
	}
	handler := &Handler{}
	response, err := handler.AddLineItem(context.TODO(), billID, req)

	assert.Error(t, err)
	assert.Nil(t, response)

	// Verify validation error
	var validationErr *errs.Error
	assert.ErrorAs(t, err, &validationErr)
	assert.Equal(t, errs.InvalidArgument, validationErr.Code)
}

func TestValidation_InvalidQuantity(t *testing.T) {
	billID := uuid.Must(uuid.NewV4())
	req := &models.AddLineItemRequest{
		Description: "Test service",
		Currency:    models.USD,
		Quantity:    decimal.NewFromFloat(-1.0), // Invalid: negative quantity
		UnitPrice:   decimal.NewFromFloat(10.00),
	}
	handler := &Handler{}
	response, err := handler.AddLineItem(context.TODO(), billID, req)

	assert.Error(t, err)
	assert.Nil(t, response)

	// Verify validation error
	var validationErr *errs.Error
	assert.ErrorAs(t, err, &validationErr)
	assert.Equal(t, errs.InvalidArgument, validationErr.Code)
}

func TestValidation_InvalidUnitPrice(t *testing.T) {
	billID := uuid.Must(uuid.NewV4())
	req := &models.AddLineItemRequest{
		Description: "Test service",
		Currency:    models.USD,
		Quantity:    decimal.NewFromFloat(1.0),
		UnitPrice:   decimal.NewFromFloat(-10.00), // Invalid: negative price
	}
	handler := &Handler{}
	response, err := handler.AddLineItem(context.TODO(), billID, req)

	assert.Error(t, err)
	assert.Nil(t, response)

	// Verify validation error
	var validationErr *errs.Error
	assert.ErrorAs(t, err, &validationErr)
	assert.Equal(t, errs.InvalidArgument, validationErr.Code)
	assert.Contains(t, validationErr.Message, "unit_price cannot be negative")
}
