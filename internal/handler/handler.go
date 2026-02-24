package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/model"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/orchestrator"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/processor"
)

// Handler holds HTTP handler dependencies.
type Handler struct {
	orch *orchestrator.Orchestrator
}

// New creates a new Handler.
func New(orch *orchestrator.Orchestrator) *Handler {
	return &Handler{orch: orch}
}

// RegisterRoutes registers all API routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /payments", h.ProcessPayment)
	mux.HandleFunc("GET /payments/{id}", h.GetPaymentHistory)
	mux.HandleFunc("GET /health/processors", h.GetProcessorHealth)
	mux.HandleFunc("POST /simulate/degrade", h.SimulateDegrade)
	mux.HandleFunc("POST /simulate/batch", h.SimulateBatch)
}

// ProcessPayment handles POST /payments
func (h *Handler) ProcessPayment(w http.ResponseWriter, r *http.Request) {
	var req model.PaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if err := validatePaymentRequest(req); err != "" {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	result := h.orch.ProcessPayment(r.Context(), req)

	status := http.StatusOK
	if result.Status == model.StatusDeclined || result.Status == model.StatusExhaustedRetries {
		status = http.StatusUnprocessableEntity
	}

	writeJSON(w, status, result)
}

// GetPaymentHistory handles GET /payments/{id}
func (h *Handler) GetPaymentHistory(w http.ResponseWriter, r *http.Request) {
	txnID := r.PathValue("id")
	if txnID == "" {
		writeError(w, http.StatusBadRequest, "transaction ID is required")
		return
	}

	result, ok := h.orch.GetPaymentHistory(txnID)
	if !ok {
		writeError(w, http.StatusNotFound, "transaction not found: "+txnID)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetProcessorHealth handles GET /health/processors
func (h *Handler) GetProcessorHealth(w http.ResponseWriter, r *http.Request) {
	healths := h.orch.HealthMonitor().GetAllHealth()

	response := map[string]interface{}{
		"processors": healths,
	}
	writeJSON(w, http.StatusOK, response)
}

// degradeRequest is the request body for POST /simulate/degrade
type degradeRequest struct {
	ProcessorName string `json:"processor_name"`
	Degraded      bool   `json:"degraded"`
}

// SimulateDegrade handles POST /simulate/degrade
func (h *Handler) SimulateDegrade(w http.ResponseWriter, r *http.Request) {
	var req degradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.ProcessorName == "" {
		writeError(w, http.StatusBadRequest, "processor_name is required")
		return
	}

	for _, p := range h.orch.Processors() {
		if p.Name() == req.ProcessorName {
			if mp, ok := p.(*processor.MockProcessor); ok {
				mp.SetDegraded(req.Degraded)
				slog.Info("processor_degradation_toggled",
					"processor", req.ProcessorName,
					"degraded", req.Degraded,
				)
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"processor": req.ProcessorName,
					"degraded":  req.Degraded,
					"message":   "degradation mode updated",
				})
				return
			}
		}
	}

	writeError(w, http.StatusNotFound, "processor not found: "+req.ProcessorName)
}

// batchRequest is the request body for POST /simulate/batch
type batchRequest struct {
	Count    int    `json:"count"`
	Method   string `json:"method"`
	Currency string `json:"currency"`
}

// SimulateBatch handles POST /simulate/batch
func (h *Handler) SimulateBatch(w http.ResponseWriter, r *http.Request) {
	var req batchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Count <= 0 || req.Count > 1000 {
		writeError(w, http.StatusBadRequest, "count must be between 1 and 1000")
		return
	}
	if req.Method == "" {
		req.Method = "card"
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}

	results := make([]model.PaymentResult, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		payReq := model.PaymentRequest{
			TransactionID: generateTxnID(i),
			Amount:        randomAmount(),
			Currency:      req.Currency,
			PaymentMethod: req.Method,
			CustomerID:    generateCustomerID(i),
		}
		result := h.orch.ProcessPayment(r.Context(), payReq)
		results = append(results, result)
	}

	// Summarize
	summary := summarizeBatch(results)
	writeJSON(w, http.StatusOK, summary)
}

func validatePaymentRequest(req model.PaymentRequest) string {
	if req.TransactionID == "" {
		return "transaction_id is required"
	}
	if req.Amount <= 0 {
		return "amount must be greater than 0"
	}
	if req.Currency == "" {
		return "currency is required"
	}
	validMethods := map[string]bool{"card": true, "pix": true, "oxxo": true, "pse": true}
	if !validMethods[req.PaymentMethod] {
		return "payment_method must be one of: card, pix, oxxo, pse"
	}
	if req.CustomerID == "" {
		return "customer_id is required"
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func generateTxnID(i int) string {
	return "batch-" + randomHex(8) + "-" + itoa(i)
}

func generateCustomerID(i int) string {
	return "cust-batch-" + itoa(i)
}

func randomAmount() float64 {
	// Random amount between $5 and $200
	return 5.0 + float64(randInt(19500))/100.0
}

func summarizeBatch(results []model.PaymentResult) map[string]interface{} {
	approved := 0
	declined := 0
	exhausted := 0
	totalAttempts := 0

	for _, r := range results {
		switch r.Status {
		case model.StatusApproved:
			approved++
		case model.StatusDeclined:
			declined++
		case model.StatusExhaustedRetries:
			exhausted++
		}
		totalAttempts += len(r.Attempts)
	}

	return map[string]interface{}{
		"total":             len(results),
		"approved":          approved,
		"declined":          declined,
		"exhausted_retries": exhausted,
		"approval_rate":     float64(approved) / float64(len(results)),
		"avg_attempts":      float64(totalAttempts) / float64(len(results)),
	}
}
