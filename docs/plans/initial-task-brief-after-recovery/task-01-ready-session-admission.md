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
(cd apps/backend && go test -tags fts5 ./internal/task/handlers -run '^TestWSAddMessage_InitialTaskBriefReadySession$' -count=1 -v)
(cd apps/backend && go test -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/executor -run 'InitialTaskBrief|WSAddMessage|StartCreatedSession|InitialPromptFallback|SessionOpenRecoveryStatusAndLaunch|ResumeSession|HasUserPromptHistory' -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite -run 'InitialTaskBrief|ConcurrentInitialBrief|HasUserPromptHistory' -count=1)
(cd apps/backend && go vet ./internal/task/handlers ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/executor)
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

The ready prompt now passes the exact acceptance-time saved-prompt context,
prepared-state bit (including an accepted empty snapshot), and validated entity
references into `PromptTaskWithPromptContext`. Missing-runtime recovery carries
those values into the fresh launch, where the executor prompt retains each
accepted block once after the definitions have changed. First-boundary commit
and queue admission share the session admission lock. A selected prompt owns a
dispatch marker until its delivery exits; contenders defer only while that
owner remains active, and completion retries the real queue drain.

The concurrent handler capture now guards its preparation inputs with a mutex.
Its test name includes `InitialTaskBrief`, and the recorded race selector also
includes `HasUserPromptHistory` so the service read-error test is exercised.

Remediation verification passed:

- Focused backend tests across handlers, service, SQLite, orchestrator, and
  executor, including `TestPromptTask_InitialTaskBriefAfterRecovery`, accepted
  empty context, and both queue-ownership schedules.
- Race tests across handlers, service, and SQLite using
  `InitialTaskBrief|ConcurrentInitialBrief|HasUserPromptHistory`.
- `go vet` for all five affected backend packages.
- `make -C apps/backend build` and `git diff --check`.

PostgreSQL parity remains unverified because `KANDEV_TEST_POSTGRES_DSN` is
unset; no skipped run is counted as a pass.
