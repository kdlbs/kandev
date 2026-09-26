---
id: "02-workflow-reset-e2e"
title: "Cover workflow reset and refresh in Playwright"
status: done
wave: 2
depends_on:
  - "01-reapply-model-after-reset"
plan: "plan.md"
spec: "../../specs/ui/requirements/acp-model-configuration-summary.md"
---

# Task 02: Cover workflow reset and refresh in Playwright

## Acceptance

- A task launched with the mock agent's non-default `Mock Smart` model retains
  that model when proceeding into a reset-context workflow step.
- The task selector shows `Mock Smart` after the reset turn settles and after a
  full page reload.
- The E2E uses the isolated mock agent and existing task/workflow UI; it does
  not require OpenCode credentials or production-agent discovery.

## Verification

- `cd apps/web && pnpm e2e:run tests/workflow/workflow-step-proceed.spec.ts -- --grep "preserves selected model across context reset"`

The Playwright regression must fail against the pre-fix backend because every
new mock ACP session advertises `Mock Fast`, then pass with Task 01 applied.

## Files likely touched

- `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts`

## Dependencies

Task 01.

## Parallelism

Sequential. The test exercises and depends on the lifecycle repair.

## Inputs

- Repair scenario in the linked ACP model-configuration spec
- Plan section `E2E Tests`
- Existing workflow seeding and proceed patterns in
  `apps/web/e2e/tests/workflow/workflow-step-proceed.spec.ts`
- Existing selector assertions in
  `apps/web/e2e/tests/chat/model-selector-error.spec.ts`
- Existing phone reachability/rendering coverage in
  `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`

## Output contract

Report the seeded workflow/profile, observable selector assertions, exact
Playwright result, failure artifacts if any, blockers, and risk notes. Mark
this task `done` and update its plan checkbox in the same conversation.
