package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/config"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/handler"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/health"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/orchestrator"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/processor"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Initialize health monitor
	monitor := health.NewMonitor()

	// Initialize processors
	processors := []processor.Processor{
		processor.NewPayFlow(),
		processor.NewCardMax(),
		processor.NewPixPay(),
		processor.NewGlobalPay(),
	}

	// Initialize orchestrator
	orch := orchestrator.New(processors, monitor)

	// Initialize HTTP handlers
	h := handler.New(orch)

	// Register routes
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	port := config.ServerPort
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = ":" + envPort
	}

	slog.Info("server_starting",
		"port", port,
		"processors", []string{"PayFlow", "CardMax", "PixPay", "GlobalPay"},
	)

	if err := http.ListenAndServe(port, mux); err != nil {
		slog.Error("server_failed", "error", err)
		os.Exit(1)
	}
}
