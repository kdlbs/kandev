---
created: 2026-09-28
status: draft
requirements:
  - REQ-TASKS-BUILTIN-WORKFLOW-RECONCILE-001
system_design:
  - ../../specs/tasks/system-design/builtin-workflow-step-reconciliation.md
legacy_specs: []
---

# Implementation Plan: Built-in Workflow Reconcile Lock Retry

## Overview

Make a transient database contention error inside a built-in workflow step
reconciliation attempt a retryable outcome, under the existing per-target budget
and warn-and-continue exhaustion path. One sequential work order owns the
outcome-contract change in the shared helper, both reconciler loops, the test
seam, and the regressions.

## Confirmed root cause

`tryHealWorkflowStepEvents` returns every database error with `retry=false`.
`healParticipantSeatRowWithRetry` and `healAgentErrorRowWithRetry` return on
the first non-nil error. `initSchemaContext` stops at the first failing step,
so a SQLite WAL snapshot conflict ("database is locked" on the `UPDATE` after
another process committed) makes the backend exit with "failed to initialize
schema".

## Scope

### In scope

- New attempt outcome type returned by `tryHealWorkflowStepEvents`, classified
  with the existing `isRetryableStateRaceError`.
- Both per-target retry loops honour both retry causes and add `retry_cause` (and
  `error` for lock contention) to the exhaustion warning.
- Test-only after-read hook and per-site DB failpoint; real-driver, per-site, and cancellation regressions.
- Table test for `isRetryableStateRaceError`.

### Out of scope

See the requirement exclusions: other schema steps, a startup-time bound under
a held lock, `_txlock=immediate`, DSN or budget changes, and the PR #3963
reconciler.

## Work orders

| Work order | Wave | Depends on | Result |
| --- | --- | --- | --- |
| [01 Classify contention in reconciler attempts](task-01-classify-contention.md) | 1 | none | Both reconcilers retry transient contention and never block startup on it |

## Verification strategy

Backend unit and integration tests in the repository package against a real
file-backed SQLite database, then backend lint. No frontend, API, or public
documentation surface changes, so no E2E test is required.

## Risks

- The exact driver error for the WAL snapshot conflict must match the
  classifier. The regression uses the real driver, so a mismatch surfaces as a
  failing test rather than a silent gap.
- Under a persistently held write lock a row can wait up to 5 attempts times
  the 5-second busy timeout before it is skipped. Accepted and excluded in the
  requirement.
