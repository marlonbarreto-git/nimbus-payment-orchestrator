package processor

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/model"
)

// OutcomeDistribution defines the probability of each response type.
type OutcomeDistribution struct {
	ApprovalRate    float64
	SoftDeclineRate float64
	HardDeclineRate float64
	ErrorRate       float64
}

// MethodOverride allows per-method outcome overrides.
type MethodOverride struct {
	Method       string
	Distribution OutcomeDistribution
}

// MockConfig holds configuration for creating a mock processor.
type MockConfig struct {
	ProcessorName    string
	Methods          []string
	DefaultOutcomes  OutcomeDistribution
	MethodOverrides  []MethodOverride
	MinLatency       time.Duration
	MaxLatency       time.Duration
}

// MockProcessor simulates a payment processor with configurable behavior.
type MockProcessor struct {
	config   MockConfig
	rng      *rand.Rand
	mu       sync.Mutex
	degraded bool
}

// NewMockProcessor creates a new mock processor from the given config.
func NewMockProcessor(cfg MockConfig) *MockProcessor {
	return &MockProcessor{
		config: cfg,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (p *MockProcessor) Name() string {
	return p.config.ProcessorName
}

func (p *MockProcessor) SupportedMethods() []string {
	return p.config.Methods
}

// SetDegraded toggles degraded mode (80% error rate) for simulation.
func (p *MockProcessor) SetDegraded(degraded bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.degraded = degraded
}

// IsDegraded returns the current degraded state.
func (p *MockProcessor) IsDegraded() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.degraded
}

func (p *MockProcessor) Process(ctx context.Context, req model.PaymentRequest) model.ProcessorResponse {
	start := time.Now()

	p.mu.Lock()
	degraded := p.degraded
	p.mu.Unlock()

	// Simulate latency
	latency := p.simulateLatency()
	select {
	case <-time.After(latency):
	case <-ctx.Done():
		return model.ProcessorResponse{
			ProcessorName: p.config.ProcessorName,
			Code:          model.Timeout,
			Message:       "context cancelled",
			Timestamp:     time.Now(),
			Latency:       time.Since(start),
		}
	}

	// Determine outcome
	code := p.determineOutcome(req.PaymentMethod, degraded)

	return model.ProcessorResponse{
		ProcessorName: p.config.ProcessorName,
		Code:          code,
		Message:       responseMessage(code),
		Timestamp:     time.Now(),
		Latency:       time.Since(start),
	}
}

func (p *MockProcessor) determineOutcome(method string, degraded bool) model.ResponseCode {
	p.mu.Lock()
	roll := p.rng.Float64()
	p.mu.Unlock()

	if degraded {
		// In degraded mode: 80% processor error, 20% approval
		if roll < 0.80 {
			return model.ProcessorError
		}
		return model.Approved
	}

	dist := p.config.DefaultOutcomes
	for _, override := range p.config.MethodOverrides {
		if override.Method == method {
			dist = override.Distribution
			break
		}
	}

	// Roll against cumulative distribution
	if roll < dist.ApprovalRate {
		return model.Approved
	}
	roll -= dist.ApprovalRate
	if roll < dist.SoftDeclineRate {
		return model.SoftDecline
	}
	roll -= dist.SoftDeclineRate
	if roll < dist.HardDeclineRate {
		return model.DeclinedInsufficientFunds
	}
	return model.ProcessorError
}

func (p *MockProcessor) simulateLatency() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	min := p.config.MinLatency
	max := p.config.MaxLatency
	if max <= min {
		return min
	}
	return min + time.Duration(p.rng.Int63n(int64(max-min)))
}

func responseMessage(code model.ResponseCode) string {
	switch code {
	case model.Approved:
		return "transaction approved"
	case model.SoftDecline:
		return "soft decline - try again"
	case model.DeclinedInsufficientFunds:
		return "insufficient funds"
	case model.DeclinedFraud:
		return "suspected fraud"
	case model.ProcessorError:
		return "internal processor error"
	case model.Timeout:
		return "request timed out"
	case model.RateLimited:
		return "rate limit exceeded"
	default:
		return "unknown response"
	}
}
