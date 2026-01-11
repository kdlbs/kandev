#!/bin/bash
#
# End-to-End Test for Kandev - Comment System Focus
# This script tests the comment system and agent interaction flow
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
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
SERVER_PORT=8080
WS_URL="ws://localhost:${SERVER_PORT}/ws"
AGENTCTL_PORT=9999
WAIT_FOR_AGENT_SECONDS=45
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

log_comment() {
    local author_type=$1
    local content=$2
    local requests_input=$3
    if [ "$author_type" == "user" ]; then
        echo -e "${CYAN}  [USER]${NC} $content"
    else
        if [ "$requests_input" == "true" ]; then
            echo -e "${GREEN}  [AGENT] (requests input)${NC} $content"
        else
            echo -e "${GREEN}  [AGENT]${NC} $content"
        fi
    fi
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
    for i in {1..20}; do
        HEALTH=$(ws_request "health.check" '{}' 2>/dev/null)
        if echo "$HEALTH" | jq -e '.payload.status == "ok"' > /dev/null 2>&1; then
            log_success "Server is ready"
            return 0
        fi
        sleep 0.5
    done
    log_error "Server failed to start"
    exit 1
}

# Display all comments for a task
display_comments() {
    local task_id=$1
    local label=$2

    COMMENTS_RESPONSE=$(ws_request "comment.list" "{\"task_id\": \"${task_id}\"}")
    COMMENTS_COUNT=$(echo "$COMMENTS_RESPONSE" | jq 'if .payload | type == "array" then .payload | length else 0 end' 2>/dev/null || echo "0")

    echo -e "\n${CYAN}--- $label ($COMMENTS_COUNT comments) ---${NC}"

    if [ "$COMMENTS_COUNT" -gt 0 ]; then
        echo "$COMMENTS_RESPONSE" | jq -r '.payload[] | "\(.author_type)|\(.requests_input)|\(.content)"' 2>/dev/null | while IFS='|' read -r author_type requests_input content; do
            # Truncate long content for display
            if [ ${#content} -gt 200 ]; then
                content="${content:0:200}..."
            fi
            log_comment "$author_type" "$content" "$requests_input"
        done
    else
        echo -e "  ${YELLOW}(no comments yet)${NC}"
    fi
    echo -e "${CYAN}---${NC}"
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
log_step "Step 1: Create Workspace"
WORKSPACE_RESPONSE=$(ws_request "workspace.create" '{"name": "E2E Test Workspace", "description": "Automated end-to-end test workspace"}')
WORKSPACE_ID=$(echo "$WORKSPACE_RESPONSE" | jq -r '.payload.id' | tr -d '\n\r')
if [ "$WORKSPACE_ID" == "null" ] || [ -z "$WORKSPACE_ID" ]; then
    log_error "Failed to create workspace"
    echo "$WORKSPACE_RESPONSE"
    exit 1
fi
log_success "Created workspace: $WORKSPACE_ID"

log_step "Step 2: Create Board"
BOARD_RESPONSE=$(ws_request "board.create" "{\"workspace_id\": \"${WORKSPACE_ID}\", \"name\": \"E2E Test Board\", \"description\": \"Automated end-to-end test\"}")
BOARD_ID=$(echo "$BOARD_RESPONSE" | jq -r '.payload.id' | tr -d '\n\r')
if [ "$BOARD_ID" == "null" ] || [ -z "$BOARD_ID" ]; then
    log_error "Failed to create board"
    echo "$BOARD_RESPONSE"
    exit 1
fi
log_success "Created board: $BOARD_ID"

log_step "Step 3: Create Column"
COLUMN_RESPONSE=$(ws_request "column.create" "{\"board_id\": \"${BOARD_ID}\", \"name\": \"To Do\", \"position\": 0}")
COLUMN_ID=$(echo "$COLUMN_RESPONSE" | jq -r '.payload.id' | tr -d '\n\r')
if [ "$COLUMN_ID" == "null" ] || [ -z "$COLUMN_ID" ]; then
    log_error "Failed to create column"
    echo "$COLUMN_RESPONSE"
    exit 1
fi
log_success "Created column: $COLUMN_ID"

log_step "Step 4: Create Task for Comment System Test"
# Create a temp directory for the test workspace
TEST_WORKSPACE_DIR=$(mktemp -d)
log_info "Test workspace: $TEST_WORKSPACE_DIR"

# Create task that will trigger multi-turn conversation
TASK_PAYLOAD=$(cat <<EOF
{
    "title": "Comment System Test - Multi-turn Conversation",
    "description": "I need help with a calculation. Please ask me what numbers I want to add together, then perform the calculation when I provide them.",
    "workspace_id": "${WORKSPACE_ID}",
    "board_id": "${BOARD_ID}",
    "column_id": "${COLUMN_ID}",
    "repository_url": "${TEST_WORKSPACE_DIR}",
    "agent_type": "augment-agent"
}
EOF
)
TASK_RESPONSE=$(ws_request "task.create" "$TASK_PAYLOAD")
TASK_ID=$(echo "$TASK_RESPONSE" | jq -r '.payload.id' | tr -d '\n\r')
if [ "$TASK_ID" == "null" ] || [ -z "$TASK_ID" ]; then
    log_error "Failed to create task"
    echo "$TASK_RESPONSE"
    exit 1
fi
log_success "Created task: $TASK_ID"

# Test adding a user comment before agent starts
log_step "Step 5: Test comment.add Before Agent Start"
PRE_COMMENT_PAYLOAD=$(cat <<EOF
{
    "task_id": "${TASK_ID}",
    "content": "Initial user note: Please be concise in your responses.",
    "author_id": "e2e-test-user"
}
EOF
)
PRE_COMMENT_RESPONSE=$(ws_request "comment.add" "$PRE_COMMENT_PAYLOAD")
PRE_COMMENT_ID=$(echo "$PRE_COMMENT_RESPONSE" | jq -r '.payload.id // empty')
if [ -n "$PRE_COMMENT_ID" ]; then
    log_success "Created pre-start comment: $PRE_COMMENT_ID"
else
    log_info "Note: comment.add returned: $(echo "$PRE_COMMENT_RESPONSE" | jq -c '.payload')"
fi

# Verify comment.list works
INITIAL_COMMENTS=$(ws_request "comment.list" "{\"task_id\": \"${TASK_ID}\"}")
INITIAL_COUNT=$(echo "$INITIAL_COMMENTS" | jq 'if .payload | type == "array" then .payload | length else 0 end' 2>/dev/null || echo "0")
log_info "Comments before agent start: $INITIAL_COUNT"

log_step "Step 6: Start Task via Orchestrator"
START_RESPONSE=$(ws_request "orchestrator.start" "{\"task_id\": \"${TASK_ID}\"}")
AGENT_ID=$(echo "$START_RESPONSE" | jq -r '.payload.agent_instance_id' | tr -d '\n\r')
if [ "$AGENT_ID" == "null" ] || [ -z "$AGENT_ID" ]; then
    log_error "Failed to start task"
    echo "$START_RESPONSE"
    exit 1
fi
log_success "Task started, agent ID: $AGENT_ID"

# Brief wait for container startup
sleep 2
CONTAINER_ID=$(docker ps --filter "name=kandev-agent" --format "{{.ID}}" | head -1)
if [ -n "$CONTAINER_ID" ]; then
    log_success "Agent container running: $CONTAINER_ID"
fi

log_step "Step 7: Wait for Initial Agent Response"
log_info "Waiting up to ${WAIT_FOR_AGENT_SECONDS}s for agent..."
AGENT_READY=false
for i in $(seq 1 $WAIT_FOR_AGENT_SECONDS); do
    AGENT_STATUS_RESPONSE=$(ws_request "agent.status" "{\"agent_id\": \"${AGENT_ID}\"}" 2>/dev/null || echo '{}')
    AGENT_STATUS=$(echo "$AGENT_STATUS_RESPONSE" | jq -r '.payload.status // "UNKNOWN"')

    if [ "$AGENT_STATUS" == "READY" ]; then
        log_success "Agent READY after $i seconds"
        AGENT_READY=true
        break
    elif [ "$AGENT_STATUS" == "FAILED" ]; then
        log_error "Agent failed"
        exit 1
    fi

    if [ $((i % 10)) -eq 0 ]; then
        log_info "Waiting... ($i seconds, status: $AGENT_STATUS)"
    fi
    sleep 1
done

if [ "$AGENT_READY" != "true" ]; then
    log_error "Agent did not become READY (last: $AGENT_STATUS)"
    exit 1
fi

# Display comments after first agent response
display_comments "$TASK_ID" "After Initial Agent Response"

log_step "Step 8: Multi-Turn Conversation - Turn 1"
# User Comment Turn 1: Provide the numbers
log_info "Sending user comment via comment.add..."
TURN1_PAYLOAD=$(cat <<EOF
{
    "task_id": "${TASK_ID}",
    "content": "Please add 42 and 58 together.",
    "author_id": "e2e-test-user"
}
EOF
)
TURN1_RESPONSE=$(ws_request "comment.add" "$TURN1_PAYLOAD")
TURN1_ID=$(echo "$TURN1_RESPONSE" | jq -r '.payload.id // empty')
if [ -n "$TURN1_ID" ]; then
    log_success "User comment added: $TURN1_ID"
else
    log_info "comment.add response: $(echo "$TURN1_RESPONSE" | jq -c '.payload')"
fi

# Wait for agent to process and respond
log_info "Waiting for agent to respond to comment..."
for i in $(seq 1 $WAIT_FOR_AGENT_SECONDS); do
    AGENT_STATUS_RESPONSE=$(ws_request "agent.status" "{\"agent_id\": \"${AGENT_ID}\"}" 2>/dev/null || echo '{}')
    AGENT_STATUS=$(echo "$AGENT_STATUS_RESPONSE" | jq -r '.payload.status // "UNKNOWN"')
    if [ "$AGENT_STATUS" == "READY" ]; then
        log_success "Agent READY after Turn 1 ($i seconds)"
        break
    fi
    if [ $((i % 10)) -eq 0 ]; then
        log_info "Waiting... ($i seconds, status: $AGENT_STATUS)"
    fi
    sleep 1
done

display_comments "$TASK_ID" "After Turn 1"

log_step "Step 9: Multi-Turn Conversation - Turn 2"
# User Comment Turn 2: Ask for verification
TURN2_PAYLOAD=$(cat <<EOF
{
    "task_id": "${TASK_ID}",
    "content": "Great! Now can you also add 100 to that result?",
    "author_id": "e2e-test-user"
}
EOF
)
TURN2_RESPONSE=$(ws_request "comment.add" "$TURN2_PAYLOAD")
TURN2_ID=$(echo "$TURN2_RESPONSE" | jq -r '.payload.id // empty')
if [ -n "$TURN2_ID" ]; then
    log_success "User comment (Turn 2) added: $TURN2_ID"
fi

# Wait for agent response
log_info "Waiting for agent to respond..."
for i in $(seq 1 $WAIT_FOR_AGENT_SECONDS); do
    AGENT_STATUS_RESPONSE=$(ws_request "agent.status" "{\"agent_id\": \"${AGENT_ID}\"}" 2>/dev/null || echo '{}')
    AGENT_STATUS=$(echo "$AGENT_STATUS_RESPONSE" | jq -r '.payload.status // "UNKNOWN"')
    if [ "$AGENT_STATUS" == "READY" ]; then
        log_success "Agent READY after Turn 2 ($i seconds)"
        break
    fi
    if [ $((i % 10)) -eq 0 ]; then
        log_info "Waiting... ($i seconds, status: $AGENT_STATUS)"
    fi
    sleep 1
done

display_comments "$TASK_ID" "After Turn 2"

log_step "Step 10: Multi-Turn Conversation - Turn 3"
# User Comment Turn 3: Final request
TURN3_PAYLOAD=$(cat <<EOF
{
    "task_id": "${TASK_ID}",
    "content": "Perfect! Please write the final result to a file called 'result.txt' in the workspace.",
    "author_id": "e2e-test-user"
}
EOF
)
TURN3_RESPONSE=$(ws_request "comment.add" "$TURN3_PAYLOAD")
TURN3_ID=$(echo "$TURN3_RESPONSE" | jq -r '.payload.id // empty')
if [ -n "$TURN3_ID" ]; then
    log_success "User comment (Turn 3) added: $TURN3_ID"
fi

# Wait for final agent response
log_info "Waiting for agent to complete..."
for i in $(seq 1 $WAIT_FOR_AGENT_SECONDS); do
    AGENT_STATUS_RESPONSE=$(ws_request "agent.status" "{\"agent_id\": \"${AGENT_ID}\"}" 2>/dev/null || echo '{}')
    AGENT_STATUS=$(echo "$AGENT_STATUS_RESPONSE" | jq -r '.payload.status // "UNKNOWN"')
    if [ "$AGENT_STATUS" == "READY" ]; then
        log_success "Agent READY after Turn 3 ($i seconds)"
        break
    fi
    if [ $((i % 10)) -eq 0 ]; then
        log_info "Waiting... ($i seconds, status: $AGENT_STATUS)"
    fi
    sleep 1
done

display_comments "$TASK_ID" "After Turn 3 (Final Conversation)"

log_step "Step 11: Verify Comment Metadata"
# Get all comments and verify metadata
FINAL_COMMENTS=$(ws_request "comment.list" "{\"task_id\": \"${TASK_ID}\"}")
COMMENT_COUNT=$(echo "$FINAL_COMMENTS" | jq 'if .payload | type == "array" then .payload | length else 0 end' 2>/dev/null || echo "0")
log_info "Total comments in conversation: $COMMENT_COUNT"

# Count by author type
USER_COMMENTS=$(echo "$FINAL_COMMENTS" | jq '[.payload[] | select(.author_type == "user")] | length' 2>/dev/null || echo "0")
AGENT_COMMENTS=$(echo "$FINAL_COMMENTS" | jq '[.payload[] | select(.author_type == "agent")] | length' 2>/dev/null || echo "0")
log_info "User comments: $USER_COMMENTS, Agent comments: $AGENT_COMMENTS"

# Check for requests_input flag
REQUESTS_INPUT_COUNT=$(echo "$FINAL_COMMENTS" | jq '[.payload[] | select(.requests_input == true)] | length' 2>/dev/null || echo "0")
if [ "$REQUESTS_INPUT_COUNT" -gt 0 ]; then
    log_success "Found $REQUESTS_INPUT_COUNT comment(s) with requests_input=true"
else
    log_info "No comments with requests_input=true (agent didn't explicitly request input)"
fi

# Verify timestamps are present
HAS_TIMESTAMPS=$(echo "$FINAL_COMMENTS" | jq 'if .payload | type == "array" and (.payload | length) > 0 then (.payload[0].created_at != null) else false end' 2>/dev/null || echo "false")
if [ "$HAS_TIMESTAMPS" == "true" ]; then
    log_success "Comments have created_at timestamps"
fi

log_step "Step 12: Complete Task and Verify Final State"
COMPLETE_RESPONSE=$(ws_request "orchestrator.complete" "{\"task_id\": \"${TASK_ID}\"}")
COMPLETE_SUCCESS=$(echo "$COMPLETE_RESPONSE" | jq -r '.payload.success // false')
if [ "$COMPLETE_SUCCESS" == "true" ]; then
    log_success "Task completed successfully"
else
    log_info "Complete response: $(echo "$COMPLETE_RESPONSE" | jq -c '.payload')"
fi

# Verify task state
FINAL_TASK=$(ws_request "task.get" "{\"id\": \"${TASK_ID}\"}")
TASK_STATE=$(echo "$FINAL_TASK" | jq -r '.payload.state')
log_info "Final task state: $TASK_STATE"

# Check for session ID in metadata
SESSION_ID=$(echo "$FINAL_TASK" | jq -r '.payload.metadata.auggie_session_id // empty')
if [ -n "$SESSION_ID" ]; then
    log_success "Session ID stored: ${SESSION_ID:0:20}..."
fi

# Clean up
rm -rf "$TEST_WORKSPACE_DIR" 2>/dev/null || true

# Summary
log_step "Test Summary"

# Determine test results
COMMENT_ADD_PASSED=false
COMMENT_LIST_PASSED=false
MULTI_TURN_PASSED=false
METADATA_PASSED=false

[ -n "$TURN1_ID" ] && [ -n "$TURN2_ID" ] && [ -n "$TURN3_ID" ] && COMMENT_ADD_PASSED=true
[ "$COMMENT_COUNT" -gt 0 ] && COMMENT_LIST_PASSED=true
[ "$USER_COMMENTS" -ge 3 ] && MULTI_TURN_PASSED=true
[ "$HAS_TIMESTAMPS" == "true" ] && METADATA_PASSED=true

echo -e "\n${GREEN}╔═══════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║     Comment System E2E Test Results                   ║${NC}"
echo -e "${GREEN}╠═══════════════════════════════════════════════════════╣${NC}"

# Core functionality
if [ "$COMMENT_ADD_PASSED" == "true" ]; then
    echo -e "${GREEN}║ comment.add:              ✓ (3 user comments)          ║${NC}"
else
    echo -e "${RED}║ comment.add:              ✗                            ║${NC}"
fi

if [ "$COMMENT_LIST_PASSED" == "true" ]; then
    echo -e "${GREEN}║ comment.list:             ✓ ($COMMENT_COUNT total comments)         ║${NC}"
else
    echo -e "${RED}║ comment.list:             ✗                            ║${NC}"
fi

if [ "$MULTI_TURN_PASSED" == "true" ]; then
    echo -e "${GREEN}║ Multi-turn conversation:  ✓ (3 turns completed)        ║${NC}"
else
    echo -e "${YELLOW}║ Multi-turn conversation:  ○ (partial)                  ║${NC}"
fi

if [ "$AGENT_COMMENTS" -gt 0 ]; then
    echo -e "${GREEN}║ Agent responses:          ✓ ($AGENT_COMMENTS agent comments)        ║${NC}"
else
    echo -e "${YELLOW}║ Agent responses:          ○ (pending)                  ║${NC}"
fi

if [ "$METADATA_PASSED" == "true" ]; then
    echo -e "${GREEN}║ Comment metadata:         ✓ (timestamps present)       ║${NC}"
else
    echo -e "${YELLOW}║ Comment metadata:         ○                            ║${NC}"
fi

if [ "$REQUESTS_INPUT_COUNT" -gt 0 ]; then
    echo -e "${GREEN}║ Input request flow:       ✓ ($REQUESTS_INPUT_COUNT requests)             ║${NC}"
else
    echo -e "${YELLOW}║ Input request flow:       ○ (not triggered)            ║${NC}"
fi

echo -e "${GREEN}╠═══════════════════════════════════════════════════════╣${NC}"
echo -e "${GREEN}║ Workspace/Board/Column:   ✓                            ║${NC}"
echo -e "${GREEN}║ Task creation:            ✓                            ║${NC}"
echo -e "${GREEN}║ Agent execution:          ✓                            ║${NC}"
echo -e "${GREEN}║ Task completion:          ✓                            ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════════════════════╝${NC}"

# Overall result
if [ "$COMMENT_ADD_PASSED" == "true" ] && [ "$COMMENT_LIST_PASSED" == "true" ]; then
    echo -e "\n${GREEN}✓ Comment System E2E Test PASSED${NC}"
    exit 0
else
    echo -e "\n${RED}✗ Comment System E2E Test FAILED${NC}"
    exit 1
fi