---
id: "04-pause-hidden-motion"
title: "Pause hidden status motion"
status: done
wave: 4
depends_on:
  - "03-repair-rendering-trigger"
plan: "plan.md"
requirements:
  - REQ-UI-PERSISTENT-STATUS-MOTION-004
acceptance_criteria:
  - AC-UI-PERSISTENT-STATUS-MOTION-004.1
  - AC-UI-PERSISTENT-STATUS-MOTION-004.2
  - AC-UI-PERSISTENT-STATUS-MOTION-004.3
  - AC-UI-PERSISTENT-STATUS-MOTION-004.4
  - AC-UI-PERSISTENT-STATUS-MOTION-004.5
system_design:
  - ../../specs/ui/system-design/persistent-status-motion.md
---

# Task 04: Pause hidden status motion

## Summary

Pause host-owned persistent status motion while its target is hidden. Resume the current active state without losing updates or replaying settled activity.

## In scope

Add a shared lifecycle helper for owned animation handles and integrate spin, pulse, and grid. Observe stable target wrappers, combine document/intersection visibility, and use existing panel visibility where needed. Apply scoped CSS fallback pause/resume. Preserve prior inline styles, cleanup, reduced-motion behavior, and grid stagger. Test mount-hidden, visibility changes, effect replacement, unmount, and settlement while hidden.

## Out of scope

Backend/state changes, plugin implementation changes, transcript virtualization,
and unmounting user content. No global production animation suppression.

## Acceptance

Hidden targets have no running owned effects; visible active targets resume with no duplicate handles. Current state and all non-motion UI behavior survive hidden intervals. Desktop/mobile browser checks and three matched final trace captures record the resulting behavior and performance.

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
(cd apps/web && rtk pnpm test lib/ui/persistent-motion-visibility.test.tsx lib/ui/compositor-pulse.test.tsx lib/ui/state-icons.test.tsx components/grid-spinner.test.tsx)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/chat/persistent-animation-motion.spec.ts tests/chat/quick-chat-idle-dot.spec.ts)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/chat/mobile-persistent-animation-motion.spec.ts tests/chat/mobile-quick-chat-idle-dot.spec.ts)
(cd apps/web && KANDEV_E2E_ANIMATION_TRACE=1 rtk pnpm e2e:run --project chromium tests/chat/animation-performance-trace.spec.ts)
rtk proxy python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Record actual discovery counts, failures, and results. Do not substitute a
skipped trace run for performance evidence. Implementation uses TDD for changed
logic; the attribution work order produces diagnostic evidence first.

## Files likely touched

- `apps/packages/ui/src/persistent-motion-visibility.tsx (new)`
- `apps/packages/ui/src/compositor-spin.tsx`
- `apps/packages/ui/src/compositor-pulse.tsx`
- `apps/web/components/grid-spinner.tsx`
- `apps/web/lib/ui/persistent-motion-visibility.test.tsx (new)`
- `apps/web/lib/ui/compositor-pulse.test.tsx`
- `apps/web/lib/ui/compositor-spin.test.tsx`
- `apps/web/lib/ui/state-icons.test.tsx`
- `apps/web/components/grid-spinner.test.tsx`
- `apps/web/e2e/helpers/animation-assertions.ts`
- `apps/web/e2e/tests/chat/persistent-animation-motion.spec.ts`
- `apps/web/e2e/tests/chat/mobile-persistent-animation-motion.spec.ts`
- `apps/web/e2e/tests/chat/animation-performance-trace.spec.ts`
- `apps/web/e2e/tests/chat/context-window-source.spec.ts`
- `apps/web/e2e/tests/chat/mobile-context-window-source.spec.ts`

## Dependencies

03-repair-rendering-trigger. See the manifest for evidence gates.

## Risks

A hidden document is not the same as a blurred window. Intersection alone may not model all panel visibility. Unsupported observers must not freeze visible feedback. If Task 03 is unreproduced, this bounded work may proceed with that limitation recorded, but the package remains incomplete.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/persistent-status-motion.md).
- [Design](../../specs/ui/system-design/persistent-status-motion.md).
- Existing animation trace, component tests, and desktop/mobile motion specs.
- Baseline findings in [plan.md](plan.md#evidence-and-limits).

## Results

Implemented the shared visibility controller in
`apps/packages/ui/src/persistent-motion-visibility.tsx` and integrated it with
the compositor spin, pulse, and grid primitives. It combines document and
intersection visibility, pauses owned Web Animations and CSS fallbacks, resumes
only active effects, and cleans up shared listeners and observers.

Focused unit coverage passed with 90 tests, including the visibility helper,
spin, pulse, grid, and ring regressions. Desktop and mobile persistent-motion
browser tests passed with 1 test each, including a real scroll across the
intersection boundary. Desktop and mobile Quick Chat indicator tests passed
with 2 tests each. The review-remediation three-repeat trace passed with
non-empty target assertions, a matched script-enabled fallback baseline, and
group-specific suppression checks. The supplied trace's production trigger
remains unidentified, so this work order does not claim to complete Task 03.
