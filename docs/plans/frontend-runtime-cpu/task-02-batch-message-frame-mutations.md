---
id: "02-batch-message-frame-mutations"
title: "Batch message frame mutations"
status: completed
wave: 2
depends_on:
  - "01-coalesce-browser-diagnostic-work"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.7
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.9
system_design:
  - ../../specs/platform/system-design/bounded-task-status-delivery.md
---

# Task 02: Batch Message Frame Mutations

## Summary

Apply all accepted replacement messages from one animation frame in one
Zustand and Immer transaction. Preserve message merge and barrier behavior.

## Failing regression first

Add a scheduler test named `applies different message keys with one bulk store
action`. Enqueue two message keys, flush one frame, and prove that the store
receives one ordered array instead of two `updateMessage` calls.

Add a real-store test named `notifies subscribers once for one replacement
frame`. Seed two messages, subscribe to the session message array, apply the
bulk action, and prove that both messages change after one notification.

## In scope

- Add `updateMessages(messages)` to the session slice type and implementation.
- Share partial-field merge logic with `updateMessage(message)`.
- Convert, settle, and stale-filter frame payloads before one store action.
- Preserve first-key insertion order from the scheduler map.
- Preserve add, delete, and turn-settle barriers.
- Keep unaffected session arrays and message objects stable.
- Extend scheduler and slice tests for multi-session frames.

## Out of scope

- Backend lifecycle or gateway coalescing changes.
- Protocol payload or timestamp changes.
- Incremental transcript grouping or virtualization.
- A replacement for Zustand or Immer.

## Acceptance

- Several accepted message keys in one frame cause one bulk store call.
- A real store emits at most one subscriber notification for that frame.
- Final message fields match the current single-update behavior.
- Stale payloads remain rejected.
- Add and delete events cannot be reordered around pending replacements.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/ws/handlers/messages.test.ts lib/state/slices/session/session-slice.update-messages.test.ts lib/state/slices/session/session-slice.merge-messages.test.ts
cd apps && pnpm --filter @kandev/web run typecheck
cd apps/web && pnpm exec eslint lib/ws/handlers/messages.ts lib/ws/handlers/messages.test.ts lib/state/slices/session/types.ts lib/state/slices/session/session-slice.ts lib/state/slices/session/session-slice.update-messages.test.ts
```

## Files likely touched

- `apps/web/lib/ws/handlers/messages.ts`
- `apps/web/lib/ws/handlers/messages.test.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/session-slice.update-messages.test.ts`

## Dependencies

- Task 01 completes first so the later comparison trace can attribute logger
  and store work separately.

## Risks

- Payload conversion inside the mutation can observe different turn state.
- A multi-session frame can accidentally replace an unaffected session array.
- Partial updates can clear known fields if undefined values are merged.
- Barrier tests can pass with mocks while real subscriber count still grows.

## Parallelism

`sequential`

## Inputs

- Bounded task status delivery acceptance criteria 7 and 9.
- The accepted replaceable session stream decision.
- Existing message scheduler and session slice tests.

## Results

Implemented the bulk replacement action and scheduler batching. Accepted
replacement payloads are converted and settled before one `updateMessages`
transaction, while add, delete, and turn-settle events remain ordered barriers.
The shared merge helper preserves partial-field behavior and unaffected session
state remains stable. The stale-snapshot regression now asserts that neither the
legacy single-update action nor the production bulk action is called.

Targeted verification passed:

- `lib/ws/handlers/messages.test.ts`
- `lib/state/slices/session/session-slice.update-messages.test.ts`

The broader work-order verification remains in the package-level check.
