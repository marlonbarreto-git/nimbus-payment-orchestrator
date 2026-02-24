#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'
BOLD='\033[1m'

BASE_URL="http://localhost:8080"

echo -e "${BOLD}${CYAN}"
echo "============================================================"
echo "    Nimbus Payment Orchestrator - Demo Suite"
echo "    Intelligent Payment Routing with Retry Logic"
echo "============================================================"
echo -e "${NC}"

echo -e "${YELLOW}Starting server...${NC}"
go run cmd/server/main.go > /tmp/nimbus-server.log 2>&1 &
SERVER_PID=$!
sleep 2

if ! curl -s "$BASE_URL/health/processors" > /dev/null 2>&1; then
    echo -e "${RED}Server failed to start. Check /tmp/nimbus-server.log${NC}"
    kill $SERVER_PID 2>/dev/null
    exit 1
fi
echo -e "${GREEN}Server running on :8080 (PID: $SERVER_PID)${NC}"
echo ""

cleanup() {
    echo -e "\n${YELLOW}Stopping server...${NC}"
    kill $SERVER_PID 2>/dev/null
    wait $SERVER_PID 2>/dev/null
    echo -e "${GREEN}Done.${NC}"
}
trap cleanup EXIT

api() {
    local method=$1 endpoint=$2 data=$3
    if [ "$method" = "GET" ]; then
        curl -s "$BASE_URL$endpoint" | python3 -m json.tool 2>/dev/null || curl -s "$BASE_URL$endpoint"
    else
        curl -s -X "$method" "$BASE_URL$endpoint" -H "Content-Type: application/json" -d "$data" | python3 -m json.tool 2>/dev/null || curl -s -X "$method" "$BASE_URL$endpoint" -H "Content-Type: application/json" -d "$data"
    fi
}

echo -e "${BOLD}${BLUE}===== SCENARIO 1: Successful Card Payment =====${NC}"
echo -e "${CYAN}Single card payment - should be approved.${NC}"
api POST /payments '{"transaction_id":"tx-demo-001","amount":49.99,"currency":"USD","payment_method":"card","customer_id":"cust-001"}'
echo -e "\n${GREEN}Check: Payment processed with routing decision logged${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== SCENARIO 2: PIX Payment (LATAM) =====${NC}"
echo -e "${CYAN}PIX payment in BRL - PixPay has 90% approval for PIX.${NC}"
api POST /payments '{"transaction_id":"tx-demo-002","amount":150.00,"currency":"BRL","payment_method":"pix","customer_id":"cust-002"}'
echo -e "\n${GREEN}Check: Routed to processor with best PIX support${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== SCENARIO 3: Payment History Lookup =====${NC}"
echo -e "${CYAN}Retrieve attempt history for tx-demo-001.${NC}"
api GET /payments/tx-demo-001
echo -e "\n${GREEN}Check: Full attempt history with routing reasons${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== SCENARIO 4: Processor Health Check =====${NC}"
echo -e "${CYAN}Current health scores for all processors.${NC}"
api GET /health/processors
echo -e "\n${GREEN}Check: All 4 processors listed with health data${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== SCENARIO 5: Batch Processing (30 payments) =====${NC}"
echo -e "${CYAN}Process 30 card payments to build health data.${NC}"
api POST /simulate/batch '{"count":30,"method":"card","currency":"USD"}'
echo -e "\n${GREEN}Check: Approval rate and average attempts shown${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== SCENARIO 6: Processor Degradation =====${NC}"
echo -e "${CYAN}Degrade PayFlow to 80% errors, process 50 payments.${NC}"
echo -e "${YELLOW}Step 1: Degrade PayFlow${NC}"
api POST /simulate/degrade '{"processor_name":"PayFlow","degraded":true}'
echo ""
echo -e "${YELLOW}Step 2: Process 50 payments${NC}"
api POST /simulate/batch '{"count":50,"method":"card","currency":"USD"}'
echo ""
echo -e "${YELLOW}Step 3: Check health scores${NC}"
api GET /health/processors
echo -e "\n${GREEN}Check: PayFlow health score dropped, others stable${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== SCENARIO 7: Processor Recovery =====${NC}"
echo -e "${CYAN}Restore PayFlow, process 50 more payments.${NC}"
echo -e "${YELLOW}Step 1: Restore PayFlow${NC}"
api POST /simulate/degrade '{"processor_name":"PayFlow","degraded":false}'
echo ""
echo -e "${YELLOW}Step 2: Process 50 recovery payments${NC}"
api POST /simulate/batch '{"count":50,"method":"card","currency":"USD"}'
echo ""
echo -e "${YELLOW}Step 3: Health after recovery${NC}"
api GET /health/processors
echo -e "\n${GREEN}Check: PayFlow health recovering${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== SCENARIO 8: PIX Batch (BRL) =====${NC}"
echo -e "${CYAN}25 PIX payments - should see high approval via PixPay.${NC}"
api POST /simulate/batch '{"count":25,"method":"pix","currency":"BRL"}'
echo -e "\n${GREEN}Check: High approval rate (PixPay 90% for PIX)${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== SCENARIO 9: OXXO Payments (MXN) =====${NC}"
echo -e "${CYAN}15 OXXO payments in Mexican Pesos.${NC}"
api POST /simulate/batch '{"count":15,"method":"oxxo","currency":"MXN"}'
echo -e "\n${GREEN}Check: Routed to PayFlow/CardMax/GlobalPay${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== SCENARIO 10: PSE Payments (COP) =====${NC}"
echo -e "${CYAN}15 PSE payments in Colombian Pesos.${NC}"
api POST /simulate/batch '{"count":15,"method":"pse","currency":"COP"}'
echo -e "\n${GREEN}Check: Routed to PayFlow/GlobalPay (only PSE support)${NC}\n"
sleep 0.5

echo -e "${BOLD}${BLUE}===== FINAL: Health Summary =====${NC}"
api GET /health/processors
echo ""

echo -e "${BOLD}${CYAN}"
echo "============================================================"
echo "    Demo Complete - 200+ payments across all methods"
echo "    card, pix, oxxo, pse | USD, BRL, MXN, COP"
echo "============================================================"
echo -e "${NC}"
