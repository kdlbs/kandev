---
id: "02-responsive-html-preview"
title: "Add responsive HTML preview surfaces"
status: done
wave: 2
depends_on:
  - "01-preview-state-and-sandbox"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 02: Add responsive HTML preview surfaces

## Summary

Render eligible HTML buffers in a sandboxed iframe from every editable file
surface. Reuse the Markdown preview interaction on desktop and mobile while
keeping Markdown rendering and comments unchanged.

## In scope

- Add `HtmlPreviewContent` with its toolbar, recoverable error state, and
  sandboxed iframe.
- Generalize file-editor preview props and toolbar buttons for preview kind.
- Wire generic state through Dockview and center-panel file editors.
- Add the same capability to the focused mobile file viewer with a 44-pixel
  touch target and file-identity reset.
- Add complete localized labels in all shipped locales and regenerate derived
  locale data with repository scripts.
- Add component tests before production component changes.

## Out of scope

- Review toolbar or diff rendering changes.
- Workspace-relative and remote resources.
- HTML source-line comments or Browser-panel inspection.
- Public docs and Playwright scenarios.

## Acceptance

- Monaco, CodeMirror, center-panel, Dockview, and mobile file viewers expose
  HTML preview only for eligible text files. Source restoration does not change
  the buffer or dirty state.
- The preview iframe has the exact sandbox, referrer, title, and CSP document
  contract. A construction error retains a localized route back to source.
- Markdown behavior, other file actions, mobile focus, touch sizing, and scroll
  containment remain unchanged.

## Verification

```bash
cd apps/web && pnpm exec vitest run components/task/html-preview-content.test.tsx components/task/file-editor-content.test.tsx components/task/file-tab-content.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx components/editors/monaco/monaco-editor-toolbar.test.tsx components/editors/codemirror/codemirror-code-editor.preview.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task/html-preview-content.tsx`
- `apps/web/components/task/html-preview-content.test.tsx`
- `apps/web/components/task/file-editor-content.tsx`
- `apps/web/components/task/file-editor-content.test.tsx`
- `apps/web/components/task/file-editor-panel.tsx`
- `apps/web/components/task/file-tab-content.tsx`
- `apps/web/components/task/file-tab-content.test.tsx`
- `apps/web/components/task/task-center-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`
- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx`
- `apps/web/components/editors/monaco/monaco-editor-toolbar.test.tsx`
- `apps/web/components/editors/codemirror/codemirror-code-editor.tsx`
- `apps/web/components/editors/codemirror/codemirror-code-editor.preview.test.tsx`
- `apps/web/src/locales/en/editors.json`
- `apps/web/src/locales/pt-pt/editors.json`
- `apps/web/src/locales/zh-cn/editors.json`
- `apps/web/src/locales/zh-hk/editors.json`
- `apps/web/src/locales/zh-tw/editors.json`
- `apps/web/src/locales/pseudo/editors.json`

## Dependencies

- Task 01 supplies preview kind, generic state, legacy restoration, and the HTML
  document builder.

## Risks

- Toolbar contracts are duplicated across Monaco and CodeMirror and can drift.
- The opaque iframe is intentionally not available to Kandev's Markdown comment
  DOM traversal or Browser-panel inspection hooks.
- Full-document HTML can impose its own scroll and color scheme inside the
  frame. The Kandev surface must keep its header and outer containment stable.

## Parallelism

`sequential`

## Inputs

- All acceptance criteria in `REQ-UI-NATIVE-HTML-PREVIEW-001`.
- The rendered file surfaces, security boundary, responsive contract, failure,
  and compatibility sections of the system design.
- Existing Markdown preview, Monaco/CodeMirror toolbar, and focused mobile file
  viewer components and tests.

## Results

Implemented the sandboxed HTML renderer and wired the format-neutral preview
state through desktop and mobile file surfaces. Added localized controls and
component coverage for HTML, Markdown, unsupported files, binary files,
toolbar selection, mobile identity reset, touch sizing, and source restoration.
Verification passed:

```text
pnpm exec vitest run components/task/html-preview-content.test.tsx components/task/file-editor-content.test.tsx components/task/file-tab-content.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx components/editors/monaco/monaco-editor-toolbar.test.tsx components/editors/codemirror/codemirror-code-editor.preview.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```
