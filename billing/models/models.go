package models

import (
	"slices"
	"time"

	"encore.dev/types/uuid"
	"github.com/Rhymond/go-money"
	"github.com/shopspring/decimal"
)

// Currency represents supported currencies
type Currency string

const (
	USD Currency = money.USD
	GEL Currency = money.GEL
)

// BillStatus represents the status of a bill
type BillStatus string

const (
	BillStatusOpen   BillStatus = "open"
	BillStatusClosed BillStatus = "closed"
)

// Bill represents a billing period with line items
type Bill struct {
	ID          uuid.UUID   `json:"id" db:"id"`
	CustomerID  string      `json:"customer_id" db:"customer_id"`
	Status      BillStatus  `json:"status" db:"status"`
	PeriodStart time.Time   `json:"period_start" db:"period_start"`
	PeriodEnd   time.Time   `json:"period_end" db:"period_end"`
	WorkflowID  string      `json:"workflow_id" db:"workflow_id"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at" db:"updated_at"`
	ClosedAt    *time.Time  `json:"closed_at,omitempty" db:"closed_at"`
	LineItems   []*LineItem `json:"line_items,omitempty"`
	Total       *Total      `json:"total,omitempty"`
}

type Total struct {
	ByCurrency map[Currency]decimal.Decimal `json:"by_currency"`
	Converted  map[Currency]Converted       `json:"converted"`
}

type Converted struct {
	Amount        decimal.Decimal `json:"amount"`
	RateUpdatedAt time.Time       `json:"rate_updated_at"`
}

// LineItem represents an individual charge within a bill
type LineItem struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	BillID      uuid.UUID       `json:"bill_id" db:"bill_id"`
	Description string          `json:"description" db:"description"`
	Currency    Currency        `json:"currency" db:"currency"`
	Quantity    decimal.Decimal `json:"quantity" db:"quantity"`
	UnitPrice   decimal.Decimal `json:"unit_price" db:"unit_price"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	Total       decimal.Decimal `json:"total"`
}

func (c Currency) Validate(cfg *AppConfig) error {
	if slices.Contains(cfg.Billing.Validation.AllowedCurrencies(), string(c)) {
		return nil
	}
	return ErrInvalidCurrency
}

// Validate validates the bill status
func (s BillStatus) Validate() error {
	switch s {
	case BillStatusOpen, BillStatusClosed:
		return nil
	default:
		return ErrInvalidBillStatus
	}
}

func (b *Bill) IsOpen() bool {
	return b.Status == BillStatusOpen
}

func (b *Bill) IsClosed() bool {
	return b.Status == BillStatusClosed
}

func (b *Bill) AddLineItem(item LineItem) (success bool) {
	if b.IsClosed() {
		return false
	}
	b.LineItems = append(b.LineItems, &item)
	return true
}

func (b *Bill) Close(at time.Time) (success bool) {
	if b.IsClosed() {
		return false
	}
	b.Status = BillStatusClosed
	b.ClosedAt = &at
	return true
}

func (b *Bill) CalculateSum(rates *RatesData) error {
	if len(b.LineItems) == 0 {
		return nil
	}

	for _, item := range b.LineItems {
		item.Total = item.UnitPrice.Mul(item.Quantity)
	}

	b.Total = &Total{}
	b.Total.ByCurrency = make(map[Currency]decimal.Decimal)
	for _, item := range b.LineItems {
		b.Total.ByCurrency[item.Currency] = b.Total.ByCurrency[item.Currency].Add(item.UnitPrice.Mul(item.Quantity))
	}

	b.Total.Converted = make(map[Currency]Converted)
	for currency, amount := range b.Total.ByCurrency {
		sum := amount
		for other, amountOther := range b.Total.ByCurrency {
			if other == currency {
				continue
			}
			fromX, ok := rates.Rates[string(other)]
			if !ok {
				return ErrCurrencyNotFound
			}
			toX, ok := rates.Rates[string(currency)]
			if !ok {
				return ErrCurrencyNotFound
			}

			converted := amountOther.
				Mul(decimal.NewFromFloat(toX / fromX)).
				Round(int32(money.GetCurrency(string(currency)).Fraction))

			sum = sum.Add(converted)
		}
		b.Total.Converted[currency] = Converted{
			Amount:        sum,
			RateUpdatedAt: rates.UpdatedAt,
		}
	}
	return nil
}

type RatesData struct {
	Rates     map[string]float64
	UpdatedAt time.Time
}
