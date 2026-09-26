---
id: "02-orchestrator-launch-log-level"
title: "Downgrade benign session-launch log noise on shutdown"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/shutdown-log-noise.md"
parallelism: sequential
---

# Task 02: Orchestrator launch log level

## Context

`internal/orchestrator/handlers/handlers.go` `wsLaunchSession` (~L91) logs
"failed to launch session" at ERROR unconditionally. During shutdown two
benign failures reach this site:

- `intent=resume`: the underlying `context.Canceled` sentinel is **destroyed**
  at `task_operations.go:2229` (`fmt.Errorf("session failed: %s", errMsg)` uses
  `%s`, not `%w`), so the error surfaces as a string containing
  `context canceled`.
- `intent=restore_workspace`: `manager_launch.go:1397` wraps
  `lifecycle.ErrSessionTerminal` with `%w`, so `errors.Is` matches.

## Acceptance

- `wsLaunchSession` logs WARN (no stack trace) when the launch failed for a
  benign teardown reason. The classifier MUST handle both:
  - `errors.Is(err, lifecycle.ErrSessionTerminal)` and
    `errors.Is(err, context.Canceled)` (wrapped-sentinel cases), AND
  - a bounded string fallback for `context canceled` (the stringified-resume
    case where the sentinel is lost).
- Where safe, convert the lossy `%s` at `task_operations.go:2229` to `%w` so the
  sentinel survives, but keep the string fallback because the resume error may
  originate from a persisted session error string rather than an error value.
- A launch failure for a non-teardown reason (unknown task, validation) still
  logs at ERROR.
- The WS response returned to the caller is unchanged.

## Verification

`cd apps/backend && go test ./internal/orchestrator/...`

Add a focused regression test asserting WARN vs ERROR classification for a
`context.Canceled`-string resume error, a wrapped `ErrSessionTerminal` error,
and a plain non-teardown error.

## Files Likely Touched

- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`
- `apps/backend/internal/orchestrator/task_operations.go` (optional `%s`->`%w`)

## Output Contract

Report the classifier predicate, whether the `%s`->`%w` change was made and
why, tests added and run, and residual risks; mark this task done in this file
and `plan.md`.
