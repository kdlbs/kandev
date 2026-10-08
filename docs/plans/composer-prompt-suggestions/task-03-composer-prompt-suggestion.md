---
id: "03-composer-prompt-suggestion"
title: "Show, accept, and send prompt suggestions in the composer"
status: done
wave: 3
depends_on:
  - "01-suggestion-agent-and-preference"
  - "02-native-suggestion-delivery"
plan: "plan.md"
requirements:
  - REQ-UI-PROMPT-SUGGEST-001
  - REQ-UI-PROMPT-SUGGEST-002
  - REQ-UI-PROMPT-SUGGEST-003
  - REQ-UI-PROMPT-SUGGEST-004
acceptance_criteria:
  - AC-UI-PROMPT-SUGGEST-001.2
  - AC-UI-PROMPT-SUGGEST-001.4
  - AC-UI-PROMPT-SUGGEST-002.2
  - AC-UI-PROMPT-SUGGEST-002.5
  - AC-UI-PROMPT-SUGGEST-003.1
  - AC-UI-PROMPT-SUGGEST-003.2
  - AC-UI-PROMPT-SUGGEST-003.3
  - AC-UI-PROMPT-SUGGEST-003.4
  - AC-UI-PROMPT-SUGGEST-003.6
  - AC-UI-PROMPT-SUGGEST-003.7
  - AC-UI-PROMPT-SUGGEST-004.1
  - AC-UI-PROMPT-SUGGEST-004.2
  - AC-UI-PROMPT-SUGGEST-004.3
  - AC-UI-PROMPT-SUGGEST-004.4
  - AC-UI-PROMPT-SUGGEST-004.5
  - AC-UI-PROMPT-SUGGEST-004.6
  - AC-UI-PROMPT-SUGGEST-004.7
  - AC-UI-PROMPT-SUGGEST-004.8
  - AC-UI-PROMPT-SUGGEST-004.9
system_design:
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
---

# Task 03: Show, Accept, and Send Prompt Suggestions in the Composer

## Summary

Consume native suggestions from `session.prompt_suggestion` and session
metadata. Run the utility fallback for non-native sessions. Render the selected
suggestion as ghost text in the shared composer:

- **Tab** fills the draft.
- **The submit shortcut** sends the suggestion.
- **Escape** dismisses it.
- **The chip** fills the draft on touch.

Prove the behavior in task chat, Quick Chat, and on a phone.

## In scope

- **Store.** A `promptSuggestions` slice. The `session.prompt_suggestion`
  WebSocket handler writes it, and session hydration seeds it from
  `metadata.prompt_suggestion`.
- **`lib/prompt-suggestion.ts`.** Helpers:
  - `resolveSuggestionTurnKey`
  - `isNativeSuggestionSession`
  - `shouldRequestFallback`
  - `buildSuggestionTranscript`: the latest exchange, with the user part capped
    at 1,500 characters and the whole at 6,000, keeping the agent tail
  - `normalizeFallbackSuggestion`: tag and label stripping plus the filter
    table from the system design
  - bounded `kandev.promptSuggestion.v1` storage for fallback results and
    dismissals
- **`hooks/use-prompt-suggestion.ts`.**
  - Selects the visible suggestion.
  - Starts the fallback immediately for eligible non-native sessions, only when
    `promptSuggestionsFallback` is also on and the built-in resolves to a
    profile, with a
    15-second `AbortSignal` held in a named constant that E2E can shorten.
  - Returns `{ suggestion, accept, send, dismiss }`.
- **Editor.**
  - `DynamicPlaceholder` gains `suggestion` storage and the
    `has-prompt-suggestion` class.
  - A keymap after the suggestion-menu handlers handles Tab (accept), the
    configured submit shortcut (send), and Escape (dismiss), only while ghost
    text is visible and no menu is open.
  - Escape uses the existing suggestion-escape claim path.
- **`ComposerPromptSuggestionHint`.**
  - Shows a `Tab` keycap with "Accept · Enter sends" on fine pointers.
  - On coarse pointers it is a "Use reply" button at least 44 px tall.
  - Its `aria-label` contains the suggestion.
  - It hides the focus hint while visible.
- **Wiring.** `ChatInputArea` → `ChatInputContainer` → `ChatInputBody` →
  `TipTapInput`. Send uses the normal submit path.
- **CSS.** Muted, wrapping ghost text.
- **Copy.** In every catalog.
- **Docs.** Finish the "Prompt suggestions" section in
  `docs/public/developer-tools.md`: keys, tap, and Escape.
- **Tests.** Unit tests, desktop and Quick Chat E2E, and phone E2E.

## Out of scope

- Backend, agentctl, and SDK changes (Task 02).
- Passthrough, run-transcript, and task-creation composers. Multiple
  suggestions and completion while typing.

## Acceptance

- **Native E2E (mock agent, preference on).**
  1. After a turn, the ghost text and chip appear with no utility request.
  2. Tab fills the draft without sending.
  3. With the draft cleared, Enter sends the suggestion as the user message.
  4. After the next turn, Escape dismisses the suggestion.
  5. A reload restores the latest suggestion.
- **Fallback E2E (non-native session, main and fallback switches on, stubbed
  execute).**
  1. One request is made with the latest-exchange payload.
  2. The ghost text appears, and a reload makes no second request.
  3. A response delayed past the budget shows nothing.
  4. Filtered responses show nothing.
- **When ghost text must not render.** It never renders when:
  - the preference is off;
  - the session is non-native and the fallback switch is off;
  - the session is busy, starting, needs recovery, has a clarification pending,
    or has a queued prompt;
  - the draft is non-empty.

  In those states, Tab, Enter, and Escape behave exactly as before.
- **Phone and Quick Chat.** On `mobile-chrome`, the "Use reply" chip is at least
  44 px and fills the draft. In Quick Chat, Escape dismisses the suggestion and
  the dialog stays open.

## ASCII UI preview

See [plan UI-01 to UI-03](plan.md#ascii-ui-preview).

```text
desktop                                          phone
+----------------------------------------------+ +---------------------------+
| sim, corre a migration e os testes   (ghost) | | sim, corre a migration    |
|                [Tab] Accept · Enter sends    | | e os testes      (ghost)  |
| [Agent v] [Model v]           [Send >]       | |            [ Use reply ]  |
+----------------------------------------------+ | [+] [Agent v]   [Send >]  |
                                                 +---------------------------+
```

## Verification

```bash
cd apps/web && pnpm test -- lib/prompt-suggestion.test.ts hooks/use-prompt-suggestion.test.ts \
  components/task/chat/tiptap-dynamic-placeholder.test.ts lib/ws/handlers/prompt-suggestions.test.ts
cd apps/web && pnpm e2e:run tests/chat/prompt-suggestions.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-prompt-suggestions.spec.ts
cd apps/web && pnpm e2e:run tests/chat/quick-chat.spec.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/prompt-suggestion.ts` and `.test.ts`
- `apps/web/hooks/use-prompt-suggestion.ts` and `.test.ts`
- `apps/web/lib/state/slices/session/` (prompt suggestion slice and types)
- `apps/web/lib/ws/handlers/prompt-suggestions.ts` and `.test.ts`, plus handler
  registration
- `apps/web/components/task/chat/tiptap-dynamic-placeholder.ts` and `.test.ts`
- `apps/web/components/task/chat/use-tiptap-editor.ts`, `tiptap-input.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`,
  `chat-input-container.tsx`, `use-chat-input-container.ts`,
  `chat-input-body.tsx`
- `apps/web/components/task/chat/composer-prompt-suggestion-hint.tsx`
- `apps/web/app/globals.css`
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/chat/prompt-suggestions.spec.ts`
- `apps/web/e2e/tests/chat/mobile-prompt-suggestions.spec.ts`
- `docs/public/developer-tools.md`

## Dependencies

- Task 01: the preference and the fallback built-in.
- Task 02: native events, metadata, and mock agent support.

## Risks

- **Enter sends.** Only claim the submit shortcut when ghost text is visible and
  no menu is open. Unit-test the Enter and Cmd/Ctrl+Enter settings.
- **Tab and Escape regressions.** Unit-test both the visible and not-visible
  branches.
- **Store churn while streaming.** Memoize selectors, as
  `createMessageHistorySelector` does. Compute turn keys only from completed
  state.
- **Placeholder ownership.** Keep the `pickInputPlaceholder` and
  `getInputPlaceholder` order unchanged.
- **TS lint limits.** Extract helpers instead of growing `ChatInputArea` or
  `useTipTapEditor`.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/composer-prompt-suggestions.md`
- `docs/specs/ui/system-design/composer-prompt-suggestions.md`
- `docs/plans/composer-prompt-suggestions/plan.md` (UI previews, E2E plan)
- `apps/web/AGENTS.md`, `docs/i18n.md`, `/mobile-parity`, `/e2e`
- `apps/web/hooks/use-summarize-session.ts` (sessionless utility call)
- `apps/web/e2e/tests/task/enhance-prompt.spec.ts` (utility route stub)
- `apps/web/components/task/chat/chat-input-focus-hint.tsx` (hint slot)

## Results

Implemented.

- **Native suggestions.** They live in the session's `metadata.prompt_suggestion`,
  written by `session.prompt_suggestion` and restored by hydration. There is no
  separate store slice.
- **Fallback.** Results and dismissals live in `kandev.promptSuggestion.v1`.
- **Ghost text.** It renders from its own `data-prompt-suggestion` attribute, so
  `data-placeholder` keeps the normal placeholder. Existing E2E idle detection
  depends on that.
- **Native detection.** It uses the agent capability, resolved through the
  session's profile or the snapshot agent name. When the store has no turns
  loaded, the backend's state-guarded turn binding is accepted.
- **Hint chip.** It sits in the composer's top-right corner. The ghost text
  reserves right padding so the two never overlap. Screenshots on desktop and
  phone confirmed that the chip no longer covers the toolbar or the send
  button.
- **Mock agent scope.** It advertises native suggestions only as `mock-agent`
  and `claude-acp`. Stand-ins for other providers keep the fallback path.

Fixes found by E2E:

- The WebSocket session broadcaster needs `GetSessionID()` on the payload struct.
  Added with a regression test. Before the fix the event was published but never
  routed.
- Overwriting `data-placeholder` broke the shared E2E idle helper. The
  suggestion moved to its own attribute.

Red-Green evidence. Each test failed before its implementation:

- `lib/prompt-suggestion.test.ts`
- `hooks/use-prompt-suggestion.test.ts`, including the no-turns native case
- `lib/ws/handlers/prompt-suggestions.test.ts`
- `components/task/chat/tiptap-prompt-suggestion.test.ts`
- `components/task/chat/use-prompt-suggestion-escape.test.tsx`
- the `settings/types.test.ts` capability case

Verification:

- Unit tests for chat, settings, ws, settings slices, ssr, quick-chat, and
  hooks/lib suggestions: 500 files and 4,248 tests, run before the final
  additions. All pass after the composer-test mocks were added.
- `pnpm e2e:run tests/chat/prompt-suggestions.spec.ts`: 4 passed. Covers native,
  fallback, preference off, and Quick Chat Escape.
- `pnpm e2e:run --project mobile-chrome tests/chat/mobile-prompt-suggestions.spec.ts`:
  1 passed.
- `pnpm e2e:run tests/settings/prompt-suggestion-settings.spec.ts`: 1 passed.
- `make typecheck`, `make lint` (backend 0 issues; web, harness, specs, and
  architecture clean), and `pnpm run i18n:check` passed.
