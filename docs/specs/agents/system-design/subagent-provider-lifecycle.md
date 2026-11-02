---
status: draft
system: agents
requirements:
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-001
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-002
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-003
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-004
---

# Subagent Provider Lifecycle System Design

## Purpose and boundaries

Provider-specific interpretation stays in `dialect_<agent>.go` and small adapter state files
in `apps/backend/internal/agentctl/server/adapter/transport/acp/`, under ADR-0044. Chat keeps
using `SubagentTaskPayload` and `tool-subagent-message.tsx`. Background liveness, prompt
admission, and session activity stay with their existing owners.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-SUBAGENT-LIFECYCLE-001` | [Codex outcomes](#codex-outcomes) |
| `REQ-AGENTS-SUBAGENT-LIFECYCLE-002` | [Copilot dialect](#copilot-dialect) |
| `REQ-AGENTS-SUBAGENT-LIFECYCLE-003` | [Claude settlement and resume](#claude-settlement-and-resume) |
| `REQ-AGENTS-SUBAGENT-LIFECYCLE-004` | [Control labels](#control-labels) |

## Wire evidence

These agents were captured live on 2026-10-30 and 2026-11-01 at their pinned versions.

| Agent | Spawn | Child calls | End evidence |
| --- | --- | --- | --- |
| Codex 2.2.2 | `spawnAgent`, title-less update carrying the child | Not emitted | `wait` and `closeAgent` `rawInput.agentsStates` |
| Copilot 1.0.88 | `rawInput.{agent_type,name,prompt,mode}`, no `_meta` | `_meta["github.com/copilot"].agentId`, not on the parent | Parent `rawOutput.content`; background prose has `agent_id` |
| Claude 0.81.2 | `_meta.claudeCode.toolName == "Agent"`; `toolResponse.agentId` | `parentToolUseId` | Prompt end; `task-notification` usage has no child identity |

In a parallel Copilot capture, the first child call belonged to the second parent. Arrival order
is therefore not a link. Claude's `asyncTasks` capability explicitly ignores subagent tasks.

## Codex outcomes

`trackToolCallPayload` records `wait` and `closeAgent` tool IDs in per-session Codex control
state when a validated Codex frame carries a `senderThreadId`. Their update frames have no title,
so this state identifies them.

A new `codexControlCardUpdates(n)` step in `handleACPUpdate` runs before the control row's own
event. When a tracked control update has `status: completed`, it reads `agentsStates`:

1. Each child resolves through `findCodexSubagentByChildLocked`. Unknown children are ignored.
2. `fillCodexStatus` applies only through the existing `codexSubagentStatusRank`, so a terminal
   state is never downgraded. A non-empty `message` sets `ResultText`.
3. For `closeAgent`, a receiver that is still non-terminal becomes `shutdown`, because the captured
   frame reports `running` at the moment of closure.
4. Each changed card emits `EventTypeToolUpdate` with its `emittedToolCallID`, stamped by
   `stampUpdateTurnIdentity`.

The control row keeps its own event. `codexSubagentSignalFromFrame` does not change, so capacity
evidence and the recognition rules in the wire-compatibility contract stay the same.

## Copilot dialect

`dialect_copilot.go` defines `newCopilotACPDialect`, selected in `newACPDialect` for
`copilot-acp`.

- `subagentFrame` accepts a nonblank `agent_type` and `prompt`. Description, name, and type map
  onto the frame. The chat card shows type, name, and description in its header; expanded Copilot
  content shows the result and child calls, followed by the prompt. `mode: "background"` sets
  `IsAsync`. `agent_type` is read only here, so other agents are unaffected.
- A new dialect hook, `subagentResult(rawOutput)`, reads `rawOutput.content` for a foreground
  result. For a background launch it matches `agent_id: <uuid>` in the same text and sets
  `AgentID` and status `async_launched`. A mismatch leaves the card without an ID.

`adapter_copilot_subagents.go` holds per-session state: open foreground cards, background
cards by agent ID, background launches whose ID is not yet known, and bindings from child ID to
card. For a child call:

1. An existing binding wins.
2. A background card with the same ID binds.
3. If any background launch has no known ID, the relationship is ambiguous and stays top-level.
4. Otherwise exactly one open, unbound foreground card binds, excluding the calling card itself.
5. Otherwise `ParentToolCallID` stays empty and the call may bind later under the same rules.

Completion removes a card from the open set but keeps its binding. Session clear releases the
state. `stampSubagentBackgroundWork` remains limited to Claude and mock, so Copilot cards never
create liveness state.

## Claude settlement and resume

`adapter_claude_subagents.go` records, per session, each Claude subagent card with an agent ID,
including foreground cards, with its tool call ID, output file, and prompt generation. The record
also tracks whether the card was async and which prompt resumed it.

**Settlement.** The prompt completion path in `adapter_prompt.go`, before it emits
`EventTypeComplete`, calls `settleClaudeSubagents(sessionID, generation, stopReason)`. It acts
only for `end_turn` and when the prompt was not handed off (`markPromptHandoff`). Async cards
started by that prompt and foreground cards resumed during that prompt get status `completed` and
a tool update. Other foreground cards and other stop reasons leave cards as they are. A newly
readable non-empty report replaces the existing result; a missing report preserves that result.

**Report.** Settlement reads the final report on a bounded best-effort basis:

- `filepath.Base(outputFile)` must equal `<agentId>.output`, so the agent-reported path can only
  name that subagent's file. Symlinks are followed, as the provider's task file is one.
- Read at most the final 256 KiB, parse JSON lines from the end, and take the text blocks of
  the last `type: "assistant"` entry.
- The file format is private to Claude. Any read or parse failure leaves `ResultText` empty and
  logs at debug. Agentctl reads it so that remote executors work without host file access.

**Resume.** A `SendMessage` tool call whose `rawInput.to` equals a recorded `AgentID` increments
`SubagentTaskPayload.ResumeCount`, sets status `running`, and records the current generation for
settlement. Child calls keep nesting through the original `parentToolUseId`. `SendMessage` reports
`to` on an update frame, so the check runs on calls and updates, once per tool call ID.

**Liveness isolation.** These presentation updates must not change `BackgroundWork`. The
orchestrator registers only the initial `async_launched` detached subagent update as new work;
settlement and resume updates only describe the card. A test completes background work before
each presentation update and asserts that the session count and background entries stay at zero.

## Control labels

`GenericPayload.SubagentControl` is a closed set: `wait`, `close`, `message`, `read`, `list`,
`resume`. Dialects set it by exact provider tool name.

| Agent | Tool | Value |
| --- | --- | --- |
| Codex | `wait`, `closeAgent`, `sendInput`, `followupTask`, `resumeAgent` | `wait`, `close`, `message`, `message`, `resume` |
| Copilot | `read_agent`, `write_agent`, `list_agents` | `read`, `message`, `list` |
| Claude | `SendMessage`, from `_meta.claudeCode.toolName` | `message` |

Codex tools also require the validated collaboration input fields, so another tool named `wait`
is untouched. `tool-call-message.tsx` maps the value through `t()` to a label, and falls back
to `prettifyToolTitle`.

## Failure and recovery

All inputs are parsed defensively. A frame that fails recognition keeps its current generic
rendering. Adapter state is process-local: after a restart, an old card keeps its last persisted
status and stops appearing active when no turn is running.

## Persistence

`ResumeCount` is a new optional JSON field in existing message metadata. No schema change.

## Security

`ResultText` is untrusted display content and uses the existing subagent rendering. The output-file
basename check limits which agent-reported path agentctl reads, and the agent already has the same
file access as agentctl.

## Observability

Existing ACP event logging applies. Report-read failures log at debug level.

## Related decisions

- [ACP compatibility dialects](../../../decisions/0044-acp-agent-compatibility-dialects.md)
- [Foreground idle busy signal](../../../decisions/0049-fine-grained-foreground-idle-busy-signal.md)
