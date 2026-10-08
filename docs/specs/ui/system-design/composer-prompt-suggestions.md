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
mount it. Suggestions come from two sources:

- **Native (Claude).** Claude Code generates the suggestion inside the session's
  own agent process. It forks the finished conversation with the prompt cache
  warm and denies all tools. The suggestion reaches the composer through
  agentctl, the orchestrator, session metadata, and a WebSocket event.
- **Utility fallback.** For sessions without native suggestions, the browser
  that shows the session runs the `suggest-next-prompt` built-in utility agent.
  The result is cached in that browser.

Adjacent contracts used, with the change each needs:

- **ACP transport (Agents).** The Claude ACP adapter (`claude-agent-acp` 0.81.2,
  the pinned version) accepts `_meta.claudeCode.options.promptSuggestions` and
  `_meta.claudeCode.emitRawSDKMessages`. It forwards a matching SDK message as
  the `_claude/sdkMessage` extension notification. Kandev's ACP client must
  receive extension notifications. The `kdlbs/acp-go-sdk` fork currently routes
  them to `MethodNotFound`, and they are dropped.
- **Utility-agent execution (Agents).** `POST /api/v1/utility/execute`, see
  [utility agent profiles](../../agents/system-design/utility-agent-profiles.md).
- **Session and turn lifecycle (Tasks).** This design reads session state,
  turns, messages, the queue, and pending clarification. It writes one session
  metadata key with a state-guarded write.
- **User settings (Platform).** One boolean is added.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-PROMPT-SUGGEST-001` | [Preference](#preference), [Native launch request](#native-launch-request), [Components and responsibilities](#frontend) |
| `REQ-UI-PROMPT-SUGGEST-002` | [Native launch request](#native-launch-request), [Native delivery](#native-delivery), [Persistence](#persistence) |
| `REQ-UI-PROMPT-SUGGEST-003` | [Utility fallback](#utility-fallback), [Fallback prompt and filters](#fallback-prompt-and-filters), [Failure and recovery](#failure-and-recovery) |
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
- **Negotiated capability.** On `initialize`, the adapter records whether the
  agent advertised the `_meta.claudeCode` namespace
  (`agentAdvertisesClaudeCode`). That identifies a Claude Code bridge whether it
  is the built-in `claude-acp` agent, a custom ACP agent that wraps
  `claude-agent-acp`, or the mock. This follows the agentctl "negotiated, not
  named" rule already used for prompt handoff and steering. Native suggestions
  are a Claude Code harness feature, not a model feature: OpenCode or Codex
  running an Anthropic model never qualify. When both the advertisement and the
  instance flag are true, the adapter builds this `_meta`:

  ```json
  {
    "claudeCode": {
      "options": { "promptSuggestions": true },
      "emitRawSDKMessages": [{ "type": "prompt_suggestion" }]
    }
  }
  ```

  The adapter sends that map as `_meta` on `session/new` and `session/load` or
  `session/resume`. Agents that did not advertise Claude Code receive no
  `_meta`, so their behavior is unchanged. The `agent_capabilities` stream event
  gains `supports_prompt_suggestions`, true only when the `_meta` is sent.
- **Client.** `acp.Client` implements `HandleExtensionNotification` and
  forwards every extension notification to a registered handler.
- **Adapter.** The adapter dispatches `_claude/sdkMessage` and accepts only
  `message.type == "prompt_suggestion"` for the active ACP session. It emits a
  new stream event, `streams.EventTypePromptSuggestion = "prompt_suggestion"`,
  with `Text`. When a `session/prompt` is in flight, the suggestion belongs to a
  superseded turn and is dropped (`currentPromptTurn() != nil`).

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
  type does not matter here; agentctl negotiates the rest.
  The decision lives in `Manager.promptSuggestionsForLaunch`. Interactive
  means MCP mode `task`, `task-title-pending`, or empty. The preference is read
  through `SetPromptSuggestionsPreference`, wired to
  `user.Service.PromptSuggestionsEnabled`. Both the fresh-launch and the
  recreate paths evaluate it.
- **Orchestrator.** `event_handlers_streaming.go` dispatches
  `prompt_suggestion` to `handlePromptSuggestionEvent`, which:
  1. Trims the text and rejects an empty value or one of 1,000 characters or
     more. This is a sanity bound; Claude Code already filters native output.
  2. Resolves the session's latest turn ID.
  3. Writes `prompt_suggestion = {turn_id, text}` with
     `SetSessionMetadataKeyIfState(..., WAITING_FOR_INPUT)`, so a suggestion
     that races a newer prompt is lost.
  4. Publishes `session.prompt_suggestion` (`events.SessionPromptSuggestion`)
     with `{task_id, session_id, turn_id, text}` through the existing session
     notification bridge.
- **Utility built-in.** Adds `apps/backend/config/utilityagents/suggest-next-prompt.md`
  and a `builtinDefs` entry with ID `builtin-suggest-next-prompt` and name
  `suggest-next-prompt`. The existing startup seed inserts it.
- **User settings.** `prompt_suggestions` and `prompt_suggestions_fallback` are
  booleans, default `false`. They follow the same path as
  `agent_generated_task_titles`.
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
  - `resolveSuggestionTurnKey`
  - `isNativeSuggestionSession`, which reads
    `metadata.prompt_suggestion_source === "native"`
  - `shouldRequestFallback`
  - `buildSuggestionTranscript`
  - `normalizeFallbackSuggestion`, the filters below
  - bounded fallback cache storage
- **`hooks/use-prompt-suggestion.ts`.** Selects the visible suggestion and runs
  the fallback when eligible. Returns `{ suggestion, accept, send, dismiss }`.
- **Editor and composer.** The editor and composer:
  - extend `DynamicPlaceholder` storage with `suggestion` and the
    `has-prompt-suggestion` class;
  - add a Tab, submit-shortcut, and Escape keymap after the suggestion-menu
    handlers;
  - add `ComposerPromptSuggestionHint` beside `ChatInputFocusHint`;
  - add `PromptSuggestionSettings` in Settings > Task behavior > Conversation. It
    holds the main switch; the fallback switch, rendered only when the main
    switch is on; and, when the fallback switch is on, an agent profile selector
    bound to the `builtin-suggest-next-prompt` utility agent. The selector
    reuses `UtilityAgentProfilePicker` and its eligibility rules, and saves
    through the existing utility agent update API.

## Data and contracts

### Preference

| Layer | Name | Type | Default |
| --- | --- | --- | --- |
| HTTP and boot payload | `prompt_suggestions` | boolean | `false` |
| Store | `userSettings.promptSuggestions` | boolean | `false` |
| HTTP and boot payload | `prompt_suggestions_fallback` | boolean | `false` |
| Store | `userSettings.promptSuggestionsFallback` | boolean | `false` |

The effective fallback is `prompt_suggestions && prompt_suggestions_fallback`.

The fallback profile is not a new setting. It is the existing
`profile_binding_state` and `agent_profile_id` of the
`builtin-suggest-next-prompt` utility agent, read and written through the
existing utility agent API. Settings > Task behavior and Settings > Utility Agents
edit the same row. `inherit` means the user's default utility agent profile.

### Native launch request

The agentctl instance creation field is `prompt_suggestions`. The ACP `_meta`
is shown above. Session metadata key:

| Key | Value | Writer |
| --- | --- | --- |
| `prompt_suggestion` | `{ "turn_id": string, "text": string }` | orchestrator |
| `prompt_suggestion_source` | `"native"` or `"none"` | orchestrator, from `agent_capabilities` |

A session counts as native only when it negotiated native suggestions. A Claude
Code session started while the preference was off is not native. It uses the
fallback, if enabled, until it restarts or resumes.

### Native delivery

WebSocket event `session.prompt_suggestion`:

```json
{ "task_id": "...", "session_id": "...", "turn_id": "...", "text": "sim, corre a migration e os testes" }
```

### Utility fallback

The request is sessionless, so it runs on the host utility manager and never
wakes the session executor:

```json
{
  "utility_agent_id": "builtin-suggest-next-prompt",
  "session_id": "",
  "task_title": "...",
  "conversation_history": "<latest exchange>",
  "fallback_agent_profile_id": "<the session's agent_profile_id>"
}
```

Profile resolution follows `AC-UI-PROMPT-SUGGEST-003.9`. The utility service
uses `fallback_agent_profile_id` only when the built-in inherits and no default
utility profile exists. It validates that profile with the same
`profileResolver` that checks an explicit binding (enabled, global,
inference-capable, not passthrough), so an ineligible session profile fails
closed. Only the suggestion hook sends the field, so every other utility action
keeps its existing fail-closed resolution.

`shouldRequestFallback` is true only when every condition holds:

- the main and fallback preferences are on;
- the session is not native;
- the state is `WAITING_FOR_INPUT`, without `needsRecovery`;
- no turn is active;
- no clarification or permission is pending;
- the queue is empty;
- the newest conversation entry is agent-authored;
- the turn key has no cache entry and no in-flight request.

The turn key is the latest completed turn ID, or the newest agent message ID
when no turn record exists.

`buildSuggestionTranscript` keeps the newest user entry and the agent entries
after it, labelled `User:` and `Agent:` (marked `i18n-exempt`). The user part is
capped at 1,500 characters and the whole transcript at 6,000. The agent tail is
kept.

### Fallback prompt and filters

The template is an original Kandev prompt that encodes the rules in
`AC-UI-PROMPT-SUGGEST-003.5`. It receives `{{TaskTitle}}` and
`{{ConversationHistory}}`. It asks for the suggestion alone, without quotes or
explanation, and asks for an empty reply when the model should stay silent.

`normalizeFallbackSuggestion`:

1. Trims the response.
2. Unwraps one `<suggestion|response|output|answer|result>` tag pair.
3. Strips a leading `suggestion:`, `reply:`, `response:`, or `answer:` label.
4. Trims again.
5. Returns `null` when any rule matches:

| Rule | Condition |
| --- | --- |
| empty | Nothing remains |
| meta | `none`, `done`, `no suggestion…`, `nothing to suggest…`, `silence`, or text fully wrapped in brackets or parentheses |
| length | More than 12 words, or 100 or more characters |
| shape | Several sentences (`[.!?]` followed by whitespace and a capital), a newline, or Markdown `*`/`**` |
| evaluative | `thanks`, `thank you`, `looks good`, `sounds good`, `perfect`, `great`, `nice`, `makes sense` |
| agent voice | Starts with `let me`, `i'll`, `i've`, `i'm`, `here's`, `sure,`, `of course`, or `certainly` |

The rule strings are protocol patterns, not user copy, and are marked
`i18n-exempt`. They follow the Claude Code client filters, limited to English
and language-neutral rules.

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
4. agentctl drops the suggestion if a newer prompt was sent. Otherwise it emits
   the `prompt_suggestion` stream event.
5. The orchestrator writes the metadata (state-guarded) and publishes
   `session.prompt_suggestion`.
6. The frontend stores `{ turnId, text, source: "native" }`. The composer shows
   it when the turn key matches and the draft is empty.

### Utility fallback

1. The composer observes `WAITING_FOR_INPUT` for a non-native session. Because
   `shouldRequestFallback` is true, the hook starts the request in the same
   effect pass, with a 15-second `AbortSignal` (`options.init.signal`).
2. The hook drops the response if the turn key changed. It normalizes the
   response, then writes the cache and the store entry. A server failure
   (`ApiError` or `success: false`) or the timeout caches `null`. A transport
   rejection (page unload, network loss) caches nothing and releases the
   in-flight key, so the next mount may request again.

### Composer selection

The visible suggestion is the store entry for the session, shown only when:

- its turn key equals the current turn key;
- it is not dismissed;
- the session is `WAITING_FOR_INPUT` with no active turn, recovery,
  clarification, or queue;
- the preference is on.

Dismissals are kept in the browser cache by session and turn key.

## Rendering and keys

| State | Placeholder slot | Hint chip | Tab | Submit shortcut | Escape |
| --- | --- | --- | --- | --- | --- |
| Suggestion, empty draft, no menu | Ghost text | Visible | Accept | Send suggestion | Dismiss |
| Suggestion, non-empty draft | Draft | Hidden | Existing | Existing | Existing |
| No suggestion, or menu open | Existing | Hidden | Existing | Existing | Existing |

- **Accept** calls the editor handle's `setValue(text)` with the caret at the
  end. The draft persists through the normal `onUpdate` path.
- **Send** calls the same submit path as `handleSubmitWithReset` with the
  suggestion as the message. Queueing, attachments, and plan mode behave as if
  the user had typed the text.
- **Escape** is claimed through the existing suggestion-escape claim path, so
  the Quick Chat dialog stays open.
- **Styling.** Ghost text uses the muted foreground and wraps on phones.
- **Hint chip.** It shows a `Tab` keycap on fine pointers. On coarse pointers it
  is a button at least 44 px tall, labelled "Use reply". Its `aria-label`
  contains the suggestion.

## Failure and recovery

- **Adapter without support.** Native suggestions never arrive, and the session
  simply shows none, because native mode suppresses the fallback. This applies
  to a missing `_meta` handling or an older pinned version. The adapter version
  is pinned (0.81.2 verified), so this is a regression signal.
- **Notification ordering.** A suggestion arriving after a newer prompt is
  dropped by agentctl (step 4). A suggestion that loses the race to a state
  change is dropped by the state-guarded write (step 5).
- **Backend restart.** The metadata survives. The frontend still shows the
  suggestion only while its turn is the latest.
- **Fallback failure, timeout, or no utility agent.** No toast, and `null` is
  cached for the turn. A transport rejection is not cached.
- **Storage errors.** These are treated as an empty cache.

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
- **Fallback.** About 3 to 8 seconds, measured with ACP probe logs and one-shot
  CLI calls. It is capped at 15 seconds.

## Persistence

- **Native suggestion.** Persisted in session metadata and overwritten each
  turn. No schema change.
- **Fallback results and dismissals.** Stored in `localStorage` under
  `kandev.promptSuggestion.v1`, newest first, one entry per session, at most 50
  entries.
- **Preference.** Stored in the user settings JSON document. No migration.

## Security

- **Native.** The suggestion is produced inside the user's own agent session,
  with all tools denied by Claude Code.
- **Fallback.** The transcript goes to the user's configured utility profile.
- **Rendering.** Suggestions render as plain text through CSS `attr()` and
  `aria-label`, never as HTML.
- **Sending.** Sending requires an explicit key or tap. The text passes through
  the normal prompt submit path and its validation.

## Observability

- **agentctl.** Debug log for a dropped stale suggestion.
- **Orchestrator.** Debug log for a rejected or state-lost write.
- **Fallback calls.** Recorded in `utility_agent_calls`.
- **Metrics.** None in this version.

## Related decisions

No ADR. The change reuses ACP `_meta` and extension notifications, existing
session metadata, and utility-agent execution. The SDK fork change makes its
documented extension contract complete for notifications.
