---
status: draft
system: agents
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-001
  - REQ-AGENTS-ACP-SESSION-EVENTS-002
  - REQ-AGENTS-ACP-SESSION-EVENTS-003
  - REQ-AGENTS-ACP-SESSION-EVENTS-004
  - REQ-AGENTS-ACP-SESSION-EVENTS-005
---

# ACP Session Events System Design

## Purpose and boundaries

The ACP adapter translates schema 1.28 compaction and notice updates into existing
normalized streams. Message persistence, durable delivery, and turn lifecycle keep
their current owners. The usage-derived count in the
[context compaction count](../../ui/requirements/context-compaction-count.md)
contract does not change.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-ACP-SESSION-EVENTS-001` | [SDK pin](#sdk-pin) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-002` | [Compaction as a tool-call stream](#compaction-as-a-tool-call-stream) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-003` | [Notices as status messages](#notices-as-status-messages) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-004` | [Capability advertisement](#capability-advertisement) |
| `REQ-AGENTS-ACP-SESSION-EVENTS-005` | [acpdbg](#acpdbg) |

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
