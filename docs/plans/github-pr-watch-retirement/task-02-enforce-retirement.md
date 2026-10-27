---
id: "02-enforce-retirement"
title: "Enforce retirement and expose resumption"
status: complete
wave: 2
depends_on: ["01-persist-retirement"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.1
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.2
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.3
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.4
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.5
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.6
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.7
  - AC-INTEGRATIONS-GITHUB-PR-WATCH-RETIREMENT-001.8
system_design:
  - ../../specs/integrations/system-design/github-pr-watch-retirement.md
---

# Task 02: Enforce retirement and expose resumption

## Summary

Make public watch deletion persistent and add authorized inventory and resume controls.
Carry retirement through automatic creation, polling, synchronization, and delayed-result paths.

## In scope

- HTTP and WS deletion with discovery-health cleanup after commit.
- Inclusive inventory and stored-target resume endpoint from the design.
- Typed retirement handling after effective-owner resolution.
- Branch refresh, terminal reset, shared batches, REST fallback, passive sync, and explicit sync guards.
- Delayed-result fencing with new watch IDs after resumption.
- Association writes that do not implicitly reactivate retirement.
- Public documentation of DELETE, inventory, resumption, and automation effects.

## Out of scope

New UI controls, native workflow actions, automatic Done cleanup, request counters, and other providers.

## Acceptance

1. DELETE followed by restart and repeated reconciliation produces no active watch or watch-driven request for that target.
2. Authorized resumption restores polling under a new ID. Cross-workspace operations fail. Stale results cannot affect the new watch lifetime.
3. History, associations, CI state, automation options, active shared members, and unrelated targets remain correct throughout the integration flow.

## TDD sequence and cases

Start with `TestPRWatchRetirement_DeleteThenReconcile` using the actual service deletion and poller reconciliation.
It must fail before service wiring with the recreation observed in the plan.
Use actual task/session/environment fixtures for Done, including retained repository attachments.

Add `TestPRWatchRetirement_HTTPResumeAndScope` and `TestPRWatchRetirement_WSDelete` for supported controls.
Exercise HTTP DELETE, three reconciliations, store/service reconstruction, inclusive inventory, resume, and canned provider observation.
Assert ordinary GET excludes retirement, inclusive GET exposes it, and repeated DELETE remains authorized and idempotent.
Resume must reject archived, missing, detached, and cross-workspace targets without provider calls or writes.

Use `TestPRWatchRetirement_PreservesHistoryAndMixedTargets` for searching, numbered, merged, closed, and no-PR watches.
Include retired and active members sharing a provider target, another branch, and another repository.
Only active members receive new observations. Retired TaskPR, CI, and automation values remain intact.

Use `TestPRWatchRetirement_InFlightResults` to hold provider responses across retirement and retirement/resumption.
Cover GraphQL and REST, association creation, feedback publication, terminal resets, and concurrent create/ensure.
Use deterministic channels and provider counters, not sleeps or a live GitHub account.

Use `TestPRWatchRetirement_EffectiveOwner` for shared-worktree redirection and new-session reconciliation.
Cover explicit refresh, passive refresh, push-triggered creation, branch reset, and explicit association calls.
None implicitly resumes a retired target. A distinct branch remains eligible unless it targets the same reserved PR identity.

`TestPRWatchRetirement_TaskLifecycleIntegration` combines real SQLite task, session, repository, and environment fixtures for a retained Completed task and an active Review sibling with HTTP DELETE, repeated poller reconciliation, service reconstruction, provider observations, inclusive inventory, and HTTP resume.

## Verification

Run from the repository root after Task 01 passes.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/github -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'TestPRWatchRetirement_|TestResolvePRWatchBranchForWatch' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/github ./internal/orchestrator -run '^TestPRWatchRetirement_' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
mapfile -t retirement_go_files < <(git ls-files --modified --others --exclude-standard -- apps/backend | rg '\.go$')
if ((${#retirement_go_files[@]})); then bash scripts/lint-go-changed "${retirement_go_files[@]}"; fi
```

Run `bash scripts/lint-go-changed` with every modified Go path. Record the expanded command and actual results.
Run the repository PR-documentation coverage preflight against the final changed paths.

## Files likely touched

- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_retirement.go` (new)
- `apps/backend/internal/github/service_pr_watch_result.go` (new)
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/service_task_events.go`
- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/controller.go`
- `apps/backend/internal/github/handlers.go`
- `apps/backend/internal/github/store_pr_watch_retirement_test.go` (new)
- `apps/backend/internal/github/store_pr_watch_retirement_postgres_test.go` (new)
- `apps/backend/internal/github/pr_watch_retirement_integration_test.go` (new)
- `apps/backend/internal/github/service_pr_watch_workspace_group_redirect_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go`
- `docs/public/integrations.md`
- This plan and work-order results.

## Dependencies

Task 01. The primary conversation owns integration and status updates.

## Risks

Direct callers can bypass active inventory. Audit consumers as well as creation methods.
Retirement must remain separate from PR unlink and automation configuration changes.
Expected retirement skips must not produce repeated errors.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-watch-retirement.md), all acceptance criteria.
- [Design](../../specs/integrations/system-design/github-pr-watch-retirement.md), admission, API, and result fencing.
- Existing in-flight deletion, branch-reconciliation, and workspace-authorization tests.
- `docs/public/README.md` before editing public documentation.

## Results

Completed 2026-10-26.

- Added `TestPRWatchRetirement_TaskLifecycleIntegration`. It uses real persisted task, session, repository, and environment rows for Completed and Review tasks. It proves three reconciliations and service reconstruction preserve retirement, only the Review branch reaches discovery, the ordinary and inclusive HTTP inventories differ correctly, and resume creates a new ID that admits the Completed target.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/github -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'TestPRWatchRetirement_|TestResolvePRWatchBranchForWatch' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/github ./internal/orchestrator -run '^TestPRWatchRetirement_' -count=1)`: passed.
- `make -C apps/backend build`: passed. Darwin packaging emitted the expected warning that no codesign/rcodesign binary is installed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed. `node scripts/validate-public-docs.mjs`: 54 published pages validated.
- `python3 scripts/list-docs.py validate`: 404 decisions and 1742 specifications validated. `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- `.github/scripts/pr-docs.cjs:validateCoverage` over the final changed paths: passed with status `covered` and no errors.
- `bash scripts/lint-go-changed apps/backend/internal/github/pr_watch_retirement_integration_test.go apps/backend/internal/github/service_pr_watch_result.go apps/backend/internal/github/service_pr_watch_retirement.go apps/backend/internal/github/store_pr_watch_retirement.go apps/backend/internal/github/store_pr_watch_retirement_postgres_test.go apps/backend/internal/github/store_pr_watch_retirement_test.go apps/backend/internal/github/controller.go apps/backend/internal/github/errors.go apps/backend/internal/github/handlers.go apps/backend/internal/github/models.go apps/backend/internal/github/poller.go apps/backend/internal/github/poller_test.go apps/backend/internal/github/service_pr_unwatched.go apps/backend/internal/github/service_pr_watch.go apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_fallback_test.go apps/backend/internal/github/service_pr_watch_multi_branch_test.go apps/backend/internal/github/service_pr_watch_workspace_group_redirect_test.go apps/backend/internal/github/service_task_events.go apps/backend/internal/github/store.go apps/backend/internal/github/store_pr_watch_migration_test.go apps/backend/internal/github/store_task_pr_ownership.go apps/backend/internal/github/store_watch_reset_test.go apps/backend/internal/orchestrator/event_handlers_git.go apps/backend/internal/orchestrator/event_handlers_github.go apps/backend/internal/orchestrator/event_handlers_github_test.go`: 0 issues.

## Review follow-up results

Addressed the three review findings. Branch admission now checks a numbered watch's branch retirement independently of its PR number, while preserving the separate numbered-PR reservation. Result transactions stage TaskPR and feedback events as snapshots and publish them after commit while the watch lifecycle lock serializes delivery against delete and resume. Feedback publication also detects a newer PR `updated_at` watermark independently of TaskPR semantic changes.

- `go test -timeout=5m -trimpath -tags fts5 ./internal/github -count=1`: passed in 79.708s.
- `go test -timeout=2m -trimpath -tags fts5 ./internal/orchestrator -run '^(TestDetectPushAndAssociatePR|TestEnsureSessionPRWatch_MultiRepo|TestListTasksNeedingPRWatch)$' -count=1`: passed.
- `go test -timeout=90s -race -trimpath -tags fts5 ./internal/github -run '^(TestPRWatchRetirement_NumberedBranchBlocksExistingSearchingSibling|TestPRWatchCommentOnlyUpdatesPublishFeedbackOnce|TestPRWatchResultEventsPublishAfterCommit|TestPRWatchResultCommitFailurePublishesNoEvents|TestPRWatchRetirementWaitsForCommittedResultPublication)$' -count=1`: passed.
- `make -C apps/backend build`: passed. Darwin packaging emitted the expected warning that no codesign/rcodesign binary is installed.
- `git diff --check`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/github/controller.go apps/backend/internal/github/errors.go apps/backend/internal/github/handlers.go apps/backend/internal/github/models.go apps/backend/internal/github/poller.go apps/backend/internal/github/poller_test.go apps/backend/internal/github/pr_watch_retirement_integration_test.go apps/backend/internal/github/service_automation_auth.go apps/backend/internal/github/service.go apps/backend/internal/github/service_pr_unwatched.go apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_fallback_test.go apps/backend/internal/github/service_pr_watch.go apps/backend/internal/github/service_pr_watch_multi_branch_test.go apps/backend/internal/github/service_pr_watch_result_events_test.go apps/backend/internal/github/service_pr_watch_result_feedback_test.go apps/backend/internal/github/service_pr_watch_result.go apps/backend/internal/github/service_pr_watch_retirement.go apps/backend/internal/github/service_pr_watch_workspace_group_redirect_test.go apps/backend/internal/github/service_task_events.go apps/backend/internal/github/store.go apps/backend/internal/github/store_pr_watch_migration_test.go apps/backend/internal/github/store_pr_watch_retirement.go apps/backend/internal/github/store_pr_watch_retirement_postgres_test.go apps/backend/internal/github/store_pr_watch_retirement_test.go apps/backend/internal/github/store_task_pr_ownership.go apps/backend/internal/github/store_watch_reset_test.go apps/backend/internal/orchestrator/event_handlers_git.go apps/backend/internal/orchestrator/event_handlers_github.go apps/backend/internal/orchestrator/event_handlers_github_test.go`: 0 issues.

## PR review follow-up results

Completed 2026-10-26. Regressions reproduced all three findings before their
fixes, then passed after implementation:

- Different numbered PRs on the same task/repository/branch keep distinct
  watches, and push discovery associates the second PR with its own watch.
- A terminal result keeps an existing searching watch and removes the
  redundant numbered watch in the same transaction, preserving the final
  TaskPR state.
- Discovery reconciles the comparison target after committing the TaskPR row,
  including fork-backed PRs with a replacement base branch.

Validation passed:

- `go test -trimpath -tags fts5 ./internal/github -run '^(TestCreatePRWatch_DoesNotReuseDifferentPROnSameBranch|TestTerminalPRWatchResultPreservesSearchingSibling|TestDiscoveredPRReconcilesComparisonTargetAfterCommit)$' -count=1`.
- `go test -timeout=5m -trimpath -tags fts5 ./internal/github -count=1` (115.154s).
- `go test -timeout=2m -trimpath -tags fts5 ./internal/orchestrator -run '^(TestDetectPushAndAssociatePR|TestEnsureSessionPRWatch_MultiRepo|TestListTasksNeedingPRWatch)$' -count=1`.
- `go test -timeout=90s -race -trimpath -tags fts5 ./internal/github -run '^(TestCreatePRWatch_DoesNotReuseDifferentPROnSameBranch|TestTerminalPRWatchResultPreservesSearchingSibling|TestDiscoveredPRReconcilesComparisonTargetAfterCommit)$' -count=1`.
- `bash scripts/lint-go-changed apps/backend/internal/github/service_pr_watch.go apps/backend/internal/github/service_pr_watch_result.go apps/backend/internal/github/service_pr_watch_review_findings_test.go`: 0 issues.
- `make -C apps/backend build` passed with the expected Darwin codesign-tool warnings.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and the PR documentation coverage preflight passed.
- `git diff --check` passed.

## Additional PR fixup result

Addressed CodeRabbit's legacy-watch resume finding. `validateTaskRepositoryID`
accepts an empty repository ID for legacy watches, so resume eligibility now
allows that stored identity through while continuing to validate task existence,
workspace ownership, archive state, and repository membership for non-empty
IDs. Added an HTTP regression that retires and resumes a legacy watch with an
empty repository ID.

- The new regression failed before the fix with HTTP 409, then passed after it.
- `go test -timeout=5m -trimpath -tags fts5 ./internal/github -count=1`: passed.
- `go test -timeout=90s -race -trimpath -tags fts5 ./internal/github -run '^TestPRWatchRetirement_HTTPResumeLegacyEmptyRepositoryID$' -count=1`: passed.
- `make -C apps/backend build`: passed; Darwin packaging emitted the expected missing codesign-tool warnings.
- `bash scripts/lint-go-changed apps/backend/internal/github/service_pr_watch_retirement.go apps/backend/internal/github/store_pr_watch_retirement_test.go`: 0 issues.

## E2E CI fixup results

The E2E run for PR head `9cc174a` reported four flaky tests across 4,265
executed tests; 40 tests were skipped. Its blob audit found the same four
failed attempts and no report parse errors.

- Azure saved-view setup now waits for the configuration save response. A
  deliberately delayed save reproduced the 503 before the wait and passed
  four retry-free runs afterward.
- The restart test drains and holds individual session reads while testing
  list hydration. The original missing-activity failure reproduced locally,
  and the corrected test passed eight retry-free runs.
- Profile rows retain their open menu when another profile is added or
  removed. The removal case failed before the change; tablet and phone
  regressions now cover the update, with phone duplicate and touch ordering
  paths also exercised.
- Pseudo-locale setup installs its cookie before the first navigation and
  checks the document request. The original CI symptom did not reproduce in
  local stress runs; the first-request cookie assertion failed before the
  setup change, and the complete preceding-test sequence passed 54
  retry-free cases afterward under a two-CPU, 4 GB container.

Validation also passed for the affected component's 19 Vitest tests, changed
frontend lint, E2E sleep lint and ratchet, and frontend typecheck with
`NODE_OPTIONS=--max-old-space-size=8192`. The default 4 GB Node heap was
insufficient for the typecheck.

- The rebuilt desktop E2E selection passed 9 tests with retries disabled;
  the phone duplicate, deletion, and touch-order selection passed 6.
- With two CPUs and 4 GB of memory, the CI-neighbor restart sequence passed
  12 tests and the delayed-save Azure sequence passed 8, all without retries.
- The same resource limits and retry-free settings produced 34 passing tests
  in the profile sequence, including the CI predecessor specs.

### Further E2E results

The next CI head exposed three more retry failures. A delayed agent-discovery
transition reproduced the tablet menu dismissal locally: the configured and
then discovered versions of an agent used different card keys. Both now use
the agent name, and the new transition test failed before the fix and passed
with the existing menu tests in nine retry-free runs. The earlier profile-count
remount fix remains covered by its own regression.

The phone passthrough fixture now waits for the initial session to become idle
before submitting a follow-up draft. The directory-move fixture waits for the
initial turn and Git hydration, then for Git to report all moved paths before
reopening Files. These two exact CI failures were not reproduced locally before
the waits were added. Afterward, five phone repetitions, four directory-move
repetitions, and the complete three-test drag-and-drop spec passed with retries
disabled. The affected agent page and profile component suites passed 23
Vitest tests, and changed-file ESLint and frontend typecheck passed.

A later shard failed because the first agent had no profiles. The profile
settings tests now find the fixture's seeded profile by ID; a unit regression
covers a profile-less agent preceding it. The desktop duplication spec and
mobile deletion/duplication specs each passed five tests without retries.

A Kubernetes UID-protection test also reported a closed backend socket in CI;
its artifacts do not identify the request or cause. One local Kind-backed run
passed with retries disabled after rebuilding the backend and Linux helpers
together, but that does not establish why CI failed. The original local attempt
used mismatched binary identities and was stopped after that setup error.

The tablet MCP drawer test also saw a fractional geometry value of
`800.000061` at an 800-pixel limit. Its assertion now waits for finite
animations and allows one pixel for rendering precision at the 80vh bound.
The focused test passed five retry-free runs; the exact rounding was not
reproduced locally. After resolving the current-main merge, the selected
desktop and mobile E2E suites each passed ten tests without retries.

A later mobile transcript test observed a 29-pixel difference after disabling
auto-scroll. The assertion now records the position at disable time, matching
the scroll owner's frozen-position contract, instead of a potentially clamped
position after prompt submission. The previous assertion passed eight local
runs, so the exact CI timing was not reproduced; the revised assertion also
passed eight retry-free runs.

The current-main merge preserved both versions' seeded-profile ownership check
in a shared helper. Its unit tests cover a profile-less first agent and
ambiguous ownership; the mobile profile deletion and duplication specs passed
five tests without retries. The merged GitHub backend package suite and web
typecheck passed.

The completed CI run at `9aa75eff` had one flaky canvas message test among
4,280 executed tests. Its HTTP 500 response did not expose the underlying
error. The exact failure did not reproduce in six isolated runs or three
replays of its three-test predecessor sequence under two CPUs and 4 GB.
The canvas test now waits for the task move's lifecycle marker to clear before
exercising its separate message action. Move completion changes the task
version asynchronously and can race message admission, although the generic
CI response does not prove that was the observed cause. Bounded server logs
now record the failure phase and gRPC code for internal canvas message errors
without recording prompts or downstream error text.
After the lifecycle wait and bounded diagnostics, the constrained three-test
canvas sequence passed nine cases without retries. The plugin package suite,
changed-Go lint, frontend typecheck, and E2E sleep checks passed.

The next CI run exposed two additional flaky tests. The scroll fixture now
waits for each submitted prompt's own persisted turn to finish before seeding
history; the failure screenshot showed an agent still running after the old
idle check. The exact failure did not reproduce in eight constrained baseline
runs. The revised helper passed eight mobile cases and two desktop cases
without retries. The mobile canvas marketplace now uses the existing
precision-aware 44-pixel touch-control assertion; it passed five retry-free
runs after CI measured a target at 43.99994 pixels.

### Rebase onto current main

The branch was rebased onto a main that now carries GitHub Enterprise host
support in the same watch code. Resolutions keep both sides.

- Watch creation and ensure keep main's resource-mutation lock, selected-host
  resolution, and attached-repository host admission, and now also check
  retirement before reusing a canonical watch. `CreatePRWatch`'s transactional
  insert and the resumption lifetime insert persist `github_host`.
- The poller keeps main's operation fence, resource lock, and host validation
  before delegating to the transactional result helpers, so retirement
  admission and post-commit event publication still own the apply step.
- Batched workspace sync validates each watch's host before filtering inactive
  and retired watches, and forwards main's operation fence with the branch's
  feedback-publication flag.
- `SyncTaskPR` keeps main's staleness check and host admission in the
  lock-taking prepare step; the transactional prepare/apply split from this
  branch is unchanged.
- On-demand status sync keeps the branch's resolve-only owner lookup because
  the result transaction performs ownership reconciliation, and regains main's
  host validation and expected-host context.
- Departed-from-main test fixtures now persist the watch they act on and give
  the fake task store the repository row that host admission reads.

The failing E2E shard selected `agents[0]` and required it to own profiles, so a
virtual family sorted first failed the test. The desktop and mobile
profile-layout specs now resolve the seeded profile's owning agent by ID
through `seededAgentProfile`. The shared helper keeps its exactly-one-owner
invariant and both of its unit cases.

Local validation after the rebase:

- `go test -timeout=10m -trimpath -tags fts5 ./internal/github -count=1`: passed.
- `go test -count=1 -timeout=40m ./internal/orchestrator/`: passed in 561s. The
  default 10m package timeout expired under concurrent local load, and the
  targeted retirement and push-association selection passed on its own.
- `go test -timeout=2m -trimpath -tags fts5 ./internal/orchestrator -run '^(TestDetectPushAndAssociatePR|TestEnsureSessionPRWatch_MultiRepo|TestListTasksNeedingPRWatch|TestPRWatchRetirement_.*|TestResolvePRWatchBranchForWatch.*)$' -count=1`: passed.
- `make -C apps/backend build`: passed.
- `bash scripts/lint-go-changed` over every modified Go file: 0 issues.
- `pnpm vitest run e2e/helpers/seeded-agent-profile.test.ts`: 2 passed.
- Changed-file ESLint, `pnpm run e2e:sleep-ratchet`, and `pnpm exec tsc --noEmit`: passed.
