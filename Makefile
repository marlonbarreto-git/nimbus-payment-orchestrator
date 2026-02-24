.PHONY: run test coverage lint demo clean

run:
	go run cmd/server/main.go

test:
	go test ./... -v -race

coverage:
	go test ./... -coverprofile=coverage.out -race
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

lint:
	go vet ./...

demo:
	@echo "Running demo scenarios..."
	./scripts/demo.sh

clean:
	rm -f coverage.out coverage.html
