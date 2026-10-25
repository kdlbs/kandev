---
id: "01-constrain-transcript-pagination-cycles"
title: "Constrain transcript pagination cycles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.4
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.6
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.8
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
---

# Task 01: Constrain Transcript Pagination Cycles

## Summary

Limit one upward transcript action to its visible top boundary. Keep automatic continuation only for pages that do not add a visible entry.

Add bounded debug events and browser geometry cues. These events make a future pagination cascade visible without message content.

## In scope

- Add a failing unit regression for stale-intersection continuation.
- Add failing desktop and mobile regressions for one-action request count.
- Track upward intent and the oldest non-synthetic render-item key.
- Continue through collapsed activity without a second action.
- Preserve the existing prepend anchor tolerance.
- Emit bounded `messages:pagination` debug events.

## Out of scope

- Backend, API, database, page-size, and cursor changes.
- Transcript virtualization or message-grouping changes.
- Prompt History behavior changes.
- New user-facing copy or controls.

## Acceptance

- One upward action starts one request when the page adds visible transcript entries.
- The same action can cross tool-only pages until one new visible entry appears.
- Desktop and mobile preserve the anchor row and do not load older history on task open.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- hooks/use-lazy-load-sentinel.test.ts components/task/chat/message-list-native.test.tsx
cd apps && pnpm --filter @kandev/web run typecheck
cd apps/web && pnpm exec eslint hooks/use-lazy-load-sentinel.ts hooks/use-lazy-load-sentinel.test.ts components/task/chat/message-list-native-scroll.ts components/task/chat/message-list-native.test.tsx e2e/tests/chat/message-pagination-helpers.ts e2e/tests/chat/message-pagination.spec.ts e2e/tests/chat/mobile-message-pagination.spec.ts
cd apps/web && pnpm e2e:run --host --project chromium tests/chat/message-pagination.spec.ts -- --retries=0
cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-message-pagination.spec.ts -- --retries=0
```

## Files likely touched

- `apps/web/hooks/use-lazy-load-sentinel.ts`
- `apps/web/hooks/use-lazy-load-sentinel.test.ts`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/lib/debug/log.ts`
- `apps/web/e2e/tests/chat/message-pagination-helpers.ts`
- `apps/web/e2e/tests/chat/message-pagination.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts`

## Dependencies

None.

## Risks

- React commit timing can make a boundary comparison stale.
- A loading indicator can change geometry without changing the boundary.
- Shared sentinel changes can affect Prompt History.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001` acceptance criteria 4, 5, 6, 8, and 9.
- The upward load-cycle and observability sections in the system design.
- Existing native-scroll, sentinel, desktop, and mobile pagination tests.

## Results

- Added a post-commit continuation decision to the shared sentinel. A false decision disarms stale intersection state until the sentinel exits and re-enters the preload region.
- The native transcript now compares the oldest standalone message across a request. It continues through tool-only pages, but stops when a new standalone entry appears or history is exhausted.
- A later actual upward scroll now retries a disarmed sentinel that remains inside the 200 px preload margin. The scroll owner covers wheel, keyboard, scrollbar, and touch movement while excluding guarded programmatic scrolls.
- Added bounded `messages:pagination` start and settle events with request generation, boundary keys, scroll geometry, loaded count, actual continuation outcome, and stop reason. Debug-only geometry and payload work is absent from production.
- Added red-first unit coverage and desktop/mobile browser regressions for request count, collapsed activity, short boundary pages, prompt `#1`, no eager open, and the 8 px prepend-anchor tolerance.
- Verification passed: 50 targeted unit tests, typecheck, targeted ESLint, Prettier, specification lint, 5 desktop Chromium tests, and 5 mobile Chrome tests.
