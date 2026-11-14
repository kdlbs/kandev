---
id: "02-producer-reconciliation"
title: "Producer notice reconciliation"
status: done
wave: 2
depends_on:
  - "01-owning-contract"
plan: "plan.md"
requirements:
  - REQ-TASKS-CHILD-STALL-004
acceptance_criteria:
  - AC-TASKS-CHILD-STALL-004.1
  - AC-TASKS-CHILD-STALL-004.2
  - AC-TASKS-CHILD-STALL-004.3
  - AC-TASKS-CHILD-STALL-004.4
  - AC-TASKS-CHILD-STALL-004.5
system_design:
  - ../../specs/tasks/system-design/child-turn-stalled-signal.md
---

# Task 02: Producer Notice Reconciliation

## Summary

Keep delivered candidates scannable and reconcile their notices each producer
pass so a notice follows the parent's current promptable primary and the
dashboard backlog cannot grow without bound.

## In scope

- Stop resolving a delivered candidate at admission; keep its source-turn marker
  scannable while its notice is pending.
- Add a delivered branch to the producer pass that, per candidate: resolves when
  the entry is gone or the source no longer qualifies; withdraws and holds
  (`waiting_parent`) when the parent has no promptable primary; withdraws and
  resolves when the parent task is archived/terminal; re-homes to the current
  promptable primary otherwise.
- Run the existing stale/superseded prune for each session that held a pending
  notice, even when it never drains.
- Record bounded outcomes (`withdrawn_parent_gone`, `held_waiting_parent`,
  `rehomed_notice`); never label identity.

## Out of scope

- New queue tables/queries, retention, scheduler, or dashboard changes.
- Touching non-alert entries or changing Auto-run/merge policy.

## Acceptance

1. A delivered notice whose session stops being the parent's promptable primary
   is re-homed when possible, otherwise withdrawn and held; it never remains a
   permanent dashboard row.
2. Only child-attention alert entries are withdrawn, re-homed, or pruned; user,
   agent, workflow, server-non-alert, question, answer, and approval entries are
   untouched and unreordered.
3. Read failures leave the candidate retryable; repeated passes are idempotent.

## Verification

Focused Go tests (see Task 03). Changed-code lint with
`golangci-lint run ./... --new-from-rev=<base-sha>`.

## Files likely touched

- `apps/backend/internal/orchestrator/child_stall_producer.go`
- `apps/backend/internal/orchestrator/child_stall_delivery.go`
- `apps/backend/internal/orchestrator/child_stall_dispatch.go`
- `apps/backend/internal/orchestrator/child_stall_reconcile.go` (new)

## Dependencies

Task 01.

## Risks

Re-home must preserve destination FIFO; withdrawal must be gated strictly on
`IsChildStallAlert`; the delivered marker must stay idempotent under repeated
passes and read failures.

## Parallelism

`sequential`

## Inputs

- [Design](../../specs/tasks/system-design/child-turn-stalled-signal.md)
- `apps/backend/internal/orchestrator/child_stall_dispatch.go`
- `apps/backend/internal/orchestrator/messagequeue/child_stall_alert.go`
- `apps/backend/AGENTS.md`

## Results

Delivered candidates stay scannable; each pass reconciles notices (re-home,
withdraw/hold, withdraw/resolve) and prunes stale/superseded entries for holder
sessions, gated on `IsChildStallAlert`.
