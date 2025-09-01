package models

import (
	"time"

	"encore.dev/types/uuid"
	"github.com/shopspring/decimal"
)

// CreateBillRequest represents the request to create a new bill
type CreateBillRequest struct {
	CustomerID  string    `json:"customer_id" validate:"required"`
	PeriodStart time.Time `json:"period_start" validate:"required"`
	PeriodEnd   time.Time `json:"period_end" validate:"required"`
}

// BillResponse represents the response after creating a bill
type BillResponse struct {
	Data *Bill `json:"data"`
}

// AddLineItemRequest represents the request to add a line item to a bill
type AddLineItemRequest struct {
	Description string          `json:"description" validate:"required"`
	Currency    Currency        `json:"currency" validate:"required"`
	Quantity    decimal.Decimal `json:"quantity" validate:"required,gt=0"`
	UnitPrice   decimal.Decimal `json:"unit_price" validate:"required"`
}

// AddLineItemResponse represents the response after adding a line item
type AddLineItemResponse struct {
	Data *LineItem `json:"data"`
}

// GetBillRequest represents the request to get a bill by ID
type GetBillRequest struct {
	BillID uuid.UUID `json:"bill_id" validate:"required"`
}

// GetBillResponse represents the response when getting a bill
type GetBillResponse struct {
	Data *Bill `json:"data"`
}

// ListBillsRequest represents the request to list bills
type ListBillsRequest struct {
	CustomerID string `query:"customer_id"`
	Status     string `query:"status"`
	Currency   string `query:"currency"`
	Limit      int    `query:"limit"`
	Offset     int    `query:"offset"`
}
