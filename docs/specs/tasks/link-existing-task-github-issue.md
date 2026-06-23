---
status: building
created: 2026-06-23
owner: product
---

# Link Existing Task to GitHub References

## Why

Users can create a task from GitHub, but they also need to attach GitHub references to a task that already exists. That breaks traceability when a task is started first and the GitHub pull request or issue is created later.

## What

- Any existing task can open a **Link** submenu from task menus.
- The submenu contains **GitHub Pull Request** and **GitHub Issue** actions.
- The actions are available from task card context/dropdown menus and sidebar task context menus.
- The issue action accepts a GitHub issue URL. If the task has exactly one GitHub repository, an issue number such as `#1470` is also accepted.
- The pull request action accepts a GitHub PR URL. If the task has exactly one GitHub repository, a PR number such as `#1471` is also accepted.
- The backend fetches the issue through the configured GitHub integration and only links it when the issue belongs to a GitHub repository attached to the task.
- The link is stored in task metadata using the existing `issue_url` and `issue_number` fields, so kanban cards and task detail surfaces render it through the existing issue indicator.
- Pull request linking reuses the existing task PR association model and rendering.
- A linked issue can be explicitly changed or unlinked from the same dialog.

## Out of Scope

- Creating GitHub issues from Kandev.
- Creating GitHub pull requests from Kandev.
- New issue synchronization beyond the existing metadata-backed reference.

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

## Success Criteria

- Linking does not change task state, session history, repositories, or unrelated metadata.
- Invalid issue references and repository mismatches return clear errors.
- Right-click context menu, sidebar menu, and touch/dropdown menu users can reach the Link submenu.
