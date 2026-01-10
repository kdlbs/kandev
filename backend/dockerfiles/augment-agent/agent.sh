#!/bin/bash
# Augment Agent - Runs in ACP (Agent Client Protocol) mode
# Uses JSON-RPC 2.0 over stdin/stdout for bidirectional communication

set -e

# Check if auggie is available
if ! command -v auggie &> /dev/null; then
    echo '{"jsonrpc":"2.0","method":"error","params":{"message":"Auggie CLI not found"}}' >&2
    exit 1
fi

# Check for authentication
if [ -z "$AUGMENT_SESSION_AUTH" ]; then
    echo '{"jsonrpc":"2.0","method":"error","params":{"message":"AUGMENT_SESSION_AUTH required"}}' >&2
    exit 1
fi

# Export for auggie to use
export AUGMENT_SESSION_AUTH

# Build auggie command
AUGGIE_ARGS="--acp"

# Add workspace root if we're in a workspace with files
if [ -d "/workspace" ] && [ "$(ls -A /workspace 2>/dev/null)" ]; then
    AUGGIE_ARGS="$AUGGIE_ARGS --workspace-root /workspace"
fi

# Execute auggie in ACP mode - stdin/stdout will be used for JSON-RPC communication
exec auggie $AUGGIE_ARGS
