---
id: "01-persist-default-saved-queries"
title: "Persist default saved queries"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/github-saved-query-defaults.md"
---

# Task 01: Persist Default Saved Queries

## Acceptance

- Legacy arrays normalize missing markers to `false` and duplicate defaults to
  the first valid entry per kind.
- Setting, replacing, and clearing one kind's default preserves the other kind.
- Workspace and user-setting writes publish new markers only after success;
  failed workspace writes retain prior state.

## TDD sequence

1. Add pure-model and hook regressions and confirm expected RED failures.
2. Add the normalized model and acknowledged persistence mutation.
3. Rerun focused suites and refactor while green.

## Files

- `apps/web/components/github/my-github/saved-preset-model.ts`
- `apps/web/components/github/my-github/saved-preset-model.test.ts`
- `apps/web/components/github/my-github/use-saved-presets.ts`
- `apps/web/components/github/my-github/use-saved-presets.test.ts`

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run components/github/my-github/saved-preset-model.test.ts components/github/my-github/use-saved-presets.test.ts
```

## Result

- RED: 2 normalization assertions failed because legacy and duplicate markers
  were not normalized; 6 model-export assertions and 4 persistence assertions
  then failed before their implementations existed.
- GREEN: focused command passed 21 tests across 2 files.
