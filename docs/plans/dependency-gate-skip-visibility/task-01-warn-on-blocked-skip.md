---
id: "01-warn-on-blocked-skip"
title: "Warn on blocked auto-start skip"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001
acceptance_criteria:
  - AC-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001.1
  - AC-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001.2
system_design:
  - ../../specs/tasks/system-design/dependency-gate-skip-visibility.md
---

# Task 01: Warn on Blocked Auto-Start Skip

## Summary

The dependency gate's genuine blocked verdict is logged at WARN instead of
Debug, so an operator running at the default INFO level sees why an automated
launch did not happen.

## In scope

- The one-word level change on the `isBlocked` branch of
  `dependencyBlocksAutoStart`.
- The regression test asserting the WARN entry and its structured fields.

## Out of scope

- Gate semantics, fail-closed reads, launch-token handling, and any UI change.
- The lookup-error path, which already logs at WARN.

## Acceptance

- A blocked task evaluated by any automated launch path emits one WARN entry
  whose message names the triggering event and whose fields carry `task_id`
  and `blocked_reason`.
- A task that passes the gate emits no blocked-skip WARN entry.

## Implementation sequence

1. Add `TestDependencyBlocksAutoStartWarnsOnBlockedSkip` against the observed
   logger and confirm it fails against the Debug logging.
2. Change `s.logger.Debug` to `s.logger.Warn` in the `isBlocked` branch and
   confirm the test passes.
3. Run the package suite and the documentation validators.

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

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_dependencies.go`
- `apps/backend/internal/orchestrator/event_handlers_dependencies_test.go`

## Dependencies

None.

## Risks

None beyond the bounded per-evaluation log volume noted in the plan.

## Parallelism

`sequential`

## Inputs

- Requirement acceptance criteria in
  `docs/specs/tasks/requirements/dependency-gate-skip-visibility.md`.
- Log contract in
  `docs/specs/tasks/system-design/dependency-gate-skip-visibility.md`.
- Gate call sites in `event_handlers_workflow.go` and `session_launch.go`.

## Results

Implemented the level change and the regression test. The observed-logger test
filters for the blocked-skip message at exactly `WarnLevel`, asserts a single
entry, and checks the `task_id` and `blocked_reason` fields; it was verified
red against the pre-fix Debug logging before the fix. This test covers the
blocked path. The unblocked and lookup-error paths are not asserted in this
file.

- `go test ./internal/orchestrator -run 'TestDependency|TestResolution' -count=1`:
  passed.
- `go test ./internal/orchestrator -count=1`: passed.
- `gofmt -l internal/orchestrator`: empty.
- `go vet ./internal/orchestrator`: clean.
- `python3 scripts/list-docs.py validate`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
