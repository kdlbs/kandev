#!/bin/bash
# Test Codex CLI cancel functionality in isolation
# Uses Codex app-server protocol (JSON-RPC 2.0 variant - no jsonrpc field)
#
# Cancel Method: turn/interrupt (request)
# Payload: {"id": N, "method": "turn/interrupt", "params": {"threadId": "<id>", "turnId": "<id>"}}

set -e

WORKDIR="${1:-.}"
TMPDIR=$(mktemp -d)
FIFO="$TMPDIR/codex_fifo"
mkfifo "$FIFO"

cleanup() {
    rm -rf "$TMPDIR"
    kill $CODEX_PID 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Testing Codex (App-Server Protocol) Cancel ==="
echo "Working directory: $WORKDIR"
echo ""

# Start codex in app-server mode
codex app-server < "$FIFO" &
CODEX_PID=$!
exec 3>"$FIFO"

sleep 2

echo "1. Sending initialize request..."
# Note: Codex protocol omits the "jsonrpc" field
echo '{"id":1,"method":"initialize","params":{"clientInfo":{"name":"test","title":"Test","version":"1.0"}}}' >&3
sleep 2

echo ""
echo "2. Sending initialized notification..."
echo '{"method":"initialized"}' >&3
sleep 1

echo ""
echo "3. Starting thread..."
echo '{"id":2,"method":"thread/start","params":{"cwd":"'"$WORKDIR"'","approvalPolicy":"never"}}' >&3
sleep 2

echo ""
echo "4. Starting turn (this begins agent work)..."
# Note: threadId should come from thread/start response - using placeholder
echo '{"id":3,"method":"turn/start","params":{"threadId":"THREAD_ID","input":[{"type":"text","text":"Say hello"}]}}' >&3
sleep 3

echo ""
echo "5. Sending turn/interrupt request..."
# Note: Both threadId AND turnId are required for interrupt
echo '{"id":4,"method":"turn/interrupt","params":{"threadId":"THREAD_ID","turnId":"TURN_ID"}}' >&3
sleep 2

echo ""
echo "=== Test complete ==="
echo ""
echo "Key observations:"
echo "- turn/interrupt is a REQUEST (has id field, expects response)"  
echo "- Requires BOTH threadId AND turnId"
echo "- Codex protocol omits the 'jsonrpc' field from all messages"
echo "- Thread = Session, Turn = Operation"

