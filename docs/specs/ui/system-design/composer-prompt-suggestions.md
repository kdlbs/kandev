---
status: draft
system: ui
requirements:
  - REQ-UI-PROMPT-SUGGEST-001
  - REQ-UI-PROMPT-SUGGEST-002
  - REQ-UI-PROMPT-SUGGEST-003
  - REQ-UI-PROMPT-SUGGEST-004
---

# Composer Prompt Suggestion System Design

## Purpose and boundaries

The shared chat composer (`ChatInputArea`) owns how a prompt suggestion is
selected, rendered, accepted, sent, and dismissed. Task chat and Quick Chat both
mount it. Suggestions have one source, the session's own agent runtime:

- **Native (Claude).** Claude Code generates the suggestion inside the session's
  own agent process. It forks the finished conversation with the prompt cache
  warm and denies all tools. The suggestion reaches the composer through
  agentctl, the orchestrator, session metadata, and a WebSocket event.

There is no generated replacement. A session that did not negotiate native
suggestions, or a turn that produced none, leaves the composer unchanged.

Adjacent contracts used, with the change each needs:

- **Provider support (Agents).** The [native capability design](../../agents/system-design/native-prompt-suggestion-capability.md)
  owns evidence, configuration, and adapter normalization. The pinned Claude
  bridge accepts suggestion options and raw-message filters. The current
  `kdlbs/acp-go-sdk` replacement already routes extension notifications.
  The capability follow-up is implemented in the Agents work package.
- **Session and turn lifecycle (Tasks).** This design reads session state,
  turns, messages, the queue, and pending clarification. It writes one session
  metadata key with a state-guarded write.
- **User settings (Platform).** One boolean is added.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-PROMPT-SUGGEST-001` | [Preference](#preference), [Native launch request](#native-launch-request), [Components and responsibilities](#frontend) |
| `REQ-UI-PROMPT-SUGGEST-002` | [Native launch request](#native-launch-request), [Suggestion origin](#suggestion-origin), [Native delivery](#native-delivery), [Persistence](#persistence) |
| `REQ-UI-PROMPT-SUGGEST-003` | [Composer selection](#composer-selection), [Failure and recovery](#failure-and-recovery) |
| `REQ-UI-PROMPT-SUGGEST-004` | [Composer selection](#composer-selection), [Rendering and keys](#rendering-and-keys) |

## Components and responsibilities

### ACP SDK fork (`kdlbs/acp-go-sdk`)

`ClientSideConnection` routes inbound notifications whose method starts with
`_` to an optional `ExtensionNotificationHandler`:

```go
HandleExtensionNotification(ctx context.Context, method string, params json.RawMessage) error
```

A client that does not implement the handler keeps the current ignore
behavior. Kandev bumps its `replace` directive to the fork revision that
contains this change.

### agentctl

- **Instance config.** The agentctl instance creation request gains
  `prompt_suggestions bool`. It flows through `InstanceOverrides`,
  `InstanceConfig`, and the adapter config to `shared.Config.PromptSuggestions`,
  in the same way `disable_ask_question` does. A per-instance flag avoids
  changing the session API of every adapter.
- **Effective capability.** `supports_prompt_suggestions` means that this session
  requested native suggestions from a supported integration. Preserve that meaning.
  The planned support-only field, `native_prompt_suggestions_supported`, never
  enables a composer by itself. Agents owns its evidence rules and all producer/
  consumer changes. The landed namespace-only heuristic is the subject of that
  follow-up, not proof of explicit feature support.
- **Session configuration.** Agents owns the metadata builder for `session/new`,
  `session/load`, and `session/resume`. Its design preserves the pinned bridge,
  separates support from requested enablement, and defines unsupported fallback.
  A selected model never establishes support.
- **Client.** `acp.Client` implements `HandleExtensionNotification` and
  forwards every extension notification to a registered handler.
- **Adapter.** The adapter dispatches `_claude/sdkMessage` and accepts only
  `message.type == "prompt_suggestion"` for the active ACP session. It emits a
  new stream event, `streams.EventTypePromptSuggestion = "prompt_suggestion"`,
  with `Text` and `PromptGeneration`, the lifecycle generation of the
  originating prompt. See [Suggestion origin](#suggestion-origin).

### Backend

- **Capability record.** `handleAgentCapabilitiesEvent` persists the negotiated
  result as session metadata `prompt_suggestion_source`. It writes `native` when
  the event says so, and `none` only when replacing an earlier `native`, so
  ordinary sessions add no write. The broadcast payload carries
  `supports_prompt_suggestions`.
- **Launch.** The lifecycle launch and resume path sets
  `prompt_suggestions = true` on the agentctl request only when the user
  preference is on and the session is an interactive task or Quick Chat session,
  not an Office run, automation, utility call, or passthrough session. The agent
  type does not matter here; agentctl normalizes provider support.
  The decision lives in `Manager.promptSuggestionsForLaunch`. Interactive
  means MCP mode `task`, `task-title-pending`, or empty. The preference is read
  through `SetPromptSuggestionsPreference`, wired to
  `user.Service.PromptSuggestionsEnabled`. Both the fresh-launch and the
  recreate paths evaluate it.
- **Lifecycle.** `handleAgentEventState`, and the durable delivery stream,
  resolve the event's `PromptGeneration` to its Kandev `TurnID` through the
  execution's generation map. A suggestion without a generation gets no turn.
- **Orchestrator.** `event_handlers_streaming.go` dispatches
  `prompt_suggestion` to `handlePromptSuggestionEvent`, which:
  1. Trims the text and rejects an empty value or one of 1,000 characters or
     more. This is a sanity bound; Claude Code already filters native output.
  2. Rejects the suggestion when its originating `TurnID` is empty or is not
     the session's latest turn. It never substitutes the latest turn.
  3. Writes `prompt_suggestion = {turn_id, text}` with the originating turn ID
     through `SetSessionMetadataKeyIfState(..., WAITING_FOR_INPUT)`, so a
     suggestion that races a newer prompt is lost.
  4. Publishes `session.prompt_suggestion` (`events.SessionPromptSuggestion`)
     with `{task_id, session_id, turn_id, text}` through the existing session
     notification bridge.
- **User settings.** `prompt_suggestions` is a boolean, default `false`. It
  follows the same path as `agent_generated_task_titles`.
- **Mock agent.** When `session/new` or `session/load` carries
  `_meta.claudeCode.options.promptSuggestions = true`, the mock agent sends
  `_claude/sdkMessage` with a deterministic `prompt_suggestion` after each turn
  result. A scenario directive can suppress it or delay it.

### Frontend

- **Session metadata.** There is no separate store slice. The
  `session.prompt_suggestion` handler writes `metadata.prompt_suggestion` on the
  session, and the `session.agent_capabilities` handler mirrors
  `metadata.prompt_suggestion_source`. Hydration restores both after a reload.
- **`lib/prompt-suggestion.ts`.** Pure helpers:
  - `readNativePromptSuggestion`, which validates `metadata.prompt_suggestion`
  - `resolveSuggestionTurnKey`
  - `isNativeSuggestionSession`, which reads
    `metadata.prompt_suggestion_source === "native"`
  - `selectVisibleSuggestion`, the composer selection below
  - bounded dismissal storage
- **`hooks/use-prompt-suggestion.ts`.** Reads the store and returns the visible
  suggestion and a `dismiss` callback. It never issues a request.
- **Editor and composer.** The editor and composer:
  - extend `DynamicPlaceholder` storage with `suggestion` and the
    `has-prompt-suggestion` class;
  - add a Tab, submit-shortcut, and Escape keymap after the suggestion-menu
    handlers;
  - add `ComposerPromptSuggestionHint` beside `ChatInputFocusHint`;
  - add `PromptSuggestionSettings` in Settings > Task behavior > Conversation,
    with the single "Suggest next prompt" switch.

## Data and contracts

### Preference

| Layer | Name | Type | Default |
| --- | --- | --- | --- |
| HTTP and boot payload | `prompt_suggestions` | boolean | `false` |
| Store | `userSettings.promptSuggestions` | boolean | `false` |

### Native launch request

The agentctl instance creation field is `prompt_suggestions`. Agents owns the
[provider metadata and generic capability contract](../../agents/system-design/native-prompt-suggestion-capability.md#generic-contract).
Session metadata key:

| Key | Value | Writer |
| --- | --- | --- |
| `prompt_suggestion` | `{ "turn_id": string, "text": string }` | orchestrator |
| `prompt_suggestion_source` | `"native"` or `"none"` | orchestrator, from `agent_capabilities` |

A session counts as native only when it effectively enabled native suggestions.
Provider support can remain true while the preference is off. That session shows
no suggestion until it restarts or resumes with the preference on.

### Suggestion origin

Claude Code's `prompt_suggestion` message carries `type`, `suggestion`, its own
`uuid`, and `session_id`, but no reference to the turn it predicts. Origin is
therefore established by the adapter, from the ACP prompt boundaries:

- When a prompt finishes, the adapter opens a suggestion window for that
  prompt's generation.
- A window closes when a suggestion arrives or when the next prompt starts.
- A prompt that starts while the previous window is still open overlaps it:
  a delayed suggestion from the earlier prompt could arrive after this one
  finishes. Its own window therefore has no trustworthy origin.
- A suggestion that arrives while a prompt is in flight is dropped.
- A suggestion that arrives while idle takes the open window's generation. It
  is dropped when no window is open, when the window has no trustworthy origin,
  or when the generation is unknown (`0`).

Two provider facts keep this precise rather than lossy. The SDK delivers every
notification read before a `session/prompt` response before that response, and
the handler runs inside that delivery, so a suggestion emitted before the
response is always seen while the prompt is still in flight. Claude Code
(`claude-agent-acp` 0.81.2) also cancels a pending suggestion when the next
prompt arrives: in a probe, a prompt sent immediately after a turn result
received no suggestion for the earlier turn, during or after the new turn. The
overlap rule keeps a delayed suggestion out even if a future adapter stops
cancelling. Its cost is that a turn that follows a silent turn, or a reply sent
within about two seconds, shows no suggestion.

### Native delivery

WebSocket event `session.prompt_suggestion`:

```json
{ "task_id": "...", "session_id": "...", "turn_id": "...", "text": "sim, corre a migration e os testes" }
```

## Control flow

### Native

1. The session launches or resumes. The backend evaluates the native launch
   conditions and passes `prompt_suggestions` to agentctl. agentctl sends the
   Claude `_meta`.
2. The turn ends. The ACP `session/prompt` response completes the Kandev turn,
   and the session moves to `WAITING_FOR_INPUT`.
3. Claude Code forks the conversation and emits `prompt_suggestion`, measured
   at about 1.7 seconds after the result. The adapter forwards
   `_claude/sdkMessage`.
4. agentctl drops the suggestion if a newer prompt is in flight or its origin is
   not trustworthy. Otherwise it emits the `prompt_suggestion` stream event with
   the originating prompt generation, which the lifecycle resolves to a turn ID.
5. The orchestrator checks that the originating turn is the latest, writes the
   metadata (state-guarded), and publishes `session.prompt_suggestion`.
6. The frontend stores `{ turnId, text, source: "native" }`. The composer shows
   it when the turn key matches and the draft is empty.

### Composer selection

`selectVisibleSuggestion` returns the native suggestion text only when every
condition holds, and `null` otherwise:

- the preference is on;
- the session is native (`prompt_suggestion_source === "native"`);
- `metadata.prompt_suggestion` holds a non-empty text and turn ID;
- the session is `WAITING_FOR_INPUT` with no active turn, recovery,
  clarification, queued prompt, or task move;
- the suggestion's turn ID equals the current turn key. When the browser has
  not loaded turn records yet, the backend's state-guarded binding is
  authoritative;
- the suggestion is not dismissed for its turn.

The turn key is the latest completed turn ID, or the newest agent message ID
when no turn record exists. A newer turn changes the key, so an older
suggestion is stale and stays hidden even though the metadata still holds it.

Dismissals are kept in the browser by session and turn ID.

## Rendering and keys

| State | Placeholder slot | Hint chip | Tab | Submit shortcut | Escape |
| --- | --- | --- | --- | --- | --- |
| Suggestion, empty draft, no menu | Ghost text | Visible | Accept | Send suggestion | Dismiss |
| Suggestion, non-empty draft | Draft | Hidden | Existing | Existing | Existing |
| No suggestion, or menu open | Existing | Hidden | Existing | Existing | Existing |

- **Accept** replaces the draft with the suggestion as literal text
  (`textToHtml` escapes markup and keeps line breaks as paragraphs), with the
  caret at the end. The draft persists through the normal `onUpdate` path.
- **Send** calls the same submit path as `handleSubmitWithReset` with the
  suggestion as the message. Queueing, attachments, and plan mode behave as if
  the user had typed the text.
- **Escape** is claimed through the existing suggestion-escape claim path, so
  the Quick Chat dialog stays open.
- **Styling.** Ghost text uses the muted foreground and wraps on phones.
- **Hint chip.** It shows a `Tab` keycap and the user's configured submit
  shortcut (Enter, or Cmd/Ctrl+Enter) on fine pointers. On coarse pointers it
  is a button at least 44 px tall, labelled "Use reply". Its `aria-label`
  contains the suggestion.

## Failure and recovery

- **Agent without native support.** agentctl sends no `_meta`, the session
  records no native source, and the composer keeps its normal behavior.
- **Supported adapter that sends nothing.** The session can be effectively
  enabled without receiving a suggestion. The composer shows none. Native silence
  is valid; it does not independently prove support or lack of support.
- **Suggestion payload on a non-native session.** The composer ignores it.
- **Notification ordering.** A suggestion arriving during a newer prompt is
  dropped by agentctl, and one whose origin overlaps a newer prompt is dropped
  by the origin window (step 4). A suggestion whose originating turn is no
  longer the latest is rejected by the orchestrator, and one that loses the
  race to a state change is dropped by the state-guarded write (step 5).
- **Backend restart.** The metadata survives. The frontend still shows the
  suggestion only while its turn is the latest.
- **Storage errors.** These are treated as no dismissal.

## Latency

Native suggestion frequency is controlled by Claude Code. It suppresses a
suggestion when a turn's new prompt-cache writes plus output exceed 10,000
tokens ("cache cold"), early in a conversation, in plan mode, and after
ignored suggestions (unused-streak back-off). Long tool-heavy turns therefore
often show no native suggestion, by design.

- **Native.** About 1.7 seconds after the result, measured with Claude CLI
  2.1.285 in SDK stream-json mode with `--prompt-suggestions`. The end-to-end
  probe through `claude-agent-acp@0.81.2` with the Kandev `_meta` measured 1.66
  and 1.89 seconds. It uses no extra
  agent process, and its token cost is a cache read plus a short suffix.

## Persistence

- **Native suggestion.** Persisted in session metadata and overwritten each
  turn. No schema change.
- **Dismissals.** Stored in `localStorage` under
  `kandev.promptSuggestionDismissals.v1` as `{ sessionId, turnKey }`, newest
  first, one entry per session, at most 50 entries.
- **Preference.** Stored in the user settings JSON document. No migration.

## Security

- **Native.** The suggestion is produced inside the user's own agent session,
  with all tools denied by Claude Code.
- **Rendering.** Suggestions render as plain text through CSS `attr()` and
  `aria-label`, never as HTML.
- **Sending.** Sending requires an explicit key or tap. The text passes through
  the normal prompt submit path and its validation.

## Observability

- **agentctl.** Debug log for a dropped stale suggestion.
- **Orchestrator.** Debug log for a rejected or state-lost write.
- **Metrics.** None in this version.

## Related decisions and delivery

- [Native capability boundary](../../../decisions/2026-10-28-native-prompt-suggestion-capability.md)
- [Native capability follow-up](../../../plans/native-prompt-suggestion-capability/plan.md)
- [Original composer delivery](../../../plans/composer-prompt-suggestions/plan.md)

The original SDK fork change completed extension-notification delivery.
The follow-up changes provider evidence while preserving this interaction contract.
