---
spec: docs/specs/ui/requirements/sidebar-queued-prompt-count.md
created: 2026-08-04
status: implemented
---

# Implementation Plan: Sidebar Queued Prompt Count Badge

## Overview

Show, on every task and subtask row in the left task view, a mail-icon badge
with the number of prompts currently en-queued for that task (all sessions,
pending semantics identical to `message.queue.get`). The count rides the
existing per-task status summary: computed fresh at list/snapshot assembly for
initial load, and refreshed live by the status-summary projector subscribing
to the internal queue-status event and reusing the workspace-wide
`task.status_summary.updated` broadcast. Zero renders nothing.

## Confirmed Current Behavior

- The queue is per-session in `queued_messages` (SQLite table created in
  `internal/orchestrator/messagequeue/repository_sqlite.go:initSchema`); each
  row carries a denormalized `task_id` written at insert. Pending = any row
  except durable lifecycle rows marked `lifecycle_reserved_in_flight`
  (`Service.GetStatus` filters in memory via `QueuedMessage.IsReservedInFlight`).
- Queue status reaches clients only as the session-scoped WS event
  `message.queue.status_changed` (`{session_id, entries, count, max}`), routed
  by `TaskEventBroadcaster` via `BroadcastToSession`. The payload has no
  `task_id`, and the event never reaches task-list clients for non-focused
  sessions.
- The sidebar (`TaskSessionSidebar` → `TaskSwitcher` → `TaskItem` →
  `TaskItemStatsRow`) consumes `KanbanTask.statusSummary` (type
  `TaskStatusSummary`, snake_case JSON), populated from the HTTP workflow
  snapshot and kept live by `task.status_summary.updated` (workspace-wide).
- No per-task queued count exists anywhere. There is no index on
  `queued_messages.task_id`; the table is per-install small (bounded by
  `max_per_session` per session), so a batched scan per list assembly is
  acceptable.

## Behavioral Contract

- Badge = mail icon + pending prompt count on the row metadata line; hidden at
  0; same treatment for subtasks and tasks.
- Count = pending `queued_messages` rows with `task_id` = task (excluding
  reserved-in-flight), across all of the task's sessions.
- Initial list/snapshot loads are authoritative (fresh count at assembly).
- Live updates flow through the existing status-summary broadcast; no new WS
  action, no workspace-wide leak of queue contents.
- `message.queue.status_changed` keeps its session-scoped routing; its payload
  gains an informational `task_id`.

## Backend Design

### Pending count query

- Add `CountPendingByTaskIDs(ctx, taskIDs []string) (map[string]int, error)`
  to `messagequeue.Repository` (interface, SQLite, memory). SQLite: one
  `SELECT task_id, metadata_json FROM queued_messages WHERE task_id IN (...)` +
  in-memory reserved filter (reuse `scanQueuedRow`/`IsReservedInFlight`
  semantics; do not filter JSON in SQL). Memory: iterate per-session lists.
- Add `Service.CountPendingByTaskIDs` and single-task
  `Service.CountPendingByTask(ctx, taskID) (int, error)` delegating to it.
  Export a narrow `QueuedPromptCounter`-style adapter at the composition root
  (backendapp) so neither the task service nor the status-summary projector
  imports `messagequeue`.

### Status summary live projection

- `TaskStatusSummary` gains `QueuedPromptCount int \`json:"queued_prompt_count,omitempty"\``;
  `Validate` rejects negatives; `SemanticJSON`/`semanticPayload` include it
  (persistence round-trip); `SemanticEqual` picks it up via `reflect.DeepEqual`.
- Projector: add `CountQueuedPrompts func(ctx, taskID) (int, error)` to
  `ProjectorConfig`; subscribe to `events.MessageQueueStatusChanged`; add
  `queuedCount` to `projectionState`; new `applyQueueStatusEventLocked` case
  that queries the count and returns changed only on delta; restore the count
  in `restorePersistedState`; include it in `deriveSummary`.
- Publish sites add `task_id` to the internal event payload:
  - `orchestrator/event_handlers_agent.go:publishQueueStatusEvent` — resolve
    session→task via a new small `SessionTaskID(ctx, sessionID)` helper on the
    orchestrator service (its repo owns sessions).
  - `orchestrator/handlers/queue_handlers.go:publishStatus` — resolve via an
    injected `SessionTaskResolver` (wire `orchestratorSvc.SessionTaskID` at
    `backendapp/gateway.go:155`). On resolution failure, log and publish
    without `task_id` (projector skips; next event or list load reconciles).

### List/snapshot assembly

- Task service: add optional `QueuedPromptCounter` interface + `SetQueuedPromptCounter`
  (pattern of `ForegroundActivityProvider`); batch method
  `CountPendingQueuedByTaskIDs`.
- `buildTaskDTOsWithSessionInfo` (all list/snapshot paths funnel through it:
  HTTP list, workspace list, single task, WS list, workflow/workspace
  snapshot): load counts for the listed tasks and stamp
  `taskDTO.StatusSummary.QueuedPromptCount` (only when a summary exists — never
  fabricate a summary, because `sidebarSessionStatus` treats a non-nil summary
  as authoritative for other fields).
- `HydrateMissingTaskStatusSummaries`: include the count in the rebuilt
  summaries for tasks it repairs (extend `statussummary.RebuildInput`).
- backendapp wires the adapter into the task service and the projector config.

## Frontend Design

- `lib/types/task-status-summary.ts`: add `queued_prompt_count?: number`.
  (`toKanbanTask` already carries `status_summary` wholesale; WS summary
  updates replace it wholesale, so no mapper change is needed.)
- `buildSidebarItem`: `queuedCount: summary?.queued_prompt_count` on the item.
- `TaskSwitcherItem`: new `queuedCount?: number`; `TaskRow` forwards it.
- `TaskItem`/`TaskItemStatsRow`: render
  `IconMail` + count when `queuedCount > 0`; include it in the row's
  early-return condition; `data-testid="sidebar-task-queued-count"`; accessible
  name via `t("sidebar:queuedPromptCount", { count })`
  (`_one`/`_other` plurals in `apps/web/src/locales/en/sidebar.json` +
  pseudo-locale). Badge is inline-flex, shrink-safe, non-interactive.
- i18n ratchet: new copy uses `t()` (sidebar namespace); do not migrate
  untouched literals in the file.

## Responsive and Mobile Contract

The badge is a static inline-flex text+icon pair inside the existing metadata
row. No hover dependency, no hit target, no new scroll owner; row overflow
behavior is unchanged (title truncates, badge does not wrap). Desktop and
mobile render identically because both use `TaskItem`. Mobile E2E asserts the
badge through the mobile sidebar flow.

## TDD and Verification Strategy

1. Backend repo/service tests for the count query (multi-session accumulation,
   reserved-in-flight exclusion, empty, batch). RED → implement → GREEN.
   `cd apps/backend && go test ./internal/orchestrator/messagequeue/...`
2. Model + projector tests: field round-trip, negative validation, queue event
   updates count, no-change suppresses publish, restore keeps count. RED →
   implement → GREEN.
   `cd apps/backend && go test ./internal/task/statussummary/...`
3. DTO assembly + hydration tests (count stamped on existing summaries,
   included in repaired summaries, absent when no summary). RED → implement →
   GREEN. `cd apps/backend && go test ./internal/task/...`
4. Frontend unit tests: `buildSidebarItem` mapping, `TaskItem` badge
   render/hide, plural/aria. RED → implement → GREEN.
   `cd apps && pnpm --filter @kandev/web exec vitest run components/task/task-item.test.tsx components/task/task-session-sidebar-item.test.ts`
5. Full gates: `make -C apps/backend test lint build`, then from `apps`:
   web typecheck, lint, `i18n:check`, `i18n:ratchet`.
6. E2E (desktop + mobile) as Task 05:
   `cd apps/web && pnpm e2e -- tests/task/sidebar-queued-count.spec.ts tests/task/mobile-sidebar-queued-count.spec.ts`,
   preserving/restoring queue state in teardown.

## Implementation Waves

- [x] [Task 01: Add pending prompt count queries to the message queue](task-01-pending-prompt-count-queries.md) — wave 1
- [x] [Task 02: Add queued_prompt_count to the live status summary](task-02-status-summary-queued-count.md) — wave 2, after Task 01
- [x] [Task 03: Attach the count to task list/snapshot payloads](task-03-attach-count-to-task-payloads.md) — wave 3, after Task 02
- [x] [Task 04: Render the badge in the task sidebar](task-04-sidebar-badge-ui.md) — wave 3, after Task 02 (file-disjoint from Task 03; parallel-safe)
- [x] [Task 05: E2E coverage and verification](task-05-e2e-and-verification.md) — wave 4, after Tasks 03 and 04

This package does not authorize subagents; implementation remains in the
user-controlled primary session.

## Results

All five tasks implemented and verified: backend package tests, vet and golangci-lint on touched packages
green; 50 frontend unit tests plus typecheck/lint/i18n gates green; E2E desktop + mobile 4/4 on the built
artifact (badge visible, live clear, hidden at 0, subtasks). Full per-task command results are recorded in
each task file's Results section. Pre-existing VM-environment test failures (launcher/agentctl/local-repo)
reproduce at HEAD and are unrelated.

## Environment Prerequisite

If `apps/node_modules` is absent, run `pnpm install --frozen-lockfile` from
`apps/` before frontend RED tests. Do not change the lockfile.

## Risks and Boundaries

- Fabricating a status summary for a task that has none would flip
  `sidebarSessionStatus` to summary-authoritative and hide the relative time;
  assembly must stamp only existing summaries and let hydration create the
  rest.
- JSON-in-SQL filtering of the reserved marker is fragile; keep the reserved
  exclusion in Go (mirrors `GetStatus`).
- `task_id` has no index; list assembly does one batched scan of a bounded
  table. If queue volumes ever grow, add a replayable migration for an index.
- WIP overflow queues (`queued_for_step_id`) are unrelated and out of scope.
- The projector may briefly lag on startup; fresh assembly at list time is the
  correctness backstop, and the next queue event repairs the live path.
