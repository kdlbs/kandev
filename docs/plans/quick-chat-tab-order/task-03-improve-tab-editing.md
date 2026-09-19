---
id: "03-improve-tab-editing"
title: "Improve tab editing"
status: completed
wave: 2
depends_on:
  - "02-add-sortable-tab-strip"
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-TERMINAL-002
acceptance_criteria:
  - AC-UI-QUICK-TERMINAL-002.6
  - AC-UI-QUICK-TERMINAL-002.7
system_design:
  - ../../specs/ui/system-design/quick-terminal.md
---

# Task 03: Improve Tab Editing

## Summary

Make the working indicator and rename state visually distinct. Use explicit edit actions that cannot
accidentally close the tab.

## In scope

- Add at least 6 CSS pixels between the grid spinner and title.
- Add the rename input treatment, Save, Cancel, and accessible labels.
- Unify commit, restore, keyboard, blur, and error behavior.

## Out of scope

- Drag sensors and order persistence.
- Terminal title editing.
- Changes to the backing task rename API.

## Acceptance

- Rename mode selects the title and shows a clear input plus Save and Cancel.
- Enter or Save commits once. Escape or Cancel restores once. Blur cannot submit twice.
- Coarse-pointer edit targets meet the mobile size and text requirements.

## Verification

```bash
(cd apps/web && pnpm vitest run components/quick-chat)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps && pnpm --filter @kandev/web lint)
```

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-tab-item.tsx`
- Quick Chat tab-item tests
- Required locale catalogs

## Dependencies

Task 02.

## Risks

- Blur, Save, and Enter can race and submit duplicate renames. One guarded commit path must own the
  request.

## Parallelism

`sequential`

## Inputs

- UI system design sections for rename behavior and spinner spacing.
- The existing task-backed Quick Chat rename helper.

## Results

Implemented the fixed spinner gap and responsive rename editor. Save, Cancel, Enter, Escape, blur,
and IME paths share guarded commit and restore behavior. Coarse-pointer edit controls meet the
mobile target size. The focused Quick Chat tests, typecheck, i18n checks, and lint pass.
