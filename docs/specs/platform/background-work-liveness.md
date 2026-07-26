---
status: shipped
created: 2026-07-21
updated: 2026-07-26
owner: kandev
---

# Background Work Liveness

## Why

Operators need to know whether an agent is actively generating, is available
for another prompt while recognized work continues, or has finished. Collapsing
those conditions into a coarse running state both hides active background work
and incorrectly prevents prompt delivery.

## What

- A session distinguishes foreground generation from recognized background work.
  Foreground activity takes precedence over background work; neither condition
  may appear as done.
- A foreground-idle session with recognized background work accepts a new
  prompt. A session with foreground activity continues to reject it.
- When a provider-attested, generation-bearing foreground-idle boundary also
  guarantees prompt handoff, the next human prompt reaches the provider
  immediately even if the prior prompt RPC remains open for detached work.
  Synthetic wakeups remain serialized behind the owning RPC. Providers that do
  not attest handoff keep the follow-up queued until they release ownership.
- Only adapter-attested workloads relax prompt admission. An adapter opts in a
  workload only when Kandev has fixture-tested both its launch and accountable
  terminal lifecycle. Claude subagents, background shells, and Monitor watches,
  plus Codex subagents, are initially supported; other ACP agents remain
  conservatively foreground-busy.
- The runtime retains one registration per live subagent and derives an active
  subagent count from those registrations. Background shells and Monitor
  watches contribute to background liveness but not to the subagent count.
- Task and session status surfaces distinguish generating,
  background-running, and done with text or accessible labels in addition to
  color. The compact sidebar uses the same dashed spinner for both running
  states: yellow for foreground generation and violet for background work, with
  a distinct background-work tooltip and accessible label. A pending
  permission request takes precedence over a pending clarification request,
  which in turn takes precedence over the work-state indicator for a current,
  input-capable session only; stale pending flags on starting or terminal
  sessions do not override the work-state indicator.
- A task aggregates its sessions with most-active-wins: generating outranks
  background-running, which outranks existing settled-state rendering.
- A terminal asynchronous launch result closes its tool card but does not end
  the launched workload. Work remains live until accountable completion or
  execution teardown.
- The delete confirmation warns when a task has foreground or recognized
  background work. Archive uses the same warning only when archive confirmation
  is enabled.

## API surface

- Session records and boot payloads expose `foreground_activity` as
  `generating`, `background`, or absent when no fine-grained activity is known.
  `background` can be present after the coarse session state settles.
- Session records, boot payloads, activity/state notifications, task records,
  and `task.updated` always expose `active_subagent_count` as an integer. It is
  zero when no adapter-attested subagent is live and may be positive while
  `foreground_activity` is either `generating` or `background`.
- `session.activity_changed` publishes the changed fine-grained session value
  and active subagent count; a count-only transition is publishable even when
  the activity tier does not change. `session.state_changed` carries both with
  coarse state changes.
- Task records and `task.updated` carry the most-active-wins
  `foreground_activity` aggregate and the sum of active subagents across its
  sessions. A task update is emitted when either aggregate changes, including a
  generating-to-background or count-only transition with no coarse state
  change.

## State machine

| State | Entry | Exit |
| --- | --- | --- |
| generating | A prompt is claimed or dispatched, or known top-level foreground work emits output. | The current prompt yields or completes, unless more foreground activity arrives. |
| background-running | The foreground is idle and one or more recognized workloads remain registered. | A foreground prompt/output takes precedence, or the final workload completes or is torn down. |
| idle/done | Neither foreground ownership nor recognized background work remains. | A foreground prompt/output or recognized workload begins. |

Prompt admission is atomic: only one prompt can claim foreground ownership for
a session. Delayed release or completion from an earlier prompt cycle cannot
mutate a later accepted claim. Prompt delivery is serialized per agent
execution except for one generation-matched human ownership transfer at an
adapter-attested provider handoff. The transfer finalizes the earlier lifecycle
buffer/completion owner before the successor starts; synthetic wakeups cannot
consume it, and a delayed response from the earlier RPC cannot finalize the
successor.

## Failure modes

- An unknown activity value for an in-flight `RUNNING` session is rendered as
  generating, not done.
- A foreground-idle signal without a nonzero matching prompt generation and
  adapter-attested handoff capability does not relax transport serialization.
  The session remains visibly foreground-owned and the follow-up remains
  queued rather than being reported as delivered.
- A normalized tool-card shape cannot attest to background capability. An
  unsupported adapter that emits a subagent-looking card remains foreground-busy
  until its launch, yield, and terminal lifecycle are explicitly supported.
- The development/E2E mock provider may replay captured Claude lifecycle
  metadata through the same attested path. This is a deterministic test seam,
  not an additional production-agent capability.
- A supported adapter must retire the original registration when a terminal
  child event arrives under a different tool-call ID. Codex child thread/session
  identity is the correlation key; interrupt, cancellation, failure, shutdown,
  and normal completion are terminal.
- Tool-call ownership is established by the initial call and retained across
  incremental updates. An update with unknown ownership preserves the current
  activity; missing parent metadata is not evidence that background-child work
  became foreground work.
- A task aggregate that cannot be recomputed preserves its last-known value
  rather than publishing a spurious done reading.
- When provider completion identifies a workload, only that registration is
  retired; duplicate completion is harmless. An uncorrelated completion retires
  only one outstanding registration and leaves an ambiguous remainder live.
- Execution stop, failure, cancellation, session removal, and teardown retire
  all registrations owned by that execution. Per-task publication is FIFO: a
  newly computed activity value must not be published ahead of an earlier value
  for the same task, and stale publication work must not overwrite a newer
  aggregate.

## Persistence guarantees

Fine-grained activity is in memory and is authoritative only while the owning
agent execution remains connected. A backend or agent-execution restart does
not reconstruct detached work or active subagent counts; it must not preserve a
stale live reading. Counts are derived from the live registration map rather
than persisted or incremented independently. Durable coarse state continues to
survive as before.

## Scenarios

- **GIVEN** a foreground-idle session with a recognized background workload,
  **WHEN** the operator sends a prompt, **THEN** the prompt is accepted while
  the status remains background-running until foreground activity begins.
- **GIVEN** an attested provider keeps a prompt RPC open for a detached child
  after emitting foreground-idle for the matching generation, **WHEN** the
  operator sends a follow-up, **THEN** that human prompt reaches the provider
  immediately and the detached child may continue streaming.
- **GIVEN** a synthetic wakeup is waiting when an attested human handoff occurs,
  **WHEN** a human follow-up is submitted, **THEN** the human prompt inherits
  ownership and the wakeup remains serialized until the human turn completes.
- **GIVEN** the provider has not authoritatively released the foreground,
  **WHEN** the operator submits a follow-up, **THEN** Kandev reports it as
  queued and does not send it concurrently.
- **GIVEN** two adapter-attested subagents and one background shell are live,
  **WHEN** the session is serialized or publishes an activity update, **THEN**
  `active_subagent_count` is two while all three workloads still contribute to
  background liveness.
- **GIVEN** one generating session with one live subagent and another session
  with two live subagents, **WHEN** the task is serialized, **THEN** its
  `foreground_activity` is generating and its `active_subagent_count` is three.
- **GIVEN** a Codex subagent launch and a later terminal child activity under a
  different tool-call ID, **WHEN** both identify the same child thread, **THEN**
  the original registration is retired and the count decreases exactly once.
- **GIVEN** an unsupported ACP adapter emits a normalized subagent-shaped tool
  card, **WHEN** its prompt remains in flight, **THEN** Kandev reports the
  foreground as generating and the active subagent count remains zero.
- **GIVEN** a task with one generating session and one background-running
  session, **WHEN** its aggregate is rendered, **THEN** it shows generating.
- **GIVEN** a task with no generating session and one background-running
  session, **WHEN** its aggregate is rendered on a board, list, graph, header,
  or sidebar, **THEN** it does not show done.
- **GIVEN** a detached workload launch completes, **WHEN** no workload terminal
  signal has arrived, **THEN** the session stays background-running.
- **GIVEN** a child tool call was attributed to recognized background work,
  **WHEN** an incremental update temporarily omits its parent metadata,
  **THEN** the session remains background-running.
- **GIVEN** a freshly loaded page or a second browser tab, **WHEN** a connected
  session has current background activity, **THEN** its task and session
  surfaces show background-running without waiting for a transition.
- **GIVEN** an activity transition for a task, **WHEN** a later transition is
  computed before earlier publication completes, **THEN** observers never see
  the later value followed by the stale earlier value.
- **GIVEN** a task with active foreground or recognized background work,
  **WHEN** the operator deletes it, **THEN** the confirmation warns that work is
  still in progress.
- **GIVEN** a task with active foreground or recognized background work,
  **WHEN** the operator archives it with archive confirmation enabled, **THEN**
  the archive confirmation carries the same warning.
- **GIVEN** archive confirmation is disabled, **WHEN** the operator archives a
  task with active foreground or recognized background work, **THEN** no archive
  dialog is shown.

## Out of scope

- Mid-turn steering for agents without concurrent-prompt capability.
- Reconstructing detached-work liveness after backend or agent-execution
  restart.
- Rendering the active subagent count or individual subagent details in the UI.
- Changing Office autonomous-agent status vocabulary.
- Changing archive behavior when the operator has disabled archive
  confirmation.

## Decision record

[ADR 0049 — Fine-grained foreground-idle busy signal](../../decisions/0049-fine-grained-foreground-idle-busy-signal.md) owns the architectural reasoning: ownership and admission rules, provider attestation, and the per-task publication-order invariant.

## Implementation plan

[Background work liveness implementation plan](../../plans/background-work-liveness/plan.md)
