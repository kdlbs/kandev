---
id: "02-toolbar-wiring"
title: "Toolbar action wiring"
status: done
wave: 2
depends_on: ["01-link-foundation"]
plan: "plan.md"
spec: "../../specs/ui/external-vcs-file-links.md"
---

# Task 02: Toolbar action wiring

## Acceptance

- Changes/commit diffs, Review diffs, desktop Monaco file editing/preview, and the mobile file viewer expose the shared action whenever their file context resolves.
- File status, previous path, repository ID/name, session, and explicit PR-source revision flow through without cross-repository or cross-branch fallback.
- Existing toolbar actions, desktop density, mobile 44px reachability, and file/diff scroll ownership remain intact.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/diff/diff-header-toolbar.test.tsx components/review/review-diff-toolbar.test.tsx components/editors/external-vcs-file-link.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/diff/file-diff-viewer.tsx`
- `apps/web/components/diff/diff-viewer.tsx`
- `apps/web/components/diff/use-diff-options.tsx`
- `apps/web/components/diff/diff-header-toolbar.tsx`
- `apps/web/components/diff/diff-header-toolbar.test.tsx`
- `apps/web/components/review/review-diff-list.tsx`
- `apps/web/components/review/review-diff-toolbar.tsx`
- `apps/web/components/review/review-diff-toolbar.test.tsx`
- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- Focused component tests adjacent to the editor/mobile surfaces if existing coverage does not exercise the new control.

## Dependencies

- `01-link-foundation` complete.

## Inputs

- Spec: cross-surface, status/path, new-tab, accessibility, and mobile scenarios.
- Plan: `Toolbar integrations` and `Mobile design contract`.
- Patterns: existing `ToolbarBtn`/`ToolbarIconBtn`, `FileActionsDropdown`, `PanelHeaderBarSplit`, `MobileFileViewerPanel`, and `ReviewFile` identity fields.

## Output contract

Report summary, files changed, tests/commands with results, blockers, visual/mobile risks, and any divergence. Update only this task file's `status` to `in_progress` at start and `done` after acceptance and verification pass; do not edit `plan.md`.
