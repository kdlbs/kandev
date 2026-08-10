# Forgejo integration

Kandev connects to a Forgejo server per workspace through its REST v1 API.
Configure the server origin and a scoped personal access token at **Settings →
Integrations → Forgejo**.

## Token permissions

Use a token with `read:repository` and `read:issue` for repository discovery,
the issue/PR queue, and task-link refresh. Add `write:repository` only when
users need Kandev to create pull requests, comments, or submitted reviews.

The token is stored in Kandev's secret store. It is not returned by the API,
written into workspace configuration, or injected into agent processes.

## Git credentials are separate

The Forgejo API token can read metadata and create pull requests, but it does
not push a task worktree branch. Configure Git/SSH credentials independently
for the executor or local clone before creating a pull request from a task.

## Current behavior

- Repository, issue, and pull-request discovery is scoped to the workspace
  connection.
- A task can link, refresh, or unlink Forgejo issues and pull requests.
- Kandev can create a pull request from supplied owner/repository/branch data
  and immediately stores its task association.
- The queue lists open issues and pull requests across repositories visible to
  the configured token.
- **Refresh connection** records the latest health result without replacing a
  saved token. A temporary connection failure preserves the configuration.
- Issue watches and review watches can create one Kandev task per matching open
  issue or pull request. They retain their workflow, repository, branch,
  instructions, and optional agent profile; duplicate polls are safe.
- Review-watch settings create a recurring PR-review queue; each open pull
  request is claimed only once. Saved action presets retain reusable review
  instructions per workspace and can be copied into a watch or review action.
- Watch polling runs in the background and can enforce an active-task limit.
  Manual polling remains available for verification and recovery.
- The Forgejo queue can show PR commits, changed-file counts, comments,
  reviews, mergeability, and Forgejo Actions runs for the PR head branch.
  Comments, approvals, and change requests are explicit user actions.

## Optional signed webhooks

For faster watch pickup, set a webhook secret in the Forgejo integration
settings and configure the same secret in Forgejo for Kandev's
`POST /api/v1/forgejo/webhooks?workspace_id=<workspace-id>` endpoint. Kandev
verifies the SHA-256 HMAC signature and records delivery IDs so a replay does
not create duplicate work. Background polling remains the correctness fallback
for unavailable or misconfigured webhooks.

## Boundaries and intentional exclusions

- Kandev uses Forgejo REST v1 directly; it does not shell out to `fj`, `tea`,
  `gh`, or `git` as an API client.
- Forgejo Actions runner administration and bidirectional Kanban/issue-state
  mapping are outside this integration.
- The Forgejo PAT is never shared with local, worktree, Docker, SSH, or
  Sprites agents. Git push credentials stay independently configured.

Kandev's board remains the execution workflow. Forgejo issue state is not
automatically changed when a Kandev task moves; external mutations are explicit
actions.
