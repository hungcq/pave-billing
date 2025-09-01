package models

import (
	"encoding/json"
	"testing"
	"time"

	"encore.dev/types/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestCurrency_Validate(t *testing.T) {
	tests := []struct {
		name     string
		currency Currency
		wantErr  bool
	}{
		{"valid USD", USD, false},
		{"valid GEL", GEL, false},
		{"invalid currency", "EUR", true},
		{"invalid currency", "GBP", true},
		{"empty currency", "", true},
		{"case sensitive", "usd", true},
		{"case sensitive", "gel", true},
	}

	cfg := &AppConfig{
		Billing: BillingConfig{
			Validation: ValidationConfig{
				AllowedCurrencies: func() []string {
					return []string{"USD", "GEL"}
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.currency.Validate(cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, ErrInvalidCurrency, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBillStatus_Validate(t *testing.T) {
	tests := []struct {
		name    string
		status  BillStatus
		wantErr bool
	}{
		{"valid open", BillStatusOpen, false},
		{"valid closed", BillStatusClosed, false},
		{"invalid status", "invalid", true},
		{"invalid status", "pending", true},
		{"empty status", "", true},
		{"case sensitive", "OPEN", true},
		{"case sensitive", "CLOSED", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.status.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, ErrInvalidBillStatus, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBill_IsOpen(t *testing.T) {
	tests := []struct {
		name     string
		status   BillStatus
		expected bool
	}{
		{"open status", BillStatusOpen, true},
		{"closed status", BillStatusClosed, false},
		{"empty status", "", false},
		{"invalid status", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bill := &Bill{Status: tt.status}
			result := bill.IsOpen()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBill_IsClosed(t *testing.T) {
	tests := []struct {
		name     string
		status   BillStatus
		expected bool
	}{
		{"closed status", BillStatusClosed, true},
		{"open status", BillStatusOpen, false},
		{"empty status", "", false},
		{"invalid status", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bill := &Bill{Status: tt.status}
			result := bill.IsClosed()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBill_AddLineItem(t *testing.T) {
	t.Run("when bill is open", func(t *testing.T) {
		bill := &Bill{
			Status: BillStatusOpen,
			LineItems: []*LineItem{
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "Existing item",
					Currency:    USD,
					Quantity:    decimal.NewFromFloat(1.0),
					UnitPrice:   decimal.NewFromFloat(10.00),
				},
			},
		}

		newItem := LineItem{
			ID:          uuid.Must(uuid.NewV4()),
			Description: "New item",
			Currency:    USD,
			Quantity:    decimal.NewFromFloat(2.0),
			UnitPrice:   decimal.NewFromFloat(15.00),
		}

		initialCount := len(bill.LineItems)
		success := bill.AddLineItem(newItem)

		assert.True(t, success)
		assert.Equal(t, initialCount+1, len(bill.LineItems))
		assert.Equal(t, "New item", bill.LineItems[len(bill.LineItems)-1].Description)
	})

	t.Run("when bill is closed", func(t *testing.T) {
		bill := &Bill{
			Status: BillStatusClosed,
			LineItems: []*LineItem{
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "Existing item",
					Currency:    USD,
					Quantity:    decimal.NewFromFloat(1.0),
					UnitPrice:   decimal.NewFromFloat(10.00),
				},
			},
		}

		newItem := LineItem{
			ID:          uuid.Must(uuid.NewV4()),
			Description: "New item",
			Currency:    USD,
			Quantity:    decimal.NewFromFloat(2.0),
			UnitPrice:   decimal.NewFromFloat(15.00),
		}

		initialCount := len(bill.LineItems)
		success := bill.AddLineItem(newItem)

		assert.False(t, success)
		assert.Equal(t, initialCount, len(bill.LineItems))
	})

	t.Run("when bill has no line items", func(t *testing.T) {
		bill := &Bill{
			Status:    BillStatusOpen,
			LineItems: []*LineItem{},
		}

		newItem := LineItem{
			ID:          uuid.Must(uuid.NewV4()),
			Description: "First item",
			Currency:    USD,
			Quantity:    decimal.NewFromFloat(1.0),
			UnitPrice:   decimal.NewFromFloat(10.00),
		}

		success := bill.AddLineItem(newItem)

		assert.True(t, success)
		assert.Equal(t, 1, len(bill.LineItems))
		assert.Equal(t, "First item", bill.LineItems[0].Description)
	})
}

func TestBill_Close(t *testing.T) {
	t.Run("when bill is open", func(t *testing.T) {
		now := time.Now()
		bill := &Bill{
			Status: BillStatusOpen,
		}

		success := bill.Close(now)

		assert.True(t, success)
		assert.Equal(t, BillStatusClosed, bill.Status)
		assert.Equal(t, &now, bill.ClosedAt)
	})

	t.Run("when bill is already closed", func(t *testing.T) {
		originalTime := time.Now().Add(-time.Hour)
		now := time.Now()
		bill := &Bill{
			Status:   BillStatusClosed,
			ClosedAt: &originalTime,
		}

		success := bill.Close(now)

		assert.False(t, success)
		assert.Equal(t, BillStatusClosed, bill.Status)
		assert.Equal(t, &originalTime, bill.ClosedAt) // Should not change
	})
}

func TestBill_CalculateSum(t *testing.T) {
	t.Run("with single currency line items", func(t *testing.T) {
		bill := &Bill{
			LineItems: []*LineItem{
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "Item 1",
					Currency:    USD,
					Quantity:    decimal.NewFromFloat(2.0),
					UnitPrice:   decimal.NewFromFloat(10.00),
				},
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "Item 2",
					Currency:    USD,
					Quantity:    decimal.NewFromFloat(1.0),
					UnitPrice:   decimal.NewFromFloat(15.00),
				},
			},
		}

		// Mock rates data
		rates := &RatesData{
			Rates: map[string]float64{
				"USD": 1.0,
				"GEL": 2.7,
			},
			UpdatedAt: time.Now(),
		}

		err := bill.CalculateSum(rates)

		assert.NoError(t, err)
		assert.Equal(t, decimal.NewFromFloat(35.00), bill.Total.ByCurrency[USD])
		assert.Equal(t, decimal.NewFromFloat(35.00), bill.Total.Converted[USD].Amount)
	})

	t.Run("with multiple currency line items", func(t *testing.T) {
		bill := &Bill{
			LineItems: []*LineItem{
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "USD Item",
					Currency:    USD,
					Quantity:    decimal.NewFromFloat(1.0),
					UnitPrice:   decimal.NewFromFloat(10.00),
				},
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "GEL Item",
					Currency:    GEL,
					Quantity:    decimal.NewFromFloat(2.0),
					UnitPrice:   decimal.NewFromFloat(5.00),
				},
			},
		}

		// Mock rates data
		rates := &RatesData{
			Rates: map[string]float64{
				"USD": 1.0,
				"GEL": 2.5,
			},
			UpdatedAt: time.Now(),
		}

		err := bill.CalculateSum(rates)

		assert.NoError(t, err)
		assert.True(t, decimal.NewFromFloat(10.00).Equal(bill.Total.ByCurrency[USD]))
		assert.True(t, decimal.NewFromFloat(10.00).Equal(bill.Total.ByCurrency[GEL]))

		// Check converted totals
		usdConverted := bill.Total.Converted[USD]
		assert.Equal(t, rates.UpdatedAt, usdConverted.RateUpdatedAt)
		assert.True(t, decimal.NewFromFloat(14).Equal(usdConverted.Amount))

		gelConverted := bill.Total.Converted[GEL]
		assert.Equal(t, rates.UpdatedAt, gelConverted.RateUpdatedAt)
		assert.True(t, decimal.NewFromFloat(35).Equal(gelConverted.Amount))
	})

	t.Run("with missing currency rates", func(t *testing.T) {
		bill := &Bill{
			LineItems: []*LineItem{
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "USD Item",
					Currency:    USD,
					Quantity:    decimal.NewFromFloat(1.0),
					UnitPrice:   decimal.NewFromFloat(10.00),
				},
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "GEL Item",
					Currency:    GEL,
					Quantity:    decimal.NewFromFloat(1.0),
					UnitPrice:   decimal.NewFromFloat(10.00),
				},
			},
		}

		// Mock rates data missing USD
		rates := &RatesData{
			Rates: map[string]float64{
				"GEL": 2.7,
			},
			UpdatedAt: time.Now(),
		}

		err := bill.CalculateSum(rates)

		assert.Error(t, err)
		assert.Equal(t, ErrCurrencyNotFound, err)
	})

	t.Run("with zero quantity and price", func(t *testing.T) {
		bill := &Bill{
			LineItems: []*LineItem{
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "Zero Item",
					Currency:    USD,
					Quantity:    decimal.Zero,
					UnitPrice:   decimal.Zero,
				},
			},
		}

		rates := &RatesData{
			Rates: map[string]float64{
				"USD": 1.0,
			},
			UpdatedAt: time.Now(),
		}

		err := bill.CalculateSum(rates)

		assert.NoError(t, err)
		assert.True(t, decimal.Zero.Equal(bill.Total.ByCurrency[USD]))
	})

	t.Run("with empty line items", func(t *testing.T) {
		bill := &Bill{
			LineItems: []*LineItem{},
		}

		rates := &RatesData{
			Rates: map[string]float64{
				"USD": 1.0,
			},
			UpdatedAt: time.Now(),
		}

		err := bill.CalculateSum(rates)

		assert.NoError(t, err)
		assert.Nil(t, bill.Total)
	})
}

func TestLineItem_TotalCalculation(t *testing.T) {
	t.Run("basic multiplication", func(t *testing.T) {
		item := &LineItem{
			Quantity:  decimal.NewFromFloat(3.0),
			UnitPrice: decimal.NewFromFloat(5.50),
		}

		// Calculate total
		item.Total = item.UnitPrice.Mul(item.Quantity)

		expected := decimal.NewFromFloat(16.50)
		assert.True(t, item.Total.Equal(expected))
	})

	t.Run("with decimal precision", func(t *testing.T) {
		item := &LineItem{
			Quantity:  decimal.NewFromFloat(2.5),
			UnitPrice: decimal.NewFromFloat(3.33),
		}

		// Calculate total
		item.Total = item.UnitPrice.Mul(item.Quantity)

		expected := decimal.NewFromFloat(8.325)
		assert.True(t, item.Total.Equal(expected))
	})

	t.Run("with zero values", func(t *testing.T) {
		item := &LineItem{
			Quantity:  decimal.Zero,
			UnitPrice: decimal.NewFromFloat(10.00),
		}

		// Calculate total
		item.Total = item.UnitPrice.Mul(item.Quantity)

		assert.True(t, item.Total.Equal(decimal.Zero))
	})
}

func TestBill_JSONSerialization(t *testing.T) {
	t.Run("serialize and deserialize bill", func(t *testing.T) {
		originalBill := &Bill{
			ID:          uuid.Must(uuid.NewV4()),
			CustomerID:  "customer-123",
			Status:      BillStatusOpen,
			PeriodStart: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			PeriodEnd:   time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC),
			WorkflowID:  "workflow-123",
			CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			LineItems: []*LineItem{
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "Test service",
					Currency:    USD,
					Quantity:    decimal.NewFromFloat(1.0),
					UnitPrice:   decimal.NewFromFloat(10.00),
					CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		}

		// Add closed timestamp
		closedAt := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
		originalBill.Close(closedAt)

		// Serialize to JSON
		jsonData, err := json.Marshal(originalBill)
		assert.NoError(t, err)

		// Deserialize from JSON
		var deserializedBill Bill
		err = json.Unmarshal(jsonData, &deserializedBill)
		assert.NoError(t, err)

		// Verify fields
		assert.Equal(t, originalBill.ID, deserializedBill.ID)
		assert.Equal(t, originalBill.CustomerID, deserializedBill.CustomerID)
		assert.Equal(t, originalBill.Status, deserializedBill.Status)
		assert.Equal(t, originalBill.PeriodStart.Unix(), deserializedBill.PeriodStart.Unix())
		assert.Equal(t, originalBill.PeriodEnd.Unix(), deserializedBill.PeriodEnd.Unix())
		assert.Equal(t, originalBill.WorkflowID, deserializedBill.WorkflowID)
		assert.Equal(t, originalBill.CreatedAt.Unix(), deserializedBill.CreatedAt.Unix())
		assert.Equal(t, originalBill.UpdatedAt.Unix(), deserializedBill.UpdatedAt.Unix())
		assert.Equal(t, originalBill.ClosedAt.Unix(), deserializedBill.ClosedAt.Unix())
		assert.Len(t, deserializedBill.LineItems, 1)
		assert.Equal(t, originalBill.LineItems[0].Description, deserializedBill.LineItems[0].Description)
	})
}

func TestBill_EdgeCases(t *testing.T) {
	t.Run("bill with nil line items", func(t *testing.T) {
		bill := &Bill{
			Status:    BillStatusOpen,
			LineItems: nil,
		}

		// Should not panic
		assert.False(t, bill.IsClosed())
		assert.True(t, bill.IsOpen())

		// AddLineItem should handle nil slice
		item := LineItem{
			ID:          uuid.Must(uuid.NewV4()),
			Description: "Test item",
			Currency:    USD,
			Quantity:    decimal.NewFromFloat(1.0),
			UnitPrice:   decimal.NewFromFloat(10.00),
		}

		success := bill.AddLineItem(item)
		assert.True(t, success)
		assert.Len(t, bill.LineItems, 1)
	})

	t.Run("bill with very large amounts", func(t *testing.T) {
		bill := &Bill{
			Status: BillStatusOpen,
			LineItems: []*LineItem{
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "Large amount item",
					Currency:    USD,
					Quantity:    decimal.NewFromFloat(1000000.0),
					UnitPrice:   decimal.NewFromFloat(1000000.0),
				},
			},
		}

		rates := &RatesData{
			Rates: map[string]float64{
				"USD": 1.0,
			},
			UpdatedAt: time.Now(),
		}

		err := bill.CalculateSum(rates)
		assert.NoError(t, err)

		expectedTotal := decimal.NewFromFloat(1000000000000.0) // 1 trillion
		assert.True(t, bill.Total.ByCurrency[USD].Equal(expectedTotal))
	})

	t.Run("bill with negative amounts (should be handled gracefully)", func(t *testing.T) {
		bill := &Bill{
			Status: BillStatusOpen,
			LineItems: []*LineItem{
				{
					ID:          uuid.Must(uuid.NewV4()),
					Description: "Negative amount item",
					Currency:    USD,
					Quantity:    decimal.NewFromFloat(-1.0),
					UnitPrice:   decimal.NewFromFloat(10.00),
				},
			},
		}

		rates := &RatesData{
			Rates: map[string]float64{
				"USD": 1.0,
			},
			UpdatedAt: time.Now(),
		}

		err := bill.CalculateSum(rates)
		assert.NoError(t, err)

		expectedTotal := decimal.NewFromFloat(-10.0)
		assert.True(t, bill.Total.ByCurrency[USD].Equal(expectedTotal))
	})
}
