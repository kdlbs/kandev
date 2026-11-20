---
id: "03-repeated-recovery-evidence"
title: "Prove repeated native recovery end to end"
status: done
wave: 3
depends_on:
  - "01-historical-native-proof"
  - "02-restart-fence-creation"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.23
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 03: Prove repeated native recovery end to end

## Summary

Prove repeated native recovery with real journal retirement and the shipped Resume control.
Run the same user flow on desktop and phone.
Keep the existing controls, localized copy and mobile composition.

## In scope

- Add lifecycle integration coverage using retained bbolt storage and real SQL recovery transactions.
- Perform one silent restoration, interrupt a distinct successor-generation prompt, then request native Resume.
- Repeat after journal reopen and after retirement succeeds but SQL resolution rolls back.
- Cover generic, missing-stream submission-bound and already duplicated current incident fences.
- Reuse existing isolated browser fixtures for desktop and phone.
- Assert unchanged session/native identity, retained old unknown outcomes, no fabricated completion and no old prompt replay.
- Assert no automatic queue drain during restoration. Send a distinct follow-up instruction through normal admission exactly once.
- Reload and retry recovery to prove idempotence and persistent state.
- Cover a matching fence beside an independent restriction and preserve blocked feedback.
- Record exact final commands and counts in all three work orders and the plan.

## Out of scope

- Rendered layout, copy, navigation and control changes.
- New production fixture endpoints, live-instance restarts or user-data mutation.
- Reconnect, Stop and destructive discard semantics.

## Acceptance

1. `TestRepeatedNativeResumeAfterSilentRestoreJournal` fails on the baseline stale-notice decision and passes after Tasks 01 and 02.
   It proves real retirement, journal reopen, retry after SQL failure, complete inventory and absence of historical redispatch.
2. Desktop and phone specs recover each current incident shape through existing Resume, preserve native identity and admit one distinct instruction.
   Repeated recovery and reload do not duplicate dispatch. Independent restrictions retain the existing failure state.
3. All task-defined checks pass with actual results recorded.
   PostgreSQL acceptance remains pending when its required DSN is unavailable.

## Verification

Run sequentially from the repository root. Install workspace dependencies before the first package command in a fresh worktree.
The managed browser runner builds fresh artifacts and cleans up its disposable instances.

```bash
(cd apps/backend && GOMEMLIMIT=2048MiB GOMAXPROCS=2 go test -trimpath -tags fts5 -p 1 -race ./internal/agent/runtime/lifecycle -run 'Test(RepeatedNativeResumeAfterSilentRestoreJournal|NativeResume|DurablePromptTurnRecovery)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/repeated_native_resume_test.go apps/backend/internal/agent/runtime/lifecycle/e2e_repeated_native_resume_fixture_test.go
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web exec playwright install --with-deps chromium
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/repeated-native-resume.spec.ts tests/session/native-resume-projection.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-repeated-native-resume.spec.ts tests/session/mobile-native-resume-projection.spec.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec prettier --check e2e/helpers/repeated-native-resume.ts e2e/tests/session/repeated-native-resume.spec.ts e2e/tests/session/mobile-repeated-native-resume.spec.ts e2e/helpers/interrupted-prompt-recovery.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/repeated-native-resume
git status --short -- docs/plans/repeated-native-resume
```

Run the PR-documentation coverage preflight described in the plan's final design-validation results.
Keep its accepted requirement, design and work-order references in the final verification record.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/repeated_native_resume_test.go` (new)
- `apps/web/e2e/helpers/repeated-native-resume.ts` (new)
- `apps/web/e2e/tests/session/repeated-native-resume.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-repeated-native-resume.spec.ts` (new)
- This plan and its three work orders for results and status.

Existing reusable fixture helpers can be extended only with focused tests for new fixture logic.
Include each such file in its task validation and results.

## Dependencies

Tasks 01 and 02. Existing replay, native-resume and isolated backend fixtures are available on the investigation baseline.

## Risks

SQL-only fixture edits cannot establish journal retirement or authenticated ownership.
Keep both real-journal integration evidence and browser assertions.
Use controlled interruption rather than an actual host OOM.
Run one browser shard at a time and retain every command handle until completion.

## Parallelism

`sequential`

## Inputs

- Interrupted prompt recovery design and the plan's E2E scenario matrix.
- `native_resume_binding_test.go`, `native_resume_terminal_test.go` and related journal retirement tests.
- `apps/web/e2e/helpers/native-resume-notice.ts`.
- `apps/web/e2e/helpers/silent-restart-recovery.ts` and `interrupted-prompt-recovery.ts`.
- `apps/web/e2e/README.md` and the repository E2E skill.

## Results

Added lifecycle coverage with retained bbolt storage and real SQLite recovery transactions. It covers exact, generic, missing-stream and duplicate incident fences. It reopens the journal and retries after a SQL resolution rollback. It preserves unknown outcomes and never dispatches the old prompt.

The desktop and phone specs use the existing Resume control. Both prove that matching fences resolve, historical work is not replayed, native session identity stays the same, and one distinct follow-up prompt is admitted. Both keep an independent restriction blocked.

PR review remediation strengthened the browser evidence. The helper now reads a per-test mock-agent prompt trace, confirms the original prompt ran once, confirms neither historical payload nor the interrupted successor ran during recovery, and confirms the distinct `/slow 1s` follow-up was dispatched exactly once. The independent-restriction case compares the exact seeded recovery-block IDs and consumers after each Resume attempt. The lifecycle fixture source is included in the raw E2E artifact freshness check.

Final E2E results:

- `pnpm e2e:run --host --shards 1 --project chromium tests/session/repeated-native-resume.spec.ts tests/session/native-resume-projection.spec.ts`: 4 passed, including both repeated-resume cases and both projection controls.
- `pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-repeated-native-resume.spec.ts tests/session/mobile-native-resume-projection.spec.ts`: 4 passed, including both repeated-resume cases and both projection controls.
- Both managed runs rebuilt the backend, Vite assets and fixture plugin.
- After review remediation, the desktop repeated-resume spec passed 2 tests, including matching and independent-fence cases.
- After review remediation, the mobile repeated-resume spec passed 2 tests, including matching and independent-fence cases.
- Both remediated browser runs rebuilt the backend, Vite assets and fixture plugin.
- After the final projected-terminal upsert review fix, the desktop and mobile repeated-resume specs were rerun independently and each passed 2 tests.

Other verification passed:

- The lifecycle race command in Verification passed. It ran `TestRepeatedNativeResumeAfterSilentRestoreJournal` and the native-resume and durable-prompt-turn recovery tests.
- Changed-Go-file lint reported `0 issues`. The final command covered every changed Go file:

  ```bash
  bash scripts/lint-go-changed apps/backend/internal/orchestrator/restart_recovery_presentation.go apps/backend/internal/orchestrator/restart_recovery_presentation_test.go apps/backend/internal/orchestrator/repeated_native_resume_test.go apps/backend/internal/task/repository/interface.go apps/backend/internal/task/repository/sqlite/native_resume_binding.go apps/backend/internal/task/repository/sqlite/native_resume_generic_block.go apps/backend/internal/task/repository/sqlite/native_resume_historical_metadata.go apps/backend/internal/task/repository/sqlite/native_resume_after_silent_restore_test.go apps/backend/internal/task/repository/sqlite/restart_recovery_block_ensure.go apps/backend/internal/task/repository/sqlite/restart_recovery_block_ensure_test.go apps/backend/internal/agent/runtime/lifecycle/repeated_native_resume_test.go apps/backend/internal/agent/runtime/lifecycle/e2e_repeated_native_resume_fixture_test.go
  ```
- `pnpm run typecheck` and the listed Prettier check passed.
- `python3 scripts/list-docs.py validate` validated 449 decisions and 1,984 specifications. All 36 spec-linter tests and full spec lint passed.
- The PR-documentation preflight passed with six accepted requirement/design references and no errors. `git diff --check` passed.

PostgreSQL acceptance did not run because `KANDEV_TEST_POSTGRES_DSN` is unset. The lifecycle test reopens retained bbolt and retries after SQL rollback. The browser flow uses a real mock-agent process stop and Resume, with the backend process kept running.
