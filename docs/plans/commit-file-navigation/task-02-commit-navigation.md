---
id: "02-commit-navigation"
title: "Add commit file navigation and repository metadata"
status: completed
wave: 2
depends_on:
  - "01-shared-header"
plan: "plan.md"
requirements:
  - REQ-UI-COMMIT-FILE-NAV-001
acceptance_criteria:
  - AC-UI-COMMIT-FILE-NAV-001.1
  - AC-UI-COMMIT-FILE-NAV-001.2
  - AC-UI-COMMIT-FILE-NAV-001.3
  - AC-UI-COMMIT-FILE-NAV-001.4
  - AC-UI-COMMIT-FILE-NAV-001.5
  - AC-UI-COMMIT-FILE-NAV-001.6
  - AC-UI-COMMIT-FILE-NAV-001.7
  - AC-UI-COMMIT-FILE-NAV-001.8
  - AC-UI-COMMIT-FILE-NAV-001.9
system_design:
  - ../../specs/ui/system-design/commit-file-navigation.md
---

# Task 02: Add commit file navigation and repository metadata

## Summary

Use the shared header in both commit surfaces. Deliver the flat index, independent collapse, navigation, repository identity, and retained toolbar capabilities together.
Add desktop and phone coverage using actual commit data and renderers.

## In scope

- Shared loaded commit content, target-scoped state, section refs, index focus/scroll, and repository label.
- Independent index collapse, initially expanded, with a persistent visible file count and toggle.
- Commit toolbar reusing the existing diff buttons, with phone actions and source/provider capability checks.
- Existing loading/error/empty/binary behavior, all five locales, and phone scroll ownership.
- Component tests, real rendered E2E, and a short public how-to update.

## Out of scope

- Backend changes, stored collapse state, bulk operations, review state, and new worktree actions.

## Acceptance

- Both commit surfaces satisfy criteria .1 through .9 with complete file enumeration and independent transient state.
- Existing commit tools remain usable for eligible sources/renderers, and GitHub-only restrictions remain intact.
- New and affected tests pass, including phone geometry and source-routing compatibility.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [combined preview](plan.md#ascii-ui-preview):

```text
Desktop                         Phone full-height sheet
Message / SHA / repository      Commit Changes             Close
v Files (2) [toggle]             Message / SHA / repository
  src/a.ts +12 -3 [jump]         v Files (2) [toggle]
  logo.png  +0 -0 [jump]
                                  a.ts +12 -3 / src/ [tap]
v src/a.ts +12 -3 [tools]          logo.png +0 -0 [tap]
  diff body                     v a.ts               [...]
> logo.png +0 -0                   src/             +12 -3
                                  wrapped diff body
```

The phone sheet keeps a fixed close header and one content scroller. An index activation expands and focuses the destination.
Collapsed files retain headers. Patchless files retain placeholders when expanded. Empty commits omit the index.
These views cover AC-UI-COMMIT-FILE-NAV-001.1 through .9. Collapsing the index leaves `> Files (2)` above the unchanged diff bodies.
Add a panel component case and desktop/phone E2E assertions for index collapse, count visibility, keyboard activation, and independent file state.

## Verification

Run from the repository root after Task 01. Use TDD for the panel cases named in the plan.
The first panel RED must show missing navigation/collapse behavior, rather than a fixture or import failure.
Use real renderers for E2E. Inspect phone screenshots against UI-02 and record any structural difference.

```bash
(cd apps/web && pnpm exec vitest run components/task/commit-detail-panel.test.tsx components/task/commit-file-toolbar.test.tsx components/task/commit-detail-request.test.ts components/review/review-diff-header.test.tsx components/diff/diff-header-toolbar.test.tsx)
(cd apps/web && pnpm exec eslint components/task/commit-detail-panel.tsx components/task/commit-detail-content.tsx components/task/commit-file-toolbar.tsx components/diff/diff-header-toolbar.tsx components/task/mobile/mobile-diff-sheet.tsx e2e/tests/git/commit-file-navigation.spec.ts e2e/tests/git/mobile-commit-file-navigation.spec.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/git/commit-file-navigation.spec.ts tests/git/git-changes-panel.spec.ts tests/review/review-file-status.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-commit-file-navigation.spec.ts tests/task/mobile-changes-panel.spec.ts tests/review/mobile-review-file-status.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed runner rebuilds the frontend/backend and enforces worker limits. Do not overlap the E2E commands.
If implementation extracts another source/test file, include it in the corresponding targeted command before marking the task done.

## Files likely touched

- `apps/web/components/task/commit-detail-panel.tsx` and `commit-detail-panel.test.tsx`
- `apps/web/components/task/commit-detail-content.tsx` (new)
- `apps/web/components/task/commit-file-toolbar.tsx` and `commit-file-toolbar.test.tsx` (new)
- `apps/web/components/diff/diff-header-toolbar.tsx` and `diff-header-toolbar.test.tsx`
- `apps/web/components/task/mobile/mobile-diff-sheet.tsx`
- `apps/web/e2e/tests/git/commit-file-navigation.spec.ts` (new)
- `apps/web/e2e/tests/git/mobile-commit-file-navigation.spec.ts` (new)
- `apps/web/e2e/tests/git/commit-file-navigation-helpers.ts` (new shared fixture helpers)
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json` and generated pseudo catalog as needed
- `docs/public/sessions-and-review.md`

## Dependencies

Task 01. Existing commit data, `useTaskRepositories`, `useGlobalViewMode`, and Git/GitHub test fixtures need no API changes.

## Risks

- `hideHeader` hides native toolbar controls. Cover the replacement for both Pierre and Monaco before completion.
- GitHub-only targets must not gain local context expansion, edit, reset, revert, or review controls.
- Repeated navigation, concurrent panels, same-path commits, and missing metadata need explicit component cases.
- Index scroll geometry needs a tall fixture. Preserve safe areas, mobile close behavior, and existing sheet modes.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/commit-file-navigation.md)
- [System design](../../specs/ui/system-design/commit-file-navigation.md)
- [Test matrix](plan.md#tests) and [E2E matrix](plan.md#e2e-tests)
- Existing `git-changes-panel.spec.ts`, `mobile-changes-panel.spec.ts`, and `review-diff-header.test.tsx`
- `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`

## Results

Completed on 2026-09-17.

- Added shared desktop/mobile commit detail composition with a flat, sorted file index, independent index collapse, per-file collapse, target-scoped navigation, repository identity, and preserved loading/error/empty/binary states.
- Added the commit toolbar and retained renderer display tools after hiding native file headers. Review state and source restrictions remain in their existing adapters.
- Added navigation parameters to preview and pinned commit panel actions so repeated file selections update an existing panel.
- Added focused component coverage for the content and toolbar. The exact Task 02 Vitest command passed 5 files and 27 tests.
- Desktop commit, Changes, and review E2E passed after two existing detail-opening assertions were migrated to the explicit Open commit action. The final targeted rerun passed 2 tests, and the full desktop selection passed 27 tests after the fixes. Mobile commit, Changes, and review E2E passed 9 tests.
- Typecheck, touched-file ESLint, all i18n checks, Vite build, public-doc validation, specification validation, and whitespace checks passed.

Review remediation on 2026-09-17 closed the remaining navigation and renderer gaps. Remote commit toolbars no longer expose local worktree actions and omit unrelated local session context; GitHub headers show the selected `owner/repo`; legacy multi-repository targets without identity show a localized unavailable label; Monaco and Pierre expansion state are independent and correctly mapped; and navigation requests are consumed after one successful destination focus/scroll. The focused review suite passed 5 files and 27 tests, including remote capability, both-renderer folding, and one-shot navigation cases. The final desktop fine/coarse-pointer E2E passed 2 tests, and the final mobile E2E passed 1 test with long-path hierarchy, selected-file navigation, focus, touch, and overflow assertions.
