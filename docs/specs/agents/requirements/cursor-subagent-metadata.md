---
status: draft
system: agents
created: 2026-08-20
owners:
  - cfl12
---
# Cursor Subagent Metadata Requirements

## Overview

When Cursor's agent dispatches a subagent, the only thing a Kandev user sees is a
generic `Task: Subagent task` card with no prompt, no model, and no identity. All
the useful detail (what the subagent was asked to do, which model runs it, its
agent id, whether it runs in the background) is delivered by Cursor over a
non-standard agent-to-client request, `cursor/task`, which Kandev currently
rejects. The subagent's work is therefore opaque, and a backgrounded subagent
looks identical to one that already finished.

## Requirements

### REQ-AGENTS-CURSOR-SUBAGENT-METADATA-001: Cursor Subagent Metadata

**Intent:** When Cursor's agent dispatches a subagent, the only thing a Kandev user sees is a
generic `Task: Subagent task` card with no prompt, no model, and no identity. All the useful detail
(what the subagent was asked to do, which model runs it, its agent id, whether it runs in the
background) is delivered by Cursor over a non-standard agent-to-client request, `cursor/task`, which
Kandev currently rejects. The subagent's work is therefore opaque, and a backgrounded subagent looks
identical to one that already finished.

#### Acceptance criteria

- **AC-AGENTS-CURSOR-SUBAGENT-METADATA-001.1:** A Cursor subagent card SHALL show the subagent's description, prompt, model, and (for background subagents) a background indicator, sourced from the `cursor/task` request Cursor emits alongside the standard `tool_call`.
- **AC-AGENTS-CURSOR-SUBAGENT-METADATA-001.2:** The rich metadata is correlated to the standard subagent `tool_call` by the shared `toolCallId` Cursor puts in both, so the existing subagent card is enriched in place rather than a second card appearing.
- **AC-AGENTS-CURSOR-SUBAGENT-METADATA-001.3:** `cursor/task` MUST NOT cause a JSON-RPC error back to Cursor. Kandev accepts the request and returns an empty success result.
- **AC-AGENTS-CURSOR-SUBAGENT-METADATA-001.4:** A `cursor/task` that arrives with no matching subagent `tool_call` (or before it) is retained and applied when the matching `tool_call` appears; a stored correlation that is never matched is bounded and dropped when the session ends.
- **AC-AGENTS-CURSOR-SUBAGENT-METADATA-001.5:** Cursor reports background dispatch two ways that both map to the payload's existing async concept: `cursor/task.model` ending in `-fast` is informational only, while the completion `tool_call_update.rawOutput.isBackground == true` marks the subagent as background. A background Cursor subagent renders the same "background" affordance Claude's async subagents use.
- **AC-AGENTS-CURSOR-SUBAGENT-METADATA-001.6:** No behavior changes for any non-Cursor agent, and no change to how Cursor's ordinary (non-subagent) tool calls render.
- **AC-AGENTS-CURSOR-SUBAGENT-METADATA-001.7:** **GIVEN** Cursor emits a subagent `tool_call` and a matching `cursor/task` request, **WHEN** both are processed, **THEN** the subagent card shows the Cursor description, prompt, and model, and Kandev returns a success (not `-32601`) for `cursor/task`.
- **AC-AGENTS-CURSOR-SUBAGENT-METADATA-001.8:** **GIVEN** a `cursor/task` arrives before its `tool_call`, **WHEN** the `tool_call` later arrives, **THEN** the card is enriched with the earlier metadata.

## Migrated source detail

## Why

When Cursor's agent dispatches a subagent, the only thing a Kandev user sees is a
generic `Task: Subagent task` card with no prompt, no model, and no identity. All
the useful detail (what the subagent was asked to do, which model runs it, its
agent id, whether it runs in the background) is delivered by Cursor over a
non-standard agent-to-client request, `cursor/task`, which Kandev currently
rejects. The subagent's work is therefore opaque, and a backgrounded subagent
looks identical to one that already finished.

## What

- A Cursor subagent card SHALL show the subagent's description, prompt, model,
  and (for background subagents) a background indicator, sourced from the
  `cursor/task` request Cursor emits alongside the standard `tool_call`.
- The rich metadata is correlated to the standard subagent `tool_call` by the
  shared `toolCallId` Cursor puts in both, so the existing subagent card is
  enriched in place rather than a second card appearing.
- `cursor/task` MUST NOT cause a JSON-RPC error back to Cursor. Kandev accepts
  the request and returns an empty success result.
- A `cursor/task` that arrives with no matching subagent `tool_call` (or before
  it) is retained and applied when the matching `tool_call` appears; a stored
  correlation that is never matched is bounded and dropped when the session ends.
- Cursor reports background dispatch two ways that both map to the payload's
  existing async concept: `cursor/task.model` ending in `-fast` is informational
  only, while the completion `tool_call_update.rawOutput.isBackground == true`
  marks the subagent as background. A background Cursor subagent renders the same
  "background" affordance Claude's async subagents use.
- No behavior changes for any non-Cursor agent, and no change to how Cursor's
  ordinary (non-subagent) tool calls render.

## Data model

No new persistent tables. The existing in-message
`streams.SubagentTaskPayload`
(`apps/backend/internal/agentctl/types/streams/tool_payload.go`) is the store;
its `Description`, `Prompt`, `SubagentType`, `Model`, `AgentID`, `DurationMs`,
and `IsAsync` fields already exist and already serialize to the frontend
`SubagentTaskPayload` (`apps/web/components/task/chat/types.ts`). This feature
populates the Cursor-shaped subset that is currently dropped; it adds no new
payload field unless correlation requires transient adapter-side state (a
per-session `map[toolCallId]cursorTask`, not persisted).

## API surface

`cursor/task` is an inbound agent-to-client JSON-RPC **request** (it carries an
`id`, so a response is required). Kandev is the ACP client. The method name does
not start with `_`, so the stock SDK extension hook
(`ExtensionMethodHandler`, only invoked for `_`-prefixed methods in
`kdlbs/acp-go-sdk` `extensions.go`) does not reach it, and the generated
`ClientSideConnection.handle` switch returns `-32601`.

Observed request params (wire capture, `composer-2.5`):

```text
cursor/task  (request, has id)
  agentId       string   e.g. "eaa12f70-...-60506ebd5c78"
  description   string   e.g. "Summarize src JS files"
  durationMs    number   launch duration, not the subagent's run time
  model         string   e.g. "composer-2.5-fast"
  prompt        string   the full subagent prompt
  subagentType  object   e.g. {"custom":{"unspecified":{}}} — opaque, not a string
  toolCallId    string   correlates to the standard tool_call, e.g. "tool_2494c23d-..."
```

Response: an empty JSON object result (no error).

The correlated standard frames (unchanged path): the `tool_call` carries
`rawInput:{"_toolName":"task"}` and title `Task: Subagent task`; the completion
`tool_call_update` carries `rawOutput:{"durationMs":N,"isBackground":true|false}`.

## Failure modes

- **Unknown/extra fields in `cursor/task`:** parsed defensively over untyped
  maps; missing fields stay zero-valued and the card degrades to whatever is
  present. Never errors the request.
- **`subagentType` is an object, not a string:** it is not coerced into the
  string `SubagentType` field; it is ignored unless a stable string label can be
  derived. A malformed value never blocks the rest of the metadata.
- **Ordering (request before or after the `tool_call`):** correlation by
  `toolCallId` is order-independent; whichever arrives second applies the merge.
- **No matching `tool_call` ever arrives:** the stored `cursor/task` is bounded
  per session and discarded at session teardown; it never leaks across sessions.
- **A field already set by the standard frame is not blanked** by a later empty
  Cursor value (mirrors `applySubagentResult`'s fill-if-present rule).

## Persistence guarantees

Captured metadata lives on the message payload and persists with the transcript
exactly as today. The transient `toolCallId -> cursorTask` correlation map is
in-memory adapter state and does NOT survive a restart; a subagent whose
`cursor/task` was seen only before a restart keeps whatever was already merged
onto its stored payload.

## Scenarios

- **GIVEN** Cursor emits a subagent `tool_call` and a matching `cursor/task`
  request, **WHEN** both are processed, **THEN** the subagent card shows the
  Cursor description, prompt, and model, and Kandev returns a success (not
  `-32601`) for `cursor/task`.
- **GIVEN** a `cursor/task` arrives before its `tool_call`, **WHEN** the
  `tool_call` later arrives, **THEN** the card is enriched with the earlier
  metadata.
- **GIVEN** the completion `tool_call_update.rawOutput.isBackground` is `true`,
  **WHEN** the card renders, **THEN** it shows the background affordance.
- **GIVEN** `isBackground` is `false` or absent, **WHEN** the card renders,
  **THEN** no background affordance shows.
- **GIVEN** a `cursor/task` with a missing `model` and a present `prompt`,
  **WHEN** the card renders, **THEN** the prompt shows and no empty model chip
  appears.
- **GIVEN** any non-Cursor agent's subagent, **WHEN** it is processed, **THEN**
  its rendering is byte-identical to before this feature.
- **GIVEN** a `cursor/task` that never matches a `tool_call`, **WHEN** the
  session ends, **THEN** the stored correlation is dropped and no card appears.

## Out of scope

- Fetching or resuming a **parked background subagent's result** by `agentId`.
  A wire capture proved Cursor never pushes the async result back over the ACP
  session (125s linger, zero post-`end_turn` frames); retrieval would need a new
  follow-up-prompt flow and is a separate feature.
- Result-text capture for Cursor. Unlike Claude, Cursor's completion frame
  carries only `durationMs`/`isBackground`, no result content; this remains as
  noted out of scope in
  [subagent-observability](../../ui/requirements/subagent-observability.md).
- Interpreting Cursor's `subagentType` object taxonomy.
- Any change to `active_subagent_count`, liveness, or the board chip
  ([background-work-liveness](../../platform/requirements/background-work-liveness.md)).

## Notes

- Wire evidence and the SDK dispatch analysis are recorded against task
  `hey-a-user-reported` (`acpdbg --linger`, `composer-2.5`). The `cursor/task`
  params and the `tool_call`/`tool_call_update` shapes above are from
  `acp-debug/cursor-acp-linger-20260820-145213.jsonl`.
- Decision: [ADR-2026-08-20-acp-client-non-underscore-extension-methods](../../../decisions/2026-08-20-acp-client-non-underscore-extension-methods.md)
  resolves how `cursor/task` is accepted: the `kdlbs/acp-go-sdk` fork routes any
  unrecognized inbound client method to the client's `ExtensionMethodHandler`,
  and Kandev's `Client` returns success only for methods it recognizes
  (`cursor/task`) and `NewMethodNotFound` otherwise.
