package config

const (
	// MaxRetries is the maximum number of payment attempts across all processors.
	MaxRetries = 3

	// HealthWindowSize is the number of recent transactions to consider for health calculation.
	HealthWindowSize = 50

	// HealthWindowDuration is the time window for health calculation.
	HealthWindowDurationMinutes = 10

	// DegradedThreshold is the health score below which a processor is considered degraded.
	DegradedThreshold = 0.5

	// CircuitBreakerThreshold is the health score below which a processor is skipped entirely.
	CircuitBreakerThreshold = 0.2

	// ServerPort is the default HTTP server port.
	ServerPort = ":8080"
)
