---
id: "02-automatic-path"
title: "Automatic resume and start_agent path"
status: todo
wave: 2
depends_on: ["01-eligibility-settings"]
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-AUTOMATIC-005
  - REQ-COORDINATOR-AUTOMATIC-006
  - REQ-COORDINATOR-AUTOMATIC-007
  - REQ-COORDINATOR-AUTOMATIC-008
acceptance_criteria:
  - AC-COORDINATOR-AUTOMATIC-005.3
  - AC-COORDINATOR-AUTOMATIC-007.1
  - AC-COORDINATOR-AUTOMATIC-007.2
  - AC-COORDINATOR-AUTOMATIC-007.3
  - AC-COORDINATOR-AUTOMATIC-007.4
  - AC-COORDINATOR-AUTOMATIC-008.1
  - AC-COORDINATOR-AUTOMATIC-008.2
  - AC-COORDINATOR-AUTOMATIC-008.3
  - AC-COORDINATOR-AUTOMATIC-008.5
  - AC-COORDINATOR-AUTOMATIC-007.5
system_design:
  - ../../specs/coordinator/system-design/automatic.md
  - ../../specs/coordinator/system-design/automatic-classes.md
---

# Task 02: Automatic resume and start_agent path

## Summary

Generalise `TryAutomaticApproval` to the proposal's needed classes, add
`AdmitAction`, call the path from `propose_resume_kandev`, and add the
failure and manager-override lowering with backstop retry.

## Scope

`automatic_approve.go`, `automatic_lower.go`, `admission.go`, approve service
decision paths, `mcp/handlers/coordinator_propose.go`, `approve_kinds.go`, `store_proposals.go` (automatic claim predicate), `wake_visit.go` (`CoordinatorIDsWithRaisedClasses`), `LowerGuard` on `LowerClass`, and the Pause amendment in `pause.md` (locked re-read for all classes).

## Exclusions

Raising `move`; undo for the new classes; containment changes.

## Acceptance conditions

- A held admission leaves the proposal pending with a note and counts nothing.
- A failed automatic action lowers its class; a manager reject or edit lowers it.

## Verification

`cd apps/backend && go test ./internal/coordinator/... ./internal/mcp/...`

## Likely files and risks

Concurrency between claim, lower and settings save under `withCoordinatorLock`.

## Round 3 additions

- `LowerClass` returns `(lowered bool, err error)`; reject, return and approve responses carry `lowered_class` (AC-008.3). Backstop override evidence includes rows with `edited` set and outcome `failed` (AC-008.5).
- Immediate lowering has no age limit; only the backstop queries use the 24-hour window (AC-008.5).
- `ClaimProposalRawTx` already has `automaticAt`; narrow its WHERE. Hold token for a spend read error is `spend_unmeasured` (AC-007.2). `propose_task_kandev` has no deduplication (AC-007.5).

## Round 4 additions

- Narrow the `ClaimProposalRawTx` WHERE only when `automaticAt` is non-nil (manager retry of `failed` proposals unchanged); failed rows record `edited`; override lowering runs after the settling commit; `start_agent` undo evidence joins `starts_agent = 1`; `AdmitAction` is new (splits `Admit` after spend); `ActionSettings.Lower` takes `LowerGuard` and returns `(bool, error)`; new metric `coordinator_automatic_lower_failed_total{trigger}`; `read_error` joins the held reasons.
