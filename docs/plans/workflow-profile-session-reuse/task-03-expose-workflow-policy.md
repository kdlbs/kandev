---
id: "03-expose-workflow-policy"
title: "Simplify the selector and add agent logos"
status: pending
wave: 3
depends_on:
  - "02-implement-safe-session-parking"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.9
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.10
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.13
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 03: Simplify the Selector and Add Agent Logos

## Summary

Replace the three combined choices with separate start and end groups. Show the
same agent logos as the new-task selector.

## In scope

- Update step draft, dirty state, save, and read-only behavior for both fields.
- Add the lifecycle summary and two choice groups.
- Add the new helper messages in every locale.
- Use `AgentLogo` in profile rows and the selected-profile trigger.
- Keep the generic agent icon for the workflow-default choice.
- Keep the desktop popover and mobile inset drawer.

## Acceptance

- The selector contains no combined policy labels.
- Both settings update independently and share one save action.
- Actual profiles show their agent logo.
- Mobile uses one scroll region and 44 px rows.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-step-agent-profile-selector.test.tsx components/settings/workflow-dirty-state.test.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/components/settings/workflow-step-agent-profile-selector.tsx`
- `apps/web/components/settings/workflow-step-agent-profile-selector.test.tsx`
- `apps/web/components/settings/workflow-dirty-state.ts`
- `apps/web/hooks/domains/settings/use-workflow-settings.ts`
- `apps/web/src/locales/*/workflows.json`

## Dependencies

Task 02.

## Risks

The text can imply that end behavior applies after each turn instead of a
profile-switch exit.

## Parallelism

`sequential`

## Results

Pending.
