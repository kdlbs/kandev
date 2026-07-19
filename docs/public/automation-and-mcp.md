---
title: "Automation and MCP"
description: "Create scheduled, GitHub, or webhook automations and connect agents through task, profile, and external MCP."
---

# Automation and MCP

Kandev has several mechanisms that can act without repeated manual setup. Their scopes and trust boundaries differ:

| Mechanism | Purpose |
|---|---|
| Workflow events and actions | React to an existing task entering a step, receiving a message, or completing an agent turn. |
| Workspace automations | Create task-backed work from a schedule, GitHub pull request, webhook, or manual trigger. |
| Task MCP | Give an active Kandev session task, plan, conversation, and coordination tools. |
| Profile MCP | Add third-party MCP servers to one agent profile, subject to executor policy. |
| External MCP | Let a client outside a task configure Kandev and create or manage work through the backend. |

Use workflow events for predictable transitions on existing work. Use a workspace automation when an external signal must create new work. MCP is a tool interface, not a scheduler.

## Workflow events and human gates

Regular workflow entry actions can enable plan mode, reset agent context, or auto-start an agent; auto-start can use the step prompt or a stored prompt override. Turn-start and turn-complete events can move the task, while turn-complete and step-exit actions can disable plan mode. There is no regular standalone **stop agent** or **send prompt** workflow action. Approval/review steps and steps without automatic start remain the supported human gates. Inspect events on both the source and destination step before enabling a move or automatic start; otherwise two steps can form a loop.

See [Tasks and workflows](tasks-and-workflows.md) for event configuration and defaults.

## Create a workspace automation

Open **Settings > Workspaces > _Workspace_ > Automations** (`/settings/workspace/{workspaceId}/automations`) and select **New Automation**. The top-level `/settings/automations` route redirects to, or asks you to select, a workspace.

1. Enter a required name and optional description.
2. Choose **Task** or **Run** execution mode.
3. Select an agent profile and a non-local executor profile. Passthrough agent profiles are not offered.
4. For Task mode, select a workflow and starting step.
5. Select a registered repository, a discovered local repository, or **None**. A discovered repository is registered in the workspace when the automation is saved.
6. Enter a prompt and optional task-title template.
7. Keep the default maximum concurrency of 1 unless parallel work is safe.
8. Choose a schedule and optional GitHub condition, or switch to webhook mode.
9. Save, trigger a small test, then inspect **Runs** before widening credentials or scope.

The form can save an empty agent, executor, or repository selection, but launch still needs a usable agent/executor and a repository. For scheduled, webhook, and manual work, an empty repository falls back to the workspace's first repository. If the workspace has none, the run fails with `no repository available — add a repository to the workspace`. A GitHub pull-request run instead checks out that PR's head branch and uses its base branch.

### Task mode

Task is the default. It creates a visible kanban task in the chosen workflow step. The task starts automatically only when that step has an enabled `auto_start_agent` action on entry; otherwise it waits for a person to start it or move it through the workflow.

Use Task mode for work that needs discussion, permission responses, review, or approval before merge/release. The task remains available like other tracked work.

### Run mode

Run creates an ephemeral task hidden from the kanban, starts it immediately, and exposes the result through automation run history. After the turn finishes, Kandev reaps an associated Worktree executor's worktree/branch when a worktree manager and runtime worktree ID are present. Docker, Sprites, and SSH runs do not have that Worktree cleanup record and follow their own runtime lifecycle. Workflow and step selections do not apply to this mode.

Run mode cannot wait for a permission response. Kandev rejects the request and marks the run failed. Use only a profile whose intended, constrained actions can complete without a prompt. Prefer Task mode when human approval is required.

## Trigger behavior

The editor has two exclusive layouts:

- **Scheduled**: one schedule plus at most one GitHub condition;
- **Webhook**: one authenticated webhook trigger. Switching to webhook deletes the schedule and condition, and switching back deletes the webhook trigger.

In the current backend, the schedule and GitHub PR condition are independent triggers. A non-empty schedule creates generic scheduled runs, while the PR trigger separately polls GitHub. Adding a PR condition does not constrain the scheduled run. Clear the schedule expression if the automation should run only for matching PRs.

### Schedule

The scheduler checks every 30 seconds and treats expressions as elapsed intervals, not calendar cron times. If a new enabled schedule has never been evaluated, its first check fires immediately; later runs are spaced from the last evaluation.

<DocsVideo
  webm="./media/feature-guides/scheduled-workflow-automation.webm"
  mp4="./media/feature-guides/scheduled-workflow-automation.mp4"
  poster="./media/feature-guides/scheduled-workflow-automation.webp"
  title="Schedule workflow automation"
  caption="A workflow automation is configured with a schedule and saved for recurring execution."
/>

Use the supplied presets: every 5, 15, or 30 minutes; hourly; every 6 hours; daily; or weekly. The backend also accepts `@every` followed by a Go duration, `@hourly`, `@daily`, `@weekly`, and step forms such as `*/10 * * * *` or `0 */6 * * *`.

The editor currently accepts arbitrary five-field cron text, but the backend cannot execute fixed calendar forms such as `30 8 * * *`. A saved timezone defaults to UTC but is not used in schedule evaluation. Until calendar scheduling is implemented, use a preset or `@every` and interpret it as an interval. Scheduled runs are deduplicated per trigger per minute.

### GitHub pull requests

The GitHub evaluator polls every 60 seconds and requires a working GitHub integration. It searches open PRs and supports:

- an explicit list of repositories;
- base-branch glob filters, including `*` and a trailing wildcard such as `release/*`;
- exact author-login filters;
- draft exclusion.

Select at least one repository. Although the UI offers **All repos**, an empty repository list is not evaluated, so it produces no PR runs. The editor exposes only the **Opened** event, but the evaluator currently ignores the saved event list: clearing that checkbox does not stop polling or firing. Disable the automation/trigger instead. The first evaluation considers every currently open matching PR rather than only PRs opened after the automation was enabled. Each matching PR is then deduplicated once per automation by repository and PR number.

The current evaluator does not apply label filters, and the current form does not offer them. GitHub push and generic CI-result conditions are shown as **Coming soon**; their backend evaluators do not create runs. Task-specific PR check remediation is a separate review feature, not the generic CI automation trigger.

### Webhook

After creating a webhook automation, copy its URL and one-time displayed secret. The edit page can reveal the secret again.

Send:

```http
POST /api/v1/automations/webhook/{automationId}
X-Webhook-Secret: <secret>
Content-Type: application/json
```

Kandev silently reads only the first 1 MiB of the request body; it does not reject an oversized body. If that retained prefix is valid JSON, it becomes trigger data. Empty or invalid JSON is wrapped as `{"body":"<raw text>"}`. The endpoint returns 401 for a wrong secret, 404 for an unknown automation, and 409 when the automation or its webhook trigger is disabled.

Webhook delivery has no event deduplication or filter-expression evaluator. Make downstream actions idempotent when the sender retries. The secret is stored with the automation rather than in Kandev's encrypted provider-secret store, and anyone with Kandev settings access can reveal it. Treat it as a credential, use TLS, keep it out of URLs/logs, and replace the automation if rotation is required.

### Manual trigger

The play action in the automations table fires a run with trigger type `manual` and no deduplication. Use it on an enabled automation to test repository/profile resolution and inspect the resulting history. The UI currently leaves **Play** enabled for a disabled automation: the HTTP request reports that it triggered, but the orchestrator silently discards the event and creates no run-history row. Re-enable the automation before testing.

## Prompt and title placeholders

Every trigger supports `{{trigger.type}}`, `{{trigger.timestamp}}`, and `{{data.<path>}}`.

GitHub PR runs additionally support `{{pr.number}}`, `{{pr.title}}`, `{{pr.url}}`, `{{pr.author}}`, `{{pr.repo}}`, `{{pr.branch}}`, `{{pr.base_branch}}`, and `{{pr.body}}`.

Webhook runs support `{{webhook.body}}` and `{{webhook.<path>}}`. Dot segments traverse nested objects, and a numeric segment indexes an array, for example `{{webhook.commits.0.message}}`. Scalar values are converted to text; objects and arrays become JSON. Missing or unresolved placeholders are removed rather than sent literally.

Trigger payloads are untrusted input. Do not let a PR body or webhook field silently choose credentials, repositories, shell commands, or a production target.

## Concurrency, history, and cleanup

Maximum concurrent runs defaults to 1 and cannot be less than 1. A run counts as active while its status is `task_created`. When the cap is reached, Kandev records a `skipped` run and advances the schedule's evaluation time rather than retrying every 30 seconds.

Run history shows the latest 50 entries by default and can report `triggered`, `task_created`, `succeeded`, `failed`, `skipped`, or `cancelled`. A missing or archived pending task is reconciled as cancelled. Task-mode entries link to their kanban task; Run-mode entries remain in history only.

Deleting one run also deletes its associated task. **Delete all runs** deletes all associated tasks and history for that automation and is irreversible.

## Task MCP

Kandev automatically injects a task-aware MCP server into supported agent sessions. You do not need to add it to the profile. It lets the active agent use current IDs and structured operations instead of inferring board state from text.

Task mode currently registers these tool groups:

| Group | Available operations |
|---|---|
| Board lookups and task lifecycle | List workspaces, workflows, workflow steps, tasks, agents, and executor profiles; create, update, move, archive, or delete tasks; halt all live work on a direct child. This mode does not mutate workflows, profiles, or executors. |
| Coordination | Message a task or targeted session, spawn a named session on the current or another same-workspace task, and read task conversation. |
| User interaction | Ask a structured question when the current agent/session supports it. |
| Plans | Create, get, update, and delete the current task plan. |
| Walkthroughs | Show, get, and delete the task's code walkthrough. |
| Relationships and branches | List related tasks, add a branch/worktree to a task, and change a repository's diff base. |
| Workflow signal | Signal step completion when an auto-advance step explicitly requires that signal. |

Task identity is injected for operations that require it. Workspace, parent/subtask, executor, and task-state rules still apply.

`spawn_session_kandev` creates a named sibling session on the current task by default and can target another task in the same workspace. `message_task_kandev` can address a task's primary session or an explicit session ID: a running agent receives queued input, an idle/created session can be started, and a failed or cancelled session rejects the message.

A same-task message requires the sibling's session ID. Normal messages can cross workspaces when the sender knows the full task ID. Delivery to a running session is queued by default. When a direct child must abandon its current approach and receive replacement work now, its parent should use `message_task_kandev` with `delivery_mode: "interrupt"`; another sender receives a hard error rather than a silent downgrade, and a request that cannot dispatch safely remains queued.

Use `stop_task_kandev` only when the direct child should halt without a replacement prompt. It has no session selector: one call gracefully stops every execution Kandev still observes as live across the child's active sessions, including non-primary sessions. Accepted sessions become `CANCELLED` before runtime teardown is scheduled. `status: "stopped"` confirms logical cancellation and asynchronous teardown, not operating-system process exit; a child with no live execution returns the idempotent `status: "not_running"` without changing task or session state.

After an accepted stop, Kandev attempts to move an unarchived, non-Office task from `IN_PROGRESS` or `SCHEDULING` to `REVIEW`; other task states are preserved. Worktrees, task environments, commits, task records, descendants, and queued messages remain available, and the task can be started again later.

`add_branch_to_task_kandev` works only with the Worktree executor. It can add another branch of the same repository or a second repository entirely. Select at most one of `repository_id`, `repository_url`, or `local_path`; `repository_url` accepts a GitHub repository URL, while the URL/path forms find or create that repository in the task's workspace. A locator is optional for a single-repository task and required to disambiguate a multi-repository task. The new repository/branch receives its own worktree. `update_repository_base_branch_kandev` changes the base used for Kandev's diff, not a pull request's target branch.

`step_complete_kandev` matters only for an auto-advance action configured to require an explicit signal. A user message arriving before transition can cancel that automatic move.

The task server runs inside agentctl's local runtime boundary. Its MCP routes do not use a separate bearer token. Do not expose agentctl ports; rely on the executor's process/network isolation and Kandev's session scoping.

## Profile and executor MCP

An agent profile can add `stdio`, `http`, `sse`, or `streamable_http` servers when that agent supports MCP. The built-in Kandev task server is injected separately and cannot be replaced by a profile entry named `kandev`.

Stdio normally starts per session and cannot be shared. Network servers can be shared or per-session. The executor's MCP policy can deny transports/server names, rewrite URLs, or inject environment. See [Agents and profiles](agents-and-profiles.md) for configuration, secret handling, and failure behavior.

## External MCP

Open **Settings > External MCP** (`/settings/external-mcp`) for client-specific snippets for Claude Code, Cursor, Codex, Auggie, OpenCode, and GitHub Copilot CLI.

The recommended Streamable HTTP endpoint is:

```text
http://127.0.0.1:<backend-port>/mcp
```

SSE compatibility uses `/mcp/sse` with messages sent to `/mcp/message`. A reverse proxy must support long-lived streaming connections.

External MCP exposes 32 tools in these groups:

- workspace/workflow configuration: list workspaces, workflows, repositories, and workflow steps; create, update, delete, or import workflows; create, update, delete, or reorder steps;
- agents and profiles: list/update agents; create/delete profiles; list/update profiles; get/update profile MCP configuration;
- executors: list executors and profiles; create, update, or delete executor profiles;
- tasks: list, create, move, delete, archive, or update task state, and read task conversation.

The settings page's static **Available tools** preview currently counts 29 and omits `list_repositories_kandev`, `import_workflow_kandev`, and `get_task_conversation_kandev`. Treat the client's live `tools/list` response from the endpoint—not that preview—as authoritative.

In external mode, `create_task_kandev` has no current task and does not accept the `parent_id: "self"` shorthand. Its registered top-level contract asks for a repository ID, GitHub URL, or local path; workspace and workflow resolve automatically only when unambiguous. The current handler can nevertheless accept an omitted repository and create repo-less work, which is a contract/implementation mismatch rather than a supported equivalent of the regular UI's **None** option. Supply an explicit repository locator for portable clients. A resolvable agent profile is required even with `start_agent: false`; otherwise `start_agent` defaults to true. To create a subtask, pass the full ID of an existing parent.

External mode has no live Kandev session, so it does not expose `stop_task_kandev` or other task-scoped questions, plans, walkthroughs, sibling-session spawning, targeted session messages, branch operations, or step-completion signals. Some external tools can delete or materially reconfigure data; review the client's tool approvals.

### External MCP security boundary

The backend's `/mcp`, `/mcp/sse`, and `/mcp/message` routes currently have no Kandev authentication. They are reachable on every interface to which the backend is bound. Anyone who can reach them can attempt the exposed configuration and task mutations.

- Bind the backend to loopback for a local single-user install.
- For remote use, place the whole backend behind a VPN, firewall, or authenticated TLS reverse proxy.
- Do not publish the MCP routes or backend port directly to the internet.
- Ensure the proxy protects both Streamable HTTP and SSE/message paths and permits long-lived requests.
- Scope integration, Git, and agent credentials for the damage an unattended client could cause.

## Troubleshooting

- **No scheduled run:** use a preset or `@every`; fixed calendar cron and timezone scheduling are not implemented.
- **Scheduled run happened as well as a PR run:** clear the schedule expression. The two stored triggers fire independently.
- **No GitHub PR runs:** connect GitHub and select explicit repositories; **All repos** currently evaluates none.
- **Run fails before a task starts:** select valid non-passthrough agent and non-local executor profiles, and add/select a repository.
- **Run fails on permission:** Run mode cannot answer prompts. Use Task mode or a safely constrained profile that does not require one.
- **Webhook rejected or data is incomplete:** check the exact automation ID, `X-Webhook-Secret` header, and enabled automation/trigger. Bodies over 1 MiB are not rejected; the suffix is silently discarded, so inspect the retained trigger data.
- **Missing template data:** inspect run trigger data and the dot path; unresolved placeholders are intentionally removed.
- **Task MCP tool missing:** confirm this is a Kandev task session, the agent supports the injection strategy, and the operation belongs to task rather than external mode.
- **External client cannot stream:** verify the base backend URL and configure the reverse proxy for both the selected MCP transport and long-lived requests.

Related: [Tasks and workflows](tasks-and-workflows.md), [Coordination](coordination.md), [Agents and profiles](agents-and-profiles.md), and [Integrations](integrations.md).
