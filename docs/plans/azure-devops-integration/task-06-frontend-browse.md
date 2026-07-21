---
id: "06-frontend-browse"
title: "Responsive browse and task PR UI"
status: done
wave: 3
depends_on: ["05-frontend-settings"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 06: Responsive Browse And Task PR UI

## Acceptance

- `/azure-devops` browses work items and PRs with filtering, pagination, task launch, and PR feedback detail.
- Linked Azure PR summaries appear on task surfaces through a provider-tagged presentation model without converting Azure details into GitHub types.
- Desktop and mobile expose the same required actions without horizontal page scrolling or hover-only controls.

## Verification

- `rtk pnpm --filter @kandev/web test -- --run components/azure-devops/azure-devops-status.test.ts lib/state/slices/azure-devops/azure-devops-slice.test.ts` from `apps`.
- `rtk pnpm --filter @kandev/web typecheck` from `apps`.
- `rtk pnpm --filter @kandev/web lint` from `apps`.

## Files Likely Touched

- `apps/web/app/azure-devops/azure-devops-page-client.tsx`
- `apps/web/components/azure-devops/`
- `apps/web/lib/state/slices/azure-devops/`
- `apps/web/spa-routes.tsx`
- Task PR summary/detail components identified during implementation.

## Inputs

- Completed Task 05.
- Patterns: GitLab responsive browse page and GitHub task PR detail surfaces.
- Required workflow: `.agents/skills/mobile-parity/SKILL.md`.

## Output Contract

Report surfaces and responsive behavior, files changed, unit/type/lint commands, blockers, and set this task plus its plan checkbox to done.
