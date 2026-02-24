package model

import "time"

// PaymentRequest represents an incoming payment authorization request.
type PaymentRequest struct {
	TransactionID string  `json:"transaction_id"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	PaymentMethod string  `json:"payment_method"`
	CustomerID    string  `json:"customer_id"`
}

// ResponseCode represents the outcome of a processor authorization attempt.
type ResponseCode string

const (
	Approved                  ResponseCode = "approved"
	SoftDecline               ResponseCode = "soft_decline"
	DeclinedInsufficientFunds ResponseCode = "declined_insufficient_funds"
	DeclinedFraud             ResponseCode = "declined_fraud"
	ProcessorError            ResponseCode = "processor_error"
	Timeout                   ResponseCode = "timeout"
	RateLimited               ResponseCode = "rate_limited"
)

// IsRetriable returns true if the response code indicates a retriable failure.
func (rc ResponseCode) IsRetriable() bool {
	switch rc {
	case SoftDecline, ProcessorError, Timeout, RateLimited:
		return true
	default:
		return false
	}
}

// IsHardDecline returns true if the response code indicates a non-retriable decline.
func (rc ResponseCode) IsHardDecline() bool {
	switch rc {
	case DeclinedInsufficientFunds, DeclinedFraud:
		return true
	default:
		return false
	}
}

// ProcessorResponse represents the result of a single processor authorization attempt.
type ProcessorResponse struct {
	ProcessorName string        `json:"processor_name"`
	Code          ResponseCode  `json:"code"`
	Message       string        `json:"message"`
	Timestamp     time.Time     `json:"timestamp"`
	Latency       time.Duration `json:"latency"`
}

// Attempt represents a single routing attempt within a payment orchestration.
type Attempt struct {
	ProcessorName string            `json:"processor_name"`
	Response      ProcessorResponse `json:"response"`
	RoutingReason string            `json:"routing_reason"`
	AttemptNumber int               `json:"attempt_number"`
	Timestamp     time.Time         `json:"timestamp"`
}

// PaymentStatus represents the final status of a payment after orchestration.
type PaymentStatus string

const (
	StatusApproved         PaymentStatus = "approved"
	StatusDeclined         PaymentStatus = "declined"
	StatusExhaustedRetries PaymentStatus = "exhausted_retries"
)

// PaymentResult represents the final outcome of a payment orchestration.
type PaymentResult struct {
	TransactionID string             `json:"transaction_id"`
	Status        PaymentStatus      `json:"status"`
	Attempts      []Attempt          `json:"attempts"`
	FinalResponse *ProcessorResponse `json:"final_response"`
}
