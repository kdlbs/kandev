---
created: 2026-10-13
status: implemented
requirements:
  - REQ-AGENTS-IMAGE-PREVIEW-DEFAULTS-001
system_design:
  - ../../specs/agents/system-design/image-preview-defaults.md
legacy_specs: []
---

# Implementation plan: Image preview defaults

## Overview

Make native rich-output image cards start expanded while preserving manual
disclosure and target isolation. One sequential work order delivers the default,
regression tests, desktop/phone browser evidence, and public guidance.

Confirmed intent: images should be expanded by default. Source establishes
the shared native card, existing classifier, retained disclosure state and
lazy loader. Scope is this rich-output file-card surface. No material question
blocks the narrow design.

Agents owns this package because it owns the rich-output contract shared by
task and Office consumers. Existing implementation packages linked by
`agent-rich-output.md` describe completed baselines, including file-target
isolation, json-render and retained view state. Their results remain historical;
this package changes the initial image disclosure rule only.

## Scope

In scope: image default, retained user choice, target/read regression checks,
desktop/mobile evidence and corresponding contract/public-doc reconciliation.
Out of scope: other preview surfaces, new supported formats, settings,
schema/backend changes, or a visual redesign.

## Technical approach

Use `getFileCategory(file.path) === "image"` as `FilePreview`'s initial
`useRichOutputViewState` value in `file-preview-block.tsx`. Keep the loader,
registry target key and host view store unchanged. Do not add an effect that
overwrites user collapse. Reconcile the parent lazy-only criteria and design
wording as part of implementation.

## ASCII UI preview

UI-01: Task/Office transcript, fresh image card. Current source starts collapsed:

```text
Current:  [Image] shot.png  [Preview] [Open file]
Desktop:  [Image] shot.png  [Hide preview] [Open file]
          +--------------------------------------+
          |             IMAGE                    |
          +--------------------------------------+
Phone:    [Image] shot.png
              [Hide preview] [Open file]
          +---------------------------+
          |           IMAGE           |
          +---------------------------+
Hidden:   [Image] shot.png  [Preview] [Open file]
Loading:  expanded region shows existing loading state
Error:    expanded region shows Preview unavailable.
```

Structure is required; spacing and copy are illustrative and use existing
localized labels. The phone action row applies below 420px; wider phones use
the shipped inline header. Images fit the card; transcript scrolling remains
primary. A hidden image stays hidden through retained remounts. Text cards keep
the current collapsed state. Maps to AC .1-.6 and both browser specs below.

## Tests

Extend `rich-output-renderer-file-targets.test.tsx` for AC .1/.2/.4/.5:
automatic v1/v2 image reads, non-image laziness, target replacement, stale
settlements, explicit retry and success caching. Extend
`rich-output-view-state.test.tsx` for AC .3: retained false through remount,
metadata and viewport changes, and duplicate-image independence.

## E2E tests

Add image/text payload scenarios beside existing cases in
`apps/web/e2e/tests/chat/rich-output.spec.ts` (chromium) and
`mobile-rich-output.spec.ts` (mobile-chrome). Use the shared helper's payload
override; do not enlarge the four-content-element baseline fixture. AC .1/.2/.3
cover decoded auto-preview, hide/reopen and text laziness; AC .4/.6 cover native
file-open routing and phone containment/touch targets. Unit tests own late-read
and retry timing, avoiding artificial browser transport races.

## Work orders

- [x] [Task 01: Expand image cards by default](task-01-expand-image-cards.md)

## Verification results

Implementation completed on 2026-10-13. The
[work order](task-01-expand-image-cards.md#results) records exact commands,
results, behavioral RED evidence, and local toolchain recovery.

- Targeted Vitest: 3 files / 32 tests passed after the final production edit.
- Typecheck (6 GiB Node heap), schema drift check, and focused ESLint passed.
- Desktop Playwright: 5 passed. Phone Playwright: 3 passed, including the final
  on-screen action assertions and reviewed expanded-image screenshot.
- Public documentation checks: 63 validator tests and 48 pages passed.
- Final specification catalog/lint, documentation coverage, and diff checks passed.

The final phone run used fresh backend/web builds and the existing verified
plugin archive after an unrelated corrupt Go-module cache blocked repackaging.
An earlier fresh managed phone run also passed all 3 scenarios. Sources and
runtime binaries were unchanged between those runs. Specs are active/current;
all AC .1-.6 outcomes match the implementation.

Design validation on 2026-10-13:

- `python3 scripts/list-docs.py validate`: passed (381 decisions, 1591 specs).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `.github/scripts/pr-docs.cjs` `validateCoverage`: prospective implementation
  coverage passed using the planned source path and the actual package files;
  requirement, AC, design and plan references resolve with no errors.
- `git diff --check`: passed; package status confirms the new plan and work order.

No production, permanent test or public-doc changes were made during design.

## PR integration follow-up

On 2026-10-14, the merged-tree targeted 32 tests and specification and harness
checks passed after preserving both overlapping planning links. Current-head
CI/review evidence remains external until checks finish; see the work order.

Claude's aggregate review cleanup note was addressed by settling the deferred
image read in `finally`, matching adjacent lifecycle tests. The same 32 focused
tests and targeted test-file ESLint passed. This is test cleanup only; the
product contract and rendered UI are unchanged. Remote validation is pending
for the remediation head.

The follow-up Claude review confirmed that repeated promise settlement is safe
and requested a clearer text-only target-replacement test name. That name now
distinguishes text collapse from image auto-expansion. The same 32 focused tests
and targeted ESLint passed after the rename; remote validation remains pending.

Claude's next review requested distinguishing an image target from its filename.
The image-replacement case now names the new image target, including repository
and session changes. The same 32 tests and targeted ESLint passed; remote checks
and incoming feedback remain external until terminal.

## Risks

- Mounted image cards now read their workspace files automatically, including
  historical cards. The four-content-element bound remains; unmounted items do
  not prefetch. Missing workspaces produce a local unavailable state.
- Existing SVG/text encoding behavior remains as shipped. This changes
  disclosure, not the supported rendering formats.
- A retained user collapse must take precedence over the default.
