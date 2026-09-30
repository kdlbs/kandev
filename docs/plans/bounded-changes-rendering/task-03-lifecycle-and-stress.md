---
id: "03-lifecycle-and-stress"
title: "Prove lifecycle and mobile behavior"
status: completed
wave: 3
depends_on: ["02-historical-window"]
plan: "plan.md"
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
acceptance_criteria:
  - AC-UI-BOUNDED-CHANGES-001.1
  - AC-UI-BOUNDED-CHANGES-001.2
  - AC-UI-BOUNDED-CHANGES-001.3
  - AC-UI-BOUNDED-CHANGES-001.4
  - AC-UI-BOUNDED-CHANGES-001.5
  - AC-UI-BOUNDED-CHANGES-001.6
  - AC-UI-BOUNDED-CHANGES-001.7
  - AC-UI-BOUNDED-CHANGES-001.8
  - AC-UI-BOUNDED-CHANGES-001.9
system_design:
  - ../../specs/ui/system-design/bounded-changes-rendering.md
---

# Task 03: Prove lifecycle and mobile behavior

## Summary

Complete focus, anchoring, and measurement behavior. Prove the whole panel's
bound with isolated browser stress fixtures and record the implementation results.

## In scope

- Logical keyboard traversal, bounded focus retention, and menu cancellation.
- Same-context scroll anchoring and task/environment retirement.
- Positive measurement, phone wrapping, hidden-panel recovery, and resize.
- Desktop/phone stress fixtures, small real-Git action coverage, and docs reconciliation.

## Out of scope

Do not change the user's affected worktree, introduce performance telemetry, or
run a broad unrelated verification suite.

## Acceptance

1. All plan scenarios pass with at most 120 total mounted descriptors. Repeated scrolling and collapse do not accumulate hidden rows.
2. Keyboard/menu focus, source identity, anchor restoration, and phone geometry survive updates, hiding, and task replacement.
3. All work-order results and spec statuses reflect actual checks. Documentation describes the final ownership and any observable interaction change.

## ASCII UI preview

UI-01/02/03 retain the [combined preview](plan.md#ascii-ui-preview).
The key lifecycle excerpt is:

```text
Desktop: Changes -> hide -> Changes  (same surviving anchor)
Phone:   Changes -> Chat -> Changes  (contiguous measured rows)
Commit:  v commit -> > commit        (children unmounted)
```

The focused row or open menu retains one owner. Removal moves focus to a
surviving row or section control. Covers all criteria, especially .5-.8.

## Verification

Write lifecycle regressions before each correction. Add stress browser cases
before tuning the renderer. Record failures rather than weakening the budget.

From the repository root:

```bash
cd apps/web
pnpm exec vitest run components/task/changes-timeline-interaction.test.tsx components/task/changes-timeline-measurement.test.ts components/task/changes-timeline-model.test.ts components/task/changes-timeline-viewport.test.tsx components/task/changes-timeline-selection.test.ts components/task/changes-inline-commit-state.test.ts components/task/changes-panel-focus.test.ts components/task/mobile/mobile-changes-panel.test.tsx
pnpm run typecheck
pnpm exec eslint components/task/changes-timeline-*.ts components/task/changes-timeline-*.tsx components/task/changes-inline-commit-state.ts e2e/tests/git/large-changes-helpers.ts e2e/tests/git/large-changes-virtualization.spec.ts e2e/tests/git/mobile-large-changes-virtualization.spec.ts
pnpm run i18n:check
pnpm run i18n:ratchet
pnpm e2e:run --project chromium tests/git/large-changes-virtualization.spec.ts tests/git/commit-file-navigation.spec.ts tests/git/git-changes-panel.spec.ts tests/changes-panel-multi-select.spec.ts tests/git/changes-panel-section-order.spec.ts tests/git/changes-panel-multi-repo-indent.spec.ts
pnpm e2e:run --project mobile-chrome tests/git/mobile-large-changes-virtualization.spec.ts tests/git/mobile-commit-file-navigation.spec.ts tests/task/mobile-changes-panel.spec.ts
cd ../..
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Also lint every pre-existing production file changed by Tasks 01-03 with
`pnpm exec eslint <exact changed paths>` from `apps/web`. Record the resolved
command, not the placeholder. If public docs change, run both public-doc
validators from the docs-maintainer skill.

Use the plan's E2E matrix. Test 393px and 767px phone widths, a 768px fine-pointer
boundary, and a coarse-pointer tablet. Use default mobile project devices.
Assert physical touch bounds, one scroll owner, and no document overflow.

Record an isolated production trace with extensions disabled. Report mounted
rows, DOM nodes, long tasks, and heap observations separately. Do not add flaky
elapsed-time or absolute-heap gates. Teardown only fixtures and instances owned
by this run. No performance claim relies on mocked virtualizer output.

## Files likely touched

- New `apps/web/components/task/changes-timeline-interaction.ts` and tests.
- New `apps/web/components/task/changes-timeline-measurement.ts` and tests.
- Timeline viewport/model and desktop/mobile composition from Tasks 01-02.
- New `apps/web/e2e/tests/git/large-changes-helpers.ts`.
- New `apps/web/e2e/tests/git/large-changes-virtualization.spec.ts`.
- New `apps/web/e2e/tests/git/mobile-large-changes-virtualization.spec.ts`.
- `apps/web/AGENTS.md`, this package, and paired requirement/design.
- `docs/public/sessions-and-review.md` if observable interaction wording changes.

## Risks

A DOM count scoped to one section can miss another unbounded collection.
Synthetic stress payloads need a real transport/action companion test.

## Dependencies

02-historical-window.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/bounded-changes-rendering.md).
- [System design](../../specs/ui/system-design/bounded-changes-rendering.md).
- [Plan](plan.md), including evidence, exclusions, and browser matrix.
- Existing Files virtualizer and Changes/commit tests named in this work order.

## Results

Completed on 2026-09-30.

RED:

- A model regression failed because provider-file descriptors did not retain the
  `pr-files-list` group expected by the mobile Changes surface. The model now
  gives provider rows one virtual list owner, with repository groups nested
  beneath it.
- The real-Git deep-tree browser test could open Changes before the task loaded
  the files created during that session. Reloading the task after the Git edits
  makes the status snapshot deterministic while retaining real file reads for
  the diff action.
- The desktop comparison-menu case initially lost its requested section focus
  to Radix close behavior. The viewport now applies external focus requests
  after two animation frames. Keyboard traversal reaches the first and last
  provider commits through their focusable toggles.

GREEN:

- `pnpm exec vitest run components/task/changes-timeline-interaction.test.tsx components/task/changes-timeline-measurement.test.ts components/task/changes-timeline-model.test.ts components/task/changes-timeline-viewport.test.tsx components/task/changes-timeline-selection.test.ts components/task/changes-inline-commit-state.test.ts components/task/changes-panel-focus.test.ts components/task/mobile/mobile-changes-panel.test.tsx`: 8 files and 54 tests passed.
- `pnpm run typecheck`: passed.
- Changed-file ESLint exited successfully with 31 warnings and no errors. The
  warnings identify function/file size, complexity, parameter count, nested
  ternaries, and duplicate-string thresholds in changed files; they are listed
  as a remaining maintainability limitation.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed. The ratchet reports
  zero added violations and 12 modified files clean.
- `pnpm e2e:run --project mobile-chrome tests/git/mobile-large-changes-virtualization.spec.ts tests/git/mobile-commit-file-navigation.spec.ts tests/task/mobile-changes-panel.spec.ts`: 11 tests passed without retries. This run built the production Vite bundle with the pseudo locale.
- `pnpm e2e:run --no-build --project chromium tests/git/large-changes-virtualization.spec.ts tests/git/commit-file-navigation.spec.ts tests/git/git-changes-panel.spec.ts tests/changes-panel-multi-select.spec.ts tests/git/changes-panel-section-order.spec.ts tests/git/changes-panel-multi-repo-indent.spec.ts`: 40 tests passed on the production bundle built by the mobile run.
- `pnpm e2e:run --no-build --project chromium tests/git/large-changes-virtualization.spec.ts`: 2 isolated 50,000-file stress tests passed and printed the following browser observations. `longTaskCount` is cumulative from observer installation; `heapUsedBytes` is the browser's coarse `performance.memory` sample.

| Layout | Checkpoint          | Mounted rows | DOM nodes | Heap bytes | Long tasks | Longest task |
| ------ | ------------------- | -----------: | --------: | ---------: | ---------: | -----------: |
| Tree   | Empty panel         |            0 |       681 | 72,200,000 |          7 |       222 ms |
| Tree   | 50k at end          |           39 |     2,074 | 72,200,000 |         10 |       518 ms |
| Tree   | After repeat scroll |           39 |     2,073 | 72,200,000 |         10 |       518 ms |
| Flat   | Empty panel         |            0 |       870 | 72,200,000 |          6 |       257 ms |
| Flat   | 50k at end          |           19 |     1,525 | 72,200,000 |          8 |       452 ms |
| Flat   | After repeat scroll |           19 |     1,524 | 72,200,000 |          8 |       452 ms |

The row count remained below 120 and DOM size did not accumulate during repeat
scrolls. The longest tasks occurred during synthetic inventory loading and
scrolling; this test does not claim frame-time bounds. Heap is an observation,
not a gate. Server payload size and full diff rendering remain outside scope.

- `python3 scripts/list-docs.py validate`: 331 decisions and 1,250 specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 47 published documentation pages validated.
- `git diff --check`: passed.

Resolved ESLint command from `apps/web`:

```bash
pnpm exec eslint \
  components/task/changes-file-tree-model.ts components/task/changes-file-tree-model.test.ts \
  components/task/changes-panel-body.tsx components/task/changes-panel-data.tsx \
  components/task/changes-panel-dialogs.test.tsx components/task/changes-panel-dialogs.tsx \
  components/task/changes-panel-file-row.test.tsx components/task/changes-panel-file-row.tsx \
  components/task/changes-panel-hooks.ts components/task/changes-panel-pr-files.tsx \
  components/task/changes-panel-repo-groups.tsx components/task/changes-panel-timeline-grouping.test.tsx \
  components/task/changes-panel-timeline.tsx components/task/changes-panel-tree.test.tsx \
  components/task/changes-panel-tree.tsx components/task/commit-row-files.tsx components/task/commit-row.tsx \
  components/task/changes-inline-commit-state.test.ts components/task/changes-inline-commit-state.ts \
  components/task/changes-timeline-history-row.tsx components/task/changes-timeline-interaction.test.ts \
  components/task/changes-timeline-interaction.test.tsx components/task/changes-timeline-interaction.ts \
  components/task/changes-timeline-measurement.test.ts components/task/changes-timeline-measurement.ts \
  components/task/changes-timeline-model.test.ts components/task/changes-timeline-model.ts \
  components/task/changes-timeline-selection.test.ts components/task/changes-timeline-selection.ts \
  components/task/changes-timeline-viewport.test.tsx components/task/changes-timeline-viewport.tsx \
  components/task/changes-timeline-working-tree.tsx \
  e2e/tests/git/commit-file-navigation.spec.ts e2e/tests/git/git-changes-panel.spec.ts \
  e2e/tests/git/mobile-commit-file-navigation.spec.ts e2e/tests/git/large-changes-helpers.ts \
  e2e/tests/git/large-changes-virtualization.spec.ts e2e/tests/git/mobile-large-changes-virtualization.spec.ts \
  e2e/tests/task/mobile-changes-panel.spec.ts
```

## Review remediation follow-up (2026-09-30)

All four review findings are addressed. Context changes reset working-tree and
history state, and retire inline detail requests. Bulk target grouping preserves
repository and path order with linear membership checks. Inline commit files
support keyboard activation with the complete commit target.

Verification:

- The focused Vitest run passed 11 files and 68 tests. It covered the working-tree
  and history owners, inline request state, selection grouping, row activation,
  the virtualizer, and the timeline model and interaction.
- `pnpm run typecheck` passed.
- `pnpm e2e:run --project chromium tests/git/commit-file-navigation.spec.ts -- --grep
"opens a collapsible commit detail"` rebuilt the production backend and Vite
  bundle. The desktop commit-file test passed 1/1 after ArrowDown navigation and
  Enter activation.
- `pnpm e2e:run --no-build --project mobile-chrome
tests/git/mobile-commit-file-navigation.spec.ts` passed 1/1 against that bundle.
- `pnpm run build:vite` passed. Vite reported the existing chunk-size and
  ineffective dynamic-import warnings.
- Targeted ESLint passed with 11 existing size and complexity warnings, and no
  errors. `pnpm exec prettier --check` and `git diff --check` passed.
- Public-doc validation passed: 62 validator tests and 47 pages.

## Final pre-PR validation (2026-09-30)

The final lint cleanup split timeline construction, history row rendering, and
panel state into focused helpers and modules. All review regressions and the
50,000-file bound remain covered.

- `pre-commit run web-lint --files $(git diff --cached --name-only)`: passed
  with zero warnings across changed web and E2E source files.
- `pnpm exec vitest run components/task/changes-file-tree-model.test.ts components/task/changes-inline-commit-state-owner.test.tsx components/task/changes-inline-commit-state.test.ts components/task/changes-panel-body-context.test.tsx components/task/changes-panel-dialogs.test.tsx components/task/changes-panel-file-row.test.tsx components/task/changes-panel-focus.test.ts components/task/changes-panel-hooks.test.ts components/task/changes-panel-pr-files.test.tsx components/task/changes-panel-timeline-grouping.test.tsx components/task/changes-panel-tree.test.tsx components/task/changes-timeline-history-row.test.tsx components/task/changes-timeline-interaction.test.ts components/task/changes-timeline-interaction.test.tsx components/task/changes-timeline-measurement.test.ts components/task/changes-timeline-model.test.ts components/task/changes-timeline-selection.test.ts components/task/changes-timeline-viewport.test.tsx components/task/changes-timeline-working-tree.test.tsx components/task/commit-row-files.test.tsx components/task/mobile/mobile-changes-panel.test.tsx`: 21 files and 124 tests passed.
- `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`:
  passed.
- `pnpm run build:vite`: passed. Vite reported the existing chunk-size and
  ineffective dynamic-import warnings.
- `pnpm e2e:run --project mobile-chrome tests/git/mobile-large-changes-virtualization.spec.ts tests/git/mobile-commit-file-navigation.spec.ts tests/task/mobile-changes-panel.spec.ts`: 11 tests passed, including the 50,000-file cases at 393px and 767px.
- `pnpm e2e:run --no-build --project chromium tests/git/large-changes-virtualization.spec.ts tests/git/commit-file-navigation.spec.ts tests/git/git-changes-panel.spec.ts tests/changes-panel-multi-select.spec.ts tests/git/changes-panel-section-order.spec.ts tests/git/changes-panel-multi-repo-indent.spec.ts`: 40 tests passed, including the two 50,000-file stress cases.
- `CAPTURE_PR_ASSETS=1 pnpm e2e:run --project chromium tests/git/commit-file-navigation.spec.ts -- --grep "opens a collapsible commit detail"` and `CAPTURE_PR_ASSETS=1 pnpm e2e:run --project mobile-chrome tests/git/mobile-commit-file-navigation.spec.ts`: 1 test passed on each device project; four desktop/phone screenshots were captured and manifest-checked.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, and `git diff --check`: passed; 331 decisions, 1,250 specifications, 62 public-doc tests, and 47 published pages validated.
