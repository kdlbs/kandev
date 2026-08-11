---
id: "02-prove-command-palette-search"
title: "Prove command-palette search"
status: done
wave: 2
depends_on: ["01-correct-mixed-graph-search"]
plan: "plan.md"
spec: "../../specs/ui/task-workspace-content-search.md"
---

# Task 02: Prove Command-Palette Search

## Acceptance

- Cmd/Ctrl+Shift+K shows matching files from the unnamed Git root, direct
  submodule, and nested submodule in their existing repository groups.
- The test drives the user-facing palette, reuses the established disposable
  submodule fixture, and cleans its source tree even after failure.
- No production UI or mobile composition changes are introduced.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm e2e:run tests/command-panel.spec.ts -- --grep 'finds root and submodule files'
```

## Files likely touched

- `apps/web/e2e/tests/command-panel.spec.ts`

## Dependencies

Task 01.

## Parallelism

`sequential` — browser proof requires the corrected backend aggregation.

## Inputs

- Spec scenario for Files search across a Git root and initialized submodules.
- `apps/web/e2e/tests/review/submodule-review-helpers.ts` for disposable Git
  graph creation, task startup, worktree readiness, and cleanup.
- Existing `openFileSearch` and command-dialog helpers in `command-panel.spec.ts`.
- Mobile-parity exception for shared data-only changes with no rendered,
  touch, navigation, scrolling, or breakpoint change.

## Output contract

Report the discovered Playwright project/test count, exact E2E command/result,
group assertions, disposable fixture cleanup, changed files, blockers/risks,
and synchronized task/plan status.

## Results

- `pnpm install --frozen-lockfile` completed successfully from `apps/`.
- RED: with the original file-search predicate restored in the built backend,
  `pnpm e2e:run --no-build tests/command-panel.spec.ts -- --grep 'finds root and submodule files'`
  discovered one Chromium test and failed with 2 repository groups instead of
  3; the unnamed root group was missing.
- GREEN: after restoring the fix and rebuilding,
  `pnpm e2e:run tests/command-panel.spec.ts -- --grep 'finds root and submodule files'`
  passed 1 Chromium test. It asserted the unnamed root, `vendor/outer`, and
  `vendor/outer/vendor/inner` groups.
- The established fixture cleans its disposable source tree in `finally`.
- No production UI or mobile composition changed; both use the same corrected
  backend response.
