---
title: "Integrations"
description: "Connect workspace-scoped GitHub, GitLab, Jira, Linear, Sentry, and Slack accounts to Kandev."
---

# Integrations

Integrations bring repository, ticket, incident, and notification context into a Kandev workspace. They do not replace the credentials used by an executor or agent process. A GitHub integration can let Kandev inspect and create pull requests, for example, while the task environment still needs its own Git credentials to fetch or push.

## Configure an integration

Open **Settings > Workspaces**, select the workspace, and open **Integrations**. The same providers also appear under the top-level **Settings > Integrations** route for the active workspace.

1. Choose a provider.
2. Enter its server or organization details and a scoped credential.
3. Save and run the available connection test.
4. Confirm the integration reports healthy before relying on it in a workflow.
5. Create a small test task or notification before enabling an unattended automation.

Integration configuration and secrets are workspace-scoped. Configure each workspace independently when teams or repositories use different accounts.

## Supported providers

| Provider | Kandev uses it for | Configuration notes |
|---|---|---|
| GitHub | Repository discovery, issues, pull requests, reviews, checks, branches, Gist task shares, and GitHub-triggered automations. | Prefer a fine-grained token or app access limited to the required repositories and actions. |
| GitLab | Repository and merge-request context for GitLab-hosted work. | Set the correct GitLab base URL for self-managed instances and scope the token to the projects Kandev needs. |
| Jira | Ticket lookup, project/status discovery, transitions, and issue-watch triggers. | Jira Cloud uses API-token authentication. Personal access tokens are for Jira Server or Data Center. |
| Linear | Issue/team context and watches that can create Kandev work from matching issues. | Verify the selected workspace/team and token permissions before enabling a watch. |
| Sentry | Issue and project context for debugging and task creation flows. | Limit the token to the organizations and projects used by the Kandev workspace. |
| Slack | Workspace communication and Slack-triggered agent runs where configured. | Treat channel access, bot permissions, and any exposed MCP endpoint as part of the deployment trust boundary. |

The settings screen in the running release is authoritative for fields and supported operations. Provider APIs and token formats can change independently of Kandev.

## Repository access versus provider access

There are three distinct credential paths:

- **Kandev integration credentials** let the backend call a provider API.
- **Git credentials in the task environment** let Git fetch and push repository data.
- **Agent credentials** let the selected coding agent call its model provider and any tools it owns.

One does not automatically grant the others. A task can display a pull request while its executor cannot push, or an agent can edit a worktree while Kandev cannot read CI checks. Diagnose the failing path separately.

## Use external context safely

Issue bodies, pull-request comments, commit messages, chat messages, and incident details are untrusted input. If a workflow forwards that content to an agent, it can contain instructions intended to influence the agent.

- Keep write credentials out of read-only triage workflows.
- Review generated branches, comments, transitions, and notifications before broad automation.
- Restrict repository and channel scope instead of using account-wide tokens.
- Do not include secrets in task descriptions or provider comments.
- Use explicit human gates for production changes and sensitive issue queues.

## Troubleshooting

- **Connection test fails:** check the provider base URL, token type, expiration, scopes, and network access from the Kandev backend host.
- **Repository or project is missing:** confirm the authenticated account can see it and that workspace filters do not exclude it.
- **Kandev can read but not write:** add only the specific write scope needed, then repeat the test.
- **Pull request state is stale:** refresh the task and inspect provider health; webhook and polling behavior differs by integration.
- **Task environment cannot push:** fix its Git/SSH credential path. Changing the backend integration token may not affect Git running inside an executor.
- **Automation creates unexpected work:** disable the automation, inspect its provider filters and run history, then narrow repositories, branches, authors, labels, or issue query.

Related: [Tasks and workflows](tasks-and-workflows.md), [Automation and MCP](automation-and-mcp.md), and [Executors](executors.md).
