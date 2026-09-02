---
created: 2026-08-27
status: completed
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
legacy_specs: []
---

# Implementation Plan: Markdown File Editing

## Overview

Add a source-preserving Edit mode while keeping the current sanitized Preview
and plain-text Source modes. First contain the experimental engine behind a
tested adapter. Then connect one mode and buffer contract to desktop and phone
file surfaces before proving both flows with production-build E2E tests.

## Scope

### In scope

- Hybrid Edit mode for `.md` files.
- Safe Preview and exact Source modes for `.md` and `.mdx` files.
- Open-file mode persistence and legacy Boolean-state migration.
- Existing save, dirty, update, comment, link, table, and comparison behavior.
- Desktop, tablet, and phone file editing outcomes.
- Positional table insertion controls and transient Edit-mode column sizing.
- Source-preservation, security, component, localization, and E2E evidence.

### Out of scope

- Hybrid MDX or JSX editing.
- Review-diff editing.
- Replacement of Kandev's general Monaco and CodeMirror provider contract.
- A backend file API or collaborative editing change.
- Tiptap conversion for arbitrary repository Markdown.
- Persisted Markdown table widths, merged cells, or Confluence-specific table
  semantics.

## Technical approach

### Engine boundary

- Add an exact `@vscode/markdown-editor` version in
  `apps/web/package.json` and `apps/pnpm-lock.yaml`.
- Create `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx` as
  the only upstream import boundary.
- Add Kandev-owned model lifecycle, source replacement, history, comments,
  links, baseline markers, and failure callbacks.
- Add exact-string fixtures and tests under
  `apps/web/components/editors/markdown/`.

### Shared file modes

- Add `MarkdownFileMode` to `apps/web/lib/types/workspace-files.ts` and update
  the open-file state in `apps/web/lib/state/dockview-store.ts`.
- Migrate `markdownPreview` during restoration in
  `apps/web/components/task/task-center-panel-restoration.ts` and
  `apps/web/lib/local-storage.ts`.
- Add `apps/web/components/task/markdown-file-editor.tsx` to compose the
  existing `MarkdownPreviewContent`, hybrid adapter, and selected source editor.
- Replace the eye/code switch in the file toolbar with one localized mode
  control. Keep Review-local preview state unchanged.

### Responsive file surfaces

- Keep `FileEditorPanel` as the desktop and tablet file coordinator.
- Upgrade `MobileFileViewerPanel` to accept the shared editable buffer and save
  actions for Markdown.
- Use the hybrid adapter for phone Edit and editable CodeMirror for phone
  Source.
- Keep one mobile vertical scroll owner, safe-area spacing, 44-pixel controls,
  virtual-keyboard clearance, and local table scrolling.

### Table editing refinement

- Replace append-only helpers in
  `apps/web/components/editors/markdown/markdown-table-edit.ts` with escaped-pipe-aware
  positional row and column insertion while preserving source bytes and line
  endings.
- Replace the top-right table toolbar in `HybridMarkdownEditor` with a
  table-local edge layer that maps visible rows and columns to source actions.
- Hide the upstream delimiter row in the scoped Edit theme while retaining it
  in the AST and canonical source.
- Add pointer, touch, and keyboard column resizing backed by transient
  per-file-tab presentation state rather than Markdown mutations.

## Tests

| Acceptance criteria                                   | Evidence                                                                                                 |
| ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `AC-UI-MARKDOWN-FILE-EDITING-001.2`, `.3`, `.6`       | `components/editors/markdown/hybrid-markdown-editor.test.tsx` and `markdown-source-preservation.test.ts` |
| `AC-UI-MARKDOWN-FILE-EDITING-001.1`, `.4`, `.5`, `.7` | `components/task/markdown-file-editor.test.tsx`                                                          |
| `AC-UI-MARKDOWN-FILE-EDITING-002.1`, `.2`             | Existing `components/task/markdown-preview-content*.test.tsx` plus coordinator regression cases          |
| `AC-UI-MARKDOWN-FILE-EDITING-002.3`, `.4`, `.5`, `.8` | `components/task/markdown-file-editor.test.tsx` and existing file-state tests                            |
| `AC-UI-MARKDOWN-FILE-EDITING-002.6`, `.7`             | Hybrid adapter comment and baseline cases                                                                |
| `AC-UI-MARKDOWN-FILE-EDITING-002.9` through `.12`     | `markdown-table-edit.test.ts` and `hybrid-markdown-editor.test.tsx`                                      |
| `AC-UI-MARKDOWN-FILE-EDITING-003.1`, `.5`             | Desktop mode-control component tests                                                                     |
| `AC-UI-MARKDOWN-FILE-EDITING-003.2`, `.3`, `.4`       | `components/task/mobile/mobile-file-viewer-panel.test.tsx`                                               |
| `AC-UI-MARKDOWN-FILE-EDITING-003.6`                   | Desktop and mobile Playwright flows below                                                                |

## E2E tests

| Flow                                                                           | Acceptance criteria                                                                         | Playwright evidence                                                               |
| ------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| Desktop open, edit, save, reload, Preview, Source, comments, and restored mode | `AC-UI-MARKDOWN-FILE-EDITING-001.1` through `.5`, `002.3` through `.8`, `003.1`, `.5`, `.6` | `apps/web/e2e/tests/task/markdown-file-editing.spec.ts` on `chromium`             |
| Unsafe preview and unsupported-source fallback                                 | `AC-UI-MARKDOWN-FILE-EDITING-001.6`, `.7`, `002.1`, `.2`, `.8`                              | New cases plus existing `apps/web/e2e/tests/chat/markdown-preview.spec.ts`        |
| Phone edit, save, reload, keyboard, controls, and overflow                     | `AC-UI-MARKDOWN-FILE-EDITING-003.2` through `.6`                                            | `apps/web/e2e/tests/task/mobile-markdown-file-editing.spec.ts` on `mobile-chrome` |

## Work orders

- [x] [Task 01: Add the Source-Preserving Markdown Adapter](task-01-hybrid-markdown-adapter.md)
- [x] [Task 02: Integrate Desktop Markdown Modes](task-02-desktop-markdown-modes.md)
- [x] [Task 03: Add Mobile Markdown Editing](task-03-mobile-markdown-editing.md)
- [x] [Task 04: Prove Markdown Editing End to End](task-04-markdown-editor-e2e.md)
- [x] [Task 05: Polish Code and Table Editing](task-05-polish-code-and-table-editing.md)
- [x] [Task 06: Add Positional Markdown Table Edits](task-06-positional-table-edits.md)
- [x] [Task 07: Add Table Edge Editing Chrome](task-07-table-edge-editing-chrome.md)

## Verification results

- `rtk make fmt` passed.
- `rtk make typecheck` passed for all applications.
- `rtk env -u KANDEV_DATABASE_PATH -u KANDEV_HOME_DIR -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE make test` passed. The variables were unset because the task runner injects launcher-selected configuration paths that intentionally override the temporary homes used by backend discovery tests.
- `rtk env -u KANDEV_DATABASE_PATH -u KANDEV_HOME_DIR -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE make lint` passed for backend, web, harness, specifications, and architecture.
- Web i18n checks passed with complete five-locale catalogs and no new copy violations.
- Focused web tests passed 3,264 tests with 4 skips.
- Production-build E2E passed for desktop Chromium (2/2) and mobile-chrome (1/1). Existing desktop Markdown regression passed 8/8 and existing mobile file-viewer regression passed 9/9.
- The code-and-table polish follow-up passed 14 focused adapter tests, web
  typecheck, focused ESLint, i18n checks, specification lint, and the Vite
  production build. Production-build E2E passed 4/4 desktop Chromium cases and
  1/1 mobile-chrome case, including real syntax tokens, bordered tables,
  source-preserving row and column edits, and mobile touch targets.
- Positional table edits and edge chrome passed 18 focused Vitest tests, web
  typecheck, focused ESLint, i18n checks, desktop Markdown E2E (4/4), mobile
  Markdown E2E (1/1), and specification lint. Coverage includes delimiter
  hiding, outside-cell insertion controls, mode-switch width retention,
  coarse-pointer hit targets, touch resizing, and source-preserving saves.

## Risks

- The package is experimental and publishes frequent pre-1.0 versions. One
  adapter, an exact pin, and contract tests contain upgrade risk.
- Raw HTML, MDX, and uncommon syntax can exceed hybrid renderer support.
  Preview and Source must remain available without changing file bytes.
- Selection, history, and scroll can drift across three presentations.
  Boundary tests must use one canonical source mapping.
- A phone virtual keyboard can obscure controls or active text. Mobile E2E must
  test the focused surface with constrained viewport geometry.
- DOM geometry can drift while the upstream editor rerenders an active table.
  The edge layer must reconcile through one observer without feedback loops or
  stale resize handles.
