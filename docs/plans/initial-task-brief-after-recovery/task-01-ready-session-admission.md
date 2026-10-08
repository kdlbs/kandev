---
id: "01-ready-session-admission"
title: "Preserve the first brief for ready sessions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-INITIAL-TASK-BRIEF-001
acceptance_criteria:
  - AC-TASKS-INITIAL-TASK-BRIEF-001.1
  - AC-TASKS-INITIAL-TASK-BRIEF-001.2
  - AC-TASKS-INITIAL-TASK-BRIEF-001.3
  - AC-TASKS-INITIAL-TASK-BRIEF-001.4
  - AC-TASKS-INITIAL-TASK-BRIEF-001.5
  - AC-TASKS-INITIAL-TASK-BRIEF-001.6
  - AC-TASKS-INITIAL-TASK-BRIEF-001.8
  - AC-TASKS-INITIAL-TASK-BRIEF-001.9
  - AC-TASKS-INITIAL-TASK-BRIEF-001.10
  - AC-TASKS-INITIAL-TASK-BRIEF-001.11
  - AC-TASKS-INITIAL-TASK-BRIEF-001.12
system_design:
  - ../../specs/tasks/system-design/initial-task-brief.md
---

# Task 01: Preserve the first brief for ready sessions

## Summary

Allow a never-prompted ready ordinary session to prepare the same initial brief
candidate as a CREATED session. Reuse atomic admission and dispatch the committed
result through the existing ready-session prompt/resume path.

## In scope

- Start with `TestWSAddMessage_InitialTaskBriefReadySession`: WAITING_FOR_INPUT,
  nonempty brief, no accepted/reserved history, provider conversation metadata
  and lifecycle-only boot. Assert both persisted content and captured ordinary
  dispatch, with zero created-session starter calls. Run it red before editing production.
- Add a narrow service history lookup over `MessageRepository.HasUserPromptHistory`.
  Treat read failure as admission failure; skip preparation for already-prompted
  ready recipients. Preserve the existing CREATED/redirection rules.
- Separate initial-brief eligibility from `startCreatedSession`. Keep the
  repository transaction authoritative under simultaneous sends or fallback claims.
- Add `TestWSAddMessage_InitialTaskBriefReadySessionAdmission` and service coverage
  for the history lookup/error boundary. Cover follow-ups, deletion, restart,
  zero reservations, read failure, rollback, same-ID retry, and concurrent sends.
- Extend structured/passthrough and excluded-kind table cases, saved-expansion
  snapshots (including accepted-empty context), attachments/references, plan
  mode, and selected/unselected feedback-queue delivery on a ready recipient.
- Add `TestPromptTask_InitialTaskBriefAfterRecovery` against real orchestration,
  including the missing-runtime resume seam. Keep prompt-free recovery and
  already-composed delivery intact.
- Extend SQLite admission and PostgreSQL multi-connection cases for ready-state
  first-boundary contention without changing the counter schema.

## Out of scope

No recovery auto-prompt, terminal-session permission change, browser layout,
provider branch, migration, runtime flag, historical backfill, or new scheduler.

## Acceptance

- The named primary regression fails on the original code because only the
  instruction is saved/dispatched, then passes with both texts once through
  ordinary ready delivery and its resume path, without relaunching an existing agent.
- A consumed or reserved boundary preserves normal follow-up behavior; concurrent
  first sends/fallbacks select one winner, and errors/retries retain transactional
  content, queue, and saved-context ownership.
- Existing CREATED, redirect, workflow-template, excluded-kind, and prompt-free
  session-open tests pass alongside the new ready-state and database cases.

## Verification

Run from the repository root. Use `/tdd`; record the exact red failure before
the correction and then run the complete final block. New names below are to
be introduced by this work order.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/executor ./internal/backendapp -run 'InitialTaskBrief|WSAddMessage|StartCreatedSession|InitialPromptFallback|SessionOpenRecoveryStatusAndLaunch|ResumeSession|HasUserPromptHistoryDelegatesAndReturnsReadErrors|SessionScopingHasUserPromptHistory|OrchestratorWrapperExposesInitialTaskBriefRecoveryContract' -count=1)
(cd apps/backend && go test -race -trimpath -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite -run 'InitialTaskBrief|ConcurrentInitialBrief|TestHasUserPromptHistoryDelegatesAndReturnsReadErrors|TestSessionScopingHasUserPromptHistory' -count=1)
(cd apps/backend && go vet ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/executor ./internal/backendapp)
# Configure KANDEV_TEST_POSTGRES_DSN for a disposable test database first.
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -tags fts5 ./internal/task/repository/sqlite -run '^TestInitialTaskBriefAdmissionPostgres$' -count=1)
git diff --check
```

If PostgreSQL is unavailable, record that command as blocked and the parity test
as unverified. Do not count an environment-driven skip as a pass.

## Files likely touched

Owned production paths:

- `apps/backend/internal/task/handlers/message_handlers.go`
- `apps/backend/internal/task/handlers/message_handlers_initial_task_brief.go`
- `apps/backend/internal/task/service/service_messages.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`

Owned test paths:

- `apps/backend/internal/task/handlers/message_handlers_initial_task_brief_test.go`
- `apps/backend/internal/task/handlers/message_handlers_saved_prompt_test.go`
- `apps/backend/internal/task/service/service_initial_task_brief_history_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/message_initial_task_brief_test.go`
- `apps/backend/internal/task/repository/sqlite/message_initial_task_brief_postgres_test.go`
- `apps/backend/internal/orchestrator/prompt_launch_fallback_test.go`
- `apps/backend/internal/orchestrator/session_open_recovery_launch_test.go`

Inspect existing queue/context seams in `task_operations.go`,
`task_create_prompt.go`, and `queued_dispatch.go`. Change these only if the
named ready-delivery regressions expose a necessary integration defect; retain
existing launch/queue ownership. The repository selector is an existing dependency,
not an instruction to rewrite it.

## Dependencies

None. Read the current owning requirement/design and package evidence before implementation.

## Risks

- A false history preflight does not claim the first boundary.
- A non-selected candidate must not permanently defer ordinary later messages.
- A provider conversation ID is compatible with no accepted input.
- Stale-description retry must preserve the original instruction and request fingerprint.
- Saved context must not be expanded again after acceptance.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/initial-task-brief.md).
- [Design](../../specs/tasks/system-design/initial-task-brief.md).
- [Evidence and test matrix](plan.md).
- Existing `TestWSAddMessage_InitialTaskBrief`, handler capture fakes,
  `TestInitialTaskBriefAdmission`, and `TestSessionOpenRecoveryStatusAndLaunch`.
- [Session-open policy](../../decisions/2026-09-18-session-open-resumes-conversation.md).

## Results

Red evidence: before the production edit, `TestWSAddMessage_InitialTaskBriefReadySession`
failed as expected. The persisted prompt contained only the additional instruction,
not the task brief. The regression passed after the ready-session admission path
was added.

Final checks passed:

- Focused handler, service, SQLite, orchestrator, and executor tests with `-tags
  fts5`, including the ready-session and missing-runtime fallback regressions.
- Handler and SQLite initial-brief race tests with `-race`.
- `go vet` for the affected handler, service, repository, orchestrator, and
  executor packages.
- `make -C apps/backend build`.

PostgreSQL parity remains unverified: `KANDEV_TEST_POSTGRES_DSN` is unset in
this environment, so the disposable-database test was not run.

### Review remediation results (2026-10-07)

The fallback regression was first run against the reviewed implementation and
failed at the real executor launch: canonicalization removed the accepted saved
prompt expansion. The queue regression also failed before the fix: a contender
queued after the winning turn left queue count at 1, with no drain worker.
After the regression exercised the handler's compound resume retry, it exposed
a lifecycle-lock re-entry at fresh launch. Recovery now carries its existing
lock ownership into that launch, and the real fallback regression completes.

The production orchestrator adapter now exposes the context-aware prompt and
first-boundary coordination methods used by the handler. Ready delivery carries
the exact acceptance-time saved-prompt context, prepared-state bit (including
an accepted empty snapshot), and validated entity references. The compound
resume retry carries the same values into fresh-runtime fallback, where the
real executor launch retains each accepted block exactly once after the saved
prompt definition changes. Legacy `PromptTask` callers keep their existing
composition behavior. First-boundary message admission and enqueue coordinate
under the session admission lock; queue and direct-prompt gates preserve the
selected dispatch owner and completion retries the real queue drain.

The concurrent handler capture now guards its preparation inputs with a mutex.
Its test name is `TestWSAddMessage_InitialTaskBriefConcurrentReadySessionQueuesUnselectedCandidate`,
which is matched by the recorded selector. The race selector names both
`TestHasUserPromptHistoryDelegatesAndReturnsReadErrors` and
`TestSessionScopingHasUserPromptHistory` explicitly.

Remediation verification passed on 2026-10-07:

- `go test -trimpath -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/executor ./internal/backendapp -run 'InitialTaskBrief|WSAddMessage|StartCreatedSession|InitialPromptFallback|SessionOpenRecoveryStatusAndLaunch|ResumeSession|HasUserPromptHistoryDelegatesAndReturnsReadErrors|SessionScopingHasUserPromptHistory|OrchestratorWrapperExposesInitialTaskBriefRecoveryContract' -count=1` from `apps/backend`.
- `go test -race -trimpath -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite -run 'InitialTaskBrief|ConcurrentInitialBrief|TestHasUserPromptHistoryDelegatesAndReturnsReadErrors|TestSessionScopingHasUserPromptHistory' -count=1` from `apps/backend`; this includes the renamed ready-session concurrency regression and both prompt-history tests.
- `go vet ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/executor ./internal/backendapp` from `apps/backend`.
- `golangci-lint run ./... --new-from-rev=8feffe1e17fd5ac1b079fc52469bdfff780e5ad0 --timeout=5m` from `apps/backend`; 0 issues.
- `make -C apps/backend build` and `git diff --check`.

The first CI attempt after review remediation exposed
`TestPromptTask_QueuedAcceptedTurnIdentityReadFailurePreservesExecution`.
Local full-package reproduction showed that the direct-prompt guard was
treating any accepted queued turn as a pending initial brief. The guard now
checks only the dedicated first-brief marker; queue-drain paths keep their
separate accepted-dispatch checks. The regression, the full orchestrator
package, the focused work-order suite, and the focused race suite pass after
this correction. Fresh remote checks for the resulting commit remained the
delivery gate at the time; see the PR E2E triage below.

PostgreSQL parity remains unverified because `KANDEV_TEST_POSTGRES_DSN` is
unset; no skipped run is counted as a pass.

### PR E2E triage results (2026-10-08)

The PR check run `37688349104` on head `8e296c1d0d0046208654700214c4fb504da63d70`
failed E2E shards 3 and 11. The mobile case retained `/sleep 30` after its
direct send; the desktop case retained the accepted queued follow-up after its
queue-add response was dropped. Both cases set their local retry count to zero.

On a synthetic merge against the then-current base
`c40f6d96d726b8ff765c038ceabacf50405ba9be`, the desktop case passed in isolation
and at its original shard position (23.6s). The retry-disabled mobile project
replay of shard 3 passed 58 tests with one skip; the failed case passed at
position 6 (22.6s). The desktop shard replay passed the queue case at position
24 but did not produce a clean shard result (276 passed, 1 skipped, 1 failure):
a newer mobile fork-comparison test failed because the Docker container could
not resolve the linked worktree's host `.git` pointer. The current catalog had
also moved past the downloaded CI manifest, so the assigned file list was
replayed directly.

At the time, the latest base was `db0348d1623ea9dc06e28c85afbcba9e4c087c06`;
the files changed since `c40f6d96d726b8ff765c038ceabacf50405ba9be` did not
overlap the PR's changed files. On synthetic merge
`d5e946c24cf5b38b7cf2b948185b817195556d63` (head
`8e296c1d0d0046208654700214c4fb504da63d70`), the desktop case
`reconciles without duplicating a queued message` passed in 22.2s and the mobile
case `reconciles through a touch submit` passed in 22.4s (2 passed, 49.6s,
zero retries). Docker was unavailable, so the focused replay used host mode.

During the subsequent full PR check run, `main` advanced to
`1257838968f5c92305a858427cdf723a87b0a882`; the files changed since
`db0348d1623ea9dc06e28c85afbcba9e4c087c06` did not overlap the PR's changed
files. On synthetic merge `8ab712b5b58889b41a335d5a9977dcd30ba68475` (head
`b6b3fd7ff488abfba3529d3031de27f3017160a1`),
`reconciles without duplicating a queued message` passed in 17.5s and
`reconciles through a touch submit` passed in 22.9s (2 passed, 45.3s, zero
retries). Docker was unavailable, so the replay used host mode. These results do
not replace exact-head PR checks. PostgreSQL parity remains unverified.
