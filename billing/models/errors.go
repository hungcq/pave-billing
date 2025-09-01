package models

import (
	"encore.dev/beta/errs"
)

var (
	// ErrBillNotFound is returned when a bill is not found
	ErrBillNotFound = &errs.Error{
		Code:    errs.NotFound,
		Message: "bill not found",
	}

	// ErrBillClosed is returned when trying to modify a closed bill
	ErrBillClosed = &errs.Error{
		Code:    errs.FailedPrecondition,
		Message: "bill is closed and cannot be modified",
	}

	// ErrInvalidCurrency is returned when an invalid currency is provided
	ErrInvalidCurrency = &errs.Error{
		Code:    errs.InvalidArgument,
		Message: "unsupported currency",
	}

	// ErrCurrencyNotFound is returned when an invalid currency is provided
	ErrCurrencyNotFound = &errs.Error{
		Code:    errs.NotFound,
		Message: "currency not found",
	}

	// ErrInvalidBillStatus is returned when an invalid bill status is provided
	ErrInvalidBillStatus = &errs.Error{
		Code:    errs.InvalidArgument,
		Message: "invalid bill status, supported statuses are open and closed",
	}

	// ErrInvalidPeriod is returned when period_end is before period_start
	ErrInvalidPeriod = &errs.Error{
		Code:    errs.InvalidArgument,
		Message: "period_end must be after period_start",
	}

	// ErrInvalidQuantity is returned when quantity is zero or negative
	ErrInvalidQuantity = &errs.Error{
		Code:    errs.InvalidArgument,
		Message: "quantity must be greater than zero",
	}
)
