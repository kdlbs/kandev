---
id: "02-desktop-markdown-modes"
title: "Integrate Desktop Markdown Modes"
status: completed
wave: 2
depends_on: ["01-hybrid-markdown-adapter"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.1
  - AC-UI-MARKDOWN-FILE-EDITING-001.4
  - AC-UI-MARKDOWN-FILE-EDITING-001.5
  - AC-UI-MARKDOWN-FILE-EDITING-001.7
  - AC-UI-MARKDOWN-FILE-EDITING-002.2
  - AC-UI-MARKDOWN-FILE-EDITING-002.3
  - AC-UI-MARKDOWN-FILE-EDITING-002.4
  - AC-UI-MARKDOWN-FILE-EDITING-002.5
  - AC-UI-MARKDOWN-FILE-EDITING-002.8
  - AC-UI-MARKDOWN-FILE-EDITING-003.1
  - AC-UI-MARKDOWN-FILE-EDITING-003.5
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 02: Integrate Desktop Markdown Modes

## Summary

Compose Preview, Edit, and Source around the current desktop file buffer. Add
explicit mode state, migrate restored Boolean state, and preserve all existing
file actions.

## In scope

- `MarkdownFileMode` and persisted open-file state migration.
- Desktop and tablet mode control with localized accessible labels.
- Markdown coordinator for Preview, Edit, and Source.
- MDX mode restriction and hybrid failure fallback.
- Save, dirty, update, comment, link, table, Mermaid, and baseline integration.

## Out of scope

- Mobile file surface changes.
- Review-local Markdown preview flags.
- General code-editor provider changes.

## Acceptance

- New Markdown files start in Preview. `.md` offers three modes and `.mdx`
  offers Preview and Source.
- All modes use one unsaved buffer and preserve file actions, dirty state,
  selection intent, and safe preview behavior.
- Restored legacy records migrate deterministically, and hybrid failure exposes
  Source without losing content.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- --run \
  components/task/markdown-file-editor.test.tsx \
  components/task/task-center-panel-file-tabs.test.ts \
  components/task/file-tab-content.test.tsx \
  lib/local-storage.test.ts
pnpm --dir web run i18n:check
pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/components/task/markdown-file-editor.tsx`
- `apps/web/components/task/markdown-file-editor.test.tsx`
- `apps/web/components/task/file-editor-panel.tsx`
- `apps/web/components/task/file-editor-content.tsx`
- `apps/web/components/task/file-editor-toolbar.tsx`
- `apps/web/components/task/file-tab-content.tsx`
- `apps/web/components/task/task-center-panel.tsx`
- `apps/web/components/task/task-center-panel-restoration.ts`
- `apps/web/components/task/task-center-panel-file-tabs.ts`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/local-storage.ts`
- `apps/web/lib/types/workspace-files.ts`
- `apps/web/src/locales/`

## Dependencies

Task 01 supplies the adapter and source-fidelity contract.

## Risks

- A broad state rename can alter Review-local behavior by mistake.
- Source edits can leave stale hybrid redo entries.
- New toolbar copy must remain complete in all five locales.

## Parallelism

`sequential`

## Inputs

- All requirement sections.
- System design sections `Mode model`, `Open-file state`, `Control flow`, and
  `Desktop and tablet`.
- Current file editor, restoration, local-storage, and preview tests.

## Results

- Added the explicit `preview`, `edit`, and `source` mode contract with MDX
  restricted to Preview and Source.
- Integrated the coordinator into desktop file panels and task tabs, including
  keyboard Save, dirty/update actions, Source fallback, and persisted modes.
- Added one-way storage migration for legacy Boolean records and localized mode
  controls in all supported catalogs.
- Focused Task 02 tests, localization checks, web typecheck, and lint pass.
