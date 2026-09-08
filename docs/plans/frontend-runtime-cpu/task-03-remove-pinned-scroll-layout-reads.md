---
id: "03-remove-pinned-scroll-layout-reads"
title: "Remove pinned-scroll layout reads"
status: completed
wave: 3
depends_on:
  - "02-batch-message-frame-mutations"
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.1
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.2
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.5
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.7
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
---

# Task 03: Remove Pinned-scroll Layout Reads

## Summary

Place a pinned transcript at the native maximum offset with a write-only
operation. Keep all current auto-scroll guards and responsive behavior.

## Failing regression first

Add a unit test named `pins an appended message without reading content size`.
Make the transcript `scrollHeight` getter throw, append a message while the
near-bottom guard is true, and prove that the scroll setter receives the
maximum-offset request.

## In scope

- Add one write-only native bottom-placement helper.
- Use it for the work-start and pinned message-update paths.
- Preserve near-bottom, programmatic-lock, and layout-restore guards.
- Preserve disabled-state freezing and re-enable catch-up.
- Extend desktop and mobile auto-scroll browser scenarios.
- Confirm that the native transcript remains the only scroll owner.

## Out of scope

- Removing geometry reads from user scroll detection, pagination, prepend
  restoration, or catch-up decisions.
- Changing smooth navigation to a prompt or message.
- Changing transcript controls, layout, copy, or touch targets.

## Acceptance

- A pinned message append performs no content-size read in its commit effect.
- Enabled auto-scroll remains at the bottom after a live message arrives.
- Disabled auto-scroll retains the same `scrollTop` after content arrives.
- Desktop and mobile produce the same outcome through shared logic.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/chat/message-list-native.test.tsx components/task/chat/transcript-auto-scroll.test.ts
cd apps && pnpm --filter @kandev/web run typecheck
cd apps/web && pnpm exec eslint components/task/chat/message-list-native-scroll.ts components/task/chat/message-list-native.test.tsx e2e/tests/chat/auto-scroll-toggle.spec.ts e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts
cd apps/web && pnpm e2e:run --host --project chromium tests/chat/auto-scroll-toggle.spec.ts -- --retries=0
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-auto-scroll-toggle.spec.ts -- --retries=0
```

## Files likely touched

- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/components/task/chat/transcript-auto-scroll.test.ts`
- `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`
- `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts`

## Dependencies

- Task 02 completes first so the browser scenario exercises the final store
  notification cadence.

## Risks

- A large offset can be written while a user-owned scroll is active if a guard
  is lost.
- Unit DOM behavior can differ from browser clamping.
- Sticky prompt geometry can settle after the first bottom placement.
- Mobile touch scrolling can expose a different lock-release timing.

## Parallelism

`sequential`

## Inputs

- Transcript auto-scroll acceptance criteria 1, 2, 5, and 7.
- The native transcript auto-scroll system design.
- Existing desktop and mobile auto-scroll tests.

## Results

Implemented the write-only native bottom-placement helper and wired it into
work-start, pinned message updates, and re-enable catch-up. Initial placement,
user detection, pagination, prepend restoration, and explicit scroll decisions
retain their required geometry reads.

Verification passed:

- Native transcript unit suites: 2 files and 57 tests.
- Desktop auto-scroll browser suite: 9 tests passed.
- Mobile auto-scroll browser suite: 5 tests passed.
- The new enabled-live-message scenarios confirm both layouts remain at the
  bottom after incoming content.
