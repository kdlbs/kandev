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

## Success Criteria

- Linking does not change task state, session history, repositories, or unrelated metadata.
- Invalid issue references and repository mismatches return clear errors.
- Right-click context menu, sidebar menu, and touch/dropdown menu users can reach the Link submenu.
