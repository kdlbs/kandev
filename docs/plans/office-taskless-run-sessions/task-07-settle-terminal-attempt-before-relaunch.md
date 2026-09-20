---
id: "07-settle-terminal-attempt-before-relaunch"
title: "Settle a claimed run from its terminal attempt instead of relaunching"
status: pending
wave: 1
depends_on:
  - "04-fail-closed-on-recovery-and-stop-errors"
plan: "plan.md"
requirements:
  - REQ-OFFICE-TASKLESS-001
acceptance_criteria: []
system_design:
  - ../../specs/office/system-design/taskless-run-sessions.md
---

# Task 07: Settle a claimed run from its terminal attempt instead of relaunching

> **PENDING FOLLOW-UP — 2026-09-19.** This work order is not part of the
> current coverage change. It settles a claimed run from an already-terminal
> attempt for the still-outstanding `AC-OFFICE-TASKLESS-001.6` requirement and
> carries `.8`'s replay clause. `.8` remains in scope and is satisfied
> structurally for the current launch path.
>
> The file is kept, unedited below this banner, because a follow-up should start
> from it rather than re-derive it. The current gaps and the two problems a
> follow-up must resolve first are recorded under
> [Outstanding: stop controls and restart recovery](../../specs/office/requirements/taskless-run-sessions.md#outstanding-stop-controls-and-restart-recovery).
> References below to `taskless-run-recovery.md` point at a design file superseded in the same
> change; its content is summarized in that section.

## Summary

Taskless completion persists the attempt terminal and completes the run in two
separate writes: `finishRunSession(..., finished, ...)` then `FinishRun`
(`apps/backend/internal/office/service/event_subscribers.go`). Anything that
interrupts between them — a crash, or a `FinishRun` that simply errors — leaves
a `finished` attempt whose run is still `claimed`.

Nothing settles that run.
`ListUnfinishedRunSessions` selects `state IN ('preparing','running')`, so the
attempt is not in the reconciliation input, and `ReconcileRunSessions` consumes
no other listing. The run instead ages into the generic stale-claim sweep
(`RecoverStale` in `apps/backend/internal/runs/repository/sqlite/runs.go`, an
unconditional `UPDATE runs SET status='queued' WHERE status='claimed' AND
claimed_at < ?` driven at `staleClaimedRunAge = 30 * time.Minute`), which is
blind to run-session state. Thirty minutes later the run is requeued and a fresh
attempt launches beside an attempt that already completed successfully: a
duplicate agent execution, charged again, for one wake.

That is the outcome `AC-OFFICE-TASKLESS-001.6` forbids — interrupted work with
no explicit outcome — and it defeats `.4`'s "shall not complete, charge twice"
by repeating the wake rather than by replaying its events.

The system design closes it in two places under `## Cancellation and recovery`,
because two channels can notice first and only one of them requires a restart.
This work order implements both, plus the deterministic ordering the same
section now requires.

## In scope

**1. Startup reconciliation takes a second population, and decides per run.**
`ReconcileRunSessions` must also inspect attempts already in a terminal state
whose run is still claimed. The runtime is not consulted for these: the attempt
already holds a durable outcome. Add the repository query this needs; do not
widen `ListUnfinishedRunSessions` itself, which has a caller and a meaning of
its own.

The pass then runs in the two phases the system design states, and the second
one is the part that is easy to get wrong. **Do not act per attempt row.** A
provider fallback routinely leaves several terminal attempts on one run, so an
oldest-first per-row rule requeues a run from an old failure while a later
attempt is live or has already succeeded. Implement it as:

- **Phase 1, per attempt, unchanged.** Reconcile every `preparing`/`running`
  attempt against runtime evidence as today. An attempt that reattached on proof
  stays live; every other one is left terminal. Either way phase 1 decides no
  run: the requeue that an interrupted predecessor calls for moves to phase 2,
  so it happens once per run rather than once per attempt.
- **Phase 2, per run.** Group the combined input by `run_id` and decide each
  still-claimed run once, from the complete set of its attempt rows:
  1. any attempt still `preparing`/`running` → skip the run, no error;
  2. else any `finished` attempt → complete the run as processed, settled from
     the **highest-numbered `finished`** attempt (more than one `finished`:
     highest settles, record the duplicate as an error on the run);
  3. else → requeue the run to release its claim, and decide nothing else.

Step 2 wins over step 3 by outcome, not by attempt number: one `finished`
attempt beats any number of higher-numbered failures. Step 3 must **not** apply
failure or retry policy of its own — the attempt recorded its own outcome when
it failed, and the scheduler's admission gates decide the replacement. This is
what keeps an attempt a workspace pause deliberately stopped
(`cancelled`/`interrupted`, `cancel_requested_at` set, `runs.status` untouched)
out of the retry pipeline.

**1b. Settlement side effects are named, not inferred.** Completing a run from
the record writes four of the five effects `handleTasklessAgentCompleted`
performs: the run's `"complete"` event, `refreshContinuationSummary`,
`releaseTaskCheckoutForRun` and `stampRunFinished`. Each needs only the run row
and the agent profile ID, both of which the record holds. **The continuation
summary is not optional** — the crash happens before the normal path wrote it,
and `AC-OFFICE-TASKLESS-001.2` requires it to carry context to the next fire.
`recordRunOutputSummary` is **excluded**: it resolves the final agent message
from `AgentLifecycleData.TurnID`, or from `SessionID`, which is empty for a run
owner by design, so there is no startup path to it. A settled run completes with
an empty `output_summary`; that is the stated outcome, not something to
reconstruct. The reuse named under `## Reference` performs only
`FinishRunSession` + `FinishRun`, so this work order must extend it or introduce
a shared completion helper that both it and `handleTasklessAgentCompleted` call.
Do not copy the five-call block into a second place.

**2. Reservation refuses to allocate beside a `finished` attempt.**
Before allocating the next attempt number, ask the same question phase 2 step 2
asks: does the run already carry an attempt in `finished`? If so the run's work
is done — refuse the allocation, settle the run from that attempt exactly as (1)
does, and launch nothing. A run whose attempts are all terminal with no
`finished` among them is an ordinary retry and allocates normally. This is the
half that holds when the completion write failed without the process dying and
no restart is coming.

**2b. The two refusals are distinguishable at the caller.** Reservation now
refuses for two unrelated reasons and `launchAgent` must handle them oppositely:

- **`(run_id, attempt)` conflict** (a rival caller won the row) — the loser
  fails its own launch and the run stays in the existing failure and retry
  policy. Unchanged behavior.
- **Settlement refusal** (the work is already done) — abandon the launch with
  **no** launch failure recorded against the run, no retry scheduled, no attempt
  row created, and the run left complete rather than claimed.

Return these as two distinguishable outcomes, not one error value.

A settlement whose own write fails differs by channel. On the startup pass it
fails startup, exactly as Task 04 makes a failed requeue fail startup. On the
reservation path there is no startup to fail: return the error, abandon the
launch, leave the run claimed, and record the failure on the run's event stream.
**Never fall through to allocating an attempt after a failed settlement** — that
is the duplicate execution this work order exists to prevent. The next pass of
either channel settles it; both are idempotent.

**3. Deterministic ordering.** `ListUnfinishedRunSessions` and
`ListLiveRunSessionsForWorkspace` both order by `created_at ASC` with no
secondary column, so two attempts reserved inside one clock tick are returned in
an arbitrary order. Order by `created_at, run_id, attempt` ascending. `(run_id,
attempt)` is unique, so the three together are a total order. This is
load-bearing, not tidiness: the reconciliation pass returns on the first error
it cannot resolve, so which attempts it reached before stopping must not vary
between two runs over the same rows.

**4. Tests**, in the file Task 02 creates:

- A run claimed behind a `finished` attempt is settled by the startup pass: the
  run reaches its terminal state, and **no second attempt row is created and no
  execution is launched**. The negative half is the assertion that matters.
- The same run settled through reservation with no restart in between.
- Both reaching the same run: the second settlement is a no-op and its side
  effects do not run a second time.
- A run claimed behind a `failed` / `interrupted` / `cancelled` attempt is
  requeued and not completed — this change must not turn every terminal attempt
  into a completion — and the settlement itself records no failure and schedules
  no retry beyond releasing the claim.
- **Mixed terminal outcomes, both orders.** A run with attempt 1 `finished` and
  attempt 2 `failed` completes; a run with attempt 1 `failed` and attempt 2
  `finished` completes. Both must reach the same outcome, and neither may
  relaunch. This is the case a per-attempt-row rule gets wrong, so assert the
  attempt count as well as the run status.
- **A live successor blocks settlement.** A run with attempt 1 `failed` and
  attempt 2 `running` is skipped without an error: the run stays claimed, no
  requeue is written, and attempt 2 is untouched.
- **A deliberately stopped attempt is not retried by the settlement.** An
  attempt finished `cancelled` with `cancel_requested_at` set, on a still-claimed
  run, is requeued to release the claim, with no failure recorded against the run
  by the settlement itself.
- **The four settlement side effects run, and the fifth does not.** A run settled
  from a `finished` attempt has its continuation summary written (the assertion
  that matters — `AC-OFFICE-TASKLESS-001.2` depends on it), its checkout
  released, its finished stamp applied, and a `"complete"` run event recorded,
  and its `output_summary` is empty.
- **Reservation's two refusals are distinguishable.** A settlement refusal
  abandons the launch with no launch failure recorded on the run; a
  `(run_id, attempt)` conflict still fails the launch the way it does today.
- **A failed settlement never allocates.** With the settlement write forced to
  fail on the reservation path, no attempt row is created, the run stays claimed,
  and the error reaches the caller.
- `AC-OFFICE-TASKLESS-001.8`'s replay clause: a historical taskless run already
  recorded as failed stays history. Neither the startup pass, nor the reservation
  guard, nor the stale-claim sweep may turn it back into a launch. This clause
  currently has no test at all.
- Ordering determinism: two attempts sharing a `created_at` are returned in
  `(run_id, attempt)` order across repeated reads.

## Out of scope

- Making completion a single transaction. The two writes cross a repository
  boundary and an event-bus publish; collapsing them is a larger change than this
  criterion needs, and the settlement rule above makes the window recoverable
  rather than invisible, which is what AC .6 asks for.
- Changing `RecoverStale` or `staleClaimedRunAge`. It is a generic run-level
  safety net shared with task-bound runs; a run-session-aware predicate there
  would couple the runs repository to Office storage. This work order removes the
  reason the sweep ever sees this shape, rather than teaching it about it.
- A periodic run-session sweep. Named out of scope in the system design, for
  parity with task sessions.

## Validation

```text
cd apps/backend
go test ./internal/backendapp/ -run 'TestOfficeRunSessionLauncher|TestReconcile' -v -count=1
go test ./internal/office/repository/sqlite/ -run 'TestRunSession' -v -count=1
go test ./internal/office/service/ -run 'Taskless' -v -count=1
make lint
```

Run the repository tests against the Postgres harness as well: this adds a query
and changes two `ORDER BY` clauses, which is dialect-sensitive surface.

## Reference

- `apps/backend/internal/backendapp/office_run_session_launcher.go` —
  `ReconcileRunSessions`; its `AgentStatusCompleted` branch is the nearest
  existing settlement, but it performs only `FinishRunSession` + `FinishRun`.
  Item 1b above is what it is missing: extend it, or introduce the shared
  completion helper both it and `handleTasklessAgentCompleted` call
- `apps/backend/internal/office/repository/sqlite/run_sessions.go` —
  `ListUnfinishedRunSessions`, `ListLiveRunSessionsForWorkspace`, and the
  reservation path
- `apps/backend/internal/office/service/event_subscribers.go` —
  `handleTasklessAgentCompleted`, the two writes that create the window
- `apps/backend/internal/runs/repository/sqlite/runs.go` — `RecoverStale`
- `apps/backend/internal/office/service/scheduler_integration.go` —
  `staleClaimedRunAge` and the sweep that drives it

## Dependencies

Task 04. Both change `ReconcileRunSessions`; Task 04's fail-closed handling of
the discarded requeue results should land first so this work order's new branch
is written against the corrected error contract rather than against the discard.
Task 02 supplies the test file both use.

## Risks

- **Treating every terminal attempt as a completion.** Only `finished` means the
  turn succeeded. Reading "terminal" as "done" would complete runs whose attempt
  failed and silently cancel their retry. Drive each terminal state separately.
- **Deciding per attempt row instead of per run.** This is the failure mode the
  ordering rule invites: `created_at, run_id, attempt` ascending is oldest-first,
  so a per-row settlement reaches attempt 1 `failed` before attempt 2 `finished`
  and requeues a completed run. Phase 2 exists to prevent exactly that; a test
  that only ever builds single-attempt runs will not catch it.
- **A settlement that is not idempotent.** Two channels can reach the same run.
  The run's terminal compare-and-set is what makes the second one a no-op; assert
  that the four side effects it does perform (`"complete"` event, continuation
  summary, checkout release, finished stamp) do not run twice, not merely that no
  error is returned. The output summary is out of scope here — it is never
  written by a settlement, so "does not run twice" is not the question for it.
- **Asserting the settlement and forgetting the launch.** The defect is a
  duplicate execution. A test that checks the run reached a terminal state but
  never checks the attempt count would pass against the bug.
