---
status: shipped
created: 2026-07-21
updated: 2026-07-28
owner: kandev
---

# Background Work Liveness

## Why

ACP providers do not yet expose a consistent, accountable lifecycle for
subagents and other background work. Kandev can account for recognized work,
but that best-effort signal is not reliable enough to decide whether a
`RUNNING` agent can safely receive another prompt by default. Prompt admission
and operator-visible busy state therefore use the durable coarse session
lifecycle unless a deployment deliberately enables the Claude-only experiment.

## What

- Every `RUNNING` session is busy, rejects direct prompt admission, and routes
  composer input through the queued-message path by default.
- Every `RUNNING` session is shown as generating. Background-work accounting
  does not select a separate operator-visible activity tier by default.
- A settled session follows its coarse state and does not remain visually busy
  solely because detached work is still registered.
- The runtime retains one registration per recognized live subagent and derives
  `active_subagent_count` from those registrations. Background shells and
  Monitor watches may remain internally tracked but do not contribute to the
  subagent count.
- Adapter attestation is accounting evidence only unless the
  `features.claudeBackgroundPromptHandoff` experiment is enabled for a
  `claude-acp` session.
- When that experiment is enabled, all Claude modes covered by ADR-0049 retain
  their fine-grained behavior: subagents, `run_in_background` shells, and
  Monitor watches may expose background activity and a generation-matched
  foreground handoff may admit the next prompt.
- Non-Claude providers remain coarse even when the experiment is enabled.
- The experiment is off in every embedded profile, high risk,
  restart-required, and available through
  `KANDEV_FEATURES_CLAUDE_BACKGROUND_PROMPT_HANDOFF`.
- Terminal execution teardown retires every tool-ownership and background-work
  entry owned by that execution. It releases the whole session activity record
  when no successor execution or in-flight prompt/dispatch token still owns it.

## API surface

- A `RUNNING` session record, boot payload, activity notification, or state
  notification exposes `foreground_activity=generating` by default. An
  eligible session in the enabled Claude experiment may expose `background`.
- Settled session records omit `foreground_activity`; task records aggregate
  only coarse running activity.
- `session.activity_changed` carries `task_id`, `session_id`,
  `foreground_activity`, and `active_subagent_count`. Its activity is
  `generating` while the session is running; count-only changes may publish
  without an activity-tier change.
- Session records, boot payloads, activity/state notifications, task records,
  and `task.updated` expose `active_subagent_count` as an integer. It is zero
  when no adapter-attested subagent is live.

## State machine

| Coarse state | Prompt admission | Operator activity |
| --- | --- | --- |
| `RUNNING` | Queue/reject by default; enabled Claude handoff may accept | generating by default; enabled Claude may show background |
| `WAITING_FOR_INPUT`, `COMPLETED`, or `IDLE` | Accept | omitted |
| `STARTING`, `CREATED`, `FAILED`, or `CANCELLED` | Reject as not promptable | omitted |

The background-work tracker may still transition internally between foreground
ownership and recognized background liveness. Those transitions do not change
the table above.

## Failure modes

- With the experiment off, missing, delayed, duplicated, or provider-specific
  background lifecycle frames cannot make a `RUNNING` session promptable.
- With the experiment on, only adapter-attested Claude lifecycle frames can
  relax the gate. Missing provider identity and every non-Claude provider fail
  closed.
- A normalized tool-card shape cannot attest to promptability.
- Codex child thread/session identity and Claude task notifications may be used
  for presentation or accounting, but neither changes the coarse admission
  rule.
- Count drift is prevented by deriving `active_subagent_count` from the live
  registration map rather than maintaining an independent counter.
- Execution completion, stop, failure, cancellation, crash cleanup, forced
  cleanup, and session removal retire activity owned by that execution.

## Persistence guarantees

Background-work accounting is in memory and authoritative only while the owning
agent execution remains connected. A restart does not reconstruct detached work
or active subagent counts. Durable coarse session state remains the source of
truth for prompt admission and operator-visible activity.

## Scenarios

- **GIVEN** the experiment is off and a `RUNNING` session's tracker reports
  foreground-idle with recognized background work, **WHEN** the operator
  submits a prompt, **THEN** Kandev queues it.
- **GIVEN** the experiment is on for a `claude-acp` session and its tracker
  reports adapter-attested background work plus a generation-matched foreground
  handoff, **WHEN** the operator submits a prompt, **THEN** Kandev dispatches the
  successor without queueing it.
- **GIVEN** the experiment is on for any non-Claude provider, **WHEN** the same
  tracker-shaped activity is observed, **THEN** Kandev still queues the prompt.
- **GIVEN** the same session is rendered or freshly reloaded, **WHEN** its
  activity is serialized, **THEN** it is shown as generating rather than
  background-running.
- **GIVEN** two adapter-attested subagents and one background shell are live,
  **WHEN** activity is serialized, **THEN** `active_subagent_count` is two while
  the session's operator activity still follows its coarse state.
- **GIVEN** the session leaves `RUNNING`, **WHEN** its DTO or task aggregate is
  serialized, **THEN** foreground activity is omitted even if detached work
  remains registered.
- **GIVEN** an execution terminates with orphaned tool ownership and background
  work, **WHEN** teardown runs, **THEN** its owned accounting state is released.

## Out of scope

- Mid-turn steering for agents without concurrent-prompt capability.
- Reconstructing detached-work liveness after restart.
- Rendering active subagent counts or individual subagent details in the UI.

## Decision record

[ADR-2026-07-28 — Restore Coarse Running Prompt Admission](../../decisions/2026-07-28-coarse-running-busy-signal.md)
supersedes the operator policy in
[ADR 0049 — Fine-grained foreground-idle busy signal](../../decisions/0049-fine-grained-foreground-idle-busy-signal.md).

## Implementation plan

[Coarse running busy signal fix plan](../../plans/coarse-running-busy-signal/plan.md)
