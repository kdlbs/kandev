---
spec: docs/specs/ui/tanstack-query-server-state.md
created: 2026-06-23
status: draft
---

# Implementation Plan: TanStack Query Server State

## Overview

Migrate the web frontend's server state from Zustand/fetch-effect ownership to
TanStack Query in one PR. The old PR #1130 provides the useful shape: typed
query keys, domain query options, WS -> query bridges, and E2E WebSocket
accounting. This plan adapts that path to the current Go-served Vite SPA:
`src/main.tsx`, `StateProvider`, `StateHydrator`, `src/*-routes.tsx`, and the
Go boot payload are the hydration boundary now, not Next layout/server
components.

The PR should land as one branch, but implementation is split into waves so each
domain can be tested and reviewed before removing the old Zustand server-state
handlers.

---

## Backend

### WebSocket Envelope Accounting

Files:

- `apps/backend/pkg/websocket/message.go`
- `apps/backend/internal/gateway/websocket/client.go`
- `apps/backend/internal/gateway/websocket/hub.go`
- `apps/backend/internal/gateway/websocket/hub_session_mode.go`
- new `apps/backend/internal/gateway/websocket/ws_sent_log.go`
- new/updated tests under `apps/backend/internal/gateway/websocket/*_test.go`
- E2E test endpoint in `apps/backend/cmd/kandev/e2e_reset.go` or adjacent test
  harness route file

Changes:

- Add `connection_id`, `connection_seq`, and optional `session_seq` to outbound
  WS messages.
- Stamp messages at the final send boundary so every client receives its own
  monotonic connection sequence.
- Maintain per-session sequence counters for session-scoped broadcasts.
- Record stamped envelopes in a bounded backend sent-log ring buffer.
- Add an E2E-only sent-log endpoint gated by the existing E2E mock/test harness.
- Keep the current session-focus/focused-recipient semantics.

Reason:

The current runner sets `KANDEV_E2E_WS_ASSERT=1`, but this checkout has no
sent-log, frontend `WsAccount`, or sequence fields. The docs and runner need a
real implementation before strict mode can mean anything.

---

## Frontend

### Query Foundation

Files:

- `apps/web/package.json`
- `apps/pnpm-lock.yaml`
- new `apps/web/lib/query/client.ts`
- new `apps/web/lib/query/provider.tsx`
- new `apps/web/lib/query/keys.ts`
- new `apps/web/lib/query/query-options/*`
- `apps/web/src/main.tsx`
- `apps/web/components/state-provider.tsx`
- `apps/web/components/state-hydrator.tsx`
- `apps/web/src/boot-payload.ts`
- `apps/web/src/spa-routes.tsx`
- `apps/web/src/office-routes.tsx`
- `apps/web/src/task-detail-route.tsx`

Changes:

- Add `@tanstack/react-query`, `@tanstack/react-query-devtools`, and
  `@tanstack/eslint-plugin-query` at the current registry version observed on
  2026-06-23: `5.101.1`.
- Create a browser-singleton `QueryClient` with Kandev defaults:
  `staleTime: 30_000`, `gcTime: 5 * 60_000`, no global focus/reconnect refetch,
  auth errors not retried, and non-idempotent mutations not retried.
- Wrap the SPA root in `QueryProvider` from `src/main.tsx`.
- Expose `window.__KANDEV_E2E_QUERY_CLIENT__` only when
  `__KANDEV_E2E_EXPOSE_STORE__` is set.
- Add helpers to seed query data from existing boot payload/app-state route data
  and from `StateHydrator` calls.

Reason:

The old PR wired React Query through `app/layout.tsx` and Next-style hydration.
The current app boots through `src/main.tsx` after `loadBootPayload()`, so query
hydration must integrate with that boundary.

### Query Keys And Query Options

Files:

- `apps/web/lib/query/keys.ts`
- `apps/web/lib/query/query-options/*.ts`
- existing API clients under `apps/web/lib/api/domains/*`
- focused unit tests under `apps/web/lib/query/**`

Domains:

- features
- workspace/repositories/branches/scripts
- kanban/workflows/tasks
- session/task sessions/messages/turns/plans/queue
- session runtime/git/status/prepare/context/commands/models/prompt usage
- office/dashboard/tasks/agents/projects/inbox/activity/runs/routing/costs/skills
- settings/executors/agents/editors/prompts/secrets/sprites/user settings
- integrations shell plus Jira, Linear, GitHub, GitLab, Slack/Sentry as present
- automations/system data where currently fetched into Zustand

Rules:

- Every query has a typed key factory and a query option factory.
- Infinite/paginated resources use `useInfiniteQuery` and keep cursor fields out
  of the stable filter key.
- Presentational sort/grouping stays out of fetch keys unless the backend query
  itself changes.
- Session message/turn queries use longer stale windows and explicit recovery
  invalidation so live streams are not clobbered.

### WebSocket Query Bridge

Files:

- `apps/web/lib/ws/connection.ts`
- `apps/web/lib/ws/client.ts`
- `apps/web/lib/ws/router.ts`
- new `apps/web/lib/query/bridge/index.ts`
- new `apps/web/lib/query/bridge/<domain>.ts`
- new bridge tests under `apps/web/lib/query/bridge/**`

Changes:

- Add `subscribeWebSocketClient(listener)` so QueryBridge can register even when
  it mounts before the WS client exists.
- Register a query bridge beside or instead of existing Zustand handlers.
- Wrap every bridge handler in `wrapBridgeHandler` for E2E audit.
- Export `BRIDGE_SKIPPED_ACTIONS` and `BRIDGE_SKIPPED_PREFIXES` with inline
  rationale for control-plane responses, client-only handlers, and high-volume
  streams.
- Remove each old Zustand WS handler only after all readers for that domain
  read TanStack Query.

Reason:

PR #1130's own Phase 2 notes found the key failure mode: "bridge wrote, UI did
not read." This plan requires UI readers and bridge writers to converge per
domain before the old path is removed.

### E2E WebSocket Accounting

Files:

- new `apps/web/lib/ws/ws-account.ts`
- new `apps/web/lib/ws/ws-account.test.ts`
- new `apps/web/e2e/helpers/ws-account.ts`
- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/scripts/run-e2e.sh`
- new `apps/web/e2e/tests/system/ws-event-accounting.spec.ts`

Changes:

- Record parsed WS envelopes at `WebSocketClient.handleParsedMessage` before
  response/error/notification dispatch.
- Track both connection-wide and per-session sequence windows.
- At test teardown, compare backend sent-log entries with frontend parsed
  entries when `KANDEV_E2E_WS_ASSERT=1`.
- Add bridge audit comparison for parsed server-state events with `session_id`
  or `task_id`.
- Keep the existing targeted `routeWebSocket` gap tests; they validate recovery
  from an intentionally dropped `session.message.added` frame.

---

## Tests

### Unit Tests

- **What:** query key stability and serializability
  **File:** `apps/web/lib/query/keys.test.ts`
  **How:** table-driven Vitest cases for representative keys and partial
  invalidation prefixes.

- **What:** query option factories map API responses without redundant fetches
  **File:** `apps/web/lib/query/query-options/*.test.ts`
  **How:** mocked domain API functions plus `queryClient.fetchQuery`.

- **What:** WS bridge handlers patch/invalidate correct keys
  **File:** `apps/web/lib/query/bridge/**.test.ts`
  **How:** create `QueryClient`, register bridge handler with fake WS client,
  emit payloads, assert query cache changes and bridge audit rows.

- **What:** session message merge/backfill preserves live WS messages and
  synthetic empty-turn notices
  **File:** `apps/web/lib/query/query-options/session.test.ts`
  **How:** unit tests for merge helpers and query function behavior.

- **What:** office paginated task queries keep filter/cursor semantics
  **File:** `apps/web/lib/query/query-options/office.test.ts`
  **How:** infinite-query tests for first page, next page, filter reset, and WS
  invalidation against every filter entry.

- **What:** backend WS sequence stamping and sent-log eviction
  **File:** `apps/backend/internal/gateway/websocket/*_test.go`
  **How:** Go unit tests for per-client connection seq, per-session seq, clone
  isolation, ring eviction, and unsubscribe cleanup.

- **What:** frontend `WsAccount` detects connection and session gaps
  **File:** `apps/web/lib/ws/ws-account.test.ts`
  **How:** feed ordered, missing, duplicated, and mixed-session envelopes.

### Integration Tests

- **What:** boot payload seeds query cache before child hooks fetch
  **File:** `apps/web/src/boot-payload.test.ts`, `apps/web/src/spa-routing.test.ts`
  **How:** mount route under `QueryProvider`, assert query data exists and API
  fetch mocks are not called when payload includes route data.

- **What:** `StateHydrator` seeds query cache on task-detail route transitions
  **File:** `apps/web/components/state-hydrator.test.tsx` or route-specific
  tests
  **How:** render with initial state and assert session/task query keys are
  populated before dependent hook assertions.

- **What:** old Zustand server-state handlers are removed safely
  **File:** existing `apps/web/lib/ws/handlers/*.test.ts` migrated to bridge
  tests
  **How:** move assertions from store mutation to query cache mutation before
  deleting the old handler.

---

## E2E Tests

Run all migration E2E gates from `apps/web`.

For normal browser E2E, use `pnpm e2e:docker ...`, not bare
`pnpm e2e:run ...`, so local validation uses the CI runtime image and cannot
silently fall back to host mode. The managed Docker runner enables
`KANDEV_E2E_WS_ASSERT=1` by default.

For Docker/SSH executor coverage, use the Playwright `containers` project with
host Docker access:

```bash
KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers
```

Do not run the `containers` project through `pnpm e2e:docker`; those specs need
to control the host Docker daemon.

- **Scenario:** boot-hydrated task page renders messages and sessions with no
  spinner-only first paint
  **File:** `apps/web/e2e/tests/session/session-hydration.spec.ts`
  **What to verify:** route loads, chat messages visible, no manual refresh.

- **Scenario:** missed `session.message.added` for a sent prompt still renders
  accepted prompt
  **File:** existing `apps/web/e2e/tests/chat/message-add-ws-gap.spec.ts` and
  `apps/web/e2e/tests/chat/mobile-message-add-ws-gap.spec.ts`
  **What to verify:** keep both desktop and mobile tests green after migration.

- **Scenario:** office dashboard updates from WS/query invalidation
  **File:** existing office realtime specs plus
  `apps/web/e2e/tests/office/realtime-dashboard.spec.ts`
  **What to verify:** dashboard metric/activity changes without reload.

- **Scenario:** office tasks pagination survives WS updates
  **File:** `apps/web/e2e/tests/office/realtime-tasks.spec.ts`
  **What to verify:** filters remain selected, loaded pages are not duplicated,
  changed task appears.

- **Scenario:** strict WS accounting detects no receipt or bridge gaps
  **File:** `apps/web/e2e/tests/system/ws-event-accounting.spec.ts`
  **What to verify:** two concurrent sessions produce no connection/session
  gaps and at least one bridged query cache mutation per relevant event.

- **Scenario:** mobile parity for migrated chat/task/office paths
  **File:** existing `mobile-*.spec.ts` files and targeted additions only where
  a migrated workflow lacks mobile coverage
  **What to verify:** touch-accessible controls and same data freshness.

---

## E2E Wave Gates

Each wave is incomplete until its Docker-backed E2E gate has run locally and the
task output records the exact command, result, and artifact path for any
failure. Host-mode E2E is useful for debugging but does not satisfy these gates.

Wave 1 (foundation and WS accounting):

```bash
cd apps/web && pnpm e2e:docker -- tests/system/ws-event-accounting.spec.ts tests/chat/message-add-ws-gap.spec.ts tests/task/task-list.spec.ts tests/kanban/kanban-board.spec.ts
cd apps/web && pnpm e2e:docker --no-build --project mobile-chrome -- tests/chat/mobile-message-add-ws-gap.spec.ts
```

Wave 2 (query taxonomy and WS bridge):

```bash
cd apps/web && pnpm e2e:docker tests/system/ws-event-accounting.spec.ts tests/chat/message-add-ws-gap.spec.ts
cd apps/web && pnpm e2e:docker --project mobile-chrome tests/chat/mobile-message-add-ws-gap.spec.ts
```

Wave 3 (domain migrations):

```bash
cd apps/web && pnpm e2e:docker tests/task/task-list.spec.ts tests/task/task-list-filters.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/workflow-filter.spec.ts tests/settings/config-management.spec.ts
cd apps/web && pnpm e2e:docker tests/office/realtime-dashboard.spec.ts tests/office/realtime-tasks.spec.ts tests/office/dashboard.spec.ts tests/office/tasks.spec.ts
cd apps/web && pnpm e2e:docker tests/chat/message-add-ws-gap.spec.ts tests/chat/message-pagination.spec.ts tests/session/session-tab-management.spec.ts tests/session/session-recovery.spec.ts
cd apps/web && pnpm e2e:docker tests/terminal/terminal-hanging-on-boot.spec.ts tests/terminal/terminal-dockview-ui.spec.ts tests/git/git-changes-panel.spec.ts
cd apps/web && pnpm e2e:docker tests/github tests/integrations tests/system/status-page.spec.ts tests/system/database-page.spec.ts
cd apps/web && pnpm e2e:docker --project mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/task/mobile-task-list-search.spec.ts tests/chat/mobile-message-add-ws-gap.spec.ts tests/office/mobile-onboarding.spec.ts tests/github/mobile-github-sidebar.spec.ts tests/integrations/mobile-linear-watcher-profile.spec.ts tests/terminal/mobile-terminal-keybar.spec.ts
cd apps/web && pnpm e2e:docker --project routing
```

Wave 4 (cleanup and full regression):

```bash
cd apps/web && pnpm e2e:docker --shards 3
cd apps/web && pnpm e2e:docker --project mobile-chrome
cd apps/web && pnpm e2e:docker --project routing
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e --project=containers
```

If Docker is unavailable, the wave is blocked rather than waived. Record the
Docker failure and run host-mode E2E only as diagnostic evidence while fixing
the local Docker/runtime issue.

---

## Implementation Waves

Wave 1 (foundation):

- [x] [task-01-query-foundation](task-01-query-foundation.md)
- [x] [task-02-ws-accounting](task-02-ws-accounting.md)

Wave 2 (cache taxonomy and bridge):

- [x] [task-03-query-options-taxonomy](task-03-query-options-taxonomy.md)
- [x] [task-04-query-bridge-audit](task-04-query-bridge-audit.md)

Wave 3 (domain migrations, can run in parallel by area after Wave 2):

- [x] [task-05-workspace-kanban-settings](task-05-workspace-kanban-settings.md)
- [x] [task-06-office-domain](task-06-office-domain.md)
- [x] [task-07-session-domain](task-07-session-domain.md)
- [x] [task-08-session-runtime-streams](task-08-session-runtime-streams.md)
- [x] [task-09-integrations-automations-system](task-09-integrations-automations-system.md)

Task 05 verification completed locally:

- `cd apps && pnpm --filter @kandev/web test -- hooks src lib/query` passed
  89 files / 672 tests.
- `cd apps/web && pnpm typecheck` passed.
- `cd apps/web && pnpm e2e:docker --no-build -- tests/task/task-list.spec.ts tests/task/task-list-filters.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/workflow-filter.spec.ts tests/settings/config-management.spec.ts`
  passed 35 desktop tests.
- `cd apps/web && pnpm e2e:docker --no-build --project mobile-chrome -- tests/kanban/mobile-kanban.spec.ts tests/task/mobile-task-list-search.spec.ts`
  passed 12 mobile tests.

Task 06 verification completed locally:

- `cd apps && pnpm --filter @kandev/web test -- app/office hooks/domains/office lib/query`
  passed 25 files / 124 tests.
- `cd apps/web && pnpm typecheck` passed.
- `cd apps/web && pnpm e2e:docker -- tests/office/realtime-dashboard.spec.ts tests/office/realtime-tasks.spec.ts tests/office/dashboard.spec.ts tests/office/tasks.spec.ts`
  passed 21 desktop Docker tests.
- `cd apps/web && pnpm e2e:docker -- --project=mobile-chrome tests/office/mobile-onboarding.spec.ts`
  passed 6 mobile Docker tests.
- `cd apps/web && e2e/scripts/run-e2e.sh --docker --no-build --project routing`
  passed 7 routing Docker tests.
- Final reopened Office cleanup also passed:
  - `rtk pnpm --dir apps/web test hooks/domains/office/use-office-data.test.tsx app/office/agents/[id]/components/agent-configuration-tab.test.tsx app/office/agents/[id]/components/agent-runs-tab.test.tsx app/office/components/new-task-dialog.test.tsx app/office/workspace/org/org-tree-layout.test.ts app/office/page-client.test.tsx lib/query/seed.test.ts components/state-hydrator.test.tsx lib/query/bridge/index.test.ts components/task/simple/components/pending-approval-badge.test.tsx`
    passed 10 files / 68 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web lint` passed.
  - Stale scans for removed Office store fields/actions returned no production
    server-state readers/writers; remaining `office.tasks.*` matches are
    client-only task filter/sort/view/grouping/nesting state.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/agents.spec.ts tests/office/agent-subroutes.spec.ts tests/office/agent-roles.spec.ts tests/office/agent-skills-ui.spec.ts tests/office/permissions.spec.ts tests/office/projects.spec.ts tests/office/project-repository-picker.spec.ts tests/office/routines.spec.ts tests/office/routines-ui.spec.ts tests/office/routine-fire.spec.ts tests/office/skills.spec.ts tests/office/system-skills.spec.ts tests/office/skills-readonly.spec.ts tests/office/org-chart.spec.ts tests/office/execution-stages.spec.ts tests/office/costs.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 67 Docker tests / 5 skipped with strict WS accounting.

Task 07 verification completed locally:

- `cd apps && pnpm --filter @kandev/web test -- lib/query/keys.test.ts lib/query/seed.test.ts lib/query/query-options/query-options.test.ts lib/query/bridge/index.test.ts hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-session-search.test.ts hooks/domains/session/use-session-state.test.ts hooks/domains/session/use-session-actions.test.ts hooks/domains/session/use-ensure-task-session.test.ts components/task/chat/message-list-shared.test.tsx components/task/chat/queued-ghost-list.test.tsx hooks/use-plan-panel-auto-open.test.ts hooks/use-task-removal.test.ts components/task/passthrough-chat-composer.test.ts`
  passed 14 files / 148 tests.
- `cd apps && pnpm --filter @kandev/web test -- hooks/domains/session components/task/chat lib/query`
  passed 58 files / 451 tests.
- `cd apps/web && pnpm typecheck` passed.
- Direct API-read scan for migrated session/task-plan/queue reads passed; only
  query-option factories and the server boot loader still call those APIs.
- `cd apps/web && pnpm e2e:docker tests/chat/message-add-ws-gap.spec.ts tests/chat/message-pagination.spec.ts tests/session/session-tab-management.spec.ts tests/session/session-recovery.spec.ts tests/chat/message-queue.spec.ts tests/task/plan-checkpointing.spec.ts tests/chat/implement-plan-fresh.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 35 desktop Docker tests.
- `cd apps/web && e2e/scripts/run-e2e.sh --docker --no-build --project mobile-chrome -- tests/chat/mobile-message-add-ws-gap.spec.ts tests/session/mobile-transient-retry.spec.ts`
  passed 2 mobile Docker tests.

Task 08 verification completed locally:

- `cd apps/web && rtk pnpm typecheck` passed.
- `cd apps && rtk pnpm --filter @kandev/web test -- hooks/domains/session components/session components/task lib/query`
  passed 153 files / 1264 tests / 4 skipped.
- `cd apps/web && rtk pnpm e2e:docker tests/terminal/terminal-hanging-on-boot.spec.ts tests/terminal/terminal-dockview-ui.spec.ts tests/git/git-changes-panel.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 30 desktop Docker tests.
- `cd apps/web && rtk e2e/scripts/run-e2e.sh --docker --no-build --project mobile-chrome -- tests/terminal/mobile-terminal-keybar.spec.ts tests/terminal/mobile-terminal-scroll.spec.ts`
  passed 16 mobile Docker tests.

Task 09 verification completed locally:

- `cd apps/web && rtk pnpm typecheck` passed.
- `cd apps && rtk pnpm --filter @kandev/web test -- hooks/domains/settings hooks/domains/system hooks/domains/github hooks/domains/gitlab hooks/domains/jira hooks/domains/linear hooks/domains/sentry hooks/domains/slack hooks/domains/integrations components/github components/gitlab components/jira components/linear components/slack components/sentry components/automations components/settings/system lib/query`
  passed 55 files / 487 tests.
- `cd apps && rtk pnpm --filter @kandev/web test -- lib/ws/ws-account-e2e-helper.test.ts lib/ws/ws-account.test.ts lib/ws/client.test.ts e2e/helpers`
  passed 3 files / 15 tests.
- `cd apps/web && rtk pnpm exec eslint --max-warnings 0 app/jira/jira-page-client.tsx app/linear/linear-page-client.tsx`
  passed.
- `cd apps/web && rtk pnpm e2e:docker tests/system/status-page.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 5 desktop Docker tests after fixing the strict WS accounting reload
  race found by the gate.
- `cd apps/web && rtk pnpm e2e:docker tests/system/ws-event-accounting.spec.ts tests/integrations/jira-settings.spec.ts tests/integrations/linear-settings.spec.ts tests/integrations/sentry-settings.spec.ts tests/integrations/github-watch-reset.spec.ts tests/integrations/jira-import.spec.ts tests/integrations/linear-import.spec.ts tests/github/pr-list-task-indicator.spec.ts tests/github/github-scope-bar.spec.ts tests/pr/ci-automation-options.spec.ts tests/automations-settings.spec.ts tests/system/status-page.spec.ts tests/system/database-page.spec.ts tests/system/disk-usage.spec.ts tests/system/backups-page.spec.ts tests/system/updates-page.spec.ts tests/system/logs-page.spec.ts`
  passed 60 desktop Docker tests / 1 skipped.
- `cd apps/web && rtk pnpm e2e:docker --project mobile-chrome tests/integrations/mobile-linear-watcher-profile.spec.ts tests/github/mobile-github-sidebar.spec.ts tests/settings/mobile-general-settings.spec.ts tests/mobile-automations-scroll.spec.ts`
  passed 4 mobile Docker tests.

Task 10 cleanup progress:

- System cleanup removed the system Zustand slice and `system-events` WS handler.
  System hooks and topbar metrics now read Query caches/options directly.
- GitHub cleanup removed server-backed GitHub Zustand fields and the old GitHub
  WS handler. Local-only `pendingPrUrlByTaskId` and `prFeedbackCache` remain.
- GitLab cleanup removed the GitLab Zustand slice entirely. Status, stats,
  workspace/task MRs, review watches, issue watches, and action presets now
  read and mutate TanStack Query caches directly. `/gitlab` and GitLab settings
  use `useGitLabStatus` instead of direct status fetch effects.
- Settings leaf-list cleanup converted prompts, secrets, sprites,
  notification providers, available agents, agent discovery, and editors hooks
  to Query-only readers. Prompts, secrets, editor mutations, agent refreshes,
  profile status panels, and prompt previews now patch/read Query caches instead
  of these Zustand mirrors.
- Settings catalog/bootstrap cleanup converted executors, settings agents,
  derived agent profiles, install jobs, settings route bootstrap, settings
  layout, and session boot preload to Query seed/query reads. Settings agent and
  executor write paths now patch `qk.settings.*` caches directly through sync
  helpers. The old `agents`, `executors`, and `executor-profiles` WS handlers
  were removed after their store readers/writers were gone. The settings
  Zustand slice now contains only `userSettings`.
- Workspace/kanban direct-fetch cleanup added a workflow-step Query key and
  query option, moved the shared `useWorkflowSteps` hook, task-create workflow
  step effect, and automations config workflow-step picker to Query, and made
  `workflow.step.*` bridge events invalidate the workflow-step cache. Unused
  workspace metadata maps (`repositories` loaded/loading flags, repository
  branch loaded/loading/fetchedAt/fetchError flags, and repository script
  loaded/loading flags) were removed from the Zustand slice, boot payloads, SSR
  session state, and old writer actions. The actual workspace, workflow,
  `kanban`, and `kanbanMulti` entity mirrors remain
  because mounted UI readers still use them as migration fallbacks. The mobile
  task switcher workspace-change path now fetches workflows and the first
  workflow snapshot through Query options instead of direct API imports. The
  task-detail route's active-task switch path now reads `taskQueryOptions`
  instead of running a component-owned `fetchTask` effect. Command-panel task
  search now reads `workspaceTasksQueryOptions` instead of importing
  `listTasksByWorkspace` directly, preserving active-task filtering and
  archived-last search ordering. Archive/delete dialog subtask counts now fetch
  through `queryClient.fetchQuery(subtaskCountQueryOptions(...))` while keeping
  the existing no-stale-count-on-reopen behavior.
- Session dead-code cleanup removed production-unused `clearTaskPlan` and
  `clearQueueStatus` Zustand actions and their action-only tests, and removed
  the legacy no-op `task.plan.reverted` registration from the old `task-plans`
  WS handler. The Query bridge remains the owner for `task.plan.reverted`.
  Session/chat/plan/queue mirrors remain because mounted readers still use
  them.
- Office task list/detail cleanup moved task rows/loading out of
  `office.tasks.items` / `office.tasks.isLoading`, removed the task page's
  SSR-to-store hydration, removed task detail's store fallback, and dropped the
  last production `useOfficeRefetch` callers. Task filter/sort/view/grouping/
  nesting state remains in Zustand as client-only UI state.
- Office task helper/scaffold cleanup moved project task sections, agent run
  linked task labels, and simple-pane parent/blocker task candidates to Query.
  The unused `useOfficeRefetch` hook, legacy Office WS handler registration/test,
  `office.refetchTrigger`, and the unused Office task server-state fields/actions
  were removed.
- Office simple-pane reference-data cleanup moved task detail chat/activity/
  session labels, assignee/project/reviewer/approver pickers, pending approval
  badges, and run-error labels from `office.agentProfiles`/`office.projects`
  store mirrors to active-workspace Office Query caches.
- Final Office store cleanup moved agent detail/routes, project detail and
  writes, create-agent/create-project flows, routines, workspace skills, org
  chart, new-task reference selectors, Office route bootstrap, and costs boot
  data to Office Query caches. The Office Zustand slice now retains only task
  filter/sort/view/grouping/nesting UI state.
- Queue mirror cleanup moved `useQueue` to read/write `qk.session.queue`
  directly, with Query-cache optimistic removal and mutation-local loading
  state. The old queue Zustand state/actions, default-state/root-store
  declarations, action-only queue tests, and duplicate
  `message.queue.status_changed` registration in the old `agent-session` WS
  handler were removed. Queue status WS updates now flow through the Query
  bridge only.
- Plan context cleanup moved the `@Plan` context preview and passthrough
  composer plan expansion to Query-only reads. `LazyPlanPreview` no longer
  requires `StateProvider` or mirrors fetched plans into `taskPlans`, and
  passthrough message composition reads `taskPlanQueryOptions` cache before
  fetching instead of consulting `state.taskPlans` for plan context content.
  The main plan panel hook and old `task-plans` WS handler remain because they
  still own plan editing/seen-state behavior.
- Session todos/prompt usage cleanup moved `useSessionTodoItems` to the
  `qk.sessionRuntime.todos` Query cache only and removed the old
  `session.todos_updated` and `session.prompt_usage` Zustand WS handlers,
  state fields, mutators, default/root-store declarations, and boot seed paths.
  Todo and prompt usage WS updates now flow through the Query bridge only.
- Agent capabilities/poll mode cleanup moved the task-list debug poll badge to
  the `qk.sessionRuntime.pollMode` Query cache only and removed the unused
  auth-methods indicator plus its orphaned frontend authenticate helper. The old
  `session.agent_capabilities` and `session.poll_mode_changed` Zustand WS
  handlers, state fields, mutators, default/root-store declarations, and boot
  seed paths were removed; these runtime WS updates now flow through the Query
  bridge only.
- Improve Kandev/workflow cleanup moved the Improve Kandev bootstrap follow-up
  fetches for workflow steps and workspace repositories through shared Query
  option factories, while keeping the temporary repository store sync for
  existing task-create readers. The old `workflow.*` and `workflow.step.*`
  Zustand WS handler registration and legacy workflow handler/test were
  removed; workflow cache invalidation now flows through the Query bridge only.
  `kanban.update` and `task.*` legacy handlers remain because mounted
  kanban/task readers and task-delete cleanup side effects still depend on
  them.
- Session runtime dead-plumbing cleanup removed production-unused
  `pendingModel`, `sessionRuntime.agents`, and the legacy `terminal.output`
  buffer/action/WS branch from Zustand and store hydration/re-export plumbing.
  The active `session.shell.output`, `session.process.output`, and
  `session.process.status` terminal flows remain. Backend protocol types and
  bridge audit skip metadata for `terminal.output` remain because the protocol
  still defines that stream event, but it no longer writes frontend store state.
- Session mode cleanup moved the session mode selector to Query-only live mode
  data, retaining its existing session snapshot/profile fallback before any live
  mode event arrives. The old `sessionMode` Zustand state/actions, boot
  seed/store plumbing, and legacy `session.mode_changed` WS handler were
  removed. Session mode changes now update `qk.sessionRuntime.mode` through the
  Query bridge only; the `setSessionMode` API remains the user action for
  changing mode.
- Repository scripts cleanup moved `useRepositoryScripts` to Query-only data
  and removed the old `repositoryScripts` Zustand state/actions,
  AppState/default-state plumbing, and store re-exports. SSR task data still
  carries repository scripts as a query-seed-only boot shape, and settings
  repository script saves now update `qk.workspaces.repositoryScripts(repoId)`
  directly instead of clearing a store mirror.
- Task-plan revision cleanup moved revision list/content reads to Query-only
  data, removed `revisionsByTaskId`, `revisionsLoadingByTaskId`,
  `revisionsLoadedByTaskId`, `revisionContentCache`, and their mutator actions
  from the session slice, and deleted the legacy
  `task.plan.revision.created` WS handler. The Query bridge now patches the
  revision list and invalidates individual revision detail caches. The main
  task-plan mirror remains for plan editing, last-seen/indicator, and layout
  auto-open behavior.
- Available commands cleanup moved inline slash commands, TipTap slash
  suggestions, chat-panel agent command data, and empty-turn command
  recognition to Query-only reads. The old `availableCommands` Zustand
  state/actions and legacy `session.available_commands` WS handler were
  removed, and E2E command seeding now writes directly to
  `qk.sessionRuntime.availableCommands(sessionId)`. Empty-turn local notices
  are preserved across API refetch snapshots so Query invalidation cannot drop
  the synthetic hint message. The mobile E2E page object now scopes chat editor
  and send-button locators to the visible `session-chat` panel instead of
  relying on page-wide TipTap DOM order.
- Repository branches cleanup moved `useBranches` to Query-only reads and
  removed the old `repositoryBranches` Zustand state/action, root-store/default
  state declarations, hydration, overrides, and re-export plumbing. Row-level
  branch lists now cold-load through the workspace branch query so
  provider-backed URL repositories still list branches when re-picked from the
  workspace dropdown. Manual refresh forces the repository `?refresh=true`
  endpoint and copies the result into the active workspace branch cache. Stale
  task-create comments and `apps/web/AGENTS.md` were updated so the docs no
  longer describe a removed branch store fallback.
- Workspace repositories cleanup moved workspace repository list reads to
  TanStack Query cache only and removed the old `repositories` Zustand
  state/action, root-store/default-state declarations, hydration, overrides,
  and re-export plumbing. Boot/route repository lists are now query-seed-only
  data. Shared repository lookup hooks read all workspace repository query
  caches, and repository-heavy UI surfaces now read Query cache instead of the
  workspace slice.
- Workflow list cleanup moved workspace workflow list reads to TanStack Query
  cache only and removed `workflows.items`, `setWorkflows`, and
  `reorderWorkflowItems` from the kanban slice/root store. Boot/route workflow
  lists now seed `qk.workflows.all(workspaceId, { includeHidden: true })`
  through `workflowLists.itemsByWorkspaceId`, while `workflows.activeId`
  remains in Zustand as UI/navigation state. Workflow selection, move targets,
  swimlane sorting, settings workflow editing, task-create workflow resolution,
  task mentions, watch dialogs, recent/mobile task switchers, and route
  hydration now read workflow lists from Query cache.
- All-workflow snapshot/loading cleanup moved all-workflow board/sidebar
  snapshot reads and loading state to workflow snapshot Query caches. The
  Kanban board now passes Query-owned snapshots to swimlanes, multi-select, and
  the bulk toolbar; `useWorkspaceSidebarTasks` reads Query snapshots without the
  old active single-workflow store fallback. Removed `kanbanMulti.isLoading`
  plus the unused `setKanbanMultiLoading`, `updateMultiTask`, and
  `removeMultiTask` store surface. `kanbanMulti.snapshots`,
  `setWorkflowSnapshot`, and `clearKanbanMulti` remain temporary write-through
  compatibility for legacy direct readers and WS handlers outside this
  sub-slice.
- Active workflow snapshot cleanup moved the active board read path to workflow
  snapshot Query caches. `useWorkflowSnapshot` no longer falls back to
  `state.kanban`; `useTasks`, `useKanbanData`, `KanbanBoard`, and
  `KanbanWithPreview` now derive active tasks, steps, loading, multi-select,
  and preview lookup from `qk.workflows.snapshot(...)` /
  `useAllWorkflowSnapshots`. Boot and route hydration now seed workflow
  snapshot query keys from existing `kanban` / `kanbanMulti` payload shapes,
  and `session.state_changed` patches Query-owned workflow snapshot/task-detail
  `primary_session_state`.
- Active board writer cleanup moved dialog create/edit, delete/archive, and
  swimlane drag/drop optimistic writes to workflow snapshot Query caches. It
  also deleted the unused legacy `useDragAndDrop` hook after moving
  `MoveTaskError` to `lib/kanban`, and removed the orphaned
  `swimlane-graph-content` file.
- Sidebar filter options cleanup moved workflow, workflow-step, and
  executor-type filter option lists to workflow snapshot Query caches via
  `useAllWorkflowSnapshots(activeWorkspaceId)`. The hook no longer reads
  `kanbanMulti.snapshots`; Zustand remains only for active workspace UI
  selection in this path.
- Task mention metadata cleanup moved `@task` mention menu construction and
  referenced-task prompt context expansion to workflow snapshot Query caches.
  `buildTaskMentionItems`, `buildTaskMentionsContext`, the TipTap chat input,
  normal chat send, and passthrough composer no longer read `state.kanban` /
  `kanbanMulti.snapshots` for task title/workflow/step metadata. The normal
  send path still writes created chat messages to the local session store for
  missed-frame resilience.
- Recent task switcher cleanup moved live display metadata resolution to
  workflow snapshot Query caches. The hook no longer reads active
  `state.kanban` tasks/steps/workflow id or `kanbanMulti.snapshots`; display
  titles, workflow names, step titles, task states, repositories, and session
  status now derive from Query snapshots plus the remaining session/client
  stores.
- Mobile session switcher cleanup moved the mobile task switcher sheet's step
  data, task selection/archive/delete metadata lookups, task-created cache
  upsert, and workspace-switch snapshot fetch to workflow snapshot Query
  caches. The sheet hook no longer reads or writes active `state.kanban` or
  `kanbanMulti.snapshots`; it keeps only client UI/session stores for active
  task/session selection and pending message/session badges.
- Mobile repo/session metadata cleanup moved mobile repo count/rows, repo pill
  active repo name, session primary marker, repo display name, and
  base-branch-by-repo readers to task-detail and repository Query caches. These
  mobile metadata paths no longer read active `state.kanban.tasks` or
  `kanbanMulti.snapshots`; legacy stores can be empty while cached task detail
  drives repository/session labels.
- Desktop sidebar/removal cleanup moved active sidebar task-create defaults,
  subtask parent labels, task-section workflow selection, sidebar archive/delete
  metadata, sidebar task selection metadata, sidebar move-to-step optimistic
  writes, and `useTaskRemoval` next-task/removal logic to Query-owned workflow
  snapshot and task-detail data. These paths no longer read or write active
  `state.kanban` / `kanbanMulti.snapshots`; the sidebar still keeps client UI
  state in Zustand.
- Task-page/session chrome cleanup moved task-detail route metadata, workflow
  steps, session panel step resolution, header/sidebar new-task context,
  session primary markers, base-branch repository lookup, subtask defaults,
  plan-mode layout detection, and changes-panel multi-repo labels to Query
  caches. These task-page/session chrome paths no longer synthesize or override
  task metadata from active `state.kanban.tasks` / `kanbanMulti.snapshots`.
- Board/command/dialog utility cleanup moved card context-menu workflow move
  targets, command-panel step filtering/badges, and session command subtask
  parent titles to Query caches. `useKanbanCardMoveTargets` reads workflow
  snapshot query cache, command-panel step derivation uses stable Query-owned
  snapshot data, and `SessionCommands` reads `useTaskById` instead of active
  `kanban.tasks`.
- Task-create/card/GitHub indicator/board-grid cleanup moved session-mode
  task-create repository naming, task-create workflow snapshot defaults, kanban
  card subtask parent badges, GitHub PR indicator step tooltips, and board-grid
  loading decisions to Query-owned task detail, repository, workflow list, and
  workflow snapshot caches. These UI readers no longer depend on active
  `state.kanban` or `kanbanMulti.snapshots` mirrors being populated.
- Legacy kanban WS mirror cleanup removed the old `kanban.update` Zustand
  handler registration and handler/test file. The remaining `task.*` WS handler
  keeps only client-side effects: active primary-session adoption, deleted-task
  local storage cleanup, sidebar preference cleanup, and context file cleanup.
  `session.state_changed` and `workspace.deleted` no longer patch or clear
  active `state.kanban` / `kanbanMulti.snapshots`.
- Final kanban store compatibility cleanup removed the top-level `kanban` /
  `kanbanMulti` store fields, `setWorkflowSnapshot`, legacy route/boot seed
  compatibility shapes, active snapshot fallback paths, stale test fixtures,
  and dead active-board fallback helpers. Route readiness now checks Query
  snapshot hydration directly, SSR/boot seeds use
  `workflowSnapshots.itemsByWorkflowId`, and stale scans find no production
  references to the removed store API.
- Feature flags and session worktrees cleanup moved feature flag reads and
  session worktree indexes to Query. `useFeature` now reads `qk.features()`,
  `useSessionWorktrees` reads `qk.sessionRuntime.worktrees(sessionId)`, boot
  payload/query seed handles feature and worktree data, and
  `session.agentctl_ready` patches the session worktree query cache through the
  bridge. The old feature slice, `worktrees` /
  `sessionWorktreesBySessionId` store fields/actions, unused `use-worktree`
  hook, and legacy agent-session worktree store writer were removed.
- Retained Zustand state is documented in `apps/web/AGENTS.md`: client
  navigation/UI state, live session indexes for stream ordering and
  missed-frame recovery, runtime indexes for high-frequency terminal/process/
  git/context/model streams, persisted `userSettings`, and local-only GitHub/
  office UI indexes. Query owns server snapshots for workspace repositories,
  repository branches/scripts, workflow lists, workflow snapshots, task details,
  session worktrees, feature flags, settings catalogs, integrations, office
  data, and system data.
- No GitLab-specific Docker E2E specs exist in this checkout. The GitLab
  sub-wave used focused unit coverage plus the shared integration/settings,
  sidebar, and strict-WS Docker gate; add GitLab browser specs before treating
  GitLab as fully domain-covered by E2E.

Task 10 partial verification completed locally:

- `cd apps/web && rtk pnpm typecheck` passed after system cleanup.
- `cd apps && rtk pnpm --filter @kandev/web test -- hooks/domains/system components/system-metrics lib/query/bridge/index.test.ts lib/query/query-options/query-options.test.ts lib/query/keys.test.ts`
  passed 47 tests.
- `cd apps/web && rtk pnpm typecheck` passed after GitHub cleanup.
- `cd apps && rtk pnpm --filter @kandev/web test -- hooks/domains/github components/github lib/state/slices/github lib/query/bridge/index.test.ts lib/query/query-options/query-options.test.ts lib/query/keys.test.ts`
  passed 29 files / 298 tests.
- `cd apps/web && rtk pnpm e2e:docker tests/github/pr-list-task-indicator.spec.ts tests/github/github-scope-bar.spec.ts tests/integrations/github-watch-reset.spec.ts tests/pr/ci-automation-options.spec.ts tests/git/git-changes-panel.spec.ts`
  passed 22 desktop Docker tests.
- `cd apps/web && rtk pnpm typecheck` passed after GitLab cleanup.
- `cd apps && rtk pnpm --filter @kandev/web test -- hooks/domains/gitlab components/gitlab app/gitlab lib/query/bridge/index.test.ts lib/query/query-options/query-options.test.ts lib/query/keys.test.ts`
  passed 8 files / 81 tests.
- `cd apps/web && rtk pnpm e2e:docker tests/system/sidebar-navigation.spec.ts tests/integrations/jira-settings.spec.ts tests/integrations/linear-settings.spec.ts tests/integrations/sentry-settings.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 21 desktop Docker tests.
- `cd apps/web && rtk pnpm typecheck` passed after the settings small-mirror
  cleanup.
- `cd apps && rtk pnpm --filter @kandev/web test -- hooks/domains/settings/use-settings-query-hooks.test.tsx hooks/domains/settings components/settings lib/query/bridge/index.test.ts lib/query/query-options/query-options.test.ts lib/query/keys.test.ts`
  passed 16 files / 95 tests.
- `cd apps/web && rtk pnpm e2e:docker tests/settings/prompts-settings.spec.ts tests/settings/agent-profile-acp.spec.ts tests/settings/agent-profile-cli-flags.spec.ts tests/settings/config-management.spec.ts tests/settings/utility-agents.spec.ts tests/settings/agent-install-streaming.spec.ts tests/settings/docker-profile-persistence.spec.ts`
  passed 46 desktop Docker tests.
- `cd apps/web && rtk pnpm typecheck` passed after settings catalog/bootstrap
  cleanup.
- `cd apps/web && rtk pnpm test hooks/domains/settings/use-executors-query-sync.test.ts hooks/domains/settings/use-agents-query-sync.test.ts hooks/domains/settings/use-settings-query-hooks.test.tsx lib/query/seed.test.ts src/settings-routes.test.ts components/agent/cli-profile-editor.test.tsx lib/query/bridge/index.test.ts`
  passed 7 files / 39 tests.
- `cd apps/web && rtk pnpm test components/task/handoff-profile-menu-items.test.ts app/office/agents/[id]/components/agent-configuration-tab.test.tsx components/app-sidebar/sections/settings/settings-tree-render.test.tsx components/task/model-selector.test.ts components/task/executor-settings-button.test.tsx components/quick-chat/use-quick-chat-modal.test.ts components/quick-chat/quick-chat-modal.test.ts components/agent/cli-profile-editor.test.tsx hooks/domains/settings/use-settings-query-hooks.test.tsx hooks/domains/settings/use-executors-query-sync.test.ts hooks/domains/settings/use-agents-query-sync.test.ts`
  passed 11 files / 50 tests.
- `rtk rg -n "registerAgentsHandlers|registerExecutorsHandlers|registerExecutorProfileHandlers|handlers/(agents|executors|executor-profiles)|lib/ws/handlers/(agents|executors|executor-profiles)" apps/web --glob '!dist/**'`
  returned no matches for the deleted settings catalog WS handlers.
- `cd apps/web && rtk pnpm e2e:docker tests/settings/agent-profile-acp.spec.ts tests/settings/agent-profile-delete.spec.ts tests/settings/config-management.spec.ts tests/session/new-session-dialog.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 36 desktop Docker tests.
- `cd apps/web && rtk pnpm e2e:docker --project=containers tests/ssh/executor-crud.spec.ts`
  passed 5 container-backed SSH executor tests / 1 skipped.
- `cd apps/web && rtk pnpm typecheck` passed after the final lint-driven
  refactors.
- `cd apps/web && rtk pnpm lint` passed after the final lint-driven refactors.
- `cd apps/web && rtk pnpm test hooks/domains/session/use-session-messages.test.ts hooks/domains/session/use-visibility-backfill.test.ts hooks/domains/session/use-session-commits.test.ts hooks/domains/settings/use-settings-query-hooks.test.tsx hooks/domains/settings/use-executors-query-sync.test.ts lib/query/bridge/index.test.ts lib/query/query-options/query-options.test.ts lib/query/keys.test.ts lib/query/seed.test.ts components/state-hydrator.test.tsx app/office/page-client.test.tsx app/office/tasks/use-paginated-tasks.test.tsx`
  passed 12 files / 94 tests.
- `cd apps/web && rtk pnpm e2e:docker tests/chat/message-add-ws-gap.spec.ts tests/session/new-session-dialog.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 7 desktop Docker tests.
- `cd apps/web && rtk pnpm test hooks/domains/workspace/use-repository-scripts.test.tsx lib/query/seed.test.ts lib/state/slices/workspace/workspace-slice.test.ts`
  first failed on the old Zustand dependency/missing query seed/remaining slice
  field, then passed 3 files / 8 tests after the repository scripts cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the repository scripts
  cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/domains/workspace/use-repository-scripts.ts hooks/domains/workspace/use-repository-scripts.test.tsx lib/query/seed.ts lib/query/seed.test.ts lib/state/slices/workspace/types.ts lib/state/slices/workspace/workspace-slice.ts lib/state/slices/workspace/workspace-slice.test.ts lib/state/default-state.ts lib/state/store-overrides.ts lib/state/store.ts lib/state/slices/index.ts lib/state/store-reexports.ts app/settings/workspace/workspace-repositories-client.tsx lib/ssr/session-page-state.ts`
  passed.
- `rtk rg -n -P "state\\.repositoryScripts|m\\.repositoryScripts|defaultWorkspaceState\\.repositoryScripts|\\bsetRepositoryScripts\\b|\\bclearRepositoryScripts\\b|\\bRepositoryScriptsState\\b|repositoryScripts: \\(typeof defaultWorkspaceState\\)|itemsByRepositoryId\\[repositoryId\\].*repositoryScripts" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned only the repository scripts absence assertions in
  `workspace-slice.test.ts`.
- `rtk git diff --check` passed after the repository scripts cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/session/dockview-repository-scripts.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 9 desktop Docker tests.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/session/mobile-handoff.spec.ts`
  passed 1 mobile Docker test. No repository-script-specific mobile E2E exists
  in this checkout; this validates the mobile task/session shell after the
  Query-only hook change.
- `cd apps/web && rtk pnpm test lib/state/slices/session/task-plans.test.ts lib/ws/handlers/task-plans.test.ts lib/query/bridge/index.test.ts`
  first failed on the retained revision store fields/handler and missing Query
  detail invalidation, then passed 3 files / 27 tests after the task-plan
  revision cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the task-plan revision
  cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/domains/session/use-task-plan.ts lib/state/slices/session/types.ts lib/state/slices/session/session-slice.ts lib/state/slices/session/task-plans.test.ts lib/ws/handlers/task-plans.ts lib/ws/handlers/task-plans.test.ts lib/query/bridge/session.ts lib/query/bridge/index.test.ts lib/state/store.ts`
  passed.
- `rtk rg -n "revisionsByTaskId|revisionsLoadingByTaskId|revisionsLoadedByTaskId|revisionContentCache|setPlanRevisions|upsertPlanRevision|setPlanRevisionsLoading|cachePlanRevisionContent" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned only the revision absence assertions in
  `lib/state/slices/session/task-plans.test.ts`.
- `rtk git diff --check` passed after the task-plan revision cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/task/plan-checkpointing.spec.ts tests/layout/plan-panel-indicator.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 15 desktop Docker tests.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/terminal/mobile-passthrough-rendering.spec.ts tests/chat/mobile-message-add-ws-gap.spec.ts`
  passed 2 mobile Docker tests.
- `cd apps/web && rtk pnpm typecheck` passed after the workspace/kanban
  workflow-step and metadata cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/use-workflow-steps.ts hooks/use-workflow-steps.test.ts components/automations/config-section.tsx components/automations/config-section.test.tsx components/task-create-dialog-effects.ts components/task-create-dialog-effects.test.ts lib/query/query-options/kanban.ts lib/query/query-options/query-options.test.ts lib/query/bridge/workspace.ts lib/query/bridge/index.test.ts lib/query/bridge/workspace.test.ts lib/query/keys.ts lib/query/keys.test.ts hooks/domains/workspace/use-repositories.ts hooks/domains/workspace/use-repository-branches.ts hooks/domains/workspace/use-repository-scripts.ts hooks/domains/kanban/use-kanban-actions.ts lib/state/slices/workspace/types.ts lib/state/slices/workspace/workspace-slice.ts lib/state/slices/workspace/workspace-slice.test.ts app/page.tsx app/tasks/page.tsx app/github/page.tsx lib/ssr/session-page-state.ts lib/state/store.ts`
  passed.
- `cd apps/web && rtk pnpm test hooks/use-workflow-steps.test.ts components/task-create-dialog-effects.test.ts components/automations/config-section.test.tsx lib/query/query-options/query-options.test.ts lib/query/keys.test.ts lib/query/bridge/index.test.ts lib/query/bridge/workspace.test.ts lib/state/slices/workspace/workspace-slice.test.ts hooks/use-workflow-snapshot.test.ts hooks/use-tasks.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts lib/ws/handlers/kanban.test.ts lib/ws/handlers/tasks.test.ts lib/routing/kanban-route-hydration.test.ts`
  passed 16 files / 116 tests.
- `cd apps/web && rtk rg -n "loadedByWorkspaceId|loadingByWorkspaceId|setRepositoriesLoading|loadingByRepositoryId|loadedByRepositoryId|fetchedAtByRepositoryId|fetchErrorByRepositoryId|setRepositoryBranchesLoading|setRepositoryBranchesFetchError|setRepositoryScriptsLoading|invalidateRepositories" apps/web --glob '!dist/**'`
  returned no matches for the deleted workspace metadata.
- `cd apps/web && rtk pnpm e2e:docker tests/kanban/workflow-filter.spec.ts tests/kanban/kanban-board.spec.ts tests/task/task-list.spec.ts tests/settings/config-management.spec.ts tests/session/new-session-dialog.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 35 desktop Docker tests.
- `cd apps/web && rtk rg -n "fetchWorkflowSnapshot|listWorkflows" apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`
  returned no matches after the mobile switcher Query cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the mobile switcher Query
  cleanup.
- `cd apps/web && rtk pnpm exec eslint components/task/mobile/session-task-switcher-sheet-hooks.ts`
  passed.
- `cd apps/web && rtk pnpm e2e:docker --project mobile-chrome tests/task/mobile-sidebar-subtasks.spec.ts tests/kanban/mobile-kanban.spec.ts`
  passed 12 mobile Docker tests.
- `cd apps/web && rtk rg -n "fetchTask" apps/web/components/task/task-page-content.tsx`
  returned no matches after the task-detail route Query cleanup.
- `cd apps/web && rtk pnpm test components/task/task-page-content.test.tsx`
  passed 1 file / 2 tests.
- `cd apps/web && rtk pnpm typecheck` passed after the task-detail route Query
  cleanup.
- `cd apps/web && rtk pnpm exec eslint components/task/task-page-content.tsx components/task/task-page-content.test.tsx`
  passed.
- `cd apps/web && rtk pnpm e2e:docker tests/task/sessionless-task-switch.spec.ts tests/task/task-list.spec.ts tests/session/new-session-dialog.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 9 desktop Docker tests.
- `cd apps/web && rtk rg -n "listTasksByWorkspace\\(" apps/web --glob '!dist/**' --glob '!e2e/**' --glob '!lib/api/**' --glob '!app/actions/**'`
  returned matches only in Query option factories, confirming command-panel no
  longer imports the task-list API directly.
- `cd apps/web && rtk pnpm test components/command-panel.test.ts` passed 1 file
  / 2 tests.
- `cd apps/web && rtk pnpm typecheck` passed after the command-panel Query
  cleanup.
- `cd apps/web && rtk pnpm exec eslint components/command-panel.tsx components/command-panel.test.ts`
  passed.
- `cd apps/web && rtk pnpm e2e:docker tests/command-panel.spec.ts tests/task/task-list.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 10 desktop Docker tests.
- RED board/command/dialog utility gate:
  `rtk pnpm --dir apps/web test components/kanban-card-menu-items.test.tsx components/command-panel.test.ts components/session-commands.test.tsx`
  failed because card move targets still required `StateProvider` /
  `kanbanMulti.snapshots`, command-panel step metadata ignored Query-owned
  workflow snapshots, and session subtask creation passed a blank parent title
  when only `qk.tasks.detail(...)` was populated.
- `rtk make fmt` passed after the board/command/dialog utility cleanup.
- `rtk pnpm --dir apps/web test components/kanban-card-menu-items.test.tsx components/command-panel.test.ts components/session-commands.test.tsx`
  passed 3 files / 8 tests after the utility cleanup.
- `rtk pnpm --dir apps/web typecheck` passed after the utility cleanup.
- `rtk pnpm --dir apps/web exec eslint components/kanban-card-menu-items.tsx components/kanban-card-menu-items.test.tsx components/command-panel.tsx components/command-panel.test.ts components/session-commands.tsx components/session-commands.test.tsx components/task/session-tab.tsx components/task/session-tab.test.tsx`
  passed with duplicate-string warnings only in existing test fixtures.
- `rtk rg -n "state\\.kanban|s\\.kanban|getState\\(\\)\\.kanban|kanbanMulti|setWorkflowSnapshot|findTaskInSnapshots|\\.kanban\\.(tasks|steps|workflowId|loading|error)" apps/web/components/kanban-card-menu-items.tsx apps/web/components/command-panel.tsx apps/web/components/session-commands.tsx apps/web/components/task/session-tab.tsx`
  returned no matches after the utility cleanup.
- `rtk git diff --check` passed after the utility cleanup.
- `rtk pnpm --dir apps/web e2e:docker tests/command-panel.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/task/sidebar-send-to-workflow.spec.ts tests/task/subtask.spec.ts tests/system/ws-event-accounting.spec.ts`
  first exposed a command-panel React update loop when snapshot-derived steps
  were unstable, then passed 28 desktop Docker tests after stabilizing the
  Query-derived step array.
- `rtk pnpm --dir apps/web e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/task/mobile-sidebar-subtasks.spec.ts tests/session/mobile-handoff.spec.ts`
  passed 13 mobile Docker tests after the utility cleanup.
- RED task-create/card/GitHub indicator/board-grid gate:
  `rtk pnpm --dir apps/web test components/task-create-dialog-state.test.ts components/kanban-card-content.test.tsx components/github/my-github/pr-row-task-indicator.test.tsx components/kanban-board-grid.test.tsx`
  failed because session-mode task-create repo naming, task-create workflow
  snapshot defaults, kanban card subtask parent badges, GitHub PR indicator
  step tooltips, and board-grid loading still depended on legacy `kanban` /
  `kanbanMulti` mirrors when Query caches were populated.
- `rtk make fmt` passed after the task-create/card/GitHub indicator/board-grid
  cleanup.
- `rtk pnpm --dir apps/web test components/task-create-dialog-state.test.ts components/kanban-card-content.test.tsx components/github/my-github/pr-row-task-indicator.test.tsx components/kanban-board-grid.test.tsx`
  passed 4 files / 27 tests after the cleanup.
- `rtk pnpm --dir apps/web typecheck` passed after the cleanup.
- Targeted eslint for the task-create, kanban-card, GitHub indicator, and
  board-grid files/tests passed after the cleanup.
- The migrated-file stale scan for active `state.kanban`, `kanbanMulti`,
  `setWorkflowSnapshot`, and `findTaskInSnapshots` returned no matches.
- `rtk git diff --check` passed after the cleanup.
- `rtk pnpm --dir apps/web e2e:docker tests/task/create-task.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/github/pr-list-task-indicator.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 22 desktop Docker tests after the cleanup.
- `rtk pnpm --dir apps/web e2e:docker --project=mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts tests/kanban/mobile-kanban.spec.ts`
  passed 12 mobile Docker tests after the cleanup.
- `cd apps/web && rtk rg -n "getSubtaskCount" apps/web --glob '!dist/**' --glob '!e2e/**' --glob '!lib/api/**'`
  returned only Query option/test references after the subtask-count hook stopped
  importing the API directly.
- `cd apps/web && rtk pnpm test hooks/use-subtask-count.test.ts components/task/task-archive-confirm-dialog.test.tsx components/task/task-delete-confirm-dialog.test.tsx`
  passed 3 files / 19 tests.
- `cd apps/web && rtk pnpm typecheck` passed after the subtask-count Query
  cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/use-subtask-count.ts hooks/use-subtask-count.test.ts components/task/task-archive-confirm-dialog.test.tsx components/task/task-delete-confirm-dialog.test.tsx`
  passed.
- `cd apps/web && rtk pnpm e2e:docker tests/task/subtask.spec.ts tests/task/task-list.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 15 desktop Docker tests.
- `rtk rg -n "clearTaskPlan|clearQueueStatus|task\\.plan\\.reverted" apps/web --glob '!dist/**'`
  returned no `clearTaskPlan`/`clearQueueStatus` references and only backend
  type plus Query bridge references for `task.plan.reverted`.
- `cd apps/web && rtk pnpm test lib/state/slices/session/task-plans.test.ts lib/state/slices/session/session-slice.upsert.test.ts lib/ws/handlers/task-plans.test.ts lib/query/bridge/index.test.ts`
  passed 4 files / 35 tests.
- `cd apps/web && rtk pnpm typecheck` passed after the session dead-code
  cleanup.
- `cd apps/web && rtk pnpm exec eslint lib/state/slices/session/types.ts lib/state/slices/session/session-slice.ts lib/state/slices/session/task-plans.test.ts lib/state/slices/session/session-slice.upsert.test.ts lib/state/store.ts lib/ws/handlers/task-plans.ts`
  passed.
- `cd apps/web && rtk pnpm e2e:docker tests/task/plan-checkpointing.spec.ts tests/session/new-session-dialog.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 16 desktop Docker tests.
- `cd apps/web && rtk pnpm test hooks/domains/session/use-queue.test.ts`
  first failed because `useQueue` still required `StateProvider`, then passed
  after the queue Query migration.
- `cd apps/web && rtk pnpm test hooks/domains/session/use-queue.test.ts components/task/chat/queued-ghost-list.test.tsx lib/query/bridge/index.test.ts lib/ws/handlers/agent-session.test.ts lib/state/slices/session/session-slice.upsert.test.ts`
  passed 5 files / 59 tests.
- `cd apps/web && rtk pnpm typecheck` passed after the queue mirror cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/domains/session/use-queue.ts hooks/domains/session/use-queue.test.ts lib/ws/handlers/agent-session.ts lib/state/slices/session/types.ts lib/state/slices/session/session-slice.ts lib/state/slices/session/session-slice.upsert.test.ts lib/state/store.ts lib/state/default-state.ts lib/state/store-overrides.ts lib/state/slices/index.ts`
  passed.
- `rtk rg -n "QueueMeta|QueueState|defaultSessionState\\.queue|initialState\\.queue|m\\.queue|state\\.queue|setQueueEntries|removeQueueEntry:|setQueueLoading:" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned only the local `setQueueLoading` helper type in `use-queue.ts`; no
  queue Zustand mirror fields/actions remain.
- `rtk git diff --check` passed after the queue mirror cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/chat/message-queue.spec.ts tests/workflow/workflow-manual-move-queue.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 10 desktop Docker tests.
- `cd apps/web && rtk pnpm e2e:docker --project mobile-chrome tests/workflow/mobile-workflow-manual-move-queue.spec.ts`
  passed 1 mobile Docker test.
- `cd apps/web && rtk pnpm test components/task/chat/context-items/lazy-plan-preview.test.tsx components/task/passthrough-chat-composer.test.ts`
  first failed because `LazyPlanPreview` still required `StateProvider` and the
  composer still read the store for cached plan content, then passed after the
  Query-only plan-context cleanup.
- `cd apps/web && rtk pnpm test components/task/chat/context-items/lazy-plan-preview.test.tsx components/task/passthrough-chat-composer.test.ts lib/query/bridge/index.test.ts`
  passed 3 files / 23 tests.
- `cd apps/web && rtk pnpm typecheck` passed after the plan-context cleanup.
- `cd apps/web && rtk pnpm exec eslint components/task/chat/context-items/lazy-plan-preview.tsx components/task/chat/context-items/lazy-plan-preview.test.tsx components/task/passthrough-chat-composer.tsx components/task/passthrough-chat-composer.test.ts`
  passed.
- `rtk rg -n "cachedTaskPlanContent|state\\.taskPlans|setTaskPlan\\(|useAppStore|useAppStoreApi" apps/web/components/task/chat/context-items/lazy-plan-preview.tsx apps/web/components/task/passthrough-chat-composer.tsx`
  returned only the existing passthrough composer `useAppStoreApi` usage for
  submit/task-mention state; no plan-context store fallback remains.
- `rtk git diff --check` passed after the plan-context cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/cli-mode/passthrough-toolbar.spec.ts tests/layout/plan-panel-indicator.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 13 desktop Docker tests.
- `cd apps/web && rtk pnpm test lib/ws/router.test.ts lib/query/bridge/session-runtime.test.ts lib/query/bridge/index.test.ts lib/state/slices/session-runtime/purge-session.test.ts components/task/task-item.test.tsx`
  passed 5 files / 31 tests after the session runtime mirror cleanups.
- `cd apps/web && rtk pnpm typecheck` passed after the session runtime mirror
  cleanups.
- `cd apps/web && rtk pnpm exec eslint components/task/task-item.tsx lib/api/domains/session-api.ts lib/ws/router.ts lib/ws/router.test.ts lib/query/bridge/session-runtime.test.ts lib/query/bridge/index.test.ts lib/state/default-state.ts lib/state/store.ts lib/state/store-overrides.ts lib/state/slices/session-runtime/types.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session-runtime/purge-session.test.ts lib/query/seed.ts components/task/chat/use-chat-panel-state.ts`
  passed.
- `rtk rg -n "sessionTodos|setSessionTodos|PromptUsageState|SessionTodosState|promptUsage|setPromptUsage|agentCapabilities|setAgentCapabilities|AgentCapabilitiesState|sessionPollMode|setSessionPollMode|SessionPollModeState|registerSessionTodosHandlers|registerPromptUsageHandlers|registerAgentCapabilitiesHandlers|registerSessionPollModeHandlers|handlers/session-todos|handlers/prompt-usage|handlers/agent-capabilities|handlers/session-poll-mode|AuthMethodsIndicator|auth-methods-indicator|authenticateSession" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned only Query key/bridge/query-option references, absence assertions,
  and unrelated settings hook naming; no old Zustand runtime mirror fields,
  actions, handlers, or unused auth indicator/helper remain.
- `rtk git diff --check` passed after the session runtime mirror cleanups.
- `cd apps/web && rtk pnpm e2e:docker tests/chat/chat-status-bar.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 8 desktop Docker tests for session todos/prompt usage cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project mobile-chrome tests/chat/mobile-message-add-ws-gap.spec.ts`
  passed 1 mobile Docker test for the chat hook surface.
- `cd apps/web && rtk pnpm e2e:docker tests/task/task-list.spec.ts tests/session/new-session-dialog.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 7 desktop Docker tests for agent capabilities/poll mode cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project mobile-chrome tests/task/mobile-task-list-search.spec.ts`
  passed 1 mobile Docker test for the task item surface.
- `cd apps/web && rtk pnpm test lib/ws/router.test.ts lib/query/bridge/workspace.test.ts lib/query/bridge/index.test.ts hooks/use-workflow-snapshot.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts components/improve-kandev-dialog.test.tsx`
  first failed on the still-registered workflow router handler / missing Query
  cache hydration, then passed 7 files / 35 tests after the Improve Kandev and
  workflow-handler cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the Improve Kandev and
  workflow-handler cleanup.
- `cd apps/web && rtk pnpm exec eslint components/improve-kandev-dialog.tsx components/improve-kandev-dialog.test.tsx lib/ws/router.ts lib/ws/router.test.ts lib/query/bridge/workspace.ts lib/query/bridge/workspace.test.ts hooks/use-workflow-snapshot.ts hooks/use-workflow-snapshot.test.ts hooks/domains/kanban/use-all-workflow-snapshots.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts`
  passed.
- `rtk rg -n "registerWorkflowsHandlers|handlers/workflows|lib/ws/handlers/workflows|workflow\\.created handler|workflow\\.updated handler" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned no matches after deleting the legacy workflow WS handler.
- `cd apps/web && rtk pnpm e2e:docker tests/workflow/workflow-steps.spec.ts tests/workflow/workflow-settings.spec.ts tests/task/task-list.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 13 desktop Docker tests.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/task/mobile-task-list-search.spec.ts`
  passed 12 mobile Docker tests.
- `cd apps/web && rtk pnpm test lib/ws/router.test.ts lib/state/slices/session/session-slice.upsert.test.ts lib/state/slices/session-runtime/purge-session.test.ts lib/state/slices/session-runtime/output-caps.test.ts`
  first failed on the retained legacy state/handler assertions, then passed 4
  files / 15 tests after removing the stale runtime plumbing.
- `cd apps/web && rtk pnpm typecheck` passed after the session-runtime
  dead-plumbing cleanup.
- `cd apps/web && rtk pnpm exec eslint lib/ws/router.ts lib/ws/router.test.ts lib/ws/handlers/terminals.ts lib/state/default-state.ts lib/state/store.ts lib/state/store-overrides.ts lib/state/store-reexports.ts lib/state/hydration/hydrator.ts lib/state/slices/index.ts lib/state/slices/session/types.ts lib/state/slices/session/session-slice.ts lib/state/slices/session/session-slice.upsert.test.ts lib/state/slices/session-runtime/types.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session-runtime/purge-session.test.ts lib/state/slices/session-runtime/output-caps.test.ts e2e/tests/terminal/terminal-agent.spec.ts`
  passed.
- `rtk rg -n "pendingModel|setPendingModel|clearPendingModel|PendingModel|TerminalState|setTerminalOutput|terminal\\.terminals|terminal.output|AgentState|defaultSessionRuntimeState\\.agents|draft\\.agents|state\\.agents|m\\.agents|initialState\\.agents" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned only intentional backend protocol/audit metadata, absence tests, and
  unrelated terminal UI names.
- `cd apps/web && rtk pnpm e2e:docker tests/chat/chat-status-bar.spec.ts tests/terminal/terminal-agent.spec.ts tests/search/terminal-search.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 17 desktop Docker tests after fixing the spec to use
  `errors.TimeoutError` from Playwright.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/chat/mobile-message-add-ws-gap.spec.ts`
  passed 1 mobile Docker test.
- `rtk git diff --check` passed after the session-runtime dead-plumbing
  cleanup.
- `cd apps/web && rtk pnpm test lib/ws/router.test.ts lib/state/slices/session-runtime/purge-session.test.ts components/task/mode-selector.test.tsx lib/query/bridge/session-runtime.test.ts lib/query/bridge/index.test.ts`
  first failed on the retained legacy `session.mode_changed` handler and
  `sessionMode` store field, then passed 5 files / 21 tests after the
  session-mode cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the session-mode cleanup.
- `cd apps/web && rtk pnpm exec eslint components/task/mode-selector.tsx components/task/mode-selector.test.tsx lib/ws/router.ts lib/ws/router.test.ts lib/state/default-state.ts lib/state/store.ts lib/state/store-overrides.ts lib/state/slices/session-runtime/types.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session-runtime/purge-session.test.ts lib/query/seed.ts`
  passed.
- `rtk rg -n -P "\\bsessionMode\\b|\\bclearSessionMode\\b|registerSessionModeHandlers\\b|handlers/session-mode(?:\\.|$)|lib/ws/handlers/session-mode(?:\\.|$)|state\\.sessionMode(?!l|s)|m\\.sessionMode(?!l|s)|defaultSessionRuntimeState\\.sessionMode(?!l|s)" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned only the absence assertions in `purge-session.test.ts`.
- `rtk git diff --check` passed after the session-mode cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/chat/quick-chat.spec.ts tests/session/new-session-dialog.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 11 desktop Docker tests.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/chat/mobile-message-add-ws-gap.spec.ts`
  passed 1 mobile Docker test.
- `cd apps/web && rtk pnpm test hooks/use-inline-slash.test.tsx lib/ws/router.test.ts lib/state/slices/session-runtime/purge-session.test.ts`
  first failed because `useInlineSlash` still required `StateProvider`, the
  router still registered `session.available_commands`, and the runtime slice
  still exposed `availableCommands`; this was the RED step for the available
  commands cleanup.
- `cd apps/web && rtk pnpm test lib/state/slices/session/session-slice.merge-messages.test.ts`
  first failed because API refetch snapshots dropped local empty-turn notices;
  the test passed after preserving local `metadata.empty_turn` status messages.
- `cd apps/web && rtk pnpm test lib/state/slices/session/session-slice.merge-messages.test.ts hooks/use-inline-slash.test.tsx components/task/chat/use-chat-input-container.test.ts lib/ws/handlers/empty-turn-notice.test.ts lib/ws/router.test.ts lib/state/slices/session-runtime/purge-session.test.ts lib/query/bridge/session-runtime.test.ts lib/query/seed.test.ts`
  passed 8 files / 46 tests after the available commands cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the available commands
  cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/use-inline-slash.ts hooks/use-inline-slash.test.tsx components/task/chat/tiptap-input.tsx components/task/chat/use-chat-panel-state.ts lib/ws/router.ts lib/ws/router.test.ts lib/ws/handlers/empty-turn-notice.ts lib/ws/handlers/empty-turn-notice.test.ts lib/state/slices/session-runtime/types.ts lib/state/slices/session-runtime/session-runtime-slice.ts lib/state/slices/session-runtime/purge-session.test.ts lib/state/slices/session/message-signature.ts lib/state/slices/session/session-slice.merge-messages.test.ts lib/state/slices/index.ts lib/state/store.ts lib/state/default-state.ts lib/query/seed.ts e2e/helpers/session-store.ts e2e/pages/session-page.ts`
  passed.
- `rtk rg -n -P "state\\.availableCommands|s\\.availableCommands|m\\.availableCommands|defaultSessionRuntimeState\\.availableCommands|\\bsetAvailableCommands\\b|\\bclearAvailableCommands\\b|\\bAvailableCommandsState\\b|availableCommands: \\(typeof defaultSessionRuntimeState\\)|registerAvailableCommandsHandlers|handlers/available-commands|lib/ws/handlers/available-commands" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned only the available-commands absence assertions in
  `purge-session.test.ts`.
- `rtk git diff --check` passed after the available commands cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/chat/empty-turn.spec.ts tests/chat/chat-status-bar.spec.ts tests/system/ws-event-accounting.spec.ts`
  first failed because Query invalidation refetched session messages without
  the synthetic empty-turn notice, then passed 10 desktop Docker tests after the
  local empty-turn merge preservation fix.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/chat/mobile-message-add-ws-gap.spec.ts`
  first failed because the E2E page object selected a page-wide non-editable
  TipTap node on mobile, then passed 1 mobile Docker test after scoping the
  editor locator to the visible chat panel.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/chat/mobile-empty-turn.spec.ts tests/chat/mobile-message-add-ws-gap.spec.ts`
  passed 2 mobile Docker tests after the available commands cleanup.
- `cd apps/web && rtk pnpm test hooks/domains/workspace/use-repository-branches.test.tsx lib/state/slices/workspace/workspace-slice.test.ts`
  first failed because `useBranches` still required `StateProvider` and
  `repositoryBranches` still existed in the workspace slice; this was the RED
  step for the repository branches cleanup.
- `cd apps/web && rtk pnpm test hooks/domains/workspace/use-repository-branches.test.tsx lib/state/slices/workspace/workspace-slice.test.ts lib/query/query-options/query-options.test.ts lib/query/keys.test.ts components/task-create-dialog-repo-chips.test.tsx components/task-create-dialog-branch-utils.test.ts components/task-create-dialog-pill.test.tsx`
  passed 7 files / 72 tests after adding coverage for Query-only id/path branch
  reads, workspace-endpoint cold loads, and forced repository refresh.
- `cd apps/web && rtk pnpm typecheck` passed after the repository branches
  cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/domains/workspace/use-repository-branches.ts hooks/domains/workspace/use-repository-branches.test.tsx components/task-create-dialog-state.ts components/task-create-dialog-repo-chips.tsx lib/state/slices/workspace/types.ts lib/state/slices/workspace/workspace-slice.ts lib/state/slices/workspace/workspace-slice.test.ts lib/state/default-state.ts lib/state/store-overrides.ts lib/state/hydration/hydrator.ts lib/state/store-reexports.ts lib/state/slices/index.ts lib/state/store.ts`
  passed.
- `rtk rg -n -P "state\\.repositoryBranches|m\\.repositoryBranches|defaultWorkspaceState\\.repositoryBranches|\\bsetRepositoryBranches\\b|\\bRepositoryBranchesState\\b|repositoryBranches: \\(typeof defaultWorkspaceState\\)|itemsByRepositoryId\\[.*\\].*repositoryBranches|repositoryBranches\\.byRepository|Zustand cache.*branch|store dedupes by repositoryId" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned only the repository-branches absence assertion in
  `workspace-slice.test.ts`.
- `rtk git diff --check` passed after the repository branches cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/task/create-task-branch-selector.spec.ts tests/task/create-task-url-reopen-no-branches.spec.ts tests/system/ws-event-accounting.spec.ts`
  first failed on the refresh and URL-reopen regressions, then passed 15
  desktop Docker tests after forcing repository refresh and preserving
  workspace-endpoint cold loads.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts tests/task/mobile-task-list-search.spec.ts`
  passed 2 mobile Docker tests after the repository branches cleanup.
- `cd apps/web && rtk pnpm test hooks/domains/workspace/use-repositories.test.tsx lib/query/seed.test.ts lib/state/slices/workspace/workspace-slice.test.ts`
  first failed because repository hooks still required `StateProvider`,
  initial-state repository lists did not seed the Query cache, and the
  workspace slice still exposed `repositories`/`setRepositories`; this was the
  RED step for the workspace repositories cleanup.
- `cd apps/web && rtk pnpm test hooks/domains/workspace/use-repositories.test.tsx lib/query/seed.test.ts lib/state/slices/workspace/workspace-slice.test.ts`
  passed 3 files / 12 tests after the workspace repositories cleanup.
- `cd apps/web && rtk pnpm test hooks/domains/workspace/use-repositories.test.tsx lib/query/seed.test.ts lib/state/slices/workspace/workspace-slice.test.ts components/task-create-dialog-state.test.ts components/command-panel.test.ts components/task/task-changes-panel.test.ts components/task/changes-panel.test.ts components/task/changes-panel-pr-files.test.tsx components/task/task-session-sidebar-aggregate.test.ts components/task/recent-task-switcher-model.test.ts components/review/review-dialog.build-files.test.ts`
  passed 11 files / 91 tests.
- `cd apps/web && rtk pnpm typecheck` passed after the workspace repositories
  cleanup.
- Targeted eslint for repository hooks, query seed, workspace slice/store
  plumbing, route seeders, and repository-heavy UI consumers passed.
- `rtk rg -n -P "state\\.repositories\\.itemsByWorkspaceId|\\bs\\.repositories\\.itemsByWorkspaceId|defaultWorkspaceState\\.repositories|\\brepositories: \\(typeof defaultWorkspaceState\\)|\\bRepositoriesState\\b|setRepositories:\\s*\\(workspaceId|setRepositories\\(workspaceId|state\\.setRepositories|\\bs\\.setRepositories|itemsByWorkspaceId\\[.*\\].*state\\.repositories" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned no matches after the workspace repositories cleanup.
- `rtk git diff --check` passed after the workspace repositories cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/task/create-task-branch-selector.spec.ts tests/session/dockview-repository-scripts.spec.ts tests/git/git-changes-panel.spec.ts tests/task/task-list.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 40 desktop Docker tests after the workspace repositories cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts tests/task/mobile-sidebar-subtasks.spec.ts tests/session/mobile-handoff.spec.ts`
  passed 3 mobile Docker tests after the workspace repositories cleanup.
- `cd apps/web && rtk pnpm test hooks/use-workflows.test.tsx lib/query/seed.test.ts lib/state/slices/kanban/kanban-slice.test.ts`
  first failed because `useWorkflows` still required `StateProvider`, workflow
  boot lists did not seed the Query cache, and the kanban slice still exposed
  workflow list state/actions; this was the RED step for the workflow list
  cleanup.
- `rtk make fmt` passed after the workflow list cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the workflow list cleanup.
- Targeted eslint for workflow hooks/cache helpers, query seed, kanban
  slice/store plumbing, route seeders, kanban/task-create/settings/watch-dialog
  consumers, route hydration, and mention helpers passed.
- `cd apps/web && rtk pnpm test hooks/use-workflows.test.tsx lib/query/seed.test.ts lib/state/slices/kanban/kanban-slice.test.ts lib/routing/kanban-route-hydration.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/domains/settings/use-workflow-settings.test.ts components/task-create-dialog-state.test.ts components/task/chat/task-mention-items.test.ts components/task/recent-task-switcher-model.test.ts`
  passed 10 files / 60 tests after the workflow list cleanup.
- `rtk rg -n -P "workflows\\.items|setWorkflows\\b|reorderWorkflowItems\\b|WorkflowsState\\[\"items\"\\]|workflows:\\s*\\{\\s*items|items:\\s*workflows\\.map" apps/web --glob '!dist/**' --glob '!e2e/test-results/**' --glob '!**/kanban-slice.test.ts'`
  returned no matches after the workflow list cleanup.
- `rtk git diff --check` passed after the workflow list cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/kanban/workflow-filter.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/workflow/workflow-sorting.spec.ts tests/task/create-task.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 24 desktop Docker tests after the workflow list cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/workflow/mobile-workflow-manual-move-queue.spec.ts tests/task/mobile-sidebar-subtasks.spec.ts tests/task/mobile-create-task-remote-repo.spec.ts`
  passed 14 mobile Docker tests after the workflow list cleanup.
- RED all-workflow snapshot/loading gate:
  `cd apps/web && rtk pnpm test hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts lib/state/slices/kanban/kanban-slice.test.ts`
  failed on the retained store loading/actions and store-backed sidebar data.
- `cd apps/web && rtk pnpm test hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts components/kanban/task-multi-select-toolbar.test.tsx lib/state/slices/kanban/kanban-slice.test.ts lib/ws/handlers/agent-session-kanban.test.ts lib/ws/handlers/kanban.test.ts lib/ws/handlers/tasks.test.ts lib/routing/kanban-route-hydration.test.ts components/task/chat/task-mention-items.test.ts hooks/use-message-handler.test.ts`
  passed 10 files / 45 tests after the all-workflow snapshot/loading cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the all-workflow
  snapshot/loading cleanup.
- Targeted eslint for the all-workflow snapshot hook, workspace sidebar hook,
  multi-select hook/tests, Kanban board/swimlane, kanban slice/store plumbing,
  hydration/default-state, and related WS/routing fixtures passed.
- `rtk rg -n "setKanbanMultiLoading|kanbanMulti\\.isLoading|isMultiLoading|updateMultiTask|removeMultiTask|kanbanMulti: \\{ snapshots: \\{\\}, isLoading: false \\}" apps/web --glob '!dist/**' --glob '!e2e/test-results/**' --glob '!**/*.test.ts' --glob '!**/*.test.tsx'`
  returned no production matches.
- `rtk git diff --check` passed after the all-workflow snapshot/loading
  cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/kanban/workflow-filter.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/workflow/workflow-sorting.spec.ts tests/task/create-task.spec.ts tests/task/task-list.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 25 desktop Docker tests after the all-workflow snapshot/loading
  cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/workflow/mobile-workflow-manual-move-queue.spec.ts tests/task/mobile-sidebar-subtasks.spec.ts tests/task/mobile-create-task-remote-repo.spec.ts`
  passed 14 mobile Docker tests after the all-workflow snapshot/loading
  cleanup.
- RED task-detail lookup gate:
  `cd apps/web && rtk pnpm test hooks/use-task.test.ts`
  failed with `useAppStore must be used within StateProvider`, proving
  `useTask`/`useTaskById` still depended on the legacy kanban store fallback.
- `cd apps/web && rtk pnpm test hooks/use-task.test.ts components/task/chat/messages/chat-message.test.tsx hooks/domains/session/use-session-state.test.ts components/task/task-page-content.test.tsx`
  passed 4 files / 32 tests after the task-detail lookup cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/use-task.ts hooks/use-task.test.ts hooks/domains/kanban/use-task-by-id.ts hooks/domains/kanban/use-task-repositories.ts components/task/chat/messages/chat-message.test.tsx`
  passed after the task-detail lookup cleanup.
- `rtk rg -n "useAppStore|findTaskInSnapshots|state\\.kanban|kanbanMulti\\.snapshots|kanban store|kanban tasks slice" apps/web/hooks/use-task.ts apps/web/hooks/domains/kanban/use-task-by-id.ts apps/web/hooks/domains/kanban/use-task-repositories.ts apps/web/components/task/chat/messages/chat-message.test.tsx`
  returned no matches after the task-detail lookup cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the task-detail lookup
  cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/task/task-list.spec.ts tests/session/new-session-dialog.spec.ts tests/chat/message-add-ws-gap.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 8 desktop Docker tests after the task-detail lookup cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/task/mobile-sidebar-subtasks.spec.ts tests/task/mobile-task-list-search.spec.ts tests/session/mobile-session-details.spec.ts`
  passed 2 mobile Docker tests after the task-detail lookup cleanup.
- RED task-removal Query-cache gate:
  `cd apps/web && rtk pnpm test hooks/use-task-removal.test.ts`
  failed because a cached `qk.workflows.snapshot(...)` still contained the
  removed task when the legacy `kanbanMulti.snapshots` mirror was empty.
- RED task-removal SPA redirect gate:
  `cd apps/web && rtk pnpm test hooks/use-task-removal.test.ts`
  failed because last-task removal changed `window.location.pathname` but did
  not emit the SPA location-change event, leaving strict WS accounting exposed
  to a full-document navigation race.
- `rtk make fmt` passed after the task-removal cleanup.
- `cd apps/web && rtk pnpm test hooks/use-task-removal.test.ts hooks/use-task-multi-select.test.ts components/kanban/task-multi-select-toolbar.test.tsx components/task/task-delete-confirm-dialog.test.tsx components/task/task-archive-confirm-dialog.test.tsx lib/query/bridge/tasks.test.ts lib/ws/handlers/tasks.test.ts lib/ws/handlers/kanban.test.ts`
  passed 8 files / 55 tests after the task-removal cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the task-removal cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/use-task-removal.ts hooks/use-task-removal.test.ts hooks/use-task-multi-select.ts hooks/use-task-multi-select.test.ts lib/query/workflow-snapshot-cache.ts`
  passed after the task-removal cleanup.
- `rtk rg -n "kanbanMulti|setWorkflowSnapshot" apps/web/hooks/use-task-removal.ts apps/web/lib/query/workflow-snapshot-cache.ts apps/web/hooks/use-task-multi-select.ts`
  returned no production matches after the task-removal cleanup.
- `rtk git diff --check` passed after the task-removal cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/kanban/card-menu-delete-archive.spec.ts tests/task/delete-task-redirect.spec.ts tests/task/archive-task-redirect.spec.ts tests/task/task-switcher-status.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/kanban/workflow-filter.spec.ts tests/system/ws-event-accounting.spec.ts`
  first failed in `archive-task-redirect` teardown because the old
  `window.location.href = "/"` redirect lost the strict WS browser hook, then
  passed 17 desktop Docker tests after the SPA-router redirect fix.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/task/mobile-sidebar-subtasks.spec.ts tests/task/mobile-task-list-search.spec.ts tests/kanban/mobile-kanban.spec.ts`
  passed 13 mobile Docker tests after the task-removal cleanup.
- RED active workflow snapshot gate:
  `cd apps/web && rtk pnpm test hooks/use-workflow-snapshot.test.ts hooks/use-tasks.test.ts lib/query/seed.test.ts lib/query/bridge/index.test.ts`
  failed because `useWorkflowSnapshot` still returned the active `state.kanban`
  fallback while Query was pending, `useTasks` surfaced legacy
  `kanban.tasks`, boot snapshots were not seeded into
  `qk.workflows.snapshot(...)`, and `session.state_changed` did not patch
  Query-owned card `primary_session_state`.
- `rtk make fmt` passed after the active workflow snapshot cleanup.
- `cd apps/web && rtk pnpm test hooks/use-workflow-snapshot.test.ts hooks/use-tasks.test.ts hooks/domains/kanban/use-kanban-data.test.tsx lib/query/seed.test.ts lib/query/bridge/index.test.ts`
  passed 5 files / 32 tests after the active workflow snapshot cleanup.
- `cd apps/web && rtk pnpm test hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-all-workflow-snapshots.query.test.tsx hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts components/kanban/task-multi-select-toolbar.test.tsx components/task/task-page-content.test.tsx components/kanban-with-preview.test.ts`
  passed 6 files / 20 tests after the active workflow snapshot cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the active workflow snapshot
  cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/use-workflow-snapshot.ts hooks/use-workflow-snapshot.test.ts hooks/use-tasks.ts hooks/use-tasks.test.ts hooks/domains/kanban/use-kanban-data.ts hooks/domains/kanban/use-kanban-data.test.tsx components/kanban-board.tsx components/kanban-with-preview.tsx lib/query/seed.ts lib/query/seed.test.ts lib/query/bridge/session.ts lib/query/bridge/index.test.ts`
  passed with one existing max-lines warning in `lib/query/bridge/index.test.ts`.
- `rtk rg -n "state\\.kanban|state\\.kanbanMulti|kanbanMulti\\.snapshots|snapshotState\\?\\.isLoading|kanban\\.isLoading" apps/web/hooks/use-workflow-snapshot.ts apps/web/hooks/use-tasks.ts apps/web/hooks/domains/kanban/use-kanban-data.ts apps/web/components/kanban-board.tsx apps/web/components/kanban-with-preview.tsx apps/web/lib/query/seed.ts apps/web/lib/query/bridge/session.ts`
  returned no matches after the active workflow snapshot cleanup.
- `rtk git diff --check` passed after the active workflow snapshot cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/kanban/kanban-board.spec.ts tests/kanban/workflow-filter.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/task/task-list.spec.ts tests/task/task-switcher-status.spec.ts tests/session/session-tab-management.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 23 desktop Docker tests after the active workflow snapshot cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/task/mobile-sidebar-subtasks.spec.ts tests/task/mobile-task-list-search.spec.ts`
  passed 13 mobile Docker tests after the active workflow snapshot cleanup.
- RED active board writer gate:
  `cd apps/web && rtk pnpm test hooks/domains/kanban/use-kanban-actions.test.tsx hooks/use-task-crud.test.tsx hooks/domains/kanban/use-swimlane-move.test.tsx`
  failed because dialog create/edit, task delete/archive, and swimlane moves
  still wrote/read legacy `kanban` / `kanbanMulti` mirrors instead of workflow
  snapshot Query caches.
- `rtk make fmt` passed after the active board writer cleanup.
- `cd apps/web && rtk pnpm test hooks/domains/kanban/use-kanban-actions.test.tsx hooks/use-task-crud.test.tsx hooks/domains/kanban/use-swimlane-move.test.tsx hooks/use-task-multi-select.test.ts components/kanban/task-multi-select-toolbar.test.tsx lib/query/bridge/tasks.test.ts lib/ws/handlers/tasks.test.ts lib/ws/handlers/kanban.test.ts`
  passed 8 files / 37 tests after the active board writer cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the active board writer
  cleanup.
- Targeted eslint for task CRUD, kanban actions, swimlane move/content,
  `MoveTaskError`, view registry, and workflow snapshot cache files passed.
- Stale scans for legacy kanban writer references in the touched writer files
  and for the deleted `use-drag-and-drop` / `swimlane-graph-content` paths
  returned no matches.
- `rtk git diff --check` passed after the active board writer cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/kanban/kanban-board.spec.ts tests/kanban/card-menu-delete-archive.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/kanban/pipeline-view.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 19 desktop Docker tests after the active board writer cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts`
  passed 11 mobile Docker tests after the active board writer cleanup.
- RED sidebar filter options gate:
  `cd apps/web && rtk pnpm test components/task/sidebar-filter/use-filter-value-options.test.ts`
  failed because workflow, workflow-step, and executor-type options were empty
  when the legacy `kanbanMulti.snapshots` mirror was empty but workflow
  snapshot Query caches were populated.
- `rtk make fmt` passed after the sidebar filter options cleanup.
- `cd apps/web && rtk pnpm test components/task/sidebar-filter/use-filter-value-options.test.ts`
  passed 1 file / 6 tests after the sidebar filter options cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the sidebar filter options
  cleanup.
- Targeted eslint for the sidebar filter options hook/test passed after the
  cleanup.
- The stale server-state scan for `kanbanMulti.snapshots`, active
  `state.kanban` task/step reads, and `setWorkflowSnapshot` returned no matches
  in the sidebar filter options hook.
- `rtk git diff --check` passed after the sidebar filter options cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/task/sidebar-filter.spec.ts tests/task/task-list-filters.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 22 desktop Docker tests after the sidebar filter options cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/task/mobile-task-list-search.spec.ts`
  passed 1 mobile Docker test after the sidebar filter options cleanup.
- RED task mention metadata gate:
  `cd apps/web && rtk pnpm test components/task/chat/task-mention-items.test.ts hooks/use-message-handler.test.ts components/task/passthrough-chat-composer.test.ts`
  failed because mention helpers still dereferenced `state.kanban` /
  `kanbanMulti.snapshots`, and passthrough still required a store `getState`
  callback for task mention context.
- `rtk make fmt` passed after the task mention metadata cleanup.
- `cd apps/web && rtk pnpm test components/task/chat/task-mention-items.test.ts hooks/use-message-handler.test.ts components/task/passthrough-chat-composer.test.ts`
  passed 3 files / 19 tests after the task mention metadata cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the task mention metadata
  cleanup.
- Targeted eslint for the task mention helpers, TipTap chat input, normal chat
  send, and passthrough composer files passed after the cleanup.
- The stale server-state scan for `kanbanMulti`, active `state.kanban` task/step
  reads, and task-mention context builders receiving `getState()` returned no
  matches in the touched mention files.
- `rtk git diff --check` passed after the task mention metadata cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/chat/message-add-ws-gap.spec.ts tests/cli-mode/passthrough-toolbar.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 10 desktop Docker tests after the task mention metadata cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/chat/mobile-message-add-ws-gap.spec.ts tests/cli-mode/mobile-passthrough-composer.spec.ts`
  passed 3 mobile Docker tests after the task mention metadata cleanup.
- RED recent task switcher gate:
  `cd apps/web && rtk pnpm test components/task/recent-task-switcher-model.test.ts`
  failed with `ctx.kanbanSteps is not iterable` after the test context removed
  legacy kanban fields and provided live task metadata only through workflow
  snapshots.
- `rtk make fmt` passed after the recent task switcher cleanup.
- `cd apps/web && rtk pnpm test components/task/recent-task-switcher-model.test.ts components/task/recent-task-switcher-keys.test.ts`
  passed 2 files / 17 tests after the recent task switcher cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the recent task switcher
  cleanup.
- Targeted eslint for the recent switcher hook/model/tests passed after the
  cleanup.
- The stale server-state scan for active `state.kanban`, `kanbanMulti`, and old
  recent-switcher `kanban*` context fields returned no matches.
- `rtk git diff --check` passed after the recent task switcher cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/task/recent-task-switcher.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 4 desktop Docker tests after the recent task switcher cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/task/mobile-task-list-search.spec.ts`
  passed 1 mobile Docker test after the recent task switcher cleanup.
- RED mobile session switcher gate:
  `rtk pnpm --dir apps/web test components/task/mobile/session-task-switcher-sheet-hooks.test.tsx`
  failed because the sheet still ignored Query-owned snapshots when legacy
  `kanban` / `kanbanMulti` mirrors were empty, selected no primary session from
  Query task metadata, wrote created tasks only into the legacy kanban mirror,
  and hydrated `state.kanban` during workspace switch.
- `rtk make fmt` passed after the mobile session switcher cleanup.
- `rtk pnpm --dir apps/web test components/task/mobile/session-task-switcher-sheet-hooks.test.tsx`
  passed 1 file / 4 tests after the mobile session switcher cleanup.
- `rtk pnpm --dir apps/web typecheck` passed after the mobile session switcher
  cleanup.
- Targeted eslint for the mobile session switcher hook/test passed after the
  cleanup.
- The stale server-state scan for active `state.kanban`, `kanbanMulti`,
  `setWorkflowSnapshot`, and `findTaskInSnapshots` returned no matches in the
  mobile session switcher hook.
- `rtk git diff --check` passed after the mobile session switcher cleanup.
- `rtk pnpm --dir apps/web e2e:docker --project=mobile-chrome tests/task/mobile-sidebar-subtasks.spec.ts tests/task/mobile-task-list-search.spec.ts`
  passed 2 mobile Docker tests after the mobile session switcher cleanup.
- `rtk pnpm --dir apps/web e2e:docker tests/system/ws-event-accounting.spec.ts`
  passed 1 desktop Docker WS accounting test after the mobile session switcher
  cleanup.
- RED legacy kanban WS mirror cleanup gate:
  `rtk pnpm --dir apps/web test lib/ws/router.test.ts lib/ws/handlers/tasks.test.ts lib/ws/handlers/agent-session-kanban.test.ts`
  failed because `kanban.update` was still registered, `task.updated` /
  `task.deleted` still mutated legacy kanban mirrors, and
  `session.state_changed` still patched task primary state into those mirrors.
- RED workspace deletion mirror gate:
  `rtk pnpm --dir apps/web test lib/ws/handlers/workspaces.test.ts` failed
  because deleting the active workspace still cleared `state.kanban`.
- Legacy kanban WS mirror cleanup moved `kanban.update`,
  `task.updated`/`task.deleted`, `session.state_changed`, and
  `workspace.deleted` server-state writes out of Zustand. The remaining
  `task.*` WS handler keeps only client-local side effects: active
  primary-session adoption, deleted-task local storage cleanup, sidebar
  preference cleanup, and context file cleanup.
- `rtk make fmt` passed after the legacy kanban WS mirror cleanup.
- `rtk pnpm --dir apps/web test lib/ws/router.test.ts lib/ws/handlers/tasks.test.ts lib/ws/handlers/agent-session.test.ts lib/ws/handlers/agent-session-kanban.test.ts lib/ws/handlers/workspaces.test.ts lib/query/bridge/tasks.test.ts lib/query/bridge/index.test.ts lib/query/bridge/workspace.test.ts`
  passed 8 files / 50 tests after the legacy kanban WS mirror cleanup.
- `rtk pnpm --dir apps/web typecheck` passed after the legacy kanban WS mirror
  cleanup.
- Targeted eslint for the changed WS handlers/router/tests and bridge tests
  passed, with the existing max-lines warning in
  `lib/query/bridge/index.test.ts`.
- Production stale scans for `registerKanbanHandlers`,
  `lib/ws/handlers/kanban`, direct WS `state.kanban` /
  `state.kanbanMulti`, `setWorkflowSnapshot`, and
  `syncKanbanPrimarySessionState` returned no production matches; remaining
  matches are regression assertions in WS handler tests.
- `rtk git diff --check` passed after the legacy kanban WS mirror cleanup.
- `rtk pnpm --dir apps/web e2e:docker tests/task/task-list.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/session/session-tab-management.spec.ts tests/session/session-handoff.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 20 desktop Docker tests after the legacy kanban WS mirror cleanup.
- `rtk pnpm --dir apps/web e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/task/mobile-sidebar-subtasks.spec.ts tests/session/mobile-handoff.spec.ts`
  passed 13 mobile Docker tests after the legacy kanban WS mirror cleanup.
- RED final store compatibility gate:
  `rtk pnpm --dir apps/web test lib/routing/kanban-route-hydration.test.ts lib/state/default-state.test.ts lib/state/slices/kanban/kanban-slice.test.ts lib/query/seed.test.ts`
  failed because route readiness, default-state merging, the kanban slice, and
  query seed still accepted legacy `kanban` / `kanbanMulti` shapes.
- `rtk make fmt` passed after the final store compatibility cleanup.
- `rtk pnpm --dir apps/web test lib/state/default-state.test.ts lib/kanban/find-task.test.ts components/task/task-session-sidebar-aggregate.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.tsx hooks/domains/kanban/use-kanban-data.test.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx components/app-sidebar/sections/tasks-section.test.tsx components/session-commands.test.tsx hooks/use-task-removal.test.ts lib/query/seed.test.ts lib/routing/kanban-route-hydration.test.ts lib/state/slices/kanban/kanban-slice.test.ts hooks/use-tasks.test.ts components/task/mobile/session-task-switcher-sheet-hooks.test.tsx lib/ws/handlers/tasks.test.ts lib/ws/handlers/agent-session-kanban.test.ts lib/ws/handlers/workspaces.test.ts components/kanban-board-grid.test.tsx components/kanban-card-content.test.tsx`
  passed 18 files / 85 tests after the final store compatibility cleanup.
- `rtk pnpm --dir apps/web typecheck` passed after the final store
  compatibility cleanup.
- Targeted eslint for the final store cleanup files, Query seed/routing,
  sidebar/session utility tests, and touched E2E specs passed.
- Production stale scans for active `state.kanban`, `kanbanMulti`,
  `setWorkflowSnapshot`, deleted kanban actions, and E2E store kanban access
  returned no production matches; remaining matches are client-only
  `kanbanViewMode` / `kanbanPreviewedTaskId` and absence assertions.
- `rtk git diff --check` passed after the final store compatibility cleanup.
- `rtk pnpm --dir apps/web e2e:docker tests/task/task-list.spec.ts tests/kanban/kanban-board.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/session/session-tab-management.spec.ts tests/system/ws-event-accounting.spec.ts tests/kanban/preview-primary-session.spec.ts`
  passed 20 desktop Docker tests after the final store compatibility cleanup.
- `rtk pnpm --dir apps/web e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/task/mobile-sidebar-subtasks.spec.ts tests/session/mobile-handoff.spec.ts`
  passed 13 mobile Docker tests after the final store compatibility cleanup.
- `rtk pnpm --dir apps/web test lib/query/seed.test.ts lib/query/bridge/index.test.ts components/task/new-session-dialog.test.tsx components/task/new-subtask-dialog.test.tsx lib/ws/handlers/agent-session.test.ts lib/state/slices/features/features-slice.test.ts components/app-sidebar/app-sidebar-workspace-picker.test.tsx src/boot-payload.test.ts`
  passed 8 files / 73 tests after the feature flags and session worktrees
  cleanup.
- `rtk pnpm --dir apps/web typecheck` passed after the feature flags and session
  worktrees cleanup.
- Focused stale scans for removed feature/worktree store symbols, bridge audit
  migration wording, removed Office store fields/actions, removed workspace/
  kanban/repository/session-runtime store APIs, and deleted legacy WS handler
  registrations returned no production server-state matches.
- `rtk pnpm --dir apps/web test hooks/domains/session/use-queue.test.ts lib/query/bridge/index.test.ts`
  passed 2 files / 23 tests after renaming the Query-only queue cache helper to
  avoid stale removed-action audit matches.
- `rtk pnpm --dir apps/web exec eslint --max-warnings 0 hooks/domains/session/use-queue.ts`
  passed.
- `rtk pnpm --dir apps/web e2e:docker tests/session/new-session-dialog.spec.ts tests/task/file-tree-lazy-load.spec.ts tests/chat/message-queue.spec.ts tests/office/sidebar-office-gating.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 18 Docker tests with strict WS accounting for the final Task 10
  cleanup gate.
- Office routing cleanup:
  - `rtk pnpm --dir apps/web test hooks/domains/office/use-workspace-routing.test.tsx hooks/domains/office/use-routing-query-hooks.test.tsx`
    passed 2 files / 7 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web exec eslint --max-warnings 0 hooks/domains/office/use-workspace-routing.ts hooks/domains/office/use-provider-health.ts hooks/domains/office/use-routing-preview.ts hooks/domains/office/use-agent-route.ts hooks/domains/office/use-run-attempts.ts hooks/domains/office/use-workspace-routing.test.tsx hooks/domains/office/use-routing-query-hooks.test.tsx app/office/agents/agents-page-client.tsx app/office/agents/components/agent-card.tsx`
    passed.
  - `rtk apps/web/e2e/scripts/run-e2e.sh --docker --no-build --project routing -- e2e/tests/office/office-routing-disabled.spec.ts e2e/tests/office/office-routing-fallback.spec.ts e2e/tests/office/office-routing-agent-override.spec.ts e2e/tests/office/office-routing-recovery.spec.ts`
    passed 6 Docker routing-project tests with strict WS accounting.
  - Full verify passed before commit: `rtk make fmt`,
    `rtk node apps/web/scripts/generate-release-notes.mjs`,
    `rtk node apps/web/scripts/generate-changelog.mjs`, `rtk make typecheck`,
    `rtk make test`, and `rtk make lint`.
- Office shell/sidebar cleanup:
  - `rtk pnpm --dir apps/web test app/office/page-client.test.tsx components/app-sidebar/app-sidebar-primary-nav.test.tsx components/app-sidebar/sections/office-navigation-section.test.tsx components/app-sidebar/sections/agents-section.test.tsx components/app-sidebar/sections/projects-section.test.tsx`
    passed 5 files / 10 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web exec eslint --max-warnings 0 app/office/page-client.tsx app/office/page-client.test.tsx app/office/agents/agents-page-client.tsx components/app-sidebar/app-sidebar-primary-nav.tsx components/app-sidebar/app-sidebar-primary-nav.test.tsx components/app-sidebar/sections/office-navigation-section.tsx components/app-sidebar/sections/office-navigation-section.test.tsx components/app-sidebar/sections/agents-section.tsx components/app-sidebar/sections/agents-section.test.tsx components/app-sidebar/sections/projects-section.tsx components/app-sidebar/sections/projects-section.test.tsx`
    passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/dashboard.spec.ts tests/office/agents.spec.ts tests/office/projects.spec.ts tests/office/sidebar-navigation.spec.ts tests/office/realtime-dashboard.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 30 desktop Docker tests / 1 skipped with strict WS accounting.
- Office inbox/activity bridge cleanup:
  - `rtk pnpm --dir apps/web test lib/query/bridge/index.test.ts app/office/inbox/inbox-page-client.test.tsx app/office/workspace/activity/activity-row.test.tsx`
    passed 3 files / 19 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web exec eslint --max-warnings 0 lib/query/bridge/office.ts lib/query/bridge/index.test.ts app/office/inbox/inbox-page-client.tsx app/office/inbox/inbox-page-client.test.tsx app/office/workspace/activity/activity-feed.tsx`
    passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/inbox.spec.ts tests/office/realtime-inbox.spec.ts tests/office/approval-flow.spec.ts tests/office/activity-page.spec.ts tests/office/comment-run-status.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 15 Docker tests with strict WS accounting.
- Office meta cleanup:
  - `rtk pnpm --dir apps/web test hooks/domains/office/use-office-data.test.tsx app/office/inbox/inbox-page-client.test.tsx app/office/agents/[id]/components/agent-configuration-tab.test.tsx`
    passed 3 files / 7 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web exec eslint --max-warnings 0 hooks/domains/office/use-office-data.ts hooks/domains/office/use-office-data.test.tsx src/office-routes.tsx app/office/setup/step-agent.tsx app/office/setup/step-review.tsx app/office/inbox/inbox-item-row.tsx app/office/inbox/inbox-page-client.test.tsx app/office/tasks/use-tasks-tree.ts app/office/tasks/task-board.tsx app/office/tasks/task-filters.tsx app/office/projects/create-project-dialog.tsx app/office/projects/project-card.tsx app/office/workspace/skills/skill-detail.tsx app/office/workspace/skills/create-skill-form.tsx app/office/agents/[id]/components/agent-configuration-tab.tsx app/office/agents/[id]/components/agent-configuration-tab.test.tsx app/office/agents/[id]/components/agent-permissions-tab.tsx app/office/agents/[id]/components/agent-overview-tab.tsx app/office/agents/components/agent-status-dot.tsx app/office/agents/components/agent-role-badge.tsx app/office/agents/components/create-agent-dialog.tsx app/office/routines/run-row.tsx app/office/components/new-task-bottom-bar.tsx lib/state/store.ts lib/state/slices/office/types.ts lib/state/slices/office/office-slice.ts`
    passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/onboarding.spec.ts tests/office/task-filters.spec.ts tests/office/projects.spec.ts tests/office/agents.spec.ts tests/office/skills.spec.ts tests/office/routines.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 35 Docker tests / 1 skipped with strict WS accounting.
- Office bridge parity cleanup:
  - `rtk pnpm --dir apps/web test lib/query/bridge/index.test.ts`
    passed 1 file / 21 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web exec eslint --max-warnings 0 lib/query/bridge/office.ts lib/query/bridge/index.test.ts`
    passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/comment-run-status.spec.ts tests/office/agent-dashboard.spec.ts tests/office/agent-run-detail.spec.ts tests/office/realtime-tasks.spec.ts tests/office/projects.spec.ts tests/office/approval-flow.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 18 Docker tests / 1 skipped with strict WS accounting.
  - `rtk apps/web/e2e/scripts/run-e2e.sh --docker --no-build --project routing -- e2e/tests/office/office-routing-disabled.spec.ts e2e/tests/office/office-routing-fallback.spec.ts e2e/tests/office/office-routing-agent-override.spec.ts e2e/tests/office/office-routing-recovery.spec.ts`
    passed 6 Docker routing-project tests with strict WS accounting.
- Office page refetch cleanup:
  - `rtk pnpm --dir apps/web test lib/query/bridge/index.test.ts`
    passed 1 file / 21 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web exec eslint --max-warnings 0 lib/query/bridge/office.ts lib/query/bridge/index.test.ts app/office/projects/projects-page-client.tsx app/office/agents/[id]/dashboard/dashboard-view.tsx app/office/agents/[id]/components/agent-runs-tab.tsx app/office/agents/[id]/layout.tsx app/office/routines/routines-page-client.tsx`
    passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/agent-dashboard.spec.ts tests/office/agent-run-detail.spec.ts tests/office/projects.spec.ts tests/office/routines.spec.ts tests/office/realtime-tasks.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 15 Docker tests / 1 skipped with strict WS accounting.
- Office task list/detail cleanup:
  - `rtk pnpm --dir apps/web test app/office/tasks/use-paginated-tasks.test.tsx hooks/use-optimistic-task-mutation.test.tsx lib/query/bridge/index.test.ts`
    passed 3 files / 26 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web lint` passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/tasks.spec.ts tests/office/realtime-tasks.spec.ts tests/office/task-filters.spec.ts tests/office/task-sorting.spec.ts tests/office/topbar-breadcrumb.spec.ts tests/office/comment-input.spec.ts tests/office/simple-advanced-toggle.spec.ts tests/office/regression-fixes.spec.ts tests/office/property-pickers.spec.ts`
    passed 36 Docker tests with strict WS accounting.
- Office task helper/scaffold cleanup:
  - `rtk pnpm --dir apps/web test components/task/simple/components/blockers-picker.test.tsx hooks/use-optimistic-task-mutation.test.tsx app/office/tasks/use-paginated-tasks.test.tsx lib/query/bridge/index.test.ts lib/ws/router.test.ts lib/ws/handlers/agent-session.test.ts components/state-hydrator.test.tsx`
    passed 7 files / 52 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web lint` passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/tasks.spec.ts tests/office/realtime-tasks.spec.ts tests/office/task-filters.spec.ts tests/office/task-sorting.spec.ts tests/office/topbar-breadcrumb.spec.ts tests/office/comment-input.spec.ts tests/office/simple-advanced-toggle.spec.ts tests/office/regression-fixes.spec.ts tests/office/property-pickers.spec.ts tests/office/projects.spec.ts tests/office/agent-run-detail.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 43 Docker tests / 1 skipped with strict WS accounting.
- Office simple-pane reference-data cleanup:
  - `rtk pnpm --dir apps/web test components/task/simple/components/agents-multi-picker.test.tsx components/task/simple/components/blockers-picker.test.tsx components/task/simple/components/status-picker.test.tsx components/task/simple/components/priority-picker.test.tsx components/task/simple/components/approval-action-bar.test.tsx`
    passed 5 files / 17 tests / 4 skipped.
  - `rtk rg -n "office\\.agentProfiles|office\\.projects|office\\.routines|office\\.skills|setOfficeAgentProfiles|setProjects|setRoutines|setSkills" apps/web/components/task/simple --glob '!dist/**' --glob '!**/*.test.*'`
    returned no matches.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web lint` passed.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/property-pickers.spec.ts tests/office/comment-input.spec.ts tests/office/simple-advanced-toggle.spec.ts tests/office/regression-fixes.spec.ts tests/office/tasks.spec.ts tests/office/realtime-tasks.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 30 Docker tests with strict WS accounting.
- Final Office store cleanup:
  - Moved agent detail/routes, project detail and writes, create-agent/create-project
    flows, routines, workspace skills, org chart, new-task reference selectors,
    Office route bootstrap, and costs boot data to Office Query caches.
  - The Office Zustand slice now retains only task filter/sort/view/grouping/
    nesting UI state.
  - `rtk pnpm --dir apps/web test hooks/domains/office/use-office-data.test.tsx app/office/agents/[id]/components/agent-configuration-tab.test.tsx app/office/agents/[id]/components/agent-runs-tab.test.tsx app/office/components/new-task-dialog.test.tsx app/office/workspace/org/org-tree-layout.test.ts app/office/page-client.test.tsx lib/query/seed.test.ts components/state-hydrator.test.tsx lib/query/bridge/index.test.ts components/task/simple/components/pending-approval-badge.test.tsx`
    passed 10 files / 68 tests.
  - `rtk pnpm --dir apps/web typecheck` passed.
  - `rtk pnpm --dir apps/web lint` passed.
  - Stale scans for removed Office store fields/actions returned no production
    server-state readers/writers; remaining `office.tasks.*` matches are
    client-only task filter/sort/view/grouping/nesting state.
  - `rtk pnpm --dir apps/web e2e:docker tests/office/agents.spec.ts tests/office/agent-subroutes.spec.ts tests/office/agent-roles.spec.ts tests/office/agent-skills-ui.spec.ts tests/office/permissions.spec.ts tests/office/projects.spec.ts tests/office/project-repository-picker.spec.ts tests/office/routines.spec.ts tests/office/routines-ui.spec.ts tests/office/routine-fire.spec.ts tests/office/skills.spec.ts tests/office/system-skills.spec.ts tests/office/skills-readonly.spec.ts tests/office/org-chart.spec.ts tests/office/execution-stages.spec.ts tests/office/costs.spec.ts tests/system/ws-event-accounting.spec.ts`
    passed 67 Docker tests / 5 skipped with strict WS accounting.

Current next step: run Task 11 strict QA.

Wave 4 (cleanup and full verification):

- [x] [task-10-remove-zustand-server-state](task-10-remove-zustand-server-state.md) — done
- [ ] [task-11-e2e-strict-qa](task-11-e2e-strict-qa.md) — in progress

## Open Questions

- None blocking. The user requested a one-shot migration PR; this plan treats
  feature-flagged incremental rollout as out of scope unless review discovers a
  domain that cannot safely migrate in the same PR.
