---
id: "05-review-fixes"
title: "Bind suggestions to their originating turn and accept them as literal text"
status: done
wave: 5
depends_on:
  - "04-native-only-scope"
plan: "plan.md"
requirements:
  - REQ-UI-PROMPT-SUGGEST-002
  - REQ-UI-PROMPT-SUGGEST-004
acceptance_criteria:
  - AC-UI-PROMPT-SUGGEST-002.4
  - AC-UI-PROMPT-SUGGEST-002.6
  - AC-UI-PROMPT-SUGGEST-004.2
  - AC-UI-PROMPT-SUGGEST-004.3
  - AC-UI-PROMPT-SUGGEST-004.10
system_design:
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
---

# Task 05: Bind Suggestions to Their Originating Turn and Accept Them as Literal Text

## Summary

The maintainer review of kdlbs/kandev#4306 (2026-10-26) found two correctness
gaps:

1. **Delayed suggestions can be attributed to a newer turn.** The adapter only
   rejected a suggestion while a prompt was in flight, and the orchestrator
   stored it under whichever turn was latest. If turn A finished, turn B ran and
   finished, and A's suggestion arrived afterwards, it was shown as B's.
2. **Accepting or sending parses the suggestion as HTML.** TipTap's
   `setContent` received the raw string, so `Fix the <button> label` became
   `Fix the label`.

## In scope

- **agentctl.** A suggestion origin window in the ACP adapter, as described in
  the system design's "Suggestion origin" section. The emitted event carries the
  originating `PromptGeneration`.
- **Lifecycle.** Resolve a suggestion's `PromptGeneration` to its Kandev
  `TurnID` in `handleAgentEventState`; the durable stream already resolves it
  through `promptTurnIDForEvent`.
- **Orchestrator.** Store and publish the suggestion under its originating turn
  only when that turn is the session's latest; never fall back to the latest
  turn.
- **Composer.** `acceptPromptSuggestion` and `fillPromptSuggestionForSubmit`
  insert the suggestion through `textToHtml`.
- **Tests.** Deterministic adapter coverage for delayed A-after-B delivery and
  during-B rejection, lifecycle stamping, orchestrator persistence and
  publication with the originating turn, and editor tests with angle brackets,
  entities, and line breaks through both accept and submit.

## Out of scope

- Changing Claude Code or `claude-agent-acp`.
- Recovering the suggestion of a turn that overlapped the next prompt.

## Evidence gathered

A probe of `claude-agent-acp` 0.81.2 with the Kandev `_meta` showed:

- the `prompt_suggestion` message carries `type`, `suggestion`, `uuid`, and
  `session_id`, with no reference to its turn;
- a prompt sent immediately after a turn result cancelled that turn's pending
  suggestion: none arrived during or within 15 seconds after the new turn.

The kdlbs ACP SDK delivers every notification read before a request's response
before returning that response, and Kandev handles the extension notification
synchronously inside that delivery.

## Verification

```bash
cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp/... \
  ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/... ./cmd/mock-agent/...
bash scripts/lint-go-changed <changed-go-files>
cd apps/web && pnpm vitest run components/task/chat
cd apps/web && pnpm e2e:run tests/chat/prompt-suggestions.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-prompt-suggestions.spec.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt_suggestion.go`,
  `adapter_prompt.go`, `adapter_prompt_cancel.go`, and their tests
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go` and a test
- `apps/backend/internal/orchestrator/event_handlers_prompt_suggestion.go` and
  its test
- `apps/web/components/task/chat/tiptap-prompt-suggestion.ts` and its test

## Dependencies

- Task 04.

## Parallelism

`sequential`

## Inputs

- The maintainer review comment on kdlbs/kandev#4306 (2026-10-26).
- `docs/specs/ui/system-design/composer-prompt-suggestions.md`

## Results

Implemented.

- **agentctl.** `promptSuggestionOrigin` (guarded by `promptTurnMu`) opens a
  window when the owning prompt finishes in `finishPromptTurn`, and
  `markPromptSentForSuggestion` closes it when the next prompt is sent, marking
  that prompt as overlapping if the window was still open. A predecessor that
  finishes while a successor is in flight marks the successor as overlapping.
  `handleClaudeSDKMessage` claims the window and emits the event with the
  originating `PromptGeneration`, or drops it.
- **Lifecycle.** `handleAgentEventState` resolves a suggestion's generation to
  its `TurnID` and never falls back to the current turn.
- **Orchestrator.** `handlePromptSuggestionEvent` stores and publishes the
  suggestion under its originating turn only when that turn is the latest.
- **Composer.** Accept and submit insert the suggestion through `textToHtml`.
- **Hint.** An automated review (Greptile) found that the desktop hint always
  said "Enter sends", while the default submit key is Cmd/Ctrl+Enter. The hint
  now names the configured shortcut, and the settings help says "your send
  shortcut". `composer-prompt-suggestion-hint.test.tsx` failed (`Enter sends`
  without `Ctrl+Enter`) before the change.

Red-Green evidence:

- `prompt_suggestion_origin_test.go` did not compile before the origin window
  existed; it covers the delayed A-after-B case, during-B rejection, one
  suggestion per window, unknown origin, and handed-off prompts.
- `TestPromptSuggestionUsesItsPromptGenerationTurnID` failed (empty turn ID)
  before the lifecycle case.
- `TestHandlePromptSuggestionEvent_RejectsSuggestionWithoutLatestOrigin` failed
  (stored under `turn-2`) before the orchestrator change.
- The literal-text tests failed with `Fix the label and preserve <name>` and
  `run the tests then commit` before `textToHtml`.

Verification:

- `go test` for the ACP adapter, lifecycle, orchestrator, and mock agent
  packages passed. `bash scripts/lint-go-changed` reported 0 issues.
- `pnpm vitest run components/task/chat lib/prompt-suggestion.test.ts
  hooks/use-prompt-suggestion.test.ts`: 179 files and 1,992 tests passed.
  `pnpm run typecheck` and targeted eslint passed.
- `pnpm e2e:run tests/chat/prompt-suggestions.spec.ts
  tests/settings/prompt-suggestion-settings.spec.ts`: 7 passed.
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-prompt-suggestions.spec.ts`:
  1 passed.
