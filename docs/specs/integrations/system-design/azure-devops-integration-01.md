---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001
created: 2026-07-17
updated: 2026-07-31
owners:
  - tbd
---
# Azure DevOps Integration System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

Decision: [ADR-2026-07-20-provider-neutral-remote-repositories](../../../decisions/2026-07-20-provider-neutral-remote-repositories.md)

## Why

Teams whose source code and planning work live in Azure DevOps cannot use Kandev's
GitHub or GitLab browsing surfaces to find work items, inspect pull requests, or
associate a pull request with a task. Azure users must be able to connect their
workspace, work from the same team board they use in Azure DevOps, and inspect
Azure Repos data without installing or authenticating the GitHub CLI.

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
- The Azure DevOps browser includes a Board mode alongside Work items and Pull
  requests. Board mode is the default connected view and selects context in
  Azure's hierarchy: project, then team, then board/backlog level.
- Board mode initially selects the configured default project when available,
  the first accessible team, and the first visible requirement board (falling
  back to the first visible board). Users can change every level explicitly.
  Each user's last valid mode, preset, project, team, board, focused column,
  work-item filters, and pull-request filters are restored independently for
  each workspace on the next load.
- The selected board shows Azure's columns, column item counts and limits, and
  work-item cards with ID, title, type, assignee, and tags.
- On desktop, users can move cards between board columns by drag and drop. On
  mobile, the same move is available from work-item detail; touch drag is not
  required.
- Selecting a board card or work-item result opens a work-item detail surface.
  It shows the title, ID, type, state/board column, assignee, tags, sanitized
  description, available planning/effort fields, and paginated discussion.
- Work-item detail is read-only except for moving the item to another board
  column and assigning it to the Azure DevOps identity represented by the
  workspace PAT. An assigned item can also be unassigned. Kandev does not
  expose arbitrary assignee, title, tag, description, or effort editing.
- Card updates use the displayed Azure revision as an optimistic concurrency
  guard. Kandev never silently overwrites a newer Azure DevOps edit.
- Azure board mutations are sent through a fixed Kandev field allowlist. The
  browser cannot submit provider-native JSON Patch paths, bypass Azure rules,
  or suppress Azure notifications.
- The work-item and pull-request browse surface leads with named, provider-aware
  presets and supports workspace-scoped saved views. Raw WIQL remains available
  in an Advanced section instead of occupying the primary filter surface.
- Built-in work-item queries are Recently updated, Assigned to me, Active, and
  Created by me. Built-in pull-request queries are Review requested, Open,
  Completed, and Created by me. The workspace settings contract may override a
  preset family without freezing built-in defaults into a workspace row.
- Settings exposes those default queries with the same interaction model as
  GitHub: pull-request and work-item tabs, editable rows, Reset, dirty-state
  highlighting, and the shared floating Save changes control.
- Work items and pull requests expose workspace-configurable quick actions.
  Work-item defaults are Implement, Investigate, and Reproduce; pull-request
  defaults are Review, Address feedback, and Fix CI. Choosing an action opens
  the existing Kandev task-creation dialog with provider context and the
  selected prompt already populated.
- Azure quick-action settings follow GitHub's editor UX: pull requests first,
  work items second, icon/label/hint fields, an expandable prompt editor with
  placeholder completion, Reset, dirty-state highlighting, and the shared
  floating Save changes control. Azure settings orders connection, pull-request
  watches, work-item watches, quick actions, then default queries.
- A Kandev task created from an Azure work item remains associated with that
  work item. The browse and detail surfaces show existing associated tasks and
  avoid silently creating duplicate watcher tasks for the same watch match.
- Users can browse active pull requests by project and repository, including
  pull requests authored by them and pull requests where they are a reviewer.
- Pull-request detail includes branches, author, reviewers and votes, comment
  threads, linked work items, and branch-policy evaluation status.
- A pull request can be associated with a Kandev task. The association survives
  backend restarts and refreshes in the background without requiring the task's
  agent environment to contain Azure or GitHub tooling.
- Workspace administrators can create work-item watches from a project and
  WIQL query, and pull-request watches from project/repository/reviewer filters.
  Watches poll Azure DevOps directly, reserve each provider item once per watch
  generation, and create Kandev tasks in a selected workflow step using the
  selected repository, branch, agent profile, executor profile, prompt,
  cleanup policy, and optional in-flight task limit.
- Azure watches support enable/disable, edit, run now, reset preview, reset,
  and delete. Reset advances the watch generation so a matching provider item
  can be reconsidered without racing an older in-flight dispatch.
- Azure DevOps failures are isolated from GitHub, GitLab, Jira, and other
  integrations. An absent or invalid Azure connection does not prevent Kandev
  from starting.
- Saving, copying, replacing, or deleting any integration configuration updates
  integration navigation immediately. The 90-second health poll remains a
  recovery mechanism, not the expected propagation path after a local mutation.
- Saving credentials performs an immediate bounded authentication probe. On a
  successful save, the settings status, sidebar, and home integration entry
  reflect the active connection from the save response without waiting for the
  periodic poll; the local availability refresh completes within one second of
  the successful response.
- Configured integrations show an Enabled status in the expanded workspace
  settings navigation. Azure DevOps uses the official product mark consistently
  in settings, browse, and task-creation surfaces.
- The task-creation repository picker combines repositories from every
  configured source-control provider: GitHub, GitLab, and Azure DevOps. Users
  can still paste a supported HTTPS or SSH repository URL manually. When more
  than one repository provider is available, bottom tabs switch the visible
  provider results; no provider tab bar is shown for a single provider. When
  all three providers are available, compact icon tabs retain accessible names
  and expose provider names on hover.
- Azure DevOps private repositories can be materialized with the workspace PAT
  by the Kandev backend. The PAT is never added to task metadata, clone URLs,
  agent environment variables, logs, or persisted repository rows. Push access
  remains the responsibility of the selected executor's Git credentials.
- The Azure DevOps browse and settings surfaces provide equivalent desktop and
  mobile workflows.
- Desktop Board mode contains horizontal board scrolling inside the board
  surface. Mobile Board mode shows one focused column at a time with previous,
  next, and bottom-drawer project/team/board/column navigation; neither mode
  creates document-level horizontal scrolling.
- Organization URL inputs accept an optional trailing slash and persist the
  canonical URL without it.
- PAT setup instructions and the organization-specific token-settings link are
  available from an info control beside the PAT field on hover, focus, or tap.
- Selecting Work items runs the default query as soon as the connected
  project's filters are ready; users do not need to submit the initial search
  manually.

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
| `saved_views`          | text     | required JSON array, default `[]`                              |
| `workspace_settings`   | text     | required JSON object, default `{}`                             |

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

### Azure browse preferences

`users.settings.azure_devops_browse_preferences` is a per-user JSON object keyed
by workspace ID. Each entry contains the last selected mode, preset/saved-view
identity, project ID, team ID, board ID, focused column ID, work-item filter
values, and pull-request filter values. The backend user-settings record is the
only durable source of truth; browser storage is not a fallback. Provider IDs
are hints: an inaccessible or deleted value falls back to the first valid
choice without making the page unusable.

The SPA boot payload includes the complete Azure preference object before the
settings store is marked loaded. A hard refresh and client-side navigation must
therefore hydrate the same persisted project, team, board, column, mode, query,
and filter values; boot hydration may not replace them with page defaults.

### Azure query and action presets

Azure workspace settings contain nullable overrides for work-item and
pull-request default queries plus nullable overrides for work-item and
pull-request quick actions. A null override means “use current built-in
defaults”; an explicit non-empty list is the workspace customization. Query
presets contain ID, label, group, and provider-native filters. Action presets
contain ID, label, hint, icon key, and prompt template. Credentials and result
data are never included.

### `azure_devops_task_work_items`

One row associates a Kandev task with an Azure work item:

| Field           | Type     | Constraint       |
| --------------- | -------- | ---------------- |
| `id`            | text     | primary key UUID |
| `workspace_id`  | text     | required         |
| `task_id`       | text     | required         |
| `project_id`    | text     | required         |
| `work_item_id`  | integer  | required         |
| `work_item_url` | text     | required         |
| `title`         | text     | required         |
| `state`         | text     | required         |
| `created_at`    | datetime | required         |
| `updated_at`    | datetime | required         |

The tuple `(task_id, workspace_id, project_id, work_item_id)` is unique.

### Azure watches

`azure_devops_work_item_watches` and `azure_devops_pr_watches` are
workspace-owned records. Both contain workflow/step, repository/base branch,
agent/executor profile, prompt, enabled state, poll interval (default 300
seconds, minimum 60), cleanup policy, optional maximum in-flight tasks,
generation, deleting state, last check/error, and timestamps. `repository_id`
always means the Kandev repository used to create the task. Work-item watches
add the Azure project ID and WIQL. Pull-request watches add the Azure project
ID, optional Azure repository ID, status (default `active`), creator, and
reviewer filters.

Each watch kind has a reservation table keyed by watch ID, watch generation,
and provider item identity. Reservations record the matched URL and nullable
Kandev task ID so retries cannot create duplicate tasks and reset can safely
start a new generation.

The work-item reservation identity is `(watch_id, generation, project_id,
work_item_id)`. The pull-request reservation identity is `(watch_id,
generation, project_id, azure_repository_id, pull_request_id)`. Every attach
or release operation includes the current generation in its write condition.
A reset increments the generation before configured cleanup and removes
reservations from prior generations after cleanup. Delete marks the watch as
deleting and disables it before cleanup. An old in-flight dispatch that loses
generation ownership is terminal and cannot attach or release a reservation in
the new generation.

Create and Run now checks are bounded to 100 provider matches. The WIQL or
pull-request filters are authoritative for new matches. Polling also
reconciles provider terminal state for reservations that already own Kandev
tasks, applying `auto`, `always`, or `never` cleanup exactly as the shared
watcher contract defines. Cleanup never affects a manually created task or a
task created by another watch generation.

### Azure board state

Board definitions, columns, work-item membership, and work-item field values
remain provider-owned and are fetched on demand. Kandev does not persist a
board cache. A board work item includes its Azure revision, the board column ID
derived from the board's column field, and the split-column done value when the
board exposes a done field.
