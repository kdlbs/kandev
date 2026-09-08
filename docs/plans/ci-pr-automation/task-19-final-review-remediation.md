---
id: "19-final-review-remediation"
title: "Final review remediation"
status: done
wave: 11
depends_on:
  - "18-lifecycle-prompt-security-remediation"
plan: "plan.md"
spec: "../../specs/ui/requirements/ci-pr-automation.md"
---

# Task 19: Final review remediation

## Acceptance

- Lifecycle queued dispatch performs `on_turn_start`, runtime resume, and model
  preparation before its final claim. Under the per-session cancel guard, that
  final claim verifies the current queue token, resolves the current selected
  task session, and atomically claims it only while the task is active.
- The active-task claim has explicit `claimed`, `busy`, and `inactive`
  outcomes. Busy and transient claim failures retain a durable coalesced retry;
  only a missing or inactive task discards it. An active task with no currently
  promptable session retains the original event. The selected primary session
  is eligible for immediate lifecycle delivery from either `IDLE` or
  `WAITING_FOR_INPUT`.
- If the guarded claim updates no row, it re-reads the task/session inside the
  same transaction. An active session that became non-promptable is `busy` and
  retained for retry; a missing or archived task/session is `inactive` and
  discarded.
- After a successful claim, Kandev reconciles the task to `IN_PROGRESS` and
  publishes the session's `RUNNING` transition before persisting the visible
  lifecycle message or calling the executor.
- Final session resolution follows task-level selection. If another session
  becomes selected after the event was accepted, the event is durably requeued
  to that session rather than discarded or dispatched through the stale one.
- A claim records the session's pre-claim state. A reset or superseded token
  after a successful claim restores that state and requeues the entry; a failed
  post-claim dispatch likewise restores then retries. The visible user message
  is created only after the final claim succeeds, so no losing path creates a
  duplicate or stale chat message.
- Lifecycle dispatch captures its rollback state before the PTY/executor
  handoff. An executor failure uses that captured lifecycle rollback rather
  than a newly observed session state, then retains the row for retry.
- Visible lifecycle message persistence is itself part of acceptance. If it
  fails after claim, Kandev restores the pre-claim session state, closes a turn
  only when that dispatch created it, requeues the event, and never calls the
  executor for that failed attempt. Task-state rollback uses a compare-and-set
  from `IN_PROGRESS`, so it cannot overwrite a concurrent terminal transition
  or archive. A pre-existing turn remains open.
- Queue ownership reserves agent-, workflow-, and server-owned entries for
  backend dispatch. Browser and MCP clients may create and mutate only
  user-owned entries and cannot spoof a reserved identity to alter lifecycle
  work.
- Ordinary queued messages retain destructive dequeue. Lifecycle rows use
  `ReserveHead` and remain durable until `AcknowledgeByID` after executor
  prompt acceptance. A per-session in-process guard prevents concurrent
  duplicate dispatch; restart before acknowledgement redelivers, while the
  PTY/external-acceptance-before-ack crash window may duplicate under the
  explicit at-least-once contract.
- A passthrough lifecycle reservation leaves the ready-handler guard before it
  invokes the ordinary lifecycle dispatcher. It claims the durable row's
  in-flight token while it still holds that guard; the deferred dispatcher
  consumes the preclaimed token, preventing another in-process drain from
  claiming the same reservation during the deferral. It therefore uses the
  same final claim, visible-message, retry, and acknowledgement path as ACP
  delivery; only ordinary passthrough entries retain direct stdin delivery.
- Active failures retain or coalesce the durable lifecycle row; inactive
  outcomes acknowledge and discard it. Session reselection inserts or
  coalesces the target row before acknowledging the source reservation.
- The active-task archive guard applies to initial lifecycle queue acceptance
  and every retry insertion. Archiving between dequeue and retry insertion
  discards the retry, including if the task is later unarchived.
- Archive/delete privileged queue cleanup purges lifecycle rows even when they
  are reserved. A task-queue generation invalidates accepted, reserved, and
  in-flight lifecycle work, preventing a stale cancellation/retry path from
  reappearing after an archive/unarchive cycle.
- The supplied SQLite queue is purged and generation-advanced transactionally
  with task archive/delete. The post-commit callback mirrors cleanup only to a
  registered or fallback ephemeral queue, never to that persistent queue.
  Workspace cascade deletion performs this cleanup for every captured task in
  its transaction and, after commit, sends exactly one mirror notification per
  captured task.
- Lifecycle success cannot erase a same-pass auto-fix or auto-merge error:
  shared CI errors are persisted after lifecycle evaluation.
- Frontend PR automation types model lifecycle configuration as the three
  booleans only; no lifecycle prompt override fields remain in the client
  contract.
- Deterministic backend and frontend coverage verifies claim ordering and
  outcomes, `IDLE`/`WAITING_FOR_INPUT` immediate delivery, restoration,
  session reselection, no-promptable-session retention,
  reset/supersede/archive loser behavior, post-claim visible-message ordering
  and persistence failure, task/session state publication, guarded zero-row
  classification, dispatch-owned turn rollback, compare-and-set task rollback,
  reserved queue ownership, reserve/ack and restart behavior, durable/discarded
  retries, executor-failure captured rollback, reserved-row archive/delete
  purge, task-queue generation invalidation across archive/unarchive,
  post-guard passthrough lifecycle dispatch, transactional SQLite cleanup,
  post-commit ephemeral mirroring, one-notification-per-task workspace cascade,
  reselection acknowledgement ordering, preclaimed passthrough-token
  serialization, error precedence, and the boolean-only contract.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs
rg -n -i 'lifecycle.*override|override.*lifecycle|archive.*retry|retry.*archive' docs
```

## Files touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
- `apps/backend/internal/orchestrator/event_handlers_queue_test.go`
- `apps/backend/internal/orchestrator/event_handlers_github_ci_automation_test.go`
- `apps/backend/internal/task/{models,repository}.go`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/orchestrator/messagequeue/`
- `apps/web/lib/types/github.ts`
- `apps/web/lib/api/domains/github-api.test.ts`
- `docs/decisions/0051-pr-agent-notifications-extend-task-pr-automation.md`
- `docs/specs/ui/requirements/ci-pr-automation.md`
- `docs/plans/ci-pr-automation/`

## Dependencies

- Completed in the shared worktree: final lifecycle retry validation,
  queue-claim recovery, Auto-merge error-precedence remediation, and frontend
  lifecycle-contract cleanup.
- Plan dependency: Task 18 lifecycle prompt security remediation.

## Constraints

- Preserve the existing task PR automation subsystem, one-minute PR watch
  poller, per-PR checkpoints, and durable queue. Do not add a scheduler,
  generic automation subsystem, replacement session, or lifecycle prompt
  customization contract.
- Preserve normal archive cancellation semantics for accepted work while
  preventing archived-task retries from being reinserted or revived by a later
  unarchive. Reserved-row cleanup and task-queue generation must cover stale
  accepted/reserved/in-flight lifecycle paths as well as newly queued retries.
- Do not change the accepted immutable lifecycle prompt behavior. The duplicate
  embedded-template and hard-coded prompt source is a non-blocking follow-up
  suggestion only.
- Do not commit, push, or alter unrelated application behavior.

## Output contract

- Mark this task done only after the final review remediation is implemented,
  tests are deterministic, and the listed documentation validation commands
  pass.
- Report changed files, validation results, residual suggestions, blockers,
  risks, and any divergence from the plan.
