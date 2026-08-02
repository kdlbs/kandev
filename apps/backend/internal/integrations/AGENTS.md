# Adding a new third-party integration

Jira and Linear are the model: per-workspace credentials, a 90s auth-health poller, a settings page with status banner + reconnect CTA, link/import buttons that gate on availability. New integrations should reuse the shared shapes in this folder rather than copying either.

This file covers both halves of the playbook (backend + frontend) so the pattern is readable in one place.

## Backend (`apps/backend/internal/<name>/`)

- Mirror the package layout: `service.go`, `store.go`, `client.go`, `provider.go`, `handlers.go`, `models.go`, `poller.go`. Expose `Provide(writer, reader *sqlx.DB, secrets SecretStore, eventBus bus.EventBus, log *logger.Logger) (*Service, func() error, error)`. Pass `nil` for `eventBus` when the integration doesn't publish events; both Jira and Linear take and use it for issue-watch publishing.
- Use `internal/integrations/secretadapter` instead of writing your own upsert wrapper around `secrets.SecretStore`. The adapter satisfies any per-integration `SecretStore` interface shaped as `{Reveal, Set, Delete, Exists}`.
- Use `internal/integrations/healthpoll` for the auth-health loop. Implement the `Prober` interface (`ListConfiguredWorkspaces` + `RecordAuthHealth`) on a small adapter and let `healthpoll.New("name", prober, log)` own Start/Stop/ticker. Keep integration-specific loops (JQL polling, webhook reconciliation, etc.) separate, like jira's issue-watch loop.
- Wire the service via a per-domain `init<Name>Service(...)` helper in `cmd/kandev/services.go`, not inline in `provideServices`.
- Ship a `mock_client.go` + `mock_controller.go` next to the real client. `Provide` branches on `KANDEV_MOCK_<NAME>=true` and returns the in-memory client; `RegisterMockRoutes(router, svc, log)` mounts `/api/v1/<name>/mock/*` only when the service was built with the mock. The e2e backend fixture sets the env var so Playwright tests drive the mock via `apiClient.mock<Name>*()` helpers — see jira/linear for the layout.
- **If the watcher has a per-watch `MaxInflightTasks` cap**, implement `WatchMetadataKey()` on the integration's `WatcherSource` (`internal/orchestrator/source_<name>.go`) to return the task-metadata watch-id key — the **same constant** the source writes into `BuildTaskRequest`'s `Metadata` map (e.g. `sentry_issue_watch_id`). The throttle gate (`acquireWatcherSlot`) passes that key to `CountOpenWatcherCreatedTasks(metadataKey, watchID)`, which counts open tasks for the watch. The task repository (`internal/task/repository/sqlite/task.go`) is intentionally **agnostic of integrations** — it keys purely on the metadata key, so no repository change is needed per integration. If `WatchMetadataKey()` returns `""` the gate treats the watch as uncapped and the cap **silently never applies** (Sentry originally shipped with this gap — the cap was stored and validated but never enforced because the repository's old integration switch had no `sentry` case).

## Frontend (`apps/web/`)

- Hooks live under `hooks/domains/<name>/`, **not** `components/<name>/`.
- Use `hooks/domains/integrations/use-integration-availability.ts` and `use-integration-enabled.ts` — each integration's `useXAvailable` / `useXEnabled` should be a one-line wrapper passing the storage key + sync event + config-fetch function.
- Settings page reuses `<IntegrationAuthStatusBanner>` (`components/integrations/auth-status-banner.tsx`).
- "Auth required / reconnect" UI reuses `<IntegrationAuthErrorMessage>` (`components/integrations/auth-error-message.tsx`) — supply the integration's display name, regex check, and reconnect href.
- Link / import popovers reuse `<ValidatedPopover>` (`components/integrations/validated-popover.tsx`) — supply the icon, label, key regex, fetch function, and success callback.

## Per-user scoping (the integration route group is query-only)

The global `integrationWorkspaceScopeMiddleware` (`backendapp/helpers.go`) only
authorizes the workspace named in the **`workspace_id` query param**, and it
**falls through when that param is absent**. So two shapes bypass it and MUST
guard themselves at the service layer:

- **A body-supplied workspace** — e.g. `POST /config/copy`'s `targetWorkspaceId`.
  The middleware authorizes the source (query) but never the target (body), so a
  copy must call `authorizeWorkspaceAccess` on **both** source and target, up
  front — before any per-workspace side effect (the pre-write `UpdateAuthHealth`
  flip counts). Mirror `github/copy.go`.
- **An omitted `workspace_id`** — `normalizeWorkspaceID("")` resolves a global
  *default* workspace not owned by the caller. User-facing config entry points
  resolve+authorize in one step via `resolveWorkspaceID(ctx, id)` instead of bare
  `normalizeWorkspaceID`. The unscoped `ListAllIssueWatches` (reached by omitting
  `workspace_id`) filters to the caller's workspaces; an identity-less internal
  caller (nil authorizer) still sees all, preserving poller use.

Wire the boundary with `SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess)`
in `backendapp/helpers.go`; nil (unit tests, auth disabled) means unscoped.
Denials surface `repoerrors.ErrWorkspaceNotFound`, which handlers map to 404 (no
existence leak). Jira/Linear/Slack follow this; **Sentry and GitLab's
`ListAllIssueWatches` still need the same filter** — the WS gateway backstop does
not read `workspace_id`, so GitLab's `workspace_id`-keyed WS list is unscoped too.

## Auto-link guarantee (GitHub pull requests / GitLab merge requests)

A code-host PR/MR opened on a task's session branch gets linked to the task
without a manual step, on both providers, through three independent
*discovery* paths (rows 1–3 below find and create the association). Two more
rows exist alongside them: manual URL linking is an explicit user action, not
discovery, and the background poller mostly *refreshes* an association
discovery already created — except that a Discovery row can itself leave
behind a *placeholder* watch (`pr_number` / `mr_iid` = 0) when it ran before
any PR/MR existed on the branch yet, and the poller resolves that placeholder
into a real association on a later tick. The poller never creates a new watch
or discovers a PR/MR with no watch at all — resolving a placeholder is the one
exception to "refresh-only", not independent discovery. "Auto-link"
(discovery) means: a `github_task_prs` / `gitlab_task_mrs` row is created, and
a refresh watch (`github_pr_watches` / `gitlab_mr_watches`) is created so
ongoing review/CI/pipeline status keeps syncing via the poller from then on.

| Trigger | Role | GitHub | GitLab |
| --- | --- | --- | --- |
| Kandev's own Create-PR action (`worktree.create_pr`) | Discovery | ✅ `gateway.go` → `createdChangeAssociationRouter.associate` → `associateGitHub` | ✅ same router → `associateGitLab` → `gitlab.Service.AssociateExistingMRByURL` |
| Push detected on the session branch (agent/CLI `git push`, or a human pushing directly — `gh pr create` / `glab mr create` alone create no push event, so discovery here fires from the branch push that preceded or accompanied them, not the create call itself) | Discovery | ✅ `event_handlers_git.go`'s `trackPushAndAssociatePR` → `event_handlers_github.go`'s `detectPushAndAssociatePR` (retries `[0, 30s, 60s]`) | ✅ same `trackPushAndAssociatePR` dispatches by repository provider → `event_handlers_gitlab_mr.go`'s `detectPushAndAssociateMR` (same retry shape) → `gitlab.Service.AutoLinkMRForBranch`, which now also leaves a placeholder (`mr_iid=0`) watch behind when no MR is open yet, so an MR opened later (e.g. from the GitLab web UI, well after the retry window) is still found by the poller |
| On-demand check (no UI trigger today; callable via WS action) | Discovery | ✅ `Service.CheckSessionPR`, `ws.ActionGitHubCheckSessionPR` (`github.check_session_pr`) | ✅ `Service.CheckSessionMR`, `ws.ActionGitLabCheckSessionMR` (`gitlab.check_session_mr`) |
| Manual URL linking | Explicit (not auto-link) | ✅ `POST /api/v1/github/task-prs` | ✅ `POST /api/v1/gitlab/task-mrs` (`AssociateExistingMRByURL`) |
| Background poller | Refresh, plus placeholder resolution — completes discovery only for a placeholder watch (`pr_number` / `mr_iid` = 0) a Discovery row above already created; never creates a new watch itself | `github.Poller` iterates `github_pr_watches`; `PRNumber == 0` resolves via `FindPRByBranch` before upserting `github_task_prs` | `gitlab.Poller.runMRMonitor` iterates `gitlab_mr_watches`; `MRIID <= 0` resolves via `FindMRByBranch` before `CheckMRWatch` upserts `gitlab_task_mrs` and publishes `gitlab.task_mr.updated` only when visible MR fields change |

Provider dispatch is structural, not a runtime flag: `trackPushAndAssociatePR`
resolves the pushing repository's `provider` column once and routes to
exactly one provider's detect function, so a push on a GitHub repository
issues zero GitLab client calls and vice versa. Multi-repository and
multi-branch tasks scope every association/watch by `(repository_id,
branch)` on both providers, so linking one repo/branch never clobbers a
sibling.

**Does not trigger auto-link on either provider:**

- Opening the PR/MR in the code host's web UI with no corresponding push
  event reaching Kandev (e.g. the agent's worktree was never pushed) — there
  is nothing for push detection to observe.
- **Azure DevOps** (`internal/azuredevops`) has no auto-link of any kind —
  it persists PR summaries against tasks but has no watch loop or push
  detection. Same gap GitHub/GitLab had before this guarantee; file
  separately if wanted.

## Where Jira and Linear deliberately diverge

- **Issue model:** Jira uses transitions + JQL; Linear uses state IDs + structured filters. Don't merge these schemas — the upstream APIs are genuinely different.
- **Watch filter persistence:** Jira stores the JQL string verbatim; Linear stores the structured `SearchFilter` as JSON in `filter_json` (Linear has no JQL equivalent). The orchestrator emits `NewJiraIssueEvent` / `NewLinearIssueEvent` respectively and dedups by issue key (Jira) vs identifier (Linear).
- **Health column extras:** Linear's `linear_configs` row carries an `org_slug` captured from successful probes; Jira's row does not.
- **Sentry — multiple instances per workspace:** unlike the one-config-per-workspace integrations, `sentry_configs` is keyed by an instance `id` (UUID) with a `workspace_id` column + `UNIQUE(workspace_id, name)`, so a workspace holds several named instances. Secrets are keyed `sentry:instance:<id>:token`. Issue watches carry a nullable `sentry_instance_id` FK (`ON DELETE RESTRICT`); the bound instance is immutable. The HTTP surface is instance CRUD under `/api/v1/sentry/instances?workspace_id=` (no install-wide `/config`); deleting an in-use instance is 409 `SENTRY_INSTANCE_IN_USE`. See ADR-0030.
- **Azure DevOps — REST browse, task links, and watchers:** `internal/azuredevops` accepts only canonical Azure DevOps Services organization URLs, stores one encrypted PAT per workspace through `secretadapter`, and uses REST API 7.1 without `gh` or `az`. It reuses `healthpoll` and the standard mock-provider gate, and its independent watcher poller persists generation-safe work-item/PR reservations before publishing orchestrator events. Azure pull-request summaries are persisted against tasks in the integration package; provider-native feedback remains transient and Azure-specific.
