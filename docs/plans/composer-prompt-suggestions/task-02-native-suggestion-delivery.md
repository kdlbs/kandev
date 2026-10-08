---
id: "02-native-suggestion-delivery"
title: "Deliver native Claude prompt suggestions"
status: in_progress
wave: 2
depends_on:
  - "01-suggestion-agent-and-preference"
plan: "plan.md"
requirements:
  - REQ-UI-PROMPT-SUGGEST-001
  - REQ-UI-PROMPT-SUGGEST-002
acceptance_criteria:
  - AC-UI-PROMPT-SUGGEST-001.2
  - AC-UI-PROMPT-SUGGEST-002.1
  - AC-UI-PROMPT-SUGGEST-002.2
  - AC-UI-PROMPT-SUGGEST-002.3
  - AC-UI-PROMPT-SUGGEST-002.4
system_design:
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
---

# Task 02: Deliver Native Claude Prompt Suggestions

## Summary

Ask Claude ACP sessions for Claude Code's native prompt suggestions and receive
them over the `_claude/sdkMessage` extension notification. Persist each one on
the session and publish `session.prompt_suggestion`. This includes the
`kdlbs/acp-go-sdk` fork change, without which extension notifications never
reach Kandev.

## In scope

- **`kdlbs/acp-go-sdk`.**
  - Add `ExtensionNotificationHandler` and route inbound `_`-prefixed
    notifications to it on the client side. Unimplemented clients keep ignoring
    them.
  - Add SDK unit tests.
  - Ship it as a fork revision and bump the `replace` in
    `apps/backend/go.mod`.
- **agentctl instance config.** Add `prompt_suggestions` to the instance
  creation request and carry it to `shared.Config`.
- **agentctl dialect.** Add a `promptSuggestions` capability to `acpDialect`,
  set by the Claude and mock dialects. The adapter attaches the `_meta` from the
  system design to `session/new` and to `session/load` or `session/resume`.
- **agentctl client and adapter.**
  - `Client.HandleExtensionNotification` handles `_claude/sdkMessage` and
    `prompt_suggestion` only.
  - The adapter emits `streams.EventTypePromptSuggestion`. It drops the
    suggestion when a newer `session/prompt` was sent after the last prompt
    completed.
- **Negotiated capability.** Detect Claude Code from the `initialize`
  `_meta.claudeCode` advertisement, report `supports_prompt_suggestions` on
  `agent_capabilities`, and persist it as `prompt_suggestion_source`.
- **Launch and resume.** Set the agentctl flag only when all of these hold:
  - the preference is on;
  - the agent supports native suggestions;
  - the session is an interactive task or Quick Chat session.

- **Orchestrator.** `handlePromptSuggestionEvent`:
  - applies the sanity bound;
  - resolves the latest turn ID;
  - writes `prompt_suggestion` through `SetSessionMetadataKeyIfState`, rejecting
    a session in `RUNNING` or `STARTING`;
  - publishes `events.SessionPromptSuggestion` through the session notification
    bridge.
- **Mock agent.** When the `_meta` requests it, send a deterministic
  `_claude/sdkMessage` `prompt_suggestion` after each turn result. Scenario
  directives can suppress or delay it.
- **Manual probe.** Run `/acp-debug` against the real
  `claude-agent-acp@0.81.2` with the `_meta` over two turns. Record the
  `_claude/sdkMessage` frame and its delay after the `session/prompt` response.

## Out of scope

- Frontend rendering, keys, and the fallback (Task 03).
- `set_prompt_suggestions_paused`, back-off, and non-Claude native sources.

## Acceptance

- With the preference on, a mock-agent task session sends the Claude `_meta` on
  `session/new` and on resume. After a turn, the session's metadata holds
  `prompt_suggestion {turn_id, text}`, and a `session.prompt_suggestion` event
  is published. With the preference off, the `_meta` is absent and no event is
  published.
- No native request is made for Office runs, automation sessions, utility
  calls, passthrough sessions, or non-native agents. A suggestion that arrives
  after a newer prompt, or while the session is running, is neither persisted
  nor published.
- The `/acp-debug` probe shows a real `_claude/sdkMessage` `prompt_suggestion`
  frame after the second turn. The frame and timing are recorded in Results.
  If it does not appear, stop and report before Task 03.

## Verification

```bash
# in the kdlbs/acp-go-sdk checkout
go test ./...
# in this repo
cd apps/backend && go test ./internal/agentctl/server/acp/... ./internal/agentctl/server/adapter/transport/acp/... \
  ./internal/agentctl/server/api/... ./internal/agent/agents/... ./internal/agent/runtime/... \
  ./internal/orchestrator/... ./cmd/mock-agent/...
make -C apps/backend lint
```

Use `/acp-debug` for the manual adapter probe.

## Files likely touched

- `kdlbs/acp-go-sdk`: `extensions.go`, `connection.go` or `client_gen` routing,
  and tests
- `apps/backend/go.mod`, `apps/backend/go.sum`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agentctl/server/acp/client.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`,
  `dialect_claude.go`, `dialect_mock.go`, `adapter_session.go`, and an update or
  notification file
- `apps/backend/internal/agentctl/types/streams/agent.go`
- `apps/backend/internal/agent/agents/claude_acp.go`, the mock agent definition,
  and the `agent.go` interface
- The lifecycle launch and resume request builders under
  `apps/backend/internal/agent/runtime/`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go` and a new
  handler file
- `apps/backend/internal/events/types.go` and the WebSocket notification bridge
- `apps/backend/cmd/mock-agent/` (`main.go`, emitter, scenarios)

## Dependencies

- Task 01 provides the `prompt_suggestions` user setting reader.
- The SDK fork change can start in parallel with Task 01.

## Risks

- **SDK bump scope.** Diff the fork range, and keep the bump to this change plus
  already-reviewed commits.
- **Interactive-session predicate.** Reuse the existing Office, automation, and
  passthrough discriminators rather than adding a new one. Test each excluded
  kind.
- **Turn attribution.** Prompt-generation tracking in the adapter is the primary
  stale guard. The state-guarded write is the second. Test both.
- **Go lint limits.** Keep the new handler and dialect hooks small. Extract
  helpers.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/composer-prompt-suggestions.md`
- `docs/specs/ui/system-design/composer-prompt-suggestions.md`
- `docs/plans/composer-prompt-suggestions/plan.md` (evidence section)
- `apps/backend/AGENTS.md`, `apps/backend/internal/agentctl/AGENTS.md`
- `internal/agentctl/server/acp/client.go` (the `cursor/task` extension pattern)
- `internal/agentctl/server/adapter/transport/acp/dialect_grok.go` (a dialect
  `_meta` example)
- `claude-agent-acp@0.81.2` `dist/acp-agent.js` (`emitRawSDKMessages`,
  `shouldEmitRawMessage`, `promptSuggestions` option allowlist)

## Results

Implemented in this repository. The SDK change (`2259551`) is published on
`JnManso/acp-go-sdk` branch `feat/client-extension-notifications` and proposed
upstream in kdlbs/acp-go-sdk#6. Until that merges, `go.mod` replaces the SDK with
`github.com/JnManso/acp-go-sdk v0.13.6-0.20261006200825-2259551ab03d`. After it
merges, the `replace` must move back to a `kdlbs/acp-go-sdk` pseudo version; that
bump is what remains before this work order is done.

Deviations from the original package, now reflected in the system design:

- The flag travels in the per-instance agentctl config, not as new
  `NewSession`/`LoadSession` parameters. This avoids changing every adapter
  interface.
- Native mode is negotiated per session from the `initialize` `_meta.claudeCode`
  advertisement, not from the agent type. It is recorded as session metadata
  `prompt_suggestion_source` and mirrored live by the `agent_capabilities`
  handler. This makes custom ACP agents that run Claude Code native, and
  OpenCode or Codex never native, whatever their model. An earlier
  per-agent-type capability (`NativePromptSuggester`) was removed.

Red-Green evidence. Each test failed (undefined symbol or empty result) before
implementation, then passed:

- SDK: `TestExtensionNotifications_*`.
- `agents`: `TestSupportsNativePromptSuggestions`.
- `settings/controller`: `TestToAgentDTOReportsNativePromptSuggestions`.
- `transport/acp`: `TestPromptSuggestionSessionMeta` and
  `TestHandleClaudeSDKMessage*`.
- `server/acp`: `TestHandleExtensionNotification*`.
- `lifecycle`: `TestNativePromptSuggestionsForLaunch`.
- `orchestrator`: `TestHandlePromptSuggestionEvent_*`.
- `user/service`: `TestPromptSuggestionsEnabled`.
- `mock-agent`: `TestMockPromptSuggestionFollowsSessionMeta`.

Verification:

- In the SDK fork, `go test ./...` passed.
- In this repository, this command passed (exit 0):

  ```bash
  go test ./internal/agentctl/... ./internal/agent/... ./internal/orchestrator/... \
    ./internal/gateway/... ./internal/events/... ./internal/user/... ./cmd/mock-agent/
  ```

- Real adapter probe: `claude-agent-acp@0.81.2` with the Kandev `_meta`, four
  short turns, using a raw JSON-RPC script. `acpdbg` cannot send a custom
  `_meta` or several prompts.
  - Turn 1: `_claude/sdkMessage` `prompt_suggestion` "sim, corre", 1.66 s after
    the `session/prompt` response.
  - Turn 2: "sim, corre os testes", 1.89 s after the response.
  - Turns 3 and 4: none, from Claude Code's unused-streak back-off after the
    ignored suggestions.
- An earlier two-turn probe got none because turn 2 wrote 9,852 cache tokens
  plus 251 output tokens, which exceeds Claude Code's 10,000-token cache-cold
  limit.
