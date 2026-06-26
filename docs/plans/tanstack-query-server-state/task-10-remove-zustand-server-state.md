---
id: "10-remove-zustand-server-state"
title: "Remove Zustand server state"
status: in_progress
wave: 4
depends_on:
  [
    "05-workspace-kanban-settings",
    "06-office-domain",
    "07-session-domain",
    "08-session-runtime-streams",
    "09-integrations-automations-system",
  ]
plan: "plan.md"
spec: "../../specs/ui/tanstack-query-server-state.md"
---

# Task 10: Remove Zustand Server State

## Acceptance

- Old server-state Zustand fields/actions and WS handlers are removed after
  their readers have migrated.
- Remaining Zustand fields are documented as client-only or temporary indexes.
- `apps/web/AGENTS.md` accurately describes the new data flow.

## Progress

Completed cleanup sub-waves:

- **Office routing cleanup:** moved provider health, routing preview,
  workspace routing, agent route, and run attempts hook reads to TanStack Query
  only. `AgentCard` now receives routing data from query-backed page hooks
  instead of reading `office.routing.*` mirrors. Verified with focused hook
  tests, typecheck/lint, full verify, and the Docker `routing` project gate.
- **Office shell/sidebar cleanup:** moved the app sidebar primary inbox badge,
  Office navigation counters, sidebar agent/project sections, Office dashboard
  page, and Office agents page off `office.dashboard`, `office.inbox*`,
  `office.agentProfiles`, and `office.projects` store mirrors. These paths now
  read `qk.office.dashboard`, `qk.office.inbox`, `qk.office.agents`, and
  `qk.office.projects`; active workspace/sidebar/session state remains in
  Zustand.
- **Office inbox/activity bridge cleanup:** moved the Inbox page off
  `office.inboxItems`, removed the Activity page's old `useOfficeRefetch`
  subscription, and added Query bridge invalidation for task comments on
  `office.run.queued` and `office.run.processed` so comment run-status badges
  are covered without the legacy comments trigger.
- **Office meta cleanup:** moved all status/priority/role/executor/project/
  inbox/routine/skill metadata readers to `qk.office.meta()` through
  `useOfficeMetaData`, seeded the meta query during Office route bootstrap, and
  removed the unused `setMeta` Zustand action. The temporary `office.meta` field
  remains only as part of the broader Office state shape/query seed scaffold
  until the final Office slice removal wave.
- **Office bridge parity cleanup:** filled Query bridge invalidation gaps for
  task-linked project counts/details, task activity, agent summaries, agent run
  lists/details, run-driven dashboard data, approval-driven agent data, and
  agent route/routing updates. This unblocks later removal of page-level
  `useOfficeRefetch` subscriptions that were covering those surfaces.

Remaining cleanup:

- Office routines, costs, skills, deeper agent/project/task helper components,
  and remaining `useOfficeRefetch`/`registerOfficeHandlers` fanout still have
  store readers or writers. Task 10 stays `in_progress` until those paths are
  removed or explicitly documented as client-only temporary indexes.

- **System:** removed the system Zustand slice and `system-events` WS handler.
  System hooks and topbar metrics now read Query caches/options directly. The
  local `systemHealth` UI slice remains.
- **GitHub:** removed server-backed GitHub Zustand fields and the old GitHub WS
  handler. Query now owns status, task/workspace PRs, task CI options, watches,
  action presets, and rate-limit bridge updates. Local-only
  `pendingPrUrlByTaskId` and `prFeedbackCache` remain in Zustand.
- **GitLab:** removed the GitLab Zustand slice entirely. Status, stats,
  workspace/task MRs, review watches, issue watches, and action presets now read
  and mutate TanStack Query caches directly. `/gitlab` and GitLab settings use
  `useGitLabStatus` instead of direct status fetch effects.
- **Settings leaf lists:** converted prompts, secrets, sprites,
  notification providers, available agents, agent discovery, and editors hooks
  to Query-only readers. Prompts, secrets, editor mutations, agent refreshes,
  profile status panels, and prompt previews now patch/read Query caches instead
  of these Zustand mirrors.
- **Settings catalog/bootstrap:** converted executors, settings agents, derived
  agent profiles, install jobs, settings route bootstrap, settings layout, and
  session boot preload to Query seed/query reads. Settings agent and executor
  write paths now patch `qk.settings.*` caches directly through small sync
  helpers. Removed the old `agents`, `executors`, and `executor-profiles` WS
  handlers after their store readers/writers were gone. The settings Zustand
  slice now contains only `userSettings`.
- **Workspace/kanban direct-fetch cleanup:** added a workflow-step Query key and
  query option, moved the shared `useWorkflowSteps` hook, task-create workflow
  step effect, and automations config workflow-step picker to Query, and made
  `workflow.step.*` bridge events invalidate the workflow-step cache. Removed
  unused workspace metadata maps (`repositories` loaded/loading flags,
  repository branch loaded/loading/fetchedAt/fetchError flags, and repository
  script loaded/loading flags) from the Zustand slice, boot payloads, SSR
  session state, and old writer actions. The actual workspace, workflow,
  `kanban`, and `kanbanMulti` entity mirrors remain
  because mounted UI readers still use them as migration fallbacks. The mobile
  task switcher workspace-change path now fetches workflows and the first
  workflow snapshot through Query options instead of direct API imports. The
  task-detail route's active-task switch path now reads `taskQueryOptions`
  instead of running a component-owned `fetchTask` effect. Command-panel task
  search now reads `workspaceTasksQueryOptions` instead of importing
  `listTasksByWorkspace` directly, preserving active-task filtering and
  archived-last search ordering. Archive/delete dialog subtask counts now
  fetch through `queryClient.fetchQuery(subtaskCountQueryOptions(...))` while
  keeping the existing no-stale-count-on-reopen behavior.
- **Session dead-code cleanup:** removed production-unused `clearTaskPlan` and
  `clearQueueStatus` Zustand actions and their action-only tests, and removed
  the legacy no-op `task.plan.reverted` registration from the old `task-plans`
  WS handler. The Query bridge remains the owner for `task.plan.reverted`.
  Session/chat/plan/queue mirrors remain because mounted readers still use
  them.
- **Queue mirror cleanup:** moved `useQueue` to read/write
  `qk.session.queue` directly, with Query-cache optimistic removal and
  mutation-local loading state. Removed the old queue Zustand state/actions,
  default-state/root-store declarations, action-only queue tests, and the
  duplicate `message.queue.status_changed` registration from the old
  `agent-session` WS handler. Queue status WS updates now flow through the
  Query bridge only.
- **Plan context cleanup:** moved the `@Plan` context preview and passthrough
  composer plan expansion to Query-only reads. `LazyPlanPreview` no longer
  requires `StateProvider` or mirrors fetched plans into `taskPlans`; passthrough
  message composition reads `taskPlanQueryOptions` cache before fetching and no
  longer consults `state.taskPlans` for plan context content. The main plan
  panel hook and old `task-plans` WS handler remain because they still own
  plan editing/seen-state behavior.
- **Session todos/prompt usage cleanup:** moved `useSessionTodoItems` to the
  `qk.sessionRuntime.todos` Query cache only and removed the old
  `session.todos_updated` and `session.prompt_usage` Zustand WS handlers,
  state fields, mutators, default/root-store declarations, and boot seed paths.
  Todo and prompt usage WS updates now flow through the Query bridge only.
- **Agent capabilities/poll mode cleanup:** moved the task-list debug poll badge
  to the `qk.sessionRuntime.pollMode` Query cache only and removed the unused
  auth-methods indicator plus its orphaned frontend authenticate helper. Removed
  the old `session.agent_capabilities` and `session.poll_mode_changed` Zustand
  WS handlers, state fields, mutators, default/root-store declarations, and boot
  seed paths. These runtime WS updates now flow through the Query bridge only.
- **Improve Kandev/workflow cleanup:** moved the Improve Kandev bootstrap
  follow-up fetches for workflow steps and workspace repositories through the
  shared Query option factories, while keeping the temporary repository store
  sync for existing task-create readers. Removed the old `workflow.*` and
  `workflow.step.*` Zustand WS handler registration and deleted the legacy
  workflow handler/test file; workflow cache invalidation now flows through the
  Query bridge only. `kanban.update` and `task.*` legacy handlers remain because
  mounted kanban/task readers and task-delete cleanup side effects still depend
  on them.
- **Session runtime dead-plumbing cleanup:** removed production-unused
  `pendingModel`, `sessionRuntime.agents`, and the legacy `terminal.output`
  buffer/action/WS branch from Zustand and store hydration/re-export plumbing.
  The active `session.shell.output`, `session.process.output`, and
  `session.process.status` terminal flows remain. Backend protocol types and
  bridge audit skip metadata for `terminal.output` remain because the protocol
  still defines that stream event, but it no longer writes frontend store state.
- **Session mode cleanup:** moved the session mode selector to Query-only live
  mode data, retaining its existing session snapshot/profile fallback before any
  live mode event arrives. Removed the old `sessionMode` Zustand state/actions,
  boot seed/store plumbing, and legacy `session.mode_changed` WS handler.
  Session mode changes now update `qk.sessionRuntime.mode` through the Query
  bridge only; the `setSessionMode` API remains the user action for changing
  mode.
- **Repository scripts cleanup:** moved `useRepositoryScripts` to Query-only
  data and removed the old `repositoryScripts` Zustand state/actions,
  AppState/default-state plumbing, and store re-exports. SSR task data still
  carries repository scripts as a query-seed-only boot shape, and settings
  repository script saves now update `qk.workspaces.repositoryScripts(repoId)`
  directly instead of clearing a store mirror.
- **Task-plan revision cleanup:** moved task-plan revision list/content reads to
  TanStack Query only and removed the old `revisionsByTaskId`,
  `revisionsLoadingByTaskId`, `revisionsLoadedByTaskId`,
  `revisionContentCache`, and revision mutator actions from the session slice.
  The legacy `task.plan.revision.created` WS handler was removed; the Query
  bridge patches the revisions list and invalidates the individual revision
  detail cache. The main task-plan mirror remains for plan editing,
  last-seen/indicator, and layout auto-open behavior.
- **Available commands cleanup:** moved inline slash commands, TipTap slash
  suggestions, chat-panel agent command data, and empty-turn command
  recognition to Query-only reads. Removed the old `availableCommands`
  Zustand state/actions and legacy `session.available_commands` WS handler.
  E2E command seeding now writes directly to
  `qk.sessionRuntime.availableCommands(sessionId)`. Empty-turn local notices
  are preserved across API refetch snapshots so Query invalidation cannot drop
  the synthetic hint message. The mobile E2E page object now scopes chat editor
  and send-button locators to the visible `session-chat` panel instead of
  relying on page-wide TipTap DOM order.
- **Repository branches cleanup:** moved `useBranches` to Query-only reads and
  removed the old `repositoryBranches` Zustand state/action, root-store/default
  state declarations, hydration, overrides, and re-export plumbing. Row-level
  branch lists now cold-load through the workspace branch query so
  provider-backed URL repositories still list branches when re-picked from the
  workspace dropdown. Manual refresh forces the repository
  `?refresh=true` endpoint and copies the result into the active workspace
  branch cache. Stale task-create comments and `apps/web/AGENTS.md` were
  updated so the docs no longer describe a removed branch store fallback.
- **Workspace repositories cleanup:** moved workspace repository list reads to
  TanStack Query cache only and removed the old `repositories` Zustand
  state/action, root-store/default-state declarations, hydration, overrides,
  and re-export plumbing. Boot/route repository lists are now query-seed-only
  data. Shared repository lookup hooks read all workspace repository query
  caches, and repository-heavy UI surfaces (task-create, mobile task switcher,
  recent switcher, kanban cards, changes/review panels, quick chat, dockview
  repository scripts, and sidebar filters) now read Query cache instead of the
  workspace slice.
- **Workflow list cleanup:** moved workspace workflow list reads to TanStack
  Query cache only and removed `workflows.items`, `setWorkflows`, and
  `reorderWorkflowItems` from the kanban slice/root store. Boot and route
  workflow lists now seed `qk.workflows.all(workspaceId, { includeHidden:
true })` through `workflowLists.itemsByWorkspaceId`, while
  `workflows.activeId` remains in Zustand as UI/navigation state. Kanban
  workflow selection, move targets, swimlane ordering, task-create workflow
  resolution, settings workflow editing, task mentions, integration watch
  dialogs, recent/mobile task switchers, and route hydration now read workflow
  lists from Query cache. Swimlane workflow drag sorting now optimistically
  reorders workflow query caches before calling the backend reorder API.
- **All-workflow snapshot/loading cleanup:** moved all-workflow board/sidebar
  snapshot reads and loading state to workflow snapshot Query caches. The
  Kanban board now passes Query-owned snapshots to swimlanes, multi-select
  logic, and the bulk toolbar; `useWorkspaceSidebarTasks` consumes Query
  snapshots without the old active single-workflow store fallback. Removed the
  old `kanbanMulti.isLoading`, `setKanbanMultiLoading`, `updateMultiTask`, and
  `removeMultiTask` store surface. `kanbanMulti.snapshots`,
  `setWorkflowSnapshot`, and `clearKanbanMulti` remain temporary write-through
  compatibility for legacy direct readers and WS handlers outside this
  sub-slice.
- **Task detail lookup cleanup:** moved `useTask` and `useTaskById` to
  Query-only task-detail reads and removed their active `kanban.tasks` /
  `kanbanMulti.snapshots` fallback. Sender task badges now resolve live titles
  from `qk.tasks.detail(taskId)` and retain the snapshotted metadata fallback
  when no task detail is cached/fetched.
- **Task removal cleanup:** moved `useTaskRemoval` removal/switch source data
  to workflow snapshot Query caches and introduced a shared
  `workflow-snapshot-cache` helper used by both task removal and multi-select
  bulk operations. `useTaskRemoval` no longer reads `kanbanMulti.snapshots` or
  calls `setWorkflowSnapshot`; it keeps only a temporary active `kanban.tasks`
  cleanup write for remaining single-workflow mirror readers. Last-task removal
  now redirects home through the SPA router instead of `window.location.href`,
  preserving strict WS accounting hooks during E2E teardown.
- **Active workflow snapshot cleanup:** moved the active board read path to
  workflow snapshot Query caches. `useWorkflowSnapshot` no longer falls back to
  `state.kanban`; `useTasks`, `useKanbanData`, `KanbanBoard`, and
  `KanbanWithPreview` now derive active tasks, steps, loading, multi-select,
  and preview task lookup from `qk.workflows.snapshot(...)` /
  `useAllWorkflowSnapshots`. Boot and route hydration now seed workflow
  snapshot query keys from existing `kanban` / `kanbanMulti` payload shapes so
  first paint remains populated while the legacy mirror still exists. The
  Query bridge now patches workflow snapshot and task-detail
  `primary_session_state` on `session.state_changed`, replacing the active
  board's dependency on the old `syncKanbanPrimarySessionState` store merge.
- **Active board writer cleanup:** moved the live board optimistic writers to
  workflow snapshot Query caches. Dialog create/edit success now patches
  `qk.tasks.detail(...)` and `qk.workflows.snapshot(task.workflow_id)`;
  delete/archive success removes tasks from cached workflow snapshots; swimlane
  drag/drop uses the Query snapshot with rollback on failed moves. The unused
  legacy `useDragAndDrop` hook was deleted after moving its shared
  `MoveTaskError` type to `lib/kanban`, and the orphaned
  `swimlane-graph-content` file was removed after import scans confirmed it was
  dead. The used Kanban and Pipeline views now share the Query-backed
  `useSwimlaneMove` path.
- **Sidebar filter options cleanup:** moved workflow, workflow-step, and
  executor-type filter option lists to workflow snapshot Query caches via
  `useAllWorkflowSnapshots(activeWorkspaceId)`. The sidebar filter hook no
  longer reads `kanbanMulti.snapshots`; Zustand remains only for the active
  workspace UI selection in this path.
- **Task mention metadata cleanup:** moved `@task` mention menu construction
  and referenced-task prompt context expansion to workflow snapshot Query
  caches. `buildTaskMentionItems`, `buildTaskMentionsContext`, the TipTap chat
  input, normal chat send, and passthrough composer no longer read
  `state.kanban` / `kanbanMulti.snapshots` for task title/workflow/step
  metadata. The normal send path still writes created chat messages to the
  local session store for missed-frame resilience.
- **Recent task switcher cleanup:** moved recent task switcher live metadata
  resolution to workflow snapshot Query caches. The hook no longer reads active
  `state.kanban` tasks/steps/workflow id or `kanbanMulti.snapshots`; display
  titles, workflow names, step titles, task states, repositories, and session
  status are derived from Query snapshots plus the remaining session/client
  stores.
- **Mobile session switcher cleanup:** moved the mobile task switcher sheet's
  step data, task selection/archive/delete metadata lookups, task-created cache
  upsert, and workspace-switch snapshot fetch to workflow snapshot Query
  caches. The sheet hook no longer reads or writes active `state.kanban` or
  `kanbanMulti.snapshots`; it keeps only client UI/session stores for active
  task/session selection and pending message/session badges.
- **Mobile repo/session metadata cleanup:** moved mobile repo count/rows, repo
  pill active repo name, session primary marker, repo display name, and
  base-branch-by-repo readers to task-detail and repository Query caches. These
  mobile metadata paths no longer read active `state.kanban.tasks` or
  `kanbanMulti.snapshots`; legacy stores can be empty while cached task detail
  drives repository/session labels.
- **Desktop sidebar/removal cleanup:** moved active sidebar creation defaults,
  subtask parent labels, task-section workflow selection, sidebar archive/delete
  metadata, sidebar task selection metadata, sidebar move-to-step optimistic
  writes, and `useTaskRemoval` next-task/removal logic to Query-owned workflow
  snapshot and task-detail data. These paths no longer read or write active
  `state.kanban` / `kanbanMulti.snapshots`; the sidebar still keeps client UI
  state in Zustand.
- **Task-page/session chrome cleanup:** moved task-detail route metadata,
  workflow steps, session panel step resolution, header/sidebar new-task
  context, session primary markers, base-branch repository lookup, subtask
  defaults, plan-mode layout detection, and changes-panel multi-repo labels to
  Query caches. These task-page/session chrome paths no longer synthesize or
  override task metadata from active `state.kanban.tasks` /
  `kanbanMulti.snapshots`.
- **Board/command/dialog utility cleanup:** moved card context-menu workflow
  move targets, command-panel step filtering/badges, and session command
  subtask parent titles to Query caches. `useKanbanCardMoveTargets` reads
  workflow snapshot query cache, command-panel step derivation uses stable
  Query-owned snapshot data, and `SessionCommands` reads `useTaskById` instead
  of active `kanban.tasks`.
- **Task-create/card/GitHub indicator/board-grid cleanup:** moved session-mode
  task-create repository naming, task-create workflow snapshot defaults, kanban
  card subtask parent badges, GitHub PR indicator step tooltips, and board-grid
  loading decisions to Query-owned task detail, repository, workflow list, and
  workflow snapshot caches. These UI readers no longer depend on active
  `state.kanban` or `kanbanMulti.snapshots` mirrors being populated.
- **Legacy kanban WS mirror cleanup:** removed the old `kanban.update` Zustand
  handler registration and handler/test file. The remaining `task.*` WS handler
  now keeps only client-side side effects: active primary-session adoption,
  deleted-task local storage cleanup, sidebar preference cleanup, and context
  file cleanup. `session.state_changed` no longer patches active
  `state.kanban` / `kanbanMulti.snapshots`, and `workspace.deleted` no longer
  clears the legacy kanban mirror. Query bridge handlers now own workflow
  snapshot invalidation/patching for these server-state events.
- **Final kanban store compatibility cleanup:** removed the top-level
  `kanban` / `kanbanMulti` store fields, `setWorkflowSnapshot`, route/boot
  seed compatibility shapes, active snapshot fallback paths, stale test
  fixtures, and dead active-board fallback helpers. Route readiness now checks
  Query snapshot hydration directly, SSR/boot seeds use
  `workflowSnapshots.itemsByWorkflowId`, `mergeInitialState` allowlists known
  store fields, and stale scans find no production references to the removed
  store API.

Retained Zustand inventory:

- Client navigation/UI state: `tasks.activeTaskId`, `tasks.activeSessionId`,
  `workflows.activeId`, `workspaces.activeId`, preview/sidebar/mobile/dialog
  state, user settings, and persisted local preferences.
- Live session indexes: `messages`, `turns`, `taskSessions`,
  `taskSessionsByTask`, `taskPlans`, and `sessionAgentctl` remain for stream
  ordering, missed-frame recovery, plan editing/seen-state UI, active-session
  chrome, and local panel behavior while Query owns server snapshots and
  invalidation.
- Runtime stream indexes: `shell`, `processes`, `gitStatus`,
  `sessionCommits`, `contextWindow`, `prepareProgress`, `sessionModels`,
  `userShells`, and `environmentIdBySessionId` remain for high-frequency
  terminal/process/git/context/model updates and environment-scoped cleanup.
- Local integration/client indexes: GitHub `pendingPrUrlByTaskId` and
  `prFeedbackCache` remain local-only; office view/filter/sort/grouping state
  remains local UI state.

No GitLab-specific Docker E2E specs exist in this checkout. The GitLab sub-wave
used focused unit coverage plus the integration/settings/sidebar/strict-WS
Docker gate below; add GitLab browser specs before treating GitLab as fully
covered by domain-specific E2E.

## Verification

- `cd apps && pnpm --filter @kandev/web typecheck`
- `cd apps && pnpm --filter @kandev/web lint`
- `cd apps && pnpm --filter @kandev/web test`
- `cd apps/web && pnpm e2e:docker tests/task/task-list.spec.ts tests/kanban/kanban-board.spec.ts tests/chat/message-add-ws-gap.spec.ts tests/session/session-tab-management.spec.ts tests/office/dashboard.spec.ts tests/github/github-scope-bar.spec.ts tests/integrations/jira-settings.spec.ts tests/system/status-page.spec.ts`
- Repeat any Wave 3 Docker E2E gate whose migrated domain still had retained
  Zustand paths at the start of this cleanup task.

Completed Wave 4 sub-wave verification:

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
- `cd apps/web && rtk pnpm typecheck` passed after the workspace/kanban
  workflow-step and metadata cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/use-workflow-steps.ts hooks/use-workflow-steps.test.ts components/automations/config-section.tsx components/automations/config-section.test.tsx components/task-create-dialog-effects.ts components/task-create-dialog-effects.test.ts lib/query/query-options/kanban.ts lib/query/query-options/query-options.test.ts lib/query/bridge/workspace.ts lib/query/bridge/index.test.ts lib/query/bridge/workspace.test.ts lib/query/keys.ts lib/query/keys.test.ts hooks/domains/workspace/use-repositories.ts hooks/domains/workspace/use-repository-branches.ts hooks/domains/workspace/use-repository-scripts.ts hooks/domains/kanban/use-kanban-actions.ts lib/state/slices/workspace/types.ts lib/state/slices/workspace/workspace-slice.ts lib/state/slices/workspace/workspace-slice.test.ts app/page.tsx app/tasks/page.tsx app/github/page.tsx lib/ssr/session-page-state.ts lib/state/store.ts`
  passed.
- `cd apps/web && rtk pnpm test hooks/use-workflow-steps.test.ts components/task-create-dialog-effects.test.ts components/automations/config-section.test.tsx lib/query/query-options/query-options.test.ts lib/query/keys.test.ts lib/query/bridge/index.test.ts lib/query/bridge/workspace.test.ts lib/state/slices/workspace/workspace-slice.test.ts hooks/use-workflow-snapshot.test.ts hooks/use-tasks.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts lib/ws/handlers/kanban.test.ts lib/ws/handlers/tasks.test.ts lib/ws/handlers/workflows.test.ts lib/routing/kanban-route-hydration.test.ts`
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
- RED workflow-list gate:
  `cd apps/web && rtk pnpm test hooks/use-workflows.test.tsx lib/query/seed.test.ts lib/state/slices/kanban/kanban-slice.test.ts`
  failed on the retained Zustand dependency/state and missing workflow query
  seed path.
- `rtk make fmt` passed after the workflow-list cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the workflow-list cleanup.
- `cd apps/web && rtk pnpm exec eslint hooks/use-workflows.ts hooks/use-workflows.test.tsx hooks/use-workflow-cache.ts lib/query/seed.ts lib/query/seed.test.ts lib/state/slices/kanban/types.ts lib/state/slices/kanban/kanban-slice.ts lib/state/slices/kanban/kanban-slice.test.ts lib/state/default-state.ts lib/state/hydration/hydrator.ts lib/state/store.ts lib/state/slices/index.ts lib/state/store-reexports.ts app/page.tsx app/tasks/page.tsx app/github/page.tsx app/jira/page.tsx app/linear/page.tsx lib/ssr/session-page-state.ts src/spa-routes.tsx lib/routing/kanban-route-hydration.ts lib/routing/kanban-route-hydration.test.ts hooks/domains/settings/use-workflow-settings.ts hooks/domains/settings/use-workflow-settings.test.ts hooks/domains/kanban/use-all-workflow-snapshots.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts components/kanban-board.tsx hooks/domains/kanban/use-kanban-actions.ts hooks/domains/kanban/use-kanban-data.ts components/kanban/swimlane-container.tsx components/kanban-card-menu-items.tsx components/task-create-dialog-state.ts components/task-create-dialog-types.ts components/automations/config-section.tsx components/sentry/sentry-issue-watch-dialog.tsx components/jira/jira-issue-watch-dialog.tsx components/linear/linear-issue-watch-dialog.tsx components/github/issue-watch-dialog.tsx components/github/review-watch-dialog.tsx components/task/recent-task-switcher-hooks.ts components/task/mobile/session-task-switcher-sheet-hooks.ts components/task/chat/task-mention-items.ts components/task/chat/task-mention-items.test.ts components/task/chat/tiptap-input.tsx lib/ws/handlers/workspaces.ts components/kanban-display-dropdown.tsx components/kanban/mobile-menu-sheet.tsx hooks/use-kanban-display-settings.ts lib/kanban/resolve-workflow.ts lib/kanban/resolve-workflow.test.ts hooks/use-message-handler.test.ts`
  passed after the workflow-list cleanup.
- `cd apps/web && rtk pnpm test hooks/use-workflows.test.tsx lib/query/seed.test.ts lib/state/slices/kanban/kanban-slice.test.ts lib/routing/kanban-route-hydration.test.ts hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-workspace-sidebar-tasks.test.ts hooks/domains/settings/use-workflow-settings.test.ts components/task-create-dialog-state.test.ts components/task/chat/task-mention-items.test.ts components/task/recent-task-switcher-model.test.ts`
  passed 10 files / 60 tests after the workflow-list cleanup.
- `rtk rg -n -P "workflows\\.items|setWorkflows\\b|reorderWorkflowItems\\b|WorkflowsState\\[\"items\"\\]|workflows:\\s*\\{\\s*items|items:\\s*workflows\\.map" apps/web --glob '!dist/**' --glob '!e2e/test-results/**' --glob '!**/kanban-slice.test.ts'`
  returned no matches after the workflow-list cleanup.
- `rtk git diff --check` passed after the workflow-list cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/kanban/workflow-filter.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/workflow/workflow-sorting.spec.ts tests/task/create-task.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 24 desktop Docker tests after the workflow-list cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts tests/workflow/mobile-workflow-manual-move-queue.spec.ts tests/task/mobile-sidebar-subtasks.spec.ts tests/task/mobile-create-task-remote-repo.spec.ts`
  passed 14 mobile Docker tests after the workflow-list cleanup.
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
- `cd apps/web && rtk pnpm exec eslint hooks/use-task-crud.ts hooks/use-task-crud.test.tsx hooks/domains/kanban/use-kanban-actions.ts hooks/domains/kanban/use-kanban-actions.test.tsx hooks/domains/kanban/use-swimlane-move.ts hooks/domains/kanban/use-swimlane-move.test.tsx components/kanban/swimlane-kanban-content.tsx components/kanban/swimlane-container.tsx components/kanban-board.tsx lib/query/workflow-snapshot-cache.ts lib/kanban/move-task-error.ts lib/kanban/view-registry.ts`
  passed.
- `rtk rg -n "state\\.kanban|kanban\\.tasks|kanban\\.workflowId|hydrate\\(\\{\\s*kanban|kanbanMulti\\.snapshots|setWorkflowSnapshot" apps/web/hooks/domains/kanban/use-kanban-actions.ts apps/web/hooks/use-task-crud.ts apps/web/hooks/domains/kanban/use-swimlane-move.ts apps/web/components/kanban/swimlane-kanban-content.tsx apps/web/lib/query/workflow-snapshot-cache.ts`
  returned no production matches after the active board writer cleanup.
- `rtk rg -n "@/hooks/use-drag-and-drop|SwimlaneGraphContent|swimlane-graph-content" apps/web --glob '!dist/**' --glob '!e2e/test-results/**'`
  returned no matches after deleting the unused legacy hook and orphaned graph
  content.
- `rtk git diff --check` passed after the active board writer cleanup.
- `cd apps/web && rtk pnpm e2e:docker tests/kanban/kanban-board.spec.ts tests/kanban/card-menu-delete-archive.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/kanban/pipeline-view.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 19 desktop Docker tests after the active board writer cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project=mobile-chrome tests/kanban/mobile-kanban.spec.ts`
  passed 11 mobile Docker tests after the active board writer cleanup.
- RED sidebar filter options gate:
  `cd apps/web && rtk pnpm test components/task/sidebar-filter/use-filter-value-options.test.ts`
  failed because workflow, workflow-step, and executor-type options were still
  empty when the legacy `kanbanMulti.snapshots` mirror was empty but workflow
  snapshot Query caches were populated.
- `rtk make fmt` passed after the sidebar filter options cleanup.
- `cd apps/web && rtk pnpm test components/task/sidebar-filter/use-filter-value-options.test.ts`
  passed 1 file / 6 tests after the sidebar filter options cleanup.
- `cd apps/web && rtk pnpm typecheck` passed after the sidebar filter options
  cleanup.
- `cd apps/web && rtk pnpm exec eslint components/task/sidebar-filter/use-filter-value-options.ts components/task/sidebar-filter/use-filter-value-options.test.ts`
  passed after the sidebar filter options cleanup.
- `rtk rg -n "kanbanMulti\\.snapshots|state\\.kanban|s\\.kanban\\.tasks|s\\.kanban\\.steps|setWorkflowSnapshot" apps/web/components/task/sidebar-filter/use-filter-value-options.ts`
  returned no matches after the sidebar filter options cleanup.
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
- `cd apps/web && rtk pnpm exec eslint components/task/chat/task-mention-items.ts components/task/chat/task-mention-items.test.ts components/task/chat/tiptap-input.tsx hooks/use-message-handler.ts hooks/use-message-handler.test.ts components/task/chat/chat-input-area.tsx components/task/passthrough-chat-composer.tsx components/task/passthrough-chat-composer.test.ts`
  passed after the task mention metadata cleanup.
- `rtk rg -n "kanbanMulti|state\\.kanban|\\.kanban\\.(tasks|steps|workflowId)|buildTaskMentionsContext\\([^\\n]+getState|buildTaskMentionItems\\([^\\n]+getState" apps/web/components/task/chat/task-mention-items.ts apps/web/components/task/chat/tiptap-input.tsx apps/web/hooks/use-message-handler.ts apps/web/components/task/chat/chat-input-area.tsx apps/web/components/task/passthrough-chat-composer.tsx`
  returned no matches after the task mention metadata cleanup.
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
- `cd apps/web && rtk pnpm exec eslint components/task/recent-task-switcher-hooks.ts components/task/recent-task-switcher-model.ts components/task/recent-task-switcher-model.test.ts components/task/recent-task-switcher-keys.test.ts`
  passed after the recent task switcher cleanup.
- `rtk rg -n "state\\.kanban|kanbanMulti|kanbanTasks|kanbanSteps|kanbanWorkflowId" apps/web/components/task/recent-task-switcher-hooks.ts apps/web/components/task/recent-task-switcher-model.ts`
  returned no matches after the recent task switcher cleanup.
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
- `rtk pnpm --dir apps/web exec eslint components/task/mobile/session-task-switcher-sheet-hooks.ts components/task/mobile/session-task-switcher-sheet-hooks.test.tsx`
  passed after the mobile session switcher cleanup.
- `rtk rg -n "state\\.kanban|s\\.kanban|getState\\(\\)\\.kanban|kanbanMulti|setWorkflowSnapshot|findTaskInSnapshots|\\.kanban\\.(tasks|steps|workflowId|loading|error)" apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`
  returned no matches after the mobile session switcher cleanup.
- `rtk git diff --check` passed after the mobile session switcher cleanup.
- `rtk pnpm --dir apps/web e2e:docker --project=mobile-chrome tests/task/mobile-sidebar-subtasks.spec.ts tests/task/mobile-task-list-search.spec.ts`
  passed 2 mobile Docker tests after the mobile session switcher cleanup.
- `rtk pnpm --dir apps/web e2e:docker tests/system/ws-event-accounting.spec.ts`
  passed 1 desktop Docker WS accounting test after the mobile session switcher
  cleanup.
- RED mobile repo/session metadata gate:
  `rtk pnpm --dir apps/web test components/task/mobile/mobile-repos-section.test.tsx hooks/domains/session/use-base-branch-by-repo.test.tsx hooks/domains/session/use-repo-display-name.test.tsx`
  failed because the mobile repo count, base-branch map, and repo display name
  readers still required legacy `kanban` metadata when Query-owned task detail
  and repository caches were populated.
- `rtk make fmt` passed after the mobile repo/session metadata cleanup.
- `rtk pnpm --dir apps/web test components/task/mobile/mobile-repos-section.test.tsx hooks/domains/session/use-base-branch-by-repo.test.tsx hooks/domains/session/use-repo-display-name.test.tsx`
  passed 3 files / 3 tests after the mobile repo/session metadata cleanup.
- `rtk pnpm --dir apps/web typecheck` passed after the mobile repo/session
  metadata cleanup.
- `rtk pnpm --dir apps/web exec eslint components/task/mobile/mobile-repos-section.tsx components/task/mobile/mobile-repos-section.test.tsx components/task/mobile/mobile-repo-pill.tsx components/task/mobile/mobile-sessions-section.tsx hooks/domains/session/use-repo-display-name.ts hooks/domains/session/use-repo-display-name.test.tsx hooks/domains/session/use-base-branch-by-repo.ts hooks/domains/session/use-base-branch-by-repo.test.tsx`
  passed after the mobile repo/session metadata cleanup.
- `rtk rg -n "state\\.kanban|s\\.kanban|getState\\(\\)\\.kanban|kanbanMulti|setWorkflowSnapshot|\\.kanban\\.(tasks|steps|workflowId|loading|error)" apps/web/components/task/mobile/mobile-repos-section.tsx apps/web/components/task/mobile/mobile-repo-pill.tsx apps/web/components/task/mobile/mobile-sessions-section.tsx apps/web/hooks/domains/session/use-repo-display-name.ts apps/web/hooks/domains/session/use-base-branch-by-repo.ts`
  returned no matches after the mobile repo/session metadata cleanup.
- `rtk git diff --check` passed after the mobile repo/session metadata cleanup.
- `rtk pnpm --dir apps/web e2e:docker --project=mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts tests/task/mobile-changes-panel.spec.ts tests/session/mobile-handoff.spec.ts`
  passed 7 mobile Docker tests after the mobile repo/session metadata cleanup.
- `rtk pnpm --dir apps/web e2e:docker tests/system/ws-event-accounting.spec.ts`
  passed 1 desktop Docker WS accounting test after the mobile repo/session
  metadata cleanup.
- RED desktop sidebar/removal gate:
  `rtk pnpm --dir apps/web test hooks/use-task-removal.test.ts components/app-sidebar/app-sidebar-new-task-item.test.tsx components/app-sidebar/sections/tasks-section.test.tsx components/task/task-session-sidebar.test.tsx`
  failed because `useTaskRemoval` still mutated and selected from active
  `kanban.tasks`, the app sidebar still ignored Query workflow/task metadata,
  and the task section still passed legacy `kanban.workflowId`. After fixing the
  test harness, `components/task/task-session-sidebar.test.tsx` also failed
  because archive metadata fell back to "this task" and sidebar move-to-step was
  a no-op with empty `kanbanMulti`.
- `rtk make fmt` passed after the desktop sidebar/removal cleanup.
- `rtk pnpm --dir apps/web test hooks/use-task-removal.test.ts components/app-sidebar/app-sidebar-new-task-item.test.tsx components/app-sidebar/sections/tasks-section.test.tsx components/task/task-session-sidebar.test.tsx`
  passed 4 files / 33 tests after the desktop sidebar/removal cleanup.
- `rtk pnpm --dir apps/web typecheck` passed after the desktop sidebar/removal
  cleanup.
- `rtk pnpm --dir apps/web exec eslint hooks/use-task-removal.ts hooks/use-task-removal.test.ts components/app-sidebar/app-sidebar-new-task-item.tsx components/app-sidebar/app-sidebar-new-task-item.test.tsx components/app-sidebar/sections/tasks-section.tsx components/app-sidebar/sections/tasks-section.test.tsx components/task/task-session-sidebar.tsx components/task/task-session-sidebar.test.tsx`
  passed after the desktop sidebar/removal cleanup.
- `rtk rg -n "state\\.kanban|s\\.kanban|getState\\(\\)\\.kanban|kanbanMulti|setWorkflowSnapshot|findTaskInSnapshots|\\.kanban\\.(tasks|steps|workflowId|loading|error)" apps/web/hooks/use-task-removal.ts apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx apps/web/components/app-sidebar/sections/tasks-section.tsx apps/web/components/task/task-session-sidebar.tsx`
  returned no matches after the desktop sidebar/removal cleanup.
- `rtk git diff --check` passed after the desktop sidebar/removal cleanup.
- `rtk pnpm --dir apps/web e2e:docker tests/task/sidebar-delete-confirm.spec.ts tests/task/delete-task-redirect.spec.ts tests/task/archive-task-redirect.spec.ts tests/kanban/card-menu-delete-archive.spec.ts tests/task/sidebar-send-to-workflow.spec.ts tests/kanban/cross-workflow-task-move.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 15 desktop Docker tests after the desktop sidebar/removal cleanup.
- `rtk pnpm --dir apps/web e2e:docker --project=mobile-chrome tests/task/mobile-sidebar-subtasks.spec.ts tests/task/mobile-task-list-search.spec.ts tests/kanban/mobile-kanban.spec.ts`
  passed 13 mobile Docker tests after the desktop sidebar/removal cleanup.
- RED task-page/session chrome gate:
  `rtk pnpm --dir apps/web test components/task/task-page-content.test.tsx components/task/dockview-header-actions.test.tsx components/task/sessions-dropdown.test.tsx components/task/session-reopen-menu.test.tsx components/task/session-tab.test.tsx components/task/base-branch-picker.test.tsx components/task/new-session-dialog.test.tsx components/task/new-subtask-dialog.test.tsx components/task/changes-panel-data.test.tsx components/task/chat/use-chat-panel-state.test.tsx hooks/domains/kanban/use-plan-actions.test.tsx`
  failed because task-page/session chrome readers still required or preferred
  active `kanban` mirrors for workflow steps, task titles, primary-session
  markers, repository metadata, subtask defaults, plan-mode step detection, and
  multi-repo PR labels when Query caches were populated.
- `rtk make fmt` passed after the task-page/session chrome cleanup.
- `rtk pnpm --dir apps/web test components/task/task-page-content.test.tsx components/task/dockview-header-actions.test.tsx components/task/sessions-dropdown.test.tsx components/task/session-reopen-menu.test.tsx components/task/session-tab.test.tsx components/task/base-branch-picker.test.tsx components/task/new-session-dialog.test.tsx components/task/new-subtask-dialog.test.tsx components/task/changes-panel-data.test.tsx components/task/chat/use-chat-panel-state.test.tsx hooks/domains/kanban/use-plan-actions.test.tsx`
  passed 11 files / 16 tests after the task-page/session chrome cleanup.
- `rtk pnpm --dir apps/web typecheck` passed after the task-page/session chrome
  cleanup.
- `rtk pnpm --dir apps/web exec eslint components/task/task-page-content.tsx components/task/task-page-content.test.tsx components/task/dockview-header-actions.tsx components/task/dockview-header-actions.test.tsx components/task/sessions-dropdown.tsx components/task/sessions-dropdown.test.tsx components/task/session-reopen-menu.tsx components/task/session-reopen-menu.test.tsx components/task/session-tab.tsx components/task/session-tab.test.tsx components/task/base-branch-picker.tsx components/task/base-branch-picker.test.tsx components/task/new-session-dialog.tsx components/task/new-session-dialog.test.tsx components/task/new-subtask-dialog.tsx components/task/new-subtask-dialog.test.tsx components/task/changes-panel-data.tsx components/task/changes-panel-data.test.tsx components/task/chat/use-chat-panel-state.ts components/task/chat/use-chat-panel-state.test.tsx hooks/domains/kanban/use-plan-actions.ts hooks/domains/kanban/use-plan-actions.test.tsx`
  passed after the task-page/session chrome cleanup.
- `rtk rg -n "state\\.kanban|s\\.kanban|getState\\(\\)\\.kanban|kanbanMulti|setWorkflowSnapshot|findTaskInSnapshots|\\.kanban\\.(tasks|steps|workflowId|loading|error)" apps/web/components/task/task-page-content.tsx apps/web/components/task/dockview-header-actions.tsx apps/web/components/task/sessions-dropdown.tsx apps/web/components/task/session-reopen-menu.tsx apps/web/components/task/session-tab.tsx apps/web/components/task/base-branch-picker.tsx apps/web/components/task/new-session-dialog.tsx apps/web/components/task/new-subtask-dialog.tsx apps/web/components/task/changes-panel-data.tsx apps/web/components/task/chat/use-chat-panel-state.ts apps/web/hooks/domains/kanban/use-plan-actions.ts apps/web/components/task/dockview-desktop-layout.tsx apps/web/components/task/sidebar-filter/use-filter-value-options.ts apps/web/components/task/task-session-sidebar-aggregate.ts`
  returned no matches after the task-page/session chrome cleanup.
- `rtk git diff --check` passed after the task-page/session chrome cleanup.
- `rtk pnpm --dir apps/web e2e:docker tests/session/session-layout.spec.ts tests/session/session-tab-management.spec.ts tests/session/new-session-dialog.spec.ts tests/session/session-handoff.spec.ts tests/task/subtask.spec.ts tests/task/session-isolation.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 36 desktop Docker tests after the task-page/session chrome cleanup.
- `rtk pnpm --dir apps/web e2e:docker --project=mobile-chrome tests/task/mobile-changes-panel.spec.ts tests/session/mobile-handoff.spec.ts`
  passed 6 mobile Docker tests after the task-page/session chrome cleanup.
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
- `rtk pnpm --dir apps/web exec eslint components/task-create-dialog-state.ts components/task-create-dialog-state.test.ts components/kanban-card-content.tsx components/kanban-card-content.test.tsx components/github/my-github/pr-row-task-indicator.tsx components/github/my-github/pr-row-task-indicator.test.tsx components/kanban-board-grid.tsx components/kanban-board-grid.test.tsx`
  passed after the cleanup.
- `rtk rg -n "state\\.kanban|s\\.kanban|getState\\(\\)\\.kanban|kanbanMulti|setWorkflowSnapshot|findTaskInSnapshots|\\.kanban\\.(tasks|steps|workflowId|loading|error)" apps/web/components/task-create-dialog-state.ts apps/web/components/kanban-card-content.tsx apps/web/components/github/my-github/pr-row-task-indicator.tsx apps/web/components/kanban-board-grid.tsx`
  returned no matches after the cleanup.
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
- `rtk git diff --check` passed after the session runtime mirror cleanups.
- `cd apps/web && rtk pnpm e2e:docker tests/chat/chat-status-bar.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 8 desktop Docker tests for session todos/prompt usage cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project mobile-chrome tests/chat/mobile-message-add-ws-gap.spec.ts`
  passed 1 mobile Docker test for the chat hook surface.
- `cd apps/web && rtk pnpm e2e:docker tests/task/task-list.spec.ts tests/session/new-session-dialog.spec.ts tests/system/ws-event-accounting.spec.ts`
  passed 7 desktop Docker tests for agent capabilities/poll mode cleanup.
- `cd apps/web && rtk pnpm e2e:docker --project mobile-chrome tests/task/mobile-task-list-search.spec.ts`
  passed 1 mobile Docker test for the task item surface.
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
- RED legacy kanban WS mirror cleanup gate:
  `rtk pnpm --dir apps/web test lib/ws/router.test.ts lib/ws/handlers/tasks.test.ts lib/ws/handlers/agent-session-kanban.test.ts`
  failed because `kanban.update` was still registered, `task.updated` /
  `task.deleted` still mutated legacy kanban mirrors, and
  `session.state_changed` still patched task primary state into those mirrors.
- RED workspace deletion mirror gate:
  `rtk pnpm --dir apps/web test lib/ws/handlers/workspaces.test.ts` failed
  because deleting the active workspace still cleared `state.kanban`.
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
  passed 18 files / 85 tests.
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

## Files Likely Touched

- `apps/web/lib/state/store.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/hydration/*`
- `apps/web/lib/state/slices/**`
- `apps/web/lib/ws/router.ts`
- `apps/web/lib/ws/handlers/**`
- `apps/web/AGENTS.md`
- `docs/specs/ui/tanstack-query-server-state.md`

## Dependencies

- Tasks 05 through 09.

## Inputs

- Domain task outputs listing retained paths.
- Current `apps/web/AGENTS.md` "Data Flow Pattern" section.

## Output Contract

Update this task to `done`, include a retained-Zustand inventory, and update
the plan checkbox.
