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
# GitLab Integration System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITLAB-INTEGRATION-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
- Archived tasks retain task-to-MR associations for later unarchive; hard task
  deletion removes task-owned associations and refresh watches.
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
- **GIVEN** a workspace without a GitLab connection, **WHEN** the user enters a
  public `gitlab.com` repository URL in Remote task creation, **THEN** Kandev
  lists its branches anonymously without making any other GitLab browse or
  write capability available.
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
- **GIVEN** a linked MR with auto-fix enabled, an open state, a settled
  failing pipeline, and a promptable session, **WHEN** automation evaluates
  the MR, **THEN** it sends or queues exactly one `@mr-auto-fix` prompt and
  `auto_fix_round_count` increments by one; an unchanged snapshot on the next
  evaluation dispatches nothing.
- **GIVEN** a linked MR with auto-merge enabled and every readiness gate
  satisfied, **WHEN** automation evaluates the MR, **THEN** Kandev merges it
  exactly once, routed through the workspace client resolved with the linked
  row's own host; a host mismatch never merges.
- **GIVEN** a linked MR reaches `merged`, `closed`, or `locked`, **WHEN**
  auto-fix is still enabled, **THEN** no further auto-fix prompt is
  dispatched for that MR.
- **GIVEN** a task with exactly one linked MR on desktop, **WHEN** the user
  hovers the topbar button past the open delay, **THEN** a preview popover
  shows the pass-rate bar, approval row, unresolved-discussions row,
  Automation controls, a merge action when ready, and link/open/unlink
  actions, all without a click.
- **GIVEN** a task with exactly one linked MR on desktop, **WHEN** the user
  clicks the topbar button, **THEN** the MR detail panel opens directly (no
  dropdown) and any open hover popover closes.
- **GIVEN** a task with two or more linked MRs, or any touch/coarse-pointer
  viewport regardless of MR count, **WHEN** the user interacts with the
  topbar button, **THEN** no hover popover is ever rendered and
  clicking/tapping opens the existing per-MR dropdown.
- **GIVEN** a task with two linked MRs, one merged and one open with a failing
  pipeline, **WHEN** the Kanban card renders its MR badge, **THEN** the badge
  shows a count of two and the open MR's (red) colour, not the merged MR's.
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
- A multi-MR aggregate hover preview (the popover only covers the single-MR
  topbar case; a task with 2+ linked MRs keeps the existing click-only
  dropdown).
- GitLab's per-reviewer `requested_changes` states as a three-way review
  label; "awaiting review" is approvals-count-based only (version/tier
  dependent otherwise).
- Per-job CI log tailing in the auto-fix prompt.
