---
spec: docs/specs/ui/requirements/review-markdown-preview.md
created: 2026-07-29
status: done
---

# Implementation Plan: Review Markdown Preview

## Overview

The shipped action currently closes Review and opens a file-editor or mobile viewer. Replace that
navigation with a row-local changed-content preview derived from the unified diff already present
in `ReviewFile`. First add a tested parser that extracts honest new-side Markdown fragments, then
wire the Review row and toolbar to toggle between the textual diff and the rendered fragments on
desktop, tablet, and mobile.

## Frontend

### Diff-content extraction

- Add a focused utility under `apps/web/components/review/` that parses the loaded unified diff
  into new-side Markdown fragments.
- Exclude diff headers, hunk headers, deleted lines, and newline markers. Treat complete
  added/untracked diffs as one document; keep modified-file hunks separate.
- Return whether the preview is complete or partial so the UI can label truncated or fragmented
  content without implying it is the full file.

### In-place Review rendering

- Keep preview state local to each `FileDiffSection` in
  `apps/web/components/review/review-diff-list.tsx`.
- Replace `renderDiffContent(...)` with a Review-specific Markdown preview component while that row
  is in preview mode. Reuse the existing sanitized Markdown rendering primitives rather than
  introducing another renderer.
- Change the Review toolbar contract from an external file-opening callback to an in-place toggle.
  Show `Preview markdown` only when the parser reports renderable new-side content, and show
  `Show diff` while preview mode is active.
- Remove the Review-only preview callback chain and dialog-close behavior from
  `review-dialog.tsx`, `review-dialog-surface.tsx`, and task-layout mounts. Preserve the independent
  file-editor Markdown preview feature.

### Mobile behavior

- Keep the existing 44 px `More actions` menu and replace its preview callback with the same
  row-local toggle used on desktop.
- Remove Review-specific mobile/tablet routing and stale-request logic that becomes unused once the
  action no longer fetches or navigates.

### Mobile design contract

- **Desktop outcome:** the eye action in the sticky review header replaces that row's diff with
  rendered changed content; `Show diff` restores it.
- **Mobile entry point:** the existing 44 px `More actions` button in the sticky review header.
- **Nearest exemplar:** the current mobile Review diff row, which already provides the full-width
  content surface, sticky identity header, contained actions menu, and dialog-owned scrolling.
- **Hierarchy and primary action:** Review remains the sole focal surface; preview changes only the
  selected row's body and retains its header, reviewed checkbox, and return action.
- **Presentation:** inline replacement fits a frequent comparison task and avoids an intermediate
  navigation layer or stacked overlay.
- **Scrolling and safe area:** `ReviewDiffList` remains the single vertical scroll owner. Rendered
  fragments flow within the row and do not introduce a nested viewport scroller.
- **Shared versus responsive state:** parsing, renderable-content detection, preview state, and
  Markdown rendering are shared; only the existing desktop icon/mobile menu presentation differs.

## Tests

- `apps/web/components/review/review-markdown-diff-preview.test.ts`: prove complete added-file
  extraction, separate modified hunks, deleted-line removal, marker removal, and partial metadata.
- `apps/web/components/review/review-diff-toolbar.test.tsx`: prove preview/show-diff toggles on
  desktop and mobile, and absence for non-Markdown or non-renderable diffs.
- Add focused rendered coverage for `FileDiffSection`: activating preview replaces the diff with
  sanitized Markdown fragments and restoring diff does not alter reviewed state.

## E2E Tests

- `apps/web/e2e/tests/review/review-markdown-preview.spec.ts`: create changed `.md` and `.mdx` files, open
  expanded Review, activate the desktop eye action, assert the dialog stays open and no file tab
  appears, then restore the diff.
- `apps/web/e2e/tests/review/mobile-review-markdown-preview.spec.ts`: create a changed `.md` or `.mdx` file,
  open Review from mobile Changes, choose the action from the file menu, and assert rendered
  changed content remains inside the Review dialog without navigating to Files.

## Implementation

- [x] [task-01-review-markdown-preview](task-01-review-markdown-preview.md) — done
- [x] [task-02-inline-diff-markdown-preview](task-02-inline-diff-markdown-preview.md) — done

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- --run \
  components/review/review-markdown-diff-preview.test.ts \
  components/review/review-diff-toolbar.test.tsx \
  components/review/review-diff-list-grouping.test.tsx
pnpm --dir web e2e:run tests/review/review-markdown-preview.spec.ts
pnpm --dir web e2e:run tests/review/mobile-review-markdown-preview.spec.ts -- --project=mobile-chrome
cd ..
make fmt
make typecheck test lint
```

## Risks

- Unified diffs are partial by design. Modified hunks must remain visibly separate so the preview
  never claims omitted lines are adjacent or reconstructs content it does not have.
- Added/untracked diffs may be truncated; completeness must account for `diff_skip_reason`.
- Removing the shipped navigation path must not remove the independent Markdown preview capability
  from file editors or mobile file viewers.
