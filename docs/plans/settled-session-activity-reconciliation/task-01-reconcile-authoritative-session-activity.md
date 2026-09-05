---
id: "01-reconcile-authoritative-session-activity"
title: "Reconcile authoritative session activity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BACKGROUND-WORK-LIVENESS-001
acceptance_criteria:
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.3
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.7
  - AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.9
system_design:
  - ../../specs/platform/system-design/background-work-liveness.md
---

# Task 01: Reconcile Authoritative Session Activity

## Summary

Clear a pre-restart activity projection when a complete session snapshot omits
it, without allowing an older HTTP response to overwrite a newer live event.

## In scope

- Add per-session client activity ordering state and cleanup.
- Reconcile complete list snapshots separately from partial session events.
- Add focused store, hook, and request-order regression tests.

## Out of scope

- Backend wire fields or tracker behavior.
- Session status components and copy.
- Browser E2E coverage, which belongs to Task 02.

## Acceptance

- A complete session-list record clears omitted activity, while a partial event
  continues to preserve omitted activity.
- Every complete-snapshot call supplies its request-start activity epoch map,
  and forced hydration calls serialize before loading-state rerenders.
- An explicit event received after a list request starts wins as the complete
  activity projection, including when it repeats the stored value.
- Session removal clears its activity epoch, and existing session merge guards
  remain green.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/session/session-slice.upsert.test.ts hooks/use-task-sessions.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/session-slice.upsert.test.ts`
- `apps/web/hooks/use-task-sessions.ts`
- `apps/web/hooks/use-task-sessions.test.ts`
- `apps/web/hooks/use-task-removal.ts`
- `apps/web/hooks/use-task-removal-session-loading.test.ts`
- `apps/web/app/office/tasks/[id]/page.tsx`
- `apps/web/app/office/tasks/[id]/page.test.tsx`

## Dependencies

None.

## Parallelism

`sequential`

The store contract and request-boundary capture must be implemented and tested
together.

## Inputs

- `AC-PLATFORM-BACKGROUND-WORK-LIVENESS-001.3` and `.9`.
- The complete-snapshot versus partial-event boundary in the system design.
- The existing sessions-added-during-load reconciliation in
  `useTaskSessions`.

## Risks

- Incrementing only when the value changes would miss a repeated newer event;
  the epoch must advance on every explicit event projection.
- Applying complete-snapshot omission semantics inside the generic merge would
  clear activity on unrelated partial updates.

## Results

- Added optional, client-local activity epochs that advance on every explicit
  activity event and are deleted with the session.
- Complete list snapshots now clear omitted activity defaults, while partial
  events retain their omission-preserving merge behavior.
- A shared helper ensures every client-side asynchronous session-list loader
  captures request-start epochs, so a newer live activity projection wins over
  an older response without blocking durable-field reconciliation.
- The focused store and loader suite passes all 58 tests, and web typecheck
  passes.
