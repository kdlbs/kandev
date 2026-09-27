---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-003
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-005
---

# Detached Agent Continuity System Design Part 2

## Purpose and boundaries

Part 2 of the design begun in [part 1](detached-agent-continuity-01.md), which
states the purpose, the contracts this design uses, and the backend side:
link state, redial, notices, persistence, and the UI.
[Part 3](detached-agent-continuity-03.md) covers reconnect. Part 2 covers
what runs on the executor host while no backend is attached:

- the record and reap of an agent that outlived its agentctl;
- Kandev tool calls that wait for reattach;
- the offline budget that bounds a detached turn;
- the guidance the agent receives.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001` | [Orphaned agent after agentctl loss](#orphaned-agent-after-agentctl-loss) (`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.7`) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-003` | [Kandev tool calls while detached](#kandev-tool-calls-while-detached) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-004` | [Offline budget](#offline-budget) |
| `REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-005` | [Agent guidance](#agent-guidance) |

## Orphaned agent after agentctl loss

agentctl starts the agent in its own process group (`setAgentProcGroup`,
`agentctl/server/process/procattr_*.go`). On macOS and BSD, no parent-death
signal exists. `procattr_unix.go` states that orphan cleanup relies on
explicit `Stop()` calls. If agentctl exits, the agent keeps running there. It
can keep editing the workspace and pushing branches with nothing tracking it.

The design handles this in three places:

- **Record.** When agentctl starts the agent, it writes the process group ID
  (`Manager.AgentPID()`, because `Setpgid` makes the group ID equal the PID)
  and the process start time to `agent.pgid` in the session directory. For
  SSH, that directory is the one `ensureRemoteSessionDir` creates at
  `<taskDir>/.kandev/sessions/<sessionID>`. agentctl deletes the file when it
  stops the agent.
- **Reap on redial.** When a redial concludes that agentctl is gone, the
  executor reads `agent.pgid` before it returns:
  - File absent: `already_gone`.
  - No live process with that ID and start time: `already_gone`. The
    start-time check stops a reused PID from being signalled.
  - Live match: send `SIGTERM` to the group, wait 10 seconds, send `SIGKILL`,
    then check again. Gone: `reaped`. Still alive: `reap_failed`.
  - File unreadable, or the signal or check command fails: `reap_failed`.

  For remote Docker, the executor runs the same steps inside the container
  through `docker exec`, if the container still exists.
- **Report.** `reaped` and `already_gone` return `ErrRedialTargetGone` with
  the reap result as detail. Only then does durable delivery reconciliation
  assign the outcome (`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.7`).
  `reap_failed` returns `ErrRedialOrphanUnreaped`. The session stays
  Disconnected with `last_error` set to `orphan_reap_failed`, and no outcome
  is reported. Each later trigger, including user Reconnect, repeats the
  redial and therefore the reap. The start-time check keeps a repeat safe
  after the host has started other processes.

Where no reap is needed:

| Executor | Reason |
| --- | --- |
| Kubernetes | A restarted or deleted container ends every process in it |
| Remote Docker, container gone | The container's processes ended with it |
| Sprites, sprite gone | The sprite's processes ended with it |
| Sprites, sprite present | Needs the in-sprite reap; task 07, blocked |

Local executors are outside this design. The same orphaning applies to them
on macOS, and it is tracked separately with the local agentctl death handling.

## Kandev tool calls while detached

`ChannelBackendClient.RequestPayload` (`mcp/server/backend_client.go`) hands a
request to the stream writer through the unbuffered `requestCh`, bounded by
`time.After(5 * time.Second)`. No writer reads `requestCh` while detached, so
every call fails.

### Attachment state

`attachment.go` replaces the atomic counter with state guarded by one mutex,
`attachMu`:

- `attachedCount`, still a count, so an overlapping reconnect cannot report
  detached while a stream is live;
- `episode`, incremented when the count drops to zero;
- `attachedCh`, closed when the count rises from zero;
- `exhaustedCh`, closed when the episode's budget expires;
- `detachedSince` and the budget timer (see [Offline budget](#offline-budget));
- `attachedAtSequence`, the journal high water read when the count last rose
  from zero.

`GetDeliveryStatus` reports `attachedAtSequence` as `AttachedAtSequence`. It
is zero while detached. The backend uses it as the end of an episode (see
[part 3](detached-agent-continuity-03.md#attempt-steps)). The attach reads
the high water under `attachMu` as it raises the count, so every event at or
below it was journaled before the stream attached.

When the count drops to zero, a new `attachedCh` and `exhaustedCh` pair is
created for the new episode. Channels from an older episode are never reused.

```go
type AttachmentSnapshot struct {
    Attached        bool
    Episode         uint64
    AttachedCh      <-chan struct{} // closed when this episode ends by attach
    BudgetExhausted <-chan struct{} // closed when this episode's budget expires
}

type AttachmentWaiter interface {
    // Snapshot reads all fields under attachMu, so they describe one instant.
    Snapshot() AttachmentSnapshot
}
```

`IsAttached()` stays for its existing callers and reads under the same
mutex.

### Sent and not sent

A call is **sent** when the stream writer has bound it to its stream. Before
that point the call is **not sent**, and the backend cannot have seen it.

`writeAgentStreamMCPRequest` (`agentctl/server/api/agent.go`) changes its
order. Today it writes the request, calls `FailRequest` with the write error
on failure, and calls `BindRequestToStream` after a successful write. The new
order is:

1. The writer receives the call from `requestCh`. The writer reads
   `requestCh` only while its stream is live, and stops reading before the
   stream's pending set is failed.
2. It calls `BindRequestToStream(id, streamID)`. `ChannelBackendClient`
   (`mcp/server/backend_client.go`) records each stream that
   `FailStreamRequests` has failed, under the same mutex as the pending set.
   Binding to a failed stream returns an error and binds nothing.
3. **Bind failed:** the call was never written. The writer completes it with
   the internal sentinel `errRequestNotSent`. `RequestPayload` treats that
   sentinel as not sent and returns to step 1 of its [send loop](#send-loop).
   The agent never sees the sentinel.
4. **Bound:** the writer writes the request. A write error calls
   `FailRequest(id, ErrKandevCallOutcomeUnknown)`, because a partial write may
   have reached the backend.

A bound call that has no answer when its stream ends fails with
`ErrKandevCallOutcomeUnknown`. `FailRequest` on a call that is already
completed or failed is a no-op, so a write error and a stream end cannot
complete a call twice.

`FailStreamRequests(streamID, err)` is defined in
`mcp/server/backend_client.go`. Its call site in `agentctl/server/api/agent.go`
runs after the stream goroutines exit, on every stream end. It runs the same
way whether the backend will treat the loss as Disconnected or as terminal,
because agentctl cannot tell the two apart. The call site changes only the
error it passes, from `errors.New("agent stream disconnected")` to
`ErrKandevCallOutcomeUnknown`. The backend may already have applied such a
call, and resending a non-idempotent call could apply it twice.

### Send loop

`RequestPayload` repeats these steps:

1. Take a `Snapshot`.
2. **Attached:** select on the `requestCh` send, a 5 s timer, and `ctx`.
   - Taken by the writer: wait for the answer as today. An
     `errRequestNotSent` answer goes to step 1.
   - Timer fired: take a new snapshot. If still attached, return the
     existing send-timeout error, because a stuck writer is a local fault.
     If now detached, go to step 1.
3. **Detached:** select on `AttachedCh`, `BudgetExhausted`, and `ctx`. The
   select does not include `requestCh`, so no dying writer can take the call.
   - `AttachedCh` closed: go to step 1.
   - `BudgetExhausted` closed: return `ErrOfflineBudgetExhausted`.
   - `ctx` done: return the context error.

A call made after the episode's budget expired returns
`ErrOfflineBudgetExhausted` at once, because that episode's `BudgetExhausted`
is already closed.

### Keepalive

Every handler that calls `RequestPayload` shares one keepalive wrapper:
`emitKeepAlivePings` at the 20 s `askQuestionKeepAliveInterval`, with a
progress token per call. Today only `ask_user_question_kandev` has it.

### Harness tool timeout

Claude runs with `MCP_TOOL_TIMEOUT=7200000`, a 2-hour limit
(`agents/claude_acp.go`). A budget above that would let the harness time out
a waiting call before the budget ends, which
`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.1` forbids. The launch therefore
guarantees that the harness tool timeout exceeds the budget, instead of
cutting the wait short:

- An agent's `RuntimeConfig` declares its tool-timeout environment key, if it
  has one. Claude declares `MCP_TOOL_TIMEOUT`.
- Launch sets that managed default to the larger of its current value and
  the resolved budget plus 10 minutes, in the key's unit. A 15-minute budget
  leaves Claude at 2 hours. A 1440-minute budget sets it to 1450 minutes.
- If a higher-precedence source (for example the executor profile
  environment) sets the key below the budget plus 1 minute, launch fails
  with the typed `ErrToolTimeoutBelowOfflineBudget`. The error names both
  values. The environment precedence contract itself does not change.
- An agent with no declared tool timeout, such as Codex, uses the budget.
  Task 02 measures Codex. If it finds a fixed limit that cannot be raised, it
  records it, and launch rejects a budget above that limit minus 1 minute
  with the same typed error.

The waiting client therefore holds a call until the budget ends, and every
`ErrOfflineBudgetExhausted` means what its text says.

### Permission requests

Permission requests are not MCP calls. `sendPermissionNotification`
(`agentctl/server/process/manager.go`) already parks a request while
detached, and journal replay delivers it on reattach. The request stays
pending while detached, and nothing answers it automatically.

The one exception is the offline budget
(`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.5`). Budget enforcement cancels
the turn, and the turn's context cancellation completes each pending request
through the existing path: `consumePermission` with `Cancelled: true`, then
`sendPermissionCancelledNotification`, which journals a
`permission_cancelled` event. After enforcement, agentctl cancels any
request of the instance that is still pending the same way, so none is left
waiting. A cancelled request is neither approved nor denied. On reattach,
replay delivers the `permission_cancelled` event, the backend marks the
permission message cancelled as it does today, and the budget notice that
follows it records the pause.

## Offline budget

The detach clock in `agentctl/server/process` uses the
[attachment state](#attachment-state). All steps below run under `attachMu`
unless marked otherwise.

- **Detach.** When the count drops to zero: increment `episode`, record
  `detachedSince` on agentctl's clock, create the new channel pair, and arm
  `time.AfterFunc(budget, expire(episode))`.
- **Attach.** If the ending episode's [enforcement](#budget-enforcement) has
  started and is not settled, the new stream waits for it, outside the lock,
  before its handshake completes. Then, when the count rises from zero: stop
  the timer, close
  `attachedCh`, and clear `detachedSince`
  (`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.4`). If the timer had already
  fired, its callback finds the episode over and does nothing.
- **Expire.** `expire(E)` returns without effect if `episode != E`, the count
  is above zero, or `exhaustedCh` is already closed. Otherwise it closes
  `exhaustedCh`, which releases waiting calls whether or not a turn runs.
  Cancellation therefore happens at most once per episode.
- **Enforce,** outside the lock, in one goroutine per episode. See
  [Budget enforcement](#budget-enforcement).

### Budget enforcement

Enforcement starts when `expire(E)` closes `exhaustedCh`. A stream that
attaches meanwhile waits until enforcement is settled (see
[Offline budget](#offline-budget)), so the budget event is always journaled
before the attach.

1. **No turn.** If the adapter reports no active turn, enforcement ends.
   Nothing is journaled.
2. **Cancel.** Call the adapter's `Cancel` (the same `(*Adapter).Cancel` that
   `handleWSCancel` uses), bounded by 10 s. Retry up to 3 attempts in total,
   2 s apart. Before each retry, check the turn again. A turn that has ended
   ends the cancel with outcome `cancelled`.
3. **Stop.** If all 3 attempts fail, stop the agent process group, as
   `Manager.Stop` does (`killProcessGroup`), and remove `agent.pgid`. agentctl
   and its journal stay up. The outcome is `stopped`.
4. **Journal.** After `cancelled` or `stopped`, cancel any permission request
   still pending (see [Permission requests](#permission-requests)). Then
   journal `agent_link.offline_budget_exhausted` with `detached_since`,
   `exhausted_at`, `outcome`, and the last `cancel_error` if any. It is
   journaled once per episode.
5. **Stop failed.** If the stop fails, log it, wait 60 s, and repeat step 3
   while no stream waits to attach, until it succeeds or the agent process is
   gone. A process that is gone gives outcome `stopped`. When a stream waits
   to attach, enforcement ends instead: it journals the event with outcome
   `stop_failed` and cancels nothing more. The turn keeps running, now
   visible to the user, who can stop it.

Enforcement is **complete** when step 4 has journaled, or when step 1 found
no turn. It is **settled** when it is complete, or when a stop has failed
once. The worst case before settled is 3 cancels of 10 s, 2 waits of 2 s, and
one stop of about 12 s, so under 50 s. The backend bounds the step 5
handshake at 90 s.

No turn can start while detached. Prompts reach agentctl only over the
backend stream, and none is attached. A turn that ends on its own while the
cancel runs makes the cancel a no-op.

agentctl keeps running after expiry, so the journal remains readable.

### Budget configuration

1. The executor profile `Config` key `offline_budget_minutes` is added to
   `profileConfigAuthoritativeKeys` (`orchestrator/executor/executor_state.go`),
   like `ssh_reclaim_task_dir`.
2. Validation, at profile create and update and again at launch:
   - absent or empty: 15;
   - otherwise a base-10 integer from 1 to 1440, with surrounding spaces
     trimmed;
   - anything else, including a sign, a fraction, a unit, or an overflow, is
     rejected with the typed `ErrInvalidOfflineBudget`. Profile save returns
     it as a validation error. Launch fails with it rather than using a
     default.

   An existing profile has no key, so it gets 15 minutes. No migration is
   needed.
3. `applyProfileConfigToMetadata` copies the validated value into execution
   metadata.
4. It travels as `OfflineBudget` in `agentctl.CreateInstanceRequest`, which
   becomes `config.InstanceOverrides` and then the instance config.

No global setting and no feature toggle exist.

### Displayed pause time

agentctl's clock is authoritative. Its episode starts when agentctl sees the
stream end, which can differ from when the backend sees it. The backend's
`BudgetDeadline` (`Since` plus budget) is therefore an estimate, and the
banner labels it as approximate. After a reconnect, the budget notice uses the
journaled `exhausted_at`.

### Unowned reaper

The unowned reaper (`cmd/agentctl/unowned_reaper_gate.go`) runs only with
agent survival on. It can shut agentctl down after `agentctl.unownedPeriod`
(10 minutes) without an ownership renewal, which is before a 15-minute budget.

`ownershipperiod.Resolve` does not change, and neither does the
`unowned_period_ms` that `/identity` reports. The reaper gate gains one
condition, evaluated on every tick from live instance state: it does not
shut agentctl down while any instance is detached with its budget not yet
expired, or with its budget expired and its
[enforcement](#budget-enforcement) not complete
(`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-004.5`). A stop that keeps failing
therefore keeps agentctl up, because its exit would leave a running agent
with nothing tracking it. The gate reads each
instance's episode state under its `attachMu`, so instances created or
removed after startup are covered, and a removed instance no longer holds the
gate. The unowned period also counts from the later of the last renewal and
the latest enforcement completion, so the journal stays readable for at least one
full unowned period after the cancel. After that, the existing reaper
contract applies. A later reconnect that finds agentctl gone takes the
reconciliation path (`AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.4`).

## Agent guidance

Error texts are stable Go constants in `mcp/server`:

- `ErrOfflineBudgetExhausted`: "Kandev is unreachable and the offline budget
  was reached. Stop now and end your turn. Do not poll, sleep, or retry this
  call."
- `ErrKandevCallOutcomeUnknown`: "The connection to Kandev dropped before this
  call returned. Its outcome is unknown. Check the current state before
  retrying."

`sysprompt.go` adds a `connectionLossSection` to the `kandev-context`
template (`config/prompts/kandev-context.md`), next to
`{step_complete_section}`. It states that Kandev tool calls can wait during a
connection loss, and that the agent must never wrap them in sleep or polling
loops.

## Related decisions

- [ADR-2026-09-27-backend-dialed-detached-agents](../../../decisions/2026-09-27-backend-dialed-detached-agents.md)
- [Durable sessions across harness generations](../../../decisions/2026-09-10-durable-harness-session-boundaries.md)
