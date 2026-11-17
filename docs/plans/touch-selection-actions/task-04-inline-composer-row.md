---
id: "04-inline-composer-row"
title: "Chat Comment and workflow actions in the composer utility row"
status: done
wave: 4
depends_on:
  - "03-compact-square-dock"
plan: "plan.md"
requirements:
  - REQ-UI-TOUCH-SELECTION-ACTIONS-001
  - REQ-UI-COMPOSER-ACTION-WRAP-001
acceptance_criteria:
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.10
  - AC-UI-COMPOSER-ACTION-WRAP-001.2
  - AC-UI-COMPOSER-ACTION-WRAP-001.6
system_design:
  - ../../specs/ui/system-design/touch-selection-actions.md
  - ../../specs/ui/system-design/composer-action-wrapping.md
---

# Task 04: Chat Comment and Workflow Actions in the Composer Utility Row

## Summary

Phone review showed three stacked rows above the composer: the Comment square,
the workflow "Done" action, and the transcript utility icons. Put all of them
in one row: the workflow action leads, utilities follow, and the Comment icon
joins the end of the utilities while text is selected.

## In scope

- `SelectionActionSlotProvider` / `SelectionActionSlot` /
  `useSelectionActionSlot`; providers in `TaskChatPanel` and
  `QuickChatContent`; slot at the end of `ChatStatusBar`'s utility controls.
- `MessageCommentControls` portals the Comment button into the slot when
  mounted, otherwise uses the dock.
- Phone workflow action no longer forces a full-width row.
- Revise `REQ-UI-COMPOSER-ACTION-WRAP-001` (.2, .6) and its E2E helper.

## Out of scope

- Desktop status-bar order and density.
- Office threads, Markdown preview, and code editors (keep the dock).

## ASCII UI preview

See [plan.md UI-02](plan.md#ascii-ui-preview).

```text
| [Done ->]      [=] [v] [|] [<] [(C)] |  one row: next step, utilities, Comment
| [ Message the agent...         ] (>) |
```

## Acceptance

- Slot, status-bar, and chat-surface tests fail first, then pass.
- `mobile-chrome` E2E: the chat Comment icon is in the utility controls and
  vertically aligned with them; a short workflow action shares the utility row;
  crowded rows wrap without overlap.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/shared components/task/chat components/task/task-chat-panel
pnpm e2e:run --host --shards 1 --project mobile-chrome \
  tests/chat/mobile-agent-message-comments.spec.ts \
  tests/office/mobile-thread-extract-comments.spec.ts \
  tests/workflow/mobile-workflow-step-move-overrides.spec.ts \
  tests/chat/mobile-chat-status-bar.spec.ts \
  tests/task/mobile-file-viewer.spec.ts
pnpm e2e:run --host --shards 1 --project chromium \
  tests/workflow/workflow-step-move-overrides.spec.ts
pnpm run typecheck
pnpm run lint
```

## Files likely touched

- `apps/web/components/shared/selection-action-slot.tsx` (+ test)
- `apps/web/components/task/chat/chat-status-bar.tsx` (+ test)
- `apps/web/components/task/chat/messages/message-comment-controls.tsx`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/quick-chat/quick-chat-content.tsx`
- `apps/web/e2e/tests/workflow/composer-action-layout-helpers.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-step-move-overrides.spec.ts`
- `apps/web/e2e/tests/chat/mobile-agent-message-comments.spec.ts`
- `docs/specs/ui/requirements/composer-action-wrapping.md` and its design

## Dependencies

Task 03.

## Risks

- This reverses the 2026-10-14 decision that gave the phone workflow action
  its own row; the requirement records the correction.
- Long workflow-step names wrap inside the button rather than truncating.

## Parallelism

`sequential`

## Results

Added `SelectionActionSlotProvider` / `SelectionActionSlot` /
`useSelectionActionSlot`. `TaskChatPanel` and `QuickChatContent` provide the
slot and `ChatStatusBar` renders it after its utility controls; the chat
Comment button portals into it when present and otherwise uses the dock. On
phones the workflow action's wrapper is now `display: contents`, so it leads
the shared status/utility row instead of taking its own row.
`REQ-UI-COMPOSER-ACTION-WRAP-001` (.2, .6) and its design were revised, and
the phone branch of `expectComposerActionLayout` now asserts no overlap, with
`wrapped: false` asserting one shared row.

Verification:

- New slot, status-bar, and chat-surface tests failed first (missing module,
  then `w-full` wrapper and missing slot/portal assertions), then passed.
- `mobile-chrome` E2E: `mobile-agent-message-comments` (Comment icon inside the
  utility controls, vertically aligned with them), `mobile-thread-extract-comments`,
  `mobile-workflow-step-move-overrides` (short action shares the utility row),
  `mobile-file-viewer`, and `mobile-chat-status-bar` passed (31 tests, after
  updating the shared-row spec for the inline action).
- `chromium` E2E: `workflow-step-move-overrides` and `agent-message-comments`
  passed (11 tests).
- Related Vitest: 2,829 passed across 274 files. The full web suite was not
  rerun because it cannot finish within this environment's time limit.
- Typecheck, lint, Prettier, i18n, docs, spec, and architecture checks passed.
