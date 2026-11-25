---
created: 2026-11-10
status: complete
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-002
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
legacy_specs: []
---

# Implementation Plan: HTML preview from diffs

## Overview

Add `Preview HTML` to the file header shown in Changes and expanded Review.
One work order connects the action to the existing native HTML publisher and
proves the desktop and phone flows. The preview replaces the selected row's
body and provides `Show diff`.

The owning [requirement](../../specs/ui/requirements/native-html-preview.md)
extends the reusable native-preview interaction. Its
[design](../../specs/ui/system-design/native-html-preview.md#diff-entry-point)
keeps the existing backend and trusted-content boundary. Work order 01
implemented this package.

## Scope

### In scope

- All-file and single-file Changes diffs and the expanded Review dialog.
- Case-insensitive `.html` and `.htm` actions for non-deleted, non-binary files.
- Complete current-workspace content, repository scope, native relative assets,
  explicit refresh, retry, and return to the diff.
- Desktop, phone, coarse-pointer, keyboard, and localized source/trust copy.
- Focused tests and the existing HTML preview section in public developer docs.

### Out of scope

- Reconstructing HTML from patches or showing historical Git/turn snapshots.
- Dedicated commit-detail and historical-turn viewers, or chat-inline diffs.
- Using unsaved editor content for a workspace diff preview.
- Backend endpoints, persistence, dependencies, feature flags, or iframe policy
  changes. No new ADR is necessary for this existing native-preview entry point.
- Changes to Markdown preview, editor preview behavior, review persistence, or
  file mutation authority.

## Technical approach

### Shared diff boundary

The screenshot's header is `ReviewDiffHeader`, which uses `FileDiffToolbar`.
The inner `ReviewFileDiffContent` hides the Pierre/Monaco header. Add the action
to this shared outer header so both diff providers receive it. Do not add an
unused action only to `components/diff/diff-header-toolbar.tsx`.

`FileDiffSection` retains its current display controls and Markdown selection.
It uses a focused `useReviewHtmlPreview` hook and a small
`ReviewHtmlDiffPreview` body wrapper. Keep new logic outside the already large
`review-diff-list.tsx`; do not refactor unrelated list behavior.

### Workspace read and publication

Use `requestFileContent` in the new hook on activation, refresh, or retry.
Publish the full returned text through `useHtmlPreviewPublisher`. Preserve
`sessionId`, `repository_name`, and the current path for renamed entries.
Exclude known binary/deleted rows and reject binary read responses before any
publish. The preview source stays the current workspace file even when the
selected row represents staged, committed, or PR content.

Guard both asynchronous boundaries with identity and request generation.
Returning, identity change, and unmount retire pending work. A late file read
cannot publish, and a late publish cannot reopen a dismissed preview. Row
remounts start in diff mode. No activation occurs during render, list loading,
or browser-state restoration.

### Native preview and responsive behavior

Reuse `HtmlPreviewContent` and add an optional localized return label, defaulted
to `Show code`. Diff callers provide `review:showDiff`. Keep loading, read
errors, publish errors, source copy, refresh, retry, and return inside the
selected row. Reuse existing trust copy and add any new source copy to all
supported catalogs. Generate Traditional Chinese through `pnpm run i18n:zh-hant`.

The phone actions menu is the nearest shipped entry-point exemplar. The mobile
HTML viewer supplies preview controls and recovery. Keep the current diff
sheet/dialog as the focused surface, with the file header above the preview.
This frequent comparison action needs no additional drawer or navigation step.

`ReviewDiffList` keeps list scrolling; a frame sized to `min(70dvh, 36rem)`
contains the native document's own scrolling. Keep host safe-area behavior and
avoid document overflow. Desktop retains compact 24-pixel file actions. Phone
and coarse-pointer preview/return/refresh/retry controls have 44-pixel targets.
Verify phone width independently of pointer precision. Suppress automatic review
marking caused by preview transitions with the existing `suppressAutoMark` guard.

### Documentation

`docs/public/developer-tools.md` now explains the diff entry point, `Show diff`,
the current-workspace source, ineligible files, and failure recovery under
`Preview an HTML file`. The source-editor section continues to explain
unsaved-buffer preview. README and screenshot-catalog searches found no other
HTML-preview procedure requiring an update.

## ASCII UI preview

### UI-01: Desktop Changes or Review, HTML diff row

Current, verified from the shared toolbar:

```text
[ ] site/index.html  +517  [copy] [expand] [wrap] [split] [edit] [revert]
    textual diff
```

Proposed:

```text
[ ] site/index.html  +517  [copy] [expand] [wrap] [split] [eye] [edit] [revert]
    [eye]: Preview HTML
    Help: Uses the current workspace file. Runs trusted workspace code.
```

After activation:

```text
[ ] site/index.html  +517  [...existing controls...] [Show diff]
    Current workspace file. It can differ from this diff.
    [Refresh HTML preview] [Show diff]
    Existing trusted-code notice
    +-------------------------------------------------------+
    | Native preview iframe                                 |
    +-------------------------------------------------------+
```

### UI-02: Phone Changes diff sheet or expanded Review

```text
Focused diff surface
[ ] site/index.html                +517 [More actions]
    +-------------------------------------+
    | Preview HTML                        |
    | Current workspace file              |
    | Existing trusted-code notice        |
    | ...existing file actions...         |
    +-------------------------------------+

After Preview HTML:
[ ] site/index.html                +517 [More actions]
    Current workspace file
    [Refresh] [Show diff]
    Existing trusted-code notice
    +-------------------------------------+
    | Native preview iframe               |
    +-------------------------------------+
```

### UI-03: Shared loading, error, and ineligible states

```text
Loading:   file header [Show diff] | existing HTML loading state
Error:     file header [Show diff] | localized error [Retry HTML preview]
Ineligible deleted/binary/non-HTML row: existing actions; no Preview HTML
```

Structural requirements are the retained file header, explicit source and trust
copy, selected-row replacement, and reachable return action. Spacing and icon
text in these drawings are illustrative. Preserve existing control order and
use localized labels. Desktop help uses hover and keyboard focus; phone help
is visible in the menu. Frame scrolling stays inside the existing safe-area
host. UI-01 through UI-03 map to AC-UI-NATIVE-HTML-PREVIEW-002.1 through .7.

## Tests

- `components/review/use-review-html-preview.test.tsx`: full workspace content
  despite partial patch text; repo/renamed-path forwarding; no eager reads;
  binary rejection; read/publish failure; fresh retry/refresh; stale read and
  publish retirement on return, session/repo/path/source change, and unmount
  (AC-002.3, .5, .6, .7).
- `components/review/review-diff-toolbar.test.tsx`: desktop and mobile HTML
  actions, the coarse-pointer disclosure before activation, case-insensitive
  extensions, source/trust copy, and absent actions for unsupported rows
  (AC-002.1, .3, .4).
- `components/review/review-diff-list.html-preview.test.tsx`: row replacement,
  return with wrap/expansion intact, error/retry, unchanged review state and
  comments, repository isolation, and both diff providers (AC-002.2, .5, .7).
- `components/task/html-preview-content.test.tsx`: optional `Show diff` label
  and unchanged default `Show code`; `file-editor-content.test.tsx` preserves
  the existing unsaved-editor preview contract.
- Existing `review-diff-list-grouping.test.tsx` and
  `review-markdown-diff-preview.test.ts` preserve Markdown and grouping behavior.

The short AC-002 references above mean `AC-UI-NATIVE-HTML-PREVIEW-002.*`.
Write tests before production changes under `/tdd` and `/e2e`.

## E2E tests

- `tests/git/html-diff-preview.spec.ts`, project `chromium`: added-file preview
  from a single-file diff; staged content that differs from workspace content
  in the all-file view; expanded Review activation/return; scripts and relative
  CSS assets; publication failure and retry. Assert no file save, file tab, or
  automatic Browser panel (AC-002.1, .2, .3, .5, .6, .7).
- `tests/git/mobile-html-diff-preview.spec.ts`, project `mobile-chrome`: activate
  from the visible menu in the focused diff sheet and expanded Review; native
  page and return; 44-pixel menu/return targets, containment, safe-area clearance,
  and zero document overflow (AC-002.1 through .7).
- Add a narrow fine-pointer viewport scenario and a coarse-pointer tablet
  scenario in the desktop spec. Measure actual target bounds at phone width
  and verify the disclosure precedes activation above the phone breakpoint
  (AC-002.3, .4).
- Run the existing desktop/mobile Review Markdown E2E alongside the new tests.
  Keep the immutable seed checkout reset before and after new scenarios; use
  causal HTTP/WS waits and restore shared fixture mutations.

## Work orders

- [x] [Task 01: Add native HTML preview to diff rows](task-01-preview-html-diffs.md).

Dependency order: Task 01 is one sequential end-to-end slice. No delegation is
authorized. Exact commands, including the documentation-coverage preflight,
are in [Task 01 verification](task-01-preview-html-diffs.md#verification).

## Verification results

Design-package checks passed on 2026-11-10:

- `python3 scripts/list-docs.py validate`: 446 decisions and 1,963 specifications
  validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Task 01's local `validateCoverage` preflight: `covered`, no errors; the plan,
  work order, requirement IDs, acceptance IDs, and design references resolve.
- `git diff --check`: passed.
- The catalog discovers both native HTML preview documents. The original
  native-preview plan remains `complete` and owns only requirement 001.

Implementation checks passed on 2026-11-10:

- Focused Vitest suite (7 files, 63 tests): hook activation, complete workspace
  reads, repository forwarding, binary rejection, read/publish errors, retry,
  refresh, stale read and publication retirement, desktop/phone actions,
  case-insensitive extensions, source/trust copy, row body replacement, return
  with display controls, review-state preservation, and shared preview defaults.
  A broader Review/Changes regression run (31 files, 296 tests) also passed.
- `pnpm run typecheck` passed.
- `pnpm exec oxlint -c oxlint.config.mjs --deny-warnings` passed for every
  changed review/task component and both new E2E specs.
- `pnpm run i18n:check` passed; `pnpm run i18n:ratchet` reported 0 added and 5
  modified files clean. Catalogs hold the two new `review` keys in all required
  locales.
- Chromium `tests/git/html-diff-preview.spec.ts` (6 tests): pass. Covers a
  single-file diff preview, staged-patch versus workspace source, publication
  failure and retry, expanded Review, the 760-pixel fine-pointer viewport, and
  a 1024-pixel coarse-pointer tablet where the source/trust drawer must be read
  and confirmed before activation. Rendered screenshots were captured through
  `prCapture`.
- mobile-chrome `tests/git/mobile-html-diff-preview.spec.ts` (2 tests): pass.
  Covers the focused diff sheet and expanded Review, native script/CSS assets,
  source/trust copy, 44-pixel menu/refresh/return targets, containment, and no
  document horizontal overflow. The desktop narrow-viewport scenario also
  verifies 44-pixel retry and return targets after a failed publication. A
  rendered mobile screenshot was captured through `prCapture`.
- Existing `tests/review/review-markdown-preview.spec.ts` and
  `tests/review/mobile-review-markdown-preview.spec.ts`: pass.
- `node --test scripts/validate-public-docs.test.mjs` (63 tests) and
  `node scripts/validate-public-docs.mjs` (55 pages): pass.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`
  (36 tests), `python3 scripts/lint-spec-files.py --all`, and `git diff --check`:
  pass.
- Task 01's local `validateCoverage` preflight with the implemented runtime
  boundary (`review-diff-toolbar.tsx`, `use-review-html-preview.ts`): `covered`,
  no errors.

## Risks

- Current workspace content can differ from staged, committed, or PR patches.
  Keep the source copy visible before and after activation.
- A workspace read can finish after the row changes or closes. Guard the read
  before publication as well as the publish result.
- A bounded iframe introduces document scrolling inside a scrollable diff
  list. Verify the phone controls and frame remain reachable.
- Shared native-preview label changes must preserve source-editor defaults.
