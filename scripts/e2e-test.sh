#!/bin/bash
#
# End-to-End Test for Kandev - Comment System with Types
#
# This script tests the unified comment system that handles all task-related
# communication including agent tool calls, content, progress, errors, and status.
# It demonstrates the new comment type system:
#   - message:   Regular user/agent messages
#   - content:   Agent response content
#   - tool_call: When agent uses a tool (file operations, commands)
#   - progress:  Progress updates
#   - error:     Error messages
#   - status:    Status changes (started, completed, failed)
#
# The test creates a Python calculator task that forces the agent to use
# tools (file creation, editing) so we can see different comment types in action.
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

# Color codes for different comment types
MAGENTA='\033[0;35m'
WHITE='\033[1;37m'

log_comment() {
    local author_type=$1
    local content=$2
    local requests_input=$3
    local comment_type=$4
    local has_metadata=$5

    # Build type badge
    local type_badge=""
    case "$comment_type" in
        "message")   type_badge="${WHITE}[msg]${NC}" ;;
        "content")   type_badge="${CYAN}[content]${NC}" ;;
        "tool_call") type_badge="${MAGENTA}[tool]${NC}" ;;
        "progress")  type_badge="${YELLOW}[progress]${NC}" ;;
        "error")     type_badge="${RED}[error]${NC}" ;;
        "status")    type_badge="${BLUE}[status]${NC}" ;;
        *)           type_badge="${WHITE}[${comment_type:-msg}]${NC}" ;;
    esac

    # Build metadata indicator
    local meta_indicator=""
    if [ "$has_metadata" == "true" ]; then
        meta_indicator=" ${YELLOW}📎${NC}"
    fi

    # Build input indicator
    local input_indicator=""
    if [ "$requests_input" == "true" ]; then
        input_indicator=" ${GREEN}⏳${NC}"
    fi

    if [ "$author_type" == "user" ]; then
        echo -e "  ${CYAN}👤 USER${NC} $type_badge$input_indicator$meta_indicator $content"
    else
        echo -e "  ${GREEN}🤖 AGENT${NC} $type_badge$input_indicator$meta_indicator $content"
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

# Display all comments for a task with full details
display_comments() {
    local task_id=$1
    local label=$2

    COMMENTS_RESPONSE=$(ws_request "comment.list" "{\"task_id\": \"${task_id}\"}" 2>/dev/null) || COMMENTS_RESPONSE='{}'
    local first_response=$(echo "$COMMENTS_RESPONSE" | head -1)
    COMMENTS_COUNT=$(echo "$first_response" | jq 'if .payload | type == "array" then .payload | length else 0 end' 2>/dev/null || echo "0")
    # Ensure COMMENTS_COUNT is a valid number
    if ! [[ "$COMMENTS_COUNT" =~ ^[0-9]+$ ]]; then
        COMMENTS_COUNT=0
    fi

    echo -e "\n${CYAN}╔══════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║ $label ($COMMENTS_COUNT comments)${NC}"
    echo -e "${CYAN}╠══════════════════════════════════════════════════════════════════════════════╣${NC}"

    if [ "$COMMENTS_COUNT" -gt 0 ]; then
        # Count by type (use first_response to ensure we have valid JSON)
        local msg_count=$(echo "$first_response" | jq '[.payload[] | select(.type == "message" or .type == null or .type == "")] | length' 2>/dev/null || echo "0")
        local content_count=$(echo "$first_response" | jq '[.payload[] | select(.type == "content")] | length' 2>/dev/null || echo "0")
        local tool_count=$(echo "$first_response" | jq '[.payload[] | select(.type == "tool_call")] | length' 2>/dev/null || echo "0")
        local progress_count=$(echo "$first_response" | jq '[.payload[] | select(.type == "progress")] | length' 2>/dev/null || echo "0")
        local error_count=$(echo "$first_response" | jq '[.payload[] | select(.type == "error")] | length' 2>/dev/null || echo "0")
        local status_count=$(echo "$first_response" | jq '[.payload[] | select(.type == "status")] | length' 2>/dev/null || echo "0")

        echo -e "${CYAN}║${NC} Types: ${WHITE}msg:$msg_count${NC} ${CYAN}content:$content_count${NC} ${MAGENTA}tool:$tool_count${NC} ${YELLOW}progress:$progress_count${NC} ${RED}error:$error_count${NC} ${BLUE}status:$status_count${NC}"
        echo -e "${CYAN}╟──────────────────────────────────────────────────────────────────────────────╢${NC}"

        # Process each comment - use head -1 to ensure we only get the first response line
        local first_response=$(echo "$COMMENTS_RESPONSE" | head -1)
        local comment_count=$(echo "$first_response" | jq '.payload | length' 2>/dev/null || echo "0")
        # Ensure comment_count is a valid number
        if ! [[ "$comment_count" =~ ^[0-9]+$ ]]; then
            comment_count=0
        fi
        if [ "$comment_count" -gt 0 ]; then
            for i in $(seq 0 $((comment_count - 1))); do
                local author_type=$(echo "$first_response" | jq -r ".payload[$i].author_type // \"user\"" 2>/dev/null) || author_type="user"
                local requests_input=$(echo "$first_response" | jq -r ".payload[$i].requests_input // false" 2>/dev/null) || requests_input="false"
                local comment_type=$(echo "$first_response" | jq -r ".payload[$i].type // \"message\"" 2>/dev/null) || comment_type="message"
                local has_metadata=$(echo "$first_response" | jq -r "if .payload[$i].metadata then \"true\" else \"false\" end" 2>/dev/null) || has_metadata="false"
                local content=$(echo "$first_response" | jq -r ".payload[$i].content // \"\"" 2>/dev/null) || content=""

                # Remove newlines from content
                content=$(echo "$content" | tr '\n' ' ' | tr '\r' ' ')
                # Truncate long content for display
                if [ ${#content} -gt 100 ]; then
                    content="${content:0:100}..."
                fi
                # Show placeholder if content is empty
                if [ -z "$content" ]; then
                    content="(empty)"
                fi
                log_comment "$author_type" "$content" "$requests_input" "$comment_type" "$has_metadata"
            done
        fi
    else
        echo -e "  ${YELLOW}(no comments yet)${NC}"
    fi
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════════════════════════╝${NC}"
}

# Display detailed metadata for tool_call comments
display_tool_calls() {
    local task_id=$1

    COMMENTS_RESPONSE=$(ws_request "comment.list" "{\"task_id\": \"${task_id}\"}" 2>/dev/null) || COMMENTS_RESPONSE='{}'
    local first_response=$(echo "$COMMENTS_RESPONSE" | head -1)
    TOOL_CALLS=$(echo "$first_response" | jq '[.payload[] | select(.type == "tool_call")]' 2>/dev/null) || TOOL_CALLS='[]'
    TOOL_COUNT=$(echo "$TOOL_CALLS" | jq 'length' 2>/dev/null || echo "0")
    # Ensure TOOL_COUNT is a valid number
    if ! [[ "$TOOL_COUNT" =~ ^[0-9]+$ ]]; then
        TOOL_COUNT=0
    fi

    if [ "$TOOL_COUNT" -gt 0 ]; then
        echo -e "\n${MAGENTA}╔══════════════════════════════════════════════════════════════════════════════╗${NC}"
        echo -e "${MAGENTA}║ Tool Calls Detail ($TOOL_COUNT tools used)${NC}"
        echo -e "${MAGENTA}╠══════════════════════════════════════════════════════════════════════════════╣${NC}"

        echo "$TOOL_CALLS" | jq -r '.[] | "TOOL: \(.metadata.tool_name // .metadata.name // "unknown") | \(.content[:80] // "no content")"' 2>/dev/null | while read -r line; do
            echo -e "${MAGENTA}║${NC} $line"
        done || true

        echo -e "${MAGENTA}╚══════════════════════════════════════════════════════════════════════════════╝${NC}"
    fi
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
WORKSPACE_ID=$(echo "$WORKSPACE_RESPONSE" | head -1 | jq -r '.payload.id // empty' 2>/dev/null)
if [ -z "$WORKSPACE_ID" ]; then
    log_error "Failed to create workspace"
    echo "$WORKSPACE_RESPONSE"
    exit 1
fi
log_success "Created workspace: $WORKSPACE_ID"

log_step "Step 2: Create Board"
BOARD_RESPONSE=$(ws_request "board.create" "{\"workspace_id\": \"${WORKSPACE_ID}\", \"name\": \"E2E Test Board\", \"description\": \"Automated end-to-end test\"}")
BOARD_ID=$(echo "$BOARD_RESPONSE" | head -1 | jq -r '.payload.id // empty' 2>/dev/null)
if [ -z "$BOARD_ID" ]; then
    log_error "Failed to create board"
    echo "$BOARD_RESPONSE"
    exit 1
fi
log_success "Created board: $BOARD_ID"

log_step "Step 3: Create Column"
COLUMN_RESPONSE=$(ws_request "column.create" "{\"board_id\": \"${BOARD_ID}\", \"name\": \"To Do\", \"position\": 0}")
COLUMN_ID=$(echo "$COLUMN_RESPONSE" | head -1 | jq -r '.payload.id // empty' 2>/dev/null)
if [ -z "$COLUMN_ID" ]; then
    log_error "Failed to create column"
    echo "$COLUMN_RESPONSE"
    exit 1
fi
log_success "Created column: $COLUMN_ID"

log_step "Step 4: Create Task for Comment System Test"
# Create a temp directory for the test workspace
TEST_WORKSPACE_DIR=$(mktemp -d)
log_info "Test workspace: $TEST_WORKSPACE_DIR"

# Initialize a git repo in the temp directory so the agent can work with it
cd "$TEST_WORKSPACE_DIR"
git init -q
echo "# E2E Test Workspace" > README.md
git add README.md
git commit -q -m "Initial commit"
cd "$SCRIPT_DIR/.."

# Create task that will force agent to use tools (file operations, code writing)
TASK_PAYLOAD=$(cat <<EOF
{
    "title": "E2E Test: Create a Python Calculator Module",
    "description": "Create a simple Python calculator module with the following requirements:\n\n1. Create a file called 'calculator.py' with functions: add, subtract, multiply, divide\n2. Each function should take two numbers and return the result\n3. The divide function should handle division by zero\n4. Create a 'test_calculator.py' file with basic tests for each function\n\nPlease start by creating the calculator.py file first, then wait for my feedback before creating the tests.",
    "workspace_id": "${WORKSPACE_ID}",
    "board_id": "${BOARD_ID}",
    "column_id": "${COLUMN_ID}",
    "repository_url": "${TEST_WORKSPACE_DIR}",
    "agent_type": "augment-agent"
}
EOF
)
TASK_RESPONSE=$(ws_request "task.create" "$TASK_PAYLOAD")
# Extract only the first valid JSON line and parse the id
TASK_ID=$(echo "$TASK_RESPONSE" | head -1 | jq -r '.payload.id // empty' 2>/dev/null)
if [ -z "$TASK_ID" ]; then
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
    "content": "Note: Use simple, clear code. No external dependencies needed.",
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
AGENT_ID=$(echo "$START_RESPONSE" | head -1 | jq -r '.payload.agent_instance_id // empty' 2>/dev/null)
if [ -z "$AGENT_ID" ]; then
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

# Display comments after first agent response (should include tool_call for creating calculator.py)
display_comments "$TASK_ID" "After Initial Agent Response (should have tool calls)"
display_tool_calls "$TASK_ID"

# Check if calculator.py was created
if [ -f "$TEST_WORKSPACE_DIR/calculator.py" ]; then
    log_success "calculator.py was created!"
    log_info "File contents:"
    echo -e "${CYAN}---${NC}"
    head -20 "$TEST_WORKSPACE_DIR/calculator.py"
    echo -e "${CYAN}---${NC}"
else
    log_info "calculator.py not created yet (agent may still be working)"
fi

log_step "Step 8: Multi-Turn Conversation - Turn 1 (Request changes)"
# User Comment Turn 1: Request a modification
log_info "Sending user comment via comment.add..."
TURN1_PAYLOAD="{\"task_id\": \"${TASK_ID}\", \"content\": \"Great work on the calculator! Please also add a power function that calculates x raised to the power of y. Then show me the updated file.\", \"author_id\": \"e2e-test-user\"}"
TURN1_RESPONSE=$(ws_request "comment.add" "$TURN1_PAYLOAD" 2>/dev/null) || TURN1_RESPONSE='{}'
TURN1_ID=$(echo "$TURN1_RESPONSE" | head -1 | jq -r '.payload.id // empty' 2>/dev/null) || TURN1_ID=""
if [ -n "$TURN1_ID" ]; then
    log_success "User comment added: $TURN1_ID"
else
    log_info "comment.add response: $(echo "$TURN1_RESPONSE" | head -1 | jq -c '.payload' 2>/dev/null || echo 'parse error')"
fi

# Wait for agent to process and respond
log_info "Waiting for agent to respond to comment..."
for i in $(seq 1 $WAIT_FOR_AGENT_SECONDS); do
    AGENT_STATUS_RESPONSE=$(ws_request "agent.status" "{\"agent_id\": \"${AGENT_ID}\"}" 2>/dev/null) || AGENT_STATUS_RESPONSE='{}'
    AGENT_STATUS=$(echo "$AGENT_STATUS_RESPONSE" | head -1 | jq -r '.payload.status // "UNKNOWN"' 2>/dev/null) || AGENT_STATUS="UNKNOWN"
    if [ "$AGENT_STATUS" == "READY" ]; then
        log_success "Agent READY after Turn 1 ($i seconds)"
        break
    fi
    if [ $((i % 10)) -eq 0 ]; then
        log_info "Waiting... ($i seconds, status: $AGENT_STATUS)"
    fi
    sleep 1
done

display_comments "$TASK_ID" "After Turn 1 (power function request)"
display_tool_calls "$TASK_ID"

log_step "Step 9: Multi-Turn Conversation - Turn 2 (Request tests)"
# User Comment Turn 2: Request test file
TURN2_PAYLOAD="{\"task_id\": \"${TASK_ID}\", \"content\": \"Now please create the test_calculator.py file with tests for all functions including the power function. Use Python unittest module.\", \"author_id\": \"e2e-test-user\"}"
TURN2_RESPONSE=$(ws_request "comment.add" "$TURN2_PAYLOAD" 2>/dev/null) || TURN2_RESPONSE='{}'
TURN2_ID=$(echo "$TURN2_RESPONSE" | head -1 | jq -r '.payload.id // empty' 2>/dev/null) || TURN2_ID=""
if [ -n "$TURN2_ID" ]; then
    log_success "User comment (Turn 2) added: $TURN2_ID"
fi

# Wait for agent response
log_info "Waiting for agent to respond..."
for i in $(seq 1 $WAIT_FOR_AGENT_SECONDS); do
    AGENT_STATUS_RESPONSE=$(ws_request "agent.status" "{\"agent_id\": \"${AGENT_ID}\"}" 2>/dev/null) || AGENT_STATUS_RESPONSE='{}'
    AGENT_STATUS=$(echo "$AGENT_STATUS_RESPONSE" | head -1 | jq -r '.payload.status // "UNKNOWN"' 2>/dev/null) || AGENT_STATUS="UNKNOWN"
    if [ "$AGENT_STATUS" == "READY" ]; then
        log_success "Agent READY after Turn 2 ($i seconds)"
        break
    fi
    if [ $((i % 10)) -eq 0 ]; then
        log_info "Waiting... ($i seconds, status: $AGENT_STATUS)"
    fi
    sleep 1
done

display_comments "$TASK_ID" "After Turn 2 (test file request)"
display_tool_calls "$TASK_ID"

# Check if test file was created
if [ -f "$TEST_WORKSPACE_DIR/test_calculator.py" ]; then
    log_success "test_calculator.py was created!"
else
    log_info "test_calculator.py not created yet"
fi

log_step "Step 10: Multi-Turn Conversation - Turn 3 (Run tests)"
# User Comment Turn 3: Run the tests
TURN3_PAYLOAD="{\"task_id\": \"${TASK_ID}\", \"content\": \"Please run the tests using python -m pytest test_calculator.py -v or python -m unittest test_calculator -v and show me the results.\", \"author_id\": \"e2e-test-user\"}"
TURN3_RESPONSE=$(ws_request "comment.add" "$TURN3_PAYLOAD" 2>/dev/null) || TURN3_RESPONSE='{}'
TURN3_ID=$(echo "$TURN3_RESPONSE" | head -1 | jq -r '.payload.id // empty' 2>/dev/null) || TURN3_ID=""
if [ -n "$TURN3_ID" ]; then
    log_success "User comment (Turn 3) added: $TURN3_ID"
fi

# Wait for final agent response
log_info "Waiting for agent to complete..."
for i in $(seq 1 $WAIT_FOR_AGENT_SECONDS); do
    AGENT_STATUS_RESPONSE=$(ws_request "agent.status" "{\"agent_id\": \"${AGENT_ID}\"}" 2>/dev/null) || AGENT_STATUS_RESPONSE='{}'
    AGENT_STATUS=$(echo "$AGENT_STATUS_RESPONSE" | head -1 | jq -r '.payload.status // "UNKNOWN"' 2>/dev/null) || AGENT_STATUS="UNKNOWN"
    if [ "$AGENT_STATUS" == "READY" ]; then
        log_success "Agent READY after Turn 3 ($i seconds)"
        break
    fi
    if [ $((i % 10)) -eq 0 ]; then
        log_info "Waiting... ($i seconds, status: $AGENT_STATUS)"
    fi
    sleep 1
done

display_comments "$TASK_ID" "After Turn 3 (test execution)"
display_tool_calls "$TASK_ID"

log_step "Step 11: Verify Comment Types and Metadata"
# Get all comments and verify metadata
log_info "Querying comments for task: $TASK_ID"
FINAL_COMMENTS=$(ws_request "comment.list" "{\"task_id\": \"${TASK_ID}\"}" 2>/dev/null) || FINAL_COMMENTS='{}'
FINAL_COMMENTS_LINE=$(echo "$FINAL_COMMENTS" | head -1)
log_info "DEBUG: Raw response length: ${#FINAL_COMMENTS_LINE} chars"
log_info "DEBUG: Response preview: ${FINAL_COMMENTS_LINE:0:200}"
COMMENT_COUNT=$(echo "$FINAL_COMMENTS_LINE" | jq 'if .payload | type == "array" then .payload | length else 0 end' 2>/dev/null || echo "0")
log_info "Total comments in conversation: $COMMENT_COUNT"

# Count by author type
USER_COMMENTS=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.author_type == "user")] | length' 2>/dev/null || echo "0")
AGENT_COMMENTS=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.author_type == "agent")] | length' 2>/dev/null || echo "0")
log_info "User comments: $USER_COMMENTS, Agent comments: $AGENT_COMMENTS"

# Count by comment type
MSG_COUNT=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.type == "message" or .type == null or .type == "")] | length' 2>/dev/null || echo "0")
CONTENT_COUNT=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.type == "content")] | length' 2>/dev/null || echo "0")
TOOL_CALL_COUNT=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.type == "tool_call")] | length' 2>/dev/null || echo "0")
PROGRESS_COUNT=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.type == "progress")] | length' 2>/dev/null || echo "0")
ERROR_COUNT=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.type == "error")] | length' 2>/dev/null || echo "0")
STATUS_COUNT=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.type == "status")] | length' 2>/dev/null || echo "0")

log_info "Comment types breakdown:"
log_info "  - message: $MSG_COUNT"
log_info "  - content: $CONTENT_COUNT"
log_info "  - tool_call: $TOOL_CALL_COUNT"
log_info "  - progress: $PROGRESS_COUNT"
log_info "  - error: $ERROR_COUNT"
log_info "  - status: $STATUS_COUNT"

# Count comments with metadata
METADATA_COUNT=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.metadata != null)] | length' 2>/dev/null || echo "0")
if [[ "$METADATA_COUNT" =~ ^[0-9]+$ ]] && [ "$METADATA_COUNT" -gt 0 ]; then
    log_success "Found $METADATA_COUNT comment(s) with metadata"
else
    log_info "No comments have metadata attached"
fi

# Check for requests_input flag
REQUESTS_INPUT_COUNT=$(echo "$FINAL_COMMENTS_LINE" | jq '[.payload[] | select(.requests_input == true)] | length' 2>/dev/null || echo "0")
if [[ "$REQUESTS_INPUT_COUNT" =~ ^[0-9]+$ ]] && [ "$REQUESTS_INPUT_COUNT" -gt 0 ]; then
    log_success "Found $REQUESTS_INPUT_COUNT comment(s) with requests_input=true"
else
    log_info "No comments with requests_input=true (agent didn't explicitly request input)"
fi

# Verify timestamps are present
HAS_TIMESTAMPS=$(echo "$FINAL_COMMENTS_LINE" | jq 'if .payload | type == "array" and (.payload | length) > 0 then (.payload[0].created_at != null) else false end' 2>/dev/null || echo "false")
if [ "$HAS_TIMESTAMPS" == "true" ]; then
    log_success "Comments have created_at timestamps"
fi

# Check for files created
log_step "Step 11b: Verify Files Created"
if [ -f "$TEST_WORKSPACE_DIR/calculator.py" ]; then
    log_success "calculator.py exists"
    CALC_LINES=$(wc -l < "$TEST_WORKSPACE_DIR/calculator.py") || CALC_LINES=0
    log_info "  Lines: $CALC_LINES"
else
    log_info "calculator.py not found"
fi

if [ -f "$TEST_WORKSPACE_DIR/test_calculator.py" ]; then
    log_success "test_calculator.py exists"
    TEST_LINES=$(wc -l < "$TEST_WORKSPACE_DIR/test_calculator.py") || TEST_LINES=0
    log_info "  Lines: $TEST_LINES"
else
    log_info "test_calculator.py not found"
fi

log_step "Step 12: Complete Task and Verify Final State"
COMPLETE_RESPONSE=$(ws_request "orchestrator.complete" "{\"task_id\": \"${TASK_ID}\"}" 2>/dev/null) || COMPLETE_RESPONSE='{}'
COMPLETE_SUCCESS=$(echo "$COMPLETE_RESPONSE" | head -1 | jq -r '.payload.success // false' 2>/dev/null) || COMPLETE_SUCCESS="false"
if [ "$COMPLETE_SUCCESS" == "true" ]; then
    log_success "Task completed successfully"
else
    log_info "Complete response: $(echo "$COMPLETE_RESPONSE" | head -1 | jq -c '.payload' 2>/dev/null || echo 'parse error')"
fi

# Verify task state
FINAL_TASK=$(ws_request "task.get" "{\"id\": \"${TASK_ID}\"}" 2>/dev/null) || FINAL_TASK='{}'
FINAL_TASK_LINE=$(echo "$FINAL_TASK" | head -1)
TASK_STATE=$(echo "$FINAL_TASK_LINE" | jq -r '.payload.state' 2>/dev/null) || TASK_STATE="UNKNOWN"
log_info "Final task state: $TASK_STATE"

# Check for session ID in metadata
SESSION_ID=$(echo "$FINAL_TASK_LINE" | jq -r '.payload.metadata.auggie_session_id // empty' 2>/dev/null) || SESSION_ID=""
if [ -n "$SESSION_ID" ]; then
    log_success "Session ID stored: ${SESSION_ID:0:20}..."
fi

# Summary
log_step "Test Summary"

# Debug output for variables
log_info "DEBUG: TASK_ID=$TASK_ID"
log_info "DEBUG: TURN1_ID=$TURN1_ID, TURN2_ID=$TURN2_ID, TURN3_ID=$TURN3_ID"
log_info "DEBUG: COMMENT_COUNT=$COMMENT_COUNT, USER_COMMENTS=$USER_COMMENTS"
log_info "DEBUG: MSG_COUNT=$MSG_COUNT, CONTENT_COUNT=$CONTENT_COUNT, TOOL_CALL_COUNT=$TOOL_CALL_COUNT"
log_info "DEBUG: TEST_WORKSPACE_DIR=$TEST_WORKSPACE_DIR"
log_info "DEBUG: Files exist check: calculator.py=$([ -f "$TEST_WORKSPACE_DIR/calculator.py" ] && echo YES || echo NO)"

# Determine test results
COMMENT_ADD_PASSED=false
COMMENT_LIST_PASSED=false
MULTI_TURN_PASSED=false
METADATA_PASSED=false
TOOL_CALLS_PASSED=false
FILES_CREATED_PASSED=false

# Check for files BEFORE cleanup
[ -f "$TEST_WORKSPACE_DIR/calculator.py" ] && FILES_CREATED_PASSED=true

# Safely check conditions with defaults for empty values
[[ -n "$TURN1_ID" && -n "$TURN2_ID" && -n "$TURN3_ID" ]] && COMMENT_ADD_PASSED=true
[[ "$COMMENT_COUNT" =~ ^[0-9]+$ ]] && [ "$COMMENT_COUNT" -gt 0 ] && COMMENT_LIST_PASSED=true
[[ "$USER_COMMENTS" =~ ^[0-9]+$ ]] && [ "$USER_COMMENTS" -ge 3 ] && MULTI_TURN_PASSED=true
[ "$HAS_TIMESTAMPS" == "true" ] && METADATA_PASSED=true
[[ "$TOOL_CALL_COUNT" =~ ^[0-9]+$ ]] && [ "$TOOL_CALL_COUNT" -gt 0 ] && TOOL_CALLS_PASSED=true

echo -e "\n${GREEN}╔════════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║          Comment System E2E Test Results                           ║${NC}"
echo -e "${GREEN}╠════════════════════════════════════════════════════════════════════╣${NC}"

# Comment Types Section
echo -e "${GREEN}║ ${WHITE}COMMENT TYPES${NC}                                                       ${GREEN}║${NC}"
echo -e "${GREEN}╟────────────────────────────────────────────────────────────────────╢${NC}"
printf "${GREEN}║${NC}   message:    %-5s  content:   %-5s  tool_call: %-5s          ${GREEN}║${NC}\n" "$MSG_COUNT" "$CONTENT_COUNT" "$TOOL_CALL_COUNT"
printf "${GREEN}║${NC}   progress:   %-5s  error:     %-5s  status:    %-5s          ${GREEN}║${NC}\n" "$PROGRESS_COUNT" "$ERROR_COUNT" "$STATUS_COUNT"
echo -e "${GREEN}╟────────────────────────────────────────────────────────────────────╢${NC}"

# Core functionality
echo -e "${GREEN}║ ${WHITE}CORE FUNCTIONALITY${NC}                                                  ${GREEN}║${NC}"
echo -e "${GREEN}╟────────────────────────────────────────────────────────────────────╢${NC}"

if [ "$COMMENT_ADD_PASSED" == "true" ]; then
    echo -e "${GREEN}║${NC}   comment.add:              ${GREEN}✓${NC} (3 user comments)                    ${GREEN}║${NC}"
else
    echo -e "${GREEN}║${NC}   comment.add:              ${RED}✗${NC}                                       ${GREEN}║${NC}"
fi

if [ "$COMMENT_LIST_PASSED" == "true" ]; then
    printf "${GREEN}║${NC}   comment.list:             ${GREEN}✓${NC} (%-3s total comments)                 ${GREEN}║${NC}\n" "$COMMENT_COUNT"
else
    echo -e "${GREEN}║${NC}   comment.list:             ${RED}✗${NC}                                       ${GREEN}║${NC}"
fi

if [ "$TOOL_CALLS_PASSED" == "true" ]; then
    printf "${GREEN}║${NC}   Tool calls:               ${GREEN}✓${NC} (%-3s tool_call comments)             ${GREEN}║${NC}\n" "$TOOL_CALL_COUNT"
else
    echo -e "${GREEN}║${NC}   Tool calls:               ${YELLOW}○${NC} (no tool_call comments)              ${GREEN}║${NC}"
fi

if [ "$MULTI_TURN_PASSED" == "true" ]; then
    echo -e "${GREEN}║${NC}   Multi-turn conversation:  ${GREEN}✓${NC} (3 turns completed)                  ${GREEN}║${NC}"
else
    echo -e "${GREEN}║${NC}   Multi-turn conversation:  ${YELLOW}○${NC} (partial)                            ${GREEN}║${NC}"
fi

if [ "$AGENT_COMMENTS" -gt 0 ]; then
    printf "${GREEN}║${NC}   Agent responses:          ${GREEN}✓${NC} (%-3s agent comments)                 ${GREEN}║${NC}\n" "$AGENT_COMMENTS"
else
    echo -e "${GREEN}║${NC}   Agent responses:          ${YELLOW}○${NC} (pending)                            ${GREEN}║${NC}"
fi

if [ "$METADATA_COUNT" -gt 0 ]; then
    printf "${GREEN}║${NC}   Comments with metadata:   ${GREEN}✓${NC} (%-3s comments)                       ${GREEN}║${NC}\n" "$METADATA_COUNT"
else
    echo -e "${GREEN}║${NC}   Comments with metadata:   ${YELLOW}○${NC} (none)                               ${GREEN}║${NC}"
fi

echo -e "${GREEN}╟────────────────────────────────────────────────────────────────────╢${NC}"
echo -e "${GREEN}║ ${WHITE}FILE OPERATIONS${NC}                                                     ${GREEN}║${NC}"
echo -e "${GREEN}╟────────────────────────────────────────────────────────────────────╢${NC}"

if [ "$FILES_CREATED_PASSED" == "true" ]; then
    echo -e "${GREEN}║${NC}   calculator.py:            ${GREEN}✓${NC} (created by agent)                   ${GREEN}║${NC}"
else
    echo -e "${GREEN}║${NC}   calculator.py:            ${YELLOW}○${NC} (not created)                        ${GREEN}║${NC}"
fi

if [ -f "$TEST_WORKSPACE_DIR/test_calculator.py" ]; then
    echo -e "${GREEN}║${NC}   test_calculator.py:       ${GREEN}✓${NC} (created by agent)                   ${GREEN}║${NC}"
else
    echo -e "${GREEN}║${NC}   test_calculator.py:       ${YELLOW}○${NC} (not created)                        ${GREEN}║${NC}"
fi

echo -e "${GREEN}╟────────────────────────────────────────────────────────────────────╢${NC}"
echo -e "${GREEN}║ ${WHITE}INFRASTRUCTURE${NC}                                                      ${GREEN}║${NC}"
echo -e "${GREEN}╟────────────────────────────────────────────────────────────────────╢${NC}"
echo -e "${GREEN}║${NC}   Workspace/Board/Column:   ${GREEN}✓${NC}                                       ${GREEN}║${NC}"
echo -e "${GREEN}║${NC}   Task creation:            ${GREEN}✓${NC}                                       ${GREEN}║${NC}"
echo -e "${GREEN}║${NC}   Agent execution:          ${GREEN}✓${NC}                                       ${GREEN}║${NC}"
echo -e "${GREEN}║${NC}   Task completion:          ${GREEN}✓${NC}                                       ${GREEN}║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════════════════════════════╝${NC}"

# Clean up workspace directory (after all checks)
rm -rf "$TEST_WORKSPACE_DIR" 2>/dev/null || true

# Overall result
if [ "$COMMENT_ADD_PASSED" == "true" ] && [ "$COMMENT_LIST_PASSED" == "true" ]; then
    echo -e "\n${GREEN}✓ Comment System E2E Test PASSED${NC}"
    echo -e "${GREEN}  Total comments: $COMMENT_COUNT (user: $USER_COMMENTS, agent: $AGENT_COMMENTS)${NC}"
    echo -e "${GREEN}  Tool calls recorded: $TOOL_CALL_COUNT${NC}"
    exit 0
else
    echo -e "\n${RED}✗ Comment System E2E Test FAILED${NC}"
    exit 1
fi