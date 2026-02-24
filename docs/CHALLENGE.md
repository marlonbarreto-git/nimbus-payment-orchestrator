# Nimbus Rides — Payment Orchestration Challenge

## Background

Nimbus Rides is a ride-hailing platform operating across Latin America. Their payment acceptance rate has dropped from 87% to 61% because failed transactions are not retried on alternative processors.

## Task

Build a Payment Orchestration Service that intelligently routes payments through multiple processors, implements retry logic for failed transactions, and adapts routing based on processor health.

## Functional Requirements

### Requirement 1: Multi-Processor Payment Routing with Retry Logic (30pts)

Payment authorization request must contain:
- Transaction amount and currency
- Payment method type (card, pix, oxxo, pse)
- Customer/transaction identifier

Service must:
- Route to appropriate processor (simulate 3-4 processors)
- On retriable failure (soft decline, processor error, timeout, rate limit) → retry on fallback
- Respect max retry limit (max 3 total attempts across processors)
- NEVER retry hard declines (insufficient funds, fraud flags)
- Return final response with metadata about which processors were tried

Mock processors:
- Processor A: 70% approval, 20% soft decline, 10% processor errors
- Processor B: 85% approval, 10% soft decline, 5% hard decline
- Processor C: 90% approval for PIX, 50% for cards

### Requirement 2: Processor Health Monitoring & Adaptive Routing (25pts)
- Monitor recent outcomes per processor (rolling window: last 50 txns or last 10 min)
- Calculate health score per processor (approval rates + error rates)
- Adapt routing: low health score → deprioritize, route to healthier ones first
- Expose endpoint to query current processor health
- Example: 15 consecutive errors in 5 min → skip processor, go to next

### Requirement 3 (Stretch): Payment Attempt History & Debugging (5pts)
- Given transaction ID → return all attempts (processors, responses, timestamps)
- Include routing decisions ("skipped Processor A due to low health score")
- Used by support team to debug failed payments

## Test Data Requirements
- At least 100 simulated payment requests
- Varied amounts ($5-$200 USD equivalent)
- Multiple currencies (BRL, COP, MXN, USD)
- All payment methods (card, PIX, OXXO, PSE)
- Different failure profiles triggering retry logic frequently
- Scenarios with degraded processor (80% error rate) for adaptive routing testing
- Enough volume to observe health score changes over time

## Scoring Breakdown

| Criteria | Points |
|----------|--------|
| Core Orchestration Logic | 30 |
| Processor Health Monitoring | 25 |
| Code Quality & Architecture | 15 |
| Testing & Demonstration | 15 |
| Documentation | 10 |
| Stretch - Payment History | 5 |
| **Total** | **100** |

## Deliverables
- Source code for the payment orchestration service
- README with setup instructions, architecture overview, and how to run/test
- Demo script or test suite that exercises key orchestration scenarios
- Brief write-up explaining routing strategy and trade-offs
