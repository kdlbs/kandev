---
spec: docs/specs/ui/agent-launch-prompt-composer.md
created: 2026-08-02
status: complete
---

# Implementation Plan: Agent Launch Prompt Composer

## Overview

The New Agent and handoff flows currently render a bespoke `SessionPromptField`, while task creation renders `TaskFormInputs`. The bespoke field independently implements attachments and enhancement but never installs `useTaskCreatePromptMention` or the voice control, so `@saved-prompt` remains literal text in the affected dialog. The repair replaces that duplicate field with `TaskFormInputs` in session mode and adapts the surrounding context, enhancement, and launch hooks to its existing `TaskFormInputsHandle` contract before adding desktop and mobile regressions.

## Confirmed root cause and reproduction

- `apps/web/components/task/new-session-dialog.tsx` renders `SessionPromptField` from `apps/web/components/task/new-session-form-prompt.tsx`.
- `SessionPromptField` is a plain controlled `Textarea`; it has attachment and enhancement buttons but no `useTaskCreatePromptMention`, `MentionMenu`, or `VoiceInputButton` wiring.
- `apps/web/components/task-create-dialog-selectors.tsx` already provides those capabilities in exported `TaskFormInputs`, including a session-mode layout and the imperative `TaskFormInputsHandle` used by session submissions elsewhere.
- Smallest reproduction: create a saved custom prompt, open `/t/:taskId`, open New Agent, and type `@<prompt-name>`. No mention menu opens, while the same input in Create Task opens the menu and inlines the selected prompt.

## Backend

No backend, persistence, protocol, or session-launch contract changes are required. The existing launch request already accepts prompt text and message attachments.

## Frontend

### Shared composer integration

- In `apps/web/components/task/new-session-dialog.tsx`, render `TaskFormInputs` with `isSessionMode`, autofocus, disabled/busy state, enhancement callbacks, and voice auto-send. Keep `PromptResultRecovery` adjacent to the shared composer as it is in the task-create flow.
- Replace the controlled-textarea shim with a `RefObject<TaskFormInputsHandle | null>`. Context copy/blank/summary, guarded enhancement delivery, submit enablement, Ctrl/Cmd+Enter, and voice auto-send all read or mutate through `getValue()` / `setValue()`.
- In `apps/web/components/task/new-session-form-actions.ts`, accept the shared composer handle and build the launch request from `getValue()` plus `getAttachments()`. Preserve the current context fallback, task environment, profile selection, activation, close, and error behavior.
- Remove `apps/web/components/task/new-session-form-prompt.tsx` after the New Agent dialog uses the shared component. Keep the focused controller regression in `new-session-form-prompt.test.tsx` because the hook now lives in `new-session-dialog.tsx`; leave `session-dialog-shared.tsx` in place because subtask composition still consumes its attachment helpers.

### Existing shared component

No new composer capability is planned. `TaskFormInputs` already owns saved-prompt mentions, file and image handling, enhancement controls, voice insertion/auto-send, session-mode sizing, and the stable `task-description-input` selector. Change it only if the integration reveals a narrowly required handle or accessibility adjustment.

### Mobile design contract

- **Desktop outcome / mobile entry:** desktop continues opening New Agent from the session tab/header controls; phone continues through `mobile-sessions-pill` and the visible `mobile-launch-session` action.
- **Nearest shipped exemplar:** `apps/web/e2e/tests/session/mobile-handoff.spec.ts` and `apps/web/components/task/mobile/mobile-sessions-section.tsx` establish the existing mobile entry and the same `NewSessionDialog` surface.
- **Hierarchy and primary action:** environment and profile context remain above the composer, Start Agent remains the primary footer action, and prompt-authoring tools stay inside the shared composer toolbar.
- **Presentation:** retain the existing centered Dialog because this is a short, temporary launch form rather than primary dense content. The prompt textarea remains the single bounded scroll owner for long prompt content; the mention menu uses its existing overlay behavior.
- **Touch and geometry:** reuse the shipped composer controls and touch selection behavior; verify the dialog and mention interaction fit the Pixel 5 viewport and do not introduce document horizontal overflow.
- **Shared versus responsive logic:** prompt state, mention selection, attachments, enhancement, voice, and launch handlers are shared. Only the existing mobile entry composition differs.

## Tests

- **What:** context copy and automatic handoff summary update the shared composer; guarded enhancement and submit read the current shared value; accepted attachments come from the shared handle; voice auto-send uses the same submit path.
  **Files:** `apps/web/components/task/new-session-dialog.test.tsx`, `apps/web/components/task/new-session-form-actions.test.ts`.
  **How:** React Testing Library with the shared composer handle exposed by the component mock, plus focused hook/action tests that assert the built launch request and non-submit menu key handling.
- **What:** the reused composer continues to provide voice, attachment feedback, saved-prompt mention wiring, and session-mode layout.
  **File:** `apps/web/components/task-create-dialog-selectors.test.tsx`.
  **How:** retain the existing focused shared-component suite; add only a regression that is missing for the agent-launch integration contract.
- **Targeted command:** `cd apps && pnpm --filter @kandev/web test -- --run components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.test.ts components/task-create-dialog-selectors.test.tsx`.

## E2E Tests

- **Scenario:** given a task and saved prompt, selecting `@prompt` inside New Agent inserts the prompt without submitting, and Start Agent then creates a second session with that prompt.
  **File:** `apps/web/e2e/tests/session/new-session-dialog.spec.ts`.
  **What to verify:** mention menu appears, Enter inserts content while the dialog remains open and session count stays one, explicit Start Agent closes the dialog and creates/activates session two.
- **Scenario:** the same saved-prompt selection and launch are reachable from the phone session controls.
  **File:** `apps/web/e2e/tests/session/mobile-new-session-dialog.spec.ts`.
  **What to verify:** `mobile-sessions-pill` -> `mobile-launch-session` opens the dialog, a prompt result can be tapped, Start Agent creates the new session, dialog geometry remains within the viewport, and document horizontal overflow is absent.
- **Page object:** update `apps/web/e2e/pages/session-page.ts` to target the stable shared-composer test ID and add a phone New Agent helper.
- **Targeted commands:**
  - `cd apps/web && pnpm e2e:run tests/session/new-session-dialog.spec.ts`
  - `cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-new-session-dialog.spec.ts`

## Verification Results

- RED evidence: the original New Agent field was a plain `Textarea` with only local attachment and enhancement controls, so the saved-prompt mention menu and voice control were absent. The controller regression also initially failed against an unavailable shared-handle target until the recovery path was adapted to the imperative handle contract.
- GREEN focused unit suite: `cd apps && pnpm --filter @kandev/web test -- --run components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.test.ts components/task/new-session-form-prompt.test.tsx components/task/session-context-summary.test.ts components/task-create-dialog-selectors.test.tsx` — 5 files, 34 tests passed.
- GREEN typecheck: `cd apps/web && pnpm run typecheck` — passed.
- GREEN i18n guard: `cd apps/web && pnpm run i18n:ratchet` — `0 added + 3 modified file(s) clean`.
- GREEN targeted lint: `cd apps/web && pnpm exec eslint components/task/new-session-dialog.tsx components/task/new-session-dialog.test.tsx components/task/new-session-form-actions.ts components/task/new-session-form-actions.test.ts components/task/new-session-form-prompt.test.tsx components/task/session-context-summary.ts e2e/pages/session-page.ts e2e/tests/session/new-session-dialog.spec.ts e2e/tests/session/mobile-new-session-dialog.spec.ts` — passed.
- GREEN desktop E2E: `cd apps/web && pnpm e2e:run tests/session/new-session-dialog.spec.ts` — 6 tests passed; the saved-prompt test confirmed Enter inserts without submitting and explicit Start Agent creates session two.
- GREEN mobile E2E: `cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-new-session-dialog.spec.ts` — 1 test passed; viewport containment and document overflow assertions passed.
- E2E runs produced no screenshots or traces that need to be checked in. Both prompt fixtures use `afterEach` cleanup by name.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-reuse-shared-composer](task-01-reuse-shared-composer.md) — complete; focused unit, typecheck, i18n, and lint checks pass

Wave 2:

- [x] [task-02-agent-launch-e2e](task-02-agent-launch-e2e.md) — complete; desktop (6) and mobile (1) focused scenarios pass

No task is marked parallel-safe. The E2E contract depends on the component integration, and both tasks touch New Agent test surfaces.

## Risks

- `TaskFormInputs` updates its imperative value synchronously but renders React state asynchronously. Voice auto-send and context-driven submit must read the handle, not a captured render value.
- Enter must remain owned by the open mention menu; Ctrl/Cmd+Enter and voice auto-send must still reach the launch handler only after the shared component updates its handle.
- Handoff uses the same dialog and will inherit the shared composer. Its automatic summary and target profile must remain intact.
- Existing compact composer toolbar geometry is reused unchanged; a broader toolbar sizing or dialog redesign is outside this repair.
