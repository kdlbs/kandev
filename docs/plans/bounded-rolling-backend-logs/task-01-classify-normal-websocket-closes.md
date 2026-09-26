---
id: "01-classify-normal-websocket-closes"
title: "Classify normal WebSocket closes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/diagnostic-logging.md"
---

# Task 01: Classify Normal WebSocket Closes

## Intent

Stop error-level stack traces for WebSocket close code `1000`. Keep the
existing diagnostic signal for unexpected close codes.

## Acceptance

- `Client.ReadPump` treats `websocket.CloseNormalClosure` as an expected close.
- A real code-`1000` close produces no `WebSocket read error` entry.
- An unexpected close still produces one error entry with a stack trace.

## Files Likely Touched

- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/client_pump_close_test.go`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 02. This task owns the two gateway files. Task 02
owns only logger-package files. Execute sequentially unless the user authorizes
delegation.

## Inputs

- The WebSocket close requirements in the diagnostic-logging specification.
- The confirmed root cause in `plan.md`.
- The real WebSocket fixture helpers in the websocket test package.
- The observed-log pattern in `terminal_wsutil_test.go`.

## Implementation

1. Add a focused integration test for normal and unexpected close codes.
2. Run the test and confirm that the normal-close case fails.
3. Add `websocket.CloseNormalClosure` to the expected close-code list.
4. Run the focused test again and refactor only if clarity improves.

## Verification

```bash
cd apps/backend && go test ./internal/gateway/websocket -run TestReadPumpCloseLogging -count=1
```

## Output Contract

Report the changed files, the exact test result, blockers, and remaining risks.
Update this task and `plan.md` in the same conversation.

## Results

- Changed `apps/backend/internal/gateway/websocket/client.go` to treat close
  code `1000` as an expected lifecycle close.
- Added `client_pump_close_test.go`, which drives real WebSocket close frames
  and checks both normal and unexpected logging behavior.
- Verification: `cd apps/backend && go test ./internal/gateway/websocket
  -run TestReadPumpCloseLogging -count=1` passed.
- Blockers: none.
- Remaining risk: the writer and bundle tasks still need to preserve later
  diagnostics under sustained log volume.
