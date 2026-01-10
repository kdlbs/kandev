#!/bin/bash
# Entrypoint script for Augment Agent
# Supports two modes:
# - "agentctl": Runs agentctl HTTP server that manages auggie subprocess
# - "direct": Legacy mode that runs auggie directly (stdin/stdout)

set -e

MODE="${AGENTCTL_MODE:-agentctl}"

echo "Starting Augment Agent in ${MODE} mode..."

# Common validation
if ! command -v auggie &> /dev/null; then
    echo "ERROR: auggie CLI not found. Is @augmentcode/auggie installed?"
    exit 1
fi

if [ -z "$AUGMENT_SESSION_AUTH" ]; then
    echo "ERROR: AUGMENT_SESSION_AUTH environment variable not set"
    exit 1
fi

case "$MODE" in
    "agentctl")
        # Run agentctl HTTP server
        echo "Starting agentctl on port ${AGENTCTL_PORT:-9999}..."
        exec /usr/local/bin/agentctl
        ;;
    "direct")
        # Legacy mode: run auggie directly
        echo "Running auggie directly in ACP mode..."
        exec auggie --acp --workspace-root /workspace
        ;;
    *)
        echo "ERROR: Unknown AGENTCTL_MODE: $MODE"
        echo "Valid modes: agentctl, direct"
        exit 1
        ;;
esac

