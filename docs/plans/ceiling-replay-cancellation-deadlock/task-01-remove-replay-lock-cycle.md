---
id: "01-remove-replay-lock-cycle"
title: "Remove the replay lock cycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001
  - REQ-TASKS-QUEUED-SESSION-OWNERSHIP-002
  - REQ-AGENTS-SESSION-CEILING-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-CANCELLED-TURN-COMPLETION-001.2
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.1
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.2
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.3
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.4
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.5
  - AC-TASKS-QUEUED-SESSION-OWNERSHIP-002.6
  - AC-AGENTS-SESSION-CEILING-001.3
  - AC-AGENTS-SESSION-CEILING-001.5
  - AC-AGENTS-SESSION-CEILING-001.6
system_design:
  - ../../specs/tasks/system-design/queued-session-ownership.md
  - ../../specs/agents/system-design/session-concurrency-ceiling.md
---

# Task 01: Remove the replay lock cycle

## Summary

Make replay, callbacks, and cancellation complete without a lock cycle.
Preserve exact workflow entry ownership and task-state precedence through short,
ordered critical sections and existing conditional claims.

## In scope

- Write the four regressions in the [plan test matrix](plan.md#tests).
- Prove `TestCeilingReplayBootReadyAndCancellationProgress` fails before production changes.
- Narrow replay admission locking and preserve final dispatch validation for all seven kinds.
- Order task admission before the global state mutex in review reconciliation.
- Audit route, deferral, claim, Send Now, and publication paths for reverse acquisition.
- Keep lock markers local to held sections and immutable bindings across dispatch.
- Run targeted race tests and existing desktop/mobile end-to-end scenarios.
- Record exact results and update only this package's implementation status.

## Out of scope

No live-instance writes, restarts, new scheduling policy, schema, provider change,
UI markup, or new cancellation setting. Do not replace the existing claim system.

## Acceptance

1. Barrier-controlled replay, boot-ready, and stream activity complete. An unrelated
   cancel persists input-ready session state, closes its turn, publishes Review,
   clears pending cancellation, and returns. The sweep then dispatches another
   eligible task once. Run the callback case with synchronous MemoryEventBus
   delivery and an asynchronous delivery fixture.
2. Any working sibling preserves active state. With no working sibling, a valid
   deferred destination preserves Scheduling. Otherwise the eligible task reaches
   Review. Include idle/failed siblings, archive, Office, and terminal outcomes.
3. Route replacement, deleted destination, successor claim, capacity refusal,
   concurrent Send Now, and attempt cancellation preserve exact ownership.
   Unaccepted work remains retryable. Existing restart and stale-clear tests pass.

## Verification

Run from the repository root. First add the regression and run only its RED
command. Record a behavioral failure, not a compilation or fixture error.

```bash
(cd apps/backend && go test ./internal/orchestrator -run '^TestCeilingReplayBootReadyAndCancellationProgress$' -count=1 -timeout=60s)
```

After correction, run these commands sequentially. Install pnpm dependencies
from `apps/` first only if this checkout lacks them.

```bash
(cd apps/backend && go test -race ./internal/orchestrator -run '^TestCeiling(Replay(BootReadyAndCancellationProgress|RevalidatesAfterRuntimePreparation|PreservesReconciliationPrecedence|ClaimAndShutdownProgress|RejectsSuccessorClaimAfterPreparation|SynchronousReconciliationCallback)|PromptRevalidatesAfterDispatchReceipt|ModelSwitchRevalidatesAfterDispatchReceipt|DispatchAdmissionSerializesRouteMutation)$' -count=10 -timeout=180s)
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test.*Ceiling|Test.*ReviewState|Test.*BootReady|TestCancelAgent|Test.*QueuedEntry|Test.*QueuedSession|Test.*SendNow|Test.*DoNotDeadlock' -count=1 -timeout=300s)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run 'Test.*DeferredLaunch|Test.*AdmittedSession|TestUpdateTaskStateIfCurrentIn' -count=1 -timeout=120s)
(cd apps/web && pnpm e2e:run --project chromium tests/workflow/queued-session-ownership.spec.ts tests/workflow/workflow-cancel-completion.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/workflow/mobile-queued-session-ownership.spec.ts tests/workflow/mobile-workflow-cancel-completion.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner builds fresh artifacts and owns teardown. Do not overlap
desktop and phone runs. Record discovered test counts and actual results.
No PostgreSQL mutation is planned. If implementation changes repository behavior,
extend this work order with the required isolated PostgreSQL coverage before delivery.

## Files likely touched

- `apps/backend/internal/orchestrator/ceiling_replay.go`
- `apps/backend/internal/orchestrator/ceiling_entry_lock.go`
- `apps/backend/internal/orchestrator/ceiling_entry.go`
- `apps/backend/internal/orchestrator/ceiling_claim.go`
- `apps/backend/internal/orchestrator/ceiling_dispatch_admission.go`
- `apps/backend/internal/orchestrator/ceiling_defer.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/dynamic_launch.go`
- `apps/backend/internal/orchestrator/queue_send_now.go`
- `apps/backend/internal/orchestrator/workflow_session_target.go`
- New `apps/backend/internal/orchestrator/ceiling_replay_lock_order_test.go`
- New `apps/backend/internal/orchestrator/ceiling_replay_progress_test.go`
- New `apps/backend/internal/orchestrator/ceiling_dispatch_admission_test.go`
- Existing `ceiling_replay_test.go` and `task_operations_test.go` only where needed.

The work order owns these changes. Additional affected launch seams remain
within scope only when necessary to preserve final entry validation. Put new
test helpers in a separate file if existing files exceed lint limits.

## Dependencies

None. The original queued-session implementation is already present in the base.
This work order does not depend on completing its historical PostgreSQL checks.

## Risks

Moving a lock can expose stale route dispatch. Cover the race after preparation,
not only before the first read. A test deadline alone cannot clean up a deadlocked
goroutine. Use releasable barriers or an isolated subprocess for pre-fix RED.

## Parallelism

`sequential`

## Inputs

- [Design: replay and reconciliation locking](../../specs/tasks/system-design/queued-session-ownership.md#replay-and-reconciliation-locking).
- [Cancellation requirements](../../specs/tasks/requirements/workflow-cancelled-turn-completion.md).
- `ceiling_replay_test.go`: stale route, successor record, claim, and retry fixtures.
- `route_action_lock_order_test.go`: deterministic lock-order regression patterns.
- `task_operations_test.go`: explicit cancel reconciliation and terminal precedence.
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.

## Initial implementation results

Implemented the replay lock-cycle correction and added
`ceiling_replay_lock_order_test.go` and the dispatch-admission regression
coverage. The final dispatch boundary renews the exact claim after preparation
for every replay kind and rejects successor claim or route changes.

The mandatory RED run failed behaviorally before the production change: replay
timed out while a synchronous dispatch callback attempted task-state
reconciliation. After the correction, the focused four-test run passed, as did
the repeated race run, the targeted orchestrator and SQLite race selections,
and the required Chromium and mobile Chrome workflow specs. The exact results
were:

- Focused replay tests: 4 passed.
- Repeated replay race tests: 4 tests x 10 repetitions passed.
- Dispatch-admission tests: all seven replay kinds passed, including route
  mutation serialization and successor-claim rejection.
- Targeted orchestrator race selection: passed.
- Targeted SQLite repository race selection: passed.
- Chromium workflow specs: 3 passed.
- Mobile Chrome workflow specs: 2 passed.

No live instance state changed. The package remains uncommitted.

## Review remediation

The local review found two gaps: final dispatch lacked an atomic claim boundary,
and the callback regression did not reproduce cancellation or sweep progress.

The replacement progress test uses the real watcher, task service, SQLite
repository, stream handler, boot-ready handler, cancellation path, and sweep.
Barriers establish the stream guard, replay admission lock, and blocked boot-ready
handler. The test verifies persisted Review, a closed turn, cleared cancellation,
event publication, and exactly one dispatch for the second queued task.
Both synchronous MemoryEventBus delivery and asynchronous delivery use this test.
An isolated child process bounds failure without leaking blocked production
goroutines into the parent test process.

A Go overlay restored the two pre-fix lock-order files from `HEAD` without
changing the worktree. The replacement progress test failed in both delivery
modes. The synchronous run reported `no progress: stream completed`. The
asynchronous run timed out with the original lock cycle and blocked cancellation.

Additional regressions failed before correction for successor claim replacement,
late route replacement, and model-switch admission. Final admission now validates
the route and renews the exact existing claim under one task-admission lock.
The seven-kind admission matrix verifies that route mutation waits for this
renewal. Runtime calls occur after lock release. Cleanup preserves successor
claims even when their route also changes.

The repeated race command passed all nine selected tests for ten repetitions
in 107.603 seconds. The SQLite race selection passed in 6.819 seconds.
The local sandbox required `GOCACHE=/tmp/kandev-review-go-cache` for Go commands.

The additional scoped lint command passed with zero issues:

```bash
(cd apps/backend && GOCACHE=/tmp/kandev-review-go-cache GOLANGCI_LINT_CACHE=/tmp/kandev-review-lint-cache golangci-lint run ./internal/orchestrator/... --new-from-rev=HEAD --timeout=5m)
```

The broader orchestrator race selection passed in 30.183 seconds. This run
included the final regression files after the test-only lint correction.
The fresh Chromium build passed three tests in 34.0 seconds. The fresh mobile
Chrome build passed two tests in 31.8 seconds. Both builds used the managed
runner and isolated Docker fixtures.

The specification catalog, specification lint, and `git diff --check` passed.
The temporary pre-fix overlay files were removed. No live instance changed.
All source and documentation changes remain unstaged and uncommitted.
