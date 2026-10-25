---
id: "02-continue-preload-pagination"
title: "Continue pagination through the preload region"
status: done
wave: 2
depends_on:
  - "01-establish-transcript-start"
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001
acceptance_criteria:
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.5
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.6
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.7
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.8
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.9
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.11
  - AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.12
system_design:
  - ../../specs/ui/system-design/task-prompt-transcript-visibility.md
---

# Task 02: Continue pagination through the preload region

## Summary

Make positive older-page loads continue from committed sentinel geometry
instead of visible-row identity. Show the existing loader only for recoverable
error or no-progress state, then prove the combined flow on desktop and mobile.

## In scope

- Evaluate current sentinel geometry after layout and scroll restoration.
- Continue while the sentinel remains inside the configured preload region.
- Retain prompt `#1`, exhaustion, stale-session, and no-progress stops.
- Track recovery state and expose a managed retry action.
- Hide the manual loader during normal pagination.
- Preserve prepend anchoring and raw search backfill.
- Add desktop and mobile long-history regressions.

## Out of scope

- Changes to initial history-fetch size or older-page size.
- Backend cursors or message persistence.
- A new mobile surface or new localized copy.

## Acceptance

- One upward navigation crosses short standalone and collapsed pages while the
  sentinel remains in preload, then stops when current geometry leaves it.
- Successful pagination never exposes the manual loader; failure or no progress
  exposes one retry control and a successful retry clears it.
- Desktop and mobile reach a stored prompt more than twenty older pages away
  without a click while retaining stable scroll position.

## Verification

```bash
cd apps/web
pnpm exec vitest run \
  hooks/use-lazy-load-sentinel.test.ts \
  hooks/use-lazy-load-messages.test.ts \
  components/task/chat/message-list-native.test.tsx \
  components/task/chat/message-list-shared.test.tsx
pnpm run typecheck
```

```bash
make build-web
cd apps/web
pnpm e2e:run --host --no-build --project chromium -- \
  tests/chat/message-pagination.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome -- \
  tests/chat/mobile-message-pagination.spec.ts
```

## Files likely touched

- `apps/web/hooks/use-lazy-load-sentinel.ts`
- `apps/web/hooks/use-lazy-load-sentinel.test.ts`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native.test.tsx`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-shared.test.tsx`
- `apps/web/e2e/tests/chat/message-pagination-helpers.ts`
- `apps/web/e2e/tests/chat/message-pagination.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts`

## Dependencies

- Task 01 must provide authoritative transcript-start state before the combined
  browser flow changes fallback expectations.

## Risks

- Geometry must be measured after prepend restoration, not from a stale
  IntersectionObserver sample.
- Recovery retries must not loop automatically on the same failed cursor.
- The recovery control needs a 44-pixel coarse-pointer target without changing
  fine-pointer desktop density.
- Long-history fixtures must not make focused E2E runs flaky or exceed their
  existing timeout budget.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001` acceptance criteria `.5` to
  `.9`, `.11`, and `.12`.
- Upward pagination, failure and recovery, and responsive sections of the
  system design.
- Existing lazy-sentinel, native-scroll, desktop, and mobile pagination tests.

## Results

- Replaced visible-boundary continuation with committed sentinel/root geometry,
  retaining boundary keys for anchoring and diagnostics.
- Added transcript-local recovery state and an explicit retry path for rejected
  or zero-progress requests; routine successful pagination stays button-free.
- Added committed-geometry continuation and session-epoch guards so stale
  observer entries and prior-session settlements cannot stop or alter the
  current transcript.
- Preserved the desktop fine-pointer density and added a coarse-pointer
  minimum 44-pixel recovery target.
- Added long-history and short-boundary fixtures plus matching desktop/mobile
  browser regressions.
- The focused Task 02 Vitest suite passed (4 files, 115 tests).
- `pnpm run typecheck` and `make build-web` passed.
- Chromium pagination passed (6 tests); mobile Chrome pagination passed (6
  tests).
