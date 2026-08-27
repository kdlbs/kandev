---
id: "02-prove-command-palette-search"
title: "Prove command-palette search"
status: done
wave: 2
depends_on: ["01-correct-mixed-graph-search"]
plan: "plan.md"
spec: "../../specs/ui/requirements/task-workspace-content-search.md"
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
cd apps/web && pnpm e2e:run --no-build tests/review/submodule-review.spec.ts -- --grep 'cleans source repositories when setup fails' --retries=0
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
- Review remediation moved setup-failure cleanup into the shared fixture helper
  while preserving caller cleanup after successful setup. The focused regression
  failed RED with one leaked directory, then passed GREEN; the command-palette
  happy path also passed with retries disabled.
- PR CI remediation made workspace-picker workspaces disposable, aligned its
  focus assertion with Radix's initial item focus, and made office deletion
  accept any remaining kanban workspace. Both specs passed together (5 tests),
  then exact Chromium/mobile shard 4/14 passed 137 tests with 7 intentional
  skips and retries disabled.
- No production UI or mobile composition changed; both use the same corrected
  backend response.
