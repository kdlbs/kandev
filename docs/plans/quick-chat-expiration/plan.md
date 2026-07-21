---
spec: docs/specs/tasks/quick-chat-expiration.md
created: 2026-07-03
status: implemented
---

# Implementation Plan: Quick Chat Persistence & Expiration

## Overview

Restore quick-chat tabs from backend state during SPA boot, then add a backend-owned idle sweeper that deletes only true quick-chat tasks after 7 days of inactivity. The repository owns candidate selection because the filter depends on task metadata, task origin, workflow absence, active session state, and last activity across task/session timestamps. The service layer owns deletion so expiration reuses `Service.DeleteTask` and preserves existing cleanup and `task.deleted` behavior.

---

## Backend

### Quick-chat candidate query

Files:

- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/task_repository_test.go`
- `apps/backend/internal/db/dialect/time.go` and `dialect_test.go` if a small `GreatestTimestamp` helper is needed

Add a task repository method:

```go
ListExpiredQuickChatTasks(ctx context.Context, cutoff time.Time) ([]*models.Task, error)
```

The SQLite query must return only rows where:

- `t.is_ephemeral = 1`
- `COALESCE(t.workflow_id, '') = ''`
- `COALESCE(t.origin, '') != models.TaskOriginAutomationRun`
- `excludeConfigModePredicate(driver, "t.metadata")`
- `t.archived_at IS NULL`
- no session for the task has state in `('CREATED', 'STARTING', 'RUNNING', 'WAITING_FOR_INPUT')`
- dialect-aware `last_activity = max/greatest(t.updated_at, COALESCE(MAX(ts.updated_at), t.updated_at)) < cutoff`

Order by computed last activity ascending for deterministic deletion and tests.

### Expiration service

Files:

- New `apps/backend/internal/task/service/quick_chat_expiration.go`
- `apps/backend/internal/task/service/service_test.go`
- `apps/backend/internal/backendapp/main.go`

Add constants:

```go
const quickChatIdleRetention = 7 * 24 * time.Hour
const quickChatExpirationInterval = 24 * time.Hour
```

Add service methods:

```go
func (s *Service) StartQuickChatExpirationLoop(ctx context.Context)
func (s *Service) runQuickChatExpiration(ctx context.Context, now time.Time)
```

`runQuickChatExpiration` computes `cutoff := now.Add(-quickChatIdleRetention)`, calls `ListExpiredQuickChatTasks`, logs and returns without deleting on list errors, then iterates candidates and calls `s.DeleteTask(ctx, task.ID)`. A delete failure logs the task ID/error and continues with the rest.

Wire `services.Task.StartQuickChatExpirationLoop(ctx)` next to `StartAutoArchiveLoop(ctx)` in `apps/backend/internal/backendapp/main.go`.

### Boot-state quick-chat hydration

Files:

- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/backendapp/boot_state_routes.go`
- `apps/backend/internal/backendapp/helpers_test.go`
- `apps/web/lib/api/domains/workspace-api.ts`
- `apps/web/app/page.tsx`
- `apps/web/lib/state/hydration/hydrator.test.ts`

Add a boot-state helper that populates:

```json
"quickChat": {
  "isOpen": false,
  "sessions": [
    {
      "sessionId": "...",
      "workspaceId": "...",
      "name": "...",
      "agentProfileId": "..."
    }
  ],
  "activeSessionId": null
}
```

The helper should:

- resolve the active workspace from already-built boot state when possible, otherwise from query/cookie/user settings/first workspace using existing helpers;
- fetch all pages of `ListTasksByWorkspace(ctx, workspaceID, "", "", "", page, pageSize, false, false, true, true)` so there is no count cap;
- filter in Go to `workflow_id == ""`, `origin != automation_run`, and `primary_session_id != nil`;
- use `BatchGetSessionsForTasks` and `GetPrimarySessionInfoForTasks` through `taskDTOsWithSessionInfo` or a small shared helper rather than adding a new endpoint;
- compute last activity as `max(task.UpdatedAt, max(session.UpdatedAt))` and sort restored tabs newest first;
- derive `agentProfileId` from `task.Metadata[models.MetaKeyAgentProfileID]`;
- preserve local display-name overrides through the existing frontend `hydrateUI` quick-chat name overlay.

Call this helper from SPA boot-state assembly so quick chats restore on `/`, `/tasks`, task-detail, local context routes (`/github`, `/gitlab`, `/jira`, `/linear`, `/stats`), `/office`, and settings routes when an active workspace can be resolved.

Update the legacy `listQuickChatSessions` response typing and `app/page.tsx` mapping to include `origin`, `updated_at`, and the same automation-run filtering so stale Next-compatible code stays type-correct.

---

## Frontend

No new visible controls are required. The frontend behavior change is boot-time state hydration: `QuickChatProvider` already mounts globally, and `hydrateUI` already overlays local quick-chat names from `localStorage`.

State and tests:

- Keep `QuickChatState` in `apps/web/lib/state/slices/ui/types.ts` unchanged unless the boot payload needs an explicit helper type.
- Extend `apps/web/lib/state/hydration/hydrator.test.ts` if needed to assert that hydrated sessions replace the in-memory list, preserve local names, and clear the modal when the backend returns no sessions.
- Add or update `apps/web/src/boot-payload.test.ts` only if TypeScript parsing/shape guards need to understand a narrower quick-chat payload.

---

## Tests

- **What:** repository expiration query includes quick chats idle longer than cutoff.
  **File:** `apps/backend/internal/task/repository/task_repository_test.go`
  **How:** SQLite integration test with backdated `tasks.updated_at` and `task_sessions.updated_at`.

- **What:** any new timestamp SQL dialect helper emits SQLite and Postgres fragments correctly.
  **File:** `apps/backend/internal/db/dialect/dialect_test.go`
  **How:** unit test for the helper only if Task 01 adds one.

- **What:** repository query excludes active sessions, config-mode chats, automation-run ephemeral tasks, non-ephemeral tasks, archived tasks, and ephemeral tasks with a workflow.
  **File:** `apps/backend/internal/task/repository/task_repository_test.go`
  **How:** table-driven SQLite test seeding each row variant.

- **What:** session activity keeps an old quick-chat task alive.
  **File:** `apps/backend/internal/task/repository/task_repository_test.go`
  **How:** set `tasks.updated_at` older than 7 days and primary `task_sessions.updated_at` newer than cutoff; assert no candidate.

- **What:** expiration uses the service delete path, publishes `task.deleted`, and cleans quick-chat directories through existing async cleanup.
  **File:** `apps/backend/internal/task/service/service_test.go`
  **How:** service test with real SQLite repo, `SetQuickChatDir`, `cleanupDoneForTest`, and event-bus assertions.

- **What:** expiration fails closed on candidate-list error and continues when a single delete fails.
  **File:** `apps/backend/internal/task/service/quick_chat_expiration_test.go` or `service_test.go`
  **How:** use a small fake repository/service seam if the real repo cannot simulate the list/delete errors cleanly.

- **What:** boot payload contains quick-chat sessions ordered by last activity, excluding automation/config tasks.
  **File:** `apps/backend/internal/backendapp/helpers_test.go`
  **How:** use `newBootStateTestHarness`, create multiple ephemeral tasks/sessions, backdate rows with SQL, call `bootPayload`, and unmarshal `initialState.quickChat.sessions`.

- **What:** frontend hydration overlays local quick-chat names onto backend-restored sessions.
  **File:** `apps/web/lib/state/hydration/hydrator.test.ts`
  **How:** existing jsdom localStorage tests extended for multiple restored sessions.

---

## E2E Tests

- **Scenario:** GIVEN an open quick chat with a message, WHEN the user reloads the page, THEN the quick-chat tab restores and prior history is visible.
  **File:** `apps/web/e2e/tests/chat/quick-chat.spec.ts`
  **What to verify:** create a quick chat with `openQuickChatWithAgent`, send a mock-agent message, reload, reopen quick chat with the shortcut, assert the same response text appears without selecting a new agent.

- **Scenario:** GIVEN multiple quick-chat tabs, WHEN the user reloads, THEN all tabs restore.
  **File:** `apps/web/e2e/tests/chat/quick-chat.spec.ts`
  **What to verify:** extend the existing multi-tab test or add a focused test that counts restored tab buttons after reload.

Backend expiration is covered by Go integration tests rather than E2E because it has no new visible controls and depends on a 7-day idle clock.

---

## Implementation Waves

Wave 1:

- [x] [task-01-backend-candidate-query](task-01-backend-candidate-query.md)
- [x] [task-03-boot-hydration](task-03-boot-hydration.md)

Wave 2:

- [x] [task-02-backend-expiration-service](task-02-backend-expiration-service.md)

Wave 3:

- [ ] [task-04-e2e-and-verification](task-04-e2e-and-verification.md)

---

## Open Questions

- Confirm during implementation whether quick-chat messages update `tasks.updated_at`, `task_sessions.updated_at`, or only message/turn rows. The planned candidate query and hydration sort use `MAX(task.updated_at, session.updated_at)`, so the feature remains correct as long as at least one of those rows moves on activity.
