---
id: "02-current-turn-backend-authority"
title: "Current-turn backend authority"
status: completed
wave: 2
depends_on: ["01-clarification-regression-red"]
plan: "plan.md"
spec: "../../specs/clarification-active-lifecycle/spec.md"
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
