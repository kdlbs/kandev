---
spec: docs/specs/ui/slash-command-composer.md
created: 2026-07-02
status: implemented
---

# Implementation Plan: Slash Command Composer Selection

## Overview

The backend already delivers ACP slash commands through `session.available_commands`, and the frontend already renders them in the shared TipTap composer. The fix is frontend-only: replace the current selection callback that calls `onSubmit('/cmd')` with draft insertion inside the TipTap suggestion command, remove the now-dangerous submit callback plumbing, then add focused unit and E2E coverage for task chat, quick chat, and mobile touch selection.

---

## Backend

No backend changes are planned. The existing ACP conversion and WebSocket path stays unchanged:

- `apps/backend/internal/agent/agentctl/transport/acp/adapter_updates.go`
- `apps/backend/internal/agent/lifecycle/events.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/web/lib/ws/handlers/available-commands.ts`

Reason:

- The bug happens after commands are already in the client store. Selection currently submits from the frontend composer callback.

---

## Frontend

### Slash Command Types and Insertion Text

Files:

- `apps/web/components/task/chat/slash-command-types.ts` (new)
- `apps/web/components/task/chat/tiptap-suggestion.tsx`
- `apps/web/components/task/chat/slash-command-menu.tsx`
- `apps/web/components/task/chat/tiptap-input.tsx`

Changes:

- Move `SlashCommand` and `SlashCommandAction` out of the legacy `apps/web/hooks/use-inline-slash.ts` file into a chat-local type module.
- Add a small pure helper, for example `formatSlashCommandInsertion(command: SlashCommand): string`, that returns a slash-prefixed command plus trailing space for continued typing.
- In `createSlashSuggestion().command`, replace the active TipTap suggestion `range` with `formatSlashCommandInsertion(cmd)` and keep focus in the editor.
- Remove `SlashSuggestionCallbacks.onAgentCommand`; selection must not call an external submit callback.
- Keep the current menu filtering, keyboard navigation, Escape close behavior, and portal-rendered `SlashCommandMenu`.

Reason:

- The selection command is the only place that should mutate the draft. Sending is owned by the normal composer submit path.

### Remove Auto-Submit Plumbing

Files:

- `apps/web/components/task/chat/tiptap-input.tsx`
- `apps/web/components/task/chat/chat-input-body.tsx`
- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task/chat/use-chat-input-container.ts`
- `apps/web/components/task/chat/chat-input-body.test.tsx`
- `apps/web/hooks/use-inline-slash.ts`

Changes:

- Remove the `onAgentCommand` prop from `TipTapInput`.
- Remove `handleAgentCommand` from `ChatInputEditorAreaProps`, `buildEditorAreaProps`, and `useChatInputContainer`.
- Delete `apps/web/hooks/use-inline-slash.ts` after moving the shared types if `rg "useInlineSlash"` confirms no runtime consumers. If deletion is not clean, update the hook to use the same draft insertion helper and remove its `onAgentCommand` callback.
- Update affected mocks and prop builders in tests.

Reason:

- Leaving a `handleAgentCommand -> onSubmit('/cmd')` path in the composer keeps the original bug one import away from returning.

---

## Tests

- **What:** slash command insertion text is normalized and leaves room for arguments.
  **File:** `apps/web/components/task/chat/slash-command-types.test.ts`
  **How:** unit test the exported helper with `agentCommandName: "slow"`, labels with and without a leading slash, and command names that contain punctuation such as `tool:read`.

- **What:** Enter in a suggestion menu still defers to the suggestion plugin instead of submit, including plain-Enter submit mode.
  **File:** `apps/web/components/task/chat/use-tiptap-editor.test.ts`
  **How:** keep the existing `decideSubmitShortcut` regression tests and add a case name that ties it to slash command selection if useful.

- **What:** removing auto-submit plumbing does not break the chat input container's disabled and submit state derivation.
  **File:** `apps/web/components/task/chat/use-chat-input-container.test.ts`
  **How:** update existing tests after `handleAgentCommand` is removed; no new container behavior is expected.

- **What:** TypeScript catches stale prop usage after the prop cleanup.
  **File:** no dedicated file.
  **How:** run:
  ```bash
  cd apps
  pnpm --filter @kandev/web typecheck
  ```

---

## E2E Tests

- **Scenario:** GIVEN a task chat session with advertised commands, WHEN the user types `/s` and presses Enter to choose `/slow`, THEN the composer contains `/slow`, focus remains in the composer, and no user message appears in `.chat-message-list`.
  **File:** `apps/web/e2e/tests/chat/slash-command-composer.spec.ts`
  **What to verify:** selection by Enter inserts a draft and does not auto-send.

- **Scenario:** GIVEN the selected `/slow` draft remains in task chat, WHEN the user types `1s` and clicks Send, THEN the user message `/slow 1s` appears and the mock agent responds through the normal command path.
  **File:** `apps/web/e2e/tests/chat/slash-command-composer.spec.ts`
  **What to verify:** explicit send still sends the edited draft.

- **Scenario:** GIVEN quick chat is open with an initialized mock agent, WHEN the user selects `/slow` from the slash menu, THEN the quick chat editor keeps `/slow` as a draft and no message is sent until explicit send.
  **File:** `apps/web/e2e/tests/chat/slash-command-composer.spec.ts`
  **What to verify:** the shared composer behavior works in quick chat.

- **Scenario:** GIVEN mobile task chat has advertised commands, WHEN the user types `/s` and taps `/slow`, THEN the mobile composer keeps `/slow` as draft text and the Send button remains the only send action.
  **File:** `apps/web/e2e/tests/chat/mobile-slash-command-composer.spec.ts`
  **What to verify:** touch selection has parity with desktop and no workflow depends on hover or a wide viewport.

Recommended E2E command:

```bash
cd apps/web
pnpm e2e:run tests/chat/slash-command-composer.spec.ts tests/chat/mobile-slash-command-composer.spec.ts
```

---

## Implementation Waves

Wave 1:

- [x] [task-01-frontend-selection](task-01-frontend-selection.md)

Wave 2:

- [x] [task-02-unit-tests](task-02-unit-tests.md)

Wave 3:

- [x] [task-03-e2e-verification](task-03-e2e-verification.md)
