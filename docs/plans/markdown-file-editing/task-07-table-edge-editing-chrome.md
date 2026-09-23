---
id: "07-table-edge-editing-chrome"
title: "Add Table Edge Editing Chrome"
status: pending
wave: 7
depends_on: ["06-positional-table-edits"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-002.9
  - AC-UI-MARKDOWN-FILE-EDITING-002.10
  - AC-UI-MARKDOWN-FILE-EDITING-002.11
  - AC-UI-MARKDOWN-FILE-EDITING-002.12
  - AC-UI-MARKDOWN-FILE-EDITING-003.7
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 07: Add Table Edge Editing Chrome

## Summary

Replace the overlapping top-right table toolbar with Confluence-style edge
insertion controls and source-neutral column resizing. Keep the interaction
usable with mouse, keyboard, and coarse-pointer touch input.

## In scope

- Hide the active delimiter row without changing canonical source.
- Render row insertion controls in a left gutter and column insertion controls
  in a top gutter outside editable cells.
- Reveal compact fine-pointer actions on hover or focus and show discoverable
  44-pixel coarse-pointer actions without hover.
- Add drag and keyboard column resize handles with a bounded minimum width.
- Retain transient widths across mode switches while the file tab stays open.
- Reconcile controls and widths after upstream table DOM rerenders.

## Out of scope

- Creating tables from the file toolbar.
- Deleting or reordering rows and columns.
- Header, numbered-column, chart, macro, merged-cell, or persisted-width
  semantics.

## Acceptance

- No table control covers or intercepts an editable cell, including the
  top-right header cell.
- Row and column insertion acts at the selected edge and participates in one
  undoable source edit; resizing changes no Markdown bytes.
- Desktop and mobile production E2E prove delimiter hiding, cell editability,
  positional insertion, resizing, contained overflow, and touch targets.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/editors/markdown/hybrid-markdown-editor.test.tsx components/editors/markdown/markdown-table-edit.test.ts
pnpm run typecheck
pnpm run i18n:check
pnpm e2e:run tests/task/markdown-file-editing.spec.ts
pnpm e2e:run --project=mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts
cd ../..
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.css`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.test.tsx`
- `apps/web/e2e/tests/task/markdown-file-editing.spec.ts`
- `apps/web/e2e/tests/task/mobile-markdown-file-editing.spec.ts`
- `apps/web/src/locales/*/common.json`

## Dependencies

- Task 06 positional source edits.

## Risks

- Mutation and resize observers can form a feedback loop if geometry writes
  trigger unbounded reconciliation.
- Touch hit areas must remain large without obscuring narrow columns.
- Wide resized tables must stay inside the existing local horizontal scroller.

## Parallelism

`sequential`

## Inputs

- `AC-UI-MARKDOWN-FILE-EDITING-002.9` through `.12` and `003.7`.
- Confluence edge-control interaction reference.
- Existing hybrid adapter lifecycle and desktop/mobile Markdown E2E fixtures.

## Results

Pending.
