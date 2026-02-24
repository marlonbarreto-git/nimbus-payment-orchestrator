package processor

import "time"

// NewPayFlow creates Processor A: general purpose, 70% approval, 20% soft decline, 10% errors.
func NewPayFlow() *MockProcessor {
	return NewMockProcessor(MockConfig{
		ProcessorName: "PayFlow",
		Methods:       []string{"card", "pix", "oxxo", "pse"},
		DefaultOutcomes: OutcomeDistribution{
			ApprovalRate:    0.70,
			SoftDeclineRate: 0.20,
			HardDeclineRate: 0.00,
			ErrorRate:       0.10,
		},
		MinLatency: 50 * time.Millisecond,
		MaxLatency: 200 * time.Millisecond,
	})
}

// NewCardMax creates Processor B: strong on cards, 85% approval, 10% soft decline, 5% hard decline.
func NewCardMax() *MockProcessor {
	return NewMockProcessor(MockConfig{
		ProcessorName: "CardMax",
		Methods:       []string{"card", "oxxo"},
		DefaultOutcomes: OutcomeDistribution{
			ApprovalRate:    0.85,
			SoftDeclineRate: 0.10,
			HardDeclineRate: 0.05,
			ErrorRate:       0.00,
		},
		MinLatency: 80 * time.Millisecond,
		MaxLatency: 300 * time.Millisecond,
	})
}

// NewPixPay creates Processor C: LATAM specialist, 90% for PIX, 50% for cards.
func NewPixPay() *MockProcessor {
	return NewMockProcessor(MockConfig{
		ProcessorName: "PixPay",
		Methods:       []string{"card", "pix"},
		DefaultOutcomes: OutcomeDistribution{
			ApprovalRate:    0.50,
			SoftDeclineRate: 0.30,
			HardDeclineRate: 0.10,
			ErrorRate:       0.10,
		},
		MethodOverrides: []MethodOverride{
			{
				Method: "pix",
				Distribution: OutcomeDistribution{
					ApprovalRate:    0.90,
					SoftDeclineRate: 0.05,
					HardDeclineRate: 0.00,
					ErrorRate:       0.05,
				},
			},
		},
		MinLatency: 30 * time.Millisecond,
		MaxLatency: 150 * time.Millisecond,
	})
}

// NewGlobalPay creates Processor D: universal fallback, 75% flat approval, never rate limits.
func NewGlobalPay() *MockProcessor {
	return NewMockProcessor(MockConfig{
		ProcessorName: "GlobalPay",
		Methods:       []string{"card", "pix", "oxxo", "pse"},
		DefaultOutcomes: OutcomeDistribution{
			ApprovalRate:    0.75,
			SoftDeclineRate: 0.15,
			HardDeclineRate: 0.05,
			ErrorRate:       0.05,
		},
		MinLatency: 60 * time.Millisecond,
		MaxLatency: 250 * time.Millisecond,
	})
}
