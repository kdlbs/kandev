---
status: draft
system: agents
created: 2026-10-27
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-007
  - REQ-AGENTS-BACKGROUND-WORK-008
  - REQ-AGENTS-BACKGROUND-WORK-009
---

# OpenCode background-work continuity

## Purpose and boundaries

Agents owns OpenCode workload identity and its protocol observations. This design
extends [shared background work](background-work.md) through its existing store,
runtime, and responsive viewer. It does not create another terminal service.

[Platform admission](../../platform/requirements/background-work-liveness.md)
retains coarse session and steering authority. The
[parked projection](../../disambiguate-waiting/spec.md) owns visible background
activity. Its verified-live rule remains in force. Workflow placement and goal
completion remain independent from inspection rows and background counts.

The scope covers ordinary OpenCode shell jobs that implement polling or
monitoring. Their command text does not turn them into Claude Monitor tools.
Their normalized kind remains shell. Native Codex steering and its pending work
orders are separate work.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-BACKGROUND-WORK-007 | Existing launch observations; Workload lifecycle; Recovery |
| REQ-AGENTS-BACKGROUND-WORK-008 | Producer prerequisite; Automatic foreground turns; Ownership and delivery |
| REQ-AGENTS-BACKGROUND-WORK-009 | Activity and input; Shared desktop and phone experience; Rollout |

## Evidence and delivery prerequisite

The investigation used OpenCode 2.2.0 and Kandev revision
0b1017d81547f44c6dddaf52ed1471767e499c1b on 2026-10-27. Three launches supplied
background=true, followed by shellID and status=running in result metadata.
Kandev retained the launch text but lost the structured background identity.

The native OpenCode database contained a later shell result and assistant
response. Neither appeared in the retained ACP notification log. All 762
captured notifications matched normalized events. These logs contain decoded
notifications, not a complete transport capture.

At OpenCode tag v2.2.0, commit
1097e2668b1e08ad63bdf0b1c4860b9bc01e420c,
[turn.ts](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/cli/src/acp/turn.ts#L295)
closes the prompt subscription when no child sessions remain. Parent-session
shell jobs do not retain it. Kandev cannot recover an event that this producer
does not publish.

Full continuity therefore depends on an upstream OpenCode producer change.
[Task 02](../../../plans/opencode-background-continuity/task-02-opencode-producer.md)
owns that external dependency and protocol proof. Existing 2.2.0 support remains
observation-only. No Kandev implementation or mock fixture can satisfy that
producer gate by itself.

## Existing launch observations

Add an OpenCode-specific recognizer beside
internal/agentctl/server/adapter/transport/acp/enricher_opencode.go. Keep pure
wire translation in proposed dialect_opencode.go and register it through
newACPDialect. The adapter continues to own serialization and event delivery.

The recognizer correlates two observations from the same session and tool call:

1. The execute/shell input contains the boolean background=true.
2. The result contains a nonempty metadata.shellID and metadata.status=running.

Do not parse the human-readable background banner. Reject conflicting IDs,
malformed booleans, missing shell identity, and metadata from another provider.
An ordinary completed shell with metadata.exit retains its existing behavior.

Retain ShellExecPayload.Background for presentation. Stamp detached-work
attestation only after the identified running result confirms launch. Register
the OpenCode recognizer through backgroundlaunch, preserving the existing
cross-package registration boundary. Other providers retain their recognizers.

Use the OpenCode shellID as opaque work_id and run_id within its owning session.
Use the original tool ID as source_call_id. Preserve the original turn link.
The launch tool can be complete while its workload remains running.

Keep the shell binding after the terminal launch result. A later job result
addresses the binding by native shell identity, even when the launch payload
has left activeToolCalls. Do not reuse the launch call as a foreground call or
attach its result to the most recent user turn.

## Producer prerequisite

The upstream work must preserve the ACP identity and use the existing stdio
connection. It must subscribe for the lifetime of an attached session. A prompt
response ends that prompt, not the session subscription. Close, reset, EOF, and
process shutdown drain the subscription and its permission handlers.

The upstream implementation translates native session.shell.started and
session.shell.ended events, plus automatic root-session execution and content.
The
[native schema](https://github.com/anomalyco/opencode/blob/1097e2668b1e08ad63bdf0b1c4860b9bc01e420c/packages/schema/src/session-event.ts#L285)
already distinguishes synthetic input, shell lifecycle, and root execution.
One producer owns each translation. Overlapping prompt and session subscribers
must not emit duplicate content or permission requests.

### Proposed negotiated wire contract

The following contract is a proposal for upstream implementation and proof.
OpenCode 2.2.0 does not advertise it. Its spelling is frozen only after upstream
acceptance and the recorded wire tests in Task 02. A changed proposal requires
this design and its consumer fixtures to change together.

- Client initialize metadata requests opencode/background-continuity version 1
  only when Kandev's shared background-work feature is enabled.
- Agent capability metadata acknowledges that version and identifies the
  supported lifecycle, output, and attach-snapshot surfaces.
- Job changes use ordinary session/update tool notifications with the existing
  shellID/result shape. Completion includes actual state and available exit
  status. Snapshot items include stable job and original launch identities.
- Session-info update metadata under opencode/background-continuity carries
  version, event ID, session ID, event kind, and a native foreground-cycle ID.
  Foreground cycle kinds are started, completed, failed, and interrupted.
- Every automatic text, reasoning, tool, and permission event carries that
  cycle identity. Job-only updates carry job identity and no foreground cycle.
- Foreground start identifies its cause as a client prompt or native automatic
  input. It preserves the triggering native message ID when one exists.
- Foreground boundaries include a monotonic revision. An opted-in client prompt
  carries its expected idle revision. The producer atomically admits that input
  only while the same idle revision holds. A rejection explicitly states that
  input was not accepted and reports the replacement revision.
- Session attach/load produces a complete current workload snapshot or an
  explicit incomplete result. Snapshot absence is not an empty snapshot.

These are metadata on existing ACP messages, not a fabricated standard ACP
list/stop API. Kandev accepts the new semantics only after negotiation and a
recorded upstream producer proves them. Unknown versions remain unsupported.
Older clients keep prompt-scoped behavior unless they explicitly opt in.

Do not hardcode a future OpenCode release number. Record the exact upstream
commit, packaged binary version, wire contract, and replay fixture before full
support is enabled. A version string alone is not a capability advertisement.
Do not silently replace a user's selected managed or native runtime.

## Workload lifecycle

Translate identified launches, terminal outcomes, and fresh snapshots to existing
WorkloadRunObservation and WorkloadOutputChunk values. Use the shared background
provider interfaces for reads. All action capabilities initially report
supported=false. Observation support does not authorize Stop or process stdin.

Task service persists the shared workload/run projection before publication.
Reuse task_session_background_work, task_session_background_runs, and the
existing reducer. No feature-specific table or frontend state slice is needed.
Existing background action receipts remain unchanged.

Deduplicate provider event IDs within the connection and retain native
message/cycle identities before the existing agentctl journal boundary.
Journal replay retains the same normalized event identities. Apply terminal
state monotonically. A fresh running observation cannot reopen a completed job.
Malformed or contradictory terminal evidence leaves the prior state uncertain.

For 2.2.0, identified launch is historical evidence. After its foreground
prompt ends, mark unverified retained work unknown. Do not claim automatic
completion, current output streaming, or a permanent running status.
The legacy launch's banner remains available in the original tool message.

## Automatic foreground turns

The existing adapter_async_complete.go detects unowned content and uses an idle
timer for legacy asynchronous providers. Keep that behavior for existing peers.
Negotiated OpenCode cycles use explicit provider boundaries instead of silence.

Add a provider-cycle owner beside the prompt owner. Resolve its identity on
notification admission, before queued conversion can observe a successor prompt.
The dialect parser returns typed lifecycle descriptions and never executes RPCs.
Only the adapter creates or settles normalized activity events.

For a client-origin cycle, bind the provider ID to the exact admitted prompt
generation. Its session/prompt response retains completion ownership. The native
terminal marker contributes evidence but does not emit a second completion.

For an automatic cycle, emit one existing turn-started event before its content.
Use stable OperationID/ProtocolMessageID values derived from native identity.
Emit one terminal event after the notification barrier. Feed usage through the
existing usage path once. A job-only outcome does not create a root turn.

An explicit automatic cycle is a real provider turn in the existing Kandev
session. It does not create a Kandev task or another session. Backend turn
records remain owned by the existing message/turn service.

Do not route negotiated OpenCode cycles through the five-second async idle
finalizer. Silence, a quiet poller, and job completion do not finish an automatic
agent response. Generation-zero job updates never inherit the current user
prompt merely because that prompt exists when conversion runs.

## Ownership and delivery

Serialize outgoing user prompts and automatic foreground ownership at the
adapter admission boundary. OpenCode's active ACP prompt remains non-steerable.
If automatic execution starts while a user send is being admitted, keep the
user message in the existing queue until foreground ownership is idle.
Preserve queue order and the submitted draft. Never retry a dispatched prompt
merely because a response was lost.

The producer's conditional idle-revision admission closes the race that a
Kandev mutex alone cannot close. A typed, definite not-accepted result can defer
the same user message through the existing queue. Preserve its message identity
and record a distinct immutable dispatch attempt if another attempt is needed.
Do not reuse a rejected journal submission as a different payload. Deferral is
not root completion or a failed user turn. An uncertain response retains existing
uncertain-delivery recovery and never triggers automatic redispatch.

Keep stable provider identities inside normalized events before durable
submission. Use the existing authenticated owner, SQL generation, active
submission, and projected cursor fences. Replayed terminal events cannot settle
a successor user turn. Reset/replacement invalidates the previous provider-cycle
map before accepting fresh notifications.

Add focused integration tests around the existing watcher, journal projection,
and orchestrator paths. Do not add a second transcript writer, usage accumulator,
or delivery inbox. If an existing DTO needs an additive lifecycle discriminator,
carry it end to end in the current event envelope and test older-peer omission.

## Activity and input

| Foreground | Workload evidence | Visible activity | Input |
| --- | --- | --- | --- |
| Generating | Any | Working | Queue for OpenCode |
| Idle | Verified active | Working in background | Ordinary immediate send |
| Idle | Unknown | Activity unavailable; inspectable history | Ordinary immediate send |
| Idle | All terminal | Ready; history remains | Ordinary immediate send |

Keep durable coarse session state truthful. An idle root remains
WAITING_FOR_INPUT. Background visibility does not hold a synthetic RUNNING
session or enable promptQueueing. Use the parked/background display projection
with fresh provider evidence. Clear the projection when automatic foreground
execution starts, then recompute it after that cycle ends.

Extend the existing parked-probe seam with a normalized, generation-fenced
provider sample for negotiated OpenCode. The adapter's retained live subscription
and current job evidence supply that sample. Connection loss, incomplete snapshot,
or unsupported provider reports unknown. Do not infer liveness from stored rows,
an empty output file, or a busy OpenCode helper process.

This provider sample belongs to the existing parked projection. It does not
change the legacy process-walk formula for Claude or invent another UI spinner
predicate. A new ordinary user prompt can coexist with earlier jobs; retain
their identities even when the parked display clears for foreground activity.

Workflow columns, on_turn_complete transitions, notifications, and goal success
retain their existing owners. The background display must not label an active
job as complete. It also must not move a task into or out of a workflow column.

## Shared desktop and phone experience

Reuse BackgroundWorkChip in chat-status-bar.tsx, useBackgroundWork, and the
existing normalized list/output subscriptions. No OpenCode branches belong in
React. The summary distinguishes running, waiting, and unknown counts.

Desktop uses the compact chip, its summary popover, and named central detail
tabs. Phone uses the existing chip drawer with a focused list and a full-height
detail surface. Back returns to the selected job; dismissal returns focus to
the chip. Keep one scroll owner and respect keyboard and safe-area clearance.
Phone action targets are at least 44px.

For observation-only OpenCode, hide unsupported Stop, Interrupt, and stdin
controls. The command banner is not live command output. Show unavailable output
when the producer has no output capability. When final output arrives, show its
retained result with failure or exit status.

When the active count reaches zero, retain a history entry near the composer.
Do not change the preserved behavior for an empty session that never had jobs.
Reuse the shared viewer for completed history. Localize every new status,
accessibility name, count, and unavailable-state message in all catalogs.

## Recovery, security, and observability

Browser reconnect reads shared persisted history and follows accepted revisions.
Runtime/backend reconnect marks open rows unknown until a fresh negotiated
snapshot or event re-establishes ownership. Do not replay missed native automatic
responses unless the producer supplies a proven replay/history boundary.
The supported initial guarantee is continuous attached-session delivery and
existing durable agentctl replay, not lossless recovery across an upstream crash.

Session reset, close, and deletion invalidate retained bindings and stop owned
readers. Late notifications cannot recreate a removed session or another run.
Route every observation through its owning executor and authenticated session.
Never read OpenCode SQLite, output files, or another executor's host processes
as a production protocol substitute. The live investigation used these sources
only as explicitly authorized diagnostic evidence.

Keep raw frames behind the existing debug flag. Record negotiated contract,
connection generation, job/cycle identity, and rejected stale evidence in bounded
structured diagnostics. Identifiers stay in logs, never metric labels. Reuse
existing delivery, replay, and projection counters. Do not introduce a second
background poller in React or a timer that sends diagnostic user prompts.

## Rollout and compatibility

Reuse features.agentBackgroundWork and KANDEV_FEATURES_AGENT_BACKGROUND_WORK.
The package's final rollout work order changes prod/dev/e2e defaults to true.
Retain the existing restart requirement and operator off switch during rollout.
Existing explicit false overrides remain effective. Keep the browser fallback
false until the backend supplies feature state.

Carry the effective typed value through the existing launch/configure DTO chain
into an additive adapter option. Older agentctl peers omit the option and remain
off. Do not read ambient public environment values as another configuration tier.

When disabled, do not request producer continuity, instantiate its readers,
publish new OpenCode background capabilities, or write inspection projections.
Preserve ordinary ACP prompts, messages, output, and existing provider behavior.
Do not enable the Claude background-handoff or native Codex feature flags.
Default-on background inspection does not advertise unproven producer support
or steering. Remove this shared flag only through a later retirement change.

| Provider | Scope in this package | Negative case |
| --- | --- | --- |
| OpenCode 2.2.0 | Confirmed launches and honest unknown state after prompt settlement | No continuous completion or steering claim |
| Negotiated OpenCode v1 contract | Jobs, automatic cycles, shared viewer, verified background activity | Malformed/missing capability remains legacy |
| OpenCode v1 family | Existing ACP behavior unless it proves the same contract | No version-name inference |
| Claude ACP | Existing monitor, wakeup, launch, and handoff contracts | No new admission or parsing behavior |
| Codex native / other ACP | Existing shared viewer and provider capabilities | No OpenCode metadata recognition |

## Related decisions and delivery

- [Provider-neutral ownership](../../../decisions/2026-09-27-provider-neutral-background-work.md).
- [Coarse prompt admission](../../../decisions/2026-07-28-coarse-running-busy-signal.md).
- [Implementation package](../../../plans/opencode-background-continuity/plan.md).

The existing ownership decisions cover this extension. The upstream producer
proposal is an explicit dependency, not a new unproven capability in shipped code.
