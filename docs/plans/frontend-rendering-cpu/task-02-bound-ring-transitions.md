---
id: "02-bound-ring-transitions"
title: "Bound context-ring transitions"
status: done
wave: 2
depends_on:
  - "01-attribute-rendering"
plan: "plan.md"
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-005
acceptance_criteria:
  - AC-UI-PERSISTENT-STATUS-MOTION-005.1
  - AC-UI-PERSISTENT-STATUS-MOTION-005.2
  - AC-UI-PERSISTENT-STATUS-MOTION-005.3
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
---

# Task 02: Bound context-ring transitions

## Summary

Remove accidental context-ring scrollbar and color transitions. Keep the existing value, geometry, 300 ms easing, and disclosure behavior.

## In scope

Change only ContextWindowRing's transition property to stroke-dashoffset. Add a browser regression that changes usage across a color threshold and inspects computed transition properties and resulting arc. Retain existing context-source and touch disclosure cases. Begin with a failing rendered assertion against transition-all.

## Out of scope

Backend/state changes, plugin implementation changes, transcript virtualization,
and unmounting user content. No global production animation suppression.

## Acceptance

The ring reaches the new usage value and threshold color. No scrollbar-color transition or unrelated transition is created. Desktop and mobile disclosure behavior and geometry remain intact.

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
(cd apps/web && rtk pnpm test components/task/chat/token-usage-display.test.tsx)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/chat/context-window-source.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/chat/mobile-context-window-source.spec.ts)
```

Record actual discovery counts, failures, and results. Do not substitute a
skipped trace run for performance evidence. Implementation uses TDD for changed
logic; the attribution work order produces diagnostic evidence first.

## Files likely touched

- `apps/web/components/task/chat/token-usage-display.tsx`
- `apps/web/components/task/chat/token-usage-display.test.tsx`
- `apps/web/e2e/tests/chat/context-window-source.spec.ts`
- `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts`

## Dependencies

01-attribute-rendering. See the manifest for evidence gates.

## Risks

SVG stroke motion still uses rendering work during the bounded transition. Do not describe this as the main CPU fix.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/persistent-status-motion.md).
- [Design](../../specs/ui/system-design/persistent-status-motion.md).
- Existing animation trace, component tests, and desktop/mobile motion specs.
- Baseline findings in [plan.md](plan.md#evidence-and-limits).

## Results

The ring now transitions only `stroke-dashoffset` while preserving its 300 ms
duration, easing, geometry, threshold colors, and disclosure behavior. The
unit suite passed with 14 tests. The desktop context-source suite passed with
3 tests, and the mobile context-source suite passed with 3 tests. Task 01
evidence is recorded in [evidence.md](evidence.md); no production rendering
target has been selected for Task 03.
