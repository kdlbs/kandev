---
id: "03-repair-rendering-trigger"
title: "Repair the attributed rendering trigger"
status: pending
wave: 3
depends_on:
  - "01-attribute-rendering"
  - "02-bound-ring-transitions"
plan: "plan.md"
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-001
  - REQ-UI-PERSISTENT-STATUS-MOTION-002
  - REQ-UI-PERSISTENT-STATUS-MOTION-003
acceptance_criteria:
  - AC-UI-PERSISTENT-STATUS-MOTION-001.1
  - AC-UI-PERSISTENT-STATUS-MOTION-001.4
  - AC-UI-PERSISTENT-STATUS-MOTION-002.3
  - AC-UI-PERSISTENT-STATUS-MOTION-003.3
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
---

# Task 03: Repair the attributed rendering trigger

## Summary

Eliminate the reproduced host-owned continuous rendering trigger while preserving visible motion. This work order is evidence-gated until Task 01 identifies the target.

## In scope

Restrict the repair to the target and existing motion boundaries identified in Task 01. Reuse compositor spin, grid, or pulse mechanisms where applicable. Add a failing behavioral or node-attributed trace regression before changing production. Update the design and this work order with exact ownership and targeted test commands before execution; do not treat the candidate list as blanket edit scope.

## Out of scope

Backend/state changes, plugin implementation changes, transcript virtualization,
and unmounting user content. No global production animation suppression.

## Acceptance

Pause/restore evidence identifies a specific target. A pre-fix regression fails for its recurring invalidations and passes after repair. Normal-page before/after captures demonstrate removal of the attributed recurring work with visible feedback, settlement, and mobile behavior preserved.

## ASCII UI preview

UI-01, excerpt from the [combined preview](plan.md#ascii-ui-preview):

```text
Desktop / phone existing task surface:
Visible active -> motion runs
Hidden active  -> motion pauses, state remains current
Visible settled -> settled status
Context ring -> arc transition only
```

Preserve the existing desktop panels and dedicated phone Chat composition.
The work order's frontmatter ACs map this preview to its rendered checks.

## Verification

From the repository root, after the plan's one-time dependency bootstrap:

```bash
(cd apps/web && rtk pnpm test lib/ui/state-icons.test.tsx lib/ui/compositor-pulse.test.tsx components/grid-spinner.test.tsx)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/chat/persistent-animation-motion.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/chat/mobile-persistent-animation-motion.spec.ts)
(cd apps/web && KANDEV_E2E_ANIMATION_TRACE=1 rtk pnpm e2e:run --project chromium tests/chat/animation-performance-trace.spec.ts)
```

Record actual discovery counts, failures, and results. Do not substitute a
skipped trace run for performance evidence. Implementation uses TDD for changed
logic; the attribution work order produces diagnostic evidence first.

## Files likely touched

- `apps/packages/ui/src/compositor-spin.tsx (candidate only)`
- `apps/packages/ui/src/compositor-pulse.tsx (candidate only)`
- `apps/web/components/grid-spinner.tsx (candidate only)`
- `apps/web/app/globals.css (candidate only)`
- `apps/web/e2e/tests/chat/animation-performance-trace.spec.ts`

## Dependencies

01-attribute-rendering, 02-bound-ring-transitions. See the manifest for evidence gates.

## Risks

The exact source is unknown. If it is plugin-owned, outside status motion, or requires transcript virtualization, record the finding and revise the relevant design before proceeding. No speculative memoization or new rendering architecture.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/persistent-status-motion.md).
- [Design](../../specs/ui/system-design/persistent-status-motion.md).
- Existing animation trace, component tests, and desktop/mobile motion specs.
- Baseline findings in [plan.md](plan.md#evidence-and-limits).

## Results

Pending by design. Task 01 isolates the disposable fixture's grid and composer
pulse invalidations, but the supplied trace's recurring Layerize work is not
attributed to a production node. The evidence gate therefore prevents a
speculative repair. Reopen this work order only after a target-specific trace
and failing regression identify the source.
