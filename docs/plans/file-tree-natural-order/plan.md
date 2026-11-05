---
created: 2026-10-28
status: implemented
requirements:
  - REQ-UI-FILE-TREE-NATURAL-ORDER-001
system_design:
  - ../../specs/ui/system-design/file-tree-natural-order.md
legacy_specs: []
---

# Implementation Plan: File Tree Natural Order

## Overview

One sequential frontend slice changes the shared Files-tree comparator, proves
the behavior in focused utility and rendered desktop/phone tests, and updates
the public Files guidance. Begin with a causal failing regression before the
production change. Implementation requires a later separate execution request
and exclusive global HEAVY release after ROOT reads all four design artifacts.

## Scope

### In scope

- Numeric collation within directory and file groups with the existing lexical
  tie-break, for roots, nested rows, inserts, renames, and visible selection.
- Focused regression coverage and dedicated isolated desktop and phone E2E.
- One concise instruction beside the Files paragraph in
  `docs/public/developer-tools.md` (a task-oriented how-to guide).

### Out of scope

- File/path/content or backend mutations, hidden-file policy, search sorting,
  sort preferences, controls, storage, APIs, global CSS, and new UI copy.
- Parsing semantic versions, decimal values, or negative numbers.

## Technical approach

Update only the comparator in `apps/web/components/task/file-tree-utils.ts`:
keep directory precedence, use a default-locale `Intl.Collator` with
`numeric: true`, then `name.localeCompare` on numeric ties. The shared
comparator already drives `sortRootChildren`, `file-browser-hooks.ts`
`getChildren`, `insertNodeInTree`, `renameNodeInTree`, and `getVisiblePaths`.
Preserve their copy-before-sort behavior and original test fixtures/assertions.
The UI system owns presentation order; no filesystem or transport change is
needed. Runtime locale differences remain possible and a lexical comparison can
itself equate distinct strings; no stronger ordering is promised for those.

## ASCII UI preview

UI-01: Desktop Files panel, expanded folder (AC .1-.3):

```text
Files
  folder2/
    step2.md
    step10.md
  folder10/
  step2.md
  step10.md
```

UI-02: Phone Files bottom-navigation entry, expanded folder (AC .1, .4):

```text
Files
  folder2/
    step2.md      <- tap opens this file
    step10.md
  folder10/
[ existing bottom navigation ]
```

Order and hierarchy are required; spacing is illustrative. Desktop retains its
existing panel and range selection. Phone retains the focused Files surface,
internal scrolling, safe-area handling, and full-height read-only file viewer;
no composition or target-size change is introduced. Verify rendered rows and
phone containment in the two dedicated Playwright tests.

## Tests

Extend `apps/web/components/task/file-tree-utils.test.ts` without replacing
original fixtures, assertions, or their meaning. Cover root and nested file and
directory order, directory precedence, zero-padded ties under reversed input,
ordinary nonnumeric/dot/case names, input immutability, insertion and rename,
and DFS visible paths consistent with rendered order (AC .1-.3). Establish a
causal RED for numeric ordering before production changes; record the actual
assertion and exit.

## E2E tests

- `apps/web/e2e/tests/task/file-tree-natural-order.spec.ts`, Chromium: isolated
  real files and folders; assert root and nested rendered order and actual
  keyboard range selection against displayed rows (AC .1, .3).
- `apps/web/e2e/tests/task/mobile-file-tree-natural-order.spec.ts`, configured
  `mobile-chrome`: assert the same rendered order, tap a numbered file, verify
  its unique contents and no document horizontal overflow (AC .1, .4).
- One worker, zero retries, no inherited `file-tree-sorting.spec.ts` describe
  override, no live developer data or broad suite. Preserve synthetic originals,
  manifests and hashes externally before managed runs can clear assets.

## Work orders

- [x] [Task 01: Natural file order](task-01-natural-file-order.md)

## Verification results

Task 01 completed. Focused utility tests passed (17 tests); desktop Chromium and mobile-chrome E2E passed (one test each, one worker, zero retries). The work order records the RED and exact validation commands and results.

## Risks

- Numeric-collation equalities require the existing lexical tie-break, or server
  response order can determine zero-padded name order.
- Directly testing sorted data without rendered rows or keyboard selection can
  miss a consumer using a different order; dedicated E2E checks both surfaces.
- Shared docs and comparator have recorded open-PR overlaps. ROOT reviews actual
  overlapping patches before admission; refresh fully paginated open-PR
  inventory for all actual owned paths and actual main/source gap before
  publication. Preserve PR 1512's disjoint `mergeTreeNodes` addition and reassess
  if its head changes.
