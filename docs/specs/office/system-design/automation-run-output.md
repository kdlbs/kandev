---
status: draft
system: office
created: 2026-11-12
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
owners:
  - kandev
---

# Automation run output capture guidance

## Purpose and boundaries

Office owns this outcome because it owns automation run reading and its
selected run identity. This design extends [automation runs](automation-runs.md)
for AC-OFFICE-AUTOMATION-RUNS-001.13 through .18.

The [UI notice design](../../ui/system-design/empty-turn-notice.md#passthrough-exclusion)
owns the independent, reusable exclusion of passthrough from empty-turn feedback.
Task messages, agent execution, and terminal buffers retain their existing owners.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-OFFICE-AUTOMATION-RUNS-001.13, .14 | Session identity and presentation |
| AC-OFFICE-AUTOMATION-RUNS-001.15 | Shared empty-turn filtering |
| AC-OFFICE-AUTOMATION-RUNS-001.16 | Passive viewing and retained interaction |
| AC-OFFICE-AUTOMATION-RUNS-001.17 | Desktop and phone composition |
| AC-OFFICE-AUTOMATION-RUNS-001.18 | Session identity and presentation |

## Session identity and presentation

`RunTranscript` retains `useEnsureTaskSession(sessionId)`. Hidden automation
tasks can be absent from boot data, so a direct URL needs this hydration path.
Read the selected session from the existing `useChatPanelState` result.
Use `isPassthroughSession(panelState.session)` to decide whether guidance appears.
Explicit session mode takes precedence over `agent_profile_snapshot.cli_passthrough`.
The helper supports the legacy snapshot when the session flag is absent.

Do not derive mode from the automation's editable profile or the globally active
task session. Existing run/session keys remain responsible for selection changes.
Before the selected row loads, retain existing loading behavior without assuming
that the automation's current profile describes a historical run.

Show a non-dismissable informational block above the existing message region.
Its English text has two parts:

> CLI passthrough output is not captured in this run's transcript.
>
> Disable CLI passthrough in the automation's agent profile to record output for future sessions. Previous terminal output cannot be recovered here.

Use `t()` with keys in the `automations` namespace. Add every supported locale
and regenerate the Traditional Chinese and pseudo catalogs through their scripts.
The copy describes capture only. It does not assert success, failure, or that
the agent produced nothing. Actual run status and errors remain authoritative.

The explanation appears in both running and parked sessions. The message list
remains available underneath, including any structured output from earlier turns.
The composer retains its current submission and cancellation handlers.

## Shared empty-turn filtering

Apply the shared UI exclusion at both notice creation and message filtering.
This handles a completion event that precedes session hydration. Do not replace
all status entries with the explanation or filter the message list to user text.
Scope the filter to `metadata.empty_turn === true` for a passthrough session.

The guidance is presentation state. Do not store it as an agent message,
synthetic turn, automation summary, or new run outcome.

## Passive viewing and retained interaction

Do not mount `PassthroughTerminal` or `AgentTerminalInput` in this read path.
The agent-terminal WebSocket handler calls `EnsurePassthroughExecution`; viewing
that terminal can start or restart an execution. Its receive-only client option
does not remove the server-side execution effect.

`GetPassthroughBuffer` reads a process buffer through an existing execution.
It is not an API for durable, run-scoped history. This repair does not attach to
that buffer, invent a report, parse a provider transcript, or restart an agent.
An explicit user reply continues through the existing submission path.

The work adds no event fields, database columns, runtime flags, or permissions.
No ADR is required because this local presentation correction preserves existing
execution and persistence ownership. Durable terminal recording needs a separate design.

## Desktop and phone composition

Retain the desktop runs rail and the phone's existing selected-run route with
its runs drawer. `AutomationDetailPage` and `mobile-automation-detail.spec.ts`
provide the nearest shipped feature composition. The curated mobile language's
dense-reader rule also favors this existing full-height reading surface.

Both viewports share the same informational block and transcript logic.
The block wraps to the available width and needs no hover or additional action.
It sits above the transcript scroller. The message region remains the single
internal scroll owner, and the reply composer retains existing safe-area handling.
Keep dynamic viewport sizing and drawer focus behavior in their current owners.
Do not add a second scroller, desktop pane, or terminal key bar on phones.

At narrow widths, long translated guidance must remain readable without clipping
the composer or causing document-level horizontal scrolling. Existing controls
retain desktop density and phone touch targets. No new control is required.

## Failure and compatibility behavior

- Missing or unavailable session data retains existing loading/error behavior.
  It must not trigger a terminal connection or a profile lookup to infer mode.
- A structured session keeps its transcript and normal empty-turn feedback.
- Selecting another session recalculates the explanation and the filtered list.
- Explicit `is_passthrough=false` overrides a conflicting legacy snapshot.
- Editing a profile affects future session creation through existing behavior.
  It does not recover old PTY output or change the stored mode of an existing session.
- The same explanation applies to any provider that uses passthrough.
  It makes no provider-specific transcript or terminal-retention promise.

## Verification

Real-store component tests cover hydration, running and parked states, selection,
preserved errors and replies, and a warning inserted before hydration.
Unit and event-handler tests cover the shared exclusion and structured controls.

Focused desktop and Pixel 5 browser tests load hidden runs from their URL,
reload, switch between structured and passthrough sessions, and read the guidance.
They assert retained status/composer behavior and no terminal connection or
automatic start/resume request. A persisted fixture mode must survive reload.

See the [fix package](../../../plans/automation-passthrough-output/plan.md)
for exact test names, owned files, commands, and reproduction evidence.
