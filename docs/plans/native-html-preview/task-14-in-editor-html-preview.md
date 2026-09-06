---
id: "14-in-editor-html-preview"
title: "Move desktop HTML preview into the file editor"
status: done
wave: 5
depends_on:
  - "13-browser-fidelity-docs-and-e2e"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
  - AC-UI-NATIVE-HTML-PREVIEW-001.9
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---

# Task 14: Move desktop HTML preview into the file editor

## Summary

Replace automatic Browser-panel opening with the same focused iframe used by
the mobile file viewer. Preserve the file tab, unsaved buffer, dirty state,
source recovery, and explicit access to the Browser panel.

## In scope

- Shared desktop publication state scoped to session, repository, and path.
- In-editor loading, iframe, failure, retry, refresh, and `Show code` states.
- An explicit secondary action that opens the published URL in a Browser panel.
- Monaco, CodeMirror, Dockview file-panel, and task-center file-tab wiring.
- Removal of obsolete desktop publish hooks and shared global busy state.
- Focused component and controller tests, including stale completion guards.

## Out of scope

- Agentctl server or task-session API changes.
- Markdown preview behavior.
- Automatic shutdown of the session-scoped shared preview server.

## Acceptance

- Activating `Preview HTML` replaces the active editor body without creating a
  Browser panel and publishes the current unsaved buffer.
- `Show code` restores the same file buffer and dirty state; refresh republishes
  the latest buffer and retry recovers from publication failure.
- Preview state resets on file or session identity change, and only the explicit
  secondary action opens the Browser panel.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/task/file-editor-content.test.tsx components/task/file-tab-content.test.tsx components/task/html-preview-content.test.tsx hooks/use-html-preview-publisher.test.ts components/editors/monaco/monaco-editor-toolbar.test.tsx components/editors/codemirror/codemirror-code-editor.preview.test.tsx
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task/file-editor-content.tsx`
- `apps/web/components/task/html-preview-content.tsx`
- `apps/web/components/task/file-editor-panel.tsx`
- `apps/web/components/task/file-tab-content.tsx`
- `apps/web/components/task/task-center-panel.tsx`
- `apps/web/hooks/use-html-preview-publisher.ts`
- Obsolete desktop HTML-preview hooks and tests.
- `apps/web/src/locales/**`

## Dependencies

Task 13 supplies the trusted static server, publish API, iframe surface, and
browser-fidelity coverage.

## Risks

- A stale response can reveal preview for a previous file or session.
- Replacing the editor body can accidentally discard unsaved parent state.
- Shared toolbar changes can regress Markdown preview controls.

## Parallelism

`sequential`

## Inputs

- Acceptance criteria `.1`, `.2`, `.3`, and `.7` through `.10`.
- Desktop control flow and responsive contract in the system design.
- Existing Markdown toggle and mobile HTML preview as shipped exemplars.

## Results

- Desktop `Preview HTML` now replaces the active editor body with the shared
  native-browser iframe while preserving the current unsaved buffer and dirty
  state. `Show code` restores the same editor state, and refresh republishes the
  latest buffer.
- Preview identity is scoped to session, repository, and path. File or session
  changes immediately hide stale previews, reset the request generation, and
  prevent late responses from opening or mutating the new file.
- Browser-panel opening is now an explicit secondary action. The obsolete
  automatic-open hooks and global publish-busy plumbing were removed.
- Focused Vitest coverage passed: 6 files and 22 tests. Web typecheck, lint,
  i18n checks, and the changed-line i18n ratchet also passed.
