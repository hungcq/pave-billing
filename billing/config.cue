package billing

Temporal: {
	Address:                           string | *"eu-central-1.aws.api.temporal.io:7233"
	Namespace:                         "quickstart-pave-billing.l5kfr"
	TaskQueue:                         "pave-billing"
	WorkflowExecutionTimeoutBuffer:    3600 // 1 hour
	ActivityStartToCloseTimeout:       604800 // 1 week
	ActivityRetryPolicy: {
		InitialInterval:    5 // second
		BackoffCoefficient: 2.0
		MaximumInterval:    3600 // seconds
		MaximumAttempts:    100
	}
}

ExternalServices: {
	ExchangeRates: {
		BaseURL:    "https://openexchangerates.org/api/latest.json"
		TTL:        86400 // 24 hours
		CacheKey:   "exchange_rates"
		Timeout:    30 // seconds
	}
}

Billing: {
	Validation: {
		MaxBillingPeriodDays: 365
		MaxPastStartDays:     1
		MaxDescriptionLength: 500
		MaxQuantity:          1000000
		MaxUnitPrice:         1000000
		MaxTotalAmount:       10000000
		AllowedCurrencies: 		["USD", "GEL"]
	}
	Workflow: {
		WorkflowIDPrefix: "bill-"
	}
}

// An application running due to `encore run`
if #Meta.Environment.Type == "development" && #Meta.Environment.Cloud == "local" {
	Temporal: {
		Address: "localhost:7233"
	}
}