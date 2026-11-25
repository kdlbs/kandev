---
id: "02-restart-fence-creation"
title: "Preserve restart recovery incident fences"
status: done
wave: 2
depends_on:
  - "01-historical-native-proof"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 02: Preserve restart recovery incident fences

## Summary

Make restart fence creation conditional on the complete current open-fence inventory.
Reuse an existing eligible fence without changing its binding.
Keep already duplicated incidents recoverable through Task 01's authenticated proof.

## In scope

- Reproduce `recreateRestartRecoveryBlock` adding a generic companion beside a current submission-bound fence.
- Keep the passing ordinary-presentation-repair control.
- Add the narrow `EnsureRestartRecoveryBlock` repository capability.
- Under existing turn/delivery locks, revalidate the current generation before reading and creating a fence.
- Return one eligible existing fence unchanged. Insert only when no current fence exists.
- Permit resolved history and a later open fence with the same key by migrating to an open-only uniqueness index.
- Refuse to recreate the same resolved submission fence; create a new generic fence only when retained interrupted work proves a later incident.
- Preserve multiple and independent fences without another insertion.
- Use the capability in restart presentation repair and propagate read/write failures.
- Cover PostgreSQL concurrent repair and delivery-fence insertion using separate connections and synchronization barriers.
- Cover a real `RecoverSession` retry after historical silent restoration and already duplicated incident fences.

## Out of scope

- Deletion of existing recovery fences or broadening generic recovery authority.
- Relaxing the generic resolver's single-fence guard.
- Changing independent consumer, ordinary upsert or Office admission behavior.

## Acceptance

1. `TestRestartRecoveryBlockEnsureReusesIncident` reproduces the companion defect before the correction.
   Repeated and concurrent repair then retain the existing fence ID, reason, submission and stream.
2. Missing-fence repair creates one fence for the current eligible generation.
   A resolved same-incident fence remains closed, while a distinct retained successor interruption permits a new generic fence. Generation replacement, independent fences, multiple causes and storage failures preserve the refusal without partial writes.
3. `TestRepeatedNativeResumeAfterSilentRestore` proves exact authenticated resolution of the issue's existing matching companion fences.
   A mixed matching/independent set remains blocked, with no old dispatch or outcome rewrite.

## Verification

Run from the repository root. Set `KANDEV_TEST_POSTGRES_DSN` for the concurrent PostgreSQL acceptance case.
Use barriers instead of timing-based sleeps.

```bash
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -p 1 -race ./internal/task/repository/sqlite -run 'Test(Postgres)?(RestartRecoveryBlockEnsure|NativeResumeAfterSilentRestore|UpsertProjectedTerminalResolvesExistingOpenBlock)' -count=1)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -p 1 -race ./internal/task/repository/sqlite -run 'Test(Postgres)?SessionRecoveryBlockIncidentUniquenessMigration' -count=1)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -p 1 ./internal/automation -run '^TestRetryLegacyQuarantineUsesOpenRecoveryFenceIndex$' -count=1)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -p 1 -race ./internal/orchestrator -run 'Test(RepeatedNativeResumeAfterSilentRestore|RestartRecovery|RestartInventory|RecoverSessionUnknownPromptKeepsNativeHistory)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -p 1 -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/repository/interface.go apps/backend/internal/task/repository/sqlite/restart_recovery_block_ensure.go apps/backend/internal/task/repository/sqlite/restart_recovery_block_ensure_test.go apps/backend/internal/orchestrator/restart_recovery_presentation.go apps/backend/internal/orchestrator/restart_recovery_presentation_test.go apps/backend/internal/orchestrator/repeated_native_resume_test.go
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/restart_recovery_block_ensure.go` (new)
- `apps/backend/internal/task/repository/sqlite/restart_recovery_block_ensure_test.go` (new)
- `apps/backend/internal/orchestrator/restart_recovery_presentation.go`
- `apps/backend/internal/orchestrator/restart_recovery_presentation_test.go`
- `apps/backend/internal/orchestrator/repeated_native_resume_test.go` (new)

## Dependencies

Task 01 supplies verified historical proof and current incident-set resolution.

## Risks

A read followed by an unlocked upsert leaves the concurrency defect open.
Preserve the existing PostgreSQL advisory-lock namespace and turn-before-delivery lock order.
Do not infer common incident ownership merely from matching generation numbers.

## Parallelism

`sequential`

## Inputs

- The plan's helper reproduction and passing serial control.
- Interrupted prompt recovery design, section `Repeated recovery after silent restoration`.
- `session_continuity.go`: current upsert, lock and block-list helpers.
- `session_delivery_reconstruction.go`: `selectDeliveryReconstructionBlock`.
- `restart_recovery_presentation_test.go` and `session_native_delivery_recovery_test.go`.

## Results

Implemented `EnsureRestartRecoveryBlock` with the existing turn and delivery locks. The transaction rechecks the current generation and complete open-fence inventory. It returns one eligible fence unchanged and inserts only when no fence exists.

Restart presentation repair now uses this repository operation. It preserves existing bindings and does not add a generic companion beside an existing incident fence. The authenticated resolver still rejects an independent restriction.

Review remediation permits resolved recovery history to coexist with later open incidents. SQLite rebuilds legacy `session_recovery_blocks` tables with all bindings and timestamps preserved; PostgreSQL drops the matching legacy unique constraint. Both engines enforce one open block per session, incarnation, generation and reason. A bound candidate cannot reopen its resolved submission. A generic candidate needs a retained interrupted submission created after the resolved generic fence. Automation's legacy quarantine insert now tolerates both the partial open index and its deterministic row ID.

Terminal-effect binding now updates a matching open block by its existing ID before resolved rows are stored. Repeated upserts reuse the exact resolved binding, while a different open submission keeps its original binding.

The projected-terminal upsert regression was first run against the partial-index implementation and failed with two rows instead of one. After the correction, the SQLite race test passes and confirms the original row is resolved in place with its terminal identity and outcome preserved across an idempotent retry. The PostgreSQL counterpart is gated and did not run because `KANDEV_TEST_POSTGRES_DSN` is unset.

Verification passed:

- The SQLite race suite passed for `RestartRecoveryBlockEnsure` and `NativeResumeAfterSilentRestore`.
- The SQLite race suite passed for resolved same-incident refusal, distinct successor creation and legacy-constraint migration replay. The automation quarantine test passed with the open-only index.
- The orchestrator race suite passed for repeated Resume and restart recovery.
- SQL guard and store-conformance race tests passed.
- Changed-Go-file lint reported `0 issues`. `git diff --check` passed.
- `make -C apps/backend build` passed. The focused SQLite repository race command, including terminal-effect upsert and repeated-resume regressions, passed.
- A full SQLite repository race run timed out after the package's 10-minute limit in `TestSidebarQueryPreparationMemory/101`; it did not report a failure in the modified recovery paths.
- PostgreSQL concurrency cases did not run because `KANDEV_TEST_POSTGRES_DSN` is unset.
