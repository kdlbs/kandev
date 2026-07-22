---
status: building
created: 2026-05-04
updated: 2026-07-22
owner: tbd
---

# GitLab Integration

## Why

Teams whose code lives on GitLab cannot complete the same task, review, and
automation workflows available for GitHub without leaving Kandev. Existing
GitLab support can browse merge requests and issues and contains partial watch
and review plumbing, but its connection is installation-wide and the main
workflows are not usable end to end.

## What

- GitLab and GitHub can be connected at the same time. Each integration only
  reads or mutates its own provider.
- Each Kandev workspace owns exactly one GitLab connection: one normalized host
  URL, one authentication method, one credential, and one health record.
- The default host is `https://gitlab.com`; self-managed `http://` and
  `https://` origins are supported for API calls, web links, clone URLs, and
  merge request creation. Kandev preserves the configured scheme.
- A workspace can authenticate with a personal access token or a `glab` login
  for its configured host. `GITLAB_TOKEN` remains an explicit deployment
  fallback, but it is never persisted and only applies to workspaces configured
  to use that fallback.
- GitLab browse, task-link, review, watch, and write endpoints require an
  authoritative `workspace_id` and resolve that workspace's connection. Data
  or credentials from another workspace are never used as fallback.
- GitLab repository matching uses provider, normalized provider host, and full
  subgroup project path. Repositories with unknown or mismatched provider hosts
  are not eligible for GitLab linking or merge-request actions.
  Decision: ADR-2026-07-20-repository-provider-origin-identity.
- Users can browse and search merge requests and issues, then launch a task
  from either row with the same configurable action presets used by GitHub.
- Launching from a merge request records the task-to-MR association after task
  creation. Launching from an issue includes the issue as task context but does
  not create a durable issue-sync relationship.
- Users can link an existing task to a merge request by pasting a full MR URL,
  including URLs from the workspace's configured self-managed host. They can
  unlink one association without deleting the task or upstream MR.
- For a workspace with GitLab configured, task context menus expose
  `GitLab Merge Request` inside the shared `Link` submenu. Desktop users can
  reach it by right-clicking a task, and touch users can reach the same action
  through the task row's visible actions menu.
- An unlinked task does not show a persistent `Link MR` action in its task top
  bar. After at least one MR is linked, the top bar shows the linked-MR status
  control and continues to allow opening, unlinking, or linking another MR.
- Linked MRs are visible from both the GitLab list and task detail. Multiple
  tasks can link to one MR, and a multi-repository task can link one MR per
  repository.
- A linked MR has an in-app review surface with title and description, source
  and target branches, mergeability/conflicts, files, commits, approvals,
  reviewers, pipeline rollup, and threaded discussions.
- From that surface, users can reply to and resolve discussions, approve or
  unapprove, merge, update labels and assignees, and set individual reviewers.
  Project-member search powers reviewer selection and uses GitLab numeric user
  IDs, not display names.
- Review feedback can be added to the active task session's prompt context.
  Adding context does not mutate the MR.
- Users can subscribe or unsubscribe the authenticated GitLab user from an
  individual issue or MR. This controls GitLab notifications upstream and does
  not create a Kandev automation watch.
- Users can configure workspace-scoped Kandev review watches for MRs requesting
  their review and issue watches for matching issues. A watch selects a
  workflow step, agent profile, executor profile, prompt, optional project and
  query filters, cleanup policy, enabled state, and poll interval.
- Watch settings provide create, edit, enable/pause, run-now, reset, and delete
  controls. Delete removes the watch and its dedup rows and best-effort deletes
  every task the watch created; it does not rerun the watch. Reset first previews
  the number of owned tasks, then after confirmation best-effort attempts all
  owned task deletions (including archived tasks), clears dedup rows, retains
  the watch definition, and makes current matches eligible again. Review-watch
  reset schedules an immediate rerun; issue-watch reset is reconsidered on the
  next poll/run-now.
- A newly observed watch match creates at most one task for the watch and
  external item. The task is linked to its MR when the match is a merge request,
  and configured auto-start behavior uses the selected profiles.
- The task changes panel can create a GitLab merge request for a GitLab remote.
  It pushes the current branch, respects an explicitly selected target branch
  or otherwise uses the project default, supports draft MRs, returns the MR URL,
  and records the association against the originating task/repository.
- Stored workspace tokens are injected only into executions for that workspace
  as `GITLAB_TOKEN`. Host-aware clone and MR creation never silently fall back
  from a self-managed host to `gitlab.com`.
- Settings can copy a connection to another workspace. Copying overwrites the
  target connection after confirmation but never copies automation watches or
  task-to-MR associations.

## Data model

### `gitlab_configs`

One row per workspace:

| Field | Type | Constraint |
| --- | --- | --- |
| `workspace_id` | string | PK, FK to `workspaces.id`, cascade delete |
| `host` | string | required normalized HTTP(S) origin, no trailing slash |
| `auth_method` | enum | `pat`, `glab_cli`, or `environment` |
| `username` | string | last authenticated username, empty before a successful probe |
| `last_ok` | bool | last completed health result |
| `last_error` | string | sanitized provider/transport error |
| `last_checked_at` | timestamp | nullable |
| `created_at` | timestamp | required |
| `updated_at` | timestamp | required |

The PAT is stored in the secret store under
`gitlab:<workspace_id>:token`; secret values never appear in config or status
responses. `glab_cli` and `environment` rows do not copy host-machine auth data
into the secret store.

### Task and watch records

- `gitlab_task_mrs` remains the durable task-to-MR association. Its unique key
  is `(task_id, repository_id, project_path, mr_iid)`; `task_id` and a non-empty
  `repository_id` must belong to the same workspace as the resolved connection.
- `gitlab_review_watches` and `gitlab_issue_watches` are workspace-owned durable
  automation definitions. Their workflow, workflow step, repository (when
  present), agent profile, and executor profile must belong to that workspace.
- `gitlab_review_mr_tasks` and `gitlab_issue_watch_tasks` are reservation/dedup
  records. Reservation occurs before task creation; a failed dispatch releases
  the reservation, while a successful dispatch attaches the created task ID.
- `gitlab_mr_watches` remains the linked-MR refresh record for an active task
  session. It is not a replacement for a review watch.
- GitLab notification subscription state is owned by GitLab. Kandev reads it
  live and does not duplicate it in SQLite.

## API surface

All routes below are under `/api/v1/gitlab`. `workspace_id` is required unless
the workspace is unambiguously derived from the referenced task and is still
validated against any supplied value.

### Connection

- `GET /config?workspace_id=<id>` returns host, auth method, username, health,
  and `has_secret`; returns `204` when unconfigured.
- `PUT /config?workspace_id=<id>` accepts
  `{host, auth_method, token?}` and returns the saved config after validation.
- `DELETE /config?workspace_id=<id>` deletes the config and workspace secret;
  watch definitions remain persisted with their enabled flags unchanged but
  cannot poll or dispatch until the workspace is configured again.
- `POST /config/test?workspace_id=<id>` tests `{host, auth_method, token?}`
  without persisting it.
- `POST /config/copy?workspace_id=<source>` accepts
  `{targetWorkspaceId}` and copies connection settings and a stored PAT only.
- Existing status, project, search, feedback, watch, preset, and write routes
  gain the same required workspace scope.

### Task-to-MR association

- `POST /task-mrs?workspace_id=<id>` accepts
  `{task_id, repository_id?, mr_url}`. It parses and validates the URL host,
  fetches the MR through the workspace client, and idempotently returns a
  `TaskMR`.
- `DELETE /task-mrs/:association_id?workspace_id=<id>` removes only that
  association and its refresh watch.
- `GET /workspaces/:workspace_id/task-mrs` and `GET /tasks/:task_id/mrs`
  return associations visible to the requested workspace/task.

### Browse and review

- `GET /user/mrs?workspace_id=<id>&filter=<filter>&page=<n>&per_page=<n>` and
  `GET /user/issues?workspace_id=<id>&filter=<filter>&page=<n>&per_page=<n>`
  return the active workspace's paginated search results.
- `GET /mrs/feedback?workspace_id=<id>&project=<path>&iid=<n>` returns MR,
  approvals, discussions, and pipeline rollup.
- `GET /mrs/files?workspace_id=<id>&project=<path>&iid=<n>` and
  `GET /mrs/commits?workspace_id=<id>&project=<path>&iid=<n>` return changed
  files and commits for the same workspace/project/IID identity.
- `POST /mrs/discussions/notes?workspace_id=<id>` accepts
  `{project, iid, discussion_id, body}`; `POST /mrs/discussions/resolve` accepts
  `{project, iid, discussion_id}`.
- `POST /mrs/approve?workspace_id=<id>` and
  `POST /mrs/unapprove?workspace_id=<id>` accept `{project, iid}`.
  `PUT /mrs/merge?workspace_id=<id>` accepts
  `{project, iid, squash, squash_commit_message?}`.
- `PUT /mrs/labels?workspace_id=<id>` accepts `{project, iid, labels}` and
  `PUT /mrs/assignees?workspace_id=<id>` accepts
  `{project, iid, assignee_ids}`.

### Automation watches

Review and issue watches use the same route shape under `/watches/review` and
`/watches/issue`:

- `GET /watches/<kind>?workspace_id=<id>` lists the workspace's watches.
- `POST /watches/<kind>?workspace_id=<id>` creates a watch; the authoritative
  workspace comes from the query, not a body-supplied workspace ID.
- `PATCH /watches/<kind>/:id?workspace_id=<id>` partially updates a watch;
  `DELETE` applies the delete semantics above.
- `POST /watches/<kind>/:id/trigger?workspace_id=<id>` runs an enabled watch.
- `GET /watches/<kind>/:id/reset/preview?workspace_id=<id>` returns
  `{taskCount}`; `POST` to the same path without `/preview` executes reset and
  returns `{tasksDeleted}`.

### Reviewers and notifications

- `GET /projects/members?workspace_id=<id>&project=<path>&query=<text>` returns
  matching active project members as `{id, username, name, avatar_url}`.
- `PUT /mrs/reviewers?workspace_id=<id>` accepts
  `{project, iid, reviewer_ids}` and replaces the MR reviewer list.
- `GET /mrs/subscription?workspace_id=<id>&project=<path>&iid=<n>` and
  `GET /issues/subscription?workspace_id=<id>&project=<path>&iid=<n>` return
  `{subscribed: boolean}`.
- `PUT /mrs/subscription?workspace_id=<id>` and
  `PUT /issues/subscription?workspace_id=<id>` accept
  `{project, iid, subscribed}` and subscribe or unsubscribe upstream.

### Merge request creation

The existing `worktree.create_pr` WebSocket operation remains provider-neutral.
For a GitLab remote its successful response is
`{success: true, pr_url: <merge-request-url>, provider: "gitlab"}`; the product
labels the operation and result as "merge request" while preserving the
protocol action name for compatibility.

### Errors

- `400` indicates malformed URLs, hosts, filters, IDs, or request bodies.
- `404` indicates an absent resource or a resource outside the requested
  workspace; cross-workspace lookups do not reveal that a resource exists.
- `409` indicates a workspace/resource invariant conflict that cannot be
  applied idempotently.
- `422` indicates GitLab rejected a valid write, such as an ineligible reviewer.
- `503` indicates the workspace connection is absent or currently unavailable.
- Provider error bodies and logs are sanitized and never include tokens or
  authenticated remote URLs.

## State machine

### Connection health

- `unconfigured -> checking`: a config is saved or tested.
- `checking -> connected`: GitLab authenticates and returns the current user.
- `checking -> auth_required`: GitLab returns an authentication/authorization
  failure.
- `checking -> unavailable`: transport, timeout, or GitLab 5xx failure.
- `connected|auth_required|unavailable -> checking`: the health poller runs or
  the user explicitly tests/reconnects.
- Any state `-> unconfigured`: the workspace config is deleted.

### Automation watch

- `enabled`: scheduled polls and run-now can dispatch matches.
- `paused`: no scheduled or manual dispatch; configuration and dedup rows stay.
- `resetting`: the confirmed reset attempts every owned task deletion, clears
  owned dedup rows, and retains the watch. Review watches rerun immediately;
  issue watches rerun on their next poll or run-now.
- `error`: an invalid/deleted bound profile or repository disables the watch and
  records a sanitized error; editing and re-enabling returns it to `enabled`.

## Permissions

- GitLab configuration, watch, link, reviewer, subscription, and MR actions use
  the same workspace/task authorization boundary as their containing Kandev
  routes. A caller must have write access to the referenced workspace/task.
- Browse, feedback, member, and subscription reads require read access to the
  workspace and use only that workspace's GitLab connection.
- The configured GitLab identity must itself have upstream permission. Kandev
  does not elevate GitLab privileges or bypass protected-branch, approval, or
  reviewer eligibility rules.
- PAT mode requires GitLab `api` scope for the complete feature. Insufficient
  scope surfaces as `auth_required` or an action-specific error without
  deleting the saved config.

## Failure modes

- Saving an invalid host or credential fails without replacing the last known
  working connection or secret.
- A transient health or API failure keeps the saved config and linked/watch
  records. The UI shows an unavailable state and allows retry; pollers back off
  and do not create tasks from incomplete results.
- Revoked credentials mark only the affected workspace `auth_required` and
  pause API work for it until a successful probe. Other workspaces continue.
- Watch dispatch reserves the external item before creating a task. Task-create
  or auto-start failure releases or completes the reservation according to the
  shared watcher dispatcher, preventing both lost matches and duplicate tasks.
- A deleted/soft-deleted watch dependency self-disables the watch with a visible
  error instead of creating an orphan task.
- Link creation is atomic from the user's perspective: if URL validation or MR
  fetch fails, no association is written. Repeating a successful request
  returns the existing association.
- Unlink failure leaves the association and refresh watch intact. Unlink never
  closes or unsubscribes from the upstream MR.
- Watch delete is best-effort for owned-task cleanup: individual task-delete
  failures are logged, remaining owned tasks are attempted, and the watch/dedup
  rows are removed. It never schedules another poll.
- Watch reset follows the shared GitHub `watchreset` contract: after preview and
  confirmation it best-effort attempts every owned task deletion (including
  archived), logs and continues past individual task-delete failures, then
  transactionally clears dedup and `last_polled_at`. A clear failure surfaces
  together with the count already deleted; otherwise the response reports the
  successful delete count. Review watches rerun immediately and issue watches
  are eligible on their next poll/run-now.
- Reviewer, discussion, merge, and subscription failures leave the last fetched
  UI state visible and show an action error; the UI refreshes after success.
- MR creation never retries against another host. A successful push followed by
  a failed MR request is reported as partial failure with the pushed branch
  intact so the user can retry without another commit.

## Persistence guarantees

- Workspace config rows, PAT secrets, watch definitions, dedup reservations,
  task-to-MR associations, and last known MR status survive backend restarts.
- The startup migration moves the legacy global host/token to the active
  workspace, or the earliest-created workspace when no active workspace is
  available. It is idempotent and never duplicates automation watches.
- `glab` login and `GITLAB_TOKEN` remain host-process state and are not copied or
  backed up by Kandev.
- In-flight HTTP requests and poll iterations do not resume after restart. The
  next health/poll cycle re-runs safely against durable dedup state.
- GitLab notification subscriptions survive because GitLab owns them; Kandev
  re-reads their state after reload.

## Scenarios

- **GIVEN** two workspaces connected to different GitLab hosts, **WHEN** each
  opens its GitLab page, **THEN** each sees only data fetched with its own host
  and credential.
- **GIVEN** a legacy global GitLab host and token, **WHEN** Kandev starts after
  upgrade, **THEN** one deterministic workspace receives the config and secret
  and other workspaces remain unconfigured.
- **GIVEN** a self-managed workspace connection, **WHEN** a user links a valid
  MR URL from that host, **THEN** the task shows the linked MR and its live
  review details after reload.
- **GIVEN** a task with no linked MR in a GitLab-configured workspace, **WHEN**
  the task detail opens, **THEN** the top bar has no `Link MR` action and the
  task's contextual `Link` submenu offers `GitLab Merge Request`.
- **GIVEN** a touch viewport and a task with no linked MR in a GitLab-configured
  workspace, **WHEN** the user opens the task row's visible actions menu and
  chooses `Link` then `GitLab Merge Request`, **THEN** the GitLab MR link dialog
  opens without relying on right-click or long press.
- **GIVEN** a task with a linked GitLab MR, **WHEN** the task detail opens,
  **THEN** the top bar shows the linked MR status control rather than a generic
  link action.
- **GIVEN** an MR URL from a different host, **WHEN** it is linked in the current
  workspace, **THEN** the request is rejected and no association is written.
- **GIVEN** a linked MR, **WHEN** the user unlinks it, **THEN** it disappears
  from the task and GitLab list indicator while the upstream MR is unchanged.
- **GIVEN** a GitLab MR or issue search row, **WHEN** the user launches a preset,
  **THEN** the task-create dialog is prefilled with the matching project and
  context and successful creation navigates to the task.
- **GIVEN** a linked MR with discussions, approvals, conflicts, and a pipeline,
  **WHEN** the user opens its task review panel, **THEN** all states and thread
  actions are available without leaving Kandev.
- **GIVEN** an eligible project member, **WHEN** the user selects them as a
  reviewer, **THEN** GitLab and the refreshed MR panel both show that reviewer.
- **GIVEN** an MR or issue the user does not subscribe to, **WHEN** they enable
  notifications, **THEN** GitLab reports it subscribed and no Kandev watch or
  task is created.
- **GIVEN** an enabled review watch, **WHEN** a new matching MR requests the
  user as reviewer, **THEN** one linked task is created in the configured step
  and auto-starts with the configured profiles when requested.
- **GIVEN** an enabled issue watch, **WHEN** a matching issue appears, **THEN**
  one task is created with the issue URL and interpolated issue context.
- **GIVEN** a paused watch, **WHEN** scheduled polling or run-now occurs,
  **THEN** no task is created and existing dedup state is retained.
- **GIVEN** a watch with owned tasks, **WHEN** the user previews and confirms
  reset, **THEN** all shown tasks are attempted, the response reports successful
  deletions, dedup state is cleared, the watch remains enabled, and currently
  matching items can create new tasks immediately (review) or next poll (issue).
- **GIVEN** a watch with owned tasks, **WHEN** the user deletes the watch,
  **THEN** Kandev best-effort deletes those tasks and removes the watch/dedup
  state without recreating tasks from current matches.
- **GIVEN** a watch whose bound profile or repository was deleted, **WHEN** a
  match is dispatched, **THEN** the watch disables with a visible error and no
  orphan task is created.
- **GIVEN** a GitLab task branch with committed changes, **WHEN** the user
  creates a draft merge request, **THEN** the branch is pushed to the configured
  host, the returned MR URL is linked to that task repository, and the UI uses
  merge-request terminology on desktop and mobile.
- **GIVEN** a revoked token in Workspace A, **WHEN** health polling runs,
  **THEN** Workspace A shows `auth_required`, its watches stop dispatching, and
  Workspace B continues using its own connection.

## Out of scope

- GitLab webhook ingestion; this iteration uses polling.
- Durable GitLab issue-to-task linking, issue state synchronization, or
  Jira/Linear-style structured issue import.
- Editing GitLab approval rules, protected branches, CI configuration, pipeline
  jobs/logs, or merge request templates.
- Group-wide dashboards beyond results visible to the configured user.
- OAuth-based Kandev sign-in, GitLab Duo, repository migration between hosts,
  and Bitbucket parity.
- Multiple named GitLab connections inside one Kandev workspace.
