package health

import (
	"sync"
	"time"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/config"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/model"
)

// Status represents the health status of a processor.
type Status string

const (
	StatusHealthy  Status = "healthy"
	StatusDegraded Status = "degraded"
	StatusOpen     Status = "circuit_open"
)

// ProcessorHealth contains the current health information for a processor.
type ProcessorHealth struct {
	ProcessorName string    `json:"processor_name"`
	HealthScore   float64   `json:"health_score"`
	Status        Status    `json:"status"`
	TotalRecent   int       `json:"total_recent"`
	ApprovedCount int       `json:"approved_count"`
	ErrorCount    int       `json:"error_count"`
	LastUpdated   time.Time `json:"last_updated"`
}

// outcome records a single transaction outcome.
type outcome struct {
	approved  bool
	timestamp time.Time
}

// Monitor tracks processor health using a sliding window.
type Monitor struct {
	mu             sync.RWMutex
	windows        map[string][]outcome
	windowSize     int
	windowDuration time.Duration
}

// NewMonitor creates a new health monitor with default configuration.
func NewMonitor() *Monitor {
	return &Monitor{
		windows:        make(map[string][]outcome),
		windowSize:     config.HealthWindowSize,
		windowDuration: time.Duration(config.HealthWindowDurationMinutes) * time.Minute,
	}
}

// NewMonitorWithConfig creates a monitor with custom window settings for testing.
func NewMonitorWithConfig(windowSize int, windowDuration time.Duration) *Monitor {
	return &Monitor{
		windows:        make(map[string][]outcome),
		windowSize:     windowSize,
		windowDuration: windowDuration,
	}
}

// RecordOutcome records a transaction outcome for a processor.
func (m *Monitor) RecordOutcome(processorName string, code model.ResponseCode) {
	m.mu.Lock()
	defer m.mu.Unlock()

	approved := code == model.Approved
	m.windows[processorName] = append(m.windows[processorName], outcome{
		approved:  approved,
		timestamp: time.Now(),
	})

	m.pruneWindow(processorName)
}

// GetHealth returns the current health information for a processor.
func (m *Monitor) GetHealth(processorName string) ProcessorHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	window := m.getActiveWindow(processorName)

	if len(window) == 0 {
		return ProcessorHealth{
			ProcessorName: processorName,
			HealthScore:   1.0, // New/unknown processors default to healthy
			Status:        StatusHealthy,
			TotalRecent:   0,
			ApprovedCount: 0,
			ErrorCount:    0,
			LastUpdated:   time.Now(),
		}
	}

	approved := 0
	errors := 0
	for _, o := range window {
		if o.approved {
			approved++
		} else {
			errors++
		}
	}

	total := len(window)
	score := float64(approved) / float64(total)

	status := StatusHealthy
	if score < config.CircuitBreakerThreshold {
		status = StatusOpen
	} else if score < config.DegradedThreshold {
		status = StatusDegraded
	}

	return ProcessorHealth{
		ProcessorName: processorName,
		HealthScore:   score,
		Status:        status,
		TotalRecent:   total,
		ApprovedCount: approved,
		ErrorCount:    errors,
		LastUpdated:   time.Now(),
	}
}

// GetAllHealth returns health information for all tracked processors.
func (m *Monitor) GetAllHealth() []ProcessorHealth {
	m.mu.RLock()
	processors := make([]string, 0, len(m.windows))
	for name := range m.windows {
		processors = append(processors, name)
	}
	m.mu.RUnlock()

	healths := make([]ProcessorHealth, 0, len(processors))
	for _, name := range processors {
		healths = append(healths, m.GetHealth(name))
	}
	return healths
}

// IsCircuitOpen returns true if the processor's circuit breaker is open (should be skipped).
func (m *Monitor) IsCircuitOpen(processorName string) bool {
	h := m.GetHealth(processorName)
	return h.Status == StatusOpen
}

// getActiveWindow returns outcomes within the time window, already under read lock.
func (m *Monitor) getActiveWindow(processorName string) []outcome {
	window := m.windows[processorName]
	if len(window) == 0 {
		return nil
	}

	cutoff := time.Now().Add(-m.windowDuration)
	active := make([]outcome, 0, len(window))
	for _, o := range window {
		if o.timestamp.After(cutoff) {
			active = append(active, o)
		}
	}

	// Also limit by window size (most recent N)
	if len(active) > m.windowSize {
		active = active[len(active)-m.windowSize:]
	}

	return active
}

// pruneWindow removes expired outcomes, called under write lock.
func (m *Monitor) pruneWindow(processorName string) {
	cutoff := time.Now().Add(-m.windowDuration)
	window := m.windows[processorName]

	pruned := make([]outcome, 0, len(window))
	for _, o := range window {
		if o.timestamp.After(cutoff) {
			pruned = append(pruned, o)
		}
	}

	// Keep only last windowSize entries
	if len(pruned) > m.windowSize {
		pruned = pruned[len(pruned)-m.windowSize:]
	}

	m.windows[processorName] = pruned
}
