---
id: "02-frontend-link-and-indicator"
title: "Frontend automatic link and issue indicator"
status: done
wave: 2
depends_on: ["01-backend-reverse-lookup"]
plan: "plan.md"
spec: "../../specs/tasks/link-existing-task-github-issue.md"
---

# Task 02: Frontend Automatic Link And Issue Indicator

## Acceptance

- Creating a task from an issue invokes the existing task-issue link endpoint with that issue URL.
- Workspace links are inverted by owner/repo/number and rendered on matching issue rows.
- Single and multiple task indicators navigate through the existing task route behavior; unlinked issue rows show no indicator.

## Verification

`cd apps && rtk pnpm --filter @kandev/web test -- --run lib/api/domains/github-api.test.ts lib/state/slices/github/github-slice.test.ts hooks/domains/github/use-issue-key-to-tasks.test.ts components/github/my-github/quick-task-launcher.test.tsx components/github/my-github/task-row-indicator.test.tsx`

## Files Likely Touched

- `apps/web/lib/types/github.ts`
- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`
- `apps/web/lib/state/slices/github/types.ts`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/lib/state/slices/github/github-slice.test.ts`
- `apps/web/hooks/domains/github/use-issue-key-to-tasks.ts`
- `apps/web/hooks/domains/github/use-issue-key-to-tasks.test.ts`
- `apps/web/components/github/my-github/quick-task-launcher.tsx`
- `apps/web/components/github/my-github/quick-task-launcher.test.tsx`
- `apps/web/components/github/my-github/task-row-indicator.tsx`
- `apps/web/components/github/my-github/pr-row-task-indicator.tsx`
- `apps/web/components/github/my-github/issue-list.tsx`
- `apps/web/app/github/github-page-client.tsx`

## Output Contract

Report behavior, files changed, tests run, blockers, and mobile implications; mark this task done in this file and `plan.md`.
