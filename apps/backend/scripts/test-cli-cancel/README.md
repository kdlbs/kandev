# CLI Cancel/Stop Turn Testing

This directory contains scripts to test how turns can be stopped in isolation for both:
1. **Auggie CLI** (Augment) - uses ACP protocol
2. **Codex CLI** (OpenAI) - uses Codex app-server protocol

## Protocol Summary

### Auggie (ACP Protocol)

- **Transport**: JSON-RPC 2.0 over stdin/stdout
- **Cancel Method**: `session/cancel` (notification - no `id` field)
- **Payload**:
  ```json
  {"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"<session-id>"}}
  ```
- **Result**: The pending prompt request returns with `{"stopReason":"cancelled"}`
- **Notes**:
  - This is a one-way notification (no response expected)
  - Only requires `sessionId`
  - Agent immediately stops current operation

### Codex (App-Server Protocol)

- **Transport**: JSON-RPC 2.0 variant over stdin/stdout (omits `jsonrpc` field)
- **Cancel Method**: `turn/interrupt` (request - has `id` field)
- **Payload**:
  ```json
  {"id":N,"method":"turn/interrupt","params":{"threadId":"<thread-id>","turnId":"<turn-id>"}}
  ```
- **Result**: Returns a response with `id` matching the request
- **Notes**:
  - Requires BOTH `threadId` AND `turnId`
  - Thread ID comes from `thread/start` response
  - Turn ID comes from `turn/start` response (can be "0" for first turn)
  - Terminology: Thread = Session, Turn = Operation

## Test Results (Verified)

### Auggie Cancel Flow
```
1. initialize → Get agent capabilities
2. session/new → Get sessionId
3. session/prompt → Start a turn (blocking)
4. session/cancel → Stop the turn (notification)
5. prompt returns → {"stopReason":"cancelled"}
```

### Codex Cancel Flow
```
1. initialize → Get agent info
2. initialized → Acknowledge (notification)
3. thread/start → Get threadId
4. turn/start → Get turnId, starts work
5. turn/interrupt → Stop the turn (request)
```

## Usage

```bash
# Interactive test (recommended)
go run interactive_cancel.go -agent=auggie -workdir=/tmp
go run interactive_cancel.go -agent=codex -workdir=/tmp

# Shell script tests
./test_auggie_cancel.sh /tmp
./test_codex_cancel.sh /tmp

# Basic test harness
go run test_cancel_harness.go -agent=auggie
go run test_cancel_harness.go -agent=codex
```

## Requirements

- `auggie` CLI installed (`npm install -g @augmentcode/auggie`)
- `codex` CLI installed (`npm install -g @openai/codex`)
- Valid authentication for both CLIs

