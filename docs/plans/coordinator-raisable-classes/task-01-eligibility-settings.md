---
id: "01-eligibility-settings"
title: "Eligibility settings and generalised gate"
status: todo
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-AUTOMATIC-005
  - REQ-COORDINATOR-AUTOMATIC-006
  - REQ-COORDINATOR-AUTOMATIC-007
  - REQ-COORDINATOR-AUTOMATIC-008
acceptance_criteria:
  - AC-COORDINATOR-AUTOMATIC-005.1
  - AC-COORDINATOR-AUTOMATIC-005.2
  - AC-COORDINATOR-AUTOMATIC-005.4
  - AC-COORDINATOR-AUTOMATIC-006.1
  - AC-COORDINATOR-AUTOMATIC-006.2
  - AC-COORDINATOR-AUTOMATIC-006.3
  - AC-COORDINATOR-AUTOMATIC-006.4
  - AC-COORDINATOR-AUTOMATIC-006.6
  - AC-COORDINATOR-AUTOMATIC-006.7
  - AC-COORDINATOR-AUTOMATIC-006.8
  - AC-COORDINATOR-AUTOMATIC-006.9
  - AC-COORDINATOR-AUTOMATIC-006.10
  - AC-COORDINATOR-AUTOMATIC-005.5
  - AC-COORDINATOR-AUTOMATIC-008.4
system_design:
  - ../../specs/coordinator/system-design/automatic.md
  - ../../specs/coordinator/system-design/automatic-classes.md
---

# Task 01: Eligibility settings and generalised gate

## Summary

Add `Policy.Eligibility`, replace the constants in `automatic.go`, and make
`GetEligibility`, `RecordClassReview`, the class routes and the raise gate
take the class. Table-driven tests per class cover the gate interleavings
already tested for `create_task`.

## Scope

`policy.go`, `automatic.go`, `store_automatic.go`, `handlers_automatic.go`,
settings save and its audit (`coordinator_eligibility_changes`, schema file under `store_phase2_schema.go` with a fresh-DB and replay test), the review-under-lock change, the `start_agent` activity-list filter and `ClassSetting.ChangeID`.

## Exclusions

The automatic approval path and lowering triggers (task 02), UI (task 03).

## Acceptance conditions

- Raising each class succeeds when eligible and answers 409 naming the first unmet condition otherwise.
- Defaults reproduce the old values; bounds are enforced.

## Verification

`cd apps/backend && go test ./internal/coordinator/...`

## Likely files and risks

`policy.go`, `automatic.go`, `store_automatic.go`, `handlers_automatic.go`; the `start_agent` join.

## Round 3 additions

- `CountDecidedRowsTx` (three class filters) for the locked review count (AC-006.10).
- Phase 3 off: a non-null `eligibility` member is refused with 400 `policy.eligibility` (AC-006.11); the read paths still return the effective object.
- An eligibility-only save writes no `coordinator_class_changes` row; `changed_by` is null when no user resolves (AC-006.2, -006.9).

## Round 4 additions

- Keep the `total > 0` guard in the unedited test; per-field defaults for a missing or out-of-bounds stored `eligibility` field; validation order and `invalid_setting` code in `parsePolicyMember`/`Validate`; `InsertClassReviewTx` and `CountDecidedRowsTx`, 503 on in-lock failure; eligibility-only save still publishes `coordinator.updated`; the class routes stay 404 with phase 3 off.
