---
status: draft
system: agents
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-001
  - REQ-AGENTS-ACP-SESSION-EVENTS-002
  - REQ-AGENTS-ACP-SESSION-EVENTS-003
  - REQ-AGENTS-ACP-SESSION-EVENTS-004
  - REQ-AGENTS-ACP-SESSION-EVENTS-005
  - REQ-AGENTS-ACP-SESSION-EVENTS-006
  - REQ-AGENTS-ACP-SESSION-EVENTS-007
---

# ACP Session Events System Design

## Purpose and boundaries

The ACP adapter translates schema 1.28 compaction and notice updates into existing
normalized streams. Message persistence, durable delivery, and turn lifecycle keep
their current owners. The usage-derived count in the
[context compaction count](../../ui/requirements/context-compaction-count.md)
contract does not change through standard timeline events.
The [Copilot native design](copilot-native-compaction.md) defines a separate evidence-driven counter path.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-ACP-SESSION-EVENTS-001` | [SDK pin](#sdk-pin) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-002` | [Compaction as a tool-call stream](#compaction-as-a-tool-call-stream) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-003` | [Notices as status messages](#notices-as-status-messages) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-004` | [Capability advertisement](#capability-advertisement) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-005` | [acpdbg](#acpdbg) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-006` | [Copilot interrupted-call recovery](#copilot-interrupted-call-recovery) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-007` | [Routine Copilot plugin notices](#routine-copilot-plugin-notices) |

## SDK pin

`apps/backend/go.mod` replaces `github.com/coder/acp-go-sdk` with
`github.com/kdlbs/acp-go-sdk v0.13.6-0.20261101161427-70cc752b95c6`. Fork PR #7 was
squash-merged, so the previous PR-head pin is not an ancestor of fork `main`.

## Compaction as a tool-call stream

Compaction uses the tool-call stream because that path already provides in-place
upsert, persistence, durable replay, retention, and end-of-turn settlement.

`streams.ToolKindContextCompaction` carries `ContextCompactionPayload`:

| Field | Source |
| --- | --- |
| `compaction_id` | `compactionId` |
| `status` | `in_progress`, `completed`, `failed`, `cancelled`; other values become `in_progress` |
| `summary` | `summary` text blocks, or accumulated `compaction_summary_chunk` text |
| `error` | `error` |
| `trigger`, `pre_tokens`, `post_tokens`, `duration_ms` | `_meta.contextCompaction`, read defensively |

`apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session_events.go`
owns the conversion:

1. The first update for an unknown identity emits `EventTypeToolCall` with the tool
   call ID `compaction:<session>:<compactionId>`.
2. Later updates and chunks mutate the stored payload and emit `EventTypeToolUpdate`.
3. `Optional[T]` semantics apply: omit keeps, null clears, and a value replaces. An
   empty summary list also clears the summary.
4. The summary is capped at 64 KiB of retained text, and the payload uses the existing
   display-budget preparation. A chunk for an unknown identity is dropped.

Non-Claude dialects read only the standard fields. Reading `_meta.contextCompaction`
is safe for all agents because absent or invalid fields leave the values empty.

`handleACPUpdate` adds compaction and notice updates to its load-replay suppression
list. Compaction events do not touch `usageTracker` or context-window persistence.

### Web rendering

`message-renderer.tsx` matches `metadata.normalized.kind === "context_compaction"`
before the generic tool renderer and renders `ContextCompactionMessage`. The component
derives "active" from the payload status and containing turn, as
`isSubagentEffectivelyActive` does, so a stale in-progress row stops appearing active.
The summary renders through the existing bounded markdown preview. The turn group treats
the row like a subagent card, so it stays visible at the top level. Copy uses `t()` in
all seven locales.

## Notices as status messages

The [standard notice work order](../../../plans/acp-schema-1-28-adoption/task-04-provider-notices.md)
records the prerequisite for committing a notice with its delivery cursor.

`EventTypeSessionNotice` carries `SessionNotice{Severity, Title, Description}` through
`AgentStreamEventPayload` to `handleSessionNoticeEvent`. The handler creates one
`MessageTypeStatus` row with metadata `{variant, notice: true, text}`:

- `variant` is `info`, `warning`, or `error`; unknown values become `info`.
- `content` is the title and `text` is the description. `StatusMessage` already renders
  variant styling, plain text, and expandable details.
- The handler does not change session state, publish a failure, or enter
  `transient_retry` paths. `notice: true` excludes the row from failure-specific
  handling such as `isRetainedTurnFailure` and running-notice activity.

The notice has no provider identity. Exactly-once behavior therefore relies on the
durable delivery cursor. `agent_delivery_retention_proof.go` classifies the event as
projected state so its row remains recoverable. A test replays one delivered notice
across a reconnect and asserts one row. If the cursor does not provide that guarantee,
implementation stops and reports the gap instead of hashing notice text.

## Copilot interrupted-call recovery

### Recognize the ACP sequence

Copilot CLI 1.0.114 replays a pending `tool_call` during `session/load`. On the
first `session/prompt` it sends a failed `tool_call_update` with
`rawOutput.code: interrupted`, then an ID-less warning `agent_message_chunk`.
The [fix plan](../../../plans/copilot-interrupted-tool-notice/plan.md#evidence)
records the captured order and the qualified warning text.

During load, the ACP notification worker keeps one possible pending question
ID for the loading session. The exact title is
`kandev-ask_user_question_kandev` with `kind: other`. Use the existing Core MCP
delivery evidence to confirm the server. A terminal replay update clears its ID.
Competing pending IDs make the question identity uncertain. Replay follows the
normal suppression path.

Use this evidence during the first sent prompt after a successful load, scoped
to the native session and prompt generation. Match the exact single-chunk
warning template and a preceding failed update with a call ID and
`rawOutput.code: interrupted`. Use the clarification classification only if
the warning covers one call and the single observed failed ID matches the question.
Use the generic classification for other or uncertain calls. One question ID,
one failed call ID, and ambiguity flags suffice.

The existing load and prompt boundaries own the lifetime of this state. A
replacement, failed load, or the end of the first prompt clears it. Unrelated
session updates do not clear interruption evidence. Assistant content or a new
tool start closes the matching window. The Copilot dialect owns the literal
checks. Unrecognized frames follow regular ACP conversion.

### Project the status before the reply

Keep the original warning in `Text` on the existing `message_chunk`. Mark it
with `AgentEvent.Data.copilot_interruption_notice` set to `clarification` or
`tool`. Existing stream publication forwards the map. A peer that ignores
the field still receives the full warning as text.

Use a separate kind in the existing canonical message ID builder so reply
chunks stay in their own row. The canonical message insert uses
`MessageTypeStatus` for the marked chunk and copies the same field into
message metadata. The existing write commits the row and cursor together.
Resolve the owner from the event's prompt generation. Use the owned prompt turn
for placement and retain the text fallback if turn ownership is unavailable.
An unowned notice keeps its original text as a regular message.

Publish the committed row through `publishCanonicalMessageEvent`. For legacy
streams, use the existing `CreateSessionMessageIdempotent` path with a key
derived from session, execution, and prompt generation. Route the marked chunk
before assistant text buffering and activity accounting. Skip and Send follow
the existing clarification and prompt paths.

### Render the notice inline

`StatusMessage` selects the notice using the stored classification. A localized
summary uses `text-muted-foreground` at full opacity on the transcript
background. A labeled inline Details control shows the original warning as
wrapping plain text.
The shared row follows the disclosure pattern in `ContextCompactionMessage`.
One short recovery paragraph fits the existing transcript scroller. The row
shares disclosure state and localization across viewports. On phone and coarse
pointers Details has a 44px or larger hit area and wraps below the summary when
needed. Desktop retains a compact control and a visible keyboard focus state.

Status persistence uses `task_session_messages`. Existing notification tracing
records the marked event on the same authorized session path. The marker and
status content carry the fixed warning and classification. Tool inputs, raw ACP
metadata, and raw provider output stay within the existing transport boundaries.

## Routine Copilot plugin notices

Register a notification predicate on the Copilot dialect. Match only text in an
`AgentMessageChunk` with a nil `MessageId`. Compare the original text against
the two case-sensitive patterns in requirement 007. Evaluate dialect suppression
once before the interruption observer. Skip the observer for suppressed frames.
Keep the existing replay fast path and live suppression path. For a live Copilot
match, record the notice at debug level and emit no message event.

## Capability advertisement

`clientCapabilitiesForAgent` sets `Session.Notices` and `Session.Compaction` for every
agent. `clientCapabilitiesForInference` remains unchanged. Neither sets `Subagents` nor
`_meta.jetbrains.air`. Compaction and notice handling ship in the same change as
advertisement, because Claude replaces its tool-call and transcript fallbacks once it
sees the capability.

There is no release toggle. The events are additive, agents that ignore the capability
keep today's frames, and handling is per update. A failure in conversion logs and drops
that update without affecting the stream.

## acpdbg

`--client-capabilities '<json object>'` is parsed in `cmd/acpdbg/main.go` before spawn
and passed as `RunConfig.ClientCapabilities`. `initializeParams` shallow-merges it over
the defaults, then applies `--air-capabilities` under `_meta` without discarding other
`_meta` keys. `Prompt` selects a model through `session/set_config_option` when the
`session/new` response has a `model`-category select option, using the existing
`extractSelectConfigOption`. Otherwise it calls `sendSetModel`.

## Failure and recovery

Unknown severities and statuses degrade as defined above. Malformed updates decode to an
empty SDK union, reach `convertNotification` without a matching case, and keep today's
"unhandled ACP session notification" warning.

## Persistence

No schema change. Compaction entries are tool-call messages and notices are status
messages, both in existing tables. Rows created before this change keep their rendering.

## Security

Provider text is untrusted display content. Titles and descriptions render as plain text.
Summaries use the same markdown sanitization as agent messages.

## Observability

Existing raw and normalized ACP event logging covers both flows. No new metrics.

## Related decisions

- [ACP compatibility dialects](../../../decisions/0044-acp-agent-compatibility-dialects.md)
- [agentclientprotocol codex-acp](../../../decisions/0034-agentclientprotocol-codex-acp.md)
