---
title: "Integrations"
description: "Connect Azure DevOps, GitHub, GitLab, Jira, Linear, Sentry, and Slack, then browse external work or create watched tasks."
---

# Integrations

Integrations let Kandev's backend read and update provider data. They power repository and issue browsers, task associations, watches, pull-request review, and provider-specific task launchers.

They do **not** provide every credential a task needs. Keep these paths distinct:

- an integration credential lets the Kandev backend call a provider API;
- Git or SSH credentials in an executor let the task fetch and push a repository;
- an agent login or API key lets the coding CLI call its model provider.

GitHub is the important exception. For Local Docker, Sprites, and SSH launches (and the registered Remote Docker path, although that runtime is not implemented), Kandev resolves profile remote-auth secrets and selected remote credentials first. A resulting `GITHUB_TOKEN` or `GH_TOKEN` wins. Otherwise Kandev injects the globally stored `GITHUB_TOKEN`/`github_token` secret as both variables, with automatic extraction from the local `gh` login as the final fallback. The workspace repository list does **not** restrict what that injected token can access. Local and Worktree sessions continue to use credentials available on the host.

A task can therefore display a pull request while a host worktree cannot push, or edit a repository while Kandev cannot read checks. Diagnose the failing credential path separately and treat a GitHub integration token as a credential that remote task agents may receive.

## Open integration settings

Select **Settings > Workspaces > _Workspace_ > Integrations**, then choose a provider. The direct routes are:

- `/settings/workspace/{workspaceId}/integrations/github`
- `/settings/workspace/{workspaceId}/integrations/gitlab`
- `/settings/workspace/{workspaceId}/integrations/azure-devops`
- `/settings/workspace/{workspaceId}/integrations/jira`
- `/settings/workspace/{workspaceId}/integrations/linear`
- `/settings/workspace/{workspaceId}/integrations/sentry`
- `/settings/workspace/{workspaceId}/integrations/slack`

Compatibility routes under **Settings > Integrations** use the active workspace where the provider has workspace settings.

GitHub authentication is installation-wide and the current integration targets `github.com`. GitLab authentication and its selected host are also installation-wide. Azure DevOps, Jira, Linear, and Slack configuration is workspace-specific. Sentry supports multiple named instances per workspace. Do not assume that configuring one workspace gives another the same provider scope.

Provider secrets saved by these forms use Kandev's encrypted secret store. The backend must still decrypt them to make API requests. Limit access to settings and the Kandev data directory, and use the narrowest provider scope that works.

### The Enabled switch

Jira, Linear, Sentry, and Slack pages show an **Enabled** switch. It is a browser-local preference, saved per installation in that browser and on by default. It controls some client-side entry points, availability checks, and configuration fetches; settings pages can still poll provider health. It does not delete backend configuration and does not stop a server-side watch or Slack poller. Pause/delete watches or remove the provider configuration when processing must stop.

Health results are cached and periodically refreshed (normally about every 90 seconds in the settings UI). Use **Test connection** after changing a URL or credential rather than waiting for the next probe.

## GitHub

Use GitHub for pull requests, issues, reviews, checks, repository discovery, task associations, and provider-triggered work. Browse it at `/github` after connecting an account.

### Authenticate

Open the workspace GitHub settings. Authentication is resolved installation-wide in this order:

1. an authenticated `gh` CLI;
2. `GITHUB_TOKEN` in the backend environment;
3. `GH_TOKEN` in the backend environment;
4. a stored secret named `GITHUB_TOKEN` or `github_token`;
5. no authenticated client.

If `gh` is installed, use the host terminal action and run `gh auth login` as Kandev's service user. The token form validates the GitHub API before saving an encrypted token; the UI calls out `repo` and `read:org` scopes for a classic personal access token. Scope any token to only the organizations, repositories, and write operations that Kandev needs.

The status panel reports the authenticated user, selected method, rate-limit information, and diagnostics. Clearing Kandev's stored token does not remove a valid CLI or environment credential, and a valid `gh` login continues to take precedence over a stored token.

### Configure and use the workspace

Workspace GitHub settings control repository scope, default/saved searches, quick-action prompts, pull-request analytics, review watches, and issue watches. At `/github`, search or browse pull requests and issues, save queries, apply prompt presets, and launch a Kandev task. A saved query can default to one repository; choose **All repos** for no repository default, and change the repository filter without rewriting the saved query. An associated pull request also appears in task review surfaces for feedback, checks, reviews, and merge actions.

A **Review Watch** polls a GitHub search and creates review work. It requires a workflow, starting step, prompt, and workspace. The default query is `type:pr state:open review-requested:@me -is:draft`; add repository filters or replace the query as needed. An optional agent or executor profile overrides the selected step's defaults. The poll interval defaults to 300 seconds and accepts 60–3,600 seconds.

An **Issue Watch** behaves similarly for issues. Its default search is `type:issue state:open`. Choose labels or provide a custom GitHub query; the custom query takes precedence over label selection.

Both watch types default to the **Auto** cleanup policy: delete merged/closed tasks only when the user has not typed a message. **Always** deletes even after user engagement; **Never** retains every task. You can pause a watch, poll immediately, or clean completed work. Deleting a GitHub review or issue watch best-effort cascade-deletes the tasks it owns. **Reset** is also destructive: after its preview, it permanently cascade-deletes every watch-created task, including archived tasks, and clears cursor/deduplication state so current matches become eligible again. Review-watch reset schedules a re-import; issue-watch reset re-imports on its next poll. Reset is not a way to keep old tasks and rerun a query.

Repository scope and watch filters are workspace-specific even though the GitHub login is global. They constrain backend browsing and watch queries, not the permissions of a token injected into a remote executor. GitHub workspace configuration can be copied, but credentials and watches are deliberately not copied.

## GitLab

Use `/gitlab` to browse/search merge requests and issues and follow links to GitLab. The current public page does **not** launch Kandev tasks. For an already associated merge request, the task top bar can show an external link and aggregate state, but Kandev does not currently expose GitLab discussions, reply/resolve controls, full review feedback, or pipeline review actions.

The selected GitLab host and authentication are installation-wide, even though the page is reachable from workspace settings. `https://gitlab.com` is the default. For a self-managed instance, enter an HTTP or HTTPS base URL that the Kandev backend can reach. One Kandev installation can select only one GitLab host at a time; it cannot simultaneously browse `gitlab.com` and a self-managed host.

Authentication is resolved in this order:

1. an authenticated `glab` CLI for the selected host;
2. `GITLAB_TOKEN` in the backend environment;
3. a stored secret named `GITLAB_TOKEN` or `gitlab_token`;
4. no authenticated client.

The token form validates the `/user` API before saving. The UI requires a token with `api` and `read_user`; Kandev does not perform an authorization grant or request those scopes for you. GitLab's `api` scope is broad and write-capable, so use a dedicated, minimally privileged account where possible. Diagnostics distinguish a host that cannot be reached from a missing or rejected credential. Clearing the stored token does not sign out `glab` or remove an environment token.

At `/gitlab`, use built-in searches or per-user saved searches. The project picker only filters the current client-side result set (up to 25 items); it is not a provider permission boundary or a project-scoped server query. Open an item in GitLab for provider-side actions.

Current public GitLab settings expose the connection only. Although internal services contain watcher and preset data types, there is no current end-user settings workflow for GitLab watches or prompt presets. Do not rely on those features until they appear in the UI.

Unlike GitHub, Kandev does not automatically inject the stored GitLab integration token into task executors. Configure the executor's Git credentials separately when a task must fetch from or push to GitLab.

## Azure DevOps

Azure DevOps configuration is workspace-specific. The current integration supports Azure DevOps Services organizations at a canonical `https://dev.azure.com/<organization>` URL. It does not support Azure DevOps Server/TFS or alternate organization URL forms.

Create a personal access token with **Work Items (Read)** and **Code (Read)** access, then enter it on the Azure DevOps settings page. Kandev stores the PAT in its encrypted secret store and calls Azure DevOps REST API 7.1 directly. The connection, work-item, and pull-request paths do not require GitHub, `gh`, `az`, or Azure CLI authentication. When editing a saved connection, a blank PAT preserves that workspace's existing credential. Copy configuration transfers the encrypted credential to the target workspace.

Use `/azure-devops` to run WIQL work-item queries or browse pull requests by project, repository, status, creator, and reviewer. Pull-request feedback includes reviewers and votes, comment threads, linked work items, and branch-policy results. Provider content is read-only in this release: Kandev does not edit work items, vote, comment, complete pull requests, or change policies.

You can launch a task from a work item or pull request. When the selected Kandev repository is configured with matching Azure project and repository identifiers, launching from a pull request also stores a durable task association. Task surfaces show its normalized status, review, and policy summary while Azure-native feedback remains in the Azure DevOps browser. Synchronization uses the backend REST client and does not depend on tools installed in the task environment.

This release has no Entra OAuth flow, webhook, or watch poller. Configure executor Git credentials independently when a launched task must fetch from or push to an Azure Repos repository; the integration PAT is not injected into task environments.

## Jira

Jira configuration is workspace-specific. Use `/jira` to search with JQL, save views, open issue details, run supported transitions, and launch tasks with Jira prompt presets. Launch copies Jira URL/content into the task title and description; it does not store a durable Jira issue association on the task.

Enter the site URL (a missing scheme is normalized to HTTPS), choose **Cloud** or **Server/Data Center**, and optionally set a default project key. Authentication options are:

| Deployment | Method | Required values |
|---|---|---|
| Jira Cloud | API token (recommended) | Atlassian account email and API token. |
| Jira Cloud | Browser session | Only the value of the `cloud.session.token` or `tenant.session.token` cookie. Do not include the cookie name or `=`. |
| Server/Data Center | Personal access token | Bearer personal access token with the required read/write access. |

Cloud API tokens are not accepted for Server/Data Center, and Server/Data Center PATs are not the Cloud token flow. Browser-session JWTs expire and are less reliable than an API token; Kandev surfaces the decoded expiry and warns as it approaches.

When editing, a blank secret preserves the saved credential only if the URL, account identity, and authentication method still match. Supply a new secret when changing those identity fields. Save, select **Test connection**, and check the background health result.

### Jira issue watches

Create a watch with JQL, test the query, then choose a workflow and starting step. A new watch starts with `project = PROJ AND status = "Open" ORDER BY created DESC`; replace `PROJ` before testing. Repository selection is optional: leaving it blank creates repo-less tasks. When a repository is selected, a blank branch resolves to that repository's default branch. Blank agent and executor profile fields inherit the starting step's defaults. Customize the task prompt and set a poll interval, which defaults to 300 seconds and accepts 60–3,600 seconds.

The maximum in-flight value defaults to 5. Leave it blank for no cap. A cap defers remaining matches rather than importing them all at once. Each poll fetches only the first 50 JQL matches and does not paginate. Already-seen issues still occupy that provider result window, so a stable broad query can leave later matches unseen indefinitely; narrow the JQL enough that every important issue can enter the first page. Pause the watch before changing a broad query. Jira task-preset prompts can use ticket key, URL, title, and description placeholders from the preset editor.

Deleting a Jira watch leaves its previously created tasks in place. **Reset** is destructive: after the preview, it permanently deletes every watch-created task, including archived tasks, clears cursor/deduplication state, and makes current matches eligible for the next poll.

## Linear

Linear configuration is workspace-specific. Enter a personal API key and optionally a default team. Kandev calls the fixed Linear GraphQL endpoint at `https://api.linear.app/graphql` and sends the key as its authorization value. Leaving the credential blank during an edit keeps the stored key.

After saving and testing the connection, use `/linear` to search by text, team, or assignee, view issue details, change supported states, and launch tasks. Linear launch uses fixed title/description construction, has no prompt-preset editor, and does not store a durable Linear issue association.

Linear watches can filter by team, states, labels, priorities, assignee, creator, estimate range, and free-text query. At least one of those filters is required. They also define dispatch order, workflow and starting step, optional repository/base branch/profile overrides, prompt, poll interval, and a maximum in-flight count. New watches default to five in-flight tasks and **Priority (high → low)** dispatch. The poll interval defaults to 300 seconds and accepts 60–3,600 seconds; clear the in-flight field for no cap.

Leaving the repository blank creates repo-less tasks. When a repository is selected, a blank branch resolves to its default branch. Test narrow filters before enabling the watch. Deleting a Linear watch retains existing tasks; **Reset** permanently deletes every watch-created task, including archived tasks, clears cursor/deduplication state, and makes current matches eligible for the next poll.

Linear polling is also bounded. **Default (Linear order)** reads one page of 50; an explicit dispatch sort reads at most five pages of 50 before sorting locally. Matches outside that window can remain unseen, and reset does not bypass the bound.

## Sentry

Sentry configuration is workspace-specific and supports multiple named instances. This is useful when one Kandev workspace spans different Sentry organizations or self-hosted installations.

Create an instance with a unique name, base URL, and bearer authentication token. The default URL is `https://sentry.io`; replace it for self-hosted Sentry. A URL with no scheme becomes HTTPS. It must be a bare HTTP(S) host root—paths, queries, and fragments are rejected. The UI lists `org:read`, `project:read`, and `event:read` as the required read scopes.

On any saved edit, a blank token preserves the existing token, including when the URL changes. The pre-save **Test connection** candidate cannot reuse that stored token after a URL change, so paste the token to test the new URL before saving.

A Sentry watch binds to one instance, organization, and project; the selected instance is immutable after creation. It can filter environment, level, one status, and a free-text Sentry query, then select a workflow/step, optional repository/base/profile overrides, prompt, poll interval, and maximum in-flight count. New watches default to `fatal` and `error` levels, `unresolved` status, a 24-hour stats period, five in-flight tasks, and a 300-second poll interval. The interval accepts 60–3,600 seconds; clear the in-flight field for no cap. Although the UI currently permits selecting several statuses, the backend rejects save with more than one because Sentry has no OR form for `is:`. Passthrough agent profiles are not offered to watches.

Leaving the repository blank creates repo-less tasks. With a selected repository, a blank branch resolves to its default branch. Deleting a Sentry watch retains its existing tasks; **Reset** permanently deletes every watch-created task, including archived tasks, clears cursor/deduplication state, and makes current matches eligible for the next poll.

Each Sentry poll reads only the newest first page (up to 100 issues) and does not paginate. Older matches can remain unseen while newer/seen issues occupy that page; reset does not force a complete backlog import.

An instance cannot be deleted while a watch references it. Because the instance binding is immutable, delete those watches first and recreate them against another instance if needed. Sentry issues appear in task issue-selection/current-task surfaces; there is no top-level `/sentry` browser comparable to GitHub, GitLab, Jira, or Linear.

## Slack

Slack support currently uses a browser-session polling connection. It is intended for a controlled personal workspace and is more fragile than OAuth or a bot installation. Kandev does not currently offer a Slack OAuth/bot install flow.

Configure, per workspace:

- an `xoxc-...` browser session token;
- only the value of the Slack `d` cookie;
- a **Utility Agent** from **Settings > Utility agents**;
- a command prefix, default `!kandev`;
- a polling interval, default 30 seconds and allowed range 5–600 seconds.

The workspace owns this configuration record, but it does not hard-pin the destination of a created task. The built-in triage prompt deliberately lists every Kandev workspace and asks the agent to choose one. Separate workspace configurations keep separate polling cursors; reusing the same Slack account and prefix in several configurations can therefore process the same authored message more than once.

With a saved configuration whose latest authentication health check succeeded, Kandev polls messages visible to the connected Slack user. The browser-local **Enabled** preference is not part of this backend gate. A message authored by that same Slack user and beginning with the prefix can trigger in a channel or direct message.

On the first successful scan the watermark is empty. Slack search returns the newest 30 matching messages, and Kandev processes those matches oldest-first; enabling the integration can therefore act on up to 30 messages that already existed. Use a unique prefix, remove or edit old matching messages, or be ready to remove unintended tasks before first configuration.

For each match, Kandev best-effort adds an eyes reaction, fetches the surrounding thread, and gives the request, thread, and external Kandev MCP endpoint to the selected utility agent. It then best-effort posts the agent's final response in-thread. Reaction failure does not stop task creation. Reply failure still advances the watermark, so a task can exist without a Slack reply. A thread-fetch or agent-run failure stops that scan's batch and retries the failed and later matches on a future scan.

This is external/configuration MCP, not a task-scoped MCP session: the endpoint also exposes destructive task and configuration tools. Use a constrained utility agent and model, treat matching Slack text as untrusted input, and review the [external MCP security boundary](automation-and-mcp.md#external-mcp-security-boundary).

Slack has no separate prompt editor. It uses the chosen Utility Agent's prompt from **Settings > Utility agents**, which can reference `{{SlackInstruction}}`, `{{SlackThread}}`, `{{SlackPermalink}}`, `{{SlackUser}}`, `{{SlackChannelID}}`, and `{{SlackTS}}`. If the raw utility prompt contains any Slack-specific placeholder, its resolved value is the complete prompt. Otherwise Kandev uses the resolved utility prompt as the system text and appends Slack context; the built-in triage instructions are used only when that resolved prompt is blank.

Slack does not trigger on reactions, expose a slash command/shortcut, mirror task status, or provide a live chat bridge to a running coding agent. It searches matching messages rather than performing a one-time history import, which is why the first scan can process existing matches. Browser session credentials can expire without notice; reconnect when polling starts returning authentication failures. Turning off the browser-local **Enabled** switch does not stop the backend poller—remove the saved Slack configuration to stop it.

## Copy configuration between workspaces

Supported integration pages offer **Copy configuration** with provider-specific behavior:

- GitHub copies repository scope, saved/default searches, and quick-action presets. It does not copy authentication or watches.
- Azure DevOps, Jira, Linear, and Slack copy the workspace configuration and encrypted credential, replacing the target's provider configuration and re-running health checks. They do not copy watches.
- Sentry adds copies of the source instances with new IDs and copied secrets, preserves target instances, and deduplicates conflicting names. It does not copy watches.
- GitLab host and authentication are already global, so there is no workspace copy action.

Workspace automations are never copied by this action. Review the target workspace's repository and workflow scope before enabling any copied connection.

## Security and troubleshooting

Issue bodies, pull-request comments, commit messages, Slack threads, and incident details are untrusted prompt input. Use read-only credentials for triage, restrict repositories/projects/channels, and keep a human workflow gate before merge, release, deployment, or sensitive transitions.

- **Connection test fails:** verify the base URL, deployment type, token format, expiration, scopes, and network/DNS access from the backend host.
- **Cleared token but connection remains:** a higher-priority CLI or environment credential is still active for GitHub or GitLab.
- **Repository, project, or team is missing:** confirm the connected identity can see it and check workspace filters/defaults.
- **Kandev can read but cannot write:** add only the specific provider write scope needed, then repeat the test.
- **Task cannot fetch or push:** fix Git/SSH credentials in the executor. Azure DevOps, GitLab, Jira, Linear, Sentry, and Slack integration credentials are not task Git credentials. GitHub remote launches are the exception: inspect the profile's `GITHUB_TOKEN`/`GH_TOKEN`, then the globally stored GitHub token or local `gh` fallback that Kandev may have injected.
- **A watch still runs after disabling the provider:** the Enabled switch is browser-local. Pause/delete the watch, or remove the backend configuration.
- **Unexpected work is created:** pause the watch or automation, inspect its query, last-polled/status fields, and created-task list, then narrow provider filters before resetting or polling again. Watch tables do not provide a separate run/import history.

Related: [Tasks and workflows](tasks-and-workflows.md), [Sessions and review](sessions-and-review.md), and [Automation and MCP](automation-and-mcp.md).
