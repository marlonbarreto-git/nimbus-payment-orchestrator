package processor

import (
	"context"
	"testing"
	"time"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSupportsMethod(t *testing.T) {
	p := NewPayFlow()
	tests := []struct {
		name     string
		method   string
		expected bool
	}{
		{"PayFlow supports card", "card", true},
		{"PayFlow supports pix", "pix", true},
		{"PayFlow supports oxxo", "oxxo", true},
		{"PayFlow supports pse", "pse", true},
		{"PayFlow does not support crypto", "crypto", false},
		{"PayFlow does not support empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, SupportsMethod(p, tt.method))
		})
	}
}

func TestMockProcessor_Name(t *testing.T) {
	processors := []struct {
		factory func() *MockProcessor
		name    string
	}{
		{NewPayFlow, "PayFlow"},
		{NewCardMax, "CardMax"},
		{NewPixPay, "PixPay"},
		{NewGlobalPay, "GlobalPay"},
	}
	for _, tt := range processors {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.factory()
			assert.Equal(t, tt.name, p.Name())
		})
	}
}

func TestMockProcessor_SupportedMethods(t *testing.T) {
	tests := []struct {
		name     string
		factory  func() *MockProcessor
		expected []string
	}{
		{"PayFlow methods", NewPayFlow, []string{"card", "pix", "oxxo", "pse"}},
		{"CardMax methods", NewCardMax, []string{"card", "oxxo"}},
		{"PixPay methods", NewPixPay, []string{"card", "pix"}},
		{"GlobalPay methods", NewGlobalPay, []string{"card", "pix", "oxxo", "pse"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.factory()
			assert.Equal(t, tt.expected, p.SupportedMethods())
		})
	}
}

func TestMockProcessor_ProcessReturnsValidResponse(t *testing.T) {
	p := NewPayFlow()
	ctx := context.Background()
	req := model.PaymentRequest{
		TransactionID: "tx-test",
		Amount:        100.0,
		Currency:      "BRL",
		PaymentMethod: "card",
		CustomerID:    "cust-1",
	}

	resp := p.Process(ctx, req)
	assert.Equal(t, "PayFlow", resp.ProcessorName)
	assert.NotEmpty(t, resp.Message)
	assert.False(t, resp.Timestamp.IsZero())
	assert.Greater(t, resp.Latency, time.Duration(0))
}

func TestMockProcessor_OutcomeDistribution(t *testing.T) {
	// Run 1000 transactions and verify distribution is roughly correct
	p := NewPayFlow() // 70% approval, 20% soft, 10% error
	ctx := context.Background()
	req := model.PaymentRequest{
		TransactionID: "tx-dist",
		Amount:        50.0,
		Currency:      "BRL",
		PaymentMethod: "card",
		CustomerID:    "cust-1",
	}

	counts := map[model.ResponseCode]int{}
	total := 1000
	for i := 0; i < total; i++ {
		resp := p.Process(ctx, req)
		counts[resp.Code]++
	}

	// With 1000 samples, expect roughly ±10% of configured rates
	approvalRate := float64(counts[model.Approved]) / float64(total)
	assert.InDelta(t, 0.70, approvalRate, 0.10,
		"PayFlow approval rate should be ~70%%, got %.2f%%", approvalRate*100)

	softRate := float64(counts[model.SoftDecline]) / float64(total)
	assert.InDelta(t, 0.20, softRate, 0.10,
		"PayFlow soft decline rate should be ~20%%, got %.2f%%", softRate*100)
}

func TestPixPay_MethodOverride(t *testing.T) {
	// PixPay should have ~90% approval for PIX vs ~50% for cards
	p := NewPixPay()
	ctx := context.Background()
	total := 500

	pixReq := model.PaymentRequest{
		TransactionID: "tx-pix",
		Amount:        50.0,
		Currency:      "BRL",
		PaymentMethod: "pix",
		CustomerID:    "cust-1",
	}
	cardReq := model.PaymentRequest{
		TransactionID: "tx-card",
		Amount:        50.0,
		Currency:      "BRL",
		PaymentMethod: "card",
		CustomerID:    "cust-1",
	}

	pixApprovals := 0
	cardApprovals := 0
	for i := 0; i < total; i++ {
		if p.Process(ctx, pixReq).Code == model.Approved {
			pixApprovals++
		}
		if p.Process(ctx, cardReq).Code == model.Approved {
			cardApprovals++
		}
	}

	pixRate := float64(pixApprovals) / float64(total)
	cardRate := float64(cardApprovals) / float64(total)

	assert.Greater(t, pixRate, cardRate,
		"PixPay should approve PIX more often than cards (pix=%.2f, card=%.2f)", pixRate, cardRate)
	assert.InDelta(t, 0.90, pixRate, 0.10, "PixPay PIX approval should be ~90%%")
	assert.InDelta(t, 0.50, cardRate, 0.15, "PixPay card approval should be ~50%%")
}

func TestMockProcessor_DegradedMode(t *testing.T) {
	p := NewPayFlow()
	ctx := context.Background()
	req := model.PaymentRequest{
		TransactionID: "tx-degrade",
		Amount:        100.0,
		Currency:      "BRL",
		PaymentMethod: "card",
		CustomerID:    "cust-1",
	}

	// Enable degraded mode
	p.SetDegraded(true)
	assert.True(t, p.IsDegraded())

	errorCount := 0
	total := 200
	for i := 0; i < total; i++ {
		resp := p.Process(ctx, req)
		if resp.Code == model.ProcessorError {
			errorCount++
		}
	}

	errorRate := float64(errorCount) / float64(total)
	assert.InDelta(t, 0.80, errorRate, 0.10,
		"degraded mode should produce ~80%% errors, got %.2f%%", errorRate*100)

	// Disable degraded mode
	p.SetDegraded(false)
	assert.False(t, p.IsDegraded())
}

func TestMockProcessor_ContextCancellation(t *testing.T) {
	p := NewMockProcessor(MockConfig{
		ProcessorName:   "SlowProcessor",
		Methods:         []string{"card"},
		DefaultOutcomes: OutcomeDistribution{ApprovalRate: 1.0},
		MinLatency:      5 * time.Second,
		MaxLatency:      5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := model.PaymentRequest{
		TransactionID: "tx-timeout",
		Amount:        100.0,
		Currency:      "USD",
		PaymentMethod: "card",
		CustomerID:    "cust-1",
	}

	resp := p.Process(ctx, req)
	assert.Equal(t, model.Timeout, resp.Code)
	assert.Equal(t, "SlowProcessor", resp.ProcessorName)
}

func TestMockProcessor_ConcurrentAccess(t *testing.T) {
	p := NewPayFlow()
	ctx := context.Background()
	req := model.PaymentRequest{
		TransactionID: "tx-concurrent",
		Amount:        100.0,
		Currency:      "BRL",
		PaymentMethod: "card",
		CustomerID:    "cust-1",
	}

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			resp := p.Process(ctx, req)
			require.NotEmpty(t, resp.ProcessorName)
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}

func TestCardMax_OnlySupportsCardAndOXXO(t *testing.T) {
	p := NewCardMax()
	assert.True(t, SupportsMethod(p, "card"))
	assert.True(t, SupportsMethod(p, "oxxo"))
	assert.False(t, SupportsMethod(p, "pix"))
	assert.False(t, SupportsMethod(p, "pse"))
}

func TestResponseMessage(t *testing.T) {
	tests := []struct {
		code    model.ResponseCode
		message string
	}{
		{model.Approved, "transaction approved"},
		{model.SoftDecline, "soft decline - try again"},
		{model.DeclinedInsufficientFunds, "insufficient funds"},
		{model.DeclinedFraud, "suspected fraud"},
		{model.ProcessorError, "internal processor error"},
		{model.Timeout, "request timed out"},
		{model.RateLimited, "rate limit exceeded"},
		{model.ResponseCode("unknown"), "unknown response"},
	}
	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			assert.Equal(t, tt.message, responseMessage(tt.code))
		})
	}
}
