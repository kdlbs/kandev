---
status: draft
system: integrations
created: 2026-11-02
owners:
  - kandev
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007
---

# Azure DevOps Connector Plugin System Design

## Purpose and boundaries

The connector plugin lives in `JnManso/kandev-plugin-azure-devops`. It is
created from `kdlbs/kandev-plugin-template` and follows the architecture of
`kdlbs/kandev-plugin-bitbucket`:

- a Go backend served by `pluginsdk.Serve`;
- an ES module UI bundled with esbuild, with React external.

It ports the behavior of the built-in integration
(`apps/backend/internal/azuredevops`, `apps/web/components/azure-devops`) and
the Microsoft sign-in work from branch `feature/azure-devops-oauth-a-9e6`
(head `7226134a2`, closed PR #4915). That branch is the implementation
reference for Entra sign-in, renewal locking, and the sign-in UI.

The plugin uses these host contracts and does not own them: authenticated
actions; Host state, secrets, and user storage; `registerRoute`,
`registerNavItem`, `registerIntegrationSettings`; repository provider; Git
credential broker; review provider; task actions and the task link dialog;
reference sources; agent tools; plugin translations.

It needs no new host contract. It activates only on a host where the built-in
integration is removed; see
[Azure DevOps leaves core](../../plugins/system-design/azure-devops-extraction.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001` | [Repository, manifest, and build](#repository-manifest-and-build) |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002` | [Connections and credentials](#connections-and-credentials), [Microsoft sign-in](#microsoft-sign-in) |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003` | [Health and renewal](#health-and-renewal) |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004` | [Browse screens](#browse-screens) |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005` | [Tasks and links](#tasks-and-links) |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006` | [Repository provider and Git credentials](#repository-provider-and-git-credentials) |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007` | [Watches](#watches) |

## Repository, manifest, and build

```text
kandev-plugin-azure-devops/
  manifest.yaml
  server/main.go                 pluginsdk.Serve(plugin.NewRuntime())
  internal/azure/                REST client (API 7.1), models, pagination, retry
  internal/auth/                 PAT authorizer, Entra flows, renewal locks
  internal/store/                Host state snapshots and secret keys
  internal/watches/              watch service, reservations, poller
  internal/plugin/               actions, git credentials, references, agent tool
  internal/mock/                 in-process Azure DevOps + Entra fake for tests
  ui/src/                        registrations, screens, hooks, i18n catalogs
  ui/e2e/                        packaged-host Playwright specs
  .kandev-sdk-ref                pinned Kandev revision for the SDK checkout
```

Manifest essentials:

- `id: kandev-plugin-azure-devops`, `api_version: 2`, `categories: ["connector"]`,
  `min_kandev_version` set at release to the first Kandev release without the
  built-in integration.
- `capabilities`: `state`, `secrets`, `user_state`;
  `api_read: [tasks, workspaces, workflows, agent_profiles, executor_profiles, repositories]`;
  `api_write: [tasks]`.
- `repository_providers: [azure_devops]`.
- `capabilities.events`: `task.deleted`.
- `reference_sources`: `azure_work_items` (kind `work_item`) and
  `azure_pull_requests` (kind `pull_request`), provider `azure_devops`.
- `agent_tools`: `create_pull_request` on the `kanban-task` surface.
- `config_schema`: `default_oauth_client_id` (string, GUID-validated), the
  installation default application.
- `ui.bundle`, `ui.styles`, `icon`.

Build and SDK. The template and the Bitbucket plugin build against a sibling
Kandev checkout pinned by `.kandev-sdk-ref`. The plugin keeps that model and
uses the standalone SDK paths introduced by Kandev #4534:

- Go: `require github.com/kdlbs/kandev/pluginsdk` with
  `replace => ../kandev/pluginsdk`; imports use
  `github.com/kdlbs/kandev/pluginsdk` and
  `github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1`.
- npm: `"@kdlbs/plugin-sdk": "file:../kandev/apps/packages/plugin-sdk"`.
- CI and release workflows check out `kdlbs/kandev` at `.kandev-sdk-ref` into
  `kandev/`, as the template does.
- In the Kandev task workspace the sibling is `kdlbs-kandev`, so a `kandev`
  symlink next to the plugin worktree satisfies both paths.

During development the sibling checkout carries the removal change, because
the web plugin registry rejects `azure_devops` and `azure-devops` while the
built-in integration exists. When the SDK is published on npm and the Go proxy,
the plugin can switch to exact published versions and `kandev-sdk-check`; that
is not required for the first release.

The registry entry `kandev-plugin-azure-devops` is added to
`plugin-registry/plugins.yaml` in the Kandev repository after the plugin's
first release.

## Connections and credentials

Host state per workspace (`scope: workspace`), key `connection`:

```json
{ "organizationUrl": "https://dev.azure.com/org", "defaultProjectId": "", "defaultProjectName": "",
  "authMethod": "pat|oauth", "oauthClientId": "", "oauthAccount": "", "oauthTenantId": "",
  "tokenExpiresAt": "", "reauthRequired": false, "workspaceSettings": {}, "savedViews": [],
  "generation": 3 }
```

Secrets (instance-global namespace, keyed by workspace):

- `ws.<workspaceId>.pat`
- `ws.<workspaceId>.oauth.refresh`
- `ws.<workspaceId>.oauth.access`

Method switches follow the built-in rules: write the new secret, write the
`connection` with an incremented `generation`, then delete the old secret. If
the state write fails, restore the previous secret. Organization changes on
OAuth require a new sign-in (`requireSignedInOrganization` in the branch).
Copy-to-workspace copies non-secret fields and sets `reauthRequired`.
Organization URLs are validated as canonical `https://dev.azure.com/<org>`,
ported from `ValidateOrganizationURL`.

Actions are authenticated and workspace-scoped:

- `connection.get`, `connection.save`, `connection.test`, `connection.copy`,
  `connection.delete`
- `oauth.start`, `oauth.complete`, `oauth.sign_out`, `oauth.availability`

Kandev requires lowercase action keys, so multi-word keys use underscores.

The enable switch is the `action` of `registerIntegrationSettings`, built on
`host.ui.IntegrationEnabledControl`. Its value is stored per user and per
workspace in `host.storage` under `integration.enabled`, because
`host.setIntegrationEnabled` is memory-only. The plugin reads every workspace at
load and republishes it, and a missing value means on. A slower read never
replaces a newer change.

The connection form registers with `host.useSettingsSaveContributor`, as
Kandev's Jira section does, so the native Save changes bar saves it.
`connectionFormState` derives `isDirty`, `canSave`, and the localized
`invalidReason` from the saved connection and the form. For a Microsoft
account only the default project is savable there. Sign-in creates and
replaces the account. The revision is a counter bumped on every form change, so
the token never enters the host's save state.

## Microsoft sign-in

Ported from branch files `entra.go`, `service_oauth*.go`, and
`oauth_application.go`, with these changes:

- Flows are stored in workspace state `oauth.flow.<stateHash>`: workspace,
  starting actor (from `VerifiedActionContext.ActorID`), client ID, PKCE
  verifier, redirect URL, and expiry of ten minutes. `Claim`
  (`state/store.go`) makes completion single-use.
- `oauth.start` resolves the client ID:
  1. the form value;
  2. the saved workspace value;
  3. `default_oauth_client_id`.

  It then discovers the tenant from `X-VSS-ResourceTenant` and returns the
  authorize URL. The redirect URL is the page origin sent by the UI. Scope is
  `499b84ac-1321-427f-aa17-267ca6975798/.default offline_access openid profile`.
- The UI opens the popup and watches it until it returns to the page origin. It
  then reads `code` and `state` from the same-origin popup address and calls
  `oauth.complete`. When the popup cannot return, the paste panel parses the
  pasted address. This is `useOAuthCompletion` and `usePopupRedirectWatch` from
  the branch.
- `oauth.complete` checks the flow's workspace and actor, exchanges the code,
  probes the organization with the new token, then persists the connection
  under the workspace renewal lock (`replaceOAuthConnection`). On failure it
  restores the previous credential.

## Health and renewal

- `accessToken` checks freshness, takes the per-workspace lock, rechecks, then
  refreshes with the client ID that signed in. This is ported from
  `service_oauth_renewal.go`. A sign-in holds the same lock, so it wins over an
  in-flight renewal.
- REST calls retry once after a renewal on 401, ported from `rest_client.go`
  `send`/`sendOnce`.
- `invalid_grant` and `interaction_required` set `reauthRequired`. A rejected
  PAT sets an unhealthy status.
- The plugin manager goroutine polls every 90 seconds per connected workspace
  and writes `health` state. The settings screen renders it with
  `IntegrationAuthStatusBanner`, and integration health reads it through
  `connection.get`.
- The current-user identity comes from the Azure DevOps connection data
  endpoint for the active credential.

## Browse screens

`registerRoute("/azure-devops")` renders a port of
`app/azure-devops/azure-devops-page-client.tsx` and its components:

- filters, scope bar, mode tabs;
- board with columns and drag to change state;
- work-item detail with comments and linked tasks;
- pull-request list, detail, and feedback;
- save-view dialog.

Shared host components replace hand-made ones where they exist:
`IntegrationListToolbar`, `IntegrationScopeBar`, `IntegrationSaveQueryDialog`,
`IntegrationCursorPagination`, `ChangeRequestList/Row/Detail`.

`registerNavItem` places the entry in the integrations section, visible when
the workspace has a `connection`. `registerIntegrationSettings({ id:
"azure-devops" })` renders connection, query presets, quick actions, and the
watch settings.

The ported sections call `useT()`, which wraps the host's translation function
in `useCallback`. It keeps its identity until the locale changes, as in Kandev,
so effects that list it as a dependency run once rather than on every render.

| Action | Built-in route it replaces |
| --- | --- |
| `projects.list`, `teams.list`, `boards.list`, `boards.get` | `/projects`, `/teams`, `/boards*` |
| `boards.update_item`, `work_items.search`, `work_items.get`, `work_items.comments`, `work_items.update` | `/work-items*`, board `PATCH` |
| `pull_requests.list`, `pull_requests.get`, `pull_requests.feedback`, `pull_requests.people` | `/pull-requests*` |
| `views.get`, `views.save`, `workspace_settings.get`, `workspace_settings.patch` | `/views`, `/workspace-settings` |
| `identity.get` | `/identity` |

Lists use cursor pagination with page sizes bounded to keep each action
response below 1 MiB and within the 15-second action deadline. Azure DevOps
listings paged with `$top`/`$skip` (projects, teams) read the first page alone
and later pages four at a time. Browse preferences are stored with
`host.storage` (per user); the Board preference records the project and the
names of the last team and board, so the Board loads them before the team list
arrives. The UI shares one team listing per workspace and project for the page
session. `pull_requests.people` reads the creators and reviewers of the
repository's latest 300 pull requests of every status (pages of 100), so the
creator and reviewer filters are name pickers that send the identity ID, with
`@me` for the signed-in user. `boards.list` reads the team's backlog levels and skips hidden levels
and the `task` level, which has no Kanban board (`work/boards/{id}` answers
404); tasks are on the sprint taskboard. Changing board clears the previous
board's cards before the read, so a failed read shows only its error.

Phone composition ports the built-in mobile screens. The mobile E2E specs of
the built-in integration are the parity checklist.

## Tasks and links

- **Task from item.** `host.ui.TaskCreateDialog` with `initialValues` from the
  work item or pull request. On creation, the plugin links the task.
- **Links.** Pull-request and work-item links are plugin state, workspace
  scope, keys `task.<taskId>.pullRequests` and `task.<taskId>.workItems`, at
  most 50 of each kind per task. Task-scoped actions `pull_requests.link`,
  `pull_requests.unlink`, `work_items.link`, and `work_items.unlink` accept a
  URL of the connected organization or a short reference (`Project/Repo!42`,
  `!42`, `Project#42`, `#42`). The plugin subscribes to `task.deleted` and, on
  startup, drops links of tasks the host reports as not found.
- **Status.** `task_links.get` maps each linked pull request's policies,
  reviewer votes, and active threads to the host `ReviewTaskStatus`.
  `registerReviewProvider` publishes it; the host renders the native
  indicators and refreshes every 90 seconds. The review panel renders the host
  `ChangeRequestDetail` from `pull_requests.feedback`.
- **Link entries.** `registerTaskAction` adds the **Azure DevOps Pull Request**
  and **Azure DevOps Work Item** entries under **Link**, with
  `openTaskLinkDialog`.
- **Create PR.** `registerRepositoryProvider({ createChangeRequest })` calls
  the task-scoped `pull_requests.create` after the host push, with the branch
  Kandev verified for the session. An active pull request for the same branch
  pair is reused. The task's linked work items are attached, and the pull
  request is linked.
- **Agent tool.** `create_pull_request` takes `title`, `description`,
  `sourceBranch`, `targetBranch`, `repository`, and `draft`. Plugins cannot
  read a session's worktree branch, so the agent passes the branch it pushed.
  The tool resolves the task's Azure repository from host task and repository
  data (or `repository` when the task has several) and checks that the branch
  exists on the remote. It then creates the pull request, or reuses an open
  one, and links it. When the branch is not pushed, it fails and explains how
  to push.
- **References.** `EntityReferenceHandler` searches work items (WIQL title
  match or ID) and active pull requests of the default project, or of the
  first five projects. Keys are `Project#42` and `Project/Repository!42`, and
  authorization re-reads the item. This is ported from
  `mentions/provider_azure.go`.

## Repository provider and Git credentials

- `repositories.list` pages projects, then repositories.
- `repositories.branches` lists refs.
- `repositories.inspect` parses these forms into the canonical HTTPS form
  `https://dev.azure.com/<org>/<project>/_git/<repo>`, without userinfo:
  - `https://dev.azure.com/<org>/<project>/_git/<repo>`
  - `https://<org>@dev.azure.com/...`
  - `git@ssh.dev.azure.com:v3/<org>/<project>/<repo>`
  - `https://<org>.visualstudio.com/...`

  It is ported from `task/service/service_tasks.go` `parseAzure*Remote`.
  Provider identity is `provider_scope = <org>/<project>` plus the immutable
  repository GUID. Existing `azure_devops` repository rows carry
  `provider_owner` and `provider_name`, which the host accepts as identity.
- `GitCredentialHandler.ResolveGitCredential` requires task, session, and
  repository identifiers. It checks that the repository belongs to the
  workspace connection's organization and that the host and path match, then
  returns:
  - `username: kandev` with the PAT; or
  - the OAuth access token as password, renewed first.

  `GetGitCredentialBinding` returns `azure-devops-credential:<generation>`, so
  method changes and sign-outs revoke outstanding leases.

  The repository is matched by its recorded clone URL. A row without one (for
  example, the provider row Kandev records in place of a task-worktree row) is
  matched by `provider_host = https://dev.azure.com`, `owner_or_project`, and
  `provider_name` in the connected organization. A row with neither is refused.
- Executors: local and worktree redeem leases on the loopback broker. Docker,
  remote Docker, SSH, Kubernetes, and Sprites require
  `githubCredentialBroker.publicBaseUrl` (HTTPS, reachable from the executor),
  as documented for the Bitbucket plugin. The README states this.

## Watches

The watch service is ported from `watch_service.go`, `watch_store.go`, and
`poller.go`. It is stored in workspace state:

- `watch.<id>`: configuration, including `agentProfileId`,
  `executorProfileId`, generation, and disabled reason;
- `watch.<id>.item.<generation>.<itemKey>`: reservation and created task ID,
  with item keys `wi-<id>` and `pr-<repositoryGuid>-<id>`.

Behavior:

- **Editor.** The watch editor lists workflows, steps, agent profiles, and
  executor profiles from the host read APIs. The host returns only profiles
  the plugin may launch with (shared ones).
- **Polling.** The manager polls every minute. For each due watch it queries
  Azure DevOps and writes the reservation before creating the task. It calls
  `Tasks().Create` with `StartAgent`, `Launch{AgentProfileID,
  ExecutorProfileID, Prompt}`, the repository and base branch, and metadata
  `azure_devops_watch_id` and `azure_devops_item_key`. It then records the
  task identifier. A failed creation releases the reservation. A launch
  rejected because the profile is not available disables the watch and stores
  the reason, which the settings screen shows. A per-watch open-task limit
  counts created tasks that are not terminal or archived.
- **Reset and delete.** Kandev offers plugins no task deletion: `Tasks()` has
  no Delete, `PluginOwnedTaskTrees.Delete` is refused, and exact task commands
  need management approval. The built-in cleanup policy is therefore not
  ported. Reset starts a new generation, so current matches create tasks
  again. Delete removes the watch and its reservations. Both keep the tasks the
  watch created.

## Failure and recovery

- Every action maps Azure errors to stable codes (`reauth_required`,
  `forbidden`, `not_found`, `rate_limited`, `unavailable`) that the UI
  translates.
- 429 responses honor `Retry-After`.
- Host state writes are per key. Multi-key updates write the generation-bearing
  key last, so readers see either the old or the new connection.

## Security

- Secrets never leave the backend except as Git credential responses to the
  host broker.
- Actions trust only `VerifiedActionContext` for actor and workspace.
- OAuth flows are bound to the actor and workspace and are single-use.

## Observability

- Structured logs (`stderr`) cover renewals, health transitions, watch runs,
  and launch rejections. Tokens are never logged.

## Testing

- Go unit tests run against `internal/mock`, ported from the built-in tests,
  including renewal races.
- UI unit tests use Vitest.
- Packaged E2E runs in the plugin repository against a disposable host built
  from the sibling checkout: connect with PAT, sign in through the mock
  authority, browse, watch-created task, link, Create PR, agent tool. Each flow
  runs on desktop and on a mobile project.

## Related decisions

- [Plugin change-request mutations](../../../decisions/2026-08-10-plugin-change-request-mutations.md)
- [Plugin repository provider extensions](../../../decisions/2026-07-31-plugin-repository-provider-extensions.md)
