package health

import (
	"sync"
	"testing"
	"time"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMonitor_DefaultsToHealthy(t *testing.T) {
	m := NewMonitor()
	h := m.GetHealth("UnknownProcessor")

	assert.Equal(t, "UnknownProcessor", h.ProcessorName)
	assert.Equal(t, 1.0, h.HealthScore)
	assert.Equal(t, StatusHealthy, h.Status)
	assert.Equal(t, 0, h.TotalRecent)
	assert.Equal(t, 0, h.ApprovedCount)
	assert.Equal(t, 0, h.ErrorCount)
}

func TestMonitor_HealthScoreCalculation(t *testing.T) {
	tests := []struct {
		name           string
		approvals      int
		failures       int
		expectedScore  float64
		expectedStatus Status
	}{
		{
			name:           "all approved",
			approvals:      10,
			failures:       0,
			expectedScore:  1.0,
			expectedStatus: StatusHealthy,
		},
		{
			name:           "all failed",
			approvals:      0,
			failures:       10,
			expectedScore:  0.0,
			expectedStatus: StatusOpen,
		},
		{
			name:           "70% approval - healthy",
			approvals:      7,
			failures:       3,
			expectedScore:  0.7,
			expectedStatus: StatusHealthy,
		},
		{
			name:           "30% approval - degraded",
			approvals:      3,
			failures:       7,
			expectedScore:  0.3,
			expectedStatus: StatusDegraded,
		},
		{
			name:           "10% approval - circuit open",
			approvals:      1,
			failures:       9,
			expectedScore:  0.1,
			expectedStatus: StatusOpen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMonitorWithConfig(50, 10*time.Minute)

			for i := 0; i < tt.approvals; i++ {
				m.RecordOutcome("TestProc", model.Approved)
			}
			for i := 0; i < tt.failures; i++ {
				m.RecordOutcome("TestProc", model.ProcessorError)
			}

			h := m.GetHealth("TestProc")
			assert.InDelta(t, tt.expectedScore, h.HealthScore, 0.001,
				"expected score %.3f, got %.3f", tt.expectedScore, h.HealthScore)
			assert.Equal(t, tt.expectedStatus, h.Status)
			assert.Equal(t, tt.approvals+tt.failures, h.TotalRecent)
			assert.Equal(t, tt.approvals, h.ApprovedCount)
			assert.Equal(t, tt.failures, h.ErrorCount)
		})
	}
}

func TestMonitor_BoundaryThresholds(t *testing.T) {
	t.Run("exactly at degraded threshold (0.5)", func(t *testing.T) {
		m := NewMonitorWithConfig(50, 10*time.Minute)
		// 5 approvals, 5 failures = exactly 0.5
		for i := 0; i < 5; i++ {
			m.RecordOutcome("Proc", model.Approved)
		}
		for i := 0; i < 5; i++ {
			m.RecordOutcome("Proc", model.ProcessorError)
		}
		h := m.GetHealth("Proc")
		assert.InDelta(t, 0.5, h.HealthScore, 0.001)
		// At exactly 0.5, score is NOT < 0.5, so it should be healthy
		assert.Equal(t, StatusHealthy, h.Status)
	})

	t.Run("just below degraded threshold", func(t *testing.T) {
		// Window size 200 to hold all 100 entries
		m := NewMonitorWithConfig(200, 10*time.Minute)
		// 49 approvals, 51 failures = 0.49 (below 0.5)
		for i := 0; i < 49; i++ {
			m.RecordOutcome("Proc", model.Approved)
		}
		for i := 0; i < 51; i++ {
			m.RecordOutcome("Proc", model.ProcessorError)
		}
		h := m.GetHealth("Proc")
		assert.Less(t, h.HealthScore, 0.5)
		assert.Equal(t, StatusDegraded, h.Status)
	})

	t.Run("exactly at circuit breaker threshold (0.2)", func(t *testing.T) {
		m := NewMonitorWithConfig(50, 10*time.Minute)
		// 2 approvals, 8 failures = exactly 0.2
		for i := 0; i < 2; i++ {
			m.RecordOutcome("Proc", model.Approved)
		}
		for i := 0; i < 8; i++ {
			m.RecordOutcome("Proc", model.ProcessorError)
		}
		h := m.GetHealth("Proc")
		assert.InDelta(t, 0.2, h.HealthScore, 0.001)
		// At exactly 0.2, score is NOT < 0.2, so it should be degraded (not open)
		assert.Equal(t, StatusDegraded, h.Status)
	})

	t.Run("just below circuit breaker", func(t *testing.T) {
		m := NewMonitorWithConfig(50, 10*time.Minute)
		// 1 approval, 9 failures = 0.1 (below 0.2)
		for i := 0; i < 1; i++ {
			m.RecordOutcome("Proc", model.Approved)
		}
		for i := 0; i < 9; i++ {
			m.RecordOutcome("Proc", model.ProcessorError)
		}
		h := m.GetHealth("Proc")
		assert.Less(t, h.HealthScore, 0.2)
		assert.Equal(t, StatusOpen, h.Status)
	})
}

func TestMonitor_MathPrecision(t *testing.T) {
	m := NewMonitorWithConfig(50, 10*time.Minute)

	// 7 approvals out of 10 should equal exactly 0.7
	for i := 0; i < 7; i++ {
		m.RecordOutcome("Proc", model.Approved)
	}
	for i := 0; i < 3; i++ {
		m.RecordOutcome("Proc", model.ProcessorError)
	}

	h := m.GetHealth("Proc")
	assert.InDelta(t, 0.7, h.HealthScore, 0.0001,
		"7/10 should be exactly 0.7, got %v", h.HealthScore)
}

func TestMonitor_WindowSize(t *testing.T) {
	// Window of 5: only last 5 transactions should count
	m := NewMonitorWithConfig(5, 10*time.Minute)

	// Record 5 failures
	for i := 0; i < 5; i++ {
		m.RecordOutcome("Proc", model.ProcessorError)
	}
	h := m.GetHealth("Proc")
	assert.Equal(t, 0.0, h.HealthScore)

	// Record 5 approvals — should push out the failures
	for i := 0; i < 5; i++ {
		m.RecordOutcome("Proc", model.Approved)
	}
	h = m.GetHealth("Proc")
	assert.Equal(t, 1.0, h.HealthScore)
	assert.Equal(t, 5, h.TotalRecent)
}

func TestMonitor_TimeWindowExpiry(t *testing.T) {
	m := NewMonitorWithConfig(50, 100*time.Millisecond)

	// Record failures
	for i := 0; i < 5; i++ {
		m.RecordOutcome("Proc", model.ProcessorError)
	}
	h := m.GetHealth("Proc")
	assert.Equal(t, 0.0, h.HealthScore)

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// After expiry, should be healthy again (no recent data = default healthy)
	h = m.GetHealth("Proc")
	assert.Equal(t, 1.0, h.HealthScore)
	assert.Equal(t, StatusHealthy, h.Status)
}

func TestMonitor_IsCircuitOpen(t *testing.T) {
	m := NewMonitorWithConfig(50, 10*time.Minute)

	// Not tracked yet = not open
	assert.False(t, m.IsCircuitOpen("Unknown"))

	// Healthy processor = not open
	for i := 0; i < 10; i++ {
		m.RecordOutcome("HealthyProc", model.Approved)
	}
	assert.False(t, m.IsCircuitOpen("HealthyProc"))

	// Very unhealthy processor = open
	for i := 0; i < 10; i++ {
		m.RecordOutcome("BadProc", model.ProcessorError)
	}
	assert.True(t, m.IsCircuitOpen("BadProc"))
}

func TestMonitor_GetAllHealth(t *testing.T) {
	m := NewMonitorWithConfig(50, 10*time.Minute)

	m.RecordOutcome("ProcA", model.Approved)
	m.RecordOutcome("ProcB", model.ProcessorError)
	m.RecordOutcome("ProcC", model.Approved)

	healths := m.GetAllHealth()
	require.Len(t, healths, 3)

	names := make(map[string]bool)
	for _, h := range healths {
		names[h.ProcessorName] = true
	}
	assert.True(t, names["ProcA"])
	assert.True(t, names["ProcB"])
	assert.True(t, names["ProcC"])
}

func TestMonitor_RecoveryAfterDegradation(t *testing.T) {
	m := NewMonitorWithConfig(10, 10*time.Minute)

	// Degrade: 8 failures, 2 approvals = 0.2 (degraded)
	for i := 0; i < 8; i++ {
		m.RecordOutcome("Proc", model.ProcessorError)
	}
	for i := 0; i < 2; i++ {
		m.RecordOutcome("Proc", model.Approved)
	}
	h := m.GetHealth("Proc")
	assert.Equal(t, StatusDegraded, h.Status)

	// Recover: 10 more approvals pushes out failures (window size = 10)
	for i := 0; i < 10; i++ {
		m.RecordOutcome("Proc", model.Approved)
	}
	h = m.GetHealth("Proc")
	assert.Equal(t, StatusHealthy, h.Status)
	assert.Equal(t, 1.0, h.HealthScore)
}

func TestMonitor_DifferentFailureCodes(t *testing.T) {
	m := NewMonitorWithConfig(50, 10*time.Minute)

	// All non-approved codes should count as failures
	failureCodes := []model.ResponseCode{
		model.SoftDecline,
		model.DeclinedInsufficientFunds,
		model.DeclinedFraud,
		model.ProcessorError,
		model.Timeout,
		model.RateLimited,
	}

	for _, code := range failureCodes {
		m.RecordOutcome("Proc", code)
	}

	h := m.GetHealth("Proc")
	assert.Equal(t, 0.0, h.HealthScore)
	assert.Equal(t, 6, h.ErrorCount)
}

func TestMonitor_ConcurrentAccess(t *testing.T) {
	m := NewMonitor()
	var wg sync.WaitGroup

	// 50 goroutines recording outcomes concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code := model.Approved
			if i%3 == 0 {
				code = model.ProcessorError
			}
			m.RecordOutcome("ConcProc", code)
			_ = m.GetHealth("ConcProc")
			_ = m.IsCircuitOpen("ConcProc")
		}(i)
	}
	wg.Wait()

	h := m.GetHealth("ConcProc")
	assert.Equal(t, 50, h.TotalRecent)
}
