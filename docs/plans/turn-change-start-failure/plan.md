---
created: 2026-10-21
status: implemented
requirements:
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-007
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
legacy_specs: []
---

# Implementation Plan: Failed start capture settlement

## Overview

Settle terminal turn history as unavailable when the start checkpoint could
not be persisted. One sequential work order covers the repository guard,
coordinator integration, and capture-fence regression.

## Evidence and root cause

Execution `61739952-0895-4ab8-8e09-e9aff4870b20`, generation 2, logged
`persist turn change start endpoints: context deadline exceeded` at 17:39:28
on 2026-10-21. Read-only database inspection found revision 1, capture enabled,
start unaccepted, and no terminal timestamp. Terminal finalization rejects that
state through `canFinalizeTurnChangeSet` and the matching SQL predicate.
The coordinator labels the rejection an ownership revision loss. Lifecycle
retries indefinitely, retaining the successor fence.

## Scope

Repair conformance to AC-TASKS-TURN-CHANGES-003.1, .003.3, .003.6, and .007.1.
Preserve capture policy, accepted endpoints, revision guards, and completion
authority. No UI, schema, runtime flag, provider, or live-data changes.

## Technical approach

Permit unavailable, incomplete finalization without an accepted start in both
the repository precheck and SQL update predicate. Keep the change-set revision
and terminal timestamp guards. Update the coordinator test double if needed
to enforce the production invariant. Exercise the real repository path so a
permissive fake cannot hide the defect. Existing successful and disabled
capture paths retain their contracts.

## Tests

Task 01 adds `TestTurnChangeUnavailableWithoutAcceptedStart` at the real SQL
boundary, `TestCoordinatorSettlesUnacceptedStart` against the real repository,
and reuses `TestTerminalCaptureRetriesPersistenceAndReleasesFence` plus
`TestPromptAdmissionCaptureRunsBeforeProviderDispatch` for lifecycle fence coverage.
Include stale revisions, ready-result rejection, duplicate terminal events,
and an accepted-start control. PostgreSQL coverage uses the existing DSN gate.
These backend checks cover the failure and successor-admission path without
a new browser test or rendered UI change.

## Work orders

- [x] [Task 01: Settle unaccepted start capture](task-01-settle-unaccepted-start.md)

## Verification results

The real coordinator and repository regressions failed before the fix and pass
with the guarded unavailable-finalization path. Coordinator and lifecycle full
suites passed. Scoped turn-change, terminal-capture, and admission race checks
passed across all three packages. SQL guard, persistence store conformance
under race, and changed-package lint passed. Documentation catalog, spec lint,
PR documentation coverage preflight, and diff checks passed.

The complete SQLite suite reached its ten-minute global timeout during an
unrelated walkthrough fixture setup, with no assertion failure reported.
Review follow-up explicitly covers rejected `failed` availability without an
accepted start; the repository regression passed again under race.
PostgreSQL cases are included but not run locally without the test DSN.
[Task 01 results](task-01-settle-unaccepted-start.md#results) record commands,
results, and coverage limits. CI still owns complete-suite validation.

## Risks

Broadening finalization must not permit a ready result without accepted starts
or overwrite a newer revision. Preserve write-once endpoints and exclude
completed sets. The original start timeout may have an independent latency cause.
