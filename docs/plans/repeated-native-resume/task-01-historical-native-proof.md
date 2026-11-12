---
id: "01-historical-native-proof"
title: "Accept verified silent-restore history"
status: done
wave: 1
depends_on: []
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

# Task 01: Accept verified silent-restore history

## Summary

Allow current native Resume to use exact historical silent-restore proof.
Preserve the current authenticated inventory checks and the original native conversation.
Keep historical metadata distinct from active uncertainty and context continuation.

## In scope

- Add `native_resume_after_silent_restore_test.go` before production changes.
- Reproduce strict and legacy historical rejection after a real `CommitSilentRestore`.
- Reproduce generic, submission-bound and authenticated missing-stream rejection in generation 2.
- Cover both one current fence and two matching incident fences.
- Recognize `models.SilentRestoreAction` only in historical proof paths with exact existing bindings.
- Verify unchanged native lineage across every generation crossed by an older notice.
- Handle historical native restoration separately from `hasResolvedContextContinuationNoticeTx`.
- Preserve the restored historical notice, old unknown outcomes, payload hashes and absent terminal references.
- Retain strict current `HasResolvedNativeResumeRecovery`, generic single-fence restrictions and existing context-continuation rules.
- Cover SQLite and environment-gated real PostgreSQL behavior.

## Out of scope

- New recovery actions, automatic continuation, new schema or a destructive escape hatch.
- Restart fence creation, which belongs to Task 02.
- Treating a lower metadata generation or an action string alone as current proof.

## Acceptance

1. `TestNativeResumeAfterSilentRestore` fails before the fix and succeeds with complete generation-2 proof afterward.
   Include historical strict/legacy, generic, bound and duplicate-fence scenarios.
2. Foreign native/session/incarnation, unsupported action, changed timestamp, future notice, gap, pending restore and projection lag preserve every fence.
   Mixed live/retired or matched/unmatched submissions must refuse the complete transaction.
3. Retry after journal retirement or an injected SQL rollback preserves uncertainty and resolves only the exact incident once.
   No old prompt, invented terminal outcome or independent restriction changes.

## Verification

Run from the repository root. A configured `KANDEV_TEST_POSTGRES_DSN` is required for PostgreSQL acceptance.
Without that configuration, record the skipped cases and leave PostgreSQL verification pending.

```bash
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -p 1 -race ./internal/task/repository/sqlite -run 'Test(Postgres)?(NativeResumeAfterSilentRestore|NativeResumeHistoricalSilentRestoreProof|NativeResumeAuthenticatedMissingBinding|NativeResumeEmptyHistoricalBlock|SilentRestoreCommitExactTransaction)' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -p 1 -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/native_resume_generic_block.go apps/backend/internal/task/repository/sqlite/native_resume_historical_metadata.go apps/backend/internal/task/repository/sqlite/native_resume_recovery.go apps/backend/internal/task/repository/sqlite/native_resume_binding.go apps/backend/internal/task/repository/sqlite/native_resume_after_silent_restore_test.go
git diff --check
```

If implementation adds another production helper, include its path in the focused lint command.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/native_resume_generic_block.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_historical_metadata.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_recovery.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_binding.go`
- `apps/backend/internal/task/repository/sqlite/native_resume_after_silent_restore_test.go` (new)

## Dependencies

None.

## Risks

The strict current verifier and historical verifier have different authority.
A missing-stream fallback cannot receive the authority of an authenticated proof without that proof.
The legacy branch must retain its exact native-identity and unknown-submission requirements.

## Parallelism

`sequential`

## Inputs

- The plan's reproduction matrix and acceptance mapping.
- Interrupted prompt recovery design, section `Repeated recovery after silent restoration`.
- `native_resume_binding_test.go` and `native_resume_generic_block_test.go`.
- `silent_restore_test.go`: `newSilentRestoreFixture` and `prepareSilentRestoreCandidate`.
- Accepted ADR `2026-10-12-total-session-recovery.md`.

## Results

Implemented historical silent-restore proof for exact and legacy empty-stream source fences. Current `HasResolvedNativeResumeRecovery` remains restricted to an explicit `resume` action. The authenticated resolver verifies the source fence and every generation edge through the current native identity, preserves the older restored notice, and resolves only the current matching fence set.

Permanent tests in `native_resume_after_silent_restore_test.go` cover strict and legacy history, generic fallback, authenticated missing-stream binding, duplicate matching fences, independent restrictions, changed native identity, broken predecessor lineage, unsupported source action, and SQLite transaction rollback. The SQLite targeted tests passed.

Review follow-up: historical native resolution now rejects a retained proof row whose submission ID is present but whose generation or stream does not match the restored notice. A regression changes only the historical proof row's stream and confirms both current fences remain open and historical metadata stays unchanged. The requested `Test(Postgres)?NativeResumeAfterSilentRestore` race command passed for SQLite, and focused changed-file lint reported `0 issues`; PostgreSQL remains pending because `KANDEV_TEST_POSTGRES_DSN` is unset.

Verification passed:

- The planned focused race test command passed for the SQLite package.
- `go run ./cmd/sqlguard ./internal` passed from `apps/backend`.
- The planned store-conformance test passed with `-race`.
- The planned `bash scripts/lint-go-changed ...` command completed with `0 issues`.
- PostgreSQL behavior cases were not run because `KANDEV_TEST_POSTGRES_DSN` is unset.
