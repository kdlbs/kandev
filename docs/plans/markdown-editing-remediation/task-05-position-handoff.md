---
id: "05-position-handoff"
title: "Transfer source positions between modes"
status: completed
wave: 5
depends_on: ["04-phone-actions"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-001
  - REQ-UI-MARKDOWN-FILE-EDITING-002
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-001.5
  - AC-UI-MARKDOWN-FILE-EDITING-002.4
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 05: Transfer source positions between modes

## Summary

Transfer selection intent and reading position between Preview, Edit, and
Source. Use one transient source-based contract for desktop and phone.

## In scope

- Add an optional editor-local capture/restore port with anchor and active
  offsets plus a nearest source line. Preserve selection direction.
- Connect Monaco, CodeMirror, the hybrid adapter, and Preview.
- Capture before mode change and restore after the destination becomes ready
  and visible. Scope delayed restores to the live draft and transition.
- Clamp offsets after accepted source replacement. Cancel pending hybrid
  reveals when the surface hides, fails, or unmounts.

## Out of scope

- Persisting cursor state into Markdown or a new backend format.
- Replacing the global point-navigation registry.
- Synchronizing exact pixel scroll positions between different renderers.

## Acceptance

- `transfers selection and nearest line between Markdown modes` first fails
  with the existing independent surfaces. It then passes in both directions
  through Monaco, CodeMirror, hybrid Edit, and Preview where supported.
- Hidden or disposed surfaces cannot steal focus or scroll. Switching files
  or replacing source invalidates stale restoration work and clamps ranges.
- A long-document browser scenario retains useful reading context and exact
  source bytes on desktop and phone. Normal typing does not trigger restoration.

## UI preview

UI-01 retains its desktop and phone composition. Only the active editor body
changes. A mode switch brings the selected source region, or nearest mapped
line, into view. Preview retains source selection intent when its renderer
cannot show an exact editable selection. The fixed toolbar stays in place.
See the [combined preview](plan.md#ui-preview).

## Verification

Add a focused `markdown-source-position.test.ts` for offset mapping and
clamping. Extend provider tests with the optional capture/restore port.
Test reversed selection, CRLF, empty files, hidden initialization, and a stale
transition after changing files.

Add the browser scenario prefix `review regression: position handoff`.
Select text below the fold, switch modes, and inspect actual selection and
visible source-tagged blocks. Run the failing regression before the fix.
Then run this complete block from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/task/markdown-source-position.test.ts components/task/markdown-file-editor.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx components/editors/markdown/hybrid-markdown-editor.test.tsx components/editors/codemirror/codemirror-code-editor.cursor.test.tsx components/editors/monaco/use-monaco-cursor-navigation.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project=chromium tests/task/markdown-file-editing.spec.ts --grep "review regression: position handoff")
(cd apps/web && pnpm e2e:run --project=mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts --grep "review regression: position handoff")
```

## Files likely touched

- `apps/web/components/editors/source-position.ts` (new local interface)
- `apps/web/components/task/markdown-source-position.ts` (new coordinator helper)
- `apps/web/components/task/markdown-source-position.test.ts` (new)
- `apps/web/components/task/markdown-file-editor.tsx`
- `apps/web/components/task/markdown-file-editor.test.tsx`
- `apps/web/components/task/markdown-preview-content.tsx`
- `apps/web/components/task/file-editor-content.tsx`
- `apps/web/components/task/file-viewer-content.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-markdown-viewer-body.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.test.tsx`
- `apps/web/components/editors/monaco/monaco-code-editor.tsx`
- `apps/web/components/editors/monaco/use-monaco-cursor-navigation.test.tsx`
- `apps/web/components/editors/codemirror/codemirror-code-editor.tsx`
- `apps/web/components/editors/codemirror/codemirror-code-editor.cursor.test.tsx`
- Both existing Markdown file-editing E2E files.

## Dependencies

Tasks 01 through 04 establish draft identity, recovery, and mode controls.
This work touches their coordinators and must follow them.

## Risks

The pinned engine's range-reveal operation returns a disposable and can defer
work until layout. Cancel it on exit. Public upstream selection types stay
inside the adapter. Restore only the active surface, not every retained view.

## Parallelism

`sequential`

## Inputs

- [Switch presentation](../../specs/ui/system-design/markdown-file-editing.md#switch-presentation).
- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- `apps/web/lib/markdown/source-line-ranges.ts`.
- Pinned engine selection and range-reveal APIs.

## Results

Implemented optional source-position handles for Monaco, CodeMirror, hybrid,
and Preview. The coordinator transfers source offsets, selection direction,
and nearest-line anchors. Identity, content, and transition guards prevent
hidden or retired surfaces from restoring focus. Pending hybrid reveals cancel
on replacement and unmount.

Provider and coordinator regressions pass in the 255-test targeted aggregate.
Desktop and phone browser transfers passed. Each selected a word below the
fold, retained its context through Preview, replaced the exact selection in
Source, saved exact bytes, and returned to Edit at the changed paragraph.
