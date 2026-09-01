---
id: "02-scope-snapshot-capture"
title: "Scope snapshot capture"
status: completed
wave: 2
depends_on:
  - "01-migrate-snapshot-ownership"
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9
system_design:
  - ../../specs/tasks/system-design/environment-owned-git-status.md
---

# Task 02: Scope Snapshot Capture

## Summary

Resolve every live, completion, and archive capture to a task environment. Make
live upsert and completion supersession atomic in that environment scope.

## In scope

- Add task environment identity to lifecycle Git status payloads.
- Resolve a missing recovered-execution identity from the session row.
- Set environment identity on live, completion, and archive snapshots.
- Key the live-write throttle by environment and repository.
- Keep one live row for each environment and repository.
- Remove earlier live and completion rows during a completion insert.
- Add sibling-session and concurrent-write regressions.
- Update repository interfaces and test doubles.

## Out of scope

- Boot hydration selection.
- Task-summary reads.
- Frontend Git-status handling.
- Environment-scoping of commit-created events.

## Acceptance

- Two sibling sessions cannot keep separate live rows for one repository.
- A completion removes earlier live and completion rows from all siblings.
- Capture stops when it cannot resolve a non-empty environment ID.
- The session ID remains on the inserted row as provenance.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'Test.*Git.*(Environment|Snapshot|Supersed|Upsert)' -count=1
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/events_git_status_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/git_snapshot_cache_test.go`
- `apps/backend/internal/orchestrator/event_handlers_git_snapshot_environment_test.go`
- `apps/backend/internal/orchestrator/task_operations_git_snapshot_environment_test.go`

## Dependencies

Task 01 provides the final schema and repository methods.

## Risks

- A late live event can race with completion capture.
- Multi-repository events need independent environment partitions.
- Test doubles across handler and executor packages implement the repository interface.

## Parallelism

`sequential`

## Inputs

- System-design sections "Capture and supersession" and "Delivery identity"
- Current live persistence in `event_handlers_git.go`
- Current completion and archive capture in `task_operations.go`

## Results

- Scoped live, completion, and archive capture to the task environment and
  repository identity.
- Added lifecycle payload propagation and sibling-session regressions for
  shared-environment writes.
- Verified the focused orchestrator and lifecycle Git snapshot tests.
