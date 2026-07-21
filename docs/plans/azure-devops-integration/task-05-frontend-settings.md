---
id: "05-frontend-settings"
title: "Frontend data and settings"
status: done
wave: 3
depends_on: ["04-task-pr-wiring"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 05: Frontend Data And Settings

## Acceptance

- Typed Azure API helpers and domain hooks cover config, projects, repositories, work items, PRs, feedback, and task associations.
- Settings supports workspace selection, connection test/save/copy/delete, redacted credentials, and healthy/unhealthy states.
- The settings form fits narrow viewports and exposes every action to keyboard and touch users.

## Verification

- `rtk pnpm --filter @kandev/web test -- --run lib/api/domains/azure-devops-api.test.ts` from `apps`.
- `rtk pnpm --filter @kandev/web typecheck` from `apps`.

## Files Likely Touched

- `apps/web/lib/types/azure-devops.ts`
- `apps/web/lib/api/domains/azure-devops-api.ts`
- `apps/web/hooks/domains/azure-devops/`
- `apps/web/components/azure-devops/azure-devops-settings.tsx`
- `apps/web/app/settings/integrations/azure-devops/page.tsx`
- Settings routes and integration menu files.

## Inputs

- Completed backend routes from Task 04.
- Patterns: Jira workspace settings and shared integration status components.

## Output Contract

Report UI/data changes, files changed, tests and typecheck run, responsive decisions, blockers, and set this task plus its plan checkbox to done.
