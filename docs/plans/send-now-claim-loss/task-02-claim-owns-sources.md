---
id: "02-claim-owns-sources"
title: "Let the claim own its sources"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-SEND-NOW-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-SEND-NOW-001.13
system_design:
  - ../../specs/ui/system-design/message-queue-send-now.md
---

# Task 02: Let the claim own its sources

## Summary

Ordinary FIFO delivery cannot reserve a queue entry that an open Send Now claim
owns, even after the entry's delivery lease expires. A stalled Send Now then
restores its durable message instead of losing it.

## In scope

- `reserveHeadTx` (SQLite and PostgreSQL) reads the session's
  `queue_send_now_claims` row in its transaction. When the head entry is one of
  the claim's sources, it returns no reservation and does not skip to the next
  entry. Decode `claim_json` in Go; do not add dialect-specific JSON SQL.
- `reserveHeadLocked` in the memory repository applies the same rule with
  `sendNowClaimContainsLocked`. Confirm that the claim set includes durable
  sources, which keep their rows.
- Every head-reservation entry point that reaches these helpers inherits the
  rule.

## Out of scope

- The lease length and `agentPromptReadyTimeout`.
- Releasing a claim that cannot be restored (Task 03).

## Acceptance

- Repository regression (SQLite, memory, and env-gated PostgreSQL): claim a
  durable plan-comment source, expire its lease, then reserve the head. The
  reservation is empty, and `RestoreSendNowClaim` succeeds with the row back
  and the claim gone. Write it first and confirm the restore fails on `main`
  with `ErrSendNowClaimChanged`.
- Orchestrator regression: a Send Now for a durable source whose replacement
  prompt fails after the lease expires, with a readiness drain before the
  restore. The entry is restored and then delivered exactly once by the
  restore drain from #4988, with one delivery submission.
- A head entry that no claim owns keeps the existing lease behavior.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator/messagequeue -run 'SendNow|PlanCommentLease' -count=1)
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath ./internal/orchestrator -run 'SendNow' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal && go test -race -trimpath ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed <changed Go files>
```

Set `KANDEV_TEST_POSTGRES_DSN` for the PostgreSQL case when available, and
record whether it ran.

## Files likely touched

- `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`
- `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`
- `apps/backend/internal/orchestrator/messagequeue/send_now_claim_ownership_test.go` (new)
- `apps/backend/internal/orchestrator/queue_send_now_claim_ownership_test.go` (new)

## Dependencies

None. Task 03 must ship with this task: until it lands, a claim whose restore
fails for another reason keeps blocking its entry.

## Risks

- A blocked head pauses the whole session queue while the claim is open. That is
  intended, because Send Now is replacing the turn. Every claim ends in
  acknowledgement, restoration, or Task 03's release.
- Reading the claim row adds one indexed query per head reservation. The table
  has at most one row per session.

## Parallelism

`sequential`

## Inputs

- [Claim ownership over leases](../../specs/ui/system-design/message-queue-send-now.md#claim-ownership-over-leases)
- `repository_identity_boundary_test.go` (`planCommentLeaseMessage`, `TestRepositoryExpiredPlanCommentLeaseRejectsStaleOwner`)
- `queue_send_now_restore_drain_test.go`

## Results

Implemented. `reserveHeadTx` (SQLite and PostgreSQL) reads the session's
`queue_send_now_claims` row inside the reservation transaction and returns no
reservation when the head is one of its sources, without skipping to the next
entry. `reserveHeadLocked` applies the same rule through
`sendNowClaimContainsLocked`, and the memory claim set now includes every
durable source that keeps its row. The claims table is also created by
`initSchema` so fresh and existing databases expose it before the first
reservation.

`send_now_claim_ownership_test.go` covers memory, SQLite, and env-gated
PostgreSQL with an expired lease, and keeps the unowned lease behavior.
`queue_send_now_claim_ownership_test.go` covers the stalled replacement prompt
with a readiness-drain reservation. Both new tests fail without the ownership
rule.

Verified:

```bash
(cd apps/backend && go test -trimpath ./internal/orchestrator/messagequeue -run 'SendNow|PlanCommentLease' -count=1)
(cd apps/backend && env -u KANDEV_DEBUG_AGENT_MESSAGES go test -trimpath ./internal/orchestrator -run 'SendNow' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race -trimpath ./internal/persistence/storeconformance -count=1)
```

`KANDEV_TEST_POSTGRES_DSN` was not set, so the PostgreSQL subtest skipped.
