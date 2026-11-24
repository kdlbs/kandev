## Store Structure (Domain Slices)

```text
lib/state/
├── store.ts                        # Root composition
├── default-state.ts                # Default state + initial state merge
├── slices/                         # Domain slices
│   ├── kanban/                    # boards, tasks, columns
│   ├── session/                   # sessions, messages, turns, worktrees
│   ├── session-runtime/           # shell, processes, git, context
│   ├── workspace/                 # workspaces, repos, branches
│   ├── settings/                  # executors, agents, editors, prompts (incl. userSettings)
│   ├── comments/                  # code review diff comments
│   ├── github/                    # GitHub PRs, reviews
│   ├── gitlab/                    # GitLab MRs, watches, MR automation options
│   └── ui/                        # preview, connection, active state, sidebar views
├── hydration/                     # Boot payload merge strategies
hooks/domains/{kanban,session,workspace,settings,comments,github,gitlab}/  # Domain-organized hooks
lib/api/domains/                    # API clients
├── kanban-api, session-api, workspace-api, settings-api, process-api
├── plan-api, queue-api, workflow-api, stats-api, github-api
├── user-shell-api, debug-api, secrets-api, sprites-api, vscode-api
├── health-api, utility-api
```

**Key State Paths:**

- `messages.bySession[sessionId]`, `shell.outputs[taskEnvironmentId]`, `gitStatus.byEnvironmentId[taskEnvironmentId]`, `gitStatus.byEnvironmentRepo[taskEnvironmentId][repositoryName]`, the runtime-only accepted-ready display companion `gitStatusDisplay.byEnvironmentRepo[taskEnvironmentId][repositoryName]`, and `sessionCommits.byEnvironmentId[taskEnvironmentId]`; code/tests starting from a session ID must resolve `environmentIdBySessionId[sessionId]` before direct environment-scoped reads/writes, and `setGitStatus` takes the environment ID. Display values preserve reading continuity and must not be treated as ready for the current observation or hydrated from boot payloads.
- `tasks.activeTaskId`, `tasks.activeSessionId`, `workspaces.activeId`
- `repositories.byWorkspace`, `repositoryBranches.byRepository`; `automations.byWorkspace[workspaceId]` owns settings list rows, loaded/loading flags and generations per app store. `useAutomations` reads only its workspace entry; initial loads share pending/cached reads, only the newest refresh settles, and admitted reads survive consumer unmounts. Flat `automations.items/loaded/loading` remain compatibility/sidebar invalidation state, never workspace settings list input. Breadcrumbs prefer scoped entries (including empty ones), with workspace-checked flat fallback only when absent. See [list publication](../../docs/specs/office/system-design/automations-settings-02.md#workspace-list-publication); mutation/list and backend-event concurrency are excluded.

`chatMotion` owns per-device chat animation settings; keep it separate from rich-output motion and transcript scrolling. Quick Chat separates server conversations from browser-local terminal tabs. For Quick Chat lifecycle or replacement, read `quick-terminal-actions.ts` and the relevant specs. For rich output, preserve the closed catalog, stable native registry, and scoped view state. Never accept agent-provided actions, CSS, HTML, or host callbacks. See the [rich-output design](../../docs/specs/agents/system-design/agent-rich-output.md).

**Hydration:** Go injects `window.__KANDEV_BOOT_PAYLOAD__` into the SPA shell before React mounts. Boot version 2 stores repeated task and session rows once in `entities` and sends ID memberships; `src/boot-payload.ts` expands them before store hydration and still accepts version 1. Board routes boot only the selected workflow, and task detail boots one sidebar page plus the selected task. `lib/state/hydration/merge-strategies.ts` has `deepMerge()`, `mergeSessionMap()`, `mergeLoadingState()` to avoid overwriting live client state. Pass `activeSessionId` to protect active sessions. Client task navigation uses `fetchTaskNavigationData` for essential task/session hydration. Render current-workspace task projections during refresh; leave optional enrichment to domain hooks instead of repeating the full boot bundle. Read-cursor capture and automatic session creation wait for authoritative route hydration. Do not replay cached hydration snapshots over live state.

For rebasing or finishing PRs written against the old Next.js runtime, follow [`docs/nextjs-spa-migration.md`](../../docs/nextjs-spa-migration.md).

**Hooks:** Hooks in `hooks/domains/` own WS subscriptions and selection. The WS client deduplicates subscriptions. System snapshot Query owners retain the full API base URL, boot, and authentication identity boundary. For changes, read the designs for [SystemInfo](../../docs/specs/platform/system-design/system-info-query-cache.md), [database statistics](../../docs/specs/system-page/system-design/database-statistics-snapshot.md), [backups](../../docs/specs/system-page/system-design/backup-list-query-cache.md), and [disk usage](../../docs/specs/system-page/system-design/disk-usage-query-cache.md). For shell, commit, and diff reads, preserve scope and late-response guards in `lib/state/session-read-coordinator.ts`. User-shell state uses task/environment scope; see the [terminal persistence design](../../docs/specs/tasks/system-design/task-terminal-persistence.md) and [navigation design](../../docs/specs/ui/system-design/task-navigation-responsiveness.md).

Session recovery keeps typed guard state, Stop, and continuation precedence across desktop/mobile; durable adoption completes before new work is admitted.
