---
id: "07-prove-bounded-task-traffic"
title: "Prove bounded task traffic"
status: completed
wave: 7
depends_on:
  - "03-decouple-git-polling"
  - "04-stabilize-session-transport"
  - "05-consume-task-summaries"
  - "06-idempotent-message-submission"
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 07: Prove Bounded Task Traffic

## Acceptance

- A 27-task WebSocket capture proves zero inactive session subscriptions/detail
  frames and constant per-switch subscription/focus work; Playwright attaches
  frame/byte totals for diagnosis.
- Inactive clarification, error/dismissal, Git, and PR changes update desktop
  and mobile rows through summary revisions with existing precedence.
- Under deliberate notification pressure, the selected model selector remains
  usable and retrying one message ID produces one persisted message/turn with a
  resolved send result.

## TDD sequence

1. RED: replace old E2E assumptions that require bulk Git subscriptions or
   focus-triggered snapshots with assertions for the new bounded contract.
2. RED: add deterministic server-side inactive activity and verify the browser
   did not receive its detail frames; fail if the activity source was not
   exercised.
3. RED: add desktop/mobile summary updates and notification-pressure message
   retry scenarios.
4. GREEN: fix integration wiring only; do not weaken frame classification,
   task-count, activity-source, or duplicate-message assertions.
5. REFACTOR: centralize WebSocket capture/byte accounting helpers and attach
   concise diagnostics on pass/failure.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/session/session-stream-budget.spec.ts \
  -- --project=chromium
cd apps/web && pnpm e2e:run tests/task/task-status-summary.spec.ts \
  -- --project=chromium
cd apps/web && pnpm e2e:run tests/task/mobile-task-status-summary.spec.ts \
  -- --project=mobile-chrome
cd apps/web && pnpm e2e:run tests/chat/message-send-pressure.spec.ts \
  -- --project=chromium
make -C apps/backend test
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/e2e/tests/session/session-stream-budget.spec.ts`
- `apps/web/e2e/helpers/ws-traffic.ts`
- `apps/web/e2e/tests/task/task-status-summary.spec.ts`
- `apps/web/e2e/tests/task/mobile-task-status-summary.spec.ts`
- `apps/web/e2e/tests/chat/message-send-pressure.spec.ts`
- `apps/web/e2e/tests/session/session-focus-signal.spec.ts`
- `apps/web/e2e/tests/task/sidebar-diff-stats.spec.ts`
- `apps/web/e2e/tests/chat/message-add-ws-gap.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-add-ws-gap.spec.ts`
- WebSocket capture/drop and task/session seeding helpers
- plan/task status fields after verification

## Dependencies

Tasks 03–06 must be complete so the tests observe final polling, transport,
switcher, and message contracts.

## Parallelism

Sequential final integration/QA task.

## Inputs

- The captured incident baseline: 700 frames, 3.23 MB, 74 subscribes, and 47
  unsubscribes in five seconds for 27 sessions.
- Every scenario in the approved spec.
- Existing frame-drop helpers and model/message/sidebar E2E suites.

## Risks

- A quiet seed would make the isolation test meaningless; assert background
  activity occurred server-side.
- Byte totals vary with compression/build data and are diagnostic, while
  inactive-session identity and O(1) subscription counts are hard assertions.
- Mobile tests must use the existing sheet and assert no layout/overflow
  regression while status changes arrive.

## Verification results

The implementation-level backend and frontend verification is complete:

- `make -C apps/backend lint` — passed with 0 issues.
- Broad backend task/gateway/backendapp/lifecycle/orchestrator tests — passed.
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm run lint` — passed.
- `cd apps/web && pnpm exec vitest run` — passed (1,011 files; 7,743 tests, 4 skipped).
- `cd apps/web && pnpm e2e:run --host tests/task/sidebar-diff-stats.spec.ts` — passed
  (1 test; 7.2s), confirming persisted diff badges survive a backend restart for
  an inactive sidebar task.

- `cd apps/web && pnpm e2e:run --host --no-build
  tests/session/session-stream-budget.spec.ts` — passed (1 test; 9.0s).
  The 27-task capture measured 479 gateway frames / 86,243 bytes across five
  task switches, with 5 `session.subscribe` frames, 4 `session.unsubscribe`
  frames, and zero inactive-session detail frames.

- `cd apps/web && pnpm e2e:run --host --no-build
  tests/task/task-status-summary.spec.ts` — passed (1 test; 2.7s), confirming
  an inactive desktop row receives clarification, error, and PR revisions
  while another task remains selected.
- `cd apps/web && pnpm e2e:run --host --no-build
  tests/task/mobile-task-status-summary.spec.ts` — passed (1 test; 2.8s),
  confirming the native mobile task switcher receives the same bounded status
  revisions without horizontal overflow.
- `cd apps/web && pnpm e2e:run --host --no-build
  tests/chat/message-send-pressure.spec.ts` — passed (1 test; 12.6s),
  confirming 80 streamed tool-call notifications do not hide the model
  selector and a dropped `message.add` response reconciles to one persisted
  user message and one new turn.

The dedicated 27-task capture and summary-pressure scenarios are now complete;
their byte totals remain diagnostic while inactive-session detail isolation and
constant subscription work are the correctness assertions.

## Output contract

Report exact frame/subscription/byte measurements, inactive-frame assertions,
desktop/mobile results, message/turn counts, broad verification commands,
exact files changed, and any residual traffic sources.
