---
title: "Automation and MCP"
description: "Create scheduled, GitHub, or webhook automations and connect agents through task, profile, and external MCP."
---

# Automation and MCP

Kandev has several mechanisms that can act without repeated manual setup. Their scopes and trust boundaries differ:

| Mechanism                   | Purpose                                                                                                        |
| --------------------------- | -------------------------------------------------------------------------------------------------------------- |
| Workflow events and actions | React to an existing task entering a step, receiving a message, or completing an agent turn.                   |
| Workspace automations       | Create task-backed work from a schedule, GitHub pull request, webhook, or manual trigger.                      |
| Task MCP                    | Give an active Kandev session task, plan, conversation, and coordination tools.                                |
| Office MCP and runtime CLI  | Give an Office run a restricted coordination surface and permission-checked commands for Office state changes. |
| Profile MCP                 | Add third-party MCP servers to one agent profile, subject to executor policy.                                  |
| External MCP                | Let a client outside a task configure Kandev and create or manage work through the backend.                    |

Use workflow events for predictable transitions on existing work. Use a workspace automation when an external signal must create new work. MCP is a tool interface, not a scheduler.

Across Kandev's task, configuration, external, and Office MCP modes, each tool call is validated against that mode's live `tools/list` schema before its handler runs. Missing required fields, wrong types, declared constraint violations, and unknown top-level fields return a tool error without performing the requested action. A missing-field error names each absent schema property, but never echoes submitted argument values. Nested configuration maps still accept arbitrary keys when their schema defines them as open.

## Quick path

- Use a **workflow event** for predictable transitions on existing tasks.
- Use a **workspace automation** when a schedule or external signal should create work.
- Use **task MCP** for tools inside an active agent session.
- Use **profile MCP** to add servers to an agent profile.
- Use **external MCP** to expose Kandev tools to third-party clients.
- Treat credentials delivered through any MCP or executor profile as available to the receiving agent.

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

The scheduler checks every 30 seconds and computes each expression's next calendar fire time in its configured timezone. A schedule created part-way through the day first fires at its next scheduled occurrence after creation (not immediately). A schedule missed while the backend was stopped fires once on the next check rather than once per missed occurrence.

<DocsVideo
  webm="./media/feature-guides/scheduled-workflow-automation.webm"
  mp4="./media/feature-guides/scheduled-workflow-automation.mp4"
  poster="./media/feature-guides/scheduled-workflow-automation.webp"
  title="Schedule workflow automation"
  caption="A workflow automation is configured with a schedule and saved for recurring execution."
/>

Use the supplied presets: every 5, 15, or 30 minutes; hourly; every 6 hours; daily; or weekly. The backend also accepts `@every` followed by a Go duration, `@hourly`, `@daily`, `@weekly`, and step forms such as `*/10 * * * *` or `0 */6 * * *`.

The editor accepts arbitrary five-field cron text, including fixed calendar forms such as `30 8 * * *`, weekday ranges such as `15 9 * * 1-5`, and day-of-month schedules — all of which now fire at the correct time. A saved timezone (for example `America/New_York`) is honored, including that zone's daylight-saving transitions; an empty timezone means UTC, so schedules are deterministic regardless of the host clock. Scheduled runs are deduplicated per trigger per minute.

### GitHub pull requests

The GitHub evaluator polls every 60 seconds and requires a working GitHub integration. It searches open PRs and supports:

- an explicit list of repositories;
- base-branch glob filters, including `*` and a trailing wildcard such as `release/*`;
- exact author-login filters;
- draft exclusion.

Select at least one repository. Although the UI offers **All repos**, an empty repository list is not evaluated, so it produces no PR runs. The editor exposes only the **Opened** event, but the evaluator currently ignores the saved event list: clearing that checkbox does not stop polling or firing. Disable the automation/trigger instead. The first evaluation considers every currently open matching PR rather than only PRs opened after the automation was enabled. Each matching PR is then deduplicated once per automation by repository and PR number.

The current evaluator does not apply label filters, and the current form does not offer them.

### GitHub push and CI checks

Push and CI-check conditions are webhook-driven rather than polled. They require a workspace GitHub App connection (Settings > Workspaces > _workspace_ > GitHub) whose installation is subscribed to the `push` and `check_run` events. When the App delivers a matching, HMAC-verified webhook, the installation is resolved to its workspace and the matching automation fires.

- **Push**: fires when commits are pushed to a matching branch. Configure an explicit repository list and optional branch glob filters (`main`, `release/*`). Branch deletions are ignored. Deduplicated per repository by branch and pushed commit SHA.
- **CI check**: fires when a check run completes. Configure an explicit repository list, the conclusions to match (defaults to `failure`, which drives auto-fix-CI flows), and optional check-name and head-branch filters. Deduplicated per repository by check-run ID.

Because GitHub Apps only deliver events a user subscribed to at installation time, an App connection created before push/CI support was added must be reinstalled (or updated from its GitHub settings page) before these webhooks begin arriving. This generic CI trigger is distinct from the task-specific PR check remediation described under review features.

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

Names ending in `_kandev` are the canonical MCP protocol tool names. Some agent clients show or register a server-qualified alias instead. For example, a client may expose canonical `step_complete_kandev` as `mcp__kandev__step_complete_kandev`. That qualified form is client-specific, not a second tool or a universal name; use the form exposed by the active client.

Task tools use normal client discovery. When `step_complete_kandev` is required but is not already visible, the agent should search the active tool catalog for its canonical name. Kandev does not request eager loading through client-specific metadata.

`create_task_kandev` advertises `prompt` for instructions delivered to a newly started agent. Older callers may still send `description` when `prompt` is absent, but sending both is an error; the compatibility name is intentionally omitted from the advertised schema.

Task mode currently registers these tool groups:

| Group                               | Available operations                                                                                                                                                                                                                                               |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Board lookups and task lifecycle    | List workspaces, workflows, workflow steps, tasks, agents, and executor profiles; create, update, move, archive, or delete tasks; halt all live work on a direct child. This mode does not mutate workflows, profiles, or executors.                               |
| Coordination                        | Message a task or targeted session, spawn a named session on the current or another same-workspace task, and read task conversation. See [Agent Communication](agent-communication.md) for delivery semantics, bidirectional reply patterns, and a worked example. |
| User interaction                    | Ask a structured question when the current agent/session supports it.                                                                                                                                                                                              |
| Plans                               | Create, get, update, and delete the current task plan.                                                                                                                                                                                                             |
| Walkthroughs                        | Show, get, and delete the task's code walkthrough.                                                                                                                                                                                                                 |
| Relationships and workspace sources | List related tasks, add a mixed repository/folder source batch to an idle task, use the legacy one-branch tool, and change a repository's diff base.                                                                                                               |
| Workflow signal                     | Signal step completion when an auto-advance step explicitly requires that signal.                                                                                                                                                                                  |

When **Settings → General → Task Actions → Agent-generated task titles** is enabled (the default; an
explicitly saved **off** value remains off), a task-mode session for a newly created task or subtask can
expose `set_task_title_kandev`. The first eligible session to launch atomically claims the handoff and is
prompted to call it before any other work, even though the task already has a provisional title from the
prompt. Use a short title phrase targeting about six words in sentence case rather than a sentence or
progress update.
The tool is omitted for ordinary tasks, tasks created while the setting was disabled, config sessions,
Office sessions, and every later session on the task—even if the owner fails before renaming it. A human
rename wins if it happens first; a late owner call returns `title_not_pending`, while a non-owner call
returns `title_not_owner`, without changing the title.

Task identity is injected for operations that require it. Workspace, parent/subtask, executor, and task-state rules still apply.

`spawn_session_kandev` creates a named sibling session on the current task by default and can target another task in the same workspace. `message_task_kandev` can address a task's primary session or an explicit session ID: a running agent receives queued input, an idle/created session can be started, and a failed or cancelled session rejects the message.

A same-task message requires the sibling's session ID. Normal messages can cross workspaces when the sender knows the full task ID. Delivery to a running session is queued by default. When a direct child must abandon its current approach and receive replacement work now, its parent should use `message_task_kandev` with `delivery_mode: "interrupt"`; another sender receives a hard error rather than a silent downgrade, and a request that cannot dispatch safely remains queued.

Use `stop_task_kandev` only when the direct child should halt without a replacement prompt. It has no session selector: one call gracefully stops every execution Kandev still observes as live across the child's active sessions, including non-primary sessions. Accepted sessions become `CANCELLED` before runtime teardown is scheduled. `status: "stopped"` confirms logical cancellation and asynchronous teardown, not operating-system process exit; a child with no live execution returns the idempotent `status: "not_running"` without changing task or session state.

After an accepted stop, Kandev attempts to move an unarchived, non-Office task from `IN_PROGRESS` or `SCHEDULING` to `REVIEW`; other task states are preserved. Worktrees, task environments, commits, task records, descendants, and queued messages remain available, and the task can be started again later.

`add_workspace_sources_kandev` adds one or more sources to an idle task and defaults `task_id` to the current task. Its `sources` input accepts the same atomic mixed batch as the Files panel: `repository` sources use exactly one saved repository ID, local Git path, or remote repository locator plus branch fields; `folder` sources use a local path and optional display name. Repository sources work on Worktree, Local/Local PC, Local Docker, SSH, and Sprites; folders work only on Worktree and Local/Local PC. The task must be repository-backed and have no active turn or tool call. Invalid, duplicate, unsupported, or failed sources roll back the entire batch.

`add_branch_to_task_kandev` is the Worktree-only compatibility path for adding one repository/branch during an active agent turn. It creates the worktree as a sibling under the task directory, promotes the persisted Files root to that parent, and rescans it without restarting the agent, terminals, or workspace processes. The response returns `worktree_path` (the exact new repository location), `task_workspace_path` (the Files root), and `agent_cwd_changed: false`; deferred pre-launch materialization omits both paths. The original repository stays a separate Git worktree, so the sibling is not reported as an embedded repository or untracked files by its Git status. Use `add_workspace_sources_kandev` for mixed batch attachments to an idle task. `update_repository_base_branch_kandev` changes the base used for Kandev's diff, not a pull request's target branch.

The HTTP equivalent is `POST /api/v1/tasks/:id/workspace-sources`, with `{ "sources": [...] }`. It returns `400` for invalid input, `404` for a missing task/source outside the workspace, `409` for duplicates or an active task, and `422` when materialization or executor capability fails. Successful adoption publishes `task.updated` and `session.workspace_sources.updated`; clients should refresh their Files and repository state from those updates.

`step_complete_kandev` is registered and discoverable in every task-mode session. Kandev includes its completion instruction, and acts on its signal, only on Kanban steps whose auto-advance action explicitly requires that signal. A user message arriving before transition can cancel that automatic move.

The task server runs inside agentctl's local runtime boundary. Its MCP routes do not use a separate bearer token. Do not expose agentctl ports; rely on the executor's process/network isolation and Kandev's session scoping.

<details>
<summary>Office MCP and runtime CLI</summary>

## Office MCP and runtime CLI

Office runs use a smaller MCP surface than regular task-mode sessions. The built-in Office server registers exactly these tools:

- `ask_user_question_kandev`;
- `create_task_plan_kandev`, `get_task_plan_kandev`, `update_task_plan_kandev`, and `delete_task_plan_kandev`;
- `list_related_tasks_kandev`;
- `list_task_documents_kandev`, `get_task_document_kandev`, and `write_task_document_kandev`.

These tools cover human questions, the current task plan, related-task discovery, and task documents. Office state changes use the injected `$KANDEV_CLI kandev ...` commands instead. An Office agent should not search for additional Kandev MCP tools: Kanban/configuration tools and `step_complete_kandev` are task-mode only and are not registered in Office mode.

### Runtime credentials

Kandev injects `$KANDEV_CLI`, `KANDEV_API_URL`, and `KANDEV_API_KEY` when the
Office scheduler starts an Office run. The API key is a short-lived, scoped
runtime token; it is not a personal access token or a value to create, copy,
or persist in configuration. The run also receives its agent, workspace, task,
and run identifiers automatically, and the launch context is bound to that
task.

If `agentctl kandev ...` reports that `KANDEV_API_URL` or `KANDEV_API_KEY` is
missing, do not set either variable yourself. A regular task session should use
its injected Kandev MCP tools. An Office-owned task must be started or woken
through Office so the scheduler can supply its signed runtime context.

An Office run can inspect the projects in its current workspace:

```bash
$KANDEV_CLI kandev projects list
```

An agent with the `can_create_projects` permission can create a project. CEO agents receive this permission by default; other roles do not unless it is explicitly granted. `--name` is required, and `--repository` can be repeated for every repository URL or local path owned by the project:

```bash
$KANDEV_CLI kandev projects create \
  --name "Payments" \
  --description "Payment services and checkout" \
  --repository "https://github.com/acme/payments" \
  --repository "/workspace/checkout"
```

The optional project flags are `--lead-agent-profile-id`, `--color`, `--budget-cents`, and `--executor-config`.

Use the returned project ID when creating work in that project:

```bash
$KANDEV_CLI kandev task create \
  --title "Add payment retry policy" \
  --project "$PROJECT_ID" \
  --assignee "$AGENT_ID"
```

Project list and create operations are forced to the workspace in the validated Office run token; the agent cannot select another workspace in these commands. Office runs cannot create or administer workspaces. Create additional workspaces through Kandev's user-facing setup and settings surfaces.

</details>

<details>
<summary>Profile and executor MCP</summary>

## Profile and executor MCP

An agent profile can add `stdio`, `http`, `sse`, or `streamable_http` servers when that agent supports MCP. The built-in Kandev task server is injected separately and cannot be replaced by a profile entry named `kandev`.

Stdio normally starts per session and cannot be shared. Network servers can be shared or per-session. The executor's MCP policy can deny transports/server names, rewrite URLs, or inject environment. See [Agents and profiles](agents-and-profiles.md) for configuration, secret handling, and failure behavior.

### Diagnose one running session

Use the neutral plug button beside the chat composer to inspect the current session's MCP attachment report. It distinguishes configuration delivered to the agent from a connection observed by Kandev: the built-in task server becomes **Connected** after MCP initialize and **Active** after it serves `tools/list`. A third-party profile server usually remains **Delivered · connection unverified** because it connects directly to the agent rather than through Kandev. Missing observation is not a failure; red appears only for an explicit sanitized error.

On desktop, hover or focus the button for the compact status list. On touch devices, tap its 44px target to open the same list in a bottom drawer. The report is per Kandev session and execution, so simultaneous agents in one task never share a status row. It stores only bounded, sanitized attachment facts: no MCP headers, environment values, tool arguments/results, raw ACP frames, or agent output.

</details>

<details>
<summary>External MCP</summary>

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

`create_task_kandev` accepts task titles up to 60 characters. Use a concise, few-word title and put the implementation context in `description`; longer titles are rejected as validation errors.

External mode has no live Kandev session, so it does not expose `stop_task_kandev` or other task-scoped questions, plans, walkthroughs, sibling-session spawning, targeted session messages, branch operations, or step-completion signals. Some external tools can delete or materially reconfigure data; review the client's tool approvals.

</details>

### External MCP security boundary

The `/mcp`, `/mcp/sse`, and `/mcp/message` routes are mounted through
`externalMCPAuthMiddleware`. With authentication disabled, they remain open for
the current single-user behavior. When authentication is enabled, external
clients must provide a personal access token; an already-authenticated browser
session may also pass the same middleware. This is separate from task-mode MCP,
which runs inside the agentctl session boundary.

- Bind the backend to loopback for a local single-user install.
- For remote use, place the whole backend behind a VPN, firewall, or authenticated TLS reverse proxy.
- Do not publish the MCP routes or backend port directly to the internet.
- Ensure the proxy protects both Streamable HTTP and SSE/message paths and permits long-lived requests.
- Scope integration, Git, and agent credentials for the damage an unattended client could cause.

## Troubleshooting

- **No scheduled run:** confirm the cron expression is valid five-field or `@`-shorthand text and the automation/trigger is enabled; a schedule fires at its next occurrence after creation, not immediately.
- **No GitHub push or CI runs:** connect a workspace GitHub App whose installation is subscribed to `push`/`check_run` events, reinstall a pre-existing App so it receives them, and select explicit repositories (an empty repository list is not evaluated).
- **Scheduled run happened as well as a PR run:** clear the schedule expression. The two stored triggers fire independently.
- **No GitHub PR runs:** connect GitHub and select explicit repositories; **All repos** currently evaluates none.
- **Run fails before a task starts:** select valid non-passthrough agent and non-local executor profiles, and add/select a repository.
- **Run fails on permission:** Run mode cannot answer prompts. Use Task mode or a safely constrained profile that does not require one.
- **Webhook rejected or data is incomplete:** check the exact automation ID, `X-Webhook-Secret` header, and enabled automation/trigger. Bodies over 1 MiB are not rejected; the suffix is silently discarded, so inspect the retained trigger data.
- **Missing template data:** inspect run trigger data and the dot path; unresolved placeholders are intentionally removed.
- **Task MCP tool missing:** confirm this is a Kandev task session, the agent supports the injection strategy, and the operation belongs to task rather than external mode.
- **One agent did not load MCP tools:** inspect that session's toolbar report. A delivered/unverified row is evidence that ACP or passthrough configuration reached the agent, not proof that the agent contacted the server. For deeper developer investigation, run `acpdbg mcp-probe` against the agent and inspect its JSONL.
- **External client cannot stream:** verify the base backend URL and configure the reverse proxy for both the selected MCP transport and long-lived requests.

Related: [Tasks and workflows](tasks-and-workflows.md), [Coordination](coordination.md), [Agents and profiles](agents-and-profiles.md), and [Integrations](integrations.md).
