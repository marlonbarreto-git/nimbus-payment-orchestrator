package processor

import (
	"context"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/model"
)

// Processor defines the interface for payment processors.
type Processor interface {
	// Name returns the processor's unique identifier.
	Name() string
	// Process attempts to authorize a payment through this processor.
	Process(ctx context.Context, req model.PaymentRequest) model.ProcessorResponse
	// SupportedMethods returns the payment methods this processor can handle.
	SupportedMethods() []string
}

// SupportsMethod checks if a processor supports the given payment method.
func SupportsMethod(p Processor, method string) bool {
	for _, m := range p.SupportedMethods() {
		if m == method {
			return true
		}
	}
	return false
}
