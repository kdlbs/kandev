---
id: "05-add-responsive-text-hierarchy-e2e"
title: "Add responsive text hierarchy E2E"
status: pending
wave: 3
depends_on:
  - "02-normalize-structured-app-confirmations"
  - "03-refine-task-cleanup-confirmations"
  - "04-normalize-structured-task-confirmations"
plan: "plan.md"
requirements:
  - REQ-UI-SURFACE-TEXT-HIERARCHY-001
  - REQ-UI-TASK-CLEANUP-CONFIRMATION-001
acceptance_criteria:
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.1
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.2
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.3
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.5
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.4
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.5
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.6
  - AC-UI-TASK-CLEANUP-CONFIRMATION-001.7
system_design:
  - ../../specs/ui/system-design/surface-text-hierarchy.md
  - ../../specs/ui/system-design/confirmation-warning-hierarchy.md
---

# Task 05: Add Responsive Text Hierarchy E2E

## Summary

Prove the integrated text hierarchy in rendered browsers using real entry
points, long dynamic content, and a longer bundled locale.

## In scope

- Add focused phone and desktop E2E specs for representative AlertDialog,
  Dialog, Drawer, and inline Alert text behavior without adding a
  production-only test route.
- Exercise task deletion through the real phone task-action flow with a long
  task title and Portuguese or pseudo-localized copy at 320px and 393px.
- Inspect computed title/body wrapping, structured left alignment, viewport and
  document containment, body scroll ownership, persistent action geometry,
  semantic Delete treatment, Cancel, and task survival.
- Add one desktop representative check and run existing long-dialog/delete
  regressions.

## Out of scope

- Production component changes except a stable selector when existing roles and
  labels cannot identify the required geometry boundary.
- Screenshot baselines or a new fixture application.

## Implementation acceptance

1. Phone checks prove balanced titles, non-balanced pretty descriptions,
   structured left alignment, and zero horizontal overflow at both target
   widths with long localized content.
2. Task cleanup content remains reachable with visible 44px actions; Delete is
   destructive, and Cancel closes without deleting the task.
3. Desktop preserves compact action density, surface placement, and existing
   delete behavior.

## TDD sequence

1. Write rendered assertions against the integrated pre-fix baseline where
   practical and record the expected computed-style/geometry failure.
2. If a stable selector is genuinely missing, add only the smallest semantic
   test seam in the owning component and cover it in that component's test.
3. Run mobile and desktop scenarios GREEN, then the E2E sleep ratchet and
   focused lint.

## Verification

```bash
cd apps/web
pnpm e2e:run --project mobile-chrome tests/task/mobile-confirmation-text-hierarchy.spec.ts
pnpm e2e:run --project chromium tests/task/confirmation-text-hierarchy.spec.ts
pnpm e2e:run --project chromium tests/task/dialog-long-text-overflow.spec.ts tests/task/sidebar-delete-confirm.spec.ts
pnpm run e2e:sleep-ratchet
pnpm exec eslint e2e/tests/task/mobile-confirmation-text-hierarchy.spec.ts e2e/tests/task/confirmation-text-hierarchy.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/task/mobile-confirmation-text-hierarchy.spec.ts` (new)
- `apps/web/e2e/tests/task/confirmation-text-hierarchy.spec.ts` (new)
- Existing E2E helpers only if reusable setup cannot expose the real flow.
- A production selector only if roles, names, and existing test IDs are
  insufficient.

## Dependencies

- Tasks 02, 03, and 04 must be integrated after Task 01.

## Parallelism

`sequential-integration`

## Inputs

- `docs/specs/ui/requirements/surface-text-hierarchy.md`
- `docs/specs/ui/system-design/surface-text-hierarchy.md`
- `docs/specs/ui/requirements/confirmation-warning-hierarchy.md`
- `docs/specs/ui/system-design/confirmation-warning-hierarchy.md`
- `docs/plans/surface-text-hierarchy/plan.md`
- `apps/web/AGENTS.md`
- `apps/web/e2e/README.md`
- `.agents/skills/e2e/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Results

Pending implementation.
