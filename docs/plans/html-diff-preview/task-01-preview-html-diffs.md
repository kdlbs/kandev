---
id: "01-preview-html-diffs"
title: "Add native HTML preview to diff rows"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-002
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-002.1
  - AC-UI-NATIVE-HTML-PREVIEW-002.2
  - AC-UI-NATIVE-HTML-PREVIEW-002.3
  - AC-UI-NATIVE-HTML-PREVIEW-002.4
  - AC-UI-NATIVE-HTML-PREVIEW-002.5
  - AC-UI-NATIVE-HTML-PREVIEW-002.6
  - AC-UI-NATIVE-HTML-PREVIEW-002.7
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---

# Task 01: Add native HTML preview to diff rows

## Summary

Expose the native HTML preview action in the shared Changes and Review file
header. Reuse the existing publisher for the complete workspace file and keep
an explicit return to the selected diff.

## In scope

- Eligibility, desktop eye action, phone actions-menu item, source/trust copy,
  and `Show diff` in the existing shared outer header.
- A focused workspace-read/publication hook and native row preview wrapper,
  including refresh, errors, retry, and stale-result retirement.
- A return-label option for `HtmlPreviewContent` with its existing default.
- Unit/component tests, desktop/mobile E2E, supported locale catalogs, and
  the public HTML-preview procedure.

## Out of scope

- Backend, iframe-policy, storage, dependency, and feature-flag changes.
- Rebuilding patch snapshots, editor buffers, dedicated commit/turn viewers,
  chat-inline diffs, or changes to Markdown preview semantics.

## Acceptance

1. UI-01 through UI-03 work in Changes and expanded Review for eligible HTML
   rows, on desktop and phone, with native scripts/assets and a preserved diff.
2. The captured workspace identity owns every read and publish. Failure, retry,
   binary content, dismissal, and context change satisfy the linked criteria
   without file writes or automatic review marking.
3. All commands below pass. Record actual results and rendered geometry, keep
   source-editor/Markdown regressions green, and update the public procedure.

## ASCII UI preview

UI-01: Desktop HTML row, from the
[full preview](plan.md#ascii-ui-preview):

```text
[ ] site/index.html  +517  [...display controls...] [Preview HTML] [edit]
    Preview HTML -> Current workspace file + native iframe + [Show diff]
```

UI-02: Phone focused diff surface:

```text
[ ] site/index.html                         [More actions]
    Menu: [Preview HTML] + source/trust copy
    Preview: source/trust copy + [Refresh] [Show diff] + iframe
```

UI-03: Shared states:

```text
Loading: [Show diff] + loading state
Error:   [Show diff] + localized recovery + [Retry HTML preview]
Deleted/binary/non-HTML: no HTML action
```

Map these views to `AC-UI-NATIVE-HTML-PREVIEW-002.1` through `.7`.
The header, source/trust copy, row replacement, and reachable return are
required. ASCII spacing and icon words are illustrative. Phone/coarse-pointer
actions have 44-pixel targets; desktop keeps compact file-toolbar controls.
The iframe owns page scrolling within the existing diff host and safe area.

## Verification

Run from the repository root. Install dependencies once in a fresh worktree
before the first package command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Use TDD for changed logic. Run these final commands after implementation:

```bash
(cd apps/web && pnpm exec vitest run components/review/use-review-html-preview.test.tsx components/review/review-diff-list.html-preview.test.tsx components/review/review-diff-toolbar.test.tsx components/review/review-diff-list-grouping.test.tsx components/review/review-markdown-diff-preview.test.ts components/task/html-preview-content.test.tsx components/task/file-editor-content.test.tsx)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/review/use-review-html-preview.ts components/review/review-html-diff-preview.tsx components/review/review-diff-list.tsx components/review/review-diff-header.tsx components/review/review-diff-toolbar.tsx components/task/html-preview-content.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/git/html-diff-preview.spec.ts tests/review/review-markdown-preview.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-html-diff-preview.spec.ts tests/review/mobile-review-markdown-preview.spec.ts --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner builds production assets. Run desktop and mobile
sequentially. Capture one desktop and one phone rendered state with `prCapture`
and compare them with the preview. Verify actual phone/coarse-pointer target
bounds, a narrow fine-pointer viewport, frame containment, and no document
overflow. Confirm test discovery before reporting counts. Do not add full
suites or generic QA/review gates.

Run the repository documentation-coverage preflight on the package and its
planned runtime boundary. This local call does not publish a GitHub status:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const order = 'docs/plans/html-diff-preview/task-01-preview-html-diffs.md';
const paths = [
  order,
  'docs/plans/html-diff-preview/plan.md',
  'docs/specs/ui/requirements/native-html-preview.md',
  'docs/specs/ui/system-design/native-html-preview.md',
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [
  { filename: order, status: 'added', additions: 1, changes: 1 },
  { filename: 'apps/web/components/review/review-diff-toolbar.tsx', status: 'modified' },
];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
NODE
```

## Files likely touched

- `apps/web/components/review/use-review-html-preview.ts` (new).
- `apps/web/components/review/review-html-diff-preview.tsx` (new).
- `apps/web/components/review/review-html-preview-actions.tsx` (new, desktop action
  and coarse-pointer touch disclosure).
- `apps/web/components/review/review-file-diff-body.tsx` (new, extracted row body).
- `apps/web/components/review/review-file-diff-section-parts.tsx` (new, extracted row state/header).
- `apps/web/components/review/review-diff-list.tsx`.
- `apps/web/components/review/review-diff-header.tsx`.
- `apps/web/components/review/review-diff-toolbar.tsx`.
- `apps/web/components/task/html-preview-content.tsx`.
- `apps/web/components/review/use-review-html-preview.test.tsx` (new).
- `apps/web/components/review/review-diff-list.html-preview.test.tsx` (new).
- `apps/web/components/review/review-diff-toolbar.test.tsx`.
- `apps/web/components/task/html-preview-content.test.tsx`.
- `apps/web/e2e/tests/git/html-diff-preview.spec.ts` (new).
- `apps/web/e2e/tests/git/mobile-html-diff-preview.spec.ts` (new).
- `apps/web/e2e/tests/git/html-diff-preview-helpers.ts` (new, if shared setup is needed).
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/review.json`.
- `docs/public/developer-tools.md`.
- This work order, its plan, and the paired native-preview specifications.

## Dependencies

None. Existing workspace-file reads, native publisher, session proxy, and
mobile menu already provide the runtime dependencies. Keep the original
`docs/plans/native-html-preview/` package as the completed historical delivery.

## Risks

- Preview source can differ from the patch. Keep that distinction explicit.
- In-flight reads need retirement before publication, not only after it.
- Preview geometry must not trigger review marking or hide the return action.
- Preserve the `HtmlPreviewContent` source-editor default and existing feedback
  context while adding the optional diff return label.

## Parallelism

`sequential`

## Inputs

- [Native HTML requirements](../../specs/ui/requirements/native-html-preview.md),
  `REQ-UI-NATIVE-HTML-PREVIEW-002` and its seven acceptance criteria.
- [Diff entry-point design](../../specs/ui/system-design/native-html-preview.md#diff-entry-point).
- [Full UI previews and E2E matrix](plan.md#ascii-ui-preview).
- `apps/web/AGENTS.md` and `apps/web/components/review/AGENTS.md`.
- Existing Review Markdown unit/E2E tests and native HTML editor unit/E2E tests.
- `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer` for implementation.

## Results

Implemented on 2026-11-10 for `AC-UI-NATIVE-HTML-PREVIEW-002.1` through `.7`.

- `ReviewDiffList` rows now expose `Preview HTML` from the shared
  `FileDiffToolbar`: a desktop eye action with source and trust help, and a
  phone actions-menu item with visible source and trust copy. Eligibility
  excludes deleted files, `diff_skip_reason === "binary"`, and non-HTML paths.
  Coarse pointers above the phone breakpoint keep the compact eye action, but
  it opens a touch drawer that shows the source and trusted-code copy and
  requires an explicit confirmation before workspace HTML executes. Fine
  pointers keep the immediate toggle with hover and keyboard help.
- `useReviewHtmlPreview` owns the workspace read and publication. It reads the
  complete file with `requestFileContent`, rejects binary responses, forwards
  the row's repository and path, and guards reads, publications, identity
  changes, dismissal, and unmount with request generations. Activation and
  return set the existing `suppressAutoMark` guard.
- `ReviewHtmlDiffPreview` renders the source note and bounded
  `min(70dvh, 36rem)` native frame through `HtmlPreviewContent`. The shared
  component gained optional `toggleLabel` (default `Show code`) and
  `errorMessage` options, so the file editor and mobile viewer defaults are
  unchanged.
- Returning to the diff preserves collapse, wrap, expansion, review status,
  and comments. Preview state is transient and never enters review persistence
  or browser storage.
- Catalogs were updated for `en`, `pt-pt`, `zh-cn`, `ja`, `ko`, and the
  generated `zh-hk`/`zh-tw` and pseudo locales. The public procedure now
  documents the diff entry point and current-workspace source.
- Focused unit/component tests, desktop and mobile E2E (including the
  coarse-pointer tablet disclosure), and the existing Markdown Review
  regressions pass. Actual verification commands and results are recorded in
  [the plan](plan.md#verification-results).
