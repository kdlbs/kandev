---
id: "05-domain-target-coverage"
title: "Dynamic, integration, system, and account target coverage"
status: completed
wave: 5
depends_on: ["04-general-target-coverage"]
plan: "plan.md"
spec: "../../specs/ui/requirements/settings-discovery.md"
---

# Task 05: Dynamic, integration, system, and account target coverage

## Acceptance

- Workspace, agent, executor, integration, system, and account pages resolve safe page/section/control
  entries with dynamic names and encoded canonical routes.
- Auth/admin/feature/capability gates match rendering behavior and do not reveal inaccessible pages.
- Generated resource rows, secret values, and plugin-authored internals remain unindexed.

## Verification

`cd apps && pnpm --filter @kandev/web test -- --run lib/settings-discovery components/settings components/{azure-devops,github,gitlab,jira,linear,sentry,slack}`

## Files likely touched

- Dynamic, integration, system, and account files named in `plan.md`
- Relevant English and generated pseudo locale namespaces

## Dependencies

Task 04.

## Parallelism

Sequential because catalog and locale files are shared.

## Results

- `cd apps && pnpm --filter @kandev/web test -- --run lib/settings-discovery components/settings components/azure-devops components/github components/gitlab components/jira components/linear components/sentry components/slack`
  — passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet` — passed; pseudo locale
  synchronized.
