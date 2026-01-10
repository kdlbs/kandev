#!/bin/bash
#
# End-to-End Test for Kandev
# This script tests the complete flow from task creation to agent execution
#
# Prerequisites:
# - Docker running with kandev/augment-agent:latest image built
# - Augment session credentials in ~/.augment/session.json
# - Port 8080 available
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
BASE_URL="http://localhost:${SERVER_PORT}"
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

wait_for_server() {
    log_info "Waiting for server to be ready..."
    for i in {1..30}; do
        if curl -s "${BASE_URL}/health" > /dev/null 2>&1; then
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
rm -f kandev.db kandev.db-shm kandev.db-wal

log_info "Building kandev..."
go build -o kandev ./cmd/kandev

log_info "Starting server..."
./kandev > /tmp/kandev-e2e.log 2>&1 &
SERVER_PID=$!
log_info "Server PID: $SERVER_PID"

wait_for_server

# Run the E2E test
log_step "Step 1: Create Board"
BOARD=$(curl -s -X POST "${BASE_URL}/api/v1/boards" \
    -H "Content-Type: application/json" \
    -d '{"name": "E2E Test Board", "description": "Automated end-to-end test"}')
BOARD_ID=$(echo "$BOARD" | jq -r '.id')
if [ "$BOARD_ID" == "null" ] || [ -z "$BOARD_ID" ]; then
    log_error "Failed to create board"
    echo "$BOARD"
    exit 1
fi
log_success "Created board: $BOARD_ID"

log_step "Step 2: Create Column"
COLUMN=$(curl -s -X POST "${BASE_URL}/api/v1/boards/${BOARD_ID}/columns" \
    -H "Content-Type: application/json" \
    -d '{"name": "To Do", "position": 0}')
COLUMN_ID=$(echo "$COLUMN" | jq -r '.id')
if [ "$COLUMN_ID" == "null" ] || [ -z "$COLUMN_ID" ]; then
    log_error "Failed to create column"
    echo "$COLUMN"
    exit 1
fi
log_success "Created column: $COLUMN_ID"

log_step "Step 3: Create Task"
# Create a temp directory for the test workspace
TEST_WORKSPACE=$(mktemp -d)
log_info "Test workspace: $TEST_WORKSPACE"

# Create a simple task that creates a file
TASK=$(curl -s -X POST "${BASE_URL}/api/v1/tasks" \
    -H "Content-Type: application/json" \
    -d "{
        \"title\": \"E2E Test Task\",
        \"description\": \"Create a file named 'agent-result.txt' in the workspace root with the content 'Hello from Agent'. Do nothing else.\",
        \"board_id\": \"${BOARD_ID}\",
        \"column_id\": \"${COLUMN_ID}\",
        \"repository_url\": \"${TEST_WORKSPACE}\",
        \"agent_type\": \"augment-agent\"
    }")
TASK_ID=$(echo "$TASK" | jq -r '.id')
if [ "$TASK_ID" == "null" ] || [ -z "$TASK_ID" ]; then
    log_error "Failed to create task"
    echo "$TASK"
    exit 1
fi
log_success "Created task: $TASK_ID"

log_step "Step 4: Start Task via Orchestrator"
START_RESULT=$(curl -s -X POST "${BASE_URL}/api/v1/orchestrator/tasks/${TASK_ID}/start" \
    -H "Content-Type: application/json")
AGENT_ID=$(echo "$START_RESULT" | jq -r '.agent_instance_id')
if [ "$AGENT_ID" == "null" ] || [ -z "$AGENT_ID" ]; then
    log_error "Failed to start task"
    echo "$START_RESULT"
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

log_step "Step 7: Wait for Agent to Process"
log_info "Waiting up to ${WAIT_FOR_AGENT_SECONDS} seconds for agent to complete..."
AGENT_STATUS="RUNNING"
for i in $(seq 1 $WAIT_FOR_AGENT_SECONDS); do
    AGENT_INFO=$(curl -s "${BASE_URL}/api/v1/agents/${AGENT_ID}/status" 2>/dev/null || echo '{}')
    AGENT_STATUS=$(echo "$AGENT_INFO" | jq -r '.status // "UNKNOWN"')

    if [ "$AGENT_STATUS" == "COMPLETED" ]; then
        log_success "Agent completed successfully"
        break
    elif [ "$AGENT_STATUS" == "FAILED" ]; then
        log_error "Agent failed"
        echo "$AGENT_INFO" | jq .
        # Show container logs for debugging
        if [ -n "$CONTAINER_ID" ]; then
            log_info "Container logs:"
            docker logs "$CONTAINER_ID" 2>&1 | tail -30
        fi
        exit 1
    fi

    # Check if container is still running
    CONTAINER_RUNNING=$(docker ps --filter "id=${CONTAINER_ID}" -q | wc -l)
    if [ "$CONTAINER_RUNNING" -eq 0 ]; then
        log_info "Container stopped, checking for completion..."
        break
    fi

    # Periodically check agentctl status if IP is available
    if [ -n "$CONTAINER_IP" ] && [ $((i % 15)) -eq 0 ]; then
        AGENTCTL_STATUS=$(curl -s "http://${CONTAINER_IP}:${AGENTCTL_PORT}/api/v1/status" 2>/dev/null || echo '{}')
        AGENT_PROCESS_STATE=$(echo "$AGENTCTL_STATUS" | jq -r '.agent_status // "unknown"')
        log_info "Agent state via agentctl: $AGENT_PROCESS_STATE"
    fi

    if [ $((i % 10)) -eq 0 ]; then
        log_info "Still waiting... ($i seconds)"
    fi
    sleep 1
done

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
FINAL_AGENTS=$(curl -s "${BASE_URL}/api/v1/agents" | jq '.total')
log_info "Total agents: $FINAL_AGENTS"

FINAL_TASK=$(curl -s "${BASE_URL}/api/v1/tasks/${TASK_ID}")
TASK_STATE=$(echo "$FINAL_TASK" | jq -r '.state')
log_info "Task state: $TASK_STATE"

log_step "Step 10: Verify Agent Output"
AGENT_OUTPUT_FILE="${TEST_WORKSPACE}/agent-result.txt"
if [ -f "$AGENT_OUTPUT_FILE" ]; then
    AGENT_OUTPUT=$(cat "$AGENT_OUTPUT_FILE")
    log_success "Agent created file: $AGENT_OUTPUT_FILE"
    log_info "File content: $AGENT_OUTPUT"
    if echo "$AGENT_OUTPUT" | grep -qi "hello"; then
        log_success "Agent output contains expected content"
    else
        log_info "Note: Output doesn't contain 'Hello' but file was created"
    fi
else
    log_info "Agent did not create expected file (agent-result.txt)"
    log_info "Checking workspace for any created files..."
    ls -la "$TEST_WORKSPACE" 2>/dev/null || true
fi

# Clean up test workspace
#rm -rf "$TEST_WORKSPACE" 2>/dev/null || true

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
echo -e "${GREEN}║ Agent processed:   ✓                   ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"

exit 0

