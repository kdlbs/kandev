---
id: "01-rendered-ranges"
title: "Restore Markdown walkthrough indicators"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-002
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-002.16
  - AC-UI-MARKDOWN-FILE-EDITING-002.17
  - AC-UI-MARKDOWN-FILE-EDITING-002.18
  - AC-UI-MARKDOWN-FILE-EDITING-002.19
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
  - ../../specs/ui/system-design/markdown-walkthrough-ranges.md
---

# Task 01: Restore Markdown Walkthrough Indicators

## Summary

Connect active walkthrough ranges to rendered Markdown on desktop and phone.
Preserve the chosen mode, editor state, existing Source behavior, and
repository/session identity.

## In scope

- Resolve the active file-scoped target and connect Preview/Edit adapters.
- Paint the existing section indicator from source-mapped geometry. Publish
  the connector only from its visible owner. Recalculate after layout changes.
- Coordinate initial reveal with mode restoration. Acknowledge only the
  accepted walkthrough-owned pending request. Dispose every retained callback.
- Add targeted regression and native-browser tests. Update the uncommitted
  testing guide and public walkthrough guidance when behavior needs explanation.

## Out of scope

Generation changes, backend persistence, global editor-provider refactors,
new dependencies, source rebasing, commit/push, and main-instance operations.

## Acceptance

1. `.16` and `.17`: default Preview and retained Edit reveal and outline every
   matching active step. Same-file steps, repository changes, scrolling, reflow,
   and mode changes produce correct geometry with one active anchor owner.
2. `.18`: decoration and navigation preserve source, dirty state, selection,
   and history. Close/hide/replacement removes stale work. Newer search/LSP
   intents survive cleanup. Source never replays an acknowledged old jump.
3. `.19`: phone Files retains its composition and section indicator when the
   sheet occludes the anchor. Controls remain usable without horizontal overflow.

## UI preview

Use [UI-01 and UI-02 in the plan](plan.md#ui-preview). Desktop adds a blue
section outline within the document and uses the existing floating connector.
Phone adds the same outline within Files and retains the bottom sheet. A
covered target keeps its outline but has no connector. No toolbar or controls
change. Map rendered checks to `.16` through `.19`.

## Regression sequence

Write and run these tests before production changes. Record their expected
failure, then implement the smallest complete correction.

- Proposed `use-markdown-walkthrough-range.test.tsx`: default Preview and
  retained Edit receive the active matching target. Same-start/different-end
  steps update it. Wrong repository, closed walkthrough, and hidden instances
  cannot publish. Old cleanup cannot clear a replacement anchor.
- Proposed `use-hybrid-walkthrough-range.test.tsx`: inclusive LF/CRLF/BOM
  offsets, public engine geometry, empty geometry, reflow, cancellation, and
  one reveal per target. Extend real-engine adapter evidence to assert exact
  bytes, selection, and undo behavior before and after decoration.
- Proposed `use-markdown-preview-walkthrough-range.test.tsx`: smallest mapped
  blocks, nested lists/quotes, blank-line fallback, content/layout changes,
  active ownership, and scroller-relative bounds.
- `hooks/use-file-editors.test.tsx`: conditional acknowledgement cannot consume
  a newer search/LSP position or another repository/session request.
- Desktop/mobile Markdown E2E: enter through a stored walkthrough, navigate
  distant sections, switch modes, scroll, close, and verify no stale Source
  jump. Include a secondary repository with the same relative filename and a
  table/code section. Assert computed blue indicator styling and its bounds.

## Verification

Run from the repository root using pinned pnpm 9.15.9. Run browsers serially.
The first managed E2E command builds the current production artifact. Later
commands may reuse it only if production files have not changed. On the current
Nix host, retain the existing browser runtime wrapper and pinned-pnpm PATH
described in the browser-parity evidence. Never run against `:9998`.

```bash
corepack pnpm@9.15.9 --dir apps/web exec vitest run components/editors/markdown components/editors/use-rendered-walkthrough-range.test.tsx components/task/markdown-file-editor.test.tsx components/task/use-markdown-walkthrough-range.test.tsx components/task/use-markdown-preview-walkthrough-range.test.tsx components/task/use-markdown-preview-source-position.test.tsx components/task/mobile/mobile-file-viewer-panel.test.tsx components/task/mobile/session-mobile-layout.test.tsx components/task/dockview-review-dialog.delivery.test.tsx components/editors/monaco/use-monaco-walkthrough-range.test.ts components/editors/codemirror/use-codemirror-walkthrough-range.test.ts hooks/use-file-editors.test.tsx lib/walkthrough-editor-anchor.test.ts
corepack pnpm@9.15.9 --dir apps/web run typecheck
corepack pnpm@9.15.9 --dir apps/web run i18n:check
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --project chromium tests/task/markdown-file-editing.spec.ts tests/review/walkthrough.spec.ts
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --no-build --project markdown-firefox tests/task/markdown-file-editing.spec.ts
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --no-build --project markdown-webkit tests/task/markdown-file-editing.spec.ts
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --no-build --project mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts tests/review/mobile-walkthrough.spec.ts
corepack pnpm@9.15.9 --dir apps/web e2e:run --host --shards 1 --no-build --project markdown-mobile-webkit tests/task/mobile-markdown-file-editing.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/markdown-walkthrough-ranges TESTING-PR3113.md
```

Run changed-file ESLint and Prettier for the final file inventory. Record exact
commands and results here. Validate work-order requirement/design references
with the repository documentation-coverage helper. Record a desktop and phone
screenshot for the existing outline styling and occluded-connector behavior.

## Implementation files

- `apps/web/components/task/markdown-file-editor.tsx` and its test.
- `apps/web/components/task/use-markdown-walkthrough-range.ts` and test (new).
- `apps/web/components/task/markdown-preview-content.tsx`.
- `apps/web/components/task/use-markdown-preview-walkthrough-range.ts` and test (new).
- `apps/web/components/editors/markdown/hybrid-markdown-editor.tsx` and its test.
- `apps/web/components/editors/markdown/use-hybrid-walkthrough-range.ts` and test (new).
- `apps/web/components/task/mobile/mobile-markdown-viewer-body.tsx`,
  `session-mobile-layout.tsx`, `session-mobile-review-dialog.tsx`, and their tests.
- `apps/web/components/task/mobile/use-mobile-file-open-handlers.ts` (new).
- `apps/web/components/editors/use-rendered-walkthrough-range.ts` and test (new).
- `apps/web/hooks/file-editor-cursor.ts`, `apps/web/hooks/use-file-editors.test.tsx`, and `apps/web/lib/diff/walkthrough-reveal.ts`.
- `apps/web/lib/markdown/walkthrough-range.ts` (new). Existing Source range and
  anchor helpers retain their contracts.
- The desktop/mobile Markdown E2E files named above and
  `apps/web/e2e/helpers/markdown-walkthrough.ts` (new).
- `TESTING-PR3113.md` and `docs/public/sessions-and-review.md` if guidance changes.

## Dependencies

The existing uncommitted browser-parity implementation is the baseline. Preserve
its patches, input lifecycle, and source-position fixes. No separate task dependency.

## Risks

Hidden retained editors, async layout, and pending navigation can race. Match
identity at callback execution and keep geometry separate from editing state.

## Parallelism

`sequential`. This single vertical slice shares navigation and presentation state.

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [Range design](../../specs/ui/system-design/markdown-walkthrough-ranges.md).
- [Markdown design](../../specs/ui/system-design/markdown-file-editing.md).
- Existing Source hooks and `tests/review/walkthrough.spec.ts` fixtures.

## Results

Implementation and automated validation completed after the user's explicit request.

Behavioral RED evidence: the new tests exposed missing pending-request ownership,
missing Preview mappings, missing hidden-syntax geometry, mobile same-file dirty
Edit navigation entering discard/Preview, and incorrect nested-list parent
mapping. The old browser build also failed the desktop outline and phone
default-Preview regressions. All targeted unit regressions now pass.

The final unit command above passed 216 tests in 22 files. Typecheck, i18n,
production/unit ESLint, and Prettier passed. E2E ESLint passed with two existing
nested-ternary warnings. The worker command/result record is
`/tmp/markdown-walkthrough-unit-evidence.txt`; it is a summary, not raw stdout.

The final web build passed (`/tmp/kandev-run.build.xu1ZNxhw.log`).
Browser fixtures insert a real walkthrough row into the disposable E2E database
and use normal hydration. No application test hook or MCP permission change
was added. Desktop tests move the floating card with its drag handle before
mode changes. Phone tests close the sheet, change mode, and reopen the same
step. These actions preserve the existing card/sheet design.

Browser investigation evidence:

- Chromium full run: 23 of 24 passed, including all 10 existing Source
  walkthrough cases (`/tmp/kandev-run.e2e.LOO8wEY5.log`). The secondary-repository
  extension incorrectly assumed Edit survived replacement of an unpinned file
  preview before reload. The test now selects Edit explicitly before seeding
  and expands the primary tree after reload.
- Phone Chromium full run: all 11 passed, including three existing Source
  walkthrough cases (`/tmp/kandev-run.e2e.rWCvjNjA.log`).
- The live smoke found an incorrect bare primary path in our multi-repository
  seed. The guarded fixture repair added explicit repository names. No product
  routing change was needed. All 66 fixture hashes remained unchanged.
- Live visual inspection then found that Edit range geometry included the
  preceding block. Public engine selection rectangles include gaps; the adapter
  needs a strict source-block envelope. This finding requires another final
  browser pass after correction.

Final geometry correction: a new unit regression reproduced a section top of
40px when its source-intersecting blocks started at 100px. The adapter now clips
public selection rectangles to those measured blocks and retains complete table
bounds. The final 216-test run and typecheck passed. The focused Chromium
walkthrough/repository rerun passed both cases
(`/tmp/kandev-run.e2e.wY89wOBd.log`).

Final live evidence:

- The isolated direct-Tailscale instance at `http://100.101.37.39:48731`
  serves asset SHA-256
  `1ff00c00a345b50eedd6802de5d8f21f175ae26f1a81da1dbdd0fc043b340ab2`.
  The source manifest is
  `951dda36d0328cee9ff7591b1a62bf0b868578251fa13314f0e2bab8df4e6ced`.
- Guarded refresh retained the database, 66 fixture files, and fixture Git
  states. Main port 9998 retained PID 1155. Evaluation PID is 398656.
- Four seeded steps exercise early content, heading/table, below-fold content,
  and the secondary repository. Explicit repository names disambiguate README.
- Firefox 151 desktop and WebKit 26.5 phone passed 16 Preview/Edit checks,
  four same-file navigation checks, and four strict geometry checks. Native
  input values matched file source. Fresh hashes for all 66 files were unchanged.
- Strict geometry uses text-node DOM Range rectangles to exclude preceding
  text without treating editor wrapper padding as text. Desktop screenshots
  visibly confirm correct section bounds. The phone sheet obscures its
  screenshots; phone evidence is computed geometry and occlusion assertions,
  not a visual confirmation of the covered outline.
- Report: `/tmp/kandev-pr3113-evaluation-4e7mws9y/walkthrough-runtime-smoke.json`.
  Desktop screenshots: `walkthrough-firefox-desktop-1-edit.png` and
  `walkthrough-firefox-desktop-2-edit.png` in the same directory.

Physical Zen/macOS, Safari/macOS, phone keyboard, IME, and assistive-technology
acceptance remain in the browser-parity package. Linux native-engine automation
does not establish those results.

Final native desktop suites passed: Firefox 14/14
(`/tmp/kandev-run.e2e.8mbOtyGH.log`) and WebKit 14/14
(`/tmp/kandev-run.e2e.xfUZQobB.log`). Chromium has all 14 Markdown cases and all
10 existing Source cases covered by the full run plus the two-case final
walkthrough/repository rerun. Only the affected cases were repeated after the
geometry correction; unchanged compatibility results were retained.

The commands above used the existing host wrappers:
`python3 /tmp/pr3113-e2e-env.py python3 /tmp/pr3113-browser-runtime/env.py scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --no-build --project <project> <specs>`.
The explicit build used
`python3 /tmp/pr3113-e2e-env.py scripts/run-quiet build -- pnpm --dir apps/web build:e2e`.
All suites ran serially with one worker. Wrappers provide pinned pnpm and native
Nix browser libraries; they do not change browser behavior or application code.

Phone WebKit passed all eight Markdown cases on the final artifact
(`/tmp/kandev-run.e2e.OreGCyJW.log`).

The final phone Chromium outline rerun passed
(`/tmp/kandev-run.e2e.928BIdQ8.log`). Together with its full 11-case run, all
8 Markdown and 3 Source compatibility cases pass. Total coverage is 58 Markdown
browser/project cases and 13 Source walkthrough cases. No failed case remains
unresolved. The broader browser-parity native acceptance remains open.

Final status: completed. The existing branch, uncommitted application changes,
untracked testing guide, and main instance were preserved. No commit or push.
