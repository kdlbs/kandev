---
id: "07-responsive-preview-surfaces"
title: "Wire responsive preview surfaces"
status: cancelled
wave: 3
depends_on:
  - "06-preview-state-and-renderer"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 07: Wire responsive preview surfaces

## Summary

Connect the runtime-backed renderer to every existing HTML preview entry point.
Reuse the Markdown toolbar and focused mobile viewer composition without
creating a second execution path or changing unrelated file actions.

## In scope

- Monaco, CodeMirror, Dockview, center-panel, and mobile file-viewer preview
  entry points.
- Localized preview, source, runtime-failure, and unsupported-capability copy
  in all shipped locales.
- Mobile touch sizing, full-height layout, identity reset, and overflow
  containment.
- Component coverage for source restoration, dirty buffers, Markdown parity,
  and runtime failure recovery.

## Out of scope

- Runtime engine or virtual-DOM implementation.
- Public documentation.
- Review-diff preview and Browser-panel behavior.

## Acceptance

- Every eligible text-file surface uses the same runtime-backed HTML preview,
  and returning to source preserves buffer and dirty state.
- Mobile exposes a touch-sized action and the scriptless preview surface does
  not add page-level horizontal overflow.
- Markdown, binary, unsupported-file, download, delete, comment, and external
  editor behavior remains unchanged.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/task/file-editor-content.test.tsx components/task/file-tab-content.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx components/editors/monaco/monaco-editor-toolbar.test.tsx components/editors/codemirror/codemirror-code-editor.preview.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task/file-editor-content.tsx`
- `apps/web/components/task/file-editor-panel.tsx`
- `apps/web/components/task/file-tab-content.tsx`
- `apps/web/components/task/task-center-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- Monaco and CodeMirror toolbar files and tests.
- Five shipped locale files and generated pseudo-locale data.

## Dependencies

Task 06 supplies the runtime-backed renderer and stable failure contract.

## Risks

- Multiple desktop file-editor paths can accidentally create independent worker
  generations or omit disposal on unmount.
- Mobile and Dockview may mount duplicate preview controls, so selectors and
  lifecycle ownership must remain scoped to the active surface.

## Parallelism

`sequential`

## Inputs

- The responsive contract in the system design.
- Existing Markdown preview toolbar and mobile file-viewer tests.
- `/mobile-parity` guidance for native mobile interaction patterns.

## Results

Verified that Monaco, CodeMirror, Dockview, center-panel tabs, and the
focused mobile file viewer all select the same runtime-backed HTML component.
Touch controls retain 44-pixel targets, preview state resets on file identity
changes, and the existing Markdown and file action paths remain separate.

Verification completed:

```text
pnpm exec vitest run components/task/file-editor-content.test.tsx components/task/file-tab-content.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx components/editors/monaco/monaco-editor-toolbar.test.tsx components/editors/codemirror/codemirror-code-editor.preview.test.tsx
pnpm exec tsc --noEmit --pretty false
pnpm run i18n:check
pnpm run i18n:ratchet
```
