---
id: "02-comment-identity"
title: "Preserve comment repository identity"
status: completed
wave: 2
depends_on: ["01-shared-draft"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-002
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-002.6
  - AC-UI-MARKDOWN-FILE-EDITING-002.13
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 02: Preserve comment repository identity

## Summary

Carry the selected repository through hybrid comment reads and writes. Match the existing Preview and Source behavior.

## In scope

- Pass the repository name through desktop and phone callers into the bridge.
- Use the fourth selector argument and the existing comment repository-name field.
- Resolve workspace-prefixed file opens into repository and repository-relative
  path using the known checkout roots. Keep the original workspace read request.
- Carry that qualifier through both Dockview Files panel callbacks.
- Preserve legacy unqualified-comment behavior and source-line mapping.
- Connect editable hybrid selection to the existing Kandev comment popover.
  The pinned upstream comment input is available only in read-only mode.
  Keep the editor editable. Capture the selected source range before opening
  the popover, and keep its textarea outside the EditContext host.

## Out of scope

- Changing comment storage formats or backend APIs.
- Changing source selection or the shared draft model.

## Acceptance

- A failing test with two repositories containing README.md passes after the fix. New secondary-repository comments keep the selected name.
- Qualified comments never appear in another repository. Existing primary and legacy-comment tests still pass.
- The rendered desktop flow creates a secondary-repository comment and preserves the qualifier when it reaches pending comments.
- Real keyboard selection opens the shared comment popover. Comment creation
  leaves source bytes unchanged and does not change the editor mode.

## Verification

Add the named regression scenarios before the correction. Run the focused test to establish the expected failure, then run this complete block after the fix. Commands start at the repository root.

```bash
(cd apps/web && pnpm exec vitest run components/task/markdown-file-editor.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx hooks/domains/comments/use-diff-comments.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project=chromium tests/task/markdown-file-editing.spec.ts --grep "review regression: repository comments")
```

## Files likely touched

- `apps/web/components/task/markdown-editor-comment-bridge.ts`
- `apps/web/components/task/file-browser-actions.ts`
- `apps/web/components/task/file-browser-data.ts`
- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/dockview-shared.tsx`
- `apps/web/components/task/dockview-panel-content.tsx`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx`
- `apps/web/components/editors/markdown/hybrid-markdown-editor.test.tsx`
- `apps/web/components/task/markdown-file-editor.tsx`
- `apps/web/components/task/markdown-file-editor.test.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`
- `apps/web/hooks/domains/comments/use-diff-comments.test.ts`
- `apps/web/e2e/tests/task/markdown-file-editing.spec.ts`

## Dependencies

Task 01-shared-draft for integrated acceptance. Adapter, bridge, and desktop
work can start independently. Mobile changes wait for Task 01's file-ownership
handoff. The primary session owns browser test files.

## Risks

Existing records without a repository name remain compatible. Do not attach a secondary repository to the primary repository ID.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [System design](../../specs/ui/system-design/markdown-file-editing.md).
- [Review evidence and UI composition](plan.md).

## Results

Implementation and acceptance verification passed. Browser testing exposed a second
identity boundary: the file browser passed a workspace-prefixed path without
its repository qualifier. The repair normalizes the emitted tab from known
checkout roots. It preserves the original workspace read request.
Both Dockview Files panel callbacks also preserve the qualifier when opening
the canonical draft.

The comment popover rejects stale selections after source, file, session,
visibility, or instance changes. Its trigger stays within the visible viewport.
Accepted submissions return focus to the active editor. Comment and adapter
regressions passed before the file-open integration correction.

The repository-opening tests passed 51 cases. Both Dockview caller regressions
passed in their five-test suite. The real desktop scenario passed: creating a
comment in a secondary README preserves the qualifier, leaves source unchanged,
hides the comment in the primary README, and restores it in the secondary file.
