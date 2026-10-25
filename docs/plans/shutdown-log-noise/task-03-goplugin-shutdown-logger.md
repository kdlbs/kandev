---
id: "03-goplugin-shutdown-logger"
title: "Silence go-plugin ERROR on deliberate shutdown kill"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/shutdown-log-noise.md"
parallelism: sequential
---

# Task 03: go-plugin shutdown logger

## Context

`internal/plugins/runtime/manager.go` constructs the Hashicorp go-plugin client
via `hcplugin.NewClient` (~L394) with **no** `Logger` field. go-plugin then
uses its own default hclog logger, which prints the subprocess exit at ERROR:

```
[ERROR] plugin: plugin process exited: ... error="signal: terminated"
```

This fires when `client.Kill()` runs during the manager's shutdown/`StopAll`
path. The kill is deliberate, so the ERROR is misleading.

`internal/plugins/runtime/process.go` (~L232) separately logs "plugin process
exited unexpectedly" at WARN with a `plugin_id` field.

## Acceptance

- Supply an `hclog.Logger` to `hcplugin.NewClient` routed through Kandev's
  logger (adapter), OR silence the go-plugin logger for the deliberate kill,
  so an expected `signal: terminated` / `signal: killed` on shutdown is NOT
  emitted at ERROR.
- An unexpected plugin crash while the manager is NOT shutting down still
  surfaces (the existing WARN in `process.go` remains, and go-plugin output for
  genuine crashes is not fully muted).
- No change to plugin lifecycle, restart policy, or manager control flow.

## Verification

`cd apps/backend && go test ./internal/plugins/...`

Add a focused regression test asserting that a shutdown-initiated kill does not
produce an ERROR-level log line, while an unexpected exit still logs.

## Files Likely Touched

- `apps/backend/internal/plugins/runtime/manager.go`
- `apps/backend/internal/plugins/runtime/process.go`
- corresponding `_test.go`

## Output Contract

Report the logger-injection approach (adapter vs null-on-kill), how shutdown is
detected, tests added and run, and residual risks; mark this task done in this
file and `plan.md`.
