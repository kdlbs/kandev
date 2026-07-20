---
status: building
created: 2026-07-17
owner: tbd
---

# Azure DevOps Integration

Decision: [ADR-2026-07-20-provider-neutral-remote-repositories](../../decisions/2026-07-20-provider-neutral-remote-repositories.md)

## Why

Teams whose source code and planning work live in Azure DevOps cannot use Kandev's
GitHub or GitLab browsing surfaces to find work items, inspect pull requests, or
associate a pull request with a task. Azure users must be able to connect their
workspace and read Azure Boards and Azure Repos data without installing or
authenticating the GitHub CLI.

## What

- An Azure DevOps connection is configured independently for each Kandev
  workspace.
- The first release supports Azure DevOps Services organizations hosted at
  `https://dev.azure.com/<organization>` and authenticates with a personal
  access token stored in Kandev's encrypted secret store.
- Azure DevOps reads use the Azure DevOps REST API directly. Neither `gh` nor
  `az` is required for connection checks, work-item reads, pull-request reads,
  or pull-request synchronization.
- Users can test, replace, copy to another workspace, and delete an Azure DevOps
  connection from Settings > Integrations > Azure DevOps.
- Users can browse work items returned by WIQL, inspect their core fields, and
  launch the existing task-creation flow with the work-item title, description,
  URL, project, type, state, and identifier available to the launcher.
- The work-item and pull-request browse surface leads with named, provider-aware
  presets and supports workspace-scoped saved views. Raw WIQL remains available
  in an Advanced section instead of occupying the primary filter surface.
- Users can browse active pull requests by project and repository, including
  pull requests authored by them and pull requests where they are a reviewer.
- Pull-request detail includes branches, author, reviewers and votes, comment
  threads, linked work items, and branch-policy evaluation status.
- A pull request can be associated with a Kandev task. The association survives
  backend restarts and refreshes in the background without requiring the task's
  agent environment to contain Azure or GitHub tooling.
- Azure DevOps failures are isolated from GitHub, GitLab, Jira, and other
  integrations. An absent or invalid Azure connection does not prevent Kandev
  from starting.
- Saving, copying, replacing, or deleting any integration configuration updates
  integration navigation immediately. The 90-second health poll remains a
  recovery mechanism, not the expected propagation path after a local mutation.
- Configured integrations show an Enabled status in the expanded workspace
  settings navigation. Azure DevOps uses the official product mark consistently
  in settings, browse, and task-creation surfaces.
- The task-creation repository picker combines repositories from every
  configured source-control provider: GitHub, GitLab, and Azure DevOps. Users
  can still paste a supported HTTPS or SSH repository URL manually.
- Azure DevOps private repositories can be materialized with the workspace PAT
  by the Kandev backend. The PAT is never added to task metadata, clone URLs,
  agent environment variables, logs, or persisted repository rows. Push access
  remains the responsibility of the selected executor's Git credentials.
- The Azure DevOps browse and settings surfaces provide equivalent desktop and
  mobile workflows.
- Organization URL inputs accept an optional trailing slash and persist the
  canonical URL without it.
- PAT setup instructions and the organization-specific token-settings link are
  available from an info control beside the PAT field on hover, focus, or tap.
- Opening the Azure DevOps browser runs the default work-item query as soon as
  the connected project's filters are ready; users do not need to submit the
  initial search manually.

## Data Model

### `azure_devops_configs`

One row per workspace:

| Field                  | Type     | Constraint                                                     |
| ---------------------- | -------- | -------------------------------------------------------------- |
| `workspace_id`         | text     | primary key                                                    |
| `organization_url`     | text     | required, canonical `https://dev.azure.com/<organization>` URL |
| `default_project_id`   | text     | optional project GUID                                          |
| `default_project_name` | text     | optional display name                                          |
| `auth_method`          | text     | `pat` in the first release                                     |
| `last_checked_at`      | datetime | nullable                                                       |
| `last_ok`              | boolean  | required, default false                                        |
| `last_error`           | text     | required, default empty                                        |
| `created_at`           | datetime | required                                                       |
| `updated_at`           | datetime | required                                                       |

The PAT is never stored in SQLite. It is stored under the encrypted secret key
`azure_devops:<workspace_id>:pat`.

### `azure_devops_task_prs`

One row per task, repository, and Azure pull request:

| Field                 | Type     | Constraint                                                      |
| --------------------- | -------- | --------------------------------------------------------------- |
| `id`                  | text     | primary key UUID                                                |
| `task_id`             | text     | required                                                        |
| `repository_id`       | text     | Kandev repository ID, required                                  |
| `organization_url`    | text     | required                                                        |
| `project_id`          | text     | required                                                        |
| `azure_repository_id` | text     | Azure repository GUID, required                                 |
| `pull_request_id`     | integer  | required                                                        |
| `pull_request_url`    | text     | required                                                        |
| `title`               | text     | required                                                        |
| `source_branch`       | text     | required, normalized without `refs/heads/` for display          |
| `target_branch`       | text     | required, normalized without `refs/heads/` for display          |
| `author_id`           | text     | required                                                        |
| `author_name`         | text     | required                                                        |
| `status`              | text     | `active`, `completed`, or `abandoned`                           |
| `review_state`        | text     | normalized summary: `approved`, `waiting`, `rejected`, or empty |
| `policy_state`        | text     | normalized summary: `success`, `pending`, `failure`, or empty   |
| `is_draft`            | boolean  | required                                                        |
| `last_synced_at`      | datetime | nullable                                                        |
| `created_at`          | datetime | required                                                        |
| `updated_at`          | datetime | required                                                        |

The tuple `(task_id, repository_id, azure_repository_id, pull_request_id)` is
unique. Provider-native reviewer votes, threads, and policy records are fetched
on demand and are not flattened into GitHub review/check records.

### Repository provider fields

Azure repositories use the existing repository fields with
`provider = "azure_devops"`, the Azure repository GUID in `provider_repo_id`,
the project ID in `provider_owner`, and the repository name in `provider_name`.
Provider-backed repositories also persist the provider-returned canonical HTTPS
clone URL in `remote_url`. This avoids reconstructing URLs from GitHub-specific
owner/name assumptions and allows remote executors to address Azure organizations
and GitLab self-managed hosts correctly. Credentials are never embedded in this
field.

### Saved Azure views

Saved Azure views are workspace-scoped JSON records containing an ID, label,
kind (`work_item` or `pull_request`), provider-native query/filter values, and a
creation timestamp. Invalid entries are ignored when read. Saving a view never
persists result data or credentials.

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
| `GET`    | `/api/v1/azure-devops/repositories`                                                   | List repositories, optionally filtered by project                        |
| `GET`    | `/api/v1/azure-devops/repositories/:projectId/:repositoryId/branches`                 | List repository branches for task creation                               |
| `GET`    | `/api/v1/azure-devops/views`                                                          | Return workspace-scoped saved Azure views                                |
| `PUT`    | `/api/v1/azure-devops/views`                                                          | Replace workspace-scoped saved Azure views                               |
| `POST`   | `/api/v1/azure-devops/work-items/search`                                              | Execute WIQL and return hydrated work items                              |
| `GET`    | `/api/v1/azure-devops/work-items/:id`                                                 | Return one hydrated work item                                            |
| `GET`    | `/api/v1/azure-devops/pull-requests`                                                  | List PRs by project, repository, status, author, or reviewer             |
| `GET`    | `/api/v1/azure-devops/pull-requests/:projectId/:repositoryId/:pullRequestId`          | Return PR detail                                                         |
| `GET`    | `/api/v1/azure-devops/pull-requests/:projectId/:repositoryId/:pullRequestId/feedback` | Return reviewers, threads, linked work items, and policies               |
| `GET`    | `/api/v1/azure-devops/workspaces/:workspaceId/task-prs`                               | Return task PR associations grouped by task                              |
| `POST`   | `/api/v1/azure-devops/tasks/:taskId/pull-requests`                                    | Validate and associate an Azure PR with a task repository                |
| `POST`   | `/api/v1/azure-devops/tasks/:taskId/pull-requests/sync`                               | Refresh persisted state for one association                              |

Search requests contain `project`, `wiql`, and an optional `top` value. The
service hydrates WIQL references in batches no larger than 200. Descriptions
returned as HTML are sanitized before display.

Task repository inputs use the provider-neutral `remote_url` field. The legacy
`github_url` field remains accepted during migration and is normalized to the
same internal input. Provider metadata supplied by the browser is treated as a
hint and revalidated from the configured provider before persistence or clone.

## Permissions

- Any user who can configure a Kandev workspace can manage that workspace's
  Azure DevOps connection under the same authorization model as Jira and
  Linear configuration.
- The initial PAT requires read access to Work Items and Code. Kandev does not
  request or exercise work-item write, thread write, or code write permissions
  in this release.
- The backend may use the Code Read permission for a one-time authenticated Git
  clone. It supplies the PAT through an ephemeral Git credential mechanism and
  clears that mechanism when the child process exits.
- Credentials from one workspace must never be used to answer a request for a
  different workspace.

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
- Browse results, PR feedback, and REST response caches are transient.
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
- **GIVEN** multiple configured source-control providers, **WHEN** a user opens
  the Remote repository picker, **THEN** repositories are grouped by provider,
  searchable in one control, and retain the correct provider icon and branch
  source on desktop and mobile.
- **GIVEN** a private Azure repository selected for a task, **WHEN** Kandev
  materializes it, **THEN** the backend uses the workspace PAT for the clone and
  no task or agent-visible value contains the PAT.
- **GIVEN** a user chooses an Azure preset or saved view, **WHEN** they search,
  **THEN** Kandev applies the preset's provider-native query while Advanced WIQL
  remains available for custom work-item searches.
- **GIVEN** a narrow mobile viewport, **WHEN** a user configures Azure DevOps or
  browses work items and PRs, **THEN** all filters and primary actions remain
  reachable without horizontal page scrolling.

## Out Of Scope

- Azure DevOps Server or Team Foundation Server installations.
- Microsoft Entra OAuth, service principals, and managed identities.
- Creating, updating, or transitioning work items.
- Creating, approving, commenting on, abandoning, or completing pull requests.
- Automatic CI repair, auto-merge, and Azure Pipelines log streaming.
- Service-hook/webhook ingestion; reads and refreshes use requests, local
  invalidation after configuration mutations, and polling for recovery.
- Using the Azure PAT for agent-authored pushes, pull-request creation, or any
  other write operation.
- Requiring Azure CLI or the Azure DevOps CLI extension. The existing optional
  agentctl PR-create fallback remains separate until write support is added.

## Implementation Plan

See [the active implementation plan](../../plans/azure-devops-integration/plan.md).
