---
created: 2026-08-29
status: done
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
legacy_specs: []
---

# Implementation Plan: Preserve Last Activity During Passive Resume

## Overview

Exclude synthetic lifecycle turns from task activity. Apply the same rule to
live projection and restart reconstruction so both paths stay equivalent.

## Scope

### In scope

- Preserve `last_activity_at` when task opening resumes an idle agent.
- Exclude turns marked `lifecycle_only` from live activity projection.
- Exclude the same turns from SQLite and PostgreSQL activity reconstruction.
- Add focused regression tests for both activity paths.

### Out of scope

- Change agent resume, task focus, or session subscription behavior.
- Remove `agent_boot` messages or their synthetic parent turns.
- Change activity for task mutations, user prompts, or conversational turns.
- Change sidebar sorting, labels, or saved-view settings.

## Technical approach

### Live projection

Update `applyTaskActivityEventLocked` in
`apps/backend/internal/task/statussummary/projector_events.go`. Ignore
`turn.started` and `turn.completed` events whose metadata sets
`lifecycle_only`.

Use the existing marker from `models.TurnMetaKeyLifecycleOnly`. Add projector
tests that compare lifecycle turns with conversational turns.

### Durable reconstruction

Update `LoadTaskLastActivity` in
`apps/backend/internal/task/repository/sqlite/task_status_summary.go`. Filter
both turn branches with the existing dialect-aware lifecycle predicate.

Use `turnLifecycleOnlyPredicate` so SQLite and PostgreSQL use the same marker
semantics. Extend both activity-batch tests with a newer lifecycle turn. The
expected result must remain the latest qualifying activity.

## Tests

- `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.10`: the projector test proves
  that lifecycle-turn events do not change `last_activity_at`.
- `AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.10`: the SQLite test proves
  that restart reconstruction ignores a newer lifecycle turn.
- The PostgreSQL test uses the same scenario when its test database exists.

## Work orders

- [x] [Task 01: Exclude lifecycle turns from activity](task-01-exclude-lifecycle-turns.md)

## Verification results

- RED: `cd apps/backend && go test ./internal/task/statussummary -run
  TestProjectorLastActivityIgnoresLifecycleTurns -count=1` failed because a
  lifecycle-only completion advanced projected activity.
- RED: `cd apps/backend && go test ./internal/task/repository/sqlite -run
  'TestTaskLastActivityBatch|TestPostgresTaskLastActivityBatch' -count=1`
  failed because the SQLite batch included the newer lifecycle turn.
- GREEN: `cd apps/backend && go test ./internal/task/statussummary -run
  'LastActivity.*Lifecycle|Lifecycle.*LastActivity'` passed.
- GREEN: `cd apps/backend && go test ./internal/task/repository/sqlite -run
  'TaskLastActivity'` passed.
- GREEN race checks passed for both targeted commands.
- Full affected packages passed: `statussummary` (80 tests) and `sqlite` (814
  tests).
- `gofmt -l` reported no changed Go files, and `git diff --check` passed.

## Risks

- Live and reconstructed activity can diverge if only one path applies the
  lifecycle marker.
- SQLite and PostgreSQL decode JSON booleans differently. The shared predicate
  must retain the existing tolerant marker semantics.
- Conversational turns without the marker must keep their current activity
  behavior.
