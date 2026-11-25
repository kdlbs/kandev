---
status: draft
system: agents
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-011
---

# Cursor ACP Shell Watch System Design

The Cursor ACP dialect recognizes shell watch registrations. The adapter
correlates their updates and emits `WorkloadRunObservation` events of kind
`monitor`. Task persistence and the shared viewer consume those events through
the [Background Work design](background-work.md).

## Wire evidence

The registration is a `tool_call` with kind `other`, status `pending`, and a
`toolCallId` with a nonempty suffix after `watch-`. Its `rawInput.shells` is a
nonempty array. Every item must contain a nonblank string `shellId`. A shell ID
is an opaque provider reference. The optional `reason` and `title` describe
that shell reference. The outer tool title has no role in recognition.
The Cursor dialect returns the formatted title for a match.

| Cursor ACP frame | Adapter action |
| --- | --- |
| Valid registration on the bound Cursor session | Store the watch binding and capture its prompt generation. |
| Matching update with explicit `in_progress` | Publish one `running` Monitor for the binding. |
| Matching update with explicit `completed` | Publish `completed` for the same binding, including if the progress frame was absent. |
| Matching update with explicit `failed` | Publish `failed` for the same binding, including if the progress frame was absent. |

Unmatched, malformed, and unsupported frames follow the normal tool path. The
transcript continues to receive its normal tool calls and updates. Use the
existing effective Background Work setting to gate the watch observer.

The capture shows both watches completing during one held-open turn. The
matching terminal frame has only the ID and status. The installed CLI reported
`2026.11.19-f005e9d`. The launch version is unknown.
[Captured notification shapes](https://github.com/kdlbs/kandev/issues/5485)

## Watch identity and state

Keep one map under `Adapter.mu` for the bound native session and
`nativeGeneration`. Key it by the exact Cursor tool call ID. Each entry stores
one opaque ID, the formatted title, the launch prompt generation, and the last
state. One Cursor watch is one run, so the opaque ID serves as both `WorkID`
and `RunID`. The raw tool call ID is `SourceCallID`.

Format the title once from the available reasons and shell titles. Use the
shell ID as the reference when its title is absent. The existing detail view
shows the full title. Two watches of the same shell have separate tool call IDs.

The worker emits an observation when the entry changes to running or reaches a
terminal state. Use `RunState.IsTerminal` to reject progress after completion,
failure, or interruption. Keep terminal state in the same map so duplicate
registration and late progress cannot reopen the watch.

After the notification barrier for a successful cancelled prompt, interrupt
only watches that remain running for the same native session and captured
prompt generation. Terminal updates processed before the prompt response take
precedence. The watch observation has no shell exit status.

When a new, loaded, or resumed native binding commits, convert each remaining
running watch on the retired binding to `unknown` before clearing the map.
Preserve watches with terminal evidence. Publish retirement events after the
replacement binding barrier is installed, with the replacement native session
ID on the event so a context-reset fence can buffer it and replay it after the
Kandev session commit. A reused raw call ID receives a new workload identity.
Close still retires the map without publishing a replacement observation.
Only the bound session's live notifications update the map.
Release `Adapter.mu` before sending an observation.

Use `streams.InspectionOnlyWorkloadCapabilities()` for both observations.
The completed state describes the watch. The observation leaves the shell exit
code empty.

## Projection and turn ownership

Emit `EventTypeBackgroundWorkUpdated` with the launch `PromptGeneration`. The
lifecycle publisher maps that generation using `promptTurnIDForGeneration` on
the owning execution. It fills `OriginTurnID` only for a known generation and
preserves an explicit origin. An unresolved generation leaves the origin empty.

The publisher supplies the Kandev task and session IDs from the execution.
The orchestrator records and publishes the observation via
`handleBackgroundWorkUpdatedEvent`. The existing reducer retains the workload
and run. The runtime uses its existing execution and delivery fences.

The watch observer leaves `NormalizedPayload` and its attestation fields
untouched. It issues no foreground idle, prompt handoff, or turn completion
event. The existing coarse admission rule keeps a running Cursor turn in
queue mode. [Coarse prompt admission decision](../../../decisions/2026-07-28-coarse-running-busy-signal.md)

The viewer uses the same running count, details, and History on desktop and
phone. A browser reload reads the retained projection. The shared process-end
settlement path handles incomplete watches after confirmed owner exit. Failed
watches appear in History as Failed. Confirmed prompt cancellation settles only
the still-running watches from that turn as Interrupted. A replaced native
binding leaves unresolved watches as Unknown. Cursor watch frames provide no
snapshot for reattaching live work across executions.

## Implementation package

[Track Cursor ACP shell watches](../../../plans/cursor-shell-watches/plan.md)
