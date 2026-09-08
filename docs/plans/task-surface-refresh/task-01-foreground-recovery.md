---
id: "01-foreground-recovery"
title: "Foreground recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/task-surface-refresh.md"
---

# Task 01: Foreground recovery

## Acceptance

- One shared hook emits refresh on visible `visibilitychange`, `focus`,
  `pageshow`, and `online` independent of WebSocket status, while coalescing
  burst and in-flight calls.
- Open task details, session lists, active chat messages, and queued messages
  reload from their existing authoritative endpoints without changing task or
  session selection.
- Hidden events, stale task responses, and failed foreground requests retain
  the last usable UI and remain retryable.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/use-foreground-refresh.test.ts hooks/use-task-sessions.test.ts hooks/domains/session/use-visibility-backfill.test.ts hooks/domains/session/use-queue.test.ts
```

## Files likely touched

- `apps/web/hooks/use-foreground-refresh.ts`
- `apps/web/hooks/use-foreground-refresh.test.ts`
- `apps/web/hooks/use-task-sessions.ts`
- `apps/web/hooks/use-task-sessions.test.ts`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/hooks/domains/session/use-visibility-backfill.test.ts`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/hooks/domains/session/use-queue.test.ts`
- `apps/web/components/task/task-page-content.tsx`

## Dependencies

None.

## Parallelism

Sequential. Task 02 consumes this task's lifecycle hook.

## Inputs

- Spec: `What` foreground and task-detail bullets; task-detail and event-burst
  scenarios; failure modes.
- Existing forced reload behavior in `useTaskSessions`.
- Existing visibility-only message and queue backfills.
- Existing task identity/request guard in `TaskPageContent`.

## Risks

- React callback identity churn must not re-register global listeners or bypass
  coalescing.
- Task navigation during an in-flight foreground request must not commit the
  old task.

## Output contract

Completed 2026-07-27.

Verification passed:

```bash
cd apps && pnpm --filter @kandev/web test -- --run hooks/use-foreground-refresh.test.ts hooks/use-task-sessions.test.ts hooks/domains/session/use-visibility-backfill.test.ts hooks/domains/session/use-queue.test.ts
```

Result: 4 files, 27 tests passed.
