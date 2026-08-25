---
id: "02-auto-load-collapsed-history"
title: "Auto-load collapsed transcript history"
status: done
wave: 2
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-prompt-transcript-visibility.md"
---

# Task 02: Auto-load collapsed transcript history

## Intent

Continue older-page loading while the transcript sentinel remains in its
preload region after a positive request.

## Acceptance

- A positive older-page result re-arms the native transcript sentinel.
- A still-visible sentinel can load the next page without another button
  action.
- A zero-result or rejected request does not create an automatic request loop.
- A fixed synthetic task-description row does not hide an older-page prepend
  from scroll anchoring.
- Existing request coordination remains unchanged.
- Prepend scroll anchoring keeps the visible reading position stable.
- The explicit **Load older messages** action remains available.

## Files likely touched

- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/chat/message-list-native.test.tsx`

## Steps

1. Add a transcript integration regression that expects positive-result
   re-arming.
2. Add a prepend regression with a fixed synthetic task-description row.
3. Run the focused tests and record the expected failures.
4. Enable `rearmWhileIntersecting` for the native transcript sentinel.
5. Track prepend progress from the oldest non-synthetic message.
6. Run the focused tests and the web type check.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- \
  hooks/use-lazy-load-sentinel.test.ts \
  components/task/chat/message-list-native.test.tsx
cd apps/web && pnpm run typecheck
```

## Dependencies

None.

## Parallelism

Sequential. Task 03 depends on this behavior and the task-prompt fallback.

## Inputs

- The collapsed-activity and first-prompt scenarios in the spec.
- `useLazyLoadSentinel` re-arm tests in
  `apps/web/hooks/use-lazy-load-sentinel.test.ts`.
- Native transcript wiring in
  `apps/web/components/task/chat/message-list-native-scroll.ts`.
- Prepend rules in
  `apps/web/components/task/chat/transcript-auto-scroll.ts`.

## Risks

- Do not enable `joinInFlightWhileLoading` for the transcript sentinel.
- Do not retry zero-result requests automatically.
- Do not remove the explicit recovery button.
- Do not use the synthetic task-description ID as the prepend boundary.

## Output contract

Report the sentinel option, regression results, exact commands, and remaining
risks. Then mark this task as done and update its plan checkbox.

## Results

- Enabled positive-result re-arming for the native transcript sentinel without
  enabling in-flight request joining.
- Changed prepend detection to use the oldest non-synthetic render item, so the
  fixed task-description row cannot hide older-page progress.
- Red: the focused native-scroll command failed the new sentinel option and
  prepend-anchor assertions before the production change (2 failed, 16
  passed).
- Green: `cd apps && pnpm --filter @kandev/web test --
hooks/use-processed-messages.test.ts
hooks/use-processed-messages-fallback.test.ts
hooks/use-lazy-load-sentinel.test.ts
components/task/chat/message-list-native.test.tsx` passed (4 files, 84
  tests).
- Added regression coverage for a one-for-one synthetic-to-stored prompt
  replacement, where the rendered item count stays equal but the oldest real
  key changes. The native scroll anchor now compensates that layout update.
- Reused the exported synthetic row ID instead of duplicating the sentinel
  value in the native scroll owner.
- Extended the shared re-arming sentinel so an intersection observed during
  initial/refetch loading retries when the firing guard clears, and a positive
  page result schedules the next eligible page after prepend layout settles.
  The follow-up keeps the observer/node validity checks and stops on a
  zero-result or rejected request.
- The final focused pagination unit suite passed (6 files, 162 tests),
  including `message-list-shared.test.tsx` and `transcript-auto-scroll.test.ts`.
- The final focused pagination unit suite passed again after the sentinel
  follow-up change (6 files, 163 tests).
- The explicit load-older recovery path remains unchanged.
