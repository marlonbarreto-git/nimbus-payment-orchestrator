package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/config"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/health"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/model"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/processor"
)

// Orchestrator routes payments through multiple processors with retry logic.
type Orchestrator struct {
	processors []processor.Processor
	monitor    *health.Monitor
	store      *PaymentStore
	maxRetries int
}

// New creates a new Orchestrator with the given processors and health monitor.
func New(processors []processor.Processor, monitor *health.Monitor) *Orchestrator {
	return &Orchestrator{
		processors: processors,
		monitor:    monitor,
		store:      NewPaymentStore(),
		maxRetries: config.MaxRetries,
	}
}

// ProcessPayment routes a payment request through available processors with retry logic.
func (o *Orchestrator) ProcessPayment(ctx context.Context, req model.PaymentRequest) model.PaymentResult {
	result := model.PaymentResult{
		TransactionID: req.TransactionID,
		Attempts:      make([]model.Attempt, 0),
	}

	// Get eligible processors sorted by health
	eligible := o.getEligibleProcessors(req.PaymentMethod)
	if len(eligible) == 0 {
		slog.Warn("no_eligible_processors",
			"txn_id", req.TransactionID,
			"payment_method", req.PaymentMethod,
		)
		result.Status = model.StatusDeclined
		o.store.Save(result)
		return result
	}

	attemptNum := 0
	for _, ep := range eligible {
		if attemptNum >= o.maxRetries {
			break
		}
		attemptNum++

		reason := o.buildRoutingReason(ep, attemptNum, &result)

		slog.Info("payment_attempt",
			"txn_id", req.TransactionID,
			"processor", ep.proc.Name(),
			"attempt", attemptNum,
			"reason", reason,
			"health_score", fmt.Sprintf("%.2f", ep.healthScore),
		)

		resp := ep.proc.Process(ctx, req)

		attempt := model.Attempt{
			ProcessorName: ep.proc.Name(),
			Response:      resp,
			RoutingReason: reason,
			AttemptNumber: attemptNum,
			Timestamp:     time.Now(),
		}
		result.Attempts = append(result.Attempts, attempt)

		// Record outcome for health monitoring
		o.monitor.RecordOutcome(ep.proc.Name(), resp.Code)

		if resp.Code == model.Approved {
			slog.Info("payment_approved",
				"txn_id", req.TransactionID,
				"processor", ep.proc.Name(),
				"total_attempts", attemptNum,
			)
			result.Status = model.StatusApproved
			result.FinalResponse = &resp
			o.store.Save(result)
			return result
		}

		if resp.Code.IsHardDecline() {
			slog.Warn("hard_decline_stopping",
				"txn_id", req.TransactionID,
				"processor", ep.proc.Name(),
				"code", resp.Code,
				"total_attempts", attemptNum,
			)
			result.Status = model.StatusDeclined
			result.FinalResponse = &resp
			o.store.Save(result)
			return result
		}

		// Retriable failure — log and continue to next processor
		slog.Warn("retriable_failure",
			"txn_id", req.TransactionID,
			"processor", ep.proc.Name(),
			"code", resp.Code,
			"attempt", attemptNum,
		)
	}

	slog.Warn("retries_exhausted",
		"txn_id", req.TransactionID,
		"total_attempts", attemptNum,
	)
	result.Status = model.StatusExhaustedRetries
	if len(result.Attempts) > 0 {
		lastResp := result.Attempts[len(result.Attempts)-1].Response
		result.FinalResponse = &lastResp
	}
	o.store.Save(result)
	return result
}

// GetPaymentHistory returns the payment result for a given transaction ID.
func (o *Orchestrator) GetPaymentHistory(txnID string) (model.PaymentResult, bool) {
	return o.store.Get(txnID)
}

// HealthMonitor returns the health monitor for external access.
func (o *Orchestrator) HealthMonitor() *health.Monitor {
	return o.monitor
}

// Processors returns the list of processors for external access.
func (o *Orchestrator) Processors() []processor.Processor {
	return o.processors
}

type eligibleProcessor struct {
	proc        processor.Processor
	healthScore float64
	status      health.Status
}

func (o *Orchestrator) getEligibleProcessors(paymentMethod string) []eligibleProcessor {
	var eligible []eligibleProcessor

	for _, p := range o.processors {
		if !processor.SupportsMethod(p, paymentMethod) {
			continue
		}

		h := o.monitor.GetHealth(p.Name())

		if h.Status == health.StatusOpen {
			slog.Info("processor_skipped_circuit_open",
				"processor", p.Name(),
				"health_score", fmt.Sprintf("%.2f", h.HealthScore),
			)
			continue
		}

		eligible = append(eligible, eligibleProcessor{
			proc:        p,
			healthScore: h.HealthScore,
			status:      h.Status,
		})
	}

	// Sort by health score descending (healthiest first)
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].healthScore > eligible[j].healthScore
	})

	return eligible
}

func (o *Orchestrator) buildRoutingReason(ep eligibleProcessor, attemptNum int, result *model.PaymentResult) string {
	if attemptNum == 1 {
		if ep.status == health.StatusDegraded {
			return fmt.Sprintf("primary (degraded): health score %.2f", ep.healthScore)
		}
		return fmt.Sprintf("primary: highest health score %.2f", ep.healthScore)
	}

	prevAttempt := result.Attempts[len(result.Attempts)-1]
	reason := fmt.Sprintf("fallback: %s returned %s",
		prevAttempt.ProcessorName, prevAttempt.Response.Code)
	if ep.status == health.StatusDegraded {
		reason += fmt.Sprintf(" (degraded: health %.2f)", ep.healthScore)
	}
	return reason
}

// PaymentStore provides thread-safe storage for payment results.
type PaymentStore struct {
	mu      sync.RWMutex
	results map[string]model.PaymentResult
}

// NewPaymentStore creates a new empty payment store.
func NewPaymentStore() *PaymentStore {
	return &PaymentStore{
		results: make(map[string]model.PaymentResult),
	}
}

// Save stores a payment result.
func (s *PaymentStore) Save(result model.PaymentResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[result.TransactionID] = result
}

// Get retrieves a payment result by transaction ID.
func (s *PaymentStore) Get(txnID string) (model.PaymentResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.results[txnID]
	return r, ok
}
