---
id: "03-guarded-restoration"
title: "Restore verified conversations across historical loss"
status: in_progress
wave: 3
depends_on:
  - 02-gap-diagnosis
plan: plan.md
requirements:
  - REQ-PLATFORM-PRUNED-DELIVERY-RECOVERY-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
acceptance_criteria:
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.3
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.4
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.5
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.6
  - AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.8
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
system_design:
  - ../../specs/platform/system-design/pruned-delivery-recovery.md
---

# Task 03: Restore verified conversations across historical loss

## Summary

Persist honest missing-range evidence and recover the original native
conversation through one guarded successor generation. Automatic restart
reconciliation and explicit Resume share the same crash-safe progress.

## In scope

- Additive gap model in the existing restore checkpoint and immutable session
  metadata archive, SQLite/PostgreSQL parity, exact snapshot CAS,
  inventory without metadata/block, and current versus historical projections.
- Authenticated gap-recovery capability and owner-scoped journal checkpoint
  that preserves old streams, submissions, ACK, and cursor values.
- One restore-only candidate generation, exact-dead-source/submission/native
  guards, no initial prompt, exact historical unknown retirement, and atomic
  restored metadata/current generation/matching block settlement.
- Recovery from every cross-store crash checkpoint, including already-loaded
  idle candidates and subsequent backend restart/new instruction.
- Mixed-version refusal and rollback preservation for pending checkpoints.
- A separate recovered-event control uses ordinary projection and same-generation
  native Resume. Available exact events take precedence over a loss fallback.

## Out of scope

- Normal rollover guard relaxation, retained suffix deletion, invented event
  receipt, broad uncertainty clearing, native conversation replacement, live
  owner takeover, queue dispatch, local E2E, or production maintenance edits.

## Acceptance

- The exact synthetic fixture with receipt/projection 5494, retired 0,
  ACK/high-water 5499, first-retained 5500, peer unknown/SQL dispatching and no
  prior metadata/block automatically restores the same native conversation.
  Old cursor values remain unchanged, the missing range survives, the old
  instruction is never dispatched, and one distinct new instruction is admitted.
- Live/unknown source, mismatched payload/owner, prepared or accepted independent
  work, retained suffix, unsupported peer, missing native state, Office or
  archived ownership, and independent blocks each refuse restoration without
  destructive mutation. One bad session does not strand a good sibling.
- Interrupt after each journal/SQL/native checkpoint and reopen both stores.
  Retries reuse one successor, preserve payload/outcome and range, resolve only
  the matching block, and another restart remains usable without old replay.

## Verification

```bash
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'Test(PrunedDelivery|NativeResumeDelivery|Rollover)' -count=1)
(cd apps/backend && GOMAXPROCS=2 go test -p 1 -race -trimpath -tags fts5 ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/orchestrator -run 'Test(PrunedDelivery|SilentRestartRecovery|RestartRecovery|RecoverSessionUnknownPromptKeepsNativeHistory)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

New `TestPrunedDelivery...` cases must exist before a filtered result counts.
PostgreSQL gap/CAS/reopen/migration tests run in the existing PostgreSQL CI job;
record their exact cases and actual results before delivery. Run broader owning
Go package gates only as required by CI/remediation, with concurrency bounded.

## Files likely touched

- `apps/backend/internal/task/models/agent_delivery.go`
- `apps/backend/internal/task/models/session_continuity.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_history_gap.go` (new)
- `apps/backend/internal/task/repository/sqlite/silent_restore.go`
- `apps/backend/internal/agentctl/journal/retained_gap.go` and `retired_stream.go` (new)
- `apps/backend/internal/agentctl/journal/stream_rollover.go` (guard controls only)
- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/agentctl/server/api/durable_delivery.go`
- `apps/backend/internal/agentctl/server/api/retained_delivery.go`
- `apps/backend/internal/agent/runtime/lifecycle/pruned_delivery_recovery.go` (new)
- `apps/backend/internal/agent/runtime/runtime.go` and its facade/adapter seam
- `apps/backend/internal/orchestrator/silent_restart_recovery.go`
- `apps/backend/internal/orchestrator/session_native_delivery_recovery.go`
- Owning `pruned_delivery_recovery_test.go` files (new).

## Dependencies

02-gap-diagnosis; its preflight must understand pending candidate
authority before a gap candidate can initialize. Pending restore authority is
not general admission authority.

## Risks

SQL and journal writes are not atomic together. Pin all source fields and
re-read authenticated proof; never infer progress from an empty journal alone.
Keep old streams immutable while the replacement manager connects to its
new stream. Native history replay must not synthesize missing canonical effects.

## Parallelism

`sequential`. No delegation is authorized.

## Inputs

- Design: loss evidence, guarded restoration, crash convergence, and rollback.
- Existing `session_restore_attempts`, exact source liveness, native retirement,
  record reconstruction, journal rollover, and PostgreSQL test patterns.

## Results

Guarded gap proof, existing restore-checkpoint reuse, source sealing at authenticated retirement, unchanged old cursors, and atomic loss archive implemented. Journal race tests and focused orchestration regressions pass. SQLite/PostgreSQL archive/CAS regressions and no-gap historical empty-block recovery are included; final local and CI parity evidence remain pending.

## CI remediation in progress

The first PR head exposed a frontend test fixture cast error and review identified
that an inbox-covered pruned prefix used generic projection for saved message and
reasoning output. Route the retained SQL payload through canonical projection and
idempotent callbacks before advancing its cursor, with no HTTP preflight ACK or
terminal lifecycle callback. Meaningful message/reasoning/tool controls replace
the context-only positive fixture.

Contributor-task evidence adds queue-origin unknown submissions and legacy older
resolved metadata whose resolution has an empty stream and precedes the notice.
Allow that narrow historical proof only for the exact unknown submission and
same saved native conversation. The strict current restored-admission verifier
remains unchanged. Queue retirement and legacy historical-block regressions are
included; final CI results remain pending. No further local test suite is run.

Backend CI also found a duplicate harness generation in the new orchestration
fixture and repeated restoration recreating a fence for already-retired SQL
unknown history. Reuse the fixture's generation. Treat restored metadata as
historical only when the authenticated journal reports a distinct unsettled
submission; an empty journal cannot recreate a presentation fence. Existing
second-incident/repeated-restart controls cover that regression. Split proof
predicates to meet backend complexity limits without weakening their guards.

## Verified CI corrections

Removed the stale declaration left by the proof-predicate extraction. Focused
SQLite, lifecycle, orchestrator, and actual journal gap/sealing regressions pass
with the race detector. Backend lint across the five affected packages reports
zero issues. The orchestration fixture now preserves the earlier resolved block
under its original reason, matching the incident rather than reopening that row.
Startup diagnostics and restored-error matching share one stream/range formatter.
The second cleanup liveness probe is retained intentionally and covered by a
regression that changes liveness after row selection. CI parity remains pending.
