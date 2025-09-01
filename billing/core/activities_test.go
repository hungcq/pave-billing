package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"encore.app/billing/models"
	"encore.app/billing/repository"
	"encore.dev/types/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBillingActivities(t *testing.T) {
	t.Run("should_create_activities_with_repository", func(t *testing.T) {
		fakeRepo := &repository.FakeRepo{}
		activities := NewBillingActivities(fakeRepo)

		assert.NotNil(t, activities)
		assert.Equal(t, fakeRepo, activities.repository)
	})
}

func TestBillingActivities_SaveBill(t *testing.T) {
	t.Run("when_bill_is_valid", func(t *testing.T) {
		t.Run("should_save_bill_successfully", func(t *testing.T) {
			fakeRepo := &repository.FakeRepo{}
			activities := NewBillingActivities(fakeRepo)

			bill := &models.Bill{
				ID:          uuid.Must(uuid.NewV4()),
				CustomerID:  "customer-123",
				Status:      models.BillStatusOpen,
				PeriodStart: time.Now(),
				PeriodEnd:   time.Now().AddDate(0, 1, 0),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			err := activities.SaveBill(context.TODO(), bill)

			assert.NoError(t, err)

			// Verify bill was saved in the fake repo
			savedBill, err := fakeRepo.GetBillByID(context.TODO(), bill.ID)
			assert.NoError(t, err)
			assert.Equal(t, bill.ID, savedBill.ID)
			assert.Equal(t, bill.CustomerID, savedBill.CustomerID)
			assert.Equal(t, bill.Status, savedBill.Status)
		})
	})

	t.Run("when_repository_fails", func(t *testing.T) {
		t.Run("should_return_error", func(t *testing.T) {
			// Create a mock repository that always fails
			mockRepo := &MockRepository{
				createBillError: errors.New("database connection failed"),
			}
			activities := NewBillingActivities(mockRepo)

			bill := &models.Bill{
				ID:          uuid.Must(uuid.NewV4()),
				CustomerID:  "customer-123",
				Status:      models.BillStatusOpen,
				PeriodStart: time.Now(),
				PeriodEnd:   time.Now().AddDate(0, 1, 0),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			err := activities.SaveBill(context.TODO(), bill)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), "database connection failed")
		})
	})

	t.Run("when_bill_has_line_items", func(t *testing.T) {
		t.Run("should_save_bill_with_line_items", func(t *testing.T) {
			fakeRepo := &repository.FakeRepo{}
			activities := NewBillingActivities(fakeRepo)

			bill := &models.Bill{
				ID:          uuid.Must(uuid.NewV4()),
				CustomerID:  "customer-123",
				Status:      models.BillStatusOpen,
				PeriodStart: time.Now(),
				PeriodEnd:   time.Now().AddDate(0, 1, 0),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
				LineItems: []*models.LineItem{
					{
						ID:          uuid.Must(uuid.NewV4()),
						BillID:      uuid.Must(uuid.NewV4()),
						Description: "Test service",
						Currency:    models.USD,
						Quantity:    decimal.NewFromFloat(2.0),
						UnitPrice:   decimal.NewFromFloat(10.50),
						CreatedAt:   time.Now(),
					},
				},
			}

			err := activities.SaveBill(context.TODO(), bill)

			assert.NoError(t, err)

			// Verify bill was saved
			savedBill, err := fakeRepo.GetBillByID(context.TODO(), bill.ID)
			assert.NoError(t, err)
			assert.Equal(t, bill.ID, savedBill.ID)
			assert.Len(t, savedBill.LineItems, 1)
		})
	})
}

func TestBillingActivities_CloseBill(t *testing.T) {
	t.Run("when_bill_exists", func(t *testing.T) {
		t.Run("should_close_bill_successfully", func(t *testing.T) {
			fakeRepo := &repository.FakeRepo{}
			activities := NewBillingActivities(fakeRepo)

			billID := uuid.Must(uuid.NewV4())
			closedAt := time.Now()

			// Create a bill in the fake repo first
			bill := &models.Bill{
				ID:          billID,
				CustomerID:  "customer-123",
				Status:      models.BillStatusOpen,
				PeriodStart: time.Now(),
				PeriodEnd:   time.Now().AddDate(0, 1, 0),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			err := fakeRepo.CreateBill(context.TODO(), bill)
			require.NoError(t, err)

			input := CloseBillInput{
				BillID:   billID,
				ClosedAt: closedAt,
			}

			closedBill, err := activities.CloseBill(context.TODO(), input)

			assert.NoError(t, err)
			assert.NotNil(t, closedBill)
			assert.Equal(t, models.BillStatusClosed, closedBill.Status)
			assert.Equal(t, &closedAt, closedBill.ClosedAt)

			// Verify the bill was actually closed in the repo
			savedBill, err := fakeRepo.GetBillByID(context.TODO(), billID)
			assert.NoError(t, err)
			assert.Equal(t, models.BillStatusClosed, savedBill.Status)
			assert.Equal(t, &closedAt, savedBill.ClosedAt)
		})
	})

	t.Run("when_bill_does_not_exist", func(t *testing.T) {
		t.Run("should_return_error", func(t *testing.T) {
			fakeRepo := &repository.FakeRepo{}
			activities := NewBillingActivities(fakeRepo)

			billID := uuid.Must(uuid.NewV4())
			closedAt := time.Now()

			input := CloseBillInput{
				BillID:   billID,
				ClosedAt: closedAt,
			}

			closedBill, err := activities.CloseBill(context.TODO(), input)

			assert.Error(t, err)
			assert.Nil(t, closedBill)
			assert.Equal(t, models.ErrBillNotFound, err)
		})
	})

	t.Run("when_repository_close_fails", func(t *testing.T) {
		t.Run("should_return_error", func(t *testing.T) {
			// Create a mock repository that fails on CloseBill
			mockRepo := &MockRepository{
				closeBillError: errors.New("failed to close bill"),
			}
			activities := NewBillingActivities(mockRepo)

			billID := uuid.Must(uuid.NewV4())
			closedAt := time.Now()

			input := CloseBillInput{
				BillID:   billID,
				ClosedAt: closedAt,
			}

			closedBill, err := activities.CloseBill(context.TODO(), input)

			assert.Error(t, err)
			assert.Nil(t, closedBill)
			assert.Contains(t, err.Error(), "failed to close bill")
		})
	})

	t.Run("when_repository_get_fails_after_close", func(t *testing.T) {
		t.Run("should_return_error", func(t *testing.T) {
			// Create a mock repository that succeeds on CloseBill but fails on GetBillByID
			mockRepo := &MockRepository{
				getBillByIDError: errors.New("failed to retrieve bill"),
			}
			activities := NewBillingActivities(mockRepo)

			billID := uuid.Must(uuid.NewV4())
			closedAt := time.Now()

			input := CloseBillInput{
				BillID:   billID,
				ClosedAt: closedAt,
			}

			closedBill, err := activities.CloseBill(context.TODO(), input)

			assert.Error(t, err)
			assert.Nil(t, closedBill)
			assert.Contains(t, err.Error(), "failed to retrieve bill")
		})
	})

	t.Run("when_bill_has_line_items", func(t *testing.T) {
		t.Run("should_close_bill_with_line_items", func(t *testing.T) {
			fakeRepo := &repository.FakeRepo{}
			activities := NewBillingActivities(fakeRepo)

			billID := uuid.Must(uuid.NewV4())
			closedAt := time.Now()

			// Create a bill with line items in the fake repo
			bill := &models.Bill{
				ID:          billID,
				CustomerID:  "customer-123",
				Status:      models.BillStatusOpen,
				PeriodStart: time.Now(),
				PeriodEnd:   time.Now().AddDate(0, 1, 0),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			err := fakeRepo.CreateBill(context.TODO(), bill)
			require.NoError(t, err)

			// Add line items to the bill
			lineItem := &models.LineItem{
				ID:          uuid.Must(uuid.NewV4()),
				BillID:      billID,
				Description: "Test service",
				Currency:    models.USD,
				Quantity:    decimal.NewFromFloat(1.0),
				UnitPrice:   decimal.NewFromFloat(10.00),
				CreatedAt:   time.Now(),
			}
			err = fakeRepo.AddLineItemToBill(context.TODO(), lineItem)
			require.NoError(t, err)

			input := CloseBillInput{
				BillID:   billID,
				ClosedAt: closedAt,
			}

			closedBill, err := activities.CloseBill(context.TODO(), input)

			assert.NoError(t, err)
			assert.NotNil(t, closedBill)
			assert.Equal(t, models.BillStatusClosed, closedBill.Status)
			assert.Equal(t, &closedAt, closedBill.ClosedAt)
			assert.Len(t, closedBill.LineItems, 1)
		})
	})
}

func TestBillingActivities_AddLineItemToBill(t *testing.T) {
	t.Run("when_line_item_is_valid", func(t *testing.T) {
		t.Run("should_add_line_item_successfully", func(t *testing.T) {
			fakeRepo := &repository.FakeRepo{}
			activities := NewBillingActivities(fakeRepo)

			billID := uuid.Must(uuid.NewV4())
			lineItem := models.LineItem{
				ID:          uuid.Must(uuid.NewV4()),
				BillID:      billID,
				Description: "Test service",
				Currency:    models.USD,
				Quantity:    decimal.NewFromFloat(2.0),
				UnitPrice:   decimal.NewFromFloat(10.50),
			}

			err := activities.AddLineItemToBill(context.TODO(), lineItem)

			assert.NoError(t, err)

			// Verify line item was added
			lineItems, err := fakeRepo.GetLineItemsByBillID(context.TODO(), billID)
			assert.NoError(t, err)
			assert.Len(t, lineItems, 1)
			assert.Equal(t, lineItem.ID, lineItems[0].ID)
			assert.Equal(t, lineItem.Description, lineItems[0].Description)
			assert.Equal(t, lineItem.Currency, lineItems[0].Currency)
			assert.True(t, lineItem.Quantity.Equal(lineItems[0].Quantity))
			assert.True(t, lineItem.UnitPrice.Equal(lineItems[0].UnitPrice))
		})
	})

	t.Run("when_repository_fails", func(t *testing.T) {
		t.Run("should_return_error", func(t *testing.T) {
			// Create a mock repository that fails on AddLineItemToBill
			mockRepo := &MockRepository{
				addLineItemError: errors.New("failed to add line item"),
			}
			activities := NewBillingActivities(mockRepo)

			lineItem := models.LineItem{
				ID:          uuid.Must(uuid.NewV4()),
				BillID:      uuid.Must(uuid.NewV4()),
				Description: "Test service",
				Currency:    models.USD,
				Quantity:    decimal.NewFromFloat(1.0),
				UnitPrice:   decimal.NewFromFloat(10.00),
			}

			err := activities.AddLineItemToBill(context.TODO(), lineItem)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to add line item")
		})
	})

	t.Run("when_line_item_has_high_precision_values", func(t *testing.T) {
		t.Run("should_preserve_decimal_precision", func(t *testing.T) {
			fakeRepo := &repository.FakeRepo{}
			activities := NewBillingActivities(fakeRepo)

			billID := uuid.Must(uuid.NewV4())
			lineItem := models.LineItem{
				ID:          uuid.Must(uuid.NewV4()),
				BillID:      billID,
				Description: "High precision service",
				Currency:    models.USD,
				Quantity:    decimal.NewFromFloat(0.333333),
				UnitPrice:   decimal.NewFromFloat(99.999999),
			}

			err := activities.AddLineItemToBill(context.TODO(), lineItem)

			assert.NoError(t, err)

			// Verify line item was added with preserved precision
			lineItems, err := fakeRepo.GetLineItemsByBillID(context.TODO(), billID)
			assert.NoError(t, err)
			assert.Len(t, lineItems, 1)
			assert.True(t, lineItem.Quantity.Equal(lineItems[0].Quantity))
			assert.True(t, lineItem.UnitPrice.Equal(lineItems[0].UnitPrice))
		})
	})

	t.Run("when_line_item_has_zero_values", func(t *testing.T) {
		t.Run("should_handle_zero_values_correctly", func(t *testing.T) {
			fakeRepo := &repository.FakeRepo{}
			activities := NewBillingActivities(fakeRepo)

			billID := uuid.Must(uuid.NewV4())
			lineItem := models.LineItem{
				ID:          uuid.Must(uuid.NewV4()),
				BillID:      billID,
				Description: "Free service",
				Currency:    models.USD,
				Quantity:    decimal.Zero,
				UnitPrice:   decimal.Zero,
			}

			err := activities.AddLineItemToBill(context.TODO(), lineItem)

			assert.NoError(t, err)

			// Verify line item was added
			lineItems, err := fakeRepo.GetLineItemsByBillID(context.TODO(), billID)
			assert.NoError(t, err)
			assert.Len(t, lineItems, 1)
			assert.True(t, decimal.Zero.Equal(lineItems[0].Quantity))
			assert.True(t, decimal.Zero.Equal(lineItems[0].UnitPrice))
		})
	})

	t.Run("when_line_item_has_negative_values", func(t *testing.T) {
		t.Run("should_handle_negative_values", func(t *testing.T) {
			fakeRepo := &repository.FakeRepo{}
			activities := NewBillingActivities(fakeRepo)

			billID := uuid.Must(uuid.NewV4())
			lineItem := models.LineItem{
				ID:          uuid.Must(uuid.NewV4()),
				BillID:      billID,
				Description: "Credit adjustment",
				Currency:    models.USD,
				Quantity:    decimal.NewFromFloat(-1.0),
				UnitPrice:   decimal.NewFromFloat(-5.00),
			}

			err := activities.AddLineItemToBill(context.TODO(), lineItem)

			assert.NoError(t, err)

			// Verify line item was added
			lineItems, err := fakeRepo.GetLineItemsByBillID(context.TODO(), billID)
			assert.NoError(t, err)
			assert.Len(t, lineItems, 1)
			assert.True(t, lineItem.Quantity.Equal(lineItems[0].Quantity))
			assert.True(t, lineItem.UnitPrice.Equal(lineItems[0].UnitPrice))
		})
	})
}

// MockRepository is a mock implementation for testing error scenarios
type MockRepository struct {
	createBillError   error
	getBillByIDError  error
	closeBillError    error
	addLineItemError  error
	getLineItemsError error
}

func (m *MockRepository) CreateBill(ctx context.Context, bill *models.Bill) error {
	if m.createBillError != nil {
		return m.createBillError
	}
	return nil
}

func (m *MockRepository) GetBillByID(ctx context.Context, billID uuid.UUID) (*models.Bill, error) {
	if m.getBillByIDError != nil {
		return nil, m.getBillByIDError
	}
	return &models.Bill{ID: billID}, nil
}

func (m *MockRepository) CloseBill(ctx context.Context, billID uuid.UUID, closedAt time.Time) error {
	if m.closeBillError != nil {
		return m.closeBillError
	}
	return nil
}

func (m *MockRepository) AddLineItemToBill(ctx context.Context, lineItem *models.LineItem) error {
	if m.addLineItemError != nil {
		return m.addLineItemError
	}
	return nil
}

func (m *MockRepository) GetLineItemsByBillID(ctx context.Context, billID uuid.UUID) ([]*models.LineItem, error) {
	if m.getLineItemsError != nil {
		return nil, m.getLineItemsError
	}
	return []*models.LineItem{}, nil
}
