# Build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /nimbus-orchestrator cmd/server/main.go

# Run stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /nimbus-orchestrator .
EXPOSE 8080
ENV PORT=8080
CMD ["./nimbus-orchestrator"]
