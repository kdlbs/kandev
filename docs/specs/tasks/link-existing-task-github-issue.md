---
status: building
created: 2026-06-23
owner: product
---

# Link Existing Task to External References

## Why

Users can create tasks from external systems, but they also need to attach
external references to a task that already exists. Traceability breaks when a
task starts first and the GitHub pull request, GitHub issue, Jira ticket, Linear
issue, or Sentry issue is identified later.

## What

- Any existing task can open a **Link** submenu from task menus.
- The submenu contains **GitHub Pull Request**, **GitHub Issue**, **Jira
  Ticket**, **Linear Issue**, and **Sentry Issue** actions when each action is
  available for the task workspace.
- The actions are available from task card context/dropdown menus and sidebar task context menus.
- The issue action accepts a GitHub issue URL. If the task has exactly one GitHub repository, an issue number such as `#1470` is also accepted.
- The pull request action accepts a GitHub PR URL. If the task has exactly one GitHub repository, a PR number such as `#1471` is also accepted.
- The backend fetches the issue through the configured GitHub integration and only links it when the issue belongs to a GitHub repository attached to the task.
- The link is stored in task metadata using the existing `issue_url` and `issue_number` fields, so kanban cards and task detail surfaces render it through the existing issue indicator.
- Creating a task from a GitHub issue on the GitHub integration page automatically applies the same metadata-backed link after task creation.
- The GitHub issues list resolves all metadata-backed issue links in the active workspace and shows the linked task title with navigation to that task. Links created manually, by the GitHub issue quick launcher, and by issue watches use the same indicator.
- Pull request linking reuses the existing task PR association model and rendering.
- A linked issue can be explicitly changed or unlinked from the same dialog.
- Jira ticket linking is shown only when Jira is enabled and healthy for the
  current workspace. It accepts a Jira key or URL, validates it through the
  configured Jira integration, and links by rewriting the task title prefix to
  `KEY: existing title`.
- Linear issue linking is shown only when Linear is enabled and healthy for the
  current workspace. It accepts a Linear identifier or URL, validates it through
  the configured Linear integration, and links by rewriting the task title
  prefix to `IDENTIFIER: existing title`.
- Sentry issue linking is shown only when Sentry is enabled and healthy for the
  current workspace. It accepts a Sentry short ID or URL, validates it through
  the configured Sentry integration, and links by rewriting the task title
  prefix to `SHORT-ID: existing title`.
- Jira, Linear, and Sentry title-prefix linking replaces an existing leading
  external issue prefix instead of stacking prefixes.
- The dockview top bar does not show Jira or Linear create-link buttons for
  unlinked tasks. It still shows existing linked-reference affordances, such as
  Jira/Linear ticket buttons when a linked key is present in the title.

## Out of Scope

- Creating GitHub issues from Kandev.
- Creating GitHub pull requests from Kandev.
- New issue synchronization beyond the existing metadata-backed reference.
- Creating Jira tickets, Linear issues, or Sentry issues from Kandev.
- A durable cross-provider `task_external_links` model.
- Changing issue-watch reservation and deduplication semantics.
- A Sentry linked-issue top-bar affordance.

## Scenarios

### Link a GitHub issue to an existing task

GIVEN an existing task with a GitHub repository attached
WHEN the user opens Link > GitHub Issue and enters an issue URL from that repository
THEN Kandev stores the issue URL and issue number on the task without changing task state, session history, repositories, or unrelated metadata

### Reject an issue from a different repository

GIVEN an existing task with a GitHub repository attached
WHEN the user attempts to link an issue from another GitHub repository
THEN Kandev rejects the request with a clear repository mismatch error and leaves task metadata unchanged

### Link a pull request to an existing task

GIVEN an existing task with a GitHub repository attached
WHEN the user opens Link > GitHub Pull Request and enters a pull request URL from that repository
THEN Kandev creates the task pull request association using the attached repository ID so existing PR rendering surfaces can show the link

### Infer a reference number for a single-repository task

GIVEN an existing task with exactly one GitHub repository attached
WHEN the user enters a bare number or hash-prefixed number in the GitHub Issue or GitHub Pull Request dialog
THEN Kandev resolves the number against that single repository before linking the reference

### Unlink an existing issue

GIVEN an existing task that already has GitHub issue metadata
WHEN the user opens Link > GitHub Issue and chooses Unlink
THEN Kandev removes only the issue metadata keys and preserves unrelated task metadata

### Create and link a task from the GitHub issues list

GIVEN a GitHub issue shown on the GitHub integration page
WHEN the user creates a task with an issue action preset
THEN Kandev creates the task, stores that issue as the task's metadata-backed GitHub issue link, and preserves normal task navigation

### Show tasks linked to an issue

GIVEN one or more active-workspace tasks reference the same GitHub issue through task metadata
WHEN the GitHub issues list renders that issue
THEN the issue row shows the linked task title for one task or a task-count menu for multiple tasks, and each entry navigates to its task

### Include issue-watch tasks in the issue indicator

GIVEN an issue watch created a task with `issue_url`, `issue_number`, and `issue_repo` metadata
WHEN the matching issue appears on the GitHub issues list
THEN the issue row shows that task through the same indicator as a manually linked task

### Keep unlinked issues unchanged

GIVEN no task in the active workspace references a GitHub issue
WHEN the GitHub issues list renders that issue
THEN no task indicator is shown and the existing issue actions remain available

## Data And API

- Task metadata remains the canonical GitHub issue association. Manual links include `issue_url`, `issue_number`, `issue_owner`, `issue_repo`, and `github_issue_linked`; issue-watch tasks may use the legacy `issue_repo: owner/repo` shape.
- Each task links to at most one GitHub issue, while one GitHub issue may link to multiple tasks.
- `GET /api/v1/github/task-issues?workspace_id=<id>` returns links grouped by task ID for the requested workspace. The lookup derives owner, repository, and issue number from the canonical GitHub issue URL and never returns links from another workspace.
- The workspace lookup uses the indexed task workspace boundary and does not depend on the GitHub token being currently configured because it reads persisted task metadata.

## Failure Modes

- A failed automatic link attempt does not roll back a successfully created task or block navigation to it.
- Invalid or incomplete GitHub issue metadata is ignored by the workspace reverse lookup instead of producing a misleading issue-row association.
- Existing link validation continues to reject issues from repositories not attached to the task.

### Link a Jira ticket to an existing task

GIVEN Jira is enabled and healthy for the task workspace
WHEN the user opens Link > Jira Ticket and enters `PROJ-12`
THEN Kandev validates the ticket through Jira and renames the task to `PROJ-12: <old title>` without changing task state, session history, repositories, or unrelated metadata

### Link a Linear issue to an existing task

GIVEN Linear is enabled and healthy for the task workspace
WHEN the user opens Link > Linear Issue and enters `ENG-20`
THEN Kandev validates the issue through Linear and renames the task to `ENG-20: <old title>` without changing task state, session history, repositories, or unrelated metadata

### Link a Sentry issue to an existing task

GIVEN Sentry is enabled and healthy for the task workspace
WHEN the user opens Link > Sentry Issue and enters `API-99`
THEN Kandev validates the issue through Sentry and renames the task to `API-99: <old title>` without changing task state, session history, repositories, or unrelated metadata

### Replace an existing external prefix

GIVEN an existing task titled `PROJ-12: Fix login`
WHEN the user opens Link > Linear Issue and enters `ENG-20`
THEN Kandev renames the task to `ENG-20: Fix login` instead of stacking prefixes

## Success Criteria

- Linking does not change task state, session history, repositories, or unrelated metadata.
- Invalid issue references and repository mismatches return clear errors.
- Right-click context menu, sidebar menu, and touch/dropdown menu users can reach the Link submenu.
- Jira, Linear, and Sentry actions are hidden when the corresponding integration is disabled or unauthenticated.
- GitHub issue links are visible only in their task workspace, including after page reload.
