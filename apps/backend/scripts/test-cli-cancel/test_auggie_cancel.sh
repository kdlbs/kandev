#!/bin/bash
# Test Auggie CLI cancel functionality in isolation
# Uses ACP protocol (JSON-RPC 2.0 over stdin/stdout)
#
# Cancel Method: session/cancel (notification)
# Payload: {"jsonrpc": "2.0", "method": "session/cancel", "params": {"sessionId": "<id>"}}

set -e

WORKDIR="${1:-.}"
TMPDIR=$(mktemp -d)
FIFO="$TMPDIR/auggie_fifo"
mkfifo "$FIFO"

cleanup() {
    rm -rf "$TMPDIR"
    kill $AUGGIE_PID 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Testing Auggie (ACP Protocol) Cancel ==="
echo "Working directory: $WORKDIR"
echo ""

# Start auggie in ACP mode
auggie --acp --workspace-root "$WORKDIR" < "$FIFO" &
AUGGIE_PID=$!
exec 3>"$FIFO"

sleep 2

echo "1. Sending initialize request..."
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":1,"clientInfo":{"name":"test","version":"1.0"}}}' >&3
sleep 2

echo ""
echo "2. Creating new session..."
echo '{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"'"$WORKDIR"'","mcpServers":[]}}' >&3
sleep 2

echo ""
echo "3. Sending prompt (this will start a turn)..."
# Note: sessionId should come from session/new response - using placeholder
echo '{"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{"sessionId":"SESSION_ID","prompt":[{"type":"text","text":"Say hello"}]}}' >&3
sleep 3

echo ""
echo "4. Sending session/cancel notification..."
echo '{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"SESSION_ID"}}' >&3
sleep 2

echo ""
echo "=== Test complete ==="
echo ""
echo "Key observations:"
echo "- session/cancel is a NOTIFICATION (no id field, no response expected)"
echo "- Payload only requires sessionId"
echo "- Agent should stop the current turn when received"

