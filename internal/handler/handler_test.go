package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/health"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/model"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/orchestrator"
	"github.com/marlonbarreto-git/nimbus-payment-orchestrator/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServer() (*http.ServeMux, *orchestrator.Orchestrator) {
	mon := health.NewMonitorWithConfig(50, 10*time.Minute)
	procs := []processor.Processor{
		processor.NewPayFlow(),
		processor.NewCardMax(),
		processor.NewPixPay(),
		processor.NewGlobalPay(),
	}
	orch := orchestrator.New(procs, mon)
	h := New(orch)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux, orch
}

func TestProcessPayment_Success(t *testing.T) {
	mux, _ := setupTestServer()

	body := `{"transaction_id":"tx-001","amount":100.50,"currency":"BRL","payment_method":"card","customer_id":"cust-1"}`
	req := httptest.NewRequest("POST", "/payments", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	assert.Contains(t, []int{http.StatusOK, http.StatusUnprocessableEntity}, w.Code)

	var result model.PaymentResult
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "tx-001", result.TransactionID)
	assert.NotEmpty(t, result.Attempts)
}

func TestProcessPayment_ValidationErrors(t *testing.T) {
	mux, _ := setupTestServer()

	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			"missing transaction_id",
			`{"amount":100,"currency":"USD","payment_method":"card","customer_id":"c1"}`,
			"transaction_id is required",
		},
		{
			"zero amount",
			`{"transaction_id":"tx","amount":0,"currency":"USD","payment_method":"card","customer_id":"c1"}`,
			"amount must be greater than 0",
		},
		{
			"negative amount",
			`{"transaction_id":"tx","amount":-10,"currency":"USD","payment_method":"card","customer_id":"c1"}`,
			"amount must be greater than 0",
		},
		{
			"missing currency",
			`{"transaction_id":"tx","amount":100,"payment_method":"card","customer_id":"c1"}`,
			"currency is required",
		},
		{
			"invalid payment method",
			`{"transaction_id":"tx","amount":100,"currency":"USD","payment_method":"crypto","customer_id":"c1"}`,
			"payment_method must be one of",
		},
		{
			"missing customer_id",
			`{"transaction_id":"tx","amount":100,"currency":"USD","payment_method":"card"}`,
			"customer_id is required",
		},
		{
			"invalid JSON",
			`{invalid}`,
			"invalid request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/payments", bytes.NewBufferString(tt.body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
			var resp map[string]string
			json.Unmarshal(w.Body.Bytes(), &resp)
			assert.Contains(t, resp["error"], tt.expected)
		})
	}
}

func TestGetPaymentHistory_Found(t *testing.T) {
	mux, orch := setupTestServer()

	// First process a payment with a proper context
	payReq := model.PaymentRequest{
		TransactionID: "tx-history-001",
		Amount:        100.0,
		Currency:      "USD",
		PaymentMethod: "card",
		CustomerID:    "cust-1",
	}
	orch.ProcessPayment(context.Background(), payReq)

	// Then retrieve it
	req := httptest.NewRequest("GET", "/payments/tx-history-001", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result model.PaymentResult
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "tx-history-001", result.TransactionID)
}

func TestGetPaymentHistory_NotFound(t *testing.T) {
	mux, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/payments/tx-nonexistent", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetProcessorHealth(t *testing.T) {
	mux, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/health/processors", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp, "processors")
}

func TestSimulateDegrade(t *testing.T) {
	mux, _ := setupTestServer()

	body := `{"processor_name":"PayFlow","degraded":true}`
	req := httptest.NewRequest("POST", "/simulate/degrade", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "PayFlow", resp["processor"])
	assert.Equal(t, true, resp["degraded"])
}

func TestSimulateDegrade_NotFound(t *testing.T) {
	mux, _ := setupTestServer()

	body := `{"processor_name":"NonExistent","degraded":true}`
	req := httptest.NewRequest("POST", "/simulate/degrade", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSimulateBatch(t *testing.T) {
	mux, _ := setupTestServer()

	body := `{"count":10,"method":"card","currency":"USD"}`
	req := httptest.NewRequest("POST", "/simulate/batch", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(10), resp["total"])
	assert.Contains(t, resp, "approved")
	assert.Contains(t, resp, "approval_rate")
}

func TestSimulateBatch_InvalidCount(t *testing.T) {
	mux, _ := setupTestServer()

	tests := []struct {
		name string
		body string
	}{
		{"zero count", `{"count":0}`},
		{"negative count", `{"count":-5}`},
		{"too large", `{"count":1001}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/simulate/batch", bytes.NewBufferString(tt.body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestResponseContentType(t *testing.T) {
	mux, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/health/processors", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestProcessPayment_AllPaymentMethods(t *testing.T) {
	mux, _ := setupTestServer()

	methods := []string{"card", "pix", "oxxo", "pse"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			body := fmt.Sprintf(`{"transaction_id":"tx-%s","amount":50,"currency":"USD","payment_method":"%s","customer_id":"c1"}`, method, method)
			req := httptest.NewRequest("POST", "/payments", bytes.NewBufferString(body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Contains(t, []int{http.StatusOK, http.StatusUnprocessableEntity}, w.Code)
		})
	}
}

func TestProcessPayment_AllCurrencies(t *testing.T) {
	mux, _ := setupTestServer()

	currencies := []string{"USD", "BRL", "MXN", "COP"}
	for _, currency := range currencies {
		t.Run(currency, func(t *testing.T) {
			body := fmt.Sprintf(`{"transaction_id":"tx-%s","amount":100,"currency":"%s","payment_method":"card","customer_id":"c1"}`, currency, currency)
			req := httptest.NewRequest("POST", "/payments", bytes.NewBufferString(body))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			assert.Contains(t, []int{http.StatusOK, http.StatusUnprocessableEntity}, w.Code)
		})
	}
}

func TestSimulateDegrade_MissingProcessorName(t *testing.T) {
	mux, _ := setupTestServer()

	body := `{"degraded":true}`
	req := httptest.NewRequest("POST", "/simulate/degrade", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSimulateBatch_DefaultMethodAndCurrency(t *testing.T) {
	mux, _ := setupTestServer()

	body := `{"count":5}`
	req := httptest.NewRequest("POST", "/simulate/batch", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, float64(5), resp["total"])
}
