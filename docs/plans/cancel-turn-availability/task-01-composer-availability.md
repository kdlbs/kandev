---
id: "01-composer-availability"
title: "Restore shared cancellation availability"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CANCEL-TURN-PROGRESS-001
acceptance_criteria:
  - AC-UI-CANCEL-TURN-PROGRESS-001.1
  - AC-UI-CANCEL-TURN-PROGRESS-001.4
  - AC-UI-CANCEL-TURN-PROGRESS-001.6
  - AC-UI-CANCEL-TURN-PROGRESS-001.7
  - AC-UI-CANCEL-TURN-PROGRESS-001.8
  - AC-UI-CANCEL-TURN-PROGRESS-001.9
  - AC-UI-CANCEL-TURN-PROGRESS-001.10
  - AC-UI-CANCEL-TURN-PROGRESS-001.12
system_design:
  - ../../specs/ui/system-design/cancel-turn-availability.md
---

# Task 01: Restore shared cancellation availability

## Summary

Use working state for cancellation while preserving queue and steering behavior.
Deliver desktop and phone evidence through the real composer prop path.

## In scope

- Pass required `isWorking` from `useComposerProps` to `ChatInputContainer`.
- Retain clarification overrides in `shouldShowCancelAgent`; require session
  identity for an actionable control. Preserve existing preparation/startup.
- Keep explicit `canCancelAgent` through body and both toolbars. Reuse existing
  localized copy for the cancel icon accessible name and existing progress state.
- Replace `waitForComposerQueueMode`'s cancel-button wait with a scoped assertion
  on existing queue-derived `chat-input-running` / `chat-input-running-plan` classes.
- Add focused unit and browser coverage, including empty-input steering and
  background work in task and Quick Chat, and phone reachability.

## Out of scope

Palette registration, backend changes, queue ordering, and new cancellation copy.

## Acceptance

1. A rendered integration regression named `shows cancel for a working direct-input
   session` fails before the wiring change in `chat-input-container.test.tsx`.
   Assert the body receives `canCancelAgent=true` and `isAgentBusy=false`; also
   prove `useComposerProps` actually forwards working state with a hook test.
2. State-table tests cover steering, background, STARTING, preparation, idle,
   missing session, connected clarification, and disconnected clarification.
   Existing pending animation, retry eligibility, and send behavior remain intact.
3. Task and Quick Chat browser tests click/tap the cancel control with no typed
   message and assert the selected session settles. Phone tests measure 44px
   targets, viewport containment, and no document horizontal overflow, and save
   a rendered screenshot for comparison with UI-01.

## ASCII UI preview

UI-01, shared desktop/phone action order ([full preview](plan.md#ascii-ui-preview)):

```text
Working direct input: [Message editor] [Cancel][Send]
Cancellation pending: [Message editor] [Busy  ][Send]
Fully idle:           [Message editor]         [Send]
```

Criteria .7-.10 and .12 apply. Retain the phone bottom toolbar, 44px touch
controls, existing keyboard/safe-area behavior, and the desktop 28px controls.

## Verification

Run from the repository root; install once for a fresh worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/chat-input-container.test.tsx components/task/chat/use-composer-props.test.tsx components/task/chat/chat-input-body.test.tsx components/task/chat/chat-input-toolbar.test.tsx components/task/chat/chat-input-toolbar-composer.test.tsx hooks/domains/session/use-session-state.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/use-composer-props.ts components/task/chat/chat-input-container.tsx components/task/chat/chat-input-toolbar-primitives.tsx e2e/helpers/type-while-busy.ts e2e/tests/chat/cancel-turn-availability.spec.ts e2e/tests/chat/mobile-cancel-turn-availability.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/cancel-turn-availability.spec.ts tests/chat/mid-turn-steering.spec.ts tests/chat/message-queue.spec.ts tests/chat/cancel-progress-task-switch.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-cancel-turn-availability.spec.ts tests/chat/mobile-cancel-progress-reload.spec.ts)
git diff --check
```

## Files likely touched

Existing files:

- `apps/web/components/task/chat/use-composer-props.ts`
- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task/chat/chat-input-container.test.tsx`
- `apps/web/components/task/chat/chat-input-body.test.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-primitives.tsx`
- `apps/web/components/task/chat/chat-input-toolbar.test.tsx`
- `apps/web/components/task/chat/chat-input-toolbar-composer.test.tsx`
- `apps/web/hooks/domains/session/use-session-state.test.ts`
- `apps/web/e2e/helpers/type-while-busy.ts`

New files:

- `apps/web/components/task/chat/use-composer-props.test.tsx`
- `apps/web/e2e/tests/chat/cancel-turn-availability.spec.ts`
- `apps/web/e2e/tests/chat/mobile-cancel-turn-availability.spec.ts`

Update other direct `ChatInputContainer` callers/test fixtures only as required
by the new required prop; enumerate them with `rg` before implementation.

## Dependencies

None.

## Risks

Preserve clarification suppression, pending-state session identity, and direct
steering delivery. Palette sources do not deduplicate matching command IDs.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/cancel-turn-progress.md).
- [Design](../../specs/ui/system-design/cancel-turn-availability.md).
- [Plan and source-trace evidence](plan.md).

## Results

- RED: the direct working-input integration regression returned
  `canCancelAgent=false` before `isWorking` was wired through the composer;
  the hook, toolbar accessible-name, and state-table regressions also failed at
  their pre-fix boundaries.
- GREEN: `isWorking` now travels from `useComposerProps` through
  `ChatInputContainer`, cancellation requires a session identity, and the
  clarification override remains intact. The toolbar uses the existing
  localized cancel label and keeps send and queue behavior independent.
- Passthrough explicitly sets `showCancelAgent={false}` because its parent
  `onCancel` callback dismisses the composer. The parent-path regression proves
  the callback still closes the composer and cannot cancel the agent, including
  through the existing Escape dismissal.
- The queue E2E helper now waits for the queue-derived composer classes instead
  of using cancel visibility as queue evidence.
- Focused frontend suite passed with 9 files and 128 tests. Typecheck, i18n,
  targeted ESLint, targeted E2E sleep lint, Vite build, and the desktop and
  mobile cancellation regressions passed. Mobile coverage verified the 44px
  composer target, composer containment, screenshot capture, and no horizontal
  overflow. Quick Chat direct composer coverage is recorded in Task 02 because
  it shares the Quick Chat palette fixture.
