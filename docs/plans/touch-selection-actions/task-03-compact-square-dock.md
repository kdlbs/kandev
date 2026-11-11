---
id: "03-compact-square-dock"
title: "Compact square selection dock"
status: done
wave: 3
depends_on:
  - "01-shared-dock-and-chat-messages"
  - "02-preview-and-editor-dock"
plan: "plan.md"
requirements:
  - REQ-UI-TOUCH-SELECTION-ACTIONS-001
acceptance_criteria:
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.3
  - AC-UI-TOUCH-SELECTION-ACTIONS-001.8
system_design:
  - ../../specs/ui/system-design/touch-selection-actions.md
---

# Task 03: Compact Square Selection Dock

## Summary

Phone review on Android showed the full-width dock with a labelled "Comment"
pill wasted a whole row and covered the transcript footer. Match the Plan
dock's Comment action instead: one 32 px square icon in a 44 px target at the
host's bottom-right, with the rest of the row transparent to taps.

## In scope

- `SelectionActionDock`: transparent `pointer-events-none` positioner with a
  right-aligned `pointer-events-auto` toolbar; no bar background or border.
- `SelectionActionDockButton`: icon-only (`IconMessage`), `aria-label` and
  `title`, 32 px `bg-primary` square inside a 44 px hit area.
- Drop the visible `text` prop from all callers.
- Tighten the mobile E2E dock assertions to width and corner position.

## Out of scope

- Positioning, selection tracking, composer, and desktop behavior.
- The Plan editor dock.

## ASCII UI preview

See [plan.md UI-02 and UI-03](plan.md#ascii-ui-preview).

```text
|  Done ->                       [(C)] |  32px square icon, bottom-right
|--------------------------------------|
| [ Message the agent...         ] (>) |
```

## Acceptance

- Dock component tests fail against the full-width bar and pass with the
  compact positioner, icon-only square, and preserved selection.
- `mobile-chrome` E2E asserts the chat and Markdown preview docks are at most
  56 px wide in their host's bottom-right corner.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/shared/selection-action-dock.test.tsx \
  components/task/chat/messages components/editors components/task/markdown-preview-content
pnpm e2e:run --host --shards 1 --project mobile-chrome \
  tests/chat/mobile-agent-message-comments.spec.ts \
  tests/office/mobile-thread-extract-comments.spec.ts \
  tests/task/mobile-file-viewer.spec.ts \
  tests/task/mobile-plan-formatting-toolbar.spec.ts
pnpm run typecheck
pnpm run lint
```

## Files likely touched

- `apps/web/components/shared/selection-action-dock.tsx` (+ test)
- `apps/web/components/task/chat/messages/message-comment-controls.tsx`
- `apps/web/components/editors/editor-selection-comment-trigger.tsx`
- `apps/web/e2e/tests/chat/mobile-agent-message-comments.spec.ts`
- `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts`

## Dependencies

Tasks 01 and 02.

## Risks

- Icon-only actions rely on `aria-label`; keep the localized
  "Comment on selection" name.

## Parallelism

`sequential`

## Results

The dock is now a transparent 48 px positioner with a right-aligned toolbar of
icon-only actions. The Comment action is a 32 px `bg-primary` square with
`IconMessage` inside a 44 px target, named "Comment on selection" through
`aria-label` and `title`. The visible `text` prop was removed from all callers,
and the code editor dock now uses the same icon as the Plan dock.

Verification:

- The three rewritten `selection-action-dock.test.tsx` cases failed against
  the full-width bar (missing positioner classes, visible text, no `title`),
  then passed.
- Focused Vitest across the dock, chat messages, editors, Markdown preview,
  and hooks: 880 passed.
- `mobile-chrome` E2E: `mobile-agent-message-comments` (now asserts the dock
  is at most 56 px wide in the transcript's bottom-right corner),
  `mobile-thread-extract-comments`, `mobile-file-viewer` (same assertion for
  the preview), and `mobile-plan-formatting-toolbar`: 17 passed. Their
  failing run against the old bar was not recorded.
- `pnpm run typecheck`, `pnpm run lint`, Prettier, docs validation, and spec
  lint passed.
