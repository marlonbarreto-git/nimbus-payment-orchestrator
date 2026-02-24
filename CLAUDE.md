# Nimbus Payment Orchestrator

## Project Structure
- `cmd/server/` — entry point
- `internal/` — all business logic (handler, orchestrator, processor, health, model, config)
- `docs/` — challenge spec and API docs

## Conventions
- Go 1.24+, stdlib only (net/http, log/slog), testify for tests
- TDD: test files always alongside source (e.g., `orchestrator.go` + `orchestrator_test.go`)
- Structured logging via slog — NEVER use fmt.Println or log.Println
- Error handling: always wrap with context, never swallow errors
- Naming: Go conventions (CamelCase exports, camelCase internal)
- JSON tags: snake_case (e.g., `json:"transaction_id"`)
- HTTP responses: always JSON with appropriate status codes
- Tests: table-driven, test happy path + edge cases + boundaries

## Commit Convention
```
<type>: <description>

Co-Authored-By: Claude <noreply@anthropic.com>
```
Types: feat, fix, refactor, test, docs, chore

## Critical Rules
- NEVER duplicate logic between packages
- NEVER use float64 equality (==) for health scores — use epsilon comparison or integer math
- NEVER retry hard declines (insufficient_funds, fraud)
- Max 3 retry attempts across all processors
- Health monitor MUST be goroutine-safe (sync.RWMutex)
