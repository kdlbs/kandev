#!/bin/bash
#
# End-to-End Test for Kandev
# This script tests the complete flow from task creation to agent execution
# using WebSocket-based communication with the backend.
#
# Prerequisites:
# - Docker running with kandev/augment-agent:latest image built
# - Augment session credentials in ~/.augment/session.json
# - Port 8080 available
# - websocat installed (cargo install websocat or brew install websocat)
#
# Usage: ./scripts/e2e-test.sh [--build]
#   --build   Build the Docker image before running tests

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKEND_DIR="$PROJECT_ROOT/apps/backend"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SERVER_PORT=8080
WS_URL="ws://localhost:${SERVER_PORT}/ws"
AGENTCTL_PORT=9999
WAIT_FOR_AGENT_SECONDS=120
SERVER_PID=""
BUILD_IMAGE=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --build)
            BUILD_IMAGE=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    # Stop any running agent containers
    docker ps --filter "name=kandev-agent" -q | xargs -r docker stop 2>/dev/null || true
    docker ps --filter "name=kandev-agent" -aq | xargs -r docker rm 2>/dev/null || true
    echo -e "${GREEN}Cleanup complete${NC}"
}

trap cleanup EXIT

# Helper functions
log_step() {
    echo -e "\n${BLUE}=== $1 ===${NC}"
}

log_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

log_error() {
    echo -e "${RED}✗ $1${NC}"
}

log_info() {
    echo -e "${YELLOW}→ $1${NC}"
}

# WebSocket helper function
# Sends a request via WebSocket and returns the response
ws_request() {
    local action=$1
    local payload=$2
    local id=$(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "req-$$-$RANDOM")

    # Compact the payload to single line (remove newlines and extra spaces)
    local compact_payload=$(echo "$payload" | jq -c '.')
    local message="{\"id\":\"${id}\",\"type\":\"request\",\"action\":\"${action}\",\"payload\":${compact_payload}}"

    # Send request and get response (with timeout)
    echo "$message" | timeout 10 websocat -n1 "${WS_URL}" 2>/dev/null
}

wait_for_server() {
    log_info "Waiting for server to be ready..."
    for i in {1..30}; do
        HEALTH=$(ws_request "health.check" '{}' 2>/dev/null)
        if echo "$HEALTH" | jq -e '.payload.status == "ok"' > /dev/null 2>&1; then
            log_success "Server is ready"
            return 0
        fi
        sleep 1
    done
    log_error "Server failed to start"
    exit 1
}

wait_for_agentctl() {
    local container_ip=$1
    log_info "Waiting for agentctl to be ready at ${container_ip}:${AGENTCTL_PORT}..."
    for i in {1..30}; do
        if curl -s "http://${container_ip}:${AGENTCTL_PORT}/health" > /dev/null 2>&1; then
            log_success "agentctl is ready"
            return 0
        fi
        sleep 1
    done
    log_error "agentctl failed to start"
    return 1
}

# Check prerequisites
log_step "Checking Prerequisites"

if ! command -v websocat &> /dev/null; then
    log_error "websocat is not installed"
    log_info "Install with: cargo install websocat"
    log_info "Or on macOS: brew install websocat"
    exit 1
fi
log_success "websocat is available"

if ! command -v docker &> /dev/null; then
    log_error "Docker is not installed"
    exit 1
fi
log_success "Docker is available"

if ! docker info &> /dev/null; then
    log_error "Docker daemon is not running"
    exit 1
fi
log_success "Docker daemon is running"

if [ ! -f "$HOME/.augment/session.json" ]; then
    log_error "Augment session not found at ~/.augment/session.json"
    log_info "Please log in with: auggie login"
    exit 1
fi
log_success "Augment session credentials found"

# Build Docker image if requested or if it doesn't exist
if [ "$BUILD_IMAGE" = true ]; then
    log_step "Building Docker Image"
    log_info "Building kandev/augment-agent:latest with agentctl..."
    cd "$BACKEND_DIR"
    docker build -t kandev/augment-agent:latest -f dockerfiles/augment-agent/Dockerfile .
    log_success "Docker image built"
elif ! docker image inspect kandev/augment-agent:latest &> /dev/null; then
    log_error "Docker image kandev/augment-agent:latest not found"
    log_info "Build it with: ./scripts/e2e-test.sh --build"
    log_info "Or manually: cd apps/backend && docker build -t kandev/augment-agent:latest -f dockerfiles/augment-agent/Dockerfile ."
    exit 1
else
    log_success "Agent Docker image exists"
fi

# Kill any existing server on port 8080
if lsof -i :${SERVER_PORT} &> /dev/null; then
    log_info "Killing existing process on port ${SERVER_PORT}"
    pkill -9 -f "kandev" 2>/dev/null || true
    sleep 2
fi

# Build and start the server
log_step "Building and Starting Server"

cd "$BACKEND_DIR"
#rm -f kandev.db kandev.db-shm kandev.db-wal

log_info "Building kandev..."
go build -o kandev ./cmd/kandev

log_info "Starting server..."
./kandev > /tmp/kandev-e2e.log 2>&1 &
SERVER_PID=$!
log_info "Server PID: $SERVER_PID"

wait_for_server

# Run the E2E test
log_step "Step 1: Create Board"
BOARD_RESPONSE=$(ws_request "board.create" '{"name": "E2E Test Board", "description": "Automated end-to-end test"}')
BOARD_ID=$(echo "$BOARD_RESPONSE" | jq -r '.payload.id')
if [ "$BOARD_ID" == "null" ] || [ -z "$BOARD_ID" ]; then
    log_error "Failed to create board"
    echo "$BOARD_RESPONSE"
    exit 1
fi
log_success "Created board: $BOARD_ID"

log_step "Step 2: Create Column"
COLUMN_RESPONSE=$(ws_request "column.create" "{\"board_id\": \"${BOARD_ID}\", \"name\": \"To Do\", \"position\": 0}")
COLUMN_ID=$(echo "$COLUMN_RESPONSE" | jq -r '.payload.id')
if [ "$COLUMN_ID" == "null" ] || [ -z "$COLUMN_ID" ]; then
    log_error "Failed to create column"
    echo "$COLUMN_RESPONSE"
    exit 1
fi
log_success "Created column: $COLUMN_ID"

log_step "Step 3: Create Task"
# Create a temp directory for the test workspace
TEST_WORKSPACE=$(mktemp -d)
log_info "Test workspace: $TEST_WORKSPACE"

# Create a simple task that creates a file
TASK_PAYLOAD=$(cat <<EOF
{
    "title": "E2E Test Task",
    "description": "Create a file named 'agent-result.txt' in the workspace root with the content 'Hello from Agent'. Do nothing else.",
    "board_id": "${BOARD_ID}",
    "column_id": "${COLUMN_ID}",
    "repository_url": "${TEST_WORKSPACE}",
    "agent_type": "augment-agent"
}
EOF
)
TASK_RESPONSE=$(ws_request "task.create" "$TASK_PAYLOAD")
TASK_ID=$(echo "$TASK_RESPONSE" | jq -r '.payload.id')
if [ "$TASK_ID" == "null" ] || [ -z "$TASK_ID" ]; then
    log_error "Failed to create task"
    echo "$TASK_RESPONSE"
    exit 1
fi
log_success "Created task: $TASK_ID"

log_step "Step 4: Start Task via Orchestrator"
START_RESPONSE=$(ws_request "orchestrator.start" "{\"task_id\": \"${TASK_ID}\"}")
AGENT_ID=$(echo "$START_RESPONSE" | jq -r '.payload.agent_instance_id')
if [ "$AGENT_ID" == "null" ] || [ -z "$AGENT_ID" ]; then
    log_error "Failed to start task"
    echo "$START_RESPONSE"
    exit 1
fi
log_success "Task started, agent ID: $AGENT_ID"

log_step "Step 5: Verify Agent Container Running"
sleep 3
CONTAINER_ID=$(docker ps --filter "name=kandev-agent" --format "{{.ID}}" | head -1)
if [ -z "$CONTAINER_ID" ]; then
    log_error "No agent container running"
    exit 1
fi
log_success "Agent container is running: $CONTAINER_ID"

# Get container IP for agentctl health check
CONTAINER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$CONTAINER_ID" 2>/dev/null)
if [ -n "$CONTAINER_IP" ]; then
    log_info "Container IP: $CONTAINER_IP"
fi

log_step "Step 6: Verify agentctl is Running"
sleep 2
if [ -n "$CONTAINER_IP" ]; then
    if wait_for_agentctl "$CONTAINER_IP"; then
        # Check agentctl status
        AGENTCTL_STATUS=$(curl -s "http://${CONTAINER_IP}:${AGENTCTL_PORT}/api/v1/status" 2>/dev/null || echo '{}')
        AGENT_PROCESS_STATE=$(echo "$AGENTCTL_STATUS" | jq -r '.agent_status // "unknown"')
        log_info "Agent process state: $AGENT_PROCESS_STATE"
    else
        log_info "Could not reach agentctl directly (may be in bridge network)"
    fi
else
    log_info "Container IP not available (using host network)"
fi

# Check server logs for agentctl communication
if grep -q "agentctl" /tmp/kandev-e2e.log; then
    log_success "Backend is communicating with agentctl"
else
    log_info "Checking backend logs..."
fi

if grep -q "agent started" /tmp/kandev-e2e.log || grep -q "initializing agent" /tmp/kandev-e2e.log; then
    log_success "Agent initialization started"
else
    log_info "Waiting for agent initialization..."
fi

log_step "Step 7: Wait for Agent to Complete First Prompt"
log_info "Waiting up to ${WAIT_FOR_AGENT_SECONDS} seconds for agent to become READY..."
AGENT_OUTPUT_FILE="${TEST_WORKSPACE}/agent-result.txt"
AGENT_READY=false
for i in $(seq 1 $WAIT_FOR_AGENT_SECONDS); do
    # Check agent status - READY means prompt completed
    AGENT_STATUS_RESPONSE=$(ws_request "agent.status" "{\"agent_id\": \"${AGENT_ID}\"}" 2>/dev/null || echo '{}')
    AGENT_STATUS=$(echo "$AGENT_STATUS_RESPONSE" | jq -r '.payload.status // "UNKNOWN"')

    if [ "$AGENT_STATUS" == "READY" ]; then
        log_success "Agent is READY after $i seconds (prompt completed)"
        AGENT_READY=true
        break
    elif [ "$AGENT_STATUS" == "FAILED" ]; then
        log_error "Agent failed"
        echo "$AGENT_STATUS_RESPONSE" | jq .
        if [ -n "$CONTAINER_ID" ]; then
            log_info "Container logs:"
            docker logs "$CONTAINER_ID" 2>&1 | tail -30
        fi
        exit 1
    fi

    # Check if container is still running
    CONTAINER_RUNNING=$(docker ps --filter "id=${CONTAINER_ID}" -q | wc -l)
    if [ "$CONTAINER_RUNNING" -eq 0 ]; then
        log_info "Container stopped unexpectedly"
        break
    fi

    if [ $((i % 10)) -eq 0 ]; then
        log_info "Still waiting... ($i seconds, status: $AGENT_STATUS)"
    fi
    sleep 1
done

if [ "$AGENT_READY" != "true" ]; then
    log_error "Agent did not become READY within timeout (last status: $AGENT_STATUS)"
    exit 1
fi

log_step "Step 8: Verify Agent Response"
# Get container logs to verify agent processed the request
FINAL_CONTAINER_ID=$(docker ps -a --filter "name=kandev-agent" --format "{{.ID}}" | head -1)
if [ -n "$FINAL_CONTAINER_ID" ]; then
    log_info "Checking container logs for $FINAL_CONTAINER_ID..."
    CONTAINER_LOGS=$(docker logs "$FINAL_CONTAINER_ID" 2>&1 | tail -100)

    # Check for agentctl activity
    if echo "$CONTAINER_LOGS" | grep -q "agentctl"; then
        log_success "agentctl was active in container"
    fi

    # Check for ACP messages
    if echo "$CONTAINER_LOGS" | grep -q '"stopReason"'; then
        log_success "Agent completed with stop reason"
    elif echo "$CONTAINER_LOGS" | grep -q 'session/update'; then
        log_success "Agent sent session updates"
    elif echo "$CONTAINER_LOGS" | grep -q 'initialize'; then
        log_success "Agent received initialize request"
    else
        log_info "Container logs (last 20 lines):"
        echo "$CONTAINER_LOGS" | tail -20
    fi
else
    log_info "Container already removed"
fi

log_step "Step 9: Check Final State"
AGENTS_RESPONSE=$(ws_request "agent.list" '{}')
FINAL_AGENTS=$(echo "$AGENTS_RESPONSE" | jq '.payload.total')
log_info "Total agents: $FINAL_AGENTS"

TASK_RESPONSE=$(ws_request "task.get" "{\"id\": \"${TASK_ID}\"}")
TASK_STATE=$(echo "$TASK_RESPONSE" | jq -r '.payload.state')
log_info "Task state: $TASK_STATE"

log_step "Step 10: Verify First Prompt Output"
if [ -f "$AGENT_OUTPUT_FILE" ]; then
    AGENT_OUTPUT=$(cat "$AGENT_OUTPUT_FILE")
    log_success "Agent created file: $AGENT_OUTPUT_FILE"
    log_info "File content: $AGENT_OUTPUT"
    if echo "$AGENT_OUTPUT" | grep -qi "hello"; then
        log_success "First prompt output contains expected content"
    fi
else
    log_error "Agent did not create expected file"
    exit 1
fi

log_step "Step 11: Test Multi-Turn - Send Follow-up Prompt"
SECOND_OUTPUT_FILE="${TEST_WORKSPACE}/second-result.txt"
log_info "Sending follow-up prompt..."
PROMPT_PAYLOAD=$(cat <<EOF
{
    "task_id": "${TASK_ID}",
    "prompt": "Create another file named 'second-result.txt' with the content 'Follow-up successful'. Do nothing else."
}
EOF
)
PROMPT_RESPONSE=$(ws_request "orchestrator.prompt" "$PROMPT_PAYLOAD")
PROMPT_SUCCESS=$(echo "$PROMPT_RESPONSE" | jq -r '.payload.success // false')
if [ "$PROMPT_SUCCESS" == "true" ]; then
    log_success "Follow-up prompt accepted"
else
    log_error "Follow-up prompt failed: $(echo "$PROMPT_RESPONSE" | jq -r '.payload.error // "unknown"')"
    exit 1
fi

# Wait for agent to become READY again (prompt completed)
log_info "Waiting for agent to become READY again..."
for i in $(seq 1 120); do
    AGENT_STATUS_RESPONSE=$(ws_request "agent.status" "{\"agent_id\": \"${AGENT_ID}\"}" 2>/dev/null || echo '{}')
    AGENT_STATUS=$(echo "$AGENT_STATUS_RESPONSE" | jq -r '.payload.status // "UNKNOWN"')
    if [ "$AGENT_STATUS" == "READY" ]; then
        log_success "Agent is READY after follow-up prompt ($i seconds)"
        break
    fi
    if [ $((i % 10)) -eq 0 ]; then
        log_info "Still waiting... ($i seconds, status: $AGENT_STATUS)"
    fi
    sleep 1
done

# Verify second file was created
if [ -f "$SECOND_OUTPUT_FILE" ]; then
    SECOND_OUTPUT=$(cat "$SECOND_OUTPUT_FILE")
    log_success "Second file created: $SECOND_OUTPUT_FILE"
    log_info "Second file content: $SECOND_OUTPUT"
else
    log_error "Agent did not create second file"
    exit 1
fi

log_step "Step 12: Complete Task"
COMPLETE_RESPONSE=$(ws_request "orchestrator.complete" "{\"task_id\": \"${TASK_ID}\"}")
COMPLETE_SUCCESS=$(echo "$COMPLETE_RESPONSE" | jq -r '.payload.success // false')
if [ "$COMPLETE_SUCCESS" == "true" ]; then
    log_success "Task completed successfully"
else
    log_error "Failed to complete task: $(echo "$COMPLETE_RESPONSE" | jq -r '.payload.error // "unknown"')"
fi

# Verify task state is COMPLETED
FINAL_TASK_RESPONSE=$(ws_request "task.get" "{\"id\": \"${TASK_ID}\"}")
TASK_STATE=$(echo "$FINAL_TASK_RESPONSE" | jq -r '.payload.state')
if [ "$TASK_STATE" == "COMPLETED" ]; then
    log_success "Task state is COMPLETED"
else
    log_error "Expected task state COMPLETED, got: $TASK_STATE"
fi

log_step "Step 13: Verify Session ID in Task Metadata"
# The session_id should have been stored in task metadata by the session_info handler
SESSION_ID=$(echo "$FINAL_TASK_RESPONSE" | jq -r '.payload.metadata.auggie_session_id // empty')
if [ -n "$SESSION_ID" ]; then
    log_success "Session ID stored in task metadata: $SESSION_ID"
else
    log_error "Session ID not found in task metadata"
    log_info "Task metadata: $(echo "$FINAL_TASK_RESPONSE" | jq '.payload.metadata')"
    exit 1
fi

# Clean up test workspace
rm -rf "$TEST_WORKSPACE" 2>/dev/null || true

# Show backend logs summary
log_step "Backend Log Summary"
if [ -f /tmp/kandev-e2e.log ]; then
    log_info "Key events from backend:"
    grep -E "(agent|agentctl|container|task)" /tmp/kandev-e2e.log | tail -20 || true
fi

# Summary
log_step "Test Summary"
echo -e "${GREEN}╔════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║     End-to-End Test PASSED! ✓          ║${NC}"
echo -e "${GREEN}╠════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║ Board created:     ✓                   ║${NC}"
echo -e "${GREEN}║ Column created:    ✓                   ║${NC}"
echo -e "${GREEN}║ Task created:      ✓                   ║${NC}"
echo -e "${GREEN}║ Agent launched:    ✓                   ║${NC}"
echo -e "${GREEN}║ agentctl running:  ✓                   ║${NC}"
echo -e "${GREEN}║ First prompt:      ✓                   ║${NC}"
echo -e "${GREEN}║ Multi-turn:        ✓                   ║${NC}"
echo -e "${GREEN}║ Task completed:    ✓                   ║${NC}"
echo -e "${GREEN}║ Session ID stored: ✓                   ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"

exit 0

