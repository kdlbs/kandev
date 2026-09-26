---
id: "01-session-prompt-log-level"
title: "Downgrade benign prompt-completion log noise on shutdown"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/shutdown-log-noise.md"
parallelism: sequential
---

# Task 01: Session prompt log level

## Context

`internal/agent/runtime/lifecycle/session.go` logs two prompt sites at ERROR
with a stack trace:

- `dispatchInitialPrompt` "initial prompt failed" (~L835), fired from a
  detached goroutine during `InitializeAndPrompt`.
- `waitForPromptDone` "prompt completed with error" (~L919).

During graceful shutdown the agent subprocess is SIGTERM'd, so these fail with
an ACP transport death (`peer disconnected before response`, JSON-RPC `-32603`)
or `context.Canceled`. Both are benign teardown races.

The helper `isTransportDeadErr(err error) bool` (~L1598) already matches
"peer disconnected", "connection closed", "notification queue overflow",
`context.Canceled`, and `context.DeadlineExceeded`. `isCancelReleaseError(msg)`
(~L976) matches the cancel sentinels. `sm.stopCh` is available on the
`SessionManager`.

## Acceptance

- When the prompt error is a transport death / cancel-release sentinel, or
  `sm.stopCh` is already closed, both sites log at WARN with **no** stack trace
  (no `zap.Error` that emits a trace; log the error string only).
- A genuine agent-reported error that is not a transport death, on an active
  session (stopCh open), still logs at ERROR with its stack trace.
- No change to control flow, returned errors, signals, or state transitions.

## Verification

`cd apps/backend && go test ./internal/agent/runtime/lifecycle/...`

Add a focused regression test asserting WARN vs ERROR classification for a
transport-death error versus a plain agent error. Reuse the existing
`isTransportDeadErr` table-test pattern in `session_test.go`.

## Files Likely Touched

- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/session_test.go`

## Output Contract

Report the classification predicate used, the two sites changed, tests added
and run, and residual risks; mark this task done in this file and `plan.md`.
