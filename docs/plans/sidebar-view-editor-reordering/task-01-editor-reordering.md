---
id: "01-editor-reordering"
title: "Implement compact editor reordering"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-VIEW-REORDER-001
acceptance_criteria:
  - AC-UI-SIDEBAR-VIEW-REORDER-001.1
  - AC-UI-SIDEBAR-VIEW-REORDER-001.2
  - AC-UI-SIDEBAR-VIEW-REORDER-001.3
  - AC-UI-SIDEBAR-VIEW-REORDER-001.4
  - AC-UI-SIDEBAR-VIEW-REORDER-001.5
  - AC-UI-SIDEBAR-VIEW-REORDER-001.6
  - AC-UI-SIDEBAR-VIEW-REORDER-001.7
  - AC-UI-SIDEBAR-VIEW-REORDER-001.8
system_design:
  - ../../specs/ui/system-design/sidebar-view-editor-reordering.md
---

# Task 01: Implement compact editor reordering

## Summary

Replace dedicated reorder arrows with sortable grips and compact explicit move menus.
Deliver the shared desktop/phone behavior and targeted regression evidence in one implementation pass.

## In scope

- Stable-ID Sort drag integration, arbitrary insertion, cancellation, context guards, and accessible announcements.
- Shared More menu, automatic-color arrow cleanup, and task-row menu integration.
- Existing selector/removal/visibility behavior and saved draft/settings paths.
- Localized copy, focused desktop/phone tests, and public how-to instructions.
- Update the current sort-chain system design's Editor section after implementation.

## Out of scope

Filter ordering, Group by options, editor section order, Threads direction buttons, task ranking, persistence changes, and new dependencies.

## Acceptance

- Desktop sort/color rows recover field width while grips and explicit menus move the correct complete item to the requested position.
- Phone and keyboard flows retain ordering, focus, input isolation, touch targets, containment, and save/reload behavior across all three editor lists.
- All assigned regression commands pass, with existing arrow-based scenarios migrated and existing assertions retained.

## ASCII UI preview

These excerpts use [UI-01, UI-02, and UI-03 from the full plan](plan.md#ascii-ui-preview), mapping to AC .1, .3, .4, .6, and .8.

```text
UI-01 Desktop:
[::] [Color v] [Red v] [Matching first v] [...] [X]

UI-02 Phone card:
| [::] Rule 2                    [...] [X] |
| [Color                                v]|
| [Red                                  v]|
| [Matching first                       v]|

UI-03 Details:
[::] Relative time                  [...] [On]

More menu: [Move up] [Move down]
```

Phone fields stack below a header of touch-sized actions. The existing drawer body remains the sole content scroller.
Desktop controls use 28px targets. Phone/coarse-pointer controls use at least 44px targets.
At one sort rule, reorder and removal are unavailable. Boundary menu actions are disabled.

## Verification

From the repository root, run these commands sequentially.
In a fresh worktree without dependencies, first run `(cd apps && pnpm install --frozen-lockfile)`.
Use `/tdd` and `/e2e`: run the new failing cases before production changes, then run the complete focused commands.

```bash
(cd apps/web && pnpm exec vitest run components/task/sidebar-filter/sort-chain-editor-model.test.ts components/task/sidebar-filter/sort-chain-editor.test.tsx components/task/sidebar-filter/sidebar-reorder-menu.test.tsx components/task/sidebar-filter/automatic-color-settings.test.tsx components/task/sidebar-filter/task-row-settings.test.tsx components/task/sidebar-filter/sidebar-filter-popover.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/sidebar-filter/)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-running-first-activity-sort.spec.ts tests/task/sidebar-automatic-colors.spec.ts tests/task/sidebar-filter.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-running-first-activity-sort.spec.ts tests/task/mobile-sidebar-automatic-colors.spec.ts tests/task/mobile-sidebar-views.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E rebuilds web/backend/fixtures. Keep runs sequential with the runner's worker limits.
Review captured desktop/phone renders against the previews and record command results and geometry evidence.

## Files likely touched

- `apps/web/components/task/sidebar-filter/sort-chain-editor.tsx`, `sort-chain-editor-model.ts`, `sort-chain-rule-card.tsx`.
- New `apps/web/components/task/sidebar-filter/sidebar-reorder-menu.tsx` and its test.
- `automatic-color-rule-card.tsx`, `automatic-color-rule-list.tsx`, `task-row-settings.tsx`, and `sidebar-filter-popover.tsx` in the same directory.
- Assigned component/model tests and the six E2E specs in the verification block, plus adjacent helpers if needed.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json` (use the existing Traditional Chinese/pseudo generators).
- `docs/public/tasks-and-workflows.md`: short how-to text for grip and explicit menu ordering.
- `docs/specs/ui/system-design/sidebar-running-first-activity-sort.md`: replace its arrow-layout description with the implemented interaction link.
- This plan, work order, and paired requirement/design lifecycle fields.

## Dependencies

None. Existing sort-chain and color-settings implementations are already present.

## Risks

Nonadjacent swaps, unstable rule identity, stale-view drops, focus loss, and touch conflicts with drawer scrolling or dismissal.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-view-editor-reordering.md), all acceptance criteria.
- [Design](../../specs/ui/system-design/sidebar-view-editor-reordering.md), all sections.
- Existing `AutomaticColorRuleList` and `SortableDetailRow` dnd implementations and their assigned tests.
- `/mobile-parity`, `/e2e`, `/tdd`, and `apps/web/AGENTS.md`.

## Results

Implemented stable-ID Sort dragging and shared More menus for Sort, automatic colors, and task-row details. Removed the redundant automatic-color arrows, added localized labels, updated the public how-to and linked system-design history, and added focused component plus desktop/mobile regression coverage.

- Focused Vitest: 6 files, 31 tests passed. Typecheck and scoped ESLint passed.
- `i18n:check` and `i18n:ratchet` passed across all seven shipped locales and pseudo locale. The check reported 435 pre-existing orphan catalog entries.
- Chromium E2E: 26/26 passed. Mobile Chrome E2E: 15/15 passed, including 390px fine-pointer and 900px coarse-pointer geometry, containment, and overflow checks.
- Desktop and phone captures were reviewed against UI-01/UI-02; task-row action geometry and ordering were verified by E2E.
- Public-doc tests: 62 passed; 47 pages validated. Specification catalog validation covered 359 decisions and 1,425 specifications; 36 spec-linter tests and the full spec lint passed.
- `git diff --check` passed. The implementation is tracked in PR #4295; the results below include its review follow-up.

### Code-review remediation

- Guarded each pointer/touch reorder with the visible list and scroll-region bounds. The detector keeps nearest-center movement for gaps and keyboard sorting and preserves initial bounds during autoscroll. At drop, each editor validates the latest pointer position against the current visible list bounds, maps rejected drops to cancellation, and announces cancellation.
- Marked each phone drag grip `data-vaul-no-drag`, preventing Vaul from handling the same downward gesture while retaining drawer scrolling and dismissal elsewhere.
- Added a collision-unit regression for a list that moves during a drag, plus mobile E2E coverage for releasing in the stale starting bounds. Existing mobile checks cover outside drops on Sort, automatic colors, and task-row details. The color case asserts no PATCH and unchanged saved settings. Each section also tests a downward touch reorder with a stationary, open drawer.
- Renamed the sidebar-specific scroll helper, corrected the Traditional Chinese translation for “item”, and guaranteed cleanup in the clipped-scroll unit test.
- Final verification: 32 focused component tests passed; typecheck, scoped ESLint, and `build:e2e` passed. Chromium E2E passed 26/26 and mobile Chrome E2E passed 15/15. `git diff --check` passed.
- Review-follow-up verification: 38 focused component tests passed; typecheck, scoped ESLint, `i18n:check`, `i18n:ratchet`, and `git diff --check` passed. Managed Chromium E2E passed 26/26 and managed mobile Chrome E2E passed 15/15, including the stale-bounds regression.

### CI E2E failure remediation

- The failed CI attempt reported nine retry verdicts across normal/container shards and three E2E shard jobs that had no runner or test steps. An exact no-retry replay of normal shard 12 on its CI merge commit passed 273 tests and reproduced the MCP subtask test failure: a persisted collapsed sidebar placed its resize divider over the New Task button. The test now sets that collapsed state deterministically, expands it before clicking, and restores the original layout.
- The Git refresh wait now matches the task session instead of any held request. Mobile file tests wait for the task environment to become ready before opening the Files panel. The repository-set test verifies the option's actual selection instead of hit-testing its pixel edges. The workflow touch helper can continue to the boundary for up to 40 gestures. The Docker source test uses the file-tree helper that reveals virtualized rows. The setup-recovery test waits for the seeded FAILED state after reload before checking its persisted controls.
- Managed E2E checks passed with retries disabled: all four targeted desktop cases passed; the collapsed-sidebar subtask case passed three consecutive runs; the mobile file/HTML cases passed once and then twice each; the touch-scroll case passed once and then three consecutive runs; and the Docker workspace-source case passed in host-mode managed E2E. The container-mode attempt stopped before test setup because this shared worktree's `.git` pointer targets a path outside the runner's mount.
- Final follow-up typecheck, ESLint on all changed E2E files, and `git diff --check` passed. The CI rerun and review disposition are tracked by PR #4295.

### CI rerun follow-up

- The exact-head run on `72a5735e1d2727ac7bb1b8e00e1c8d5adb57bf06` failed in E2E shard 7/14. One test failed on all three Playwright attempts; 239 tests passed and 3 were skipped. The report merge and required E2E aggregate failures were downstream of that shard.
- The failure was in an unchanged desktop session-continuity test. Its downloaded error context showed that the automatic continuation response was already visible after reload, while the test still required the transient retry card. The exact test passed once in the local managed runner before the correction, so that run alone did not reproduce the CI timing.
- The desktop and phone checks now accept either the still-pending retry card or the completed continuation at the reload and second-viewer checkpoints. They retain the initial retry-card assertion and final same-runtime, single-side-effect, and successful-continuation checks.
- After the correction, the focused desktop case passed 3/3 and the matching phone case passed 3/3 in managed E2E with retries disabled. Scoped ESLint, Prettier, and `git diff --check` passed.
- Main advanced to `a8bfce19fc299a4243af4c7be63c32e1a14dc7d5` during the CI rerun and caused import-only conflicts in the desktop and mobile sidebar running-rank E2E specs and their shared helper. The merge keeps both the drag-reorder coverage and main's task-wide running-rank coverage.
- After merging that base, the managed desktop and mobile sidebar running-rank specs both passed locally (2/2 each). Exact-head CI and its retry-summary audit are pending on the new merge commit for PR #4295.
