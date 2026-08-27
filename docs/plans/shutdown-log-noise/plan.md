---
spec: docs/specs/platform/requirements/shutdown-log-noise.md
created: 2026-08-11
status: done
---

# Implementation Plan: Quiet benign teardown log noise on shutdown

## Overview
Downgrade benign shutdown teardown log lines from ERROR (with stack traces) to
WARN, and stop the go-plugin library from logging an expected `signal:
terminated` exit as ERROR. Logging-severity only: no control-flow, state, or
contract changes. Each task reuses existing teardown-classification helpers and
adds a focused regression test.

## Confirmed root cause
Ctrl+C runs `runGracefulShutdown` (`internal/backendapp/helpers.go`), which
cancels the root context and SIGTERMs the plugin and agent subprocesses. In-flight
operations then fail with `context.Canceled` / `peer disconnected before
response` / `ErrSessionTerminal`. Most of the codebase already classifies these
as WARN teardown races (`executor.handleAgentProcessStartFailure`, the ACP
`session/load` path), but three sites do not participate:
- `session.go` prompt-completion / initial-prompt logging (ERROR).
- `orchestrator/handlers.go` `wsLaunchSession` (ERROR, unconditional).
- go-plugin's own default hclog logger (ERROR on kill), because
  `hcplugin.NewClient` is constructed without a `Logger`.

## Key subtlety (must be handled)
For `intent=resume`, the underlying `context.Canceled` sentinel is destroyed by
`task_operations.go:2229` `fmt.Errorf("session failed: %s", errMsg)` (`%s`, not
`%w`). So `errors.Is(err, context.Canceled)` is FALSE on that path. For
`intent=restore_workspace`, `manager_launch.go:1397` wraps `ErrSessionTerminal`
with `%w`, so `errors.Is(err, lifecycle.ErrSessionTerminal)` is TRUE. The
orchestrator classifier must handle both: `errors.Is` for the wrapped sentinels
AND a bounded string fallback (`context canceled`, `session is terminal`) for
the stringified-resume case. Prefer, where safe, converting `%s` to `%w` at the
lossy layer so the sentinel survives — but keep the string fallback because the
resume error may originate from a persisted session error string, not an error
value.

## Task waves
- **Wave 1 (parallel-safe, disjoint packages):** task-01, task-02, task-03,
  task-04. Each touches a different package with no shared schema, migration,
  generated contract, lockfile, or package config.

Each task keeps `parallelism: sequential`. Waves are a review aid only.

## Tasks
- `task-01-session-prompt-log-level.md` — `internal/agent/runtime/lifecycle`
- `task-02-orchestrator-launch-log-level.md` — `internal/orchestrator`
- `task-03-goplugin-shutdown-logger.md` — `internal/plugins/runtime`
- `task-04-warn-site-review.md` — `internal/agent/hostutility` (+ documented no-ops)

## Global validation
From `apps/backend`:
```bash
make -C apps/backend test
make -C apps/backend lint
golangci-lint run ./... --new-from-rev="<base-sha>" --timeout=5m
```
