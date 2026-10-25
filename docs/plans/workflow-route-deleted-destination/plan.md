---
created: 2026-10-11
status: done
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Reassign a start whose recorded route destination is gone

## Overview

A step with a `session_target` records its destination session in the task's
`workflow_session_route`. When that session is deleted, by the user or by a
compensation path, the route stays. Every later start in the same step entry
reloads it and fails with `load recorded workflow session route destination`,
until the task moves to another step. A terminal destination already falls back
to a fresh session. AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6 asks for the same
fallback when the referenced conversation is absent. Issue #4324 reports the
gap.

## Scope

### In scope

- Treat a not-found recorded destination like a terminal one: log it and let
  the caller allocate the target profile's fresh session.
- Keep any other destination load failure fatal.
- Update the system design sentence that made a deleted destination fail the
  entry.

### Out of scope

- A destination that exists but no longer matches the route's task or profile,
  which still fails closed.
- Clearing the route when a session is deleted.
- Rendered UI.

## Technical approach

`loadRecordedWorkflowSessionRoute` checks the destination lookup for
`models.ErrTaskSessionNotFound` and returns the recorded route without a session,
as it does for a terminal destination. Both callers already allocate a fresh
session for that result and replace the bounded route record. A warning records
the deleted destination ID.

## Tests

`workflow_session_target_test.go` covers
`TestLoadRecordedWorkflowSessionRouteAllowsFreshFallbackFromDeletedDestination`
and `TestPrepareExplicitWorkflowSessionReplacesDeletedRouteDestinationOnce`:
AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.6. The first replaces the test that
pinned the rejection, for prepared and committed routes. The second starts the
step entry again and checks that the route records the replacement and that a
retry reuses it without another session.
`TestLoadRecordedWorkflowSessionRouteKeepsDestinationLoadFailureFatal` keeps a
repository failure fatal.

## E2E tests

The service tests use the real repository. No rendered UI changes require
browser coverage.

## Work orders

- [x] [Task 01: Fresh fallback for a deleted route destination](task-01-fresh-fallback-for-deleted-route-destination.md)

## Verification results

The deleted-destination test failed on the original code and passes with the
change. Existing route, session target, and profile switch tests pass;
`TestCeilingDispatchAdmissionSerializesRouteMutation` fails on Windows with and
without this change.

## Risks

A retry after the destination was deleted creates a new destination under the
same route operation instead of stopping. The new session replaces the record,
so later retries reuse it.
