---
id: "03-prove-complete-history-navigation"
title: "Prove complete transcript navigation"
status: done
wave: 3
depends_on:
  - "01-preserve-task-prompt-fallback"
  - "02-auto-load-collapsed-history"
plan: "plan.md"
spec: "../../specs/ui/task-prompt-transcript-visibility.md"
---

# Task 03: Prove complete transcript navigation

## Intent

Prove that desktop and mobile users can scroll through collapsed older history
to the first prompt without repeated load actions.

## Acceptance

- The desktop transcript reloads with the original task prompt visible.
- More than 100 same-turn tool events form a collapsed activity row across
  multiple backend pages.
- Upward scrolling loads every required page without selecting **Load older
  messages**.
- The recent standalone row keeps its viewport position during automatic
  prepends.
- The original first prompt remains at the transcript start after pagination.
- The same user outcome passes in the `mobile-chrome` project.

## Files likely touched

- `apps/web/e2e/tests/chat/message-pagination.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-pagination.spec.ts` (new)
- `apps/web/e2e/tests/chat/message-pagination-helpers.ts` (new shared setup)

## Steps

1. Extract shared setup for a task with a description and a seeded session.
2. Seed 80 older tool events through the session `commandCount` option.
3. Seed one standalone agent message and 60 newer tool events.
4. Change the desktop regression so it scrolls upward and never selects the
   explicit load action.
5. Measure the recent standalone row before and after pagination.
6. Add the equivalent mobile regression with the shared native scroll owner.
7. Run the desktop and mobile specs separately and record discovered test
   counts and results.

## Verification

```bash
cd apps/web && pnpm e2e:run --project chromium \
  tests/chat/message-pagination.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/chat/mobile-message-pagination.spec.ts
```

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential. This task verifies the combined behavior from tasks 01 and 02.

## Inputs

- The spec scenarios for collapsed activity and complete pagination.
- `seedTaskSession(..., { commandCount })` for bulk same-turn tool events.
- `seedSessionMessage` for the recent standalone agent message.
- `seedToolCallMessages` for the newer same-turn tool events.
- The existing pagination regression in
  `apps/web/e2e/tests/chat/message-pagination.spec.ts`.
- Mobile chat scroll patterns in
  `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts`.

## Risks

- Seed tool events on both sides of the recent agent message. The newest
  100-row window must contain that standalone message and still leave older
  tool-only pages.
- Use the visible `.chat-message-list` as the only scroll owner.
- Run desktop and mobile projects separately because the managed runner accepts
  one project per invocation.

## Output contract

Report the seeded history shape, desktop and mobile results, exact commands,
and any artifacts or cleanup. Then mark this task as done and update its plan
checkbox.

## Results

- Replaced the desktop repeated-button workaround with upward scrolling and
  shared the collapsed-history setup with a new mobile regression.
- The fixture seeds an initial user prompt, 80 older same-turn tool events, 60
  newer same-turn tool events, and a recent standalone agent message. The task
  description starts as the visible fallback and disappears when pagination
  reveals the stored prompt.
- Desktop: `cd apps/web && pnpm e2e:run --project chromium
tests/chat/message-pagination.spec.ts` passed (1 test).
- Mobile: `cd apps/web && pnpm e2e:run --project mobile-chrome
tests/chat/mobile-message-pagination.spec.ts` passed (1 test).
- Both scenarios continue upward scrolling at each loaded-page boundary rather
  than clicking the recovery button, wait for the first prompt, assert that no
  load button remains, and verify the recent standalone row stays within 8
  pixels of its anchored position after every prepend.
- After rebuilding the current web bundle, the final host-run desktop and
  mobile pagination commands passed (1 test each): `pnpm run build:e2e`, then
  `pnpm e2e:run --host --no-build --project chromium
tests/chat/message-pagination.spec.ts` and the equivalent `mobile-chrome`
  command.
- CI fixup showed that the synthetic task-description fallback also matched the
  metadata fixture text. The desktop and mobile metadata specs now use exact
  text matching for the stored message row.
- The full desktop last-prompt navigation suite passed after synthetic rows
  were excluded from prompt lookup (11 tests). Desktop and mobile metadata
  overflow suites each passed (1 test).
