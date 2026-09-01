---
id: "01-recover-pagination-after-activation"
title: "Recover pagination after transcript activation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.7
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.8
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.11
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.12
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.14
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
---

# Task 01: Recover pagination after transcript activation

## Summary

Make a restored, previously hidden transcript re-evaluate older-page
eligibility after activation, and make an upward action at hard top recover
even when the browser emits no scroll-position change.

## In scope

- Add failing native transcript and sentinel regressions before production
  changes.
- Carry real host visibility from `TaskChatPanel` through `MessageList` to
  native scroll management.
- Add a one-shot hidden-to-visible current-geometry recheck after saved scroll
  restoration.
- Add directional hard-top input recovery without treating programmatic scroll
  restoration as user intent.
- Preserve request serialization, session epochs, continuation, failure
  recovery, and prepend anchoring.
- Add a restored secondary-session desktop browser regression and run the
  existing mobile pagination suite.

## Out of scope

- Backend cursor, message-list, WebSocket, or persistence changes.
- Prompt History behavior changes.
- Automatic retries after rejected or zero-progress requests.
- New UI controls, localized copy, or mobile navigation.

## Acceptance

- A transcript hydrated while hidden issues exactly one cursor-based older-page
  request after it becomes visible at its restored oldest loaded edge.
- Upward input at `scrollTop = 0` can start the same guarded request without a
  scroll event, while repeated lifecycle renders and one physical gesture do
  not duplicate it.
- Desktop restored-tab E2E reaches older content, and existing desktop/mobile
  pagination, prompt-`#1`, bounded-opening, recovery, and anchor cases remain
  green.

## Verification

Run from `apps/web`:

```bash
pnpm exec vitest run \
  hooks/use-lazy-load-sentinel.test.ts \
  components/task/chat/message-list-native.test.tsx \
  hooks/use-lazy-load-messages.test.ts
pnpm run typecheck
pnpm exec eslint \
  hooks/use-lazy-load-sentinel.ts \
  hooks/use-lazy-load-sentinel.test.ts \
  components/task/task-chat-panel.tsx \
  components/task/chat/message-list-shared.tsx \
  components/task/chat/message-list-native.tsx \
  components/task/chat/message-list-native-scroll.ts \
  components/task/chat/message-list-native.test.tsx \
  e2e/tests/chat/message-pagination-helpers.ts \
  e2e/tests/chat/message-pagination.spec.ts
pnpm exec prettier --check \
  hooks/use-lazy-load-sentinel.ts \
  hooks/use-lazy-load-sentinel.test.ts \
  components/task/task-chat-panel.tsx \
  components/task/chat/message-list-shared.tsx \
  components/task/chat/message-list-native.tsx \
  components/task/chat/message-list-native-scroll.ts \
  components/task/chat/message-list-native.test.tsx \
  e2e/tests/chat/message-pagination-helpers.ts \
  e2e/tests/chat/message-pagination.spec.ts
pnpm e2e:run --host --project chromium -- \
  tests/chat/message-pagination.spec.ts --grep "restored inactive session"
pnpm e2e:run --host --no-build --project mobile-chrome -- \
  tests/chat/mobile-message-pagination.spec.ts
```

## Files likely touched

- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/hooks/use-lazy-load-sentinel.ts`
- `apps/web/hooks/use-lazy-load-sentinel.test.ts`
- `apps/web/e2e/tests/chat/message-pagination-helpers.ts`
- `apps/web/e2e/tests/chat/message-pagination.spec.ts`

## Dependencies

None. The existing `usePanelActive` signal, `SessionPanelContent` scroll restore,
message cursor API, and pagination fixtures are inputs to this work.

## Risks

- A recheck scheduled too early can run before restored geometry stabilizes.
- Relaxing the sentinel's disarmed state can bypass explicit failure recovery.
- Shared hook changes can affect Prompt History's bottom sentinel.
- Input listeners can double-fire when one gesture also creates a scroll event.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001` acceptance criteria `.5`,
  `.7` to `.14`.
- Upward pagination, failure and recovery, observability, and responsive
  sections of the paired system design.
- Existing panel visibility, scroll restoration, sentinel, desktop pagination,
  and mobile pagination implementations and tests.

## Results

- Added a guarded current-geometry recheck to the shared lazy-load sentinel.
- Carried Dockview visibility into native transcript scroll management and
  rechecked after hidden-to-visible restoration settles.
- Added hard-top wheel, keyboard, and touch intent recovery while preserving
  programmatic-scroll and explicit-recovery guards.
- Added red-first unit regressions and a restored secondary-session Chromium
  regression that proves the older request includes a cursor.
- Verified 87 focused unit tests, eight desktop pagination cases, eight mobile
  pagination cases, typecheck, targeted lint/formatting, and spec lint.
