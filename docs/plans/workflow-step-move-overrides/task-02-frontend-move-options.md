---
id: "02-frontend-move-options"
title: "Frontend move options"
status: completed
wave: 2
depends_on:
  - "01-backend-move-contract-and-entry"
plan: "plan.md"
spec: "../../specs/workflow-step-move-overrides/spec.md"
---

# Task 02: Frontend move options

## Scope

Add the shared one-shot move options form and connect it to workflow-stepper, chat next-step, and passthrough next-step surfaces. Keep the existing direct actions as no-override fast paths and make the options interaction responsive for desktop and touch.

## Likely files

- apps/web/lib/api/domains/kanban-api.ts and the shared frontend move type location.
- A new shared move-options form and desktop/mobile surface component under apps/web/components/task.
- apps/web/components/task/workflow-stepper.tsx.
- apps/web/hooks/domains/kanban/use-plan-actions.ts.
- apps/web/components/task/chat/chat-input-area.tsx.
- apps/web/components/task/passthrough-toolbar.tsx.
- apps/web/src/locales/en/task.json, pt-pt/task.json, pseudo/task.json, and zh-cn/task.json.
- Focused component and hook tests beside the changed code.

## Acceptance

- A stepper target can be moved directly or opened in the options form. The form submits the selected target plus reset_context, instructions, and agent_profile_id with no live session mutation before submission.
- Chat and passthrough next-step controls keep the direct action and use the same anchored options surface for the same next step.
- Agent profile suggestions use existing store/capability data and communicate that the value is for this move only; instructions copy explains that it is appended to the target workflow prompt.
- Desktop uses the existing hover/popover convention. Touch devices use a bottom Drawer with one-column controls, safe-area spacing, no horizontal overflow, and at least 44px actionable controls.
- All new copy is localized in every required catalog, and the form handles loading, validation, submit failure, and close behavior without leaving stale state.
- Tests cover payload construction, direct versus options actions, profile/reset/prompt state, chat/passthrough parity, and touch Drawer rendering.

## Verification

    cd apps && pnpm --filter @kandev/web exec vitest run components/task/workflow-stepper.test.tsx components/task/workflow-move-overrides.test.tsx components/task/passthrough-toolbar.test.tsx hooks/domains/kanban/use-plan-actions.test.ts

    cd apps/web && pnpm run typecheck
    cd apps/web && pnpm run i18n:check
    cd apps/web && pnpm run i18n:ratchet

## Handoff

Implementation complete. The focused frontend suites pass (28 tests), Prettier, i18n checks, and the earlier typecheck pass are recorded; a final typecheck rerun was killed by the environment memory limit.
