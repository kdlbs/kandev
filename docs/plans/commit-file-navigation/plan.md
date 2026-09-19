---
created: 2026-09-16
status: completed
requirements:
  - REQ-UI-COMMIT-FILE-NAV-001
system_design:
  - ../../specs/ui/system-design/commit-file-navigation.md
legacy_specs: []
---

# Implementation plan: Commit file navigation

## Overview

Implement issue [3718](https://github.com/kdlbs/kandev/issues/3718) and the user's expanded Changes interaction in three sequential work orders.
First extract the shared header, then add commit-panel navigation, then deliver expandable commit rows and explicit detail actions.
The issue is assigned to `carlosflorencio`. All three work orders are implemented and verified.

## Evidence and scope

Source trace at `4c6e80091d` confirms the reported divergence:

- `CommitFileList` maps sorted files directly to `FileDiffViewer`, without collapse state or an index.
- Patchless entries bypass that renderer and show `task:binaryOrEmptyDiff`.
- `CommitHeader` accepts author/message/time and SHA, but no repository.
- Desktop `CommitDetailPanel` and phone `CommitDiffView` both use that file list.
- `ReviewDiffHeader` and `FileDiffSection` separately implement the review collapse controls.
- `useCommitDetail` already supplies file data for local and GitHub targets. No backend change is needed.

Smallest reproduction: open a commit with two changed files from Changes. There is no file-collapse button, index, or repository label.
Repeat in the phone commit sheet. This was verified through a read-only source trace, not a live browser run.
The issue has no image attachments or comments. Historical commit-count claims are not required for the diagnosis.
The user later supplied a Changes screenshot and requested inline commit-file expansion plus a separate action to open details.
`CommitRow` currently opens details on click/Enter/Space. `changesPanelLayout` already owns the Changes flat/tree preference.
`ChangesTree` mixes tree presentation with worktree mutations, so only its pure hierarchy logic is suitable for reuse here.

This is missing intended behavior, rather than a violation of an existing collapse requirement.
The new [requirement](../../specs/ui/requirements/commit-file-navigation.md) adds the missing presentation contract.
Existing merge and PR-only specifications retain data ownership. Their completed packages and the review-file-status package need no status changes.
No relevant ADR imposes a different header composition. Component reuse does not require a new ADR.

### In scope

- Shared header presentation, independent collapse, flat index, index navigation, and repository metadata.
- A collapsible panel index, expandable commit rows, lazy inline files using the existing layout preference, and an explicit Open commit action.
- Desktop/phone parity, accessibility, source restrictions, localization, and targeted regression coverage.

### Out of scope

- Backend or API changes, review-state features, persistence, bulk actions, virtualized lists, and provider expansion.
- Production and permanent test changes during this design turn.

## Technical approach

Use the [system design](../../specs/ui/system-design/commit-file-navigation.md).
Extract `ReviewDiffHeader` identity rendering into `components/diff/collapsible-file-header.tsx` with caller slots.
Retain review behavior in its adapter. Add `commit-detail-content.tsx` and `commit-file-toolbar.tsx` for shared commit composition and existing display tools.
Reuse the presentational controls in `diff-header-toolbar.tsx`; do not duplicate its toolbar or use review mutation callbacks.
Key commit state by session plus complete target identity. Use section refs for navigation and `useTaskRepositories` for repository count.
Task 03 adds lazy inline detail loading and read-only file-tree rendering. It threads optional path navigation through desktop and phone commit-opening adapters.
The panel index starts expanded and can collapse. Inline commit lists start collapsed; the user's existing layout preference controls their rendering.

## ASCII UI preview

UI-01: Desktop commit panel, opened with the Open commit action or an inline file.

```text
CURRENT                       PROPOSED
Message / author / SHA        Message / author / SHA / repository
file A + full diff            v Files (2) [toggle index]
file B + full diff              src/a.ts          +12 -3 [jump]
file C + full diff              assets/logo.png    +0 -0 [jump]
                             v src/a.ts +12 -3      [tools]
                               diff body
                             > assets/logo.png +0 -0
```

UI-02: Phone commit sheet, opened with the Open commit action or an inline file.

```text
+--------------------------------+
| Commit Changes          Close  | fixed sheet header
| Message / author / SHA          |
| Repository: app                |
| v Files (2)                    | tap to collapse index
| a.ts                   +12 -3  | tap row to expand/jump
| src/                           |
| logo.png                +0 -0  |
| assets/                        |
| v a.ts                   [...] | tap identity to collapse
|   src/                 +12 -3  |
|   wrapped diff body            |
| > logo.png               [...] | collapsed body
+--------------------------------+
```

The detail body has one vertical scroll owner. The index scrolls with the content.
Phone targets are at least 44px. Secondary tools use the existing mobile menu treatment.
Navigation expands its destination, focuses the toggle, and makes its header visible.
An expanded patchless file shows the existing placeholder. Empty commits show only the existing empty message.
Loading/error/retry retain current behavior. Geometry is illustrative; order, hierarchy, scroll ownership, and actions are required.
Both previews map to AC-UI-COMMIT-FILE-NAV-001.1 through .9.

Collapsed index state in UI-01 and UI-02:

```text
> Files (12)              [expand index]
v src/a.ts +12 -3         [tools]
  diff body remains visible
```

UI-03: Desktop Changes, saved layout = tree, first commit expanded.

```text
COMMITS (6)                                  Push 6   PR
v d8816fc fix: preserve browser...  +76 -44 [Open commit]
    v src/
        browser.ts                 +60 -30
        browser.test.ts            +16 -14
> fd00686 fix: preview feedback...  +84 -13 [Open commit]
```

With the flat preference, replace directory rows with full-path file rows.
Clicking a commit identity only toggles its list. Clicking a file opens that commit at the file's diff.
The open action remains visible without hover and does not toggle the inline region.

UI-04: Phone Changes, saved layout = flat.

```text
COMMITS (6)
v d8816fc fix: preserve...     [Open]
  +76 -44
    browser.ts                +60 -30
    src/
    browser.test.ts            +16 -14
    src/
> fd00686 fix: feedback...     [Open]
  +84 -13
```

With the tree preference, phone rows retain nested directory toggles. No extra sheet opens on row expansion.
Both layouts use one Changes scroller and 44px touch controls. Long filenames stay within their row.
An expanded row shows Loading files, a retryable error, or No files as applicable. Its Open commit action remains available.
UI-03 and UI-04 map to AC-UI-COMMIT-FILE-NAV-001.10 through .13, plus .7 and .8.

## Tests

All new behavior was implemented with focused component tests and real desktop/mobile E2E coverage.

| Test file and proposed case                                                                                               | Criteria      |
| ------------------------------------------------------------------------------------------------------------------------- | ------------- |
| `components/diff/collapsible-file-header.test.tsx`: controlled toggle, accessible path, caller slots                      | .1, .7, .8    |
| `components/review/review-diff-header.test.tsx`: preserve review checkbox, stale state, stats, path, actions              | .8            |
| `components/task/commit-detail-panel.test.tsx`: `lists all commit files and collapses only the selected file`             | .1, .3, .6    |
| Same panel suite: identity reset, unrelated rerender, repeat navigation, repository label, missing metadata               | .2, .4, .5    |
| Same panel suite: loading/error/empty and local/GitHub capability restrictions                                            | .6, .8        |
| `components/task/commit-file-toolbar.test.tsx`: retained tools and renderer/source capability matrix                      | .8            |
| `components/task/commit-detail-panel.test.tsx`: index toggle preserves file-body state and count                          | .9            |
| `components/task/commit-row.test.tsx`: row expands without navigation; separate action opens without expansion            | .10, .11      |
| `components/task/commit-row-files.test.tsx`: lazy request, flat/tree setting, directories, errors, source identity, reuse | .10, .12, .13 |
| Commit panel and mobile-target tests: repeated file navigation into existing target                                       | .12           |

The first new panel test fails today because the index and file-toggle controls are absent.
Assert accessible user behavior, not only new test IDs. Stub renderer bodies only in component tests; E2E must use real renderers.

## E2E tests

- Add `e2e/tests/git/commit-file-navigation.spec.ts` (`chromium`). Seed at least twelve paths, long paths, binary/patchless entries, and two commits with overlapping paths.
- Prove complete index/stats, independent collapse through scrolling, a repeated index jump, and target reset. Measure destination bounds inside the active panel.
- Include local multi-repository identity, single-repository behavior, GitHub-only details, and empty commit coverage. Use existing GitHelper and mock GitHub fixtures.
- Add `e2e/tests/git/mobile-commit-file-navigation.spec.ts` (`mobile-chrome`) with the same user outcomes through the full-height commit sheet.
- Use `.tap()`, assert 44px targets, accessible long paths, viewport containment, no document overflow, dismissal, and focus behavior.
- Cover phone width and 767/768px boundary behavior in focused geometry cases. Check a coarse-pointer tablet without changing stored display preferences.
- Exercise both supported diff renderers on desktop for collapse/navigation and retained tools. Restore any editor preference changed by fixtures.
- Preserve existing review-file-status, mobile-review-file-status, markdown-preview, external-link, and submodule suites as extraction guards.
- Run existing desktop/mobile Changes suites for commit-source and sheet compatibility. Use causal waits, fixture cleanup, and fresh managed builds.
- Task 02 proves the index can collapse while diff bodies remain accessible (.9).
- Task 03 extends both new navigation E2E specs for inline expansion, saved flat/tree layouts, directory collapse, and the separate open action (.10-.13).
- Assert no detail request before expansion, one request during loading, cached reopen, inline retry, and no new detail panel on row toggle.
- Test inline file selection against an older commit while the same path differs in the current worktree. Verify the historical marker and visible destination.
- Inventory all existing commit-row click/tap tests and migrate detail-opening actions to the explicit button. Preserve each test's original source/routing assertions.

## Work orders

- [x] [Task 01: Shared collapsible file header](task-01-shared-header.md)
- [x] [Task 02: Commit navigation and metadata](task-02-commit-navigation.md)
- [x] [Task 03: Inline commit files in Changes](task-03-inline-commit-files.md)

Task 02 depends on Task 01; Task 03 depends on Task 02. Execute in this session unless the user separately authorizes delegation.
Each work order contains the exact verification commands.

## Verification results

Design validation completed on 2026-09-16:

- `python3 scripts/list-docs.py validate`: passed, 281 decisions and 971 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed for tracked changes. New package files also received a separate whitespace check.

Implementation validation completed on 2026-09-17:

- Focused Vitest coverage passed: 13 files and 88 tests in the broad affected suite, including the exact Task 02 suite (5 files, 27 tests) and Task 03 suite (8 files, 66 tests).
- `pnpm run typecheck`, touched-file ESLint, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- `pnpm run build:vite` passed.
- Desktop commit, Changes, and review E2E passed after migrating two existing detail-opening assertions to the explicit Open commit action. The targeted final rerun passed 2 tests, and the complete initial desktop selection passed 27 tests after those fixes.
- Mobile commit, Changes, and review E2E passed: 9 tests. Desktop review extraction guards passed 6 tests, and mobile review extraction guards passed 3 tests.
- `node --test scripts/validate-public-docs.test.mjs` passed 62 tests; `node scripts/validate-public-docs.mjs` validated 46 pages.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` passed. Final `git diff --check` passed.

## Risks

- Header extraction can affect review sticky offsets, status metadata, and mobile actions. Preserve the adapter and run its existing guards.
- Hiding renderer headers can remove tools. The commit toolbar and both-renderer coverage are required parts of Task 02.
- A scroll test can pass for an off-screen DOM node. Assert bounds in the actual scroll owner with a tall fixture.
- Equal paths and SHAs across targets can leak collapse state. Include repository, source, workspace, owner, and session in identity as applicable.
- Missing legacy repository metadata must not produce a misleading primary-repository label.
- Inline lists use full detail payloads. Fetch only expanded rows and retain loaded data while the row stays mounted.
- Existing tests assume row activation opens details. Migrate those gestures without weakening their original assertions.

## Documentation impact

`docs/public/sessions-and-review.md` now documents the implemented commit index, transient collapse state, inline file browsing, and explicit Open commit action for desktop and phone.

## Review remediation

The implementation review on 2026-09-17 identified six gaps. All six are now addressed:

- Tree-mode inline files read statistics and rename metadata from the original `FileInfo` payload, so flat and tree presentations are equivalent.
- Commit detail toolbars no longer render local worktree actions. Remote commits keep their provider identity and do not receive the active local session as file context.
- Monaco folding derives the expand state from its global fold preference and inverts the setter. Pierre uses local per-view expansion state, and controls are gated by renderer and source capability.
- File navigation requests remain pending until the destination is ready, then are consumed once. A new token is required for repeat navigation.
- Phone commit rows, file rows, index entries, and file headers use filename-first hierarchy with the complete path available to assistive technology and tooltips. Commit statistics sit below the identity.
- Fine-pointer desktop controls remain 28px, while phone and coarse-pointer controls use at least 44px. The toolbar uses the touch overflow menu for coarse pointers.

GitHub repository display now uses `owner/repo` while provider-link resolution retains the linked repository identity. Multi-repository headers remain visible when author or message metadata is unavailable, and legacy targets without repository metadata show a localized unavailable label.

Review-specific validation passed: the affected component suite covered 11 files and 85 tests; the focused review suite covered 5 files and 27 tests; desktop commit-navigation E2E covered fine and coarse-pointer cases (2 tests); and the mobile commit-navigation E2E covered long labels, inline historical-file activation, focus, touch hitboxes, and document containment (1 test).
