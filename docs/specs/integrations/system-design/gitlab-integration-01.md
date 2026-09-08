---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITLAB-INTEGRATION-001
created: 2026-05-04
updated: 2026-08-05
owners:
  - tbd
---
# GitLab Integration System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
- Task creation is the narrow unauthenticated exception: branch discovery for
  an explicitly entered public `gitlab.com` repository URL works without a
  saved workspace connection. It does not expose private projects, browse
  results, merge requests, issues, or write actions.
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
- A merge request opened outside Kandev's own MR-creation action (e.g. `glab mr
  create` from an agent session, or opening the MR directly in the GitLab web
  UI) on a task's session branch is auto-linked: on the next push the open MR
  matching that branch is found and linked, scoped to the pushing repository,
  with an on-demand check available as an immediate alternative to waiting for
  a push or the background poller. Linking also creates the refresh watch that
  keeps review/pipeline status current, so no manual link step is required for
  this common case. Multi-repository and multi-branch tasks each get their own
  association and watch, scoped per `(repository_id, branch)`; re-linking an
  already-linked MR is a no-op rather than a duplicate.
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
- Each linked MR's topbar control shows an "Automation" group with two
  switches — `Auto-fix CI and address comments` and `Auto-merge when ready` —
  above a collapsible `Review follow-up` group holding the three lifecycle
  notification switches (`Your review is requested`, `MR merged`, `MR closed
  without merging`) introduced for MR lifecycle notifications. All five
  switches are scoped per linked MR (see the per-MR amendment in
  [the GitLab MR lifecycle ADR](../../../decisions/2026-08-01-gitlab-mr-lifecycle-notifications.md)):
  enabling a switch on one linked MR does not affect
  any other linked MR's switches, and a `PATCH`/MCP update that omits MR
  identity fans out to every linked MR, preserving prior agent behavior.
  Auto-fix and auto-merge additionally track per-MR round/attempt state. The
  auto-fix prompt override remains task-level, as does the resolved review
  reviewer username. See "Automation (lifecycle, auto-fix, auto-merge)" below.
- `Auto-fix CI and address comments` sends or queues an agent prompt when a
  linked MR's pipeline has a new or changed failing job, or a new or changed
  unresolved discussion note, capped at 10 accepted rounds per linked MR.
  `Auto-merge when ready` merges a linked MR only when it is open, not a
  draft, its pipeline succeeded, it has zero unresolved discussions, and
  GitLab's own merge-readiness verdict agrees.
- For a single linked MR on desktop (fine-pointer), hovering the topbar
  button opens a preview popover with everything: header actions (open the
  MR detail panel, open in GitLab, unlink), a pass-rate bar and pipeline
  stage groups, an approval row, an unresolved-discussions row, the
  Automation controls, a compact merge action when the MR is fully ready,
  and "Link another merge request" — mirroring GitHub's PR hover popover.
  Clicking the button opens the MR detail panel directly (no intermediate
  dropdown), also mirroring GitHub's single-PR topbar button. A task with
  2+ linked MRs, and touch/coarse-pointer surfaces regardless of MR count,
  keep the click-only dropdown (per-MR review/open/unlink rows, one
  Automation block per linked MR — each labeled with that MR's number, its
  auto-fix/auto-merge rows always visible and its nested `Review follow-up`
  group collapsed unless one of that MR's own three lifecycle switches is
  already on — and "Link another merge request") with no hover popover.
- The Kanban card shows a merge-request badge (`IconGitMerge`, coloured by
  state/pipeline/approval) next to the existing pull-request badge when the
  task has at least one linked MR. Multiple linked MRs collapse into one badge
  showing a count and the most attention-worthy open MR's colour.

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
  Associations survive restart and archive/unarchive, but hard task deletion
  removes them and their refresh watches. Decision: ADR-2026-08-13-hard-delete-task-contribution-links.
- `gitlab_review_watches` and `gitlab_issue_watches` are workspace-owned durable
  automation definitions. Their workflow, workflow step, repository (when
  present), agent profile, and executor profile must belong to that workspace.
- `gitlab_review_mr_tasks` and `gitlab_issue_watch_tasks` are reservation/dedup
  records. Reservation occurs before task creation; a failed dispatch releases
  the reservation, while a successful dispatch attaches the created task ID.
- `gitlab_mr_watches` remains the linked-MR refresh record for an active task
  session. It is not a replacement for a review watch. Its unique key is
  `(session_id, repository_id, branch)` — branch-scoped so a session with
  multiple worktrees on the same repository (multi-branch tasks) can hold one
  watch per branch instead of colliding on the second branch's insert. It is
  populated by both explicit URL linking and by push-detection/on-demand
  auto-link, and its periodic poll refreshes the matching `gitlab_task_mrs`
  row (state, pipeline, approval, merge status) in addition to notifying on
  notable transitions.
- GitLab notification subscription state is owned by GitLab. Kandev reads it
  live and does not duplicate it in SQLite.
- `gitlab_task_mr_options` is a per-task row: `task_id` (PK), the genuinely
  task-level fields `review_reviewer_username` and `auto_fix_prompt_override`
  (nullable; empty/`NULL` means use the built-in `mr-auto-fix` prompt),
  `mr_scope_migrated_at` (nullable; guards the one-time fan-out into
  `gitlab_task_mr_automation_options` below so a replay never re-enables a
  switch a user has since turned off for one MR), and timestamps. Its five
  boolean columns (`auto_fix_enabled`, `auto_merge_enabled`,
  `prompt_on_review_requested`, `prompt_on_merged`, `prompt_on_closed`) are
  legacy: no longer written, read only by that one-time migration.
- `gitlab_task_mr_automation_options` is the per-MR source of truth for the
  five automation switches, keyed by `(task_id, repository_id, project_path,
  mr_iid)`. A `PATCH`/MCP update naming one linked MR's identity writes only
  that row; omitting MR identity fans the patch out to every row currently
  linked to the task. The public `GET` response's top-level switch booleans
  stay an aggregate ("on for every linked MR, and at least one MR linked")
  for MCP/API read compatibility; the `mr_options` array in that response is
  the per-MR source of truth the UI renders from.
- `gitlab_task_mr_state` is a per-`(task_id, repository_id, project_path,
  mr_iid)` row carrying lifecycle dedupe fields (from MR lifecycle
  notifications) plus `last_fix_signature`, `last_fix_checkpoint_json`,
  `last_fix_enqueued_at`, `last_fix_session_id`, `auto_fix_round_count`,
  `auto_fix_exhausted_at`, `last_merge_signature`, and
  `last_merge_attempt_at`.
- `gitlab_task_mrs` additionally persists `detailed_merge_status`,
  `reviewer_count`, and `unapproved_reviewers` from every lifecycle sync, and
  `unresolved_discussions` from the automation evaluation pass only (kept out
  of the lifecycle sync's `UPDATE` so a plain poll never resets it to zero for
  an MR that is not automation-subscribed).
- `custom_prompts` includes a built-in `mr-auto-fix` row (`id =
  "builtin-mr-auto-fix"`), seeded the same way as GitHub's `ci-auto-fix`.

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

- `GET /projects/branches?workspace_id=<id>&project=<path>` uses the workspace
  connection when configured. When the workspace is unconfigured and the
  requested host is `gitlab.com`, it may list branches anonymously for the
  explicitly named public project so Remote task creation can continue.
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

### MR automation (lifecycle, auto-fix, auto-merge)

- `GET /tasks/:taskID/mr-automation` returns the task's `TaskMRAutomationOptions`:
  `automation_revision`, the three lifecycle booleans,
  `review_reviewer_username`, `auto_fix_enabled`, `auto_merge_enabled`,
  `auto_fix_prompt_override` (`null` when unset), `auto_fix_max_rounds` (`10`),
  `effective_auto_fix_prompt`, `using_default_prompt`, `updated_at`,
  `mr_options` (one row of the five switches per linked MR), and `mr_states`
  (one `TaskMRLifecycleState` per linked MR, carrying both the lifecycle dedupe
  fields and the auto-fix/auto-merge checkpoint fields). The top-level switch
  booleans are compatibility aggregates: they are true only when at least one
  MR is linked and every linked MR has the switch enabled. Clients that need
  one MR's exact value must read its `mr_options` row.
- `PATCH /tasks/:taskID/mr-automation` accepts a partial body with any of the
  same fields (excluding `auto_fix_max_rounds`, `effective_auto_fix_prompt`,
  `using_default_prompt`, `updated_at`, and `mr_states`, which are
  server-computed). An unknown field returns `400 unknown MR automation
  field`; `auto_fix_enabled`/`auto_merge_enabled`/the three lifecycle booleans
  reject an explicit `null` (they are switches, not clearable values);
  `auto_fix_prompt_override: null` or `""` restores the built-in `mr-auto-fix`
  prompt. To target one linked MR, clients pass the complete
  `repository_id`, `project_path`, and `mr_iid` selector tuple. If all three
  selectors are omitted, the switch patch fans out to every linked MR. A
  partial selector tuple or an unlinked MR returns `400` without a write.
  The prompt override remains task-level and does not use these selectors.
- Current-task MCP exposes `get_task_mr_automation_kandev` and
  `update_task_mr_automation_kandev` with the same shape, scoped to the
  connected task.
- Auto-fix readiness requires the MR to be open, not draft, and its latest
  pipeline settled (not `running`/`pending`); it fires on a new or changed
  failing job, or a new/changed unresolved discussion note versus the stored
  checkpoint, and stops entirely once the MR is `merged`, `closed`, or
  `locked`.
- Auto-merge readiness is GitLab's `detailed_merge_status == "mergeable"`
  (15.6+), falling back to `merge_status == "can_be_merged"` on older hosts,
  **plus** Kandev's own gates: open, not draft, pipeline `success`, and zero
  unresolved discussions. An auto-fix dispatch in the same evaluation pass
  blocks that pass's auto-merge attempt.
- Merge attempts route through the workspace client resolved with the linked
  MR row's own stored `Host` as the expected host; a host mismatch fails
  closed (`ErrWorkspaceHostMismatch`) rather than merging through a
  differently-configured connection.

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
- `503` indicates the workspace connection is absent or currently unavailable,
  except for anonymous branch discovery of an explicitly named public
  `gitlab.com` project.
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
