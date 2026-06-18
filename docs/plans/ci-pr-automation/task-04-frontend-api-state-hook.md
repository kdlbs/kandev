---
id: "04-frontend-api-state-hook"
title: "Frontend API state hook"
status: done
wave: 3
depends_on: ["02-backend-ci-options-api"]
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 04: Frontend API State Hook

## Acceptance

- Frontend GitHub types and API functions cover CI automation options and patch payloads.
- GitHub store slice keeps task-keyed options with loading/saving/error state.
- `useTaskCIOptions` loads, updates, and resets task CI automation options.

## Verification

```bash
cd apps && rtk pnpm --filter @kandev/web test -- github-api
cd apps && rtk pnpm --filter @kandev/web test -- use-task-ci-options
cd apps && rtk pnpm --filter @kandev/web typecheck
```

## Files Likely Touched

- `apps/web/lib/types/github.ts`
- `apps/web/lib/api/domains/github-api.ts`
- `apps/web/lib/api/domains/github-api.test.ts`
- `apps/web/lib/state/slices/github/types.ts`
- `apps/web/lib/state/slices/github/github-slice.ts`
- `apps/web/lib/state/slices/github/github-slice.test.ts`
- `apps/web/hooks/domains/github/use-task-ci-options.ts`
- `apps/web/hooks/domains/github/use-task-ci-options.test.tsx`
- `apps/web/lib/ws/handlers/github.ts` if websocket updates are implemented

## Dependencies

- `02-backend-ci-options-api`

## Inputs

- Spec sections: API surface, Persistence guarantees.
- Plan sections: Frontend > Types and API client; Frontend > State and hook.
- Existing patterns: `useTaskPR`, `usePRCIPopover`, and GitHub Zustand slice actions.

## Output Contract

When complete, update this file's `status` to `done`, update the Wave 3 checkbox in `plan.md`, and report changed files, tests run, blockers, and residual risks.
