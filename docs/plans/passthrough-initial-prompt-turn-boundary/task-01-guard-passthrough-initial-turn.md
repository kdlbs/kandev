---
id: "01-guard-passthrough-initial-turn"
title: "Guard the passthrough initial turn"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PASSTHROUGH-INITIAL-TURN-001
acceptance_criteria:
  - AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.1
  - AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.2
  - AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.3
  - AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.4
  - AC-TASKS-PASSTHROUGH-INITIAL-TURN-001.5
system_design:
  - ../../specs/tasks/system-design/passthrough-initial-prompt-turn-boundary.md
---

# Task 01: Guard the Passthrough Initial Turn

## Summary

Keep fresh passthrough executions running while Kandev waits to inject their
initial prompt. Bind the pending boundary to one PTY process so successful and
failed delivery, resume fallback, and process replacement remain deterministic.

## In scope

- Add process-scoped initial-prompt state to `AgentExecution`.
- Install it for original fresh launches and fresh-start resume fallbacks that
  require stdin prompt delivery.
- Suppress matching completion callbacks until injection finishes or aborts.
- Clear only the captured process's marker on every injection exit path.
- Add focused lifecycle launch, completion, injection, failure, and replacement
  regressions.

## Out of scope

- Agentctl detector changes and mid-turn idle detection.
- Orchestrator, workflow-engine, database, API, or frontend changes.
- Prompt formatting, submit sequences, and idle timeout tuning.

## Acceptance

- A startup completion signal for a process with pending initial injection
  emits no `agent.ready` event and leaves the execution running.
- After injection clears the matching marker, one eligible signal uses the
  existing Ready path.
- Abort and replacement cleanup cannot affect a different process.
- Prompt-flag, promptless, and resume paths never acquire the marker, while a
  fresh-start fallback does.

## Verification

```bash
go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(HandlePassthroughTurnComplete|AutoInject|Passthrough.*Fallback)' -count=1
go test -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(HandlePassthroughTurnComplete|AutoInject|Passthrough.*Fallback)' -count=1
go test -tags fts5 ./internal/agent/runtime/lifecycle -count=1
```

Run all commands from `apps/backend`.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_autoinject_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_launch_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_runtime_test.go`

## Dependencies

None.

## Risks

- Process lifecycle fields currently have mixed locking discipline. New marker
  reads and writes must use the existing passthrough lifecycle lock without
  holding it across event publication or PTY I/O.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/passthrough-initial-prompt-turn-boundary.md`
- `docs/specs/tasks/system-design/passthrough-initial-prompt-turn-boundary.md`
- `docs/decisions/2026-09-01-passthrough-initial-prompt-turn-boundary.md`
- `docs/specs/cli/requirements/cli-mode-parity.md`
- GitHub issue #3247 and the deterministic failing reproduction recorded in
  Kandev task `28542e6c-c81b-4ba2-be6b-f46ee03b337d`.

## Results

- Added an in-memory marker that binds pending initial prompt delivery to one
  passthrough process.
- Fresh launches and fresh-start fallbacks install the marker before later
  startup work can publish a completion signal.
- Completion callbacks remain inactive until prompt injection finishes or
  stops. Prompt-flag and resumed processes keep their existing behavior.
- Cleanup and PTY writes compare process identity. Work from an old process
  cannot clear or write to its replacement.
- `go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(HandlePassthroughTurnComplete|AutoInject|Passthrough.*Fallback)' -count=1`
  passed 16 tests.
- `go test -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(HandlePassthroughTurnComplete|AutoInject|Passthrough.*Fallback)' -count=1`
  passed the same 16 tests.
- `go test -tags fts5 ./internal/agent/runtime/lifecycle -count=1` passed
  2,100 tests.
