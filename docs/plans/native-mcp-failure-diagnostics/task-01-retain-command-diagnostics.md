---
id: "01-retain-command-diagnostics"
title: "Retain safe native command diagnostics"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-002
  - REQ-AGENTS-MCP-PREP-003
  - REQ-AGENTS-MCP-PREP-004
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-002.4
  - AC-AGENTS-MCP-PREP-003.3
  - AC-AGENTS-MCP-PREP-004.1
  - AC-AGENTS-MCP-PREP-004.2
  - AC-AGENTS-MCP-PREP-004.3
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 01: Retain safe native command diagnostics

## Summary

Preserve the original native runner cause and failure stage through the adapter,
launch/retry preparation, saved session results and structured backend logs.
Publish the additive safe diagnostic and recovery error-code fields for Task 02.

## In scope

- Typed wrapping that preserves `errors.Is`/`errors.As`, observed exit code,
  primary wait error and secondary cleanup error. Keep existing process cleanup.
- `NativeMCPDiagnostic` sanitization, bounded messages and closed fields per design.
  Use fixed explanations for nonzero/unrecognized output; never retain streams.
- Optional `mcp_diagnostic` in preparation progress/completion, serialization and
  retry response; one correlated safe failure log per failed command.
- Add `error_code` alongside existing `error` in Cursor recovery error responses.
- New focused regression files and existing launch/recovery/handler regressions.

## Out of scope

Frontend rendering, new readiness codes, altered timeouts, new command execution
paths, raw output retention, database migration and automatic retries.

## Acceptance

1. `TestNativeMCPDiagnosticRetainsWaitDelay` fails before implementation and passes
   afterward. Cover resolution/start, timeout/cancellation, nonzero exit,
   truncation and unknown verification output. Prove primary plus cleanup errors
   remain distinct; test the real Unix inherited-pipe runner with owned cleanup.
2. `TestNativeMCPDiagnosticRedactsAndBoundsCauses` proves credential, private
   endpoint/path and auth URL redaction, control/ANSI stripping and UTF-8 caps.
   No raw stdout/stderr appears in returned metadata, events, saved results or
   captured logs. Injected runners receive the same final normalization.
3. `TestCursorMCPDiagnosticSurvivesPreparation` proves launch/retry event and
   saved-result parity, absent diagnostics on success/legacy records, skipped-row
   accuracy and stale-attempt rejection. Handler tests prove busy responses
   expose `error_code`, retain `error`, and do not invoke native recovery.

## Verification

Run from repository root; retain every long-running command handle.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/mcpconfig -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Test.*(Cursor.*MCP|Cursor.*Mcp|MCPDiagnostic|NativeMCP|SerializePrepareResult|Prepare)' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/handlers -run 'Test.*CursorMCP' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/cursor_native_mcp.go`,
  `cursor_native_mcp_resolve.go`, process guard files and new
  `cursor_native_mcp_diagnostic.go` / `cursor_native_mcp_diagnostic_test.go`.
- `apps/backend/internal/agent/mcpconfig/cursor_native_mcp_unix_test.go`.
- `apps/backend/internal/agent/runtime/lifecycle/cursor_plugin_mcp.go`,
  `cursor_mcp_recovery.go`, `cursor_mcp_recovery_types.go`, `env_preparer.go`,
  `event_types.go`, `manager_launch.go`, new `cursor_mcp_diagnostic_test.go`.
- Existing `env_preparer_test.go`, `manager_launch_prepare_events_test.go`,
  `cursor_mcp_recovery_test.go`, `cursor_plugin_mcp_test.go` where size permits.
- `apps/backend/internal/task/handlers/cursor_mcp_recovery_handlers.go` and tests.

## Dependencies

None. Read the existing pure `routingerr.Sanitize` contract; leave its general
provider behavior unchanged. No worker delegation is authorized.

## Risks

Preserve executable-unavailable sentinel matching without losing its original
cause. Preserve actual exit status when cleanup/wait fails. Revalidate safe
metadata before serialization; never log `err` or native streams directly.

## Parallelism

`sequential`

## Inputs

- REQ-AGENTS-MCP-PREP-004 and existing AC-002.4/003.3.
- Paired design's retained-diagnostics, lifecycle/progress and recovery sections.
- Existing fake native runner, Unix owned-process tests, event recorder and
  serialization tests; the incident record in [plan.md](plan.md).

## Results

Implemented the bounded, sanitized diagnostic contract across the native
runner, preparation lifecycle, persisted session metadata, progress events,
retry responses and structured logs. Recovery errors retain `error` and now
also expose `error_code`. Generation fencing prevents older approval and
verification failures from replacing diagnostics from the current attempt.

Validation passed on 2026-10-07:

- `(cd apps/backend && go test -trimpath -race ./internal/agent/mcpconfig -count=1)`
- `(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'Test.*(Cursor.*MCP|Cursor.*Mcp|MCPDiagnostic|NativeMCP|SerializePrepareResult|Prepare)' -count=1)`
- `(cd apps/backend && go test -trimpath -race ./internal/task/handlers -run 'Test.*CursorMCP' -count=1)`
- `git diff --check`

Review follow-up on 2026-10-07:

- If a preparation context ends while a native command is returning, its
  sanitized diagnostic is retained on the failed approval or verification step
  and in retry responses. Diagnostics from stale generations remain suppressed.
- A nonzero native exit with a cleanup error keeps the exit status and cleanup
  detail while allowing authentication and approval output to be classified.
- Lifecycle tests cover cancellation during initial approval, initial
  verification, retry approval and retry verification. A focused regression
  also checks deadline-ended contexts; the existing stale-generation test
  confirms that stale command details stay hidden.
- The Unix command-result test covers authentication and approval classification
  with cleanup errors and verifies that the cleanup message is sanitized.
- Focused race tests passed for `internal/agent/mcpconfig`,
  `internal/agent/runtime/lifecycle`, and `internal/task/handlers`.
