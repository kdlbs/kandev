---
id: "01-fresh-fallback-for-deleted-route-destination"
title: "Fresh fallback for a deleted route destination"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Fresh fallback for a deleted route destination

## Summary

Let a start whose recorded workflow route destination was deleted allocate the
target profile's fresh session instead of failing every start in the entry.

## In scope

- Replace the test that pinned the rejection with a fallback test, and add a
  test that keeps repository failures fatal, before the change.
- Return the recorded route without a session for a not-found destination.
- Update the system design sentence on a deleted recorded destination.

## Out of scope

Mismatched destinations, clearing the route on session delete, and UI changes.

## Acceptance

- A recorded route whose destination was deleted loads without error and
  without a session, so the caller allocates a fresh one.
- Starting the step entry again records the replacement in the route, and a
  retry of the same entry reuses it, for prepared and committed routes.
- A repository failure while loading the destination still fails the entry.
- Existing route, session target, and profile switch tests pass.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'WorkflowSession|SessionTarget|ProfileSwitch|ProfileSession|Route|ExplicitWorkflow' -count=1)
(cd apps/backend && go vet -trimpath ./internal/orchestrator)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/orchestrator/workflow_session_target.go
- apps/backend/internal/orchestrator/workflow_session_target_test.go
- docs/specs/tasks/system-design/workflow-profile-session-lifecycle.md

## Dependencies

None.

## Risks

A retry after the destination was deleted creates a new destination under the
same route operation; the new session replaces the record.

## Parallelism

`sequential`

## Inputs

- Issue #4324.
- AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6.
- The linked workflow profile session lifecycle design.

## Results

- RED: on the original code, loading a route whose destination was deleted
  returned `load recorded workflow session route destination`.
- GREEN: the new tests and the existing route tests pass.
- `TestCeilingDispatchAdmissionSerializesRouteMutation` fails on Windows with
  and without this change.
