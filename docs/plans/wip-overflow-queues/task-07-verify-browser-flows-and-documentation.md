---
id: "07-verify-browser-flows-and-documentation"
title: "Verify browser flows and documentation"
status: in_progress
wave: 7
depends_on:
  - "06-expose-visible-queue-ux"
plan: "plan.md"
spec: "../../specs/tasks/system-design/wip-limit-pull-system.md"
---

# Task 07: Verify Browser Flows and Documentation

## Acceptance

- Desktop E2E proves seven no-feeder WIP-2 tasks all appear, two are admitted,
  five are queued, and the next task promotes when capacity opens.
- Desktop E2E proves feeder overflow appears in the feeder and moves to the
  intended destination.
- Mobile E2E proves focused-column navigation, queued badge/count visibility,
  promotion, tap usability, and no page-level horizontal overflow.
- Public workflow documentation matches the implemented admission, feeder, and
  conflict behavior and distinguishes WIP from session concurrency.
- The spec, ADR, plan, and task statuses are updated to reflect the final
  implementation and actual validation evidence.

## E2E verification

```bash
cd apps/web
pnpm e2e:run tests/kanban/wip-overflow-queue.spec.ts tests/kanban/mobile-wip-overflow-queue.spec.ts
```

Use the managed E2E runner. The `mobile-` filename selects `mobile-chrome`; do
not add redundant project flags.

## Documentation verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/e2e/tests/kanban/wip-overflow-queue.spec.ts`
- `apps/web/e2e/tests/kanban/mobile-wip-overflow-queue.spec.ts`
- `docs/public/tasks-and-workflows.md`
- `docs/public/workflow-tips.md`
- this plan, task files, spec, and ADR status

## Dependencies

- Task 06 supplies the complete backend and frontend behavior.

## Parallelism

`sequential`

## Output contract

Record exact browser projects, passed scenario counts, mobile overflow/tap
evidence, docs-validator output, final artifact statuses, and any intentionally
deferred limitation. Do not mark the plan complete if the one-hop assumption
still differs from implemented behavior.
