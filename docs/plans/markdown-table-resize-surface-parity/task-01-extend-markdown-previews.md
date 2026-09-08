---
id: "01-extend-markdown-previews"
title: "Extend resize controls to Markdown previews"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-RESIZABLE-MARKDOWN-TABLES-001
acceptance_criteria:
  - AC-UI-RESIZABLE-MARKDOWN-TABLES-001.1
  - AC-UI-RESIZABLE-MARKDOWN-TABLES-001.3
  - AC-UI-RESIZABLE-MARKDOWN-TABLES-001.7
  - AC-UI-RESIZABLE-MARKDOWN-TABLES-001.8
  - AC-UI-RESIZABLE-MARKDOWN-TABLES-001.9
  - AC-UI-RESIZABLE-MARKDOWN-TABLES-001.11
  - AC-UI-RESIZABLE-MARKDOWN-TABLES-001.12
  - AC-UI-RESIZABLE-MARKDOWN-TABLES-001.13
system_design:
  - ../../specs/ui/system-design/resizable-markdown-tables.md
---

# Task 01: Extend Resize Controls to Markdown Previews

## Summary

Make rendered Markdown file previews use the existing accessible table-resize
renderer. Preserve preview source/comment behavior and a single table-local
horizontal scroll owner across desktop and phone layouts.

## In scope

- Compose the shared resizable table into `MarkdownPreviewRenderer`.
- Preserve source-line attributes, comment highlighting and badges, links, and
  interactive-target delegation.
- Keep one horizontal scroll owner and aligned separators.
- Add focused component, desktop Files-preview, and mobile Files-preview tests.

## Out of scope

- Changing shared resize geometry or chat behavior except for a proven defect.
- Changing raw-HTML sanitization.
- Adding touch resize controls.
- Changing review or share preview behavior outside inherited renderer effects.

## Acceptance

- A desktop rendered file table exposes the existing accessible separators and
  preserves adjacent-only resize, reset, minimum-width, and cleanup behavior.
- Source-range comments and normal interactive content still work, and the table
  has exactly one local horizontal scroll owner.
- A phone file preview exposes no separator, remains readable, and causes no
  document horizontal overflow.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web exec vitest run \
  components/task/markdown-preview-content.test.ts \
  components/task/markdown-preview-content.external-link.test.tsx \
  components/shared/use-markdown-table-resize.test.ts \
  lib/markdown/table-resize.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web exec eslint \
  components/shared/resizable-markdown-table.tsx \
  components/task/markdown-preview-content.tsx \
  components/task/markdown-preview-content.test.ts \
  e2e/tests/chat/markdown-preview.spec.ts \
  e2e/tests/task/mobile-file-viewer.spec.ts)
(cd apps/web && pnpm e2e:run tests/chat/markdown-preview.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-file-viewer.spec.ts)
git diff --check
```

## Files likely touched

- `apps/web/components/shared/resizable-markdown-table.tsx`
- `apps/web/components/task/markdown-preview-content.tsx`
- `apps/web/components/task/markdown-preview-content.external-link.test.tsx`
- `apps/web/e2e/tests/chat/markdown-preview.spec.ts`
- `apps/web/e2e/tests/task/mobile-file-viewer.spec.ts`

## Dependencies

None.

## Risks

- Source/comment wrappers can accidentally become a second scroll owner.
- Preview comment delegation can treat a resize press as a table-comment click
  unless the separator remains an interactive element.
- Wide-table separator geometry must use the shared scroll root's coordinate
  system.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/resizable-markdown-tables.md`
- `docs/specs/ui/system-design/resizable-markdown-tables.md`
- Existing chat renderer in
  `apps/web/components/shared/resizable-markdown-table.tsx`
- Existing preview source/comment wrapper in
  `apps/web/components/task/markdown-preview-content.tsx`

## Results

Implemented shared wrapper composition so rendered Markdown file previews use
the existing accessible table resizer while `SourceBlock` remains the single
scroll root and retains source/comment behavior.

Validation completed 2026-08-24:

- Focused Vitest suite: 18 tests passed.
- Web typecheck and scoped ESLint: passed.
- Desktop Markdown preview Playwright suite: 8 tests passed.
- Mobile Chrome file-viewer Playwright suite: 9 tests passed.
- `git diff --check`: passed.
