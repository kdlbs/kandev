#!/bin/bash
# Augment Agent - Integrates with Auggie CLI (Augment Code)
# Outputs ACP (Agent Communication Protocol) messages to stdout

set -e

# Common variables for ACP messages
AGENT_ID="${KANDEV_INSTANCE_ID:-unknown}"
TASK_ID="${KANDEV_TASK_ID:-${TASK_ID:-unknown}}"

# Helper function to output ACP messages
acp_message() {
    local type="$1"
    local content="$2"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq -n -c \
        --arg type "$type" \
        --arg agentId "$AGENT_ID" \
        --arg taskId "$TASK_ID" \
        --arg timestamp "$timestamp" \
        --arg content "$content" \
        '{type: $type, agent_id: $agentId, task_id: $taskId, timestamp: $timestamp, data: {content: $content}}'
}

acp_progress() {
    local progress="$1"
    local message="$2"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq -n -c \
        --arg agentId "$AGENT_ID" \
        --arg taskId "$TASK_ID" \
        --arg timestamp "$timestamp" \
        --arg message "$message" \
        --argjson progress "$progress" \
        '{type: "progress", agent_id: $agentId, task_id: $taskId, timestamp: $timestamp, data: {progress: $progress, message: $message}}'
}

acp_complete() {
    local result="$1"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq -n -c \
        --arg agentId "$AGENT_ID" \
        --arg taskId "$TASK_ID" \
        --arg timestamp "$timestamp" \
        --arg result "$result" \
        '{type: "result", agent_id: $agentId, task_id: $taskId, timestamp: $timestamp, data: {status: "completed", summary: $result}}'
}

acp_error() {
    local error="$1"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq -n -c \
        --arg agentId "$AGENT_ID" \
        --arg taskId "$TASK_ID" \
        --arg timestamp "$timestamp" \
        --arg error "$error" \
        '{type: "error", agent_id: $agentId, task_id: $taskId, timestamp: $timestamp, data: {error: $error}}'
}

# Session info message to store session_id for resumption
acp_session_info() {
    local session_id="$1"
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    jq -n -c \
        --arg agentId "$AGENT_ID" \
        --arg taskId "$TASK_ID" \
        --arg timestamp "$timestamp" \
        --arg sessionId "$session_id" \
        '{type: "session_info", agent_id: $agentId, task_id: $taskId, timestamp: $timestamp, data: {session_id: $sessionId}}'
}

# Main agent logic
main() {
    acp_message "thinking" "Starting Auggie agent for task: ${TASK_TITLE:-No title}"
    acp_progress 5 "Initializing Auggie CLI..."

    # Check if auggie is available
    if ! command -v auggie &> /dev/null; then
        acp_error "Auggie CLI not found. Please ensure @augmentcode/auggie is installed."
        exit 1
    fi

    # Check for authentication
    # AUGMENT_SESSION_AUTH should contain the full session JSON (same format as ~/.augment/session.json)
    if [ -z "$AUGMENT_SESSION_AUTH" ]; then
        acp_error "AUGMENT_SESSION_AUTH environment variable is required for authentication. It should contain the full session JSON."
        exit 1
    fi

    # Export for auggie to use
    export AUGMENT_SESSION_AUTH

    # Check for task description
    if [ -z "$TASK_DESCRIPTION" ]; then
        acp_error "TASK_DESCRIPTION environment variable is required."
        exit 1
    fi

    # Check if we're resuming a session
    if [ -n "$AUGGIE_SESSION_ID" ]; then
        acp_progress 10 "Resuming Auggie session: $AUGGIE_SESSION_ID"
        acp_message "thinking" "Continuing previous session with: $TASK_DESCRIPTION"
    else
        acp_progress 10 "Starting new Auggie session..."
        acp_message "thinking" "Task: $TASK_DESCRIPTION"
    fi

    # Build auggie command
    # --print: One-shot mode, output and exit
    # --output-format json: Structured JSON output
    AUGGIE_CMD="auggie --print --output-format json"

    # Add workspace root if we're in a workspace
    if [ -d "/workspace" ] && [ "$(ls -A /workspace 2>/dev/null)" ]; then
        AUGGIE_CMD="$AUGGIE_CMD --workspace-root /workspace"
    fi

    # Resume session if session ID is provided
    if [ -n "$AUGGIE_SESSION_ID" ]; then
        AUGGIE_CMD="$AUGGIE_CMD --resume $AUGGIE_SESSION_ID"
    fi

    # Add the instruction
    AUGGIE_CMD="$AUGGIE_CMD --instruction \"$TASK_DESCRIPTION\""

    acp_progress 20 "Executing Auggie..."

    # Run auggie and capture output
    # The output will be JSON when using --output-format json
    set +e
    AUGGIE_OUTPUT=$(eval $AUGGIE_CMD 2>&1)
    AUGGIE_EXIT_CODE=$?
    set -e

    acp_progress 90 "Processing Auggie output..."

    # Check if auggie succeeded
    if [ $AUGGIE_EXIT_CODE -ne 0 ]; then
        acp_error "Auggie failed with exit code $AUGGIE_EXIT_CODE: $AUGGIE_OUTPUT"
        exit $AUGGIE_EXIT_CODE
    fi

    # Extract session_id from auggie output for future resumption
    local session_id=""
    if echo "$AUGGIE_OUTPUT" | jq -e '.session_id' >/dev/null 2>&1; then
        session_id=$(echo "$AUGGIE_OUTPUT" | jq -r '.session_id')
    fi

    # Output the session_id as a separate ACP message so the orchestrator can store it
    if [ -n "$session_id" ]; then
        acp_session_info "$session_id"
    fi

    # Output the auggie result as an ACP message
    local timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    # Try to parse as JSON, if it fails, treat as plain text
    if echo "$AUGGIE_OUTPUT" | jq -e . >/dev/null 2>&1; then
        # Valid JSON output from auggie - wrap in ACP format
        jq -n -c \
            --arg agentId "$AGENT_ID" \
            --arg taskId "$TASK_ID" \
            --arg timestamp "$timestamp" \
            --argjson output "$AUGGIE_OUTPUT" \
            '{type: "auggie_result", agent_id: $agentId, task_id: $taskId, timestamp: $timestamp, data: {output: $output}}'
    else
        # Plain text output
        jq -n -c \
            --arg agentId "$AGENT_ID" \
            --arg taskId "$TASK_ID" \
            --arg timestamp "$timestamp" \
            --arg output "$AUGGIE_OUTPUT" \
            '{type: "auggie_result", agent_id: $agentId, task_id: $taskId, timestamp: $timestamp, data: {output: $output}}'
    fi

    acp_progress 100 "Task completed"
    acp_complete "Auggie agent finished successfully"
}

# Run main and catch errors
main "$@" || acp_error "Agent failed: $?"

