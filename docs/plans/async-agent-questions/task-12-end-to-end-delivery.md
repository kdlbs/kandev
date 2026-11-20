---
id: "12-end-to-end-delivery"
title: "Prove answers reach agents end to end"
status: done
wave: 10
depends_on:
  - "07-parent-session-routing"
  - "08-agent-mode-guidance"
  - "11-api-and-cleanup"
plan: "plan.md"
requirements:
  - REQ-TASKS-ASYNC-QUESTIONS-003
  - REQ-TASKS-ASYNC-QUESTIONS-004
  - REQ-TASKS-ASYNC-QUESTIONS-005
  - REQ-TASKS-ASYNC-QUESTIONS-006
  - REQ-TASKS-ASYNC-QUESTIONS-009
acceptance_criteria:
  - AC-TASKS-ASYNC-QUESTIONS-003.2
  - AC-TASKS-ASYNC-QUESTIONS-004.1
  - AC-TASKS-ASYNC-QUESTIONS-004.2
  - AC-TASKS-ASYNC-QUESTIONS-005.1
  - AC-TASKS-ASYNC-QUESTIONS-005.2
  - AC-TASKS-ASYNC-QUESTIONS-006.2
  - AC-TASKS-ASYNC-QUESTIONS-006.4
  - AC-TASKS-ASYNC-QUESTIONS-009.1
  - AC-TASKS-ASYNC-QUESTIONS-009.2
  - AC-TASKS-ASYNC-QUESTIONS-009.3
  - AC-TASKS-ASYNC-QUESTIONS-009.4
  - AC-TASKS-ASYNC-QUESTIONS-009.6
system_design:
  - ../../specs/tasks/system-design/async-agent-questions.md
---

# Task 12: Prove answers reach agents end to end

## Summary

Verify the feature's core promise in the running app: an answer given in the UI reaches the agent, and the agent acts on it.
Current specs stop when the Inbox row disappears, so this has not been shown yet.
Any defect found here is fixed in this work order, with a regression test at the right level.

## In scope

- Mock-agent scenarios that emit a marker containing the delivered answer after resuming.
- Parent/child mock scenarios for forwarding, and a withdrawal scenario.
- The E2E flows listed in the [plan](plan.md#e2e-tests), on desktop and phone.
- A run with the Inbox disabled.
- A focused re-run of the backend and frontend suites against current main.

## Out of scope

- Real-agent evaluation of mode choice.
- Backend restart safety, which integration tests own.

## Acceptance

1. An answer from the chat strip produces a mock-agent marker containing that answer, and the same holds after an explicit wait.
2. A forwarded question answered from the parent's chat reaches the child, and a withdrawn question keeps its transcript entry and leaves the strip.
3. All of this passes on current main, on `chromium` and `mobile-chrome`, including with the Inbox disabled.

## ASCII UI preview

These flows exercise UI-04 to UI-07 in [the plan](plan.md#ascii-ui-preview), and add no new views.

```text
UI-04 -> [Submit] -> mock agent: ASYNC_ANSWER_RECEIVED PostgreSQL
```

## Verification

Run from the repository root. In a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -trimpath ./cmd/mock-agent/... -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/async-agent-questions.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-async-agent-questions.spec.ts)
(cd apps/backend && go test -trimpath ./internal/task/questions ./internal/clarification ./internal/mcp/server ./internal/mcp/handlers ./internal/sysprompt ./internal/orchestrator/messagequeue -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run '^TestAsyncQuestions' -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestAsyncQuestions' -count=1)
(cd apps/web && pnpm run typecheck && pnpm run lint)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans/async-agent-questions
```

Record CI evidence that the async PostgreSQL tests ran, not only that their package passed.

## Files likely touched

- `apps/backend/cmd/mock-agent/scenarios.go`, `scenarios_clarification.go`
- `apps/web/e2e/tests/chat/async-agent-questions.spec.ts`
- `apps/web/e2e/tests/chat/mobile-async-agent-questions.spec.ts`
- `docs/plans/async-agent-questions/plan.md` and this work order's Results

## Dependencies

Tasks 07, 08, and 11.

## Risks

- This is the first run of the full delivery path. A failure here may point to a real bug in queue readiness, not a test problem, and fixing it could grow this work order.
- E2E feature flags are profile-managed. The Inbox-off run needs an existing per-spec override, so check how specs under `e2e/tests/auth` set flags. If none fits, cover that case with backend and hook tests, and record the gap.
- Use causal waits as `apps/web/e2e/README.md` requires. Do not budget for the effect with long timeouts.

## Parallelism

`sequential`

## Inputs

- `apps/backend/cmd/mock-agent/AGENTS.md`, and `/e2e` skill guidance.
- [Plan E2E section](plan.md#e2e-tests).
- [Answer reads and queued delivery](../../specs/tasks/system-design/async-agent-questions.md#answer-reads-and-queued-delivery).

## Results

Added mock-agent handling for correlated answer deliveries and E2E assertions for checkpoint reads, later delivery, live wait state, parent/author conversation visibility, rejection, and Inbox-disabled task chat.

Passed:

- `(cd apps/web && pnpm e2e:run --project chromium tests/chat/async-agent-questions.spec.ts)`: 8 tests passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-async-agent-questions.spec.ts)`: 2 tests passed.
- `go test -trimpath ./cmd/mock-agent -run '^TestEmitAsyncQuestionDeliveryResult$' -count=1`: passed, including rejected operator and parent outcomes.
- Backend package, race, SQLite async/pending-action, and named PostgreSQL tests passed. Named PostgreSQL evidence includes `TestAsyncQuestionsPostgresForwardedConversationVisibility` and `TestPostgresAsyncQuestionsParkedGateProjectsPendingAction`.
- Typecheck, lint, i18n check, i18n ratchet, and changed-file Go lint passed.
- `(cd apps/web && pnpm e2e:run --project=chromium tests/git/turn-changes-history-pending.spec.ts --retries=0)`: 1 test passed. Updated the regression to assert retained Git history serves the file while local export is pending; the earlier CI trace showed that the transient-loading assertion could miss this fast fallback.
- PR CI run `41371739136` attempt 1 failed once in the unchanged Quick Chat cancellation E2E after cancel escalation left the next prompt unadmitted. The exact managed test passed locally with retries disabled (1 passed in 18.0s), and rerunning only failed jobs passed on attempt 2. No related code change was needed.
- `scripts/pr-await 4980` confirmed the settled `04fab2bde635e54ee7de933cbfeb9f3eb0f55053` head: 89 passed, 13 skipped, 0 neutral, 0 failed, and 0 pending.
- PR CI runs `41375027965` and `41375027984` attempt 1 on `f082c57c3bc482b18bc7a33c7728e4dedb2bc1e5` initially failed in the unrelated Windows `TestDeliveryStorageSharedSamplerServesMultipleInstances` test and Kubernetes container setup (Docker image builds and Kind load/bootstrap); aggregate checks reflected those leaf failures. The container job log endpoint returned 404, while its test artifacts recorded setup errors. No source change was made for these failures.
- Rerunning only failed jobs (attempt 2) passed `Backend (windows, process 4/4)` and `E2E Containers Shard 1/6`. `scripts/pr-await 4980` confirmed the exact `f082c57c3bc482b18bc7a33c7728e4dedb2bc1e5` head: 89 passed, 13 skipped, 0 neutral, 0 failed, and 0 pending; merge state was CLEAN, with no unresolved review threads.

### Current-base conflict fixup (2026-11-06)

Merged `origin/main` at `08a36b3fe0d49e1dd23007f30fc82ac426a2c1c8` and resolved the conflicts while preserving the async question paths and current-main behavior. The post-merge desktop and mobile runs exposed that the new built-in MCP package catalog did not assign the async question tools to Core. Added `TestAsyncQuestionToolsBelongToCorePackage` and `TestBuiltinCorePublishesAsyncQuestionTools`, then mapped the four async tools to Core.

Passed after that fix:

- `(cd apps/backend && go test -trimpath ./internal/mcp/profile ./internal/mcp/server -count=1)`.
- `(cd apps/backend && go test -trimpath ./internal/clarification ./internal/sysprompt ./internal/mcp/server ./internal/backendapp -count=1)`; backendapp passed in 157.199 seconds.
- `(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='<local PostgreSQL 16 DSN>' go test -trimpath ./internal/task/repository/sqlite -run 'TestAsyncQuestions|PendingAction' -count=1 -v)`; the run passed in 22.274 seconds. Named PostgreSQL cases included `TestAsyncQuestionsPostgresResolutionRace`, `TestAsyncQuestionsPostgresCreateReplay`, `TestAsyncQuestionsPostgresConcurrentCreateAndWait`, `TestAsyncQuestionsPostgresAuthoritySweepStalesReparentedRequest`, `TestAsyncQuestionsPostgresForwardedConversationVisibility`, and `TestPostgresAsyncQuestionsParkedGateProjectsPendingAction`.
- `(cd apps/web && pnpm e2e:run --project chromium tests/chat/async-agent-questions.spec.ts)`: 8 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-async-agent-questions.spec.ts)`: 2 passed.
- `python3 scripts/list-docs.py validate`: 437 decisions and 1930 specifications validated; `python3 scripts/lint-spec-files.py --all`: passed.
- `bash scripts/lint-go-changed` for the three conflict-fixup Go files: 0 issues.

The first changed-package Go lint over the complete PR diff reached its 5-minute timeout after printing `0 issues`; that output is not treated as a successful full lint. Re-running the same full PR-diff scope in five sequential groups of four, four, four, four, and three packages completed successfully, with `0 issues` for every group.

The exact-head PR run on merge commit `a8f19ab0eae075b3fb88a977685762c0b28096b5` settled after the 15-minute strict wait and a further 7m13s all-terminal wait: `scripts/pr-await 4980` reported 89 passed, 13 skipped, 0 neutral, 0 failed, and 0 pending. `scripts/pr-state --compact 4980` confirmed the same head, complete check evidence, and `MERGEABLE` / `CLEAN`; `scripts/pr-resolve list 4980` found no unresolved review threads.

While those checks ran, `main` advanced from `08a36b3fe0d49e1dd23007f30fc82ac426a2c1c8` to `628da65a0e0b48ada18d2e43f8d623e956becdfb`. The two diffs had no overlapping paths. The synthetic merge tree `7bc5dbaacea75692905dfb451a1aaa9131930ff3` was conflict-free, and the detached synthetic commit `819721790a5dfff7a9dcca6c4225df204ec50e75` passed `(cd apps/backend && go test -trimpath ./internal/mcp/profile ./internal/mcp/server -count=1)`.

### Current-base conflict fixup (2026-11-12)

Merged live `origin/main` at `dab0d02e4571c147adf8b9c193db595dbafdd190`. Resolved six conflicts across the MCP output schema, prompt-context construction and public coordination guide, preserving async question behavior and the newer worktree-adoption and image-annotation behavior. Updated `TestAskUserQuestionOutputSchemaAllowsAnnotations` to select the blocking answer branch in the `oneOf` schema.

Passed:

- `(cd apps/backend && go test -trimpath ./internal/mcp/server -count=1)`: passed in 15.296 seconds.
- `(cd apps/backend && go test -trimpath ./internal/task/handlers -count=1)`: passed in 81.949 seconds.
- `(cd apps/backend && go test -trimpath ./internal/orchestrator -count=1)`: passed in 323.353 seconds.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- Changed-file Go lint passed in separate `mcp/server`, `orchestrator`, and `task/handlers` runs, with 0 issues each.
- The combined three-package lint command exceeded its 5-minute context-loading deadline; the printed `0 issues` is not counted as a pass.

The later 2026-11-17 current-head review-fixup results are recorded below.

### Current-base review fixup (2026-11-17)

Merged `origin/main` at `ea6af0d500e92a20420729a505c2b95c65707b36`, resolving the task-chat panel, required-store catalog, and MCP tool-count conflicts while preserving both branches' behavior. The task-transfer Postgres admission-lock fix also addresses the failing CI case `TestPostgresTransferTaskConcurrentCapacityAndChangedKey`.

Passed on the merged tree:

- `(cd apps/backend && go test -trimpath -race ./internal/task/questions -count=1)`.
- `(cd apps/backend && go test -trimpath ./internal/clarification ./internal/sysprompt ./internal/mcp/server ./internal/backendapp -count=1)`; backendapp completed in 196.025 seconds.
- `(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='<local PostgreSQL 16 DSN>' go test -trimpath ./internal/task/repository/sqlite -run 'TestAsyncQuestions|PendingAction' -count=1 -v)`; all selected SQLite and PostgreSQL tests passed, including forwarded conversation visibility and the parked pending-action projection.
- `(cd apps/backend && KANDEV_TEST_POSTGRES_DSN='<local PostgreSQL 16 DSN>' go test -trimpath -race ./internal/task/repository/sqlite -run '^(TestPostgresTransferTaskWaitsForAdmissionLock|TestPostgresTransferTaskConcurrentCapacityAndChangedKey)$' -count=5)`; passed in 52.321 seconds.
- Focused Inbox Vitest regressions: 2 files, 15 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Chromium async-question E2E: 8 passed; mobile Chrome async-question E2E: 2 passed.
- Changed Go lint completed in five sequential package groups, each with `0 issues`.

### Current-head review fixup (2026-11-17)

On exact PR head `e183efc8b5f23d9b94823b77e58c9fcd31467ec3`, CI run `44078107332` attempt 1 reported 90 passed, 16 skipped, and 2 failed. `Run golangci-lint` in `Backend Static Checks` reached its 15-minute internal timeout after printing `0 issues`; the dependent `Run Backend Tests` aggregate failed because of that result. All other checks passed. Rerunning the failed jobs once (attempt 2) passed both checks. `scripts/pr-await 4980` then reported 92 passed, 16 skipped, 0 neutral, 0 failed, and 0 pending. `scripts/pr-state --compact 4980` confirmed the exact head, `MERGEABLE` / `CLEAN`, and 0 unresolved review threads.

- Fixed the Improve test fixture by wiring the workflow-step getter. `TestImproveBootstrapUsesUpgradedInstructions` first reproduced the CI error, then passed after the fixture fix; the full `internal/improvekandev` package passed.
- Repeated `TestRefreshLockRetryBudget/queued_admission_does_not_consume_the_execution_budget` five times locally; all passed. The earlier CI failure did not recur in the current-head backend test shards.
- Added an image-bearing Inbox E2E assertion that the first child of the question fieldset is its `legend`, and moved the legend before the image section. Chromium async-question E2E passed 8 tests (2.2 minutes); mobile Chrome passed 2 tests (11 seconds).
- Published fresh desktop and mobile screenshots under `## Screenshots`. Anonymous GET requests returned `200 image/png` for both embedded assets after temporary upload comments were deleted. The UI screenshot check passed.
- Web typecheck, lint, i18n check, and i18n ratchet passed. Changed-file Go lint over all 111 PR Go files passed with 0 issues. The question race suite, clarification/sysprompt/MCP/backendapp suites, and named SQLite/PostgreSQL async and pending-action cases passed.
- `python3 scripts/list-docs.py validate` validated 456 decisions and 2032 specifications; `python3 scripts/lint-spec-files.py --all` passed.
- Replied to and resolved all 11 review threads on the pushed head.

### Current-base merge fixup (2026-11-20)

Merged live `origin/main` at `5e4c1d3a551d18c9aa25599e42dc676a7db8b2e7` in commit
`7182b90b5291fa22b36d244795471787a7c7ebd5`. Resolved the conflict in
`apps/backend/internal/improvekandev/prompt_upgrade_test.go` by retaining the
service-backed workflow-step getter from `main` alongside the lifecycle getter.
`TestImproveBootstrapUsesUpgradedInstructions` and the full `internal/improvekandev`
package passed.

Passed on the merged tree:

- `(cd apps/backend && go test -trimpath -race ./internal/task/questions -count=1)`.
- `(cd apps/backend && go test -trimpath ./internal/clarification ./internal/sysprompt ./internal/mcp/server ./internal/mcp/handlers -count=1)`.
- `(cd apps/backend && go test -trimpath ./internal/backendapp ./internal/orchestrator/executor ./internal/task/models ./internal/task/service ./pkg/websocket -count=1)`.
- `(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)`.
- `(cd apps/web && pnpm e2e:run --project chromium tests/chat/async-agent-questions.spec.ts)`: 8 passed.
- `(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-async-agent-questions.spec.ts)`: 2 passed.
- `bash scripts/lint-go-changed apps/backend/internal/improvekandev/prompt_upgrade_test.go`: 0 issues.

The first exact-head CI attempt had 85 passed, 16 skipped, and 5 failed. The
`Backend Static Checks` job reached its 15-minute timeout without a lint
diagnostic. E2E shard 27 failed to find a restarted managed agentctl process;
shard 5 timed out waiting for a delivery-journal lock. Rerunning failed jobs
passed. `scripts/pr-await 4980 --mode all-terminal --deadline-min 60` reported
90 passed, 16 skipped, 0 neutral, 0 failed, and 0 pending on exact head
`7182b90b5291fa22b36d244795471787a7c7ebd5`. `scripts/pr-state --compact 4980`
confirmed `MERGEABLE` / `CLEAN`, complete exact-head evidence, and 0 unresolved
review threads; `scripts/pr-resolve list 4980` was empty.

### Current-base merge fixup (2026-11-20, `8fbba47c`)

Merged live `main` at `8fbba47c5e0dec54a520e34fb25164a468f914d8` in commit
`9f7be96abdaaea7cbeaf0c75823913a1a38f0172`. Resolved the conflict in
`apps/backend/internal/orchestrator/messagequeue/types.go` by preserving both
the async-question notification marker and `queueOrderLess` FIFO tie-breaker.
Resolved `docs/public/coverage.json` by retaining the async-question and
message-queue automation source and test entries.

Passed on the merged tree:

- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator/messagequeue ./internal/task/questions -count=1)`; messagequeue passed in 9.639 seconds and task/questions in 16.075 seconds.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- `python3 scripts/list-docs.py validate`: 459 decisions and 2072 specifications validated.
- `bash scripts/lint-go-changed apps/backend/internal/orchestrator/messagequeue/types.go`: 0 issues.
- Commit hooks passed on retry, including changed-package Go lint. The first commit attempt reached the lint hook's 5-minute timeout after printing `0 issues`; a concurrent lint in another worktree also briefly held the shared lint lock.

The merge tree passed `git merge-tree --write-tree origin/main HEAD`; the
resolved paths contain both sides' intended changes in the PR-relative diff.
Exact-head PR CI and reviews are pending the push of this merge commit.
