---
id: "04-migrate-legacy-browser-drafts"
title: "Migrate legacy browser drafts"
status: completed
wave: 4
depends_on: ["03-project-comments-across-task-sessions"]
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-004
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-004.1
  - AC-TASKS-PLAN-COMMENTS-004.2
  - AC-TASKS-PLAN-COMMENTS-004.3
  - AC-TASKS-PLAN-COMMENTS-004.4
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 04: Migrate legacy browser drafts

## Summary

Add a bounded one-time bridge from the session-scoped plan comments shipped by
the interim repair to the backend task collection. Preserve every record until
its own UUID is acknowledged and prevent a send from racing ahead of migration.

## In scope

- Enumerating legacy `kandev.comments.<sessionId>` payloads for sessions in the
  open task.
- Idempotent per-comment upload, selective storage cleanup, retry state, and a
  task-level migration gate.
- Tests for multiple sessions, duplicate text with distinct IDs, exact replay,
  partial failure, missing plan, non-plan records, and Send/Run gating.

## Out of scope

- Recovering records already deleted by releases before the interim repair.
- Migrating comments from sessions that do not belong to the open task.
- Removing session storage for other comment types.

## Acceptance

- Every discoverable legacy plan record is uploaded with its original UUID and
  becomes part of the shared task snapshot exactly once.
- Cleanup removes only acknowledged plan records and preserves failed rows,
  non-plan comments, malformed-but-readable payloads, and data for a missing
  current plan.
- Comment-bearing Send and Run cannot omit a legacy record while migration is
  pending or failed; the UI exposes retry without losing authored content.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/state/slices/comments/persistence.test.ts hooks/domains/comments/use-plan-comment-migration.test.tsx hooks/use-message-handler.test.ts components/task/passthrough-chat-composer.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/state/slices/comments/persistence.ts`
- `apps/web/lib/state/slices/comments/persistence.test.ts`
- `apps/web/hooks/domains/comments/use-plan-comment-migration.ts`
- `apps/web/hooks/domains/comments/use-plan-comment-migration.test.tsx`
- `apps/web/components/task/task-plan-panel.tsx`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/hooks/use-message-handler.ts`
- `apps/web/components/task/passthrough-chat-composer.tsx`

## Dependencies

Task 03.

## Risks

- `sessionStorage` is tab-local, so migration must not use a global completed
  marker that suppresses data found later in another tab.
- Rewriting a mixed legacy payload after one acknowledgement can overwrite a
  concurrent local update unless cleanup rereads and removes by UUID.

## Parallelism

`sequential`

## Inputs

- Requirement: `REQ-TASKS-PLAN-COMMENTS-004`.
- System design: legacy migration and failure recovery.
- Existing `loadSessionComments`, `persistSessionComments`, task-session list,
  and message-send gating patterns.

## Results

- Added per-task migration that scans all known session payloads and uploads
  legacy plan comments with their original UUIDs.
- Cleanup rereads storage and removes only acknowledged plan IDs, preserving
  failed rows, unrelated comment sources, and missing-plan data.
- Send and Run fail closed while migration is unresolved, with localized retry
  UI; migration and persistence regression tests pass.
