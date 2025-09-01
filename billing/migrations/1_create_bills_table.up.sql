-- Bills table for storing billing information
CREATE TABLE bills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('initializing', 'open', 'closed')) DEFAULT 'open',
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    workflow_id VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    closed_at TIMESTAMPTZ NULL
);

-- Line items table for storing individual charges
CREATE TABLE line_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bill_id UUID NOT NULL REFERENCES bills(id) ON DELETE CASCADE,
    description TEXT NOT NULL,
    currency VARCHAR(3) NOT NULL,
    quantity DECIMAL(10,4) NOT NULL DEFAULT 1.0000,
    unit_price DECIMAL(15,4) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

-- Indexes for better query performance
CREATE INDEX idx_bills_customer_id ON bills(customer_id);
CREATE INDEX idx_bills_status ON bills(status);
CREATE INDEX idx_bills_workflow_id ON bills(workflow_id);
CREATE INDEX idx_bills_period ON bills(period_start, period_end);
CREATE INDEX idx_line_items_bill_id ON line_items(bill_id);
CREATE INDEX idx_line_items_created_at ON line_items(created_at);

