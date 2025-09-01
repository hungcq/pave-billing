package billing

import (
	"fmt"
	"time"

	"encore.app/billing/models"
	"encore.dev/beta/errs"
	"encore.dev/rlog"
	"github.com/shopspring/decimal"
)

func ValidateCreateBillRequest(req *models.CreateBillRequest) error {
	log := rlog.With("module", "billing_validation").With("customer_id", req.CustomerID)
	log.Debug("validating create bill request",
		"period_start", req.PeriodStart,
		"period_end", req.PeriodEnd)

	if req.CustomerID == "" {
		log.Warn("validation failed: customer_id is required")
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "customer_id is required",
		}
	}

	if req.PeriodStart.IsZero() {
		log.Warn("validation failed: period_start is required")
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "period_start is required",
		}
	}

	if req.PeriodEnd.IsZero() {
		log.Warn("validation failed: period_end is required")
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "period_end is required",
		}
	}

	if req.PeriodEnd.Before(req.PeriodStart) {
		log.Warn("validation failed: period_end is before period_start",
			"period_start", req.PeriodStart,
			"period_end", req.PeriodEnd)
		return models.ErrInvalidPeriod
	}

	// Check if period is too long using configured maximum
	maxBillingPeriodDays := cfg.Billing.Validation.MaxBillingPeriodDays()
	maxBillingPeriod := time.Duration(maxBillingPeriodDays) * 24 * time.Hour
	periodDuration := req.PeriodEnd.Sub(req.PeriodStart)
	if periodDuration > maxBillingPeriod {
		log.Warn("validation failed: billing period too long",
			"period_duration", periodDuration,
			"max_allowed", maxBillingPeriod)
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("billing period cannot exceed %d days", maxBillingPeriodDays),
		}
	}

	// Check if period starts in the past using configured maximum
	maxPastStartDays := cfg.Billing.Validation.MaxPastStartDays()
	maxPastStart := time.Duration(maxPastStartDays) * 24 * time.Hour
	cutoffTime := time.Now().Add(-maxPastStart)
	if req.PeriodStart.Before(cutoffTime) {
		log.Warn("validation failed: period_start too far in the past",
			"period_start", req.PeriodStart,
			"cutoff_time", cutoffTime)
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("period_start cannot be more than %d days in the past", maxPastStartDays),
		}
	}

	log.Debug("create bill request validation passed")
	return nil
}

func ValidateAddLineItemRequest(req *models.AddLineItemRequest) error {
	log := rlog.With("module", "billing_validation")
	log.Debug("validating add line item request",
		"description", req.Description,
		"currency", req.Currency,
		"quantity", req.Quantity,
		"unit_price", req.UnitPrice)

	if req.Description == "" {
		log.Warn("validation failed: description is required")
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "description is required",
		}
	}

	// Use configured maximum description length
	maxDescriptionLength := cfg.Billing.Validation.MaxDescriptionLength()
	if len(req.Description) > maxDescriptionLength {
		log.Warn("validation failed: description too long",
			"description_length", len(req.Description),
			"max_length", maxDescriptionLength)
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("description cannot exceed %d characters", maxDescriptionLength),
		}
	}

	if err := req.Currency.Validate(cfg); err != nil {
		log.Warn("validation failed: invalid currency", "currency", req.Currency, "error", err)
		return err
	}

	if req.Quantity.LessThanOrEqual(decimal.Zero) {
		log.Warn("validation failed: invalid quantity", "quantity", req.Quantity)
		return models.ErrInvalidQuantity
	}

	// Check for reasonable quantity limits using configured maximum
	maxQuantity := decimal.NewFromFloat(cfg.Billing.Validation.MaxQuantity())
	if req.Quantity.GreaterThan(maxQuantity) {
		log.Warn("validation failed: quantity too high",
			"quantity", req.Quantity,
			"max_quantity", maxQuantity)
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("quantity cannot exceed %f", maxQuantity),
		}
	}

	if req.UnitPrice.LessThan(decimal.Zero) {
		log.Warn("validation failed: negative unit price", "unit_price", req.UnitPrice)
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: "unit_price cannot be negative",
		}
	}

	// Check for reasonable price limits using configured maximum
	maxUnitPrice := decimal.NewFromFloat(cfg.Billing.Validation.MaxUnitPrice())
	if req.UnitPrice.GreaterThan(maxUnitPrice) {
		log.Warn("validation failed: unit price too high",
			"unit_price", req.UnitPrice,
			"max_unit_price", maxUnitPrice)
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("unit_price cannot exceed %f", maxUnitPrice),
		}
	}

	// Calculate total amount and check limits using configured maximum
	totalAmount := req.Quantity.Mul(req.UnitPrice)
	maxTotalAmount := decimal.NewFromFloat(cfg.Billing.Validation.MaxTotalAmount())
	if totalAmount.GreaterThan(maxTotalAmount) {
		log.Warn("validation failed: total amount too high",
			"total_amount", totalAmount,
			"max_total_amount", maxTotalAmount,
			"quantity", req.Quantity,
			"unit_price", req.UnitPrice)
		return &errs.Error{
			Code:    errs.InvalidArgument,
			Message: fmt.Sprintf("total line item amount cannot exceed %f", maxTotalAmount),
		}
	}

	log.Debug("add line item request validation passed", "total_amount", totalAmount)
	return nil
}
