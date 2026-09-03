---
id: "03-rich-list-row-ui"
title: "Rich list-row UI"
status: done
wave: 2
depends_on:
  - "02-portable-rich-row-setting"
plan: "plan.md"
spec: "../../specs/ui/requirements/task-listing-display-preferences.md"
role: implementer
model_tier: balanced
---

# Task 03: Rich list-row UI

## Acceptance

- Desktop dropdown and mobile menu show **Show task details** only in List,
  with visible explanatory copy, defaulting off and PATCHing the portable
  setting.
- Compact rows remain unchanged while rich rows render available repository,
  description, PR, session, parent, and review metadata.
- Rich rows retain row navigation and independent secondary controls, fit phone
  width, and use shared repository/PR display rules.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`

## Files likely touched

- `apps/web/hooks/use-user-display-settings.ts`
- `apps/web/hooks/use-kanban-display-settings.ts`
- `apps/web/components/kanban-display-dropdown.tsx`
- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- `apps/web/components/kanban/kanban-header.tsx`
- `apps/web/app/tasks/tasks-page-client.tsx`
- `apps/web/app/tasks/tasks-list-view.tsx`
- optional focused sibling under `apps/web/app/tasks/`
- optional pure helper test beside the extracted helper
- `apps/web/hooks/domains/github/use-task-pr.ts` only if the existing hook
  cannot be reused by passing `null` while details are disabled

## Inputs

- Spec: rich-row **What** bullets and rich-row/mobile scenarios.
- Plan: **Portable richer-row frontend setting**, **Rich list rows**, and the
  complete **Mobile design contract**.
- Patterns: `apps/web/components/kanban-card-content.tsx`,
  `apps/web/components/github/pr-task-icon.tsx`, and repository-label guidance
  in `apps/web/AGENTS.md`.

## Output contract

Return a compact handoff with intent/acceptance, base/head SHA if committed,
changed files and entry points, spec sections, `user-flow + persistence` risk
tags, exact command results, uncertainties, and the task-file status update. Do
not edit `plan.md`.
