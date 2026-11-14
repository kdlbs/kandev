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

PR checks and review status will be refreshed after the merge commit is pushed and the requested 15-minute wait completes.
