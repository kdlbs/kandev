---
id: "03-inline-commit-files"
title: "Expand commit files inline in Changes"
status: completed
wave: 3
depends_on:
  - "02-commit-navigation"
plan: "plan.md"
requirements:
  - REQ-UI-COMMIT-FILE-NAV-001
acceptance_criteria:
  - AC-UI-COMMIT-FILE-NAV-001.7
  - AC-UI-COMMIT-FILE-NAV-001.8
  - AC-UI-COMMIT-FILE-NAV-001.10
  - AC-UI-COMMIT-FILE-NAV-001.11
  - AC-UI-COMMIT-FILE-NAV-001.12
  - AC-UI-COMMIT-FILE-NAV-001.13
system_design:
  - ../../specs/ui/system-design/commit-file-navigation.md
---

# Task 03: Expand commit files inline in Changes

## Summary

Make commit-row activation expand a read-only changed-file list using the saved tree/flat setting.
Give each row a separate Open commit action. File activation opens that commit at the selected historical diff.

## In scope

- Semantic sibling row controls, independent expansion, lazy loading, inline error/retry/empty states, and loaded-data reuse.
- Shared pure tree-building logic, directory toggles, live `changesPanelLayout` updates, file status/stats, and long-path handling.
- Optional file-navigation requests through desktop preview/pinned panel actions and phone sheet routing.
- TDD, source-aware regression coverage, existing commit-click test migration, and public instructions.

## Out of scope

- New endpoints, global detail caches, persisted expansion, inline patch engines, worktree file mutations, and new layout settings.
- Changing commit-list order, provenance, push/PR controls, or amend/revert/reset eligibility.

## Acceptance

- Row activation only toggles files; the visible Open commit action only opens details. Both work with keyboard and touch.
- Inline files use the existing layout preference and historical data. Selecting one reveals the correct file in the correct commit.
- Lazy requests, reuse, retries, empty/binary files, identity isolation, and existing review/source behavior pass the specified tests.

## ASCII UI preview

UI-03: Desktop Changes, tree preference, from the [combined preview](plan.md#ascii-ui-preview).

```text
COMMITS (6)                                  Push 6   PR
v d8816fc fix: preserve browser...  +76 -44 [Open commit]
    v src/
        browser.ts                 +60 -30
        browser.test.ts            +16 -14
> fd00686 fix: preview feedback...  +84 -13 [Open commit]
```

UI-04: Phone Changes, flat preference.

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

Both surfaces support both preferences. Phone tree mode uses nested directory toggles; desktop flat mode shows full paths.
Row expansion stays in the existing Changes scroll region. Open commit and file activation use the existing detail panel/sheet.
Phone controls have 44px targets. Keep the open action visible without hover. Do not nest interactive controls inside another button.
Loading, error with Retry, and No files occupy only the expanded region. The open action remains available in each state.
These views map to criteria .7, .8, and .10 through .13.

## TDD cases

- `commit-row.test.tsx`: `expands files without opening details`; `opens details without toggling files`; keyboard and pointer equivalents.
- `commit-row-files.test.tsx`: no request before expansion, single in-flight request, loaded reopen, retry, empty response, patchless entries, and target/session reset.
- Same file suite: flat/tree preference, independent directories, layout change without refetch, source/repository isolation, and absence of worktree controls.
- `changes-file-tree-model.test.ts`: shared hierarchy and sort behavior preserved for current Changes consumers and historical files.
- `dockview-panel-actions.test.ts`: navigation params survive opening and repeated selection of an existing preview or pinned commit without changing panel identity.
- `commit-detail-panel.test.tsx`: selected file expands/focuses after asynchronous loading; repeated tokens navigate again; missing paths retain full commit content.
- `mobile-changes-panel.test.tsx` (new if absent): target and selected-file navigation reach `MobileDiffSheet` independently of inline expansion.
- Extend `commit-file-navigation.spec.ts` and `mobile-commit-file-navigation.spec.ts` for UI-03/UI-04 and all new cases in the plan's E2E matrix.

The first row test must fail against the current behavior because activation calls the detail callback instead of expanding files.
Use real browser requests to prove lazy loading. Seed an older commit whose selected file differs from the current worktree.
Test saved flat and tree settings on desktop and phone, restoring the fixture's previous preference afterward.
Measure action/file bounds and touch targets with a long commit message and nested long paths. Assert file navigation reveals the historical marker.
Include two equal paths from different repositories and repeat file navigation after moving the destination out of view.

## Verification

Run from the repository root after Task 02. Record RED and GREEN results.

```bash
rg -n 'commit-row-|onOpenCommitDetail|addCommitDetailPanel' apps/web/e2e/tests apps/web/components/task apps/web/lib/state
(cd apps/web && pnpm exec vitest run components/task/commit-row.test.tsx components/task/commit-row-files.test.tsx components/task/changes-file-tree-model.test.ts components/task/changes-panel-tree.test.tsx components/task/changes-panel-timeline-grouping.test.tsx components/task/commit-detail-panel.test.tsx components/task/mobile/mobile-changes-panel.test.tsx lib/state/dockview-panel-actions.test.ts)
(cd apps/web && pnpm exec eslint components/task/commit-row.tsx components/task/commit-row-files.tsx components/task/changes-file-tree-model.ts components/task/changes-panel-tree.tsx components/task/changes-diff-target.ts components/task/changes-panel-repo-groups.tsx components/task/changes-panel-timeline.tsx components/task/changes-panel-data.tsx components/task/changes-panel.tsx components/task/changes-panel-body.tsx components/task/dockview-shared.tsx components/task/dockview-panel-content.tsx components/task/mobile/mobile-changes-panel.tsx components/task/mobile/mobile-diff-sheet.tsx lib/state/dockview-panel-actions.ts lib/state/dockview-store.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/git/commit-file-navigation.spec.ts tests/git/git-changes-panel.spec.ts tests/review/review-file-status.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-commit-file-navigation.spec.ts tests/task/mobile-changes-panel.spec.ts tests/review/mobile-review-file-status.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The initial search inventories old row-click assumptions. Update affected tests to use Open commit when they intend to open details.
Preserve their original assertions and add any additional affected specs to the exact commands before completion.
Run the E2E commands sequentially with fresh managed builds. Inspect phone screenshots against UI-04.

## Files likely touched

- `apps/web/components/task/commit-row.tsx` and `commit-row.test.tsx`
- `apps/web/components/task/commit-row-files.tsx` and `commit-row-files.test.tsx` (new)
- `apps/web/components/task/changes-file-tree-model.ts` and `changes-file-tree-model.test.ts` (new extraction)
- `apps/web/components/task/changes-panel-tree.tsx` and `changes-panel-tree.test.tsx`
- `apps/web/components/task/changes-panel-repo-groups.tsx`, `changes-panel-timeline.tsx`, and their existing tests
- `apps/web/components/task/changes-panel.tsx`, `changes-panel-data.tsx`, `changes-panel-body.tsx`, and `changes-diff-target.ts`
- `apps/web/components/task/dockview-shared.tsx` and `dockview-panel-content.tsx`
- `apps/web/lib/state/dockview-panel-actions.ts`, `dockview-panel-actions.test.ts`, and `dockview-store.ts`
- `apps/web/components/task/commit-detail-panel.tsx`, `commit-detail-content.tsx`, and panel tests from Task 02
- `apps/web/components/task/mobile/mobile-changes-panel.tsx`, `mobile-changes-panel.test.tsx`, and `mobile-diff-sheet.tsx`
- Both commit-navigation E2E specs/helpers from Task 02 and existing commit-opening tests identified by the inventory
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json` and generated pseudo catalog as needed
- `docs/public/sessions-and-review.md`

## Dependencies

Task 02 supplies the shared historical file navigation and expanded-body destination behavior.

## Risks

The detail endpoint also returns patches, so eager loading all rows would be expensive. Mount only after first expansion.
Avoid transferring stage/edit/discard actions from `ChangesTree`. Keep source identity separate from the selected path.
Desktop preview reuse must deliver repeated navigation requests. Nested click/key handlers must not toggle rows when an action runs.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/commit-file-navigation.md)
- [Inline design](../../specs/ui/system-design/commit-file-navigation.md#inline-commit-files-in-changes)
- [Full plan and coverage matrix](plan.md)
- `apps/web/hooks/domains/session/use-commit-detail.ts`, `apps/web/hooks/use-tree.ts`, and existing Changes fixture patterns
- `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`

## Results

Completed on 2026-09-17.

- Changed commit rows to use separate semantic controls: row expansion only toggles the lazy inline file list, while the visible Open commit action opens the detail surface.
- Added read-only flat/tree inline file rendering with lazy loading, cached reopen, retry and empty states, directory collapse, live layout preference handling, historical file activation, and target/session identity isolation.
- Reused the existing Changes tree model and threaded optional file navigation through desktop preview, pinned panels, and the mobile sheet.
- Migrated existing detail-opening E2E gestures to the explicit Open commit action without weakening their routing assertions.
- The exact Task 03 Vitest command passed 8 files and 66 tests. Desktop and mobile commit-navigation specs passed, as did the existing Changes and review compatibility suites.
- Typecheck, touched-file ESLint, all i18n checks, public-doc validation, specification validation, and whitespace checks passed. Updated `docs/public/sessions-and-review.md` with the new user flow.

Review remediation on 2026-09-17 preserves the original `FileInfo` in tree-mode rows, including nonzero, zero, missing, and renamed-file metadata. It also verifies filename-first mobile rows with complete accessible paths, separate Open actions, and 28px fine-pointer versus at least 44px phone/coarse-pointer controls. The final affected component suite passed 11 files and 85 tests; the final mobile E2E passed 1 test with long labels, inline historical-file activation, focus, touch hitboxes, and no document overflow.

Merged-base PR fixup validation on 2026-09-26 passed 13 affected Vitest files (104 tests), 29 desktop commit/Changes E2E tests, and 10 mobile commit/Changes E2E tests. Typecheck, i18n checks, public-doc validation (62 tests, 47 pages), and specification validation (309 decisions, 1,185 specifications) also passed.
