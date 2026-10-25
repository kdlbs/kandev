---
id: message-queue-merge-04
title: E2E merge flow
status: completed
wave: 4
depends_on: ["message-queue-merge-03"]
plan: plan.md
spec: ../../specs/ui/requirements/message-queue-merge.md
---

# Task 04: E2E merge flow

## Acceptance

1. A Playwright test in the chat message-queue spec queues two messages while
   the agent is busy, opens the queue panel, clicks **Merge with above** on the
   second entry, and asserts the panel now shows a single entry whose text
   contains both messages and the queue indicator reflects one queued message.

## Verification

- `cd apps/web && pnpm e2e:run --host tests/chat/message-queue.spec.ts`

## Files

- `apps/web/e2e/tests/chat/message-queue.spec.ts`

## Inputs

- Spec scenario: "two queued messages `A` and `B` with `B` behind `A` … one
  message whose content is `A` + blank line + `B`".
- Plan section: `E2E Tests`.
- Pattern to mirror: the existing `seedTaskAndWaitForIdle` + `openQueuePanel`
  helpers in `message-queue.spec.ts` and `typeWhileBusy`.

## Risks

- Agent-kind merging is not exercised in E2E (inter-task dispatch is not
  practical in this harness) — backend + unit tests cover it.
- Timing: the first message must be slow enough (`/slow 10s`) that the second
  stays queued; reuse the existing retry + timeout conventions of the suite.
