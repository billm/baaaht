#!/bin/bash
# TUI End-to-End Verification Script
# This script verifies the TUI can connect to the orchestrator

set -e

SOCKET_PATH="/tmp/baaaht-grpc.sock"
TUI_BIN="./bin/tui"
ORCHESTRATOR_BIN="./bin/orchestrator"

echo "=== TUI End-to-End Verification ==="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
check_step() {
    if [ $? -eq 0 ]; then
        echo -e "${GREEN}✓${NC} $1"
    else
        echo -e "${RED}✗${NC} $1"
        exit 1
    fi
}

info() {
    echo -e "${YELLOW}ℹ${NC} $1"
}

# Step 1: Check binaries exist
echo "Step 1: Checking binaries..."
test -f "$TUI_BIN"
check_step "TUI binary exists"

test -f "$ORCHESTRATOR_BIN"
check_step "Orchestrator binary exists"

test -x "$TUI_BIN"
check_step "TUI binary is executable"

test -x "$ORCHESTRATOR_BIN"
check_step "Orchestrator binary is executable"

echo ""

# Step 2: Clean up any existing socket
echo "Step 2: Cleaning up existing socket..."
rm -f "$SOCKET_PATH"
check_step "Cleaned up existing socket"

echo ""

# Step 3: Start orchestrator
echo "Step 3: Starting orchestrator..."
info "Starting orchestrator in background..."
$ORCHESTRATOR_BIN serve --log-level info > /tmp/orchestrator.log 2>&1 &
ORCH_PID=$!
sleep 3

# Check orchestrator is running
ps -p $ORCH_PID > /dev/null
check_step "Orchestrator is running (PID: $ORCH_PID)"

# Check socket exists
test -S "$SOCKET_PATH"
check_step "gRPC socket created at $SOCKET_PATH"

echo ""

# Step 4: Run integration test
echo "Step 4: Running integration tests..."
/opt/homebrew/bin/go test -v ./pkg/tui -run TestOrchestratorConnection -timeout 30s
check_step "Integration tests passed"

echo ""

# Step 5: Verify TUI --help
echo "Step 5: Verifying TUI CLI..."
$TUI_BIN --help > /dev/null 2>&1
check_step "TUI --help works"

echo ""

# Step 6: Summary
echo "=== Verification Summary ==="
echo -e "${GREEN}✓${NC} All verification steps passed!"
echo ""
echo "The TUI has been verified to:"
echo "  1. Compile successfully"
echo "  2. Connect to the orchestrator via gRPC"
echo "  3. Create and manage sessions"
echo "  4. Perform health checks"
echo "  5. Close connections gracefully"
echo ""
echo "To manually test the TUI:"
echo "  1. Ensure orchestrator is running: $ORCHESTRATOR_BIN serve"
echo "  2. Start TUI: $TUI_BIN"
echo "  3. Type a message and press Ctrl+Enter to send"
echo "  4. Press Ctrl+C or Ctrl+D to quit"
echo ""
echo "To stop the orchestrator:"
echo "  kill $ORCH_PID"
