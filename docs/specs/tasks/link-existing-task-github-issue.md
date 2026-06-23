# Link Existing Task to GitHub Issue

## Why

Users can create a task from a GitHub issue, but they cannot link an issue to a task that already exists. That breaks traceability when a task is started first and the GitHub issue is created later.

## What

- Any existing task can open a **Link GitHub issue** action from the task card menu.
- The action accepts a GitHub issue URL. If the task has exactly one GitHub repository, an issue number such as `#1470` is also accepted.
- The backend fetches the issue through the configured GitHub integration and only links it when the issue belongs to a GitHub repository attached to the task.
- The link is stored in task metadata using the existing `issue_url` and `issue_number` fields, so kanban cards and task detail surfaces render it through the existing issue indicator.
- A linked issue can be explicitly changed or unlinked from the same dialog.

## Out of Scope

- Creating GitHub issues from Kandev.
- Combining GitHub PR and issue linking into a single generic association UI.
- New issue synchronization beyond the existing metadata-backed reference.

## Success Criteria

- Linking does not change task state, session history, repositories, or unrelated metadata.
- Invalid issue references and repository mismatches return clear errors.
- Right-click context menu and touch/dropdown menu users can reach the action.
