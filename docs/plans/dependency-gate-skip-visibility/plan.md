---
created: 2026-09-16
status: implemented
requirements:
  - REQ-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001
system_design:
  - ../../specs/tasks/system-design/dependency-gate-skip-visibility.md
legacy_specs: []
---

# Implementation Plan: Dependency Gate Skip Visibility

## Overview

The dependency gate correctly skips automated launches for tasks with
unresolved dependencies, but the blocked verdict was logged at Debug, which is
invisible at the default INFO level. Operators saw tasks sit on an auto-start
step (`on_enter: auto_start_agent`) with no launch, no error, and no log line,
which is indistinguishable from a broken launcher. One work order raises the
blocked-skip log to WARN and proves it with a regression test.

## Evidence and root cause

Issue #3720 records a recovery session where nine kanban tasks sat on an
auto-start step with zero sessions and zero error output. Every task had
pending `task_blockers` edges and the gate was correctly skipping the launch,
silently. The lookup-error path in `dependencyBlocksAutoStart` already logs at
WARN; only the genuine blocked verdict used Debug. Root cause is at
`apps/backend/internal/orchestrator/event_handlers_dependencies.go`: the
`isBlocked` branch's `s.logger.Debug` call.

## Scope

### In scope

- One log-level change on the blocked verdict: Debug to WARN.
- Keeping the existing `task_id` and `blocked_reason` fields and the
  event-prefixed message unchanged.
- A regression test asserting the WARN entry for a genuine block.

### Out of scope

- Gate semantics, fail-closed reads, and launch-token handling.
- Board or dashboard indicators for dependency-blocked tasks.
- Log-level changes on any other path.

## Technical approach

Change `s.logger.Debug` to `s.logger.Warn` in the `isBlocked` branch of
`dependencyBlocksAutoStart`. The log contract is specified by
[Dependency Gate Skip Visibility](../../specs/tasks/system-design/dependency-gate-skip-visibility.md):
the message keeps the caller's event-name prefix, and the structured fields
keep `task_id` and `blocked_reason`. No behavior change accompanies the level.

## Tests

- `TestDependencyBlocksAutoStartWarnsOnBlockedSkip` in
  `apps/backend/internal/orchestrator/event_handlers_dependencies_test.go`
  filters the observed zap log for the blocked-skip message at exactly
  `WarnLevel` and asserts one entry whose `task_id` and `blocked_reason` fields
  match the blocked reader (REQ-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001.1).
- The unblocked and lookup-error paths are not asserted in this file.

## Work orders

- [x] [Task 01: Warn on blocked auto-start skip](task-01-warn-on-blocked-skip.md)

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test ./internal/orchestrator -run 'TestDependency|TestResolution' -count=1)
(cd apps/backend && gofmt -l internal/orchestrator)
(cd apps/backend && go vet ./internal/orchestrator)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Implementation and verification completed. The blocked verdict now logs at
WARN with the event-prefixed message, `task_id`, and `blocked_reason`; the
lookup-error path and its WARN entry are unchanged.

- Focused gate tests: passed.
- Full `internal/orchestrator` package tests: passed.
- `gofmt` and `go vet` on the package: clean.
- Specification catalog validation and specification lint: passed.

## Risks

- A blocked task that sits on an auto-start step for a long time logs one WARN
  per gate evaluation. Evaluations happen on discrete events, not a poll, so
  the volume is bounded by board activity.

## Documentation impact

Backend log level only. Public documentation is unchanged.
