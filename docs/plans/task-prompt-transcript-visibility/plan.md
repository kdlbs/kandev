---
spec: docs/specs/ui/requirements/task-prompt-transcript-visibility.md
created: 2026-08-22
status: implemented
---

# Implementation plan: Restore complete transcript history

## Overview

Fix [GitHub issue 2914](https://github.com/kdlbs/kandev/issues/2914) and the
related older-history scrolling defect. Two independent frontend conditions
break the path to the first prompt.

A throwaway regression test confirmed the defect. It supplied one visible
agent message and a task description. The test expected
`["task-description", "boot"]` but received `["boot"]`. The focused test run
reported 1 failure and 51 passes. The throwaway change was then removed.

The task-description fallback currently requires
`visibleMessages.length === 0`. Any agent or environment row removes it. The
native transcript also calls `useLazyLoadSentinel` with its one-shot defaults.
When an older page only enlarges a collapsed activity row, the sentinel stays
in view but does not re-arm. The existing message-pagination E2E documents the
manual button as the workaround for this exact state.

The shared sentinel already supports safe positive-result re-arming. Its
focused tests cover the enabled and disabled re-arm cases and the zero-result
guard, but browser behavior also requires an eligibility retry after a blocked
intersection and a deferred follow-up after prepend layout settles. The repair
extends that shared behavior without a backend or API change.

## Frontend

- Update `apps/web/hooks/use-processed-messages.ts` so the synthetic task
  description depends on the absence of a visible user-authored message.
- Keep the synthetic message before agent and environment messages.
- Suppress the synthetic message as soon as visible history contains a real
  user message. This avoids duplicate prompts.
- Preserve the existing empty-description behavior.

### Older-history scrolling

- Update `apps/web/components/task/chat/message-list-native-scroll.ts` so the
  native transcript enables `rearmWhileIntersecting` on its shared sentinel.
- Keep `joinInFlightWhileLoading` disabled. The transcript must wait for its
  current request instead of joining another load from the sentinel callback.
- Let the intersection state stop the chain after visible older content pushes
  the sentinel above its preload region.
- Change `useScrollPositionOnPrepend` to detect older-page progress from the
  oldest non-synthetic message. The fixed task-description row must not hide a
  prepend from the scroll anchor.
- Keep the explicit **Load older messages** button for error and no-progress
  recovery.
- Preserve the existing prepend scroll anchoring. Do not expand activity rows
  or change the scroll container.

## Tests

- Add `apps/web/hooks/use-processed-messages-fallback.test.ts` with a regression
  test that fails before the fix and passes after it.
- Cover agent-only visible history with a task description. Confirm that the
  synthetic task message remains first.
- Cover visible user history with a task description. Confirm that no
  synthetic duplicate appears.
- Extend `apps/web/components/task/chat/message-list-native.test.tsx` with a
  transcript integration case. Confirm positive-result re-arming and stable
  scroll position with a fixed task-description row.
- Keep the shared zero-result and stale-completion cases in
  `apps/web/hooks/use-lazy-load-sentinel.test.ts` green.

## E2E tests

- Replace the repeated button actions in
  `apps/web/e2e/tests/chat/message-pagination.spec.ts` with upward scrolling.
  Seed more than 100 same-turn tool events so older pages only extend a
  collapsed row. Confirm that pagination reaches the first prompt without a
  button action. Measure the recent standalone row before and after the load so
  prepend anchoring cannot move the reading position.
- Add `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts` with the same
  user outcome and scroll-position assertion in the `mobile-chrome` project.

Desktop and mobile use the same native transcript and scroll owner. The mobile
entry point remains the task chat, and the existing full-height task layout is
the nearest mobile exemplar. This change adds no control, overlay, layout, or
touch target. The mobile scenario proves that the phone scroll container also
reaches the first prompt without repeated actions.

## Backend

No backend change is required. The standard launch path already records the
initial prompt. Database repair and message backfill remain outside this fix.

## Verification commands

Run the unit regressions during the red and green TDD steps:

```bash
cd apps && pnpm --filter @kandev/web test -- \
  hooks/use-processed-messages.test.ts \
  hooks/use-processed-messages-fallback.test.ts \
  hooks/use-lazy-load-sentinel.test.ts \
  components/task/chat/message-list-native.test.tsx
```

Run the focused browser regression after the unit test passes:

```bash
cd apps/web && pnpm e2e:run --project chromium \
  tests/chat/message-pagination.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/chat/mobile-message-pagination.spec.ts
```

Run the web type check after the implementation is complete:

```bash
cd apps/web && pnpm run typecheck
```

## Verification results

- Task 01 red: `cd apps && pnpm --filter @kandev/web test --
hooks/use-processed-messages.test.ts` failed at the new agent-only fallback
  assertion before the production change (1 failed, 51 passed).
- Task 01 green: the same focused unit command passed (53 tests).
- Task 02 red: the focused native-scroll command failed the new sentinel option
  and prepend-anchor assertions before the production change (2 failed, 16
  passed).
- Task 02 green: `cd apps && pnpm --filter @kandev/web test --
hooks/use-processed-messages.test.ts
hooks/use-processed-messages-fallback.test.ts
hooks/use-lazy-load-sentinel.test.ts
components/task/chat/message-list-native.test.tsx` passed (4 files, 84
  tests).
- Web typecheck: `cd apps/web && pnpm run typecheck` passed.
- Web lint: `pnpm --filter @kandev/web lint` passed with zero warnings.
- Desktop E2E: `cd apps/web && pnpm e2e:run --project chromium
tests/chat/message-pagination.spec.ts` passed (1 test).
- Mobile E2E: `cd apps/web && pnpm e2e:run --project mobile-chrome
tests/chat/mobile-message-pagination.spec.ts` passed (1 test).
- PR fixup unit suite: `cd apps && pnpm --filter @kandev/web test --
hooks/use-processed-messages.test.ts hooks/use-processed-messages-fallback.test.ts
hooks/use-lazy-load-sentinel.test.ts components/task/chat/message-list-native.test.tsx
components/task/chat/message-list-shared.test.tsx
components/task/chat/transcript-auto-scroll.test.ts` passed (6 files, 162
  tests).
- PR fixup: synthetic task-description rows now stay out of prompt navigation,
  equal-count synthetic-to-stored replacements preserve the scroll anchor, the
  synthetic row construction stays referentially stable while streaming, and
  the metadata overflow locators distinguish the stored row from its fallback.
- PR fixup desktop last-prompt suite: `cd apps/web && pnpm e2e:run --project
chromium tests/chat/last-prompt-scroll.spec.ts` passed (11 tests).
- PR fixup desktop metadata suite: `cd apps/web && pnpm e2e:run --project
chromium tests/chat/message-metadata-overflow.spec.ts` passed (1 test).
- PR fixup mobile metadata suite: `cd apps/web && pnpm e2e:run --project
mobile-chrome tests/chat/mobile-message-metadata-overflow.spec.ts` passed (1
  test).
- PR fixup web typecheck and lint passed with zero errors and warnings.
- Final PR-fixup remediation: the shared re-arming sentinel now retries when
  eligibility clears after an observed blocked state and schedules the next
  positive page after prepend layout settles. Desktop and mobile pagination
  regressions now continue upward scrolling at each boundary and assert the
  anchored row after each prepend.
- Final remediation verification: focused unit tests passed (6 files, 163
  tests), web typecheck passed, web lint passed with zero errors and warnings,
  and rebuilt-bundle desktop and mobile pagination E2E each passed (1 test).
- Blocker remediation: sentinel-owned loads are serialized, one deferred
  continuation is reserved per positive page, and idle transitions from those
  loads cannot schedule a second continuation. The stateful 20/20/20/0
  regression passes with one invocation per cursor, including the final zero
  result.
- After merging the current `origin/main`, the focused unit suite passed again
  (6 files, 170 tests), web typecheck and lint remained clean, and the backend
  E2E binary and fixture package were rebuilt before rerunning both pagination
  E2E specs successfully (1 desktop test, 1 mobile test).

## Implementation waves

Execution is sequential in the primary conversation.

Wave 1:

- [x] [task-01-preserve-task-prompt-fallback](task-01-preserve-task-prompt-fallback.md)

Wave 2:

- [x] [task-02-auto-load-collapsed-history](task-02-auto-load-collapsed-history.md)

Wave 3:

- [x] [task-03-prove-complete-history-navigation](task-03-prove-complete-history-navigation.md)

## Risks

- The fallback must inspect messages after visibility filtering. A hidden user
  row must not remove the only prompt that the user can see.
- A stored visible user prompt must remain authoritative. Otherwise normal
  sessions show duplicate prompts.
- The change must not alter message ordering after the fallback decision.
- Automatic re-arm must occur only after positive progress. A zero-result or
  rejected request must not create a tight request loop.
- Scroll anchoring must preserve the visible reading position after a new
  standalone row appears above it.
- The explicit load button must remain available as a recovery path.

## Public documentation

None. This repair restores expected transcript behavior and does not change a
public command, configuration key, API, or workflow.
