---
id: "02-apply-default-selection"
title: "Apply default saved-query selection"
status: done
wave: 2
depends_on: ["01-persist-default-saved-queries"]
plan: "plan.md"
spec: "../../specs/ui/requirements/github-saved-query-defaults.md"
---

# Task 02: Apply Default Saved-Query Selection

## Acceptance

- Dashboard entry resolves the pull-request default after hydration, including
  query and repository, unless the user already interacted.
- Kind switches resolve that kind's saved default or first configured query.
- Save/delete/default mutations preserve the current-view rules in the spec.

## Files

- `apps/web/components/github/my-github/use-sidebar-selection.ts`
- `apps/web/components/github/my-github/use-sidebar-selection.test.ts`
- `apps/web/components/github/my-github/use-saved-preset-actions.ts`
- `apps/web/components/github/my-github/use-saved-preset-actions.test.ts`
- `apps/web/app/github/github-page-client.tsx`

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run components/github/my-github/use-sidebar-selection.test.ts components/github/my-github/use-saved-preset-actions.test.ts
```

## Result

- RED: initial hydration remained on the first built-in PR query; kind switching
  selected the first issue query; action tests showed duplicate store ownership,
  no interaction marker, and no default mutation.
- GREEN: focused command passed 10 tests across 2 files.
