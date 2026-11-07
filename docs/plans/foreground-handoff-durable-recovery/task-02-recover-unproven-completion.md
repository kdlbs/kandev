---
id: "02-recover-unproven-completion"
title: "Recover unproven SQL completion history"
status: completed
wave: 2
depends_on:
  - "01-preserve-handoff-submission"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.4
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.5
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.8
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 02: Recover unproven SQL completion history

## Summary

Recover historical SQL completed/journal unknown mismatches through the existing native recovery transaction.
Keep both historical labels intact and retire only authenticated abandoned work.
Prove recovery, reload, and later new input through the existing desktop and phone paths.

## In scope

- Implement the exact unproven-completion predicate from the system design with a read-only repository verifier.
- Include eligible completed SQL metadata during native inventory discovery without broadening ordinary live adoption.
- Carry an explicit `UnprovenCompletion` proof classification through assessment, retirement, and proof creation.
- Pin and revalidate each accepted historical stream's receive/projection cursor and authenticated peer high-water mark under the admission locks.
- Use a dedicated replayable partial inbox index for the terminal-absence predicate.
- Revalidate that class in `verifyNativeResumeInventoryTx` under existing locks before resolving any matching fence.
- Keep `hasProjectedTerminalRetirement` false for this class. Unproven history cannot use a terminal-only recovery shortcut.
- Preserve all SQL historical fields and the journal interrupted-unknown label. Only the journal retirement marker and matched recovery presentation change.
- Apply the assessment consistently to automatic loaded recovery, explicit Resume, repeat adoption, and later ordinary prompt admission.
- Add real SQLite/bbolt, PostgreSQL, interruption, negative inventory, and browser scenario coverage.
- Update the existing public recovery explanation after implementation.

## Out of scope

- Broad acceptance of every completed row with missing metadata.
- Failed/cancelled SQL labels, partial terminal bindings, or pruned-history repair.
- Clearing blocks before authenticated retirement, dispatching the old prompt, or draining queued instructions during recovery.
- New UI controls, localized copy, repair commands, table/column migrations, feature flags, and journal protocol changes. One replayable partial inbox index supports the terminal-absence lookup.
- Investigation or repair of the reporter's original installation.

## Acceptance

1. Complete exact proof restores the same native conversation and resolves only the matching incident fences without changing historical submission fields.
2. A conflicting, live, unprojected, pruned, or unverifiable record prevents retirement and fence resolution. Interruption and stale proofs remain safe.
3. Automatic loaded recovery and explicit Resume converge after reopen. Desktop and phone accept one later distinct instruction without replay or duplicate recovery.

## TDD sequence

1. Add `TestNativeResumeUnprovenSQLCompletionRecovery` using real SQL and bbolt stores and the existing native-resume fixture.
2. Seed SQL completed/outcome completed with all terminal fields empty and settlement/pruning markers absent.
3. Keep exact journal history interrupted-unknown with no terminal. Supply complete owner-death, native identity, and cursor proof.
4. Run the regression before production changes. Expect `native resume terminal evidence disagrees` or the current SQL admission refusal.
5. Add the shared classification and repository predicate. Preserve existing terminal proof priority and complete-inventory validation.
6. Add transactional fence-resolution coverage and failpoints after journal retirement and before SQL commit/publication.
7. Extend the existing native-resume browser fixture. Add the `unproven SQL completion` scenario to desktop and phone recovery suites.
8. Update `docs/public/sessions-and-review.md` to explain the supported guarded recovery and preserved uncertainty.

Do not copy the original incident's payloads, IDs, timestamps, or logs into fixtures.
Extend the helper's scenario union without replacing existing cases.
The new scenario must assert that SQL remains completed, while older scenarios retain their existing expected states.

## Regression matrix

| State or boundary | Required result |
| --- | --- |
| Exact completed SQL / unknown journal, no terminal proof | Retire history, resolve matching fence, preserve labels and native ID. |
| Already-retired journal with the same SQL row | Reuse retained proof and finish resolution without another retirement effect. |
| Reopen after successful resolution | No reopened fence, replacement conversation, or old prompt dispatch. |
| One eligible row plus independent accepted/dispatching work | Refuse the whole inventory before retiring the eligible row. |
| Eligible first candidate plus later hash/owner conflict | No partial retirement and no SQL changes. |
| Completed, failed, or cancelled SQL with real terminal | Preserve ordinary authoritative terminal handling. |
| SQL terminal inbox event or terminal effect, even without final binding | Do not use the unproven fallback. Reconcile or block under existing terminal rules. |
| Partial terminal fields, settlement marker, or pruning marker | Refuse this class. Preserve existing terminal/gap behavior. |
| Terminal arrives after assessment or retirement but before SQL resolution | Stale classification cannot resolve the fence. Retry normal terminal reconciliation. |
| Foreign session/incarnation/stream/hash, future generation, duplicate/truncated inventory | Keep fence and journal history unchanged. |
| Missing historical owner-death/native proof or non-idle successor | Keep recovery blocked without replacement or resend. |
| Unproven SQL completion offered to `hasProjectedTerminalRetirement` | Return false. Require the full dead-owner native-recovery proof. |
| SQL read error, journal error, projection gap, or acknowledgement lag | Preserve the existing specific blocked/wait disposition. |
| Different recovery cause or newer notice | Never clear it with the earlier proof. |
| Direct and queue-origin interrupted instructions | Recovery never dispatches the old instruction. New admission remains separately authorized. |

Assert state, outcome, payload/hash, terminal fields, and full-settlement/pruning markers before and after each success or refusal.
Use a real multi-connection PostgreSQL test for terminal-arrival and resolution races.

## Verification

Run local backend checks from the repository root, one heavy operation at a time.
Set `KANDEV_TEST_POSTGRES_DSN` to the authorized test database before the PostgreSQL command.
A skipped PostgreSQL case is not a passing PostgreSQL result.

```bash
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -race -p=1 ./internal/agent/runtime/lifecycle -run 'Test(NativeResume|UnprovenSQLCompletion|DurableAdoption)' -count=1 -timeout 300s)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run 'Test(NativeResume|UnprovenSQLCompletion)' -count=1 -timeout 300s)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -race -p=1 ./internal/orchestrator -run 'Test(UnprovenCompletionRecoveryEntrypoints|.*NativeResume|RestartRecovery|AlreadyLoadedRestartRecovery)' -count=1 -timeout 300s)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^TestPostgresNativeResumeUnprovenSQLCompletion$' -count=1 -timeout 300s)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -race -p=1 ./internal/task/repository/sqlite -run '^TestPostgresNativeResumeUnproven' -count=1 -timeout 300s)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -race -p=1 ./internal/persistence/storeconformance -count=1 -timeout 300s)
bash scripts/lint-go-changed apps/backend/internal/task/models/native_resume_delivery.go apps/backend/internal/agent/runtime/lifecycle/native_resume_unproven_completion.go apps/backend/internal/agent/runtime/lifecycle/native_resume_unproven_completion_test.go apps/backend/internal/agent/runtime/lifecycle/native_resume_terminal.go apps/backend/internal/agent/runtime/lifecycle/native_resume_evidence.go apps/backend/internal/agent/runtime/lifecycle/native_resume_delivery.go apps/backend/internal/agent/runtime/lifecycle/native_resume_proof.go apps/backend/internal/agent/runtime/lifecycle/loaded_native_reconciliation.go apps/backend/internal/task/repository/sqlite/agent_delivery_retention_schema.go apps/backend/internal/task/repository/sqlite/native_resume_unproven_completion.go apps/backend/internal/task/repository/sqlite/native_resume_unproven_completion_test.go apps/backend/internal/task/repository/sqlite/native_resume_binding.go apps/backend/internal/task/repository/sqlite/native_resume_terminal.go apps/backend/internal/orchestrator/session_native_delivery_recovery.go apps/backend/internal/orchestrator/unproven_completion_recovery_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Update the lint command if the final file list differs. Record the exact command and documentation-coverage preflight in Results.
The first regression run is the behavioral RED gate. Record final targeted test counts and PostgreSQL execution status.

CI owns browser verification under the existing convergence package's boundary:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/session/durable-stream-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-durable-stream-recovery.spec.ts)
```

Use managed production builds and run desktop and phone suites separately.
The new scenarios must reach real backend recovery, then reload and send one new instruction.
Do not claim a successful routed mock proves SQL/journal convergence.

## Files likely touched

Backend paths:

- `apps/backend/internal/task/models/native_resume_delivery.go`.
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_unproven_completion.go` (new).
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_unproven_completion_test.go` (new).
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_terminal.go`.
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_evidence.go`.
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_delivery.go`.
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_proof.go`.
- `apps/backend/internal/task/repository/sqlite/native_resume_unproven_completion.go` (new).
- `apps/backend/internal/task/repository/sqlite/native_resume_unproven_completion_test.go` (new; includes the environment-gated PostgreSQL case).
- `apps/backend/internal/task/repository/sqlite/native_resume_binding.go` and `native_resume_terminal.go`.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_schema.go` for the replayable terminal-absence index.
- `apps/backend/internal/orchestrator/unproven_completion_recovery_test.go` (new).

`apps/web/e2e/helpers/native-resume-notice.ts` and `interrupted-prompt-recovery.ts` own the deterministic browser fixture.
Reuse their current real backend path rather than adding a production test endpoint.

Browser and documentation paths:

- `apps/web/e2e/tests/session/durable-stream-recovery.spec.ts`.
- `apps/web/e2e/tests/session/mobile-durable-stream-recovery.spec.ts`.
- `docs/public/sessions-and-review.md`.

No other production surface is planned. Record a newly required caller path in this work order before changing it.

## Dependencies

[Task 01](task-01-preserve-handoff-submission.md).
Preserve the existing journal release barrier, terminal reconciliation, native restoration, and PostgreSQL advisory locks.
Use the current source and reviewed artifacts again after any base change.

## Risks

- SQL completion labels without proof need an explicit class. They cannot enter the ordinary proven-terminal branch with sequence zero.
- Negative SQL predicates need transaction revalidation because terminal delivery can race recovery.
- A crash between stores leaves a partial recovery. The retained journal marker must make retry safe and convergent.
- New browser fixtures must not bypass the backend predicate or use a developer's live database.
- Preserve the current UI composition. Mobile tests prove the same outcome through its existing touch controls.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/durable-restart-reconciliation.md), cited ACs.
- [Convergence design](../../specs/platform/system-design/session-recovery-convergence.md#foreground-handoff-and-unproven-sql-completion).
- [Historical label decision](../../decisions/2026-10-12-total-session-recovery.md).
- Existing `native_resume_projection_sql_test.go`, `native_resume_pending_terminal_test.go`, and `native_resume_dispatch_reconciliation_test.go` fixtures.
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `/tdd`, `/e2e`, and public documentation guidance.

## Results

Implemented the guarded unproven-completion class across the lifecycle and both
SQL paths.

- New read-only verifier: `internal/task/repository/sqlite/native_resume_unproven_completion.go`.
- New lifecycle classification and discovery: `internal/agent/runtime/lifecycle/native_resume_unproven_completion.go`.
- `NativeResumeSubmissionEvidence.UnprovenCompletion` carries the class through proof creation and pins each historical stream's cursor.
- `verifyNativeResumeInventoryTx` counts this class separately, re-runs the predicate, and re-reads the exact historical cursor watermark under the existing transaction locks.
- `hasProjectedTerminalRetirement` rejects the class, so only dead-owner native recovery can resolve it.
- Explicit Resume admission and loaded discovery both recognize eligible completed metadata without broadening ordinary live adoption.
- Historical SQL state, outcome, payload, hash, and terminal absence are preserved. The journal keeps its interrupted-unknown label and only becomes retired.
- `agent_delivery_retention_schema.go` adds one replayable partial inbox index for the exact terminal-absence lookup; the predicate still checks every stream and the effect ledger.

Permanent tests:

- `internal/agent/runtime/lifecycle/native_resume_unproven_completion_test.go` uses real SQLite/bbolt stores for recovery, repeat retirement, reopen, negative evidence, mixed inventory, stale proof, historical cursor movement, and owner checks.
- `internal/task/repository/sqlite/native_resume_unproven_completion_test.go` and its environment-gated PostgreSQL cases cover the locked proof and resolution matrix, historical cursor revalidation, the inbox query plan, and a concurrent receipt schedule.
- `internal/orchestrator/unproven_completion_recovery_test.go` covers explicit Resume, loaded admission, and a terminal-bearing refusal.
- Desktop and phone E2E scenarios extend the existing native-resume fixture.

Verification results:

- Focused lifecycle suite passed; full `internal/agent/runtime/lifecycle` package passed under `-race` (`191.581s`).
- Focused SQLite suite passed; full `internal/task/repository/sqlite` package passed under `-race` (`620.827s`).
- Focused orchestrator entry-point and native-resume suites passed under `-race`; the full orchestrator package also passed (`262.822s`).
- `go run ./cmd/sqlguard ./internal` and `internal/persistence/storeconformance` passed.
- PostgreSQL was skipped locally because `KANDEV_TEST_POSTGRES_DSN` was unset. The environment-gated proof, cursor, query-plan, and concurrent-receipt tests remain for an authorized database run.
- Public docs were updated. Documentation and public-docs validators passed.
- Browser suites were authored but not run locally; CI owns them.

Review fixup for PR #5109:

- Pinned and revalidated each historical stream's peer-reported high-water mark
  in addition to its receive/projection/retire cursor. An unreceived watermark
  invalidates the proof until a fresh proof covers it.
- Rebound PostgreSQL fixture statements through `repo.db.Rebind` so the
  environment-gated tests use `$n` placeholders.
- Captured submission metadata immediately after each deliberate negative
  mutation and before acknowledgement, so rejected recovery is compared with
  the post-mutation snapshot.
- Added uncertainty-state assertions that the rendered banner exposes Retry
  connection and Stop but not Resume; the Resume UI path remains covered by the
  existing legacy and readiness scenarios.
