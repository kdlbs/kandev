---
id: "02-aggregate-replacement-e2e"
title: "Prove one aggregate replacement prompt"
status: done
wave: 2
depends_on: ["01-fifo-send-now-arbitration"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-send-now.md"
---

# Task 02: Prove one aggregate replacement prompt

## Acceptance

- The desktop header Send Now scenario proves the two or more queued bodies
  are rendered in one user-message bubble in FIFO order, not merely that all
  strings eventually appear somewhere in the transcript.
- The desktop scenario still proves the queue is empty after a successful bulk
  claim and the replacement turn completes.
- The existing Pixel 5 Send Now scenario passes against the repaired backend,
  preserving touch geometry, neighboring-row order, and zero document
  horizontal overflow.
- The test does not rely on arbitrary sleeps to establish correctness; it uses
  visible queue/prompt/idle signals and the existing mock-agent fixtures.

## Verification

```bash
cd apps/web
pnpm e2e --project=chromium e2e/tests/chat/message-queue.spec.ts --grep 'header Send Now' --retries=0
pnpm e2e --project=mobile-chrome e2e/tests/chat/mobile-message-queue-management.spec.ts --grep 'Send Now' --retries=0
```

## Files Likely Touched

- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts` only if
  the shared-path assertion needs a focused strengthening

## Dependencies

- Task 01 must pass its focused backend race tests and leave the backend
  handoff state stable.

## Parallelism

Sequential. The browser assertions depend on the backend arbitration contract
and share the queue fixtures with the existing mobile scenario.

## Inputs

- The amended spec's one-prompt FIFO handoff scenario.
- Existing desktop header and row Send Now coverage in
  `message-queue.spec.ts`.
- Existing Pixel 5 coverage in
  `mobile-message-queue-management.spec.ts`.
- `data-testid="user-message-bubble"` as the transcript-level assertion for a
  single aggregate user prompt.

## TDD Sequence

1. Strengthen the desktop bulk scenario to locate the replacement user bubble
   containing the first body and require the second body in that same bubble;
   run it against the pre-fix implementation to record the expected RED
   behavior when the FIFO handoff wins.
2. Run the scenario after Task 01 and assert the queue disappearance and idle
   transition only after the aggregate bubble is visible.
3. Re-run the existing mobile Send Now scenario, recording touch-target and
   overflow evidence; do not alter production UI for this backend repair.

## Output Contract

Report the RED/GREEN browser results, the exact single-bubble assertion, the
mobile parity and overflow checks, files changed, generated traces/screenshots
if any, and cleanup evidence. Mark this task `done` and update its plan
checkbox only after both focused commands pass.

## Results

- Strengthened the desktop bulk scenario with a single
  `user-message-bubble` assertion containing all queued bodies in FIFO order.
- Desktop managed E2E passed: 1 test for the header Send Now scenario.
- Pixel 5/mobile-chrome managed E2E passed: 1 test covering Send Now without
  hover, 44px touch geometry, neighboring queue order, and zero horizontal
  overflow.
- No production frontend change or generated E2E artifact was required.
