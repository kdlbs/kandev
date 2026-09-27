---
id: "01-offline-budget"
title: "Offline budget in agentctl"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-003
acceptance_criteria:
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.5
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.1
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.2
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.3
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.4
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.5
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity-01.md
  - ../../specs/platform/system-design/detached-agent-continuity-02.md
  - ../../specs/platform/system-design/detached-agent-continuity-03.md
---

# Task 01: Offline budget in agentctl

## Summary

agentctl measures how long no backend stream has been attached. When that
reaches the instance's offline budget and a turn is running, agentctl cancels
the turn and journals the event. The budget defaults to 15 minutes and comes
from the executor profile.

## In scope

- **Attachment state in `agentctl/server/process/attachment.go`:** replace
  the atomic counter with the `attachMu`-guarded state from part 2
  "Attachment state": the current stream (`streamID`, `attach_id`, confirmed
  flag, close function), episode, per-episode `attachedCh` and
  `exhaustedCh`, `detachedSince`, the timer, `attachedAtSequence` (the
  journal high water read when a stream becomes current), and the
  enforcement signals, and the unjournaled budget pause. An instance starts
  detached in episode 1 with a fresh channel pair, `detachedSince` set, and
  the budget timer armed; no channel is ever nil. Implement the Stream
  start, Supersede, Stream end, and Confirm transitions. `GetDeliveryStatus`
  reports `Attachment{Current, AttachID, Confirmed, AttachedAtSequence,
  UnjournaledBudgetPause}`; zero is a valid sequence; a Confirm that marks a
  stream confirmed clears the unjournaled pause.
  `Snapshot()` implements `AttachmentWaiter` for task 02, with `Attached`
  meaning confirmed. `IsAttached` reads under the same mutex and reports a
  current stream.
- **Stream confirm and supersede in `agentctl/server/api/agent.go`:** accept
  the `attach_id` query parameter on the agent stream; a stream without it is
  confirmed at once. Add `POST /api/v1/agent/stream/confirm` (204 on match,
  idempotent; 409 `ATTACH_NOT_CURRENT` otherwise). A new stream supersedes
  the current one: `FailStreamRequests(old, ErrKandevCallOutcomeUnknown)`,
  close code 4001 `superseded`, and wait for the old goroutines to exit
  before the new handshake completes. After the wait, re-check under
  `attachMu` that the new stream is still current; if not, close it with
  4001 and start no reader or writer.
- **Stream liveness in `handleAgentStreamWS`:** a ping every 15 s, a 45 s
  read deadline extended on every frame, and a 10 s write deadline on every
  write.
- **Detach clock:** a confirmed stream that ends, or is superseded by an
  unconfirmed one, starts a new episode and arms the timer; an unconfirmed
  stream changes nothing; a confirmation stops the timer and closes
  `attachedCh`; `expire(E)` is a no-op for a stale, attached, or
  already-exhausted episode, otherwise closes `exhaustedCh` even when no turn
  runs, and ends a current unconfirmed stream with close code 4002
  `offline_budget`.
- **Budget enforcement** from part 2 "Budget enforcement", outside the lock:
  - with no active turn, end and journal nothing;
  - call the adapter's `Cancel`, up to 3 attempts of 10 s, 2 s apart;
  - if all fail, stop the agent process group and remove `agent.pgid`;
  - after `cancelled` or `stopped`, cancel any still-pending permission
    request, then journal `agent_link.offline_budget_exhausted` once with
    `detached_since`, `exhausted_at`, `outcome`, and the last
    `cancel_error`; an append error retries up to 3 attempts, 1 s apart,
    then logs, counts `agent_link_budget_journal_failed_total`, keeps the
    event as the unjournaled budget pause, and ends enforcement;
  - a failed stop selects on a 60 s timer and `attachWaitCh` (checked
    first): the timer repeats the stop, which always runs to its end;
    `attachWaitCh` journals outcome `stop_failed` and ends;
  - `enforcing`, `attachWaitCh`, and `enforcementDoneCh` under `attachMu`; a
    stream start that finds `enforcing` closes `attachWaitCh` and waits on
    `enforcementDoneCh` or its request context;
  - keep agentctl and the journal running.
- **Profile override:**
  - `offline_budget_minutes` in `profileConfigAuthoritativeKeys`
    (`orchestrator/executor/executor_state.go`);
  - validated at profile create and update and again at launch, as the
    design section "Budget configuration" states: absent or empty means 15,
    otherwise a base-10 integer from 1 to 1440, else the typed
    `ErrInvalidOfflineBudget`;
  - carried as `OfflineBudget` through `agentctl.CreateInstanceRequest` and
    `config.InstanceOverrides`.
- **Reaper gate:** the unowned reaper does not shut agentctl down while any
  instance is detached with an unexpired budget, or with an expired budget
  whose enforcement has not ended, evaluated each tick from live instance
  state. The unowned period counts from the later of the last renewal and the
  latest enforcement end. `ownershipperiod.Resolve` and the
  reported `unowned_period_ms` do not change.
- **Capability and close reason:** add `detached-continuity.v1` to
  `SurvivalCapabilities` (`agentctl/server/api/identity.go`). Close the
  backend stream with WebSocket close code 1000 and reason `agent_exited` or
  `agentctl_shutdown` when the stream ends for those reasons.
- **Orphan record:** on agent start, agentctl writes `agent.pgid` (the
  process group ID from `Manager.AgentPID()` and the process start time) in
  the session directory. It removes the file on agent stop. Tasks 05 and 06
  read this file to reap an orphaned agent.

## Out of scope

- Kandev tool call waiting (task 02).
- Backend link state (task 03).
- UI (task 08).

## Acceptance

1. A detached instance with a running turn is cancelled exactly once, at the
   budget. A confirmed reattach before the budget cancels nothing, and the
   clock restarts at the next detach. An unconfirmed stream that attaches and
   ends, any number of times, leaves the clock running, and one that is
   current at expiry is closed with code 4002. A confirm that races expiry
   either wins (no cancel) or loses (one cancel), never both, under `-race`.
   A retried confirm returns 204; a confirm for another `attach_id` returns
   409 and changes nothing. A new stream supersedes the old one, which fails
   its bound calls as outcome unknown, and `AttachedAtSequence` belongs to
   the new stream; an empty journal reports `Current` true with sequence 0.
   A stream whose peer stops answering pings ends within 45 s. Expiry with
   no turn closes `exhaustedCh` and journals nothing. Each episode gets new
   channels. A cancel that fails 3 times escalates to a process-group stop
   and journals outcome `stopped`. A stop that fails keeps the reaper gate
   closed. A stream attaching during enforcement waits for it, and its
   `AttachedAtSequence` is at or above the budget event's sequence. A stream
   that starts waiting during the 60 s stop-retry wait gets outcome
   `stop_failed` without another stop; one that starts waiting during a stop
   retry sees that stop's result first. A new instance reports detached
   before any stream; a Kandev call made then waits and never blocks on a
   nil channel, and with no stream ever confirmed, `expire(1)` releases it
   with `ErrOfflineBudgetExhausted`. With three starts A, B, C where C
   supersedes B while B waits on A, B closes with 4001 and never reads
   `updatesCh` or `requestCh`. A journal append that fails 3 times ends
   enforcement (closing `enforcementDoneCh`, so a waiting stream proceeds), increments the counter, and `GetDeliveryStatus` reports the
   pause until the next Confirm.
2. The profile value reaches the instance config. Empty means 15. A
   non-integer, overflow, or out-of-range value is rejected at profile save
   and at launch with `ErrInvalidOfflineBudget`.
3. With agent survival enabled, the unowned reaper cannot stop agentctl
   while an instance created after startup is detached with an unexpired
   budget, and it can again once that instance is removed. A permission request parked while detached
   stays pending until reattach or until the budget cancels the turn. It is
   never approved or denied automatically.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/agentctl/server/process/... -run 'TestDetachClock|TestOfflineBudget|TestAttachRacesExpiry|TestPermissionParkedUntilBudgetCancel|TestAgentPgidRecord|TestBudgetEnforcementRetriesThenStops|TestBudgetStopFailedHoldsGate|TestAttachWaitsForEnforcement|TestAttachedAtSequence|TestUnconfirmedStreamKeepsBudget|TestStreamConfirm|TestStreamSupersede|TestStopRetryAttachWaitTiebreak|TestInitialAttachmentState|TestSupersedeRecheckAfterWait|TestBudgetJournalAppendFailure')
(cd apps/backend && go test -race -count=1 ./cmd/agentctl/... ./internal/orchestrator/executor/... -run 'TestReaperGateHoldsDuringOfflineBudget|TestOfflineBudgetProfileResolution|TestOfflineBudgetProfileValidation')
(cd apps/backend && go test -race -count=1 ./internal/agentctl/server/api/... -run 'TestIdentityAdvertisesDetachedContinuity|TestStreamCloseReason|TestAgentStreamLiveness|TestAgentStreamConfirmEndpoint|TestAgentStreamSupersedes')
(cd apps/backend && go test -race -count=1 ./internal/agentctl/server/config/... ./internal/agent/runtime/agentctl/...)
make -C apps/backend lint
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/attachment.go`, and a new
  `attachment_test.go`
- `apps/backend/internal/agentctl/server/process/manager.go`: journal event,
  adapter cancel
- `apps/backend/internal/agentctl/server/config/config.go`:
  `InstanceOverrides.OfflineBudget`
- `apps/backend/internal/agent/runtime/agentctl/control.go`:
  `CreateInstanceRequest`
- `apps/backend/internal/orchestrator/executor/executor_state.go` and its test
- `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`: the
  metadata key constant
- `apps/backend/cmd/agentctl/unowned_reaper_gate.go` and its test
- `apps/backend/internal/agentctl/server/api/identity.go` and `agent.go`:
  capability and close reason

## Dependencies

None. The journal event type relies on the #3598 journal already present on
the base branch.

## Risks

- The cancel races a turn that ends naturally at the same moment. Treat
  "no turn running" as a no-op.
- Replacing the atomic counter touches every `IsAttached` caller, including
  `sendPermissionNotification`. Keep its overlapping-reconnect semantics.
- The journal is full at expiry. #3598's typed journal error applies. Still
  cancel the turn.

## Parallelism

`parallel-safe` with task 03. The packages are disjoint.

## Inputs

- System design part 2 sections: Offline budget; Kandev tool calls while detached,
  for the permission note.
- `docs/specs/platform/system-design/durable-agent-delivery.md`: journal
  event types.

## Results

Pending.
