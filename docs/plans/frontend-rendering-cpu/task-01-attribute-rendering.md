---
id: "01-attribute-rendering"
title: "Attribute continuous rendering"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-002
  - REQ-UI-PERSISTENT-STATUS-MOTION-003
acceptance_criteria:
  - AC-UI-PERSISTENT-STATUS-MOTION-002.3
  - AC-UI-PERSISTENT-STATUS-MOTION-003.3
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
---

# Task 01: Attribute continuous rendering

## Summary

Produce a repeatable reproduction and identify the target that keeps the main thread rendering. Do not change production code in this work order.

## In scope

Extend the gated trace fixture with normal-script baseline, all-motion paused, restoration, and per-group isolation arms. Inventory CSS and Web Animations, including pseudo-elements. Reload the isolated fixture between arms or restore exact play states and inline declarations in finally. Assert no active motion remains in the pause control; detect animations created after the first inventory. Preserve the existing narrow compositor and CSS-fallback controls.

## Out of scope

Backend/state changes, plugin implementation changes, transcript virtualization,
and unmounting user content. No global production animation suppression.

## Acceptance

Record three equal windows per arm, environment details, target identities, CPU availability, event counts/durations, and node-level invalidations. A target is attributed only when pausing removes the recurring work and restoration reproduces it. Add the exact target files, regression, and commands to Task 03 before production repair; if not reproduced, record the limitation and keep that task pending.

## Verification

From the repository root, after the plan's one-time dependency bootstrap:

```bash
(cd apps/web && KANDEV_E2E_ANIMATION_TRACE=1 rtk pnpm e2e:run --project chromium tests/chat/animation-performance-trace.spec.ts)
```

Record actual discovery counts, failures, and results. Do not substitute a
skipped trace run for performance evidence. Implementation uses TDD for changed
logic; the attribution work order produces diagnostic evidence first.

## Files likely touched

- `apps/web/e2e/tests/chat/animation-performance-trace.spec.ts`
- `apps/web/e2e/helpers/animation-assertions.ts`
- `docs/plans/frontend-rendering-cpu/evidence.md (new sanitized report)`
- `docs/plans/frontend-rendering-cpu/task-03-repair-rendering-trigger.md`

## Dependencies

None. See the manifest for evidence gates.

## Risks

The existing small fixture may not reproduce the user's long transcript or high refresh rate. Seed disposable realistic content and compare a smaller-DOM control; do not silently call a non-reproduction a pass.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/persistent-status-motion.md).
- [Design](../../specs/ui/system-design/persistent-status-motion.md).
- Existing animation trace, component tests, and desktop/mobile motion specs.
- Baseline findings in [plan.md](plan.md#evidence-and-limits).

## Results

Done. The gated trace fixture now records normal-page, compositor-control,
CSS-fallback, all-motion, and per-target pause/restore arms. It inventories CSS
animations, pseudo-elements, and Web Animations, retains all configured
repeats, and reports medians and ranges without adding nested event durations.

The three-repeat Chromium run passed after the trace-control remediation. The
fixture identifies grid and composer pulse invalidations with targets kept
present, compares selective pause arms against a matched script-enabled
baseline, and verifies continued activity in the unpaused group. The supplied
production trace's persistent Layerize work remains unattributed. See
[evidence.md](evidence.md). Task 03 remains pending under its evidence gate.
