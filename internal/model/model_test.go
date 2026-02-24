package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseCode_IsRetriable(t *testing.T) {
	tests := []struct {
		name     string
		code     ResponseCode
		expected bool
	}{
		{"soft decline is retriable", SoftDecline, true},
		{"processor error is retriable", ProcessorError, true},
		{"timeout is retriable", Timeout, true},
		{"rate limited is retriable", RateLimited, true},
		{"approved is not retriable", Approved, false},
		{"insufficient funds is not retriable", DeclinedInsufficientFunds, false},
		{"fraud is not retriable", DeclinedFraud, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.code.IsRetriable())
		})
	}
}

func TestResponseCode_IsHardDecline(t *testing.T) {
	tests := []struct {
		name     string
		code     ResponseCode
		expected bool
	}{
		{"insufficient funds is hard decline", DeclinedInsufficientFunds, true},
		{"fraud is hard decline", DeclinedFraud, true},
		{"soft decline is not hard decline", SoftDecline, false},
		{"approved is not hard decline", Approved, false},
		{"processor error is not hard decline", ProcessorError, false},
		{"timeout is not hard decline", Timeout, false},
		{"rate limited is not hard decline", RateLimited, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.code.IsHardDecline())
		})
	}
}

func TestPaymentRequest_Fields(t *testing.T) {
	req := PaymentRequest{
		TransactionID: "tx-001",
		Amount:        150.50,
		Currency:      "BRL",
		PaymentMethod: "pix",
		CustomerID:    "cust-123",
	}
	assert.Equal(t, "tx-001", req.TransactionID)
	assert.Equal(t, 150.50, req.Amount)
	assert.Equal(t, "BRL", req.Currency)
	assert.Equal(t, "pix", req.PaymentMethod)
	assert.Equal(t, "cust-123", req.CustomerID)
}

func TestPaymentResult_Structure(t *testing.T) {
	now := time.Now()
	resp := ProcessorResponse{
		ProcessorName: "PayFlow",
		Code:          Approved,
		Message:       "Transaction approved",
		Timestamp:     now,
		Latency:       150 * time.Millisecond,
	}
	result := PaymentResult{
		TransactionID: "tx-002",
		Status:        StatusApproved,
		Attempts: []Attempt{
			{
				ProcessorName: "PayFlow",
				Response:      resp,
				RoutingReason: "primary: highest health score",
				AttemptNumber: 1,
				Timestamp:     now,
			},
		},
		FinalResponse: &resp,
	}
	require.Len(t, result.Attempts, 1)
	assert.Equal(t, StatusApproved, result.Status)
	assert.Equal(t, "PayFlow", result.FinalResponse.ProcessorName)
	assert.Equal(t, 1, result.Attempts[0].AttemptNumber)
}

func TestPaymentStatus_Values(t *testing.T) {
	assert.Equal(t, PaymentStatus("approved"), StatusApproved)
	assert.Equal(t, PaymentStatus("declined"), StatusDeclined)
	assert.Equal(t, PaymentStatus("exhausted_retries"), StatusExhaustedRetries)
}

func TestResponseCode_MutualExclusivity(t *testing.T) {
	// A code should never be both retriable and hard decline
	allCodes := []ResponseCode{
		Approved, SoftDecline, DeclinedInsufficientFunds,
		DeclinedFraud, ProcessorError, Timeout, RateLimited,
	}
	for _, code := range allCodes {
		t.Run(string(code), func(t *testing.T) {
			if code.IsHardDecline() {
				assert.False(t, code.IsRetriable(),
					"code %s cannot be both hard decline and retriable", code)
			}
		})
	}
}
