---
id: "03-mobile-markdown-editing"
title: "Add Mobile Markdown Editing"
status: completed
wave: 3
depends_on: ["02-desktop-markdown-modes"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-002
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-002.3
  - AC-UI-MARKDOWN-FILE-EDITING-002.4
  - AC-UI-MARKDOWN-FILE-EDITING-002.5
  - AC-UI-MARKDOWN-FILE-EDITING-002.6
  - AC-UI-MARKDOWN-FILE-EDITING-002.8
  - AC-UI-MARKDOWN-FILE-EDITING-003.2
  - AC-UI-MARKDOWN-FILE-EDITING-003.3
  - AC-UI-MARKDOWN-FILE-EDITING-003.4
  - AC-UI-MARKDOWN-FILE-EDITING-003.5
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 03: Add Mobile Markdown Editing

## Summary

Extend the existing mobile file viewer into a focused Markdown editor. Reuse
the shared buffer and modes while giving phone layouts native controls,
keyboard clearance, and contained scrolling.

## In scope

- Preview, Edit, Source, Save, and Back actions in the mobile Files route.
- Shared hybrid Edit and editable CodeMirror Source.
- Dirty, save-error, remote-update, comment, and fallback outcomes.
- One scroll owner, safe areas, touch targets, keyboard clearance, and table
  containment.
- Focused mobile component tests.

## Out of scope

- Desktop dock layout changes.
- A general mobile editor rewrite for every file type.
- New backend save APIs.

## Acceptance

- Phone users can edit, save, preview, and access exact source through the
  existing Files entry point.
- Mobile uses the shared canonical buffer and matches desktop dirty, update,
  comment, error, and fallback outcomes.
- The focused surface keeps controls reachable and prevents document overflow
  with the virtual keyboard open.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- --run \
  components/task/mobile/mobile-file-viewer-panel.test.tsx \
  components/task/markdown-file-editor.test.tsx
pnpm --dir web run i18n:check
pnpm --filter @kandev/web typecheck
pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/editors/code-mirror/`
- `apps/web/components/editors/markdown/`
- `apps/web/src/locales/`

## Dependencies

Task 02 supplies the shared mode state and file coordinator.

## Risks

- The phone header can become crowded when dirty and error actions appear.
- The virtual keyboard can obscure the active line or fixed Save action.
- A nested editor scroller can break existing comment and table geometry.

## Parallelism

`sequential`

## Inputs

- Requirement sections `REQ-UI-MARKDOWN-FILE-EDITING-002` and `003`.
- System design section `Responsive interaction contract`.
- Existing mobile file viewer and mobile Markdown E2E patterns.
- `.agents/skills/mobile-parity/SKILL.md`.

## Results

- Added the mobile Preview, Edit, and Source coordinator with localized 44-pixel
  controls, safe-area spacing, contained scrolling, editable CodeMirror Source,
  hybrid Edit, fixed Save and Back actions, fallback handling, and VCS/update
  integration.
- Added canonical mobile draft, save, reload, and workspace-change
  reconciliation for clean and dirty buffers, including remote-update recovery.
- Added focused mobile component and hook tests for mode changes, exact source
  buffers, save state, MDX restrictions, hybrid fallback, remote refresh and
  reload, file identity, and mobile geometry contracts.
- Verification passed: focused mobile tests (39 tests), web typecheck, web
  lint, and i18n checks.
