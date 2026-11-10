---
created: 2026-10-20
status: done
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
legacy_specs: []
---

# Fix plan: restore recovered delivery notices on native Resume

## Overview

A durable delivery notice moves to `recovered` when its transport reattaches.
If that execution is later lost and a readiness failure reopens the delivery
block, explicit Resume reloads the native conversation on a new execution and
asks SQL to resolve the block with authenticated retirement evidence. The
transaction only accepts notices in `uncertain` or `reconnecting`, so a
`recovered` notice makes the whole resolution return false and the session stays
blocked on every Resume. One work order lets that transaction restore a
`recovered` notice under the existing proof checks.

## Scope

In scope: `nativeResumeNoticeMatches` in
`apps/backend/internal/task/repository/sqlite/native_resume_binding.go` and a
permanent regression test beside it.

Excluded: orchestrator Retry handling, live recovery publication, and UI.

## Work orders

- [x] [Task 01: Restore recovered delivery notices on native Resume](task-01-recovered-notice-resume.md)

## Changed-path coverage

The work order owns the repository change, the new
`native_resume_recovered_notice_test.go`,
AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.6 in
`docs/specs/platform/requirements/durable-restart-reconciliation.md`, and the
recovered-notice paragraph in the session recovery convergence design.

## Verification

`native_resume_recovered_notice_test.go` runs
`ResolveSessionRecoveryBlockWithNativeEvidence` against a real SQLite repository
(and PostgreSQL when `KANDEV_TEST_POSTGRES_DSN` is set) with a `recovered`
notice whose execution differs from the current ready execution. It fails on
the base revision and passes with the fix; a `settled` notice stays blocked.

## Risks

- Accepting a settled or foreign notice would let Resume overwrite unrelated
  history; only `recovered` is added, and the generation, stream, submission,
  native identity, inventory, and cursor checks are unchanged.
