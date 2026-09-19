---
id: "02-add-sortable-tab-strip"
title: "Add sortable tab interactions"
status: completed
wave: 2
depends_on:
  - "01-persist-tab-order"
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-002
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-002.2
  - AC-UI-QUICK-TERMINAL-002.3
  - AC-UI-QUICK-TERMINAL-002.8
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
---

# Task 02: Add Sortable Tab Interactions

## Summary

Render one sortable list for persisted conversations and terminals. Provide equivalent mouse,
touch, keyboard, and coarse-pointer move paths.

## In scope

- Add the dnd-kit list and mouse, touch, and keyboard sensors.
- Add visible coarse-pointer tab actions with directional moves and close.
- Contain horizontal overflow inside the tab strip.

## Out of scope

- Title persistence and order resolution internals.
- Rename input styling and Save or Cancel behavior.
- Terminal title editing.

## Acceptance

- Every input mode changes the same ordered state and preserves the active tab.
- Touch scrolling and ordinary tab selection remain usable before drag activation.
- Coarse-pointer actions meet the 44 CSS pixel target and cause no document overflow.

## Verification

```bash
(cd apps/web && pnpm vitest run components/quick-chat lib/state/slices/ui)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps && pnpm --filter @kandev/web lint)
```

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-modal.tsx`
- A sortable strip component under `apps/web/components/quick-chat/`
- `apps/web/components/quick-chat/quick-chat-tab-item.tsx`
- `apps/web/components/quick-chat/quick-terminal-tab-item.tsx`
- Quick Chat component tests and required locale catalogs

## Dependencies

Task 01.

## Risks

- Pointer event boundaries can turn a click or close action into a drag.
- The tab item is also changed by Task 03, so the work orders run in sequence.

## Parallelism

`sequential`

## Inputs

- UI system design sections for interaction, accessibility, and mobile behavior.
- `sidebar-view-chips.tsx` as the existing horizontal sensor pattern.

## Results

Implemented one mixed sortable strip for conversation and terminal tabs with mouse, delayed touch,
and keyboard sensors. Added mobile directional controls, 44 CSS pixel coarse-pointer targets, and
contained horizontal overflow. The focused component, state, typecheck, i18n, and lint checks pass.
