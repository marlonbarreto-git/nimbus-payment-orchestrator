package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/health"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/model"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deterministicProcessor always returns the same response code.
type deterministicProcessor struct {
	name      string
	methods   []string
	code      model.ResponseCode
	callCount int
	mu        sync.Mutex
}

func newDeterministicProcessor(name string, methods []string, code model.ResponseCode) *deterministicProcessor {
	return &deterministicProcessor{name: name, methods: methods, code: code}
}

func (p *deterministicProcessor) Name() string             { return p.name }
func (p *deterministicProcessor) SupportedMethods() []string { return p.methods }
func (p *deterministicProcessor) Process(ctx context.Context, req model.PaymentRequest) model.ProcessorResponse {
	p.mu.Lock()
	p.callCount++
	p.mu.Unlock()
	return model.ProcessorResponse{
		ProcessorName: p.name,
		Code:          p.code,
		Message:       "test response",
		Timestamp:     time.Now(),
		Latency:       10 * time.Millisecond,
	}
}

func (p *deterministicProcessor) CallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.callCount
}

// sequenceProcessor returns different codes on successive calls.
type sequenceProcessor struct {
	name    string
	methods []string
	codes   []model.ResponseCode
	idx     int
	mu      sync.Mutex
}

func newSequenceProcessor(name string, methods []string, codes ...model.ResponseCode) *sequenceProcessor {
	return &sequenceProcessor{name: name, methods: methods, codes: codes}
}

func (p *sequenceProcessor) Name() string             { return p.name }
func (p *sequenceProcessor) SupportedMethods() []string { return p.methods }
func (p *sequenceProcessor) Process(ctx context.Context, req model.PaymentRequest) model.ProcessorResponse {
	p.mu.Lock()
	code := p.codes[p.idx%len(p.codes)]
	p.idx++
	p.mu.Unlock()
	return model.ProcessorResponse{
		ProcessorName: p.name,
		Code:          code,
		Message:       "test response",
		Timestamp:     time.Now(),
		Latency:       10 * time.Millisecond,
	}
}

func TestProcessPayment_ApprovedOnFirstTry(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)
	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.Approved),
		newDeterministicProcessor("ProcB", []string{"card"}, model.Approved),
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-001",
		Amount:        100.0,
		Currency:      "USD",
		PaymentMethod: "card",
		CustomerID:    "cust-1",
	}
	result := orch.ProcessPayment(context.Background(), req)

	assert.Equal(t, model.StatusApproved, result.Status)
	require.Len(t, result.Attempts, 1)
	assert.Equal(t, "ProcA", result.Attempts[0].ProcessorName)
	assert.Equal(t, 1, result.Attempts[0].AttemptNumber)
	assert.Contains(t, result.Attempts[0].RoutingReason, "primary")
	assert.NotNil(t, result.FinalResponse)
}

func TestProcessPayment_RetryOnSoftDecline(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)
	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.SoftDecline),
		newDeterministicProcessor("ProcB", []string{"card"}, model.Approved),
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-002",
		Amount:        50.0,
		Currency:      "BRL",
		PaymentMethod: "card",
		CustomerID:    "cust-2",
	}
	result := orch.ProcessPayment(context.Background(), req)

	assert.Equal(t, model.StatusApproved, result.Status)
	require.Len(t, result.Attempts, 2)
	assert.Equal(t, "ProcA", result.Attempts[0].ProcessorName)
	assert.Equal(t, model.SoftDecline, result.Attempts[0].Response.Code)
	assert.Equal(t, "ProcB", result.Attempts[1].ProcessorName)
	assert.Equal(t, model.Approved, result.Attempts[1].Response.Code)
	assert.Contains(t, result.Attempts[1].RoutingReason, "fallback")
}

func TestProcessPayment_HardDeclineStopsImmediately(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)
	procB := newDeterministicProcessor("ProcB", []string{"card"}, model.Approved)
	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.DeclinedInsufficientFunds),
		procB,
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-003",
		Amount:        200.0,
		Currency:      "USD",
		PaymentMethod: "card",
		CustomerID:    "cust-3",
	}
	result := orch.ProcessPayment(context.Background(), req)

	assert.Equal(t, model.StatusDeclined, result.Status)
	require.Len(t, result.Attempts, 1, "hard decline should not trigger retry")
	assert.Equal(t, model.DeclinedInsufficientFunds, result.FinalResponse.Code)
	assert.Equal(t, 0, procB.CallCount(), "ProcB should never be called after hard decline")
}

func TestProcessPayment_FraudDeclineStopsImmediately(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)
	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.DeclinedFraud),
		newDeterministicProcessor("ProcB", []string{"card"}, model.Approved),
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-fraud",
		Amount:        500.0,
		Currency:      "USD",
		PaymentMethod: "card",
		CustomerID:    "cust-fraud",
	}
	result := orch.ProcessPayment(context.Background(), req)

	assert.Equal(t, model.StatusDeclined, result.Status)
	require.Len(t, result.Attempts, 1)
	assert.Equal(t, model.DeclinedFraud, result.FinalResponse.Code)
}

func TestProcessPayment_MaxRetriesExhausted(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)
	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.ProcessorError),
		newDeterministicProcessor("ProcB", []string{"card"}, model.SoftDecline),
		newDeterministicProcessor("ProcC", []string{"card"}, model.Timeout),
		newDeterministicProcessor("ProcD", []string{"card"}, model.Approved), // 4th should NOT be tried
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-exhaust",
		Amount:        75.0,
		Currency:      "MXN",
		PaymentMethod: "card",
		CustomerID:    "cust-exhaust",
	}
	result := orch.ProcessPayment(context.Background(), req)

	assert.Equal(t, model.StatusExhaustedRetries, result.Status)
	assert.Len(t, result.Attempts, 3, "should stop after max 3 attempts")
}

func TestProcessPayment_NoCompatibleProcessors(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)
	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.Approved),
		newDeterministicProcessor("ProcB", []string{"card"}, model.Approved),
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-no-method",
		Amount:        50.0,
		Currency:      "BRL",
		PaymentMethod: "pix", // Neither processor supports PIX
		CustomerID:    "cust-pix",
	}
	result := orch.ProcessPayment(context.Background(), req)

	assert.Equal(t, model.StatusDeclined, result.Status)
	assert.Len(t, result.Attempts, 0)
}

func TestProcessPayment_SkipsCircuitOpenProcessors(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)

	// Make ProcA very unhealthy
	for i := 0; i < 20; i++ {
		mon.RecordOutcome("ProcA", model.ProcessorError)
	}

	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.Approved),
		newDeterministicProcessor("ProcB", []string{"card"}, model.Approved),
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-circuit",
		Amount:        100.0,
		Currency:      "USD",
		PaymentMethod: "card",
		CustomerID:    "cust-circuit",
	}
	result := orch.ProcessPayment(context.Background(), req)

	assert.Equal(t, model.StatusApproved, result.Status)
	require.Len(t, result.Attempts, 1)
	assert.Equal(t, "ProcB", result.Attempts[0].ProcessorName, "should skip ProcA (circuit open)")
}

func TestProcessPayment_HealthBasedRouting(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)

	// Make ProcA degraded (but not circuit open)
	for i := 0; i < 7; i++ {
		mon.RecordOutcome("ProcA", model.ProcessorError)
	}
	for i := 0; i < 3; i++ {
		mon.RecordOutcome("ProcA", model.Approved)
	}
	// ProcB is healthy
	for i := 0; i < 10; i++ {
		mon.RecordOutcome("ProcB", model.Approved)
	}

	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.Approved),
		newDeterministicProcessor("ProcB", []string{"card"}, model.Approved),
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-health-route",
		Amount:        100.0,
		Currency:      "USD",
		PaymentMethod: "card",
		CustomerID:    "cust-health",
	}
	result := orch.ProcessPayment(context.Background(), req)

	assert.Equal(t, model.StatusApproved, result.Status)
	require.Len(t, result.Attempts, 1)
	assert.Equal(t, "ProcB", result.Attempts[0].ProcessorName,
		"should route to healthier ProcB first")
}

func TestProcessPayment_AllProcessorsUnhealthy_StillTries(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)

	// Make both processors degraded but not circuit open
	for i := 0; i < 7; i++ {
		mon.RecordOutcome("ProcA", model.ProcessorError)
	}
	for i := 0; i < 3; i++ {
		mon.RecordOutcome("ProcA", model.Approved)
	}
	for i := 0; i < 8; i++ {
		mon.RecordOutcome("ProcB", model.ProcessorError)
	}
	for i := 0; i < 2; i++ {
		mon.RecordOutcome("ProcB", model.Approved)
	}

	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.Approved),
		newDeterministicProcessor("ProcB", []string{"card"}, model.Approved),
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-all-degraded",
		Amount:        100.0,
		Currency:      "USD",
		PaymentMethod: "card",
		CustomerID:    "cust-degrade",
	}
	result := orch.ProcessPayment(context.Background(), req)

	assert.Equal(t, model.StatusApproved, result.Status,
		"should still try degraded processors")
}

func TestProcessPayment_RetryOnMultipleFailureTypes(t *testing.T) {
	retriableCodes := []model.ResponseCode{
		model.SoftDecline,
		model.ProcessorError,
		model.Timeout,
		model.RateLimited,
	}

	for _, code := range retriableCodes {
		t.Run(string(code), func(t *testing.T) {
			mon := health.NewMonitorWithConfig(50, 10*time.Minute)
			procs := []processor.Processor{
				newDeterministicProcessor("ProcA", []string{"card"}, code),
				newDeterministicProcessor("ProcB", []string{"card"}, model.Approved),
			}
			orch := New(procs, mon)

			req := model.PaymentRequest{
				TransactionID: "tx-retry-" + string(code),
				Amount:        100.0,
				Currency:      "USD",
				PaymentMethod: "card",
				CustomerID:    "cust-retry",
			}
			result := orch.ProcessPayment(context.Background(), req)

			assert.Equal(t, model.StatusApproved, result.Status)
			require.Len(t, result.Attempts, 2)
		})
	}
}

func TestProcessPayment_PaymentStoreIntegration(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)
	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card"}, model.Approved),
	}
	orch := New(procs, mon)

	req := model.PaymentRequest{
		TransactionID: "tx-store-001",
		Amount:        100.0,
		Currency:      "USD",
		PaymentMethod: "card",
		CustomerID:    "cust-store",
	}
	orch.ProcessPayment(context.Background(), req)

	// Retrieve from store
	result, ok := orch.GetPaymentHistory("tx-store-001")
	assert.True(t, ok)
	assert.Equal(t, model.StatusApproved, result.Status)

	// Non-existent transaction
	_, ok = orch.GetPaymentHistory("tx-nonexistent")
	assert.False(t, ok)
}

func TestProcessPayment_ConcurrentPayments(t *testing.T) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)
	procs := []processor.Processor{
		newDeterministicProcessor("ProcA", []string{"card", "pix"}, model.Approved),
		newDeterministicProcessor("ProcB", []string{"card", "pix"}, model.Approved),
	}
	orch := New(procs, mon)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := model.PaymentRequest{
				TransactionID: fmt.Sprintf("tx-conc-%d", i),
				Amount:        float64(i * 10),
				Currency:      "USD",
				PaymentMethod: "card",
				CustomerID:    fmt.Sprintf("cust-%d", i),
			}
			result := orch.ProcessPayment(context.Background(), req)
			assert.Equal(t, model.StatusApproved, result.Status)
		}(i)
	}
	wg.Wait()
}

func TestPaymentStore_ConcurrentAccess(t *testing.T) {
	store := NewPaymentStore()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.Save(model.PaymentResult{
				TransactionID: fmt.Sprintf("tx-%d", i),
				Status:        model.StatusApproved,
			})
			store.Get(fmt.Sprintf("tx-%d", i))
		}(i)
	}
	wg.Wait()
}
