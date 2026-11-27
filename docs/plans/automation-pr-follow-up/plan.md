---
created: 2026-11-23
status: implemented
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-001
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-003
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-004
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-006
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-007
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-008
  - REQ-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009
  - REQ-OFFICE-AUTOMATION-RUNS-001
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-002
  - REQ-OFFICE-AUTOMATION-CONTINUITY-001
  - REQ-OFFICE-AUTOMATION-RETRIES-001
system_design:
  - ../../specs/office/system-design/automations-pr-follow-up.md
  - ../../specs/office/system-design/automation-run-recorded-events.md
  - ../../specs/office/system-design/automations-settings-04.md
  - ../../specs/office/system-design/automation-retries.md
legacy_specs: []
---

# Implementation Plan: GitHub PR follow-up automation

## Overview

This plan implements [issue #5464](https://github.com/kdlbs/kandev/issues/5464)
and the follow-up decisions from the design review. A `github_pr` automation
gains two events:

- **New commits** (`new_commits`), with an optional quiet period.
- **Review requested** (`review_requested`).

It also gains a `reuse_per_pr` policy. Under that policy a follow-up continues
the PR's task, using an optional follow-up prompt and an optional continuation
step. When the PR is merged or closed, the task is kept, archived, or deleted,
with a separate choice for each case.

Five work orders, in this order:

1. The new-commits event and the shared PR listing and observation layer.
2. The review-requested event, which builds on the observation layer.
3. Per-PR continuation, the busy-task deferral, and the follow-up prompt.
4. The continuation step.
5. End-of-PR actions.

Tasks 1 and 2 each work with the existing policies. Tasks 3 to 5 depend on the
PR-subject mapping that task 3 creates. Each order is a vertical slice that
covers the backend, the editor, the docs, and E2E.

## Scope

### In scope

- The REST PR listing, with pagination, labels, and reviewers. It makes the
  base-branch and label filters effective.
- PR observations: baseline, quiet period, outcome rules, at most one firing
  per PR per poll, and pruning.
- Trigger data and the placeholders `pr.head_sha`, `pr.previous_head_sha`,
  `pr.event`, and `pr.requested_reviewers`.
- The `reuse_per_pr` policy with busy-task deferral, the follow-up prompt, the
  continuation step, and the end-of-PR actions.
- Editor controls in every locale, MCP schema, YAML export, public docs, and
  desktop and phone E2E.

### Out of scope

- The `commented` event and webhook detection.
- **All repos**.
- Per-PR behavior for policies other than `reuse_per_pr` and for other trigger
  types.
- Running the step's own auto-start prompt.

## Technical approach

Follow the [system design](../../specs/office/system-design/automations-pr-follow-up.md).

**Task 01.**

- `internal/github`: add `ListOpenPRs` across the client implementations; the
  `PR` fields `Labels`, `RequestedReviewers`, and `RequestedTeams`; and
  `Service.ListAutomationOpenPRs` and `GetAutomationPR`.
- `internal/automation`: create the `automation_pr_observations` table with
  store methods in a new `store_pr_observations.go`. Add the candidate
  selection and observation update in a new `evaluator_pr_observation.go`, add
  `FireResult.Deferred`, add `QuietPeriodMinutes` validation, and update the
  interpolator and registry.
- Web: the `GitHubPRConfig` checkbox and the quiet-period input.

The REST listing replaces the search call one for one. A per-trigger row is
the smallest state that gives a baseline, the previous head, and the
quiet-period clock.

**Task 02.** Add `Reviewers` to the config, the requested-set diff with team
keys, the review generation and its dedup key, the precedence rule, the
placeholder, and the editor checkbox and reviewers input. This reuses the
task 01 row, adding no new table.

**Task 03.**

- `internal/automation`: add `ContinuationPolicyReusePerPR` and its
  validation, the `automation_pr_continuations` store in a new
  `store_pr_continuations.go`, `PRTaskBusyChecker` injection, the deferral in
  `admitTriggerLocked`, the `FollowUpPrompt` field and column, and the retry
  snapshot fields.
- Orchestrator: add `orchestrator/automation_pr_continuation.go` with
  `AutomationPRTaskBusy` and the per-PR branch, plus prompt selection in
  `createAutomationTaskLocked`.
- `backendapp`: wiring.
- MCP schema, export, and the editor's third card and follow-up prompt.

Reusing `dispatchAutomationContinuation` keeps a single binding and retry
path.

**Task 04.** Add the `PRContinuationStepID` field, its validation, the move
with the start-ownership marker, export `workflow.continuation_step`, and the
step selector.

**Task 05.** Add `PRMergedAction` and `PRClosedAction`, `runEndOfPRActions`,
the injected orchestrator archive and delete methods, export, and the two
selectors with the delete warning.

## ASCII UI preview

The structure is a requirement: control order, visibility by state, and
default values. Spacing is illustrative. Desktop and phone share one stacked
composition. On phone, inputs, cards, and selects span the full width, and
each row is at least 44 px tall.

### UI-01: PR trigger events (trigger editor, `github_pr` trigger)

```text
Events
[x] Opened (i)   [x] New commits (i)   [x] Review requested (i)
Wait until pushes stop for  [ 0 ] minutes          <- shown when New commits is checked
Reviewers  [ kandev-bot, acme/reviewers ]          <- shown when Review requested is checked
           Leave empty to react to any reviewer.
Repositories  [ owner/repo  v ]
Base branches [ main, develop ]   Authors [ alice, bob ]
[o-] Exclude draft PRs
```

Phone: the checkboxes wrap onto separate lines through the existing
`flex-wrap`. The minutes and reviewers inputs take the full width.

### UI-02: Then section, with `reuse_per_pr` selected and the normal-task target

```text
Prompt            [ Review PR #{{pr.number}} ...                     ]
Follow-up prompt  [ New commits since {{pr.previous_head_sha}}:      ]   <- per-PR only
                  [ review only what changed ...                     ]
                  Used when a run continues the PR's existing task.
                  Leave empty to reuse the prompt above.
```

### UI-03: Context between runs, with a `github_pr` trigger

```text
Context between runs
+------------------------------------------------------------------+
| ( ) Start a new task for every run                               |
+------------------------------------------------------------------+
| ( ) Continue the previous session                                |
+------------------------------------------------------------------+
| (o) Continue one task per pull request                           |
|     Each pull request keeps its own task. New commits continue   |
|     that task, and different pull requests can run in parallel.  |
+------------------------------------------------------------------+
Move to step when continuing                 <- normal-task target only
[ Stay in current step                                          v ]
When the pull request is merged
[ Archive task                                                  v ]
When the pull request is closed without merging
[ Delete task                                                   v ]
  ! Deleting removes the task's conversation and files.
```

States:

- With no `github_pr` trigger and a saved policy other than `reuse_per_pr`, the
  third card is hidden.
- With any policy other than `reuse_per_pr`, the follow-up prompt and all three
  selects are hidden.
- With the hidden target, the step select is hidden.
- The end-of-PR selects default to **Keep task**.

Maps to AC-003.x and AC-006.x.

## Tests

| AC | Evidence |
| --- | --- |
| 001.1–001.10 | `automation/evaluator_pr_new_commits_test.go`: baseline, once per head, coalescing, quiet period (wait, reset on new head, fire after expiry, restart persistence), outcome table, one firing per PR per poll, filtered PRs advance, prune only on a complete listing, `opened` unchanged. `store_pr_observations_test.go`: CRUD and cascade. |
| 002.1–002.5 | `evaluator_pr_new_commits_test.go`: `TestPRFiltersApplyBaseBranchLabelsDraftAuthor`. `github/pat_client_open_prs_test.go`: pagination, labels, reviewers, teams. `gh_client_reads_test.go`: paginate. `interpolator_test.go`, `trigger_registry_test.go`. |
| 003.1–003.5 | `components/automations/trigger-configs/github-pr-config.test.tsx`: three checkboxes, value preservation, quiet-period bounds, reviewers parsing. |
| 007.1–007.4 | `automation/evaluator_pr_review_requested_test.go`: new request fires; re-request after removal fires; several reviewers fire once; team and case matching; generation dedup key; precedence when combined with `new_commits`. |
| 004.1–004.10 | `automation/pr_continuation_admission_test.go`: deferral for an active run and a busy checker, checker error counts as busy, no run row, dedup key free, cap still applies. `orchestrator/automation_pr_continuation_test.go`: create and map, resume, replace cases, busy before dispatch fails, non-PR fallback, retry. `service_validation_test.go`, MCP handler test, export test. |
| 008.1–008.4 | `automation_pr_continuation_test.go`: follow-up prompt used only on `resumed`, main prompt otherwise, retry selection. `service_validation_test.go`, `export_service_test.go`. |
| 005.1–005.6 | `automation_pr_continuation_test.go`: move without workflow auto-start, step missing fails, new task starts in the start step, marker released. `service_validation_test.go`, `export_service_test.go`. |
| 009.1–009.6 | `automation/evaluator_pr_end_actions_test.go`: merged versus closed actions; postponed while busy; mapping removed after `keep`; already archived or missing counts as done; partial listing skips. Orchestrator test for archive and delete. Validation and export tests. |
| 006.1–006.5 | `components/automations/automation-continuation-section.test.tsx` and `automation-payload.test.ts`. |

## E2E tests

| Flow | AC | Spec and project |
| --- | --- | --- |
| Configure all three events, the quiet period, and reviewers; save, reload, and see them persisted | 003.1–003.5 | `apps/web/e2e/tests/automations-pr-follow-up.spec.ts` (`chromium`), `apps/web/e2e/tests/mobile-automations-pr-follow-up.spec.ts` (`mobile-chrome`) |
| Select per-PR; set the follow-up prompt, continuation step, and end-of-PR actions; save, reload, and see them persisted; change the workflow and see the step cleared | 006.1–006.5 | Same specs |

Firing behavior has no live GitHub in E2E. Integration tests prove it with
`MockClient` and real stores.

## Work orders

- [x] [Task 01: Fire on new PR commits](task-01-new-commits-event.md)
- [x] [Task 02: Fire when a review is requested](task-02-review-requested-event.md)
- [x] [Task 03: Continue one task per PR](task-03-per-pr-continuation.md)
- [x] [Task 04: Move continued PR tasks to a step](task-04-continuation-step.md)
- [x] [Task 05: Archive or delete PR tasks when the PR ends](task-05-end-of-pr-actions.md)
- [x] [Task 06: Fire `opened` only for new pull requests](task-06-opened-new-prs-only.md)
- [x] [Task 07: Show recorded runs without a reload](task-07-run-recorded-events.md)
- [x] [Task 08: Organize the automation editor](task-08-editor-layout.md)

## Verification results

Recorded on 2026-11-24. Each work order lists its targeted results.

- `make fmt`: no changes beyond this package.
- `NODE_OPTIONS=--max-old-space-size=8192 make typecheck`: passed.
- `make lint-web lint-specs lint-harness lint-architecture`: passed.
- `GOMAXPROCS=8 bash scripts/lint-go-changed <changed Go files>`: three findings fixed; the automation package re-lint reports 0 issues.
- `go test -trimpath ./internal/automation/ ./internal/orchestrator/ ./internal/mcp/... ./internal/github/ ./internal/backendapp/ ./internal/persistence/storeconformance -count=1`: all ok.
- `go run ./cmd/sqlguard ./internal`: passed.
- `pnpm --filter @kandev/web test`: 32,049 passed, 3 failed in `e2e/helpers/seed-repository-origin.test.ts`. Those failures come from this host's git push identity hook and are unrelated to this package.
- E2E: `automations-pr-follow-up.spec.ts` (chromium) 2 passed; `mobile-automations-pr-follow-up.spec.ts` (mobile-chrome) 1 passed.
- `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`: passed.

## Review and live-test fixes

Recorded on 2026-11-24. An independent review session (GPT-6.2 Sol XHigh) and
a live E2E-profile instance with mock GitHub found these issues after the
first commit. All are fixed with regression tests:

- A continued task moved to its continuation step also ran the step's own
  prompt, because the start-ownership marker only guards tasks without a
  session. Found live, and also by the review. The move now uses
  `MoveTaskWithOptions` with `EntryOptions{SkipStepPrompt: true}`.
- A review request after a PR was closed and reopened, after a trigger was
  recreated, or from a second trigger reused a held dedup key and was dropped.
  Found live, and also by the review. The generation is now allocated when the
  request fires, from the automation's existing run keys.
- Continuation read the newest session rather than the primary one, and
  treated read errors as missing resources.
- Delete removed a task that a person had already archived.
- End-of-PR actions used the automation snapshot from before the PR read; they
  now reload it and skip a disabled automation or a changed policy.
- Adding an event to an existing trigger fired for changes made before it was
  selected. Observations now record `tracked_events`.
- Worktree retention could reclaim the checkout of a task mapped to an open PR.
- Phone menu items in the continuation selects were 28 px tall; they are now
  44 px on coarse pointers.
- The new tables are registered as required tables, with schema replay and
  upgrade tests.

Not changed:

- A review finding about ambiguous retry-commit recovery, marked plausible.
  It comes from the shared `reuse_thread` continuation path.
- The window between the busy check and an end-of-PR delete, if a person
  starts a turn at that instant. The delete uses the lifecycle-aware task
  deletion, which stops running sessions.

## Test-phase follow-ups

Manual testing on an isolated instance found three gaps, all fixed in tasks 06
and 07:

- Saving a trigger started a task for every pull request already open. `opened`
  now fires only for pull requests created after the trigger, unless
  **Also run on pull requests already open** is checked.
- Every poll resubmitted each held `opened` key and logged a unique-key warning.
  The evaluator now checks the key before admission.
- The Automations and history pages did not show runs recorded while nothing
  was open. A workspace-scoped `automation.run.created` event refreshes them.

## Risks

- **Rate limits.** The cost is one listing request per 100 open PRs per trigger
  per minute, plus one read for each ended mapped PR. That is the same order as
  today's search.
- **Behavior change.** Base-branch and label filters that were ignored now
  apply to existing `opened` triggers. The public docs will describe this as a
  correction.
- **Delete is destructive.** It is opt-in, defaults to `keep`, and shows a
  warning in the UI. It never runs while the task is busy.
- **Start-ownership marker.** It must be released after every continuation.
  Task 04 tests release on bind, on failure, and through startup
  reconciliation.
- **Retry ledger.** Task 03 must prove that a per-PR retry resolves the same
  task and the same prompt choice and never creates a duplicate.
- **GitHub reviewer semantics.** Submitting a review removes the reviewer from
  the requested list. Only a re-request adds them back, so a re-request fires
  once.
