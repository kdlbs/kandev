---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001
created: 2026-07-17
updated: 2026-10-18
owners:
  - tbd
---
# Azure DevOps Integration System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001` | [Migrated source detail](#migrated-source-detail) |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001` (`AC-001.12` through `AC-001.14`) | [Task/detail association identity](#taskdetail-association-identity) |

## Migrated source detail

## API Surface

Every route requires `workspace_id` as a query parameter unless the workspace
is present in the path.

| Method   | Path                                                                                  | Behavior                                                                 |
| -------- | ------------------------------------------------------------------------------------- | ------------------------------------------------------------------------ |
| `GET`    | `/api/v1/azure-devops/config`                                                         | Return redacted workspace configuration or 204                           |
| `POST`   | `/api/v1/azure-devops/config`                                                         | Validate and save organization, project, and optional replacement PAT    |
| `DELETE` | `/api/v1/azure-devops/config`                                                         | Delete configuration and PAT                                             |
| `POST`   | `/api/v1/azure-devops/config/test`                                                    | Test submitted or stored credentials without persisting submitted values |
| `POST`   | `/api/v1/azure-devops/config/copy`                                                    | Copy configuration and credential to another workspace                   |
| `GET`    | `/api/v1/azure-devops/projects`                                                       | List accessible projects                                                 |
| `GET`    | `/api/v1/azure-devops/teams`                                                          | List accessible teams for a project                                      |
| `GET`    | `/api/v1/azure-devops/boards`                                                         | List visible boards/backlog levels for a project and team                |
| `GET`    | `/api/v1/azure-devops/boards/:boardId`                                                | Return board columns and hydrated work-item cards                        |
| `PATCH`  | `/api/v1/azure-devops/boards/:boardId/work-items/:id`                                 | Update an allowed card field or board position with revision protection  |
| `GET`    | `/api/v1/azure-devops/repositories`                                                   | List repositories, optionally filtered by project                        |
| `GET`    | `/api/v1/azure-devops/repositories/:projectId/:repositoryId/branches`                 | List repository branches for task creation                               |
| `GET`    | `/api/v1/azure-devops/views`                                                          | Return workspace-scoped saved Azure views                                |
| `PUT`    | `/api/v1/azure-devops/views`                                                          | Replace workspace-scoped saved Azure views                               |
| `POST`   | `/api/v1/azure-devops/work-items/search`                                              | Execute WIQL and return hydrated work items                              |
| `GET`    | `/api/v1/azure-devops/work-items/:id`                                                 | Return one hydrated work item                                            |
| `PATCH`  | `/api/v1/azure-devops/work-items/:id`                                                 | Assign the item to the PAT identity or unassign with revision protection |
| `GET`    | `/api/v1/azure-devops/work-items/:id/comments`                                        | Return one page of non-deleted work-item discussion                      |
| `GET`    | `/api/v1/azure-devops/identity`                                                       | Return the Azure identity represented by the stored workspace PAT        |
| `GET`    | `/api/v1/azure-devops/workspaces/:workspaceId/task-work-items`                        | Return work-item associations grouped by provider item                   |
| `POST`   | `/api/v1/azure-devops/tasks/:taskId/work-items`                                       | Validate and associate an Azure work item with a Kandev task             |
| `GET`    | `/api/v1/azure-devops/pull-requests`                                                  | List PRs by project, repository, status, author, or reviewer             |
| `GET`    | `/api/v1/azure-devops/pull-requests/:projectId/:repositoryId/:pullRequestId`          | Return PR detail                                                         |
| `GET`    | `/api/v1/azure-devops/pull-requests/:projectId/:repositoryId/:pullRequestId/feedback` | Return reviewers, threads, linked work items, and policies               |
| `GET`    | `/api/v1/azure-devops/workspaces/:workspaceId/task-prs`                               | Return task PR associations grouped by task                              |
| `POST`   | `/api/v1/azure-devops/tasks/:taskId/pull-requests`                                    | Validate and associate an Azure PR with a task repository                |
| `POST`   | `/api/v1/azure-devops/tasks/:taskId/pull-requests/sync`                               | Refresh persisted state for one association                              |
| `GET`    | `/api/v1/azure-devops/workspace-settings`                                             | Return resolved/default query and action presets                         |
| `PATCH`  | `/api/v1/azure-devops/workspace-settings`                                             | Update named workspace preset fields with omitted-vs-null semantics      |
| `GET`    | `/api/v1/azure-devops/watches/work-items`                                             | List workspace work-item watches                                         |
| `POST`   | `/api/v1/azure-devops/watches/work-items`                                             | Create a work-item watch and run its initial check                       |
| `PATCH`  | `/api/v1/azure-devops/watches/work-items/:id`                                         | Update or enable/disable a work-item watch                               |
| `DELETE` | `/api/v1/azure-devops/watches/work-items/:id`                                         | Delete a work-item watch under its cleanup policy                        |
| `POST`   | `/api/v1/azure-devops/watches/work-items/:id/trigger`                                 | Run one work-item watch immediately                                      |
| `GET`    | `/api/v1/azure-devops/watches/work-items/:id/reset/preview`                           | Return the tasks affected by resetting a watch                           |
| `POST`   | `/api/v1/azure-devops/watches/work-items/:id/reset`                                   | Apply cleanup and advance the work-item watch generation                 |
| `GET`    | `/api/v1/azure-devops/watches/pull-requests`                                          | List workspace pull-request watches                                      |
| `POST`   | `/api/v1/azure-devops/watches/pull-requests`                                          | Create a pull-request watch and run its initial check                    |
| `PATCH`  | `/api/v1/azure-devops/watches/pull-requests/:id`                                      | Update or enable/disable a pull-request watch                            |
| `DELETE` | `/api/v1/azure-devops/watches/pull-requests/:id`                                      | Delete a pull-request watch under its cleanup policy                     |
| `POST`   | `/api/v1/azure-devops/watches/pull-requests/:id/trigger`                              | Run one pull-request watch immediately                                   |
| `GET`    | `/api/v1/azure-devops/watches/pull-requests/:id/reset/preview`                        | Return the tasks affected by resetting a watch                           |
| `POST`   | `/api/v1/azure-devops/watches/pull-requests/:id/reset`                                | Apply cleanup and advance the pull-request watch generation              |

Search requests contain `project`, `wiql`, and an optional `top` value. The
service hydrates WIQL references in batches no larger than 200. Descriptions
returned as HTML are sanitized before display.

Team requests contain `project`. Board-list requests contain `project` and
`team`. Board-detail requests use the same query parameters plus the provider
board/backlog ID in the path. The board-detail response contains board ID/name,
column definitions (`id`, `name`, type, split flag, and item limit), and
hydrated work items with revision, core card fields, column ID, and split-column
done state.

Board work-item updates contain `project`, `team`, `revision`, and at least one
of `assigneeAction`, `columnId`, or `columnDone`. `assigneeAction` is
`assign_current_user` or `unassign`; the browser never supplies an arbitrary
identity. The server resolves the current PAT identity, derives Azure field
reference names from the selected board, validates the target column, prepends
a JSON Patch `test` operation for `/rev`, and returns the updated normalized
work item. A stale revision returns HTTP 409 with code
`azure_devops_revision_conflict`.

Direct work-item updates contain `project`, `revision`, and exactly one
`assigneeAction`. They reuse the same identity lookup, `/rev` test, allowlist,
conflict response, and normalized response as board updates, but require no
team or board context. Detail opened from a board may additionally show the
board column/split controls. Detail opened from search does not guess a board
and therefore exposes assignment only.

Mutation completion follows the opening/scope ownership rules below.

Work-item detail exposes sanitized description plus normalized planning fields
when Azure returns any of Effort, Story Points, Size, Remaining Work, Original
Estimate, or Completed Work. Discussion uses Azure's paginated work-item
comments API, defaults to newest first, excludes deleted comments, and returns
an opaque continuation token. Core detail and discussion load independently:
core failure shows a core retry, while a discussion failure preserves the
loaded item and retries only the failed page. Provider HTML is rendered through
the existing raw-HTML-aware sanitized Markdown pipeline; scripts, unsafe
attributes, and unsafe URL schemes are never mounted.

Quick-action task creation uses the existing task dialog. After successful
creation, the client persists the task/work-item association before reporting
the launch complete and invalidates the workspace task-link cache. The detail
surface lists linked Kandev tasks through the identity matching described below.
A failed association is visible and
retryable; it is never represented as a successful link.

### Task/detail association identity

This section maps `AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.12` through
`AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.14` to the existing detail path.
The association's `projectId` is a provider project reference, not a guarantee
that the value is a GUID. `convertWorkItem` in
`apps/backend/internal/azuredevops/rest_client.go` projects
`System.TeamProject` into `WorkItem.Project`, which is normally a project name.
`AzureDevOpsTaskLauncher.linkCreatedTask` supplies that name to
`AssociateTaskWorkItem`, and the service stores the validated caller value.
The board selector instead uses `AzureDevOpsProject.id` and supplies that ID
to the shared detail hook.

`useAzureDevOpsWorkItemDetail` in
`apps/web/hooks/domains/azure-devops/use-azure-devops-work-item-detail.ts`
owns display matching. Its selected `projectId` and the current provider item
argument's `initialItem.project` form the only candidate project identities.
Ignore missing, empty, or whitespace-only candidate values; use a whitespace
check only to reject unusable candidates, without trimming the value used for
comparison. Compare remaining identities exactly, with no case folding,
URL-derived aliases, workspace default, or global project-name lookup.

Read associations with `listWorkspaceAzureDevOpsTaskWorkItems(workspaceId)`.
For each row, require the current workspace ID and work-item ID, plus a match
against at least one candidate project identity. Apply one filter to the
flattened workspace response: coinciding aliases do not append a row twice,
and every distinct matching association remains in response order. If the
item has no usable project name, selected-ID matching continues independently.
If neither candidate is usable, no row matches. Keep the existing request
and generation lifecycle; this predicate does not create an identity cache.

The desktop dialog and phone drawer consume this same hook and existing
linked-task markup in `azure-devops-work-item-detail.tsx`. This is a data-only
correction: no layout, navigation, touch, scrolling, or translated-copy change.
Targeted actual-hook tests satisfy mobile parity's data-only exception; they
do not claim rendered mobile or browser coverage.

Regression coverage belongs in
`apps/web/hooks/domains/azure-devops/use-azure-devops-work-item-detail.test.ts`.
Mount the actual hook with independently authored, typed first-party data:
selected project GUID, provider item project name, and a normal name-valued
association. Assert the returned linked rows, with ID-valued associations,
missing/blank names, coinciding aliases, multiple links, and mixed foreign
workspace/project/item rows as controls. Do not substitute tests of a copied
matching helper for this consumer coverage.

No association write, backend validation, schema, migration, API, authentication,
provider mutation, or cache lifecycle change is required. Existing GUID-valued
rows remain readable; this does not expand which inputs the association service
accepts. Historical names after provider renames remain outside this contract.

Watcher task metadata uses exact provider-specific keys. Work-item tasks write
`azure_devops_work_item_watch_id`, `azure_devops_project_id`,
`azure_devops_work_item_id`, and `azure_devops_work_item_url`. Pull-request
tasks write `azure_devops_pr_watch_id`, `azure_devops_project_id`,
`azure_devops_repository_id`, `azure_devops_pull_request_id`, and
`azure_devops_pull_request_url`. Each watcher source's `WatchMetadataKey`
returns its corresponding watch ID key so shared in-flight counting sees the
same metadata written during task creation.

Task repository inputs use the provider-neutral `remote_url` field. The legacy
`github_url` field remains accepted during migration and is normalized to the
same internal input. Provider metadata supplied by the browser is treated as a
hint and revalidated from the configured provider before persistence or clone.

## Permissions

- Any user who can configure a Kandev workspace can manage that workspace's
  Azure DevOps connection under the same authorization model as Jira and
  Linear configuration.
- The board-enabled PAT requires Azure DevOps **Work Items: Read & write** and
  **Code: Read**. Kandev does not request thread write or code write
  permissions in this release.
- Existing Work Items Read PATs can still load boards but receive a permission
  error and reconnect guidance when attempting an assignment or column change.
- The backend may use the Code Read permission for a one-time authenticated Git
  clone. It supplies the PAT through an ephemeral Git credential mechanism and
  clears that mechanism when the child process exits.
- Credentials from one workspace must never be used to answer a request for a
  different workspace.
- Only users authorized for a workspace may read its Azure detail, preferences,
  task links, presets, or watches. Watcher-created tasks inherit the watch's
  workspace and never dispatch into another workspace.

## Failure Modes

- Missing workspace configuration returns a typed not-configured response and
  a connection CTA; it does not invoke `gh` or `az`.
- A 401 or 403 marks the connection unhealthy and surfaces an authentication or
  permission error without deleting the stored PAT.
- Rate limiting, timeouts, and Azure 5xx responses preserve the last known
  health and PR association data while surfacing staleness and the current
  error.
- Invalid organization URLs, unsupported hosts, missing workspace IDs, and
  malformed WIQL are rejected without persistence.
- A WIQL result larger than one batch is hydrated in deterministic batches;
  one omitted/deleted work item does not corrupt the rest of the page.
- Missing projects, teams, or boards produce an explicit empty state and keep
  the project/team/board selectors usable.
- If a board references more than 200 work items, Kandev hydrates them in
  deterministic batches and preserves the provider's backlog order.
- A rejected drag or detail mutation leaves the last confirmed card visible, restores
  an optimistic desktop move, and shows the provider error without discarding
  the rest of the loaded board.
- A stale work-item revision returns a conflict, refreshes the board, and asks
  the user to retry against the latest fields rather than overwriting them.
- A 403 received during a card mutation preserves readable board data and
  directs the user to replace the PAT with Work Items Read & write scope.
- A missing or stale remembered filter value falls back to a valid provider
  value and updates the in-memory selection; it never prevents the page from
  loading.
- A preference write failure leaves the current in-memory filters usable,
  reports a non-blocking save error, and lets the backend value win on reload.
- A work-item description or discussion failure keeps the detail surface open,
  shows the core item fields, and exposes a retry for only the failed section.
- A failed Assign to me, Unassign, or column change keeps detail open, restores
  the last confirmed fields, and leaves task actions and discussion usable.
- A failed immediate credential probe returns the saved configuration as
  unhealthy with the probe error. Integration navigation remains hidden until
  a later successful test, save, or health poll.
- A watcher authentication, query, or provider failure records the watch error
  and creates no task. A missing workflow, step, repository, agent profile, or
  executor profile disables the watch through the shared self-heal path.
- Watch reservation and task creation are generation-safe. A failed dispatch
  releases its reservation unless ownership was lost to a reset; retrying the
  same generation cannot create a second task for the same provider item.
- A watcher create/Run now result set larger than 100 is truncated
  deterministically and reports the bounded check time without dispatching an
  unbounded backlog.
- PR association fails closed when the repository is not attached to the task
  or is not an `azure_devops` repository.
- A repository selected from an integration is rejected if its canonical URL or
  provider identity no longer matches data returned for the active workspace.
- Failure to resolve or use server-side Azure clone credentials fails the clone
  without falling back to an unauthenticated URL containing a secret.
- Integration initialization errors are logged as non-fatal and the rest of
  the backend remains available.

## Persistence Guarantees

- Configuration, connection health, encrypted PATs, and task PR associations
  survive backend restarts.
- Per-user browse preferences, workspace query/action presets, task work-item
  associations, watches, reservations, configuration, connection health,
  encrypted PATs, and task PR associations survive backend restarts.
- Browse results, board snapshots, work-item discussion pages, PR feedback, and
  REST response caches are transient. Successful column and assignment changes
  persist in Azure DevOps.
- Deleting a workspace follows the existing integration cleanup behavior and
  removes its Azure configuration, PAT, and task PR associations.

## Scenarios

- **GIVEN** a workspace without GitHub CLI installed, **WHEN** a user saves a
  valid Azure organization and PAT, **THEN** the connection succeeds and Azure
  projects can be listed without executing `gh` or `az`.
- **GIVEN** two workspaces configured for different Azure organizations,
  **WHEN** each workspace searches work items, **THEN** each response contains
  only data accessible through that workspace's credential.
- **GIVEN** a valid WIQL query returning more than 200 references, **WHEN** a
  user runs the query, **THEN** Kandev hydrates the requested page in batches
  and returns normalized work items in query order.
- **GIVEN** a displayed Azure work item, **WHEN** a user launches a task from
  it, **THEN** the task-creation flow is populated with the work-item context
  and source URL.
- **GIVEN** a user is a reviewer on an active Azure PR, **WHEN** they select the
  reviewer preset, **THEN** the PR appears with its repository, branches, draft
  state, and current vote summary.
- **GIVEN** an Azure PR linked to a Kandev task, **WHEN** reviewer votes,
  threads, or policy evaluations change upstream, **THEN** a refresh updates
  the displayed summary while retaining Azure-native detail.
- **GIVEN** an expired PAT, **WHEN** the health poller checks the connection,
  **THEN** settings shows the connection as unhealthy with a reconnect action
  and existing PR associations remain stored.
- **GIVEN** a user saves or deletes an integration configuration, **WHEN** the
  request succeeds, **THEN** settings status and home integration navigation
  update without waiting for the periodic health poll.
- **GIVEN** valid new credentials, **WHEN** a user saves the Azure DevOps
  connection, **THEN** the save performs an immediate probe and the active
  sidebar/home entry appears within one second of the successful response.
- **GIVEN** multiple configured source-control providers, **WHEN** a user opens
  the Remote repository picker, **THEN** a bottom tab is shown for each
  available repository provider, only the active provider's matching results
  are visible, and selections retain the correct provider icon and branch
  source on desktop and mobile. The tab footer does not scroll vertically, and
  three-provider tabs compact to icons with accessible provider names.
- **GIVEN** only one configured source-control provider, **WHEN** a user opens
  the Remote repository picker, **THEN** its repositories are shown without a
  provider tab bar.
- **GIVEN** a private Azure repository selected for a task, **WHEN** Kandev
  materializes it, **THEN** the backend uses the workspace PAT for the clone and
  no task or agent-visible value contains the PAT.
- **GIVEN** a user chooses an Azure preset or saved view, **WHEN** they search,
  **THEN** Kandev applies the preset's provider-native query while Advanced WIQL
  remains available for custom work-item searches.
- **GIVEN** a configured default project with an accessible team and a
  requirement board, **WHEN** a user opens `/azure-devops`, **THEN** Board mode
  loads that board's Azure columns and cards without a separate search action.
- **GIVEN** an organization with several projects, teams, and boards, **WHEN**
  the user changes project, team, or board, **THEN** each dependent selector
  resets to the first valid child and only the selected board's cards appear.
- **GIVEN** a user selected Azure mode, preset, project, team, board, column,
  and filters in a workspace, **WHEN** the same user reloads or signs in on
  another device, **THEN** the last valid values are restored for that
  workspace without affecting another user's or workspace's choices.
- **GIVEN** a current board card, **WHEN** a desktop user drags it to another
  column, **THEN** Azure DevOps is updated and the card remains in that column
  after a board refresh.
- **GIVEN** a current board card, **WHEN** a user opens it, **THEN** a desktop
  dialog shows its description, planning/effort fields, and discussion while
  preserving the board behind it.
- **GIVEN** a current board card assigned to another user or unassigned,
  **WHEN** the user chooses Assign to me, **THEN** the backend assigns the item
  to the Azure identity represented by the workspace PAT and refreshes the
  displayed revision.
- **GIVEN** an assigned current board card, **WHEN** the user chooses Unassign,
  **THEN** the assignee is cleared without changing title, tags, description,
  or effort.
- **GIVEN** another client updated a card after Kandev loaded it, **WHEN** the
  user changes its assignee or column in Kandev, **THEN** Kandev reports a
  conflict, reloads the board, and does not overwrite the newer Azure revision.
- **GIVEN** a narrow mobile viewport, **WHEN** a user navigates columns, opens a
  card, reads its detail/discussion, assigns it to themselves, and moves it to
  another column, **THEN** all actions complete through a focused column and
  full-height detail surface without document horizontal scrolling.
- **GIVEN** a work item or pull request and configured quick actions, **WHEN**
  a user chooses an action, **THEN** Kandev opens task creation with the
  provider URL, title, description, repository/branch when available, and
  selected prompt prefilled; successful work-item creation persists its
  association.
- **GIVEN** a workspace has no Azure query or quick-action override, **WHEN** a
  user opens Azure browse or settings, **THEN** current Kandev built-ins are
  resolved without persisting a copied default list.
- **GIVEN** an enabled work-item or pull-request watch, **WHEN** a new
  non-reserved provider item matches its filters, **THEN** one Kandev task is
  created in the configured workflow context and records the watch/item
  metadata.
- **GIVEN** the same provider item is returned by later polls in the same
  generation, **WHEN** its watch runs again or the backend restarts, **THEN**
  its existing reservation prevents another task from being created.
- **GIVEN** an existing watcher-created task, **WHEN** the user previews and
  confirms watch reset, **THEN** cleanup follows the configured policy, the
  generation advances, and old in-flight dispatch cannot attach to the new
  generation.
- **GIVEN** a watcher references a deleted workflow step, repository, agent
  profile, or executor profile, **WHEN** dispatch validates its dependencies,
  **THEN** the watch is disabled with a visible error and no task is created.
- **GIVEN** core work-item detail loads but discussion fails, **WHEN** the user
  retries discussion, **THEN** the dialog remains open and the loaded core
  fields and actions are not discarded or refetched unnecessarily.
- **GIVEN** a narrow mobile viewport, **WHEN** a user configures Azure DevOps or
  browses work items and PRs, **THEN** all filters and primary actions remain
  reachable without horizontal page scrolling.

## Out Of Scope

- Azure DevOps Server or Team Foundation Server installations.
- Microsoft Entra OAuth, service principals, and managed identities.
- Creating, deleting, or changing the type of work items.
- Editing titles, tags, descriptions, priorities, estimates, parent
  relationships, area paths, iteration paths, or arbitrary provider-specific
  work-item fields.
- Assigning a work item to an arbitrary Azure DevOps user. This iteration
  supports Assign to me and Unassign.
- Reordering cards within one column or changing backlog priority.
- Editing board columns, WIP limits, split-column configuration, swimlanes,
  card styles, or other board metadata. Existing split-column done state is
  preserved and may be changed from work-item detail, but split subcolumns and
  swimlane rows are not rendered as separate lanes in this release.
- Creating, approving, commenting on, abandoning, or completing pull requests.
- Adding, editing, deleting, or reacting to work-item discussion comments.
- Automatic CI repair, auto-merge, and Azure Pipelines log streaming.
- Service-hook/webhook ingestion; reads and refreshes use requests, local
  invalidation after configuration mutations, and polling for recovery.
- Using the Azure PAT for agent-authored pushes, pull-request creation, or any
  other write operation.
- Requiring Azure CLI or the Azure DevOps CLI extension. The existing optional
  agentctl PR-create fallback remains separate until write support is added.

## Implementation Plan

See [the active implementation plan](../../../plans/azure-devops-integration/plan.md).

## Saved-view mutation settlement

`useAzureDevOpsSavedViews` owns one ordered mutation queue for a mounted Azure
page. `AzureDevOpsPageClient` keys its content by workspace ID. This is a
same-page ordering fix; changing hook props alone does not demonstrate a
cross-workspace page defect.

The existing GET and whole-list PUT `/api/v1/azure-devops/views` API is unchanged.
The hook retains the last confirmed server list plus ordered pending updates
in a ref-owned scope that survives renders and is replaced only when workspace
identity changes.
It publishes the confirmed list with pending updates applied, synchronously
admitting updates before another callback can run. The page appends a saved
view with a functional update; delete filters by ID. These operations can be
reapplied without carrying a failed earlier operation in a captured snapshot.
Explicit whole-list replacement remains supported for its existing semantics.

Writes wait for the initial read and preceding write to settle. Each request
applies its update to the confirmed list at dispatch. Success accepts the
normalized server response; failure leaves the confirmed baseline unchanged.
Either outcome removes the settled update and republishes the remaining
optimistic updates. Rejection remains visible to the initiating caller and
does not poison the queue. An initial GET contributes the baseline beneath
pending updates rather than replacing their visible intent. A failed read can
leave the display empty, but never confirms an empty server list. Every action
captures the read promise at admission; if that read fails, each bound action
rejects without a PUT and loses only its optimistic effect. A later retry
starts one shared fresh GET before writing against the recovered baseline.
Binding rejection to the admitted read prevents a retry from making an older
failed action eligible to write.

The page handles delete rejection through the shared reporting toast with
`integrations:failedToDeleteSavedQuery`; the save dialog keeps its existing
localized failure and retry flow. Desktop scope menus and phone bottom menus
retain their existing confirmation surfaces and share the same hook.
Queued writes finish for their captured workspace after unmount; publication
stops. There is no shared cache, cross-mount queue, backend revision protocol,
or claim of multi-tab consistency. An ambiguous transport failure is not proof
that the server rejected the write; broader reconciliation remains excluded.

This section satisfies AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.15 through
AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.18. Deferred hook tests prove request
order, confirmed-baseline recovery, read settlement, normalization, and retry.
Desktop and mobile browser tests exercise the actual confirmation and failure
feedback, and query the backend to prove the final stored list.

## Mutation completion ownership

For AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.19, capture an opening owner
before mutation. Close, reopen, item/scope change and unmount retire it;
background refresh does not. Only the current owner can merge detail, update
selection/focus, return Move data, or settle busy/error state.

Assignment retains its direct endpoint. Its receipt merges into the board
through the board hook's workspace/project/team/board owner; Move uses that
owner too. Success can update that board after Close, but cannot publish data,
rollback or error into a newer context. Higher confirmed revisions win.
Selection and canonical data remain separate. Provider payloads, permissions,
gestures and layout remain unchanged.
