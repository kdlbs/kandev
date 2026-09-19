---
id: "02-current-turn-backend-authority"
title: "Current-turn backend authority"
status: completed
wave: 2
depends_on: ["01-clarification-regression-red"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/clarification-active-lifecycle.md"
---

# Task 02: Current-turn backend authority

## Acceptance

- One repository method returns only pending clarification rows in the newest durable turn, with
  matching SQLite/PostgreSQL behavior and legacy missing-status support after ownership matches.
- Deleting every message from a newer turn cannot reactivate a pending clarification from an older
  turn.
- Detach/expiry fallback and workflow guarding consume that method; repeated detach writes and
  publishes nothing, while repository errors keep the workflow barrier closed.
- A detached bundle in the current turn accepts a late answer only after one bounded, acknowledged
  orchestrator resume dispatch, or a rejection without resuming the agent. It does not wait for the
  resumed turn to complete. Any database-fallback answer or rejection for an
  older-turn or terminal bundle returns conflict, performs no write, and initiates no agent resume.

## Verification

```bash
cd apps/backend && go test ./internal/task/repository/sqlite ./internal/clarification ./internal/orchestrator
```

The PostgreSQL behavior test skips locally unless `KANDEV_TEST_POSTGRES_DSN` is set and runs in the
repository's PostgreSQL CI job.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/message_active_clarification_test.go`
- `apps/backend/internal/task/repository/sqlite/message_crud_coverage_test.go`
- `apps/backend/internal/task/repository/sqlite/message_pending_postgres_test.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/clarification/canceller.go`
- `apps/backend/internal/clarification/canceller_test.go`
- `apps/backend/internal/clarification/handlers.go`
- `apps/backend/internal/clarification/handlers_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/clarification_guard.go`
- `apps/backend/internal/orchestrator/clarification_guard_test.go`
- Focused repository-interface mocks that compile against the renamed method

## Dependencies

Task 01.

## Parallelism

Sequential. Task 03 consumes this repository authority and its exact failure semantics.

## Inputs

- Spec `What`, `API surface`, `State machine`, and stale-client scenarios.
- ADR current-turn ownership decision.
- Existing `pendingActionsBySessionQuery` and `pendingActionMessageOrder` dialect patterns.
- Existing clarification handler primary, detached fallback, and stale-dismissed paths.

## Risks

- Do not filter solely by maximum timestamp; use the shared `started_at`, `created_at`, and `id`
  ordering for tied values.
- Derive the boundary from `task_session_turns`, never the latest surviving message.
- Do not reject a detached current-turn bundle merely because the in-memory Store no longer owns it.
- Use `context.WithoutCancel` for terminal writes already promised durable by the handler.
- Do not make query failure look like no pending clarification in workflow guarding.

## Output contract

Use TDD: add focused failing repository/canceller/handler/guard cases, run RED, implement minimally,
then run the exact package command. Report files, test counts/results, PostgreSQL skip/run state,
blockers/risks, and update task/plan status.

## Results

- Added newest-durable-turn clarification authority for SQLite and PostgreSQL, including stable turn
  ordering, missing-status compatibility, and deletion-proof ownership.
- Routed canceller, workflow guard, and detached-response fallback through that authority. Repeated
  detachment is a no-op; superseded/terminal responses return conflict without writes or resume
  events; repository errors fail closed.
- PR review follow-up now claims durable current-turn ownership before live waiter delivery, withholds
  terminal message events until delivery succeeds, and restores a still-current detached bundle when
  acknowledged resume acceptance fails so the answer can be retried. The detached HTTP path calls the
  orchestrator synchronously, so executor rejection cannot be hidden by event-bus publication success.
  SQLite concurrency, rollback, supersession, cancelled-context, and PostgreSQL-dialect cases pin the
  behavior.
- Detach and expiry counts now include only bundles whose messages changed. Malformed messages without
  their schema-required durable turn remain inert instead of becoming pending authority.
- Final review remediation lets atomic response claims recover pending rows from a mixed-status bundle,
  validates answers only for those pending rows, and preserves terminal siblings; targeted rollback
  restores only rows owned by the failed delivery attempt.
- Restore serialization uses copied metadata, so a failed transactional write cannot mutate its
  in-memory terminal snapshot.
- Detached HTTP resume now preserves a 30-second context through the executor and uses dispatch-only
  acknowledgement, so request completion cannot wait for the full agent turn.
- Detached resume now reserves its successor durably before agentctl acknowledgement so early provider
  frames retain a valid turn foreign key, while an unpublished marker keeps the empty row from becoming
  current-turn authority. The reservation stores source turn, pending ID, and exact claimed message IDs.
- Startup reconciliation runs before executor discovery, deletes empty unattempted unpublished
  reservations, and restores only their claimed clarification rows. A message-backed reservation is
  preserved and its marker cleared as ambiguous acceptance. SQLite/startup regressions passed;
  PostgreSQL parity remains environment-gated.
- Review remediation now reports agentctl acceptance followed by successor-publication failure as a
  non-retryable server error. It keeps the answer terminal, removes rollback eligibility, and preserves
  active process ownership so the accepted prompt cannot be dispatched twice in-process.
- At-most-once remediation persists `prompt_dispatch_attempted=true` immediately before the external
  executor call. Startup preserves an attempted empty successor as dispatch-ambiguous authority;
  marker-write failure rolls back before dispatch, while known synchronous rejection can still restore
  the claimed clarification.
- Ready-race remediation gives each live reservation an accepted-or-rolled-back completion signal.
  `agent.ready` waits on that signal outside the cancellation guard, reacquires the guard, and
  revalidates prompt generation before it can settle turns or run workflow completion.
- Bot-review remediation bounds each clarification claim and restore database operation with a fresh
  detached 30-second context. Post-acceptance transport errors now run normal failure cleanup while
  retaining the non-retryable accepted marker, including when durable publication also fails.
- A ready event resumes normal predecessor completion after a reservation rolls back and its prompt
  generation remains authoritative. Startup now fails before watcher, scheduler, or prompt admission
  when any unpublished reservation cannot be reconciled.
- Follow-up review remediation represents a wired no-active-turn snapshot explicitly during
  clarification-pause cancellation. A first turn created during detachment now rejects the stale pause
  just like a successor to an existing turn; the unwired turn-service fallback stays unchanged.
- Later review remediation keeps a failed reserved-turn rollback unresolved in memory, quarantining
  ready handling and later prompt admission until restart recovery. Detach and expiry repository work
  now reuse the fresh detached 30-second persistence context instead of stripping every deadline.
- Session cancellation now drains all in-memory waiters but mutates and counts only bundles returned by
  durable current-turn authority, so a stale timeout cannot cancel a newer active turn.
- Final review remediation makes that durable detach an atomic `UPDATE ... RETURNING` claim over the
  current turn, pending status, and non-detached marker. Restore writes recheck current-turn ownership
  in the update itself, unexpected live-delivery failures distinguish recovered retry state, and test
  doubles preserve caller-owned claim snapshots instead of mutating them in place.
- SQLite and environment-gated PostgreSQL regressions cover one-shot detachment and the successor-turn
  restore guard.
- `cd apps/backend && go test ./internal/task/repository/sqlite ./internal/clarification ./internal/orchestrator`
  passed. The environment-gated PostgreSQL case skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` was unset; it remains enabled for PostgreSQL CI.
- The same exact package command passed again after review remediation; changed-code `golangci-lint`
  reported zero issues against merge base `8c9456074a2f61abec48ddd8742ec81635faa16e`.
- After dispatch-acknowledgement remediation, the repository, clarification, and orchestrator package
  command passed again; focused detached-resume tests and changed-code `golangci-lint` also passed.
- Bot-review remediation passed the full clarification and orchestrator suites, changed-code
  `golangci-lint` with zero issues, and ten race-enabled repetitions of the reservation-ready and
  post-acceptance failure regressions.
- The explicit no-turn cancellation regression and the existing successor-turn case passed ten
  focused repetitions; the full orchestrator and clarification suites passed, and changed-code
  `golangci-lint` reported zero issues.
- Failed-rollback quarantine plus bounded detach/expiry context regressions passed ten focused
  repetitions.
- Task-session HTTP and WebSocket list handlers now return an internal error when authoritative pending
  projection fails instead of silently presenting every session as clean; the focused dual-transport
  regression and changed-code Go lint passed.
- Claude review remediation splits the handler and canceller repository surfaces, removes the unused
  pending-session lookup, and clones completion metadata without exposing partial in-memory mutation on
  write failure. The failed second-write regression and adjacent claim/restore cases passed.
- Final Codex review returns committed pending rows from restoration and publishes them before reporting
  a retryable resume rejection. The interleaved-refresh handler regression, repository restore cases,
  detached-resume orchestrator case, and service/adapter compile checks passed.
- CodeRabbit remediation decodes boolean, string, and numeric attempted markers consistently with SQL,
  preventing ambiguous accepted reservations from being deleted during recovery. Claude follow-up adds
  direct coverage for dropping a generationless ready after rollback; both groups passed ten times.
- Final review remediation bounds synchronous detached resume with the same fresh 30-second context as
  durable claim and restore work. Repository tests now inject exact mutation clocks, removing
  scheduler-dependent timestamp-boundary sleeps while retaining nanosecond ordering coverage.
- Latest bot review remediation rolls back failed successor-turn and attempt-marker admission through a
  fresh bounded context after caller cancellation. Shared SQLite/PostgreSQL predicates now recognize
  boolean, numeric, and string pending/attempted encodings; focused regressions cover both rollback paths
  and both database dialects. Focused race checks and the six affected backend package suites passed;
  the PostgreSQL case skipped locally without `KANDEV_TEST_POSTGRES_DSN` and remains enabled in CI.
- Follow-up review bounds every terminal clarification publication with the fresh 30-second persistence
  context. A cancelled-caller regression and the full clarification suite passed with zero lint issues.
- Later Claude review makes malformed claimed-message ID arrays fail startup recovery atomically instead
  of silently restoring a subset. The SQLite early-return path documents its database-level writer
  serialization, and the pending-action query documents its current-turn session boundary.
- The full SQLite repository suite passed; changed-code Go lint reported zero issues.
- Final review follow-up moves unpublished-turn reconciliation onto the compile-time turn-repository
  contract and fails loudly when the service is miswired. PostgreSQL parity coverage now pins string
  `"true"` and `"1"` detached flags alongside the SQLite cases.
- The focused service regression, full SQLite repository and task-service suites, handler compile, and
  changed-code Go lint passed. The PostgreSQL test remains environment-gated and skipped locally.
- Final Claude review distinguishes durable restore from live convergence: once the database bundle is
  pending again, a publication or summary error is logged but the response still identifies the answer
  as safe to retry. The focused failure cases and full clarification suite passed.
- Latest Codex review makes the orchestrator's composition boundary fail closed when no turn service is
  wired, preventing watcher, scheduler, and prompt admission startup without reservation recovery.
- Fresh Claude review turns the pre-existing expiry scaffold into shipped behavior: any accepted
  completed, failed, or cancelled session transition expires its current-turn pending bundles. Focused
  coverage and the full clarification and orchestrator suites passed.
- Delayed CodeRabbit review guards the anomalous `(nil, nil)` session-repository result before turn
  admission dereferences it. `ReserveTurn` now returns an error instead of panicking, with a focused
  regression.
- Exact-head Codex review makes public turn-event metadata recovery-clean at the shared publication
  boundary. The fast-completion regression now uses `Service.CompleteTurn` and verifies the concurrently
  emitted completion event does not leak any `prompt_dispatch_*` fields.
- Exact-head Greptile review adds a durable startup start-event marker for accepted and ambiguous
  reservations. Startup replays `turn.started` and any already-required `turn.completed` event before
  clearing it; a failed replay fails startup and leaves the marker for the next restart.
- Replay regressions, the full task subtree, backend composition, integration, orchestrator, executor,
  and changed-code Go lint pass. PostgreSQL parity remains CI-gated without a local DSN.
- Delayed Claude review caps post-deadline delivery-confirmation waiting at five minutes, fails startup
  on partial clarification recovery metadata, logs expected watchdog cancellation at debug, and
  documents the intentional duplicated pending-ID SQL bind.
- Clarification, SQLite repository, orchestrator, and changed-code Go lint pass after remediation.
- Latest CodeRabbit follow-up makes claimed messages immutable after construction and keeps finalized
  snapshots callback-owned. A callback may complete after the responder's bounded wait without racing
  recovery reads; the existing repository guards serialize its durable outcome against compensation.
- Focused immutable-snapshot and confirmation tests pass under the race detector, and the full
  clarification suite passes.
