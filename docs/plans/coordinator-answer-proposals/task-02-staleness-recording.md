---
id: "02-staleness-recording"
title: "Superseded settling and recording"
status: done
wave: 2
depends_on: ["01-kind-policy-flag"]
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-ANSWER-002
  - REQ-COORDINATOR-ANSWER-003
  - REQ-COORDINATOR-ANSWER-006
acceptance_criteria:
  - AC-COORDINATOR-ANSWER-003.1
  - AC-COORDINATOR-ANSWER-003.2
  - AC-COORDINATOR-ANSWER-003.3
  - AC-COORDINATOR-ANSWER-003.4
  - AC-COORDINATOR-ANSWER-003.5
  - AC-COORDINATOR-ANSWER-006.1
  - AC-COORDINATOR-ANSWER-006.2
  - AC-COORDINATOR-ANSWER-006.3
  - AC-COORDINATOR-ANSWER-002.8
system_design:
  - ../../specs/coordinator/system-design/answer-proposals.md
---

# Task 02: Refusal settling and recording

## Summary

Cover and finish recording of the refusal settle that task 01 adds (`RefuseOpenTx`, `RefuseClaimedTx`, `settleRefusedTx`; status `rejected`, reserved error code): the `refused` `stale` and
`task_archived` activity mapping, the 409 refusal body and
activity summary counts (`ProposalRefused`) for class `answer`, the approved-detail builder, and the no-undo rule.

## Scope

`approve_kinds.go` (`executeKind` handles `*execRefusal`), `approve.go` (409
body), `activity*.go`, `activity_service.go`, ledger known-tool list if one
exists, tests covering idempotent settle, read error, deleted/archived/no-session
tasks, a refused row absent from the cap and pending list.

## Exclusions

Delivery, policy and the refusal store methods (task 01), UI (task 03). The web activity-log display of answer rows, the class filter option and flag-off summary hiding are out of scope (requirements, Out of scope).

## Acceptance

- A newer propose or an approval after a target change settles the proposal as
  refused (`stale`) once, with one activity row and no second row on a repeat.
- A read error leaves the proposal unrefused (propose refused with the read
  error; approval `failed` `not_delivered`).

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Answer|Activity|Summary' -count=1
```

Round 3 additions to cover with tests: `outcome_unknown` rows are never refused by
approve or a newer propose (`answer_pending`), and approve of one returns the
conflict; `RefuseOpenTx` is a no-op when
`updated_at` changed; a deleted task settles `task_archived` on every path, a permission
`ErrPermissionTaskOrSessionNotFound` on an unavailable task settles
`task_archived`;
`retryable` is set only for the four retryable codes; edits are validated live
before the claim (HTTP 400, row unchanged); change detection treats an absent
`answer` as `requires_approval`; the refused activity row carries the proposal
id and counts in `ProposalRefused`, never in `Refused`; propose errors reach the
coordinator with the typed `{code, field}` shape (AC-001.10).
