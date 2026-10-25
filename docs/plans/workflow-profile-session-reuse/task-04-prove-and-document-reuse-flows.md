---
id: "04-prove-and-document-reuse-flows"
title: "Prove all lifecycle combinations"
status: pending
wave: 4
depends_on:
  - "03-expose-workflow-policy"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.12
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.13
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 04: Prove All Lifecycle Combinations

## Summary

Prove both lifecycle settings from the editor to runtime session identity.
Update public workflow guidance.

## In scope

- Desktop save, reload, and runtime scenarios for all four combinations.
- Mobile selection, save, reload, focus, touch, safe-area, and overflow checks.
- Public Tasks and Workflows documentation.

## Acceptance

- Runtime IDs and states prove all four combinations.
- Mobile proves the same selection value as desktop.
- Public docs explain start and end settings without combined policy terms.

## Verification

```bash
(cd apps/web && pnpm e2e:run tests/workflow/workflow-agent-switch.spec.ts -- --grep 'session lifecycle')
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-settings.spec.ts -- --grep 'session lifecycle')
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/e2e/tests/workflow/workflow-agent-switch.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`
- `apps/web/e2e/pages/workflow-settings-page.ts`
- `docs/public/tasks-and-workflows.md`

## Dependencies

Task 03.

## Risks

The E2E must assign source and destination settings to the correct steps.

## Parallelism

`sequential`

## Results

Pending.
