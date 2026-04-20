#!/bin/bash
# Entrypoint script for Kandev Multi-Agent
# Runs agentctl HTTP server that manages agent subprocesses (auggie, codex, etc.)

set -e

echo "Starting Kandev Multi-Agent..."

# Validate agent CLIs are available
if ! command -v auggie &> /dev/null; then
    echo "WARNING: auggie CLI not found"
fi

if ! command -v codex &> /dev/null; then
    echo "WARNING: codex CLI not found"
fi

# Run prepare script if provided (e.g., clone repo, configure git)
if [ -n "$KANDEV_PREPARE_SCRIPT" ]; then
    echo "Running prepare script..."
    eval "$KANDEV_PREPARE_SCRIPT"
    echo "Prepare script completed."
fi

# Run agentctl HTTP server
echo "Starting agentctl on port ${AGENTCTL_PORT:-9999}..."
exec /usr/local/bin/agentctl

