package models

import "encore.dev/config"

// AppConfig holds the main application configuration
type AppConfig struct {
	// Temporal configuration
	Temporal TemporalConfig

	// External services configuration
	ExternalServices ExternalServicesConfig

	// Billing configuration
	Billing BillingConfig
}

// TemporalConfig holds Temporal workflow engine configuration
type TemporalConfig struct {
	// Connection settings
	Address   config.String
	Namespace config.String
	TaskQueue config.String

	// Workflow settings
	WorkflowExecutionTimeoutBuffer config.Int // in seconds
	ActivityStartToCloseTimeout    config.Int // in seconds
	ActivityRetryPolicy            ActivityRetryPolicy
}

// ActivityRetryPolicy holds Temporal activity retry configuration
type ActivityRetryPolicy struct {
	InitialInterval    config.Int // in seconds
	BackoffCoefficient config.Float64
	MaximumInterval    config.Int // in seconds
	MaximumAttempts    config.Int
}

// ExternalServicesConfig holds external service configuration
type ExternalServicesConfig struct {
	ExchangeRates ExchangeRatesConfig
}

// ExchangeRatesConfig holds exchange rate service configuration
type ExchangeRatesConfig struct {
	// API configuration
	BaseURL config.String

	// Cache configuration
	TTL      config.Int // in seconds
	CacheKey config.String

	// HTTP client configuration
	Timeout config.Int // in seconds
}

// BillingConfig holds billing-specific configuration
type BillingConfig struct {
	// Validation rules
	Validation ValidationConfig

	// Workflow settings
	Workflow WorkflowConfig
}

// ValidationConfig holds validation rule configuration
type ValidationConfig struct {
	// Period constraints
	MaxBillingPeriodDays config.Int
	MaxPastStartDays     config.Int

	// Line item constraints
	MaxDescriptionLength config.Int
	MaxQuantity          config.Float64
	MaxUnitPrice         config.Float64
	MaxTotalAmount       config.Float64
	AllowedCurrencies    config.Values[string]
}

// WorkflowConfig holds workflow-specific configuration
type WorkflowConfig struct {
	WorkflowIDPrefix config.String
}
