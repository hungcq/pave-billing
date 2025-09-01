package repository

import (
	"context"
	"database/sql"
	"time"

	"encore.app/billing/models"
	"encore.dev/rlog"
	"encore.dev/storage/sqldb"
	"encore.dev/types/uuid"
)

// Repository defines the interface for data persistence
type Repository interface {
	// Bill operations
	CreateBill(ctx context.Context, bill *models.Bill) error
	GetBillByID(ctx context.Context, billID uuid.UUID) (*models.Bill, error)
	CloseBill(ctx context.Context, billID uuid.UUID, closedAt time.Time) error

	// Line item operations
	AddLineItemToBill(ctx context.Context, lineItem *models.LineItem) error
	GetLineItemsByBillID(ctx context.Context, billID uuid.UUID) ([]*models.LineItem, error)
}

// SQLRepository implements Repository using SQL database
type SQLRepository struct {
	db *sqldb.Database
}

// NewSQLRepository creates a new SQL repository
func NewSQLRepository(db *sqldb.Database) Repository {
	log := rlog.With("module", "billing_repository")
	log.Info("SQL repository initialized", "database_available", db != nil)
	return &SQLRepository{db: db}
}

func (r *SQLRepository) CreateBill(ctx context.Context, bill *models.Bill) error {
	log := rlog.With("module", "billing_repository").With("bill_id", bill.ID.String()).With("customer_id", bill.CustomerID)
	log.Info("creating bill in database", "status", bill.Status, "workflow_id", bill.WorkflowID)

	query := `
		INSERT INTO bills (id, customer_id, status, period_start, period_end, workflow_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := r.db.Exec(ctx, query,
		bill.ID,
		bill.CustomerID,
		bill.Status,
		bill.PeriodStart,
		bill.PeriodEnd,
		bill.WorkflowID,
		bill.CreatedAt,
		bill.UpdatedAt,
	)

	if err != nil {
		log.Error("failed to create bill in database", "error", err)
		return err
	}

	log.Info("bill created successfully in database")
	return nil
}

func (r *SQLRepository) GetBillByID(ctx context.Context, billID uuid.UUID) (*models.Bill, error) {
	log := rlog.With("module", "billing_repository").With("bill_id", billID.String())
	log.Info("retrieving bill from database")

	query := `
		SELECT id, customer_id, status, period_start, period_end, workflow_id, created_at, updated_at, closed_at
		FROM bills 
		WHERE id = $1
	`

	var bill models.Bill
	var closedAt sql.NullTime

	err := r.db.QueryRow(ctx, query, billID).Scan(
		&bill.ID,
		&bill.CustomerID,
		&bill.Status,
		&bill.PeriodStart,
		&bill.PeriodEnd,
		&bill.WorkflowID,
		&bill.CreatedAt,
		&bill.UpdatedAt,
		&closedAt,
	)

	if err != nil {
		log.Error("failed to retrieve bill from database", "error", err)
		return nil, err
	}

	if closedAt.Valid {
		bill.ClosedAt = &closedAt.Time
		log.Debug("bill has closed timestamp", "closed_at", closedAt.Time)
	}

	// Load line items
	log.Info("loading line items for bill")
	lineItems, err := r.GetLineItemsByBillID(ctx, billID)
	if err != nil {
		log.Error("failed to load line items for bill", "error", err)
		return nil, err
	}
	bill.LineItems = lineItems

	log.Info("bill retrieved successfully from database",
		"status", bill.Status,
		"line_items_count", len(lineItems),
		"customer_id", bill.CustomerID)

	return &bill, nil
}

func (r *SQLRepository) CloseBill(ctx context.Context, billID uuid.UUID, closedAt time.Time) error {
	log := rlog.With("module", "billing_repository").With("bill_id", billID.String()).With("closed_at", closedAt)
	log.Info("closing bill in database")

	query := `
		UPDATE bills 
		SET status = 'closed', closed_at = $1, updated_at = NOW()
		WHERE id = $2 AND status = 'open'
	`

	result, err := r.db.Exec(ctx, query, closedAt, billID)
	if err != nil {
		log.Error("failed to close bill in database", "error", err)
		return err
	}

	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		log.Warn("no rows affected when closing bill - bill may already be closed or not found")
		return sql.ErrNoRows
	}

	log.Info("bill closed successfully in database", "rows_affected", rowsAffected)
	return nil
}

// GetLineItemsByBillID retrieves all line items for a bill
func (r *SQLRepository) GetLineItemsByBillID(ctx context.Context, billID uuid.UUID) ([]*models.LineItem, error) {
	log := rlog.With("module", "billing_repository").With("bill_id", billID.String())
	log.Debug("retrieving line items for bill")

	query := `
		SELECT id, bill_id, description, currency, quantity, unit_price, created_at
		FROM line_items 
		WHERE bill_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.Query(ctx, query, billID)
	if err != nil {
		log.Error("failed to query line items", "error", err)
		return nil, err
	}
	defer rows.Close()

	lineItems := make([]*models.LineItem, 0)
	for rows.Next() {
		lineItem := &models.LineItem{}

		err := rows.Scan(
			&lineItem.ID,
			&lineItem.BillID,
			&lineItem.Description,
			&lineItem.Currency,
			&lineItem.Quantity,
			&lineItem.UnitPrice,
			&lineItem.CreatedAt,
		)
		if err != nil {
			log.Error("failed to scan line item row", "error", err)
			return nil, err
		}

		lineItems = append(lineItems, lineItem)
	}

	log.Debug("line items retrieved successfully", "count", len(lineItems))
	return lineItems, nil
}

func (r *SQLRepository) AddLineItemToBill(ctx context.Context, lineItem *models.LineItem) error {
	log := rlog.With("module", "billing_repository").With("bill_id", lineItem.BillID.String()).With("line_item_id", lineItem.ID.String())
	log.Info("adding line item to bill in database",
		"description", lineItem.Description,
		"currency", lineItem.Currency,
		"quantity", lineItem.Quantity,
		"unit_price", lineItem.UnitPrice)

	lineItemQuery := `
		INSERT INTO line_items (id, bill_id, description, currency, quantity, unit_price, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := r.db.Exec(ctx, lineItemQuery,
		lineItem.ID,
		lineItem.BillID,
		lineItem.Description,
		lineItem.Currency,
		lineItem.Quantity,
		lineItem.UnitPrice,
		lineItem.CreatedAt,
	)

	if err != nil {
		log.Error("failed to add line item to bill in database", "error", err)
		return err
	}

	log.Info("line item added successfully to bill in database")
	return nil
}
