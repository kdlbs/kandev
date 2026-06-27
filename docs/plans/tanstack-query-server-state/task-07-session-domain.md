---
id: "07-session-domain"
title: "Session domain"
status: done
wave: 3
depends_on: ["03-query-options-taxonomy", "04-query-bridge-audit"]
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 07: Session Domain

## Acceptance

- Task sessions, by-id session lookup, messages, turns, task plans/revisions,
  and message queue data read from TanStack Query.
- Chat message lists on desktop and mobile read the same query key that
  `session.message.*` bridge handlers update.
- Existing missed-message E2E recovery tests remain green without relying on
  manual reload.

## Verification

- PASS `cd apps && pnpm --filter @kandev/web test -- lib/query/keys.test.ts lib/query/seed.test.ts lib/query/query-options/query-options.test.ts lib/query/bridge/index.test.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-search.test.ts hooks/domains/session/use-session-state.test.ts hooks/domains/session/use-session-actions.test.ts hooks/domains/session/use-ensure-task-session.test.ts components/task/chat/message-list-shared.test.tsx components/task/chat/queued-ghost-list.test.tsx hooks/use-plan-panel-auto-open.test.ts hooks/use-task-removal.test.ts components/task/passthrough-chat-composer.test.ts`
  - 14 files, 148 tests.
- PASS `cd apps && pnpm --filter @kandev/web test -- hooks/domains/session components/task/chat lib/query`
  - 58 files, 451 tests.
- PASS `cd apps/web && pnpm typecheck`
- PASS direct API-read scan for migrated session/task-plan/queue reads:
  `rg -n "fetchTaskSession\\(|listTaskSessions\\(|listTaskSessionMessages\\(|listSessionTurns\\(|searchSessionMessages\\(|getTaskPlan\\(|listPlanRevisions\\(|getPlanRevision\\(|getQueueStatus\\(" apps/web/hooks apps/web/components apps/web/app apps/web/lib/query`
  - Only query-option factories and the server boot loader remain.
- PASS `cd apps/web && pnpm e2e:docker tests/chat/message-add-ws-gap.spec.ts tests/chat/message-pagination.spec.ts tests/session/session-tab-management.spec.ts tests/session/session-recovery.spec.ts tests/chat/message-queue.spec.ts tests/task/plan-checkpointing.spec.ts tests/chat/implement-plan-fresh.spec.ts tests/system/ws-event-accounting.spec.ts`
  - 35 desktop Docker tests.
- PASS `cd apps/web && e2e/scripts/run-e2e.sh --docker --no-build --project mobile-chrome -- tests/chat/mobile-message-add-ws-gap.spec.ts tests/session/mobile-transient-retry.spec.ts`
  - 2 mobile Docker tests.

## Files Likely Touched

- `apps/web/hooks/domains/session/*`
- `apps/web/components/task/chat/*`
- `apps/web/components/task/mobile/*`
- `apps/web/lib/query/query-options/session.ts`
- `apps/web/lib/query/bridge/session.ts`
- `apps/web/lib/query/bridge/session-state.ts`
- `apps/web/lib/ws/handlers/{messages,turns,task-plans,agent-session}.ts`
- `apps/web/lib/state/slices/session/*`

## Dependencies

- Tasks 03 and 04.

## Inputs

- Old PR `query-options/session.ts`, `bridge/session.ts`,
  `bridge/session-state.ts`.
- Current gap tests under `apps/web/e2e/tests/chat/*message-add-ws-gap.spec.ts`.

## Output Contract

Update this task to `done`, summarize chat/message migration, and list any
session UI state intentionally left in Zustand.

## Implementation Notes

- Migrated task session lists, session-by-id lookups, latest/page message reads,
  turns, active turn state, queue status, task plan detail, plan revisions, and
  revision content to TanStack Query readers.
- Added session query keys/options for task plans, plan revisions, queue status,
  session message pages with `before`/`after` cursors, and turns with
  query-owned `activeTurnId`.
- Added session bridge handlers for `session.message.*`, `session.turn.*`,
  `session.state_changed`, `message.queue.status_changed`, and `task.plan.*`
  events so the cache keys mounted UI reads are patched or invalidated directly.
- Removed durable session/chat/plan/queue events from the bridge audit skip list;
  control-plane request/response events remain intentionally skipped.
- Moved chat/session hooks and plan-preview/composer helpers away from direct
  domain API reads. Explicit recovery/backfill paths still force fresh query
  fetches with `staleTime: 0` where stale cached snapshots would hide missed
  messages.
- Tightened the shared E2E `SessionPage` send helper to wait for TipTap
  `contenteditable="true"` before filling. The idle placeholder can render just
  before the editor editable effect commits on mobile, and the Docker mobile
  rerun passed after that readiness wait.
- Retained Zustand for client-only session UI and compatibility mirrors until
  Task 10: active task/session ids, chat drafts/input/context state,
  layout/panel state, and mirrored task session/message/turn/task plan/queue
  server snapshots for older readers that are removed later in the one-shot PR.
