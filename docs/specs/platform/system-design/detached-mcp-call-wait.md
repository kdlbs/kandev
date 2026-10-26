---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-DETACHED-MCP-WAIT-001
  - REQ-PLATFORM-DETACHED-MCP-WAIT-002
  - REQ-PLATFORM-DETACHED-MCP-WAIT-003
  - REQ-PLATFORM-DETACHED-MCP-WAIT-004
  - REQ-PLATFORM-DETACHED-MCP-WAIT-005
  - REQ-PLATFORM-DETACHED-MCP-WAIT-006
created: 2026-10-26
owners:
  - kandev
---

# Detached MCP Call Wait Design

## Boundary and mapping

| Criteria | Implementation boundary |
| -------- | ----------------------- |
| 001.1 to 001.7, 003.1 to 003.6 | `mcp/server.ChannelBackendClient` and the agent stream writer in `agentctl/server/api/agent.go` |
| 002.1, 002.2 | Tool wrapper in `mcp/server` (replaces the `ask_user_question` pinger) |
| 004.1, 004.2, 004.10 | Executor profile validation (`orchestrator/executor`), launch metadata, `agentctl.CreateInstanceRequest` JSON body |
| 004.3 to 004.9, 004.11, 004.12 | New offline-budget tracker in `agentctl/server/process`, beside `attachment.go` |
| 005.1 to 005.3 | Terminal stamp in `agentctl/server/process`, projection in the task service, renderer in `apps/web` |
| 006.1, 006.2 | `lifecycle/environment_resolution.go` and `agents.RuntimeConfig` |
| 006.3 | `config/prompts/kandev-context.md` through `sysprompt` |

The design reads the attach signal main already has. It adds no attachment
state machine, no backend confirmation frame, and no new stream protocol.
Every tracker, clock, and budget below is per instance: one `process.Manager`.

## Attach signal and budget tracker

`attachedCount` (an atomic today, also read by the permission-park site)
moves under one tracker mutex; `IsAttached` keeps its signature and takes the
mutex. The tracker owns `{count, streams, epoch, writerReady, detachedSince, deadline, exhausted, deadlinePassed, gen, changeCh}`.

- **Epoch and ready (per stream).** The tracker keeps one record per attached stream, `{streamID, ready}`; stream IDs are unique and never reused. An attach adds a record with `ready=false`. The stream's writer calls `MarkWriterReady(streamID)` the first time it reaches a select that receives Kandev calls, after the journal replay of that attach. `writerReady` is true when at least one record is ready; `epoch` increments each time `writerReady` goes from false to true. A detach removes only that stream's record and recomputes `writerReady`, so one ready stream ending while another is still replaying clears readiness. A call is bound by the writer that reads it from the send channel, so it binds only to a ready stream; with two ready streams either writer may take it.
- **Clock (004.3, 004.4).** `detachedSince` and `deadline = detachedSince +
  budget` are set at instance creation and by the first 1 to 0 transition
  after a stable attachment. They are not touched by any other transition. A
  single budget timer is armed for `deadline` (not re-armed per detach).
- **Stability (004.4).** The 0 to 1 transition arms a 30-second stability
  timer carrying the current `gen`; overlapping attaches (count 1 to 2) do not
  re-arm it. Only the 0 to 1 and 1 to 0 transitions increment `gen`; 1 to 2 and 2 to 1 do not, so an overlapping stream neither invalidates nor re-arms the timer. When the timer runs it takes
  the mutex and acts only if `gen` is unchanged and `count > 0`: it then
  sets `detachedSince = now`, `deadline = now + budget`, re-arms the budget
  timer, and clears `exhausted` and `deadlinePassed`. A stale callback sees a
  different `gen` and does nothing.
- **Expiry decision (004.5, 004.9).** One procedure `decide(gen)` runs from two callers: the budget timer, and the 1 to 0 transition path after it has released the tracker mutex. It always starts with a fresh hold of the tracker mutex T and re-checks `count == 0`, `!exhausted`, `now >= deadline`, and that `gen` equals the value the caller captured; any mismatch makes it a no-op, so a stream that attached first wins (004.9). The timer, if `count > 0`, sets `deadlinePassed` under T and returns; the 1 to 0 path sets `decisionDue` under T when `deadlinePassed` is set, releases T, and then calls `decide(gen)`. No code path takes another lock while holding T. Under that one T hold the decision sets `exhausted`, signals `changeCh`, and records `{submissionID, state: pending}` for the tracker's in-flight submission (none means 004.7); the states are `pending` (decided, cancel not yet acknowledged), `issued`, `failed`, and `skipped`. The tracker's in-flight submission is set at actual dispatch, not at admission: `DispatchDeliverySubmission` wraps the callback it passes to `SubmissionDelivery.Dispatch`, so the marker is set under T only after `prepareDispatch` decided to dispatch and immediately before the harness call, and a re-admitted settled submission never sets it. `TrackDeliverySubmission` (api/agent.go, admission) does not touch the marker. `DispatchDeliverySubmission` clears it under T in a deferred step on every exit (success, error, panic) if it still names this submission, and the terminal stamp step below clears it too; an instance without a journal never dispatches through that path and never tracks one. After releasing T the decision, in this order, fails the waiting calls under `pendingMu` with the budget error, then, if a turn was recorded, runs the cancel. Under `pendingMu` a bind and the failure pass are mutually exclusive, so a call bound by a reconnecting stream first is a bound call and is written, and one the pass reaches first gets the budget error; never both, never neither (004.6). An attach after the decision does not undo it.
- **Cancel (004.5, 004.11).** The cancel does not take `deliverySubmissionMu`: `DispatchDeliverySubmission` holds that mutex across the whole harness call, which blocks for the entire turn, so waiting for it would wait for the turn being cancelled. The successor-turn fence is the in-flight marker instead. A new prompt dispatch cannot start while the recorded turn's dispatch still runs (it needs `deliverySubmissionMu`), and the dispatch exit clears the marker under T. The cancel takes T once to re-read the marker; if it no longer names the recorded submission, or is gone, it sets state `skipped` (the 004.7 outcome: no cancel, no permission cancel) and stops. Otherwise it releases T and calls `adapter.Cancel` (the call `handleWSCancel` makes for a user cancel) in a goroutine with a 5-second deadline, holding no lock. `adapter.Cancel` sends the cancel and then waits for the in-flight prompt to end, so it is never called under T or any journal lock. It does not use `cancelDeliveryPressureTurn`, which sets `deliveryBlocked` and would reject the user's next prompt. Acknowledged means `adapter.Cancel` returned nil within the deadline: state `issued`. An error or the deadline sets state `failed`, writes the `budget_cancel_failed` structured log and counter (not durable), abandons the goroutine's later result, and does nothing else. `CancelPendingPermissions` runs only in the `issued` case. Every state transition is made under T and is conditional: it applies only if the record for the same submission still exists, and a missing record (already consumed by its terminal's stamp step) means log only; a transition never inserts or recreates a record, only the decision creates one. Residual: between the marker re-read and the cancel leaving the process the turn can end and a successor start, so the cancel can reach the successor, the same window a user cancel has; and a cancel notification leaving after the 5-second deadline could reach a later turn. Both are bounded by the stuck-connection case and stated as accepted.
- **Terminal stamp (004.5, 005.1, 005.2).** The stamp step is the first statement of the per-event loop in `persistDeliveryBatchRaw`, before the event is marshaled and before `appendDeliveryWithRecovery`, for every event that will be journaled as `Terminal` (complete or error). The event's submission is the `DeliverySubmissionID` already on the event (the callers in `manager.go` set it from `deliverySubmissionIDForEvent` before persisting); the step reads that field and never calls `deliverySubmissionIDForEvent`, which takes other locks and consumes the generation mapping, under T or a second time. An empty field means no record lookup: the terminal gets the detached fields only and no budget cause. It takes T once, and in that hold: clears the tracker's in-flight submission if it names this submission, reads and deletes the decision record for that submission, and reads `count`. It sets `detached_at_commit` (count == 0) and `committed_at` (agentctl clock) on every stamped terminal. The state is read in that same hold and is never waited on, because `adapter.Cancel` returns only after the in-flight prompt ends and that end can be the very terminal this step is stamping. For a complete terminal whose record exists in state `pending` or `issued` the payload is forced to `stop_reason: "cancelled"` plus `cancel_cause: "offline_budget"` and `cancel_decided_at`, whatever the agent returned; in state `failed` or `skipped`, or with no record, it is stamped like any other terminal with no budget cause. A cancel that fails after its terminal was stamped only writes the log and counter. An error terminal is never forced. The record is deleted at this step for its own submission and for no other, and is not deleted by stability, so a stale record cannot stamp a later turn. The append runs after the step with no lock held and `deliveryActiveMu` is untouched, so the existing append path, which takes `deliveryActiveMu` in `deliveryEventsCommitted`, `refreshDeliveryPressure`, and the capacity-recovery helpers, is not wrapped and cannot re-enter. Append retries reuse the stamped bytes, so a retry never stamps twice. A terminal whose stamp step ran before the decision's T hold is never rewritten and the decision then finds no in-flight submission (004.5, 004.7). The backend's classifiers compare `stop_reason == "cancelled"` (`agent_delivery_settlement.go`,
  `lifecycle/events.go`, `durable_delivery_stream.go`,
  `orchestrator/event_handlers_streaming.go`) and are not changed, so an
  older backend settles a budget cancel as an ordinary cancel and ignores the
  extra fields (005.2). `CancelDeliverySubmission` is not used. An instance with no journal never reaches the stamp step (`persistDeliveryBatchRaw` returns early), so it stamps nothing and has no in-flight submission (005.3).
- **Exit paths (004.8, 004.12).** No new exit path. At instance creation the
  tracker compares the budget with `ownershipperiod.Resolve(cfg.UnownedPeriod,
  cfg.IdleTimeout)` and the idle timeout and logs the 004.12 warning.

Session `Reset()` of the MCP client fails that client's calls only; it does not touch the tracker's epoch, records, `exhausted`, timers, or the decision record. The decision record is keyed by submission and deleted only by the terminal stamp step of that submission, so a stale record cannot stamp a later turn and stability does not erase an issued cancel.

The tracker exposes `Snapshot() {Ready bool, Attached bool, Epoch uint64, Exhausted bool,
ChangeCh <-chan struct{}}`. `ChangeCh` is closed and replaced under the mutex on every change of `Attached`, `Ready`, or `Exhausted`, so a closed channel is never closed twice and a waiter that re-reads the snapshot after it fires cannot spin on a channel that stays closed. `mcp/server` cannot import
`process` because the backend binary also links `mcp/server`, so the
`cmd/agentctl/main.go` server factory, where the instance's process manager
and its MCP client both exist, adapts the tracker into a closure on
`ChannelBackendClient` (`SetAttachmentSnapshotter`). `SetSessionID` is wired
from `api/server.go`; this follows the same direction of wiring but not the
same file.

## Send loop

`RequestPayload` keeps `beginPublish`, the pending map, and the exits on the
caller's `ctx.Done`, `done` (client closed), and `respChan`. The 5-second
`time.After` becomes a loop:

1. `Snapshot()`. If `Exhausted` and not attached (count 0), fail with `ErrOfflineBudgetExhausted` (004.6). An exhausted snapshot that is attached proceeds: a `Ready` one writes the call (001.2 over 004.6) and an attached, not `Ready` one waits in step 2 (001.1). The snapshot therefore carries `Attached bool`.
2. If not `Ready`, select on `ChangeCh`, `ctx.Done`, `done`, and `respChan`, then loop to step 1 with a fresh snapshot. No timeout applies (001.1, 001.4). `respChan` is read
   so a session `Reset()` or `FailRequest` during the wait ends the call at
   once rather than after a later write.
3. If `Ready`, record `epoch`, start a 5-second timer, and select on `requestCh <- msg`, `ctx.Done`,
   `done`, `respChan`, the snapshot's `ChangeCh`, and the timer. When `ChangeCh` fires, re-read the snapshot: if it shows the same `epoch` and `Ready`, keep waiting on the same timer; otherwise stop the timer and loop to step 1, so the 5 seconds restart at the next writer-ready (001.5). A timer firing returns the
   send-timeout error only when a fresh `Snapshot()` shows the same `epoch`
   and `Ready`; otherwise loop to step 1.

A caller that is an internal best-effort notification (001.7) passes a no-wait
option: step 2 and the epoch loop are skipped and the 5-second send timeout
applies as today. `notifyClarificationTimeout` uses it. `codex_user_input`
calls wait like agent calls. Concurrent waiting calls each run this loop; Go's
select gives no ordering between them (001.6).

## Bind before write, outcome unknown

State per pending call, under `pendingMu`: `pending` (not bound), `bound`
(streamID), `done`. A call is claimed by whichever party removes its entry
from the pending map under `pendingMu` first: the response handler, the
stream-failure pass, the budget failure pass, the caller's withdrawal, an
encode failure, or a session `Reset()`; the others then find nothing and
change nothing. Only the stream's own writer goroutine binds a call to that
stream. The writer, after reading a message from the send channel:

1. Checks its own stream context. If it is cancelled the writer calls
   `FailRequestNotSent`, which leaves the entry `pending` and signals the
   waiting `RequestPayload` through a `notSent` value on its `respChan`;
   `RequestPayload` handles it by looping to step 1 (re-registering
   `publishWG` accounting as `beginPublish` does, and failing with the
   existing closed-client error if `done` is closed). This is the 003.3 case
   of a call whose stream had already ended.
2. Encodes the request. On an encode failure it claims the entry, which
   delivers an ordinary error to the caller; if the entry is already gone
   (withdrawn, reset, or failed by the budget pass) it drops the message and
   delivers nothing, so the caller keeps the result of whichever claim won
   (003.3, 001.4).
3. Calls `BindRequestToStream(requestID, streamID)`, which returns
   `errWithdrawn` when there is no pending entry (the caller abandoned or the
   call already finished); the writer then drops the message without writing
   (001.4). Withdrawal and bind both run under `pendingMu`, so they are atomic.
4. On success writes. A write error, or the end of the stream, fails every
   call bound to that stream, written or not, with
   `ErrKandevCallOutcomeUnknown` (003.1, 003.4). The unwritten bound call is
   treated as maybe-written because the writer cannot prove it did not reach
   the socket.

There is no failed-stream set and no tombstone. The stream cleanup in
`agent.go` calls `FailStreamRequests(streamID)` only after that stream's
writer goroutine has exited (it waits on its `WaitGroup` first), so no bind to
that stream can follow the failure pass, and a write error makes the writer
leave its loop and take the same cleanup path (003.3, 003.4). `FailStreamRequests`
takes `pendingMu` and claims every entry bound to the stream.

`HandleResponse` and `FailStreamRequests` both take
`pendingMu` and remove the entry; whichever runs first decides the call and
the other finds nothing (003.5, 003.6). Nothing resends a failed call
(003.2). A call not yet bound is not stream-owned and keeps waiting (003.3).

Error text is fixed: the unknown-outcome error says `outcome unknown: the
request may have reached Kandev; check current state before retrying`. The
budget error says `offline budget reached: Kandev is unreachable; stop and
end your turn without polling or sleeping`.

## Keepalive

The wrapper registered around every Kandev tool handler, `ask_user_question`
included, replaces `ask_user_question`'s own pinger so a call has one
notification stream with one monotonic counter. It starts the ping goroutine
from receipt and closes a stop channel, then waits for the goroutine to
return, before the handler's result is handed to the MCP transport (002.2). Each notification send runs with a 5-second deadline and the ping loop ticks every 15 seconds, so the gap between notification starts is at most 20 seconds, and the stop wait is at most 5 seconds (002.1, 002.2). Before starting each send, the ping loop checks the stop channel without blocking and returns if it is closed, so a tick that is already ready when stop is requested starts no send, and the stop wait is bounded by one in-flight send (002.2). It uses the client's progress token, or, when the client sent none, a synthesized token `kandev:<sessionID>:<n>` where `n` comes from a per-server atomic call counter. This is new behaviour: the existing helper `clarificationKeepAlive` builds `ask_user_question:<sessionID>`, which is shared by concurrent calls, and the request carries no id the server can read, so the counter provides the per-call uniqueness 002.1 requires. The message is
neutral except for `ask_user_question`, which keeps `Waiting for your
response in Kandev`. A failed send is ignored. `emitKeepAlivePings` is
retained as the timing loop.

## Budget configuration

`offline_budget_minutes` joins `profileConfigAuthoritativeKeys` in
`orchestrator/executor/executor_state.go`, as `ssh_reclaim_task_dir` does. One
parse function (strict base-10, 1..1440, trimmed, no leading sign; leading
zeros accepted; typed `ErrInvalidOfflineBudget`) runs at profile create and
update and again at launch (004.1, 004.2); the profile create and update
entry points are the existing handler save path and the import path, both
through the same function. Launch copies the value into execution metadata
and into a new `offline_budget_minutes` field of the `CreateInstanceRequest`
JSON body sent to the running control server. Two types carry it: the sender `agentctl.CreateInstanceRequest` (backend runtime client) gets an optional pointer field, always set by launch; the receiver `instance.CreateRequest` (`agentctl/server/instance/instance.go`, decoded by `handleCreateInstance` in `agentctl/server/api/control_server.go`) gets the field as a `json.RawMessage` so that absence, `null`, `0`, a fraction, an exponent, a string, and an overflowing number are distinguishable, which a plain `int` cannot do. The field is a JSON integer. agentctl treats absent or `null` as 15 and rejects anything else outside 1 to 1440, or of another JSON type, with a 400 validation error naming the field and range (004.10); the error is built by the handler, not taken from the stock decoder. A `ChannelBackendClient` without a snapshotter set keeps the 5-second send and never waits. A build of agentctl without this capability decodes the body without error and ignores the field, so it
has no budget and keeps the 5-second failure; a build with it uses 15 when
the field is absent (004.10). An adopted instance keeps the budget it was
created with and the backend does not re-send it. The parser lives in a
package both `orchestrator/executor` and `agent/runtime/lifecycle` import, to
avoid a cycle.

## Harness timeout floor

`agents.RuntimeConfig.ToolTimeoutEnvKey` names the key and
`ToolTimeoutUnit` its unit; Claude declares `MCP_TOOL_TIMEOUT`, in
milliseconds (7200000 today). Launch and resume of a new instance compute
`max(default, budget + 10 min)` for the managed default (006.1). A higher-
precedence value is parsed as a positive base-10 integer of the key's unit.
A positive value below `budget + 1 min` fails launch with
`ErrToolTimeoutBelowOfflineBudget` (names key, value, budget). Any other
value fails launch with `ErrToolTimeoutInvalid` (names key and budget, not the
value, since it may be a secret reference) (006.2). The precedence order is
unchanged. `executor_ssh_operations.go` already forwards the key. An existing
profile that sets the key under 16 minutes now fails launch with the typed
error; the public docs and release notes say so.

## Replay records

The terminal stamp (above) carries `detached_at_commit`, `committed_at`, and,
for a budget cancel, `cancel_cause` and `cancel_decided_at`. The task service,
inside the projection of a terminal complete event, writes at most one status
message (`MessageTypeStatus`, `metadata.kind` of `turn_ended_detached` or
`paused_offline_budget`; no new `MessageType`). A terminal with `cancel_cause:
offline_budget` writes only the paused record, at `cancel_decided_at`. Any
other terminal with `detached_at_commit` writes the detached record at
`committed_at`. A terminal with neither writes nothing. The idempotency key is `episode = <delivery stream id>:<journal sequence>`; one turn has exactly one terminal complete event, so the key identifies the turn terminal. Inside the projection transaction the projection looks up a status message whose metadata carries the key and inserts only if none exists; on PostgreSQL it first takes `pg_advisory_xact_lock(hashtextextended('turn_status_record:'+key, 0))`, and SQLite's single writer serializes the same step, so concurrent projections create one record (005.3). A stamped terminal with a missing or unparseable commit time writes its record at the projection time. The
web renders the message through `t()` with localized copy in seven catalogs.
No em dash appears. Desktop and phone use the existing status-message row; no
banner or action is added.

## Compatibility

No feature flag. No schema migration: the budget lives in profile config and
launch metadata; the notice reuses the message metadata column. New agentctl
terminal fields are additive and ignored by an older backend. A backend
launching an older agentctl gets the 5-second failure and no records, which is
the pre-existing behaviour. Metrics: reuse the structured zap logs with
bounded `reason` values (`waited_for_attach`, `budget_exhausted`,
`outcome_unknown`, `budget_cancel_failed`); no task, session, or request ID
is a label.
