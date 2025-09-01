package repository

import (
	"context"
	"time"

	"encore.app/billing/models"
	"encore.dev/types/uuid"
)

// FakeRepo is an in-memory repo used for testing
type FakeRepo struct {
	bills     map[uuid.UUID]*models.Bill
	lineItems map[uuid.UUID][]*models.LineItem
}

func (m *FakeRepo) CreateBill(ctx context.Context, bill *models.Bill) error {
	if m.bills == nil {
		m.bills = make(map[uuid.UUID]*models.Bill)
	}
	m.bills[bill.ID] = bill
	return nil
}

func (m *FakeRepo) GetBillByID(ctx context.Context, billID uuid.UUID) (*models.Bill, error) {
	if bill, exists := m.bills[billID]; exists {
		// Load line items
		if lineItems, exists := m.lineItems[billID]; exists {
			bill.LineItems = lineItems
		}
		return bill, nil
	}
	return nil, models.ErrBillNotFound
}

func (m *FakeRepo) CloseBill(ctx context.Context, billID uuid.UUID, closedAt time.Time) error {
	if bill, exists := m.bills[billID]; exists {
		bill.Status = models.BillStatusClosed
		bill.ClosedAt = &closedAt
		return nil
	}
	return models.ErrBillNotFound
}

func (m *FakeRepo) AddLineItemToBill(ctx context.Context, lineItem *models.LineItem) error {
	if m.lineItems == nil {
		m.lineItems = make(map[uuid.UUID][]*models.LineItem)
	}
	m.lineItems[lineItem.BillID] = append(m.lineItems[lineItem.BillID], lineItem)
	return nil
}

func (m *FakeRepo) GetLineItemsByBillID(ctx context.Context, billID uuid.UUID) ([]*models.LineItem, error) {
	if lineItems, exists := m.lineItems[billID]; exists {
		return lineItems, nil
	}
	return []*models.LineItem{}, nil
}
