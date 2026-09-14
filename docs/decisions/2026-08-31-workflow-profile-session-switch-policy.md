# ADR-2026-08-31-workflow-profile-session-switch-policy: Separate workflow step session start and end behavior

**Status:** accepted
**Date:** 2026-08-31
**Area:** workflow

## Context

Fixed-profile workflow steps need explicit conversation boundaries. A workflow
can return to an earlier agent conversation or start a fresh one.

The first design used one three-value policy. Each value combined two decisions:

- How the destination step obtains its session.
- What happens to the source step's session.

This combination was difficult to explain. It also omitted one valid outcome:
start a new destination session and complete the source session.

## Decision

Each workflow step stores two independent portable settings beside
`agent_profile_id`:

- `profile_session_start_policy` is `reuse` or `new`.
- `profile_session_end_policy` is `complete` or `park`.

The destination step supplies the start setting. The source step supplies the
end setting.

The defaults are `reuse` and `complete`. These defaults preserve existing
behavior. The router reuses an eligible nonterminal destination session when one
exists. It completes the source session.

For profile-only routing, consecutive steps with the same effective profile
keep the active session when the destination uses `reuse`. A destination with
`new` creates a fresh conversation even when profile IDs match. The source
step's end policy applies when that conversation is replaced.

This paragraph was reconciled on 2026-09-11 with existing acceptance criterion
AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5. Profile comparison uses logical profile
IDs. Reuse preserves user-selected runtime model and reasoning overrides; it
does not compare or restore model settings. The
[repair package](../plans/workflow-same-profile-new-session/plan.md) tracks the
remaining implementation gap.

The workflow engine keeps its transition graph, actions, events, and states. The
transition integration must provide both source and destination settings to the
orchestrator session handoff.

Parked sessions use `WAITING_FOR_INPUT`. They retain messages, provider resume
identity, task environment, and executor profile. An execution-stamped stop
intent prevents a callback from repeating workflow advancement.

The editor presents profile choice and **Session lifecycle** in one selector.
The lifecycle view has separate **When this step starts** and **When this step
ends** groups. Actual profiles use the same `AgentLogo` treatment as the
new-task selector.

## Consequences

- Workflow authors can understand and select all four lifecycle combinations.
- One step owns its entry and exit behavior without a transition-edge setting.
- Persistence, APIs, portable formats, sync, frontend state, and tests gain two
  fields instead of one enum.
- The unshipped three-value field is removed. The system has one source of truth.
- The workflow engine core does not change.
- The engine integration and orchestrator handoff must carry both sides of the
  transition.
- `park` can accumulate nonterminal sessions.
- `reuse` preserves provider conversation context.
- `new` always creates a fresh provider conversation.
- The orchestrator still matches deliberate stops by execution identity.

## Alternatives Considered

- Keep the three combined choices and improve their descriptions: rejected
  because the model still omits `new` plus `complete`.
- Store both decisions on the destination step: rejected because retirement is
  behavior of the step being left.
- Store both decisions on a transition edge: rejected because step entry and
  exit behavior must remain consistent across predecessor and successor edges.
- Use two booleans: rejected because named enums are clearer in APIs, portable
  files, logs, and tests.
- Add a `PARKED` task-session state: rejected because
  `WAITING_FOR_INPUT` already represents a stopped, resumable session.

## Proposed extension: explicit session targets (2026-09-09)

The [recipient design](../specs/tasks/system-design/workflow-profile-session-lifecycle.md#explicit-recipient-contract)
adds initial-session and earlier-step references beside legacy profile routing.
This extension is pending implementation through the
[session-targeting plan](../plans/workflow-session-targeting/plan.md).

An explicit target resolves a conversation identity before applying the start
setting. Its `new` setting must work even when profiles match. The source step
still owns retirement, and terminal sessions remain unavailable for reuse.

An initial reference retains the original identity. A source-step reference
uses that step's latest successful session selection. Deliver initial targeting
first, using the existing original marker and a write-once task metadata
snapshot that survives session deletion. A task metadata route record makes
fresh selection retry-safe from the first slice. Add durable source-step
bindings in the second slice; initial provenance stays in the snapshot.
Asynchronous history remains audit evidence rather than selection authority.

Matching only by profile was considered, but cannot distinguish two source
steps using the same profile or the original conversation from a newer tab.
Reinterpreting **No profile override** as initial was considered, but would
change existing workflows. Explicit references keep that compatibility path.

Portable version 2 is used when targets exist. Adding routing fields to version
1 was considered, but an older reader could silently ignore them and deliver a
prompt to another agent. Version rejection is the required compatibility result.
