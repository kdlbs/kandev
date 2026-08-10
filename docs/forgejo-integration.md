# Forgejo integration

Kandev connects to a Forgejo server per workspace through its REST v1 API.
Configure the server origin and a scoped personal access token at **Settings →
Integrations → Forgejo**.

## Token permissions

Use a token with `read:repository` and `read:issue` for repository discovery,
the issue/PR queue, and task-link refresh. Add `write:repository` only when
users need Kandev to create pull requests.

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

Kandev's board remains the execution workflow. Forgejo issue state is not
automatically changed when a Kandev task moves; external mutations are explicit
actions.
