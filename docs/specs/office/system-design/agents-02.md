---
status: draft
system: office
requirements:
  - REQ-OFFICE-AGENTS-001
created: 2026-04-25
owners:
  - cfl
---
# Office: Agents System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AGENTS-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AGENTS-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Runtime

Each scheduler-launched agent turn has a runtime context with workspace, agent, task, run, session, wakeup reason, and capability scope. Agents mutate Office through a narrow action surface, not direct service access.

### Capabilities

A run carries an explicit capability scope. Capabilities include: post comment, update task status, `create_task`, `create_subtask`, create project, request approval, read/write memory, inspect assigned skills.

- Runtime actions check capabilities before mutating state.
- Runtime actions attach agent/run/session identity to emitted records whenever the underlying feature supports it.
- A run may update its current task, any task explicitly granted in the scope, or every task only when the wildcard scope is granted.
- Denied runtime actions fail with a forbidden error and do not call downstream services.

### Environment variables

The Office scheduler injects the runtime environment before each Office agent
turn. These values are runtime credentials, not operator configuration:
users do not create them, copy them from settings, or reuse them between
sessions. Regular Kanban/task-mode sessions do not receive this environment
and use their injected Kandev MCP tools instead.

| Variable | Value | Purpose |
|---|---|---|
| `KANDEV_API_URL` | `http://localhost:<port>/api/v1` | Base URL for API calls |
| `KANDEV_API_KEY` | Per-run JWT | Bearer token authentication |
| `KANDEV_AGENT_ID` | Agent instance ID | Agent's own identity |
| `KANDEV_AGENT_NAME` | Agent name | Human-readable name |
| `KANDEV_WORKSPACE_ID` | Workspace ID | Scope for API calls |
| `KANDEV_TASK_ID` | Task ID | Which task to work on |
| `KANDEV_RUN_ID` | Wakeup request ID | Audit trail header |
| `KANDEV_WAKE_REASON` | Reason string | Why the agent was woken |
| `KANDEV_WAKE_COMMENT_ID` | Comment ID (if applicable) | Which comment triggered wake |
| `KANDEV_WAKE_PAYLOAD_JSON` | Inline JSON | Pre-computed task context |
| `KANDEV_WAKE_PAYLOAD_PATH` | Workspace-relative JSON file path | Pre-computed task context when too large for inline env |
| `KANDEV_CLI` | Path to agentctl | CLI binary for API operations |

An Office-mode launch requires `KANDEV_CLI`, `KANDEV_API_URL`,
`KANDEV_API_KEY`, `KANDEV_AGENT_ID`, `KANDEV_WORKSPACE_ID`, `KANDEV_RUN_ID`,
and the task-bound `KANDEV_TASK_ID`. Kandev fails the launch before starting
the agent process when that signed run context is incomplete or bound to a
different task. Generic task/session start paths must not manufacture,
persist, or accept user-supplied substitutes for these values; only the
internal Office scheduler adapter supplies the task-bound launch map.

`KANDEV_CLI` resolves per executor:
- **Docker** (`local_docker`): `/usr/local/bin/agentctl` (baked into the image).
- **Standalone** (`local_pc`): path from `launcher.findAgentctlBinary()`.
- **Sprites/Remote**: agentctl path inside the remote environment.

### Wake payload

`KANDEV_WAKE_PAYLOAD_JSON` carries pre-computed context. Fresh session: full task context (`task` object with id, identifier, title, status, priority, project, `blockedBy`, `childTasks`). Resume: only new comments since last run plus a `commentWindow` rollup (`{total, included, fetchMore}`). New comments include author, body, createdAt. If the serialized payload exceeds 64KB for inline environment delivery, Kandev writes it under the workspace and sets `KANDEV_WAKE_PAYLOAD_PATH` to that workspace-relative file path instead.

### Instructions delivery

Same strategy for all agent CLIs (no adapter-specific delivery):

1. Read `AGENTS.md` content from `runtime/<ws>/instructions/<agentId>/AGENTS.md`.
2. Append a **path directive** telling the agent where to find sibling files.
3. Prepend the combined text to the user-turn prompt.
4. Agent reads `HEARTBEAT.md`, `SOUL.md` from disk during the session (via cat, Read tool, etc.).

Path directive appended to `AGENTS.md` content:

```
The above agent instructions were loaded from {instructionsDir}/AGENTS.md.
Resolve any relative file references from {instructionsDir}.
This directory contains sibling instruction files: ./HEARTBEAT.md, ./SOUL.md, ./TOOLS.md.
Read them when referenced in these instructions.
```

**On session resume**: instructions are NOT re-injected (agent CLI retains them). Only the wake context is sent.

### Skill injection

Skill content is stored in the DB (source of truth). Before each session, each desired skill's `SKILL.md` is written into the agent's worktree CWD. If the stored content does not already begin with YAML frontmatter delimited by `---`, runtime materialization prepends generated `name` and `description` frontmatter from the skill slug so agent CLIs can load the file as a native skill.

Each agent type defines `ProjectSkillDir` in its `RuntimeConfig`:

| Agent CLI | `ProjectSkillDir` |
|---|---|
| `claude-acp` (Claude Code) | `.claude/skills` |
| `grok-acp` (Grok) | `.grok/skills` |
| `codex-acp`, `opencode-acp`, `gemini`, `copilot-acp`, `auggie`, `amp-acp` | `.agents/skills` |

Default (if unset): `.agents/skills`. Skills are written to `<worktree>/<ProjectSkillDir>/<DirName(slug)>/SKILL.md`, where `DirName` prefixes the slug with `kandev-` unless it already carries that prefix (so a bundled system skill slugged `kandev-protocol` lands at `kandev-protocol/`, not `kandev-kandev-protocol/`). The `kandev-` prefix distinguishes injected skills from team-committed skills already in the repo.

Before writing skills, all existing `kandev-*` directories in the target path are deleted (clean-slate). Removed skills don't linger; updated skills get fresh content.

`kandev-*` patterns are added to the repository's **common** git dir (`$GIT_COMMON_DIR/info/exclude`, i.e. `<repo>/.git/info/exclude`) so injected skills never appear as dirty files. Git has no per-worktree exclude file — it always reads `info/exclude` from the common dir, so a linked worktree's own `.git/worktrees/<name>/info/exclude` is never consulted, and this file is shared across every worktree of the repo.

The written pattern is `<ProjectSkillDir>` **as resolved through symlinks, relative to the worktree root** — not the literal `ProjectSkillDir` string. For a target repo where none of these dirs are symlinked, that's the same value shown in the table above:

```
.claude/skills/kandev-*
.grok/skills/kandev-*
.agents/skills/kandev-*
```

But when `ProjectSkillDir` is itself a symlink — as with this repo's own `.claude/skills -> ../.agents/skills` — the pattern resolves to the symlink's target instead, e.g. `.agents/skills/kandev-*` rather than `.claude/skills/kandev-*`. Writing the literal, unresolved `ProjectSkillDir` would silently fail to match, since injected files land at the resolved path.

**Per-agent isolation:** each agent session gets its own worktree (CWD), so skill directories are fully isolated between concurrent agents. No shared HOME directories, no symlink management, no shutdown cleanup hooks.

**Per executor type:**

| Executor | Worktree location | How skills arrive |
|---|---|---|
| `local_pc` / `worktree` | Host filesystem | Written directly by the scheduler |
| `local_docker` | Host dir, mounted into container at same path | Written on host before container start |
| `sprites` | Local staging dir, uploaded during instance setup | Written to staging, uploaded via Sprites filesystem API |

**Compatibility fallback:** for agent types without a known skill directory, the skill's `SKILL.md` content is appended to the system prompt.

### Session preparation flow

When the scheduler processes a wakeup:

1. Resolve agent instance (from wakeup payload).
2. Check guard conditions (status, cooldown, checkout, budget).
3. Export agent instructions from DB to `~/.kandev/runtime/<ws>/instructions/<agentId>/`.
4. Create or reuse session worktree (CWD for the agent process).
5. Clean `kandev-*` from the skill dir; write desired skills to `<worktree>/<ProjectSkillDir>/<DirName(slug)>/SKILL.md`; ensure the common git dir's `info/exclude` has the resolved `kandev-*` pattern.
6. Build prompt: read `AGENTS.md` content, append path directive, prepend to user-turn prompt, add wake context. For CEO heartbeat: add workspace status section.
7. Set env vars (`KANDEV_API_KEY`, `KANDEV_TASK_ID`, `KANDEV_CLI`, etc.).
8. Set `KANDEV_WAKE_PAYLOAD_JSON` with pre-computed task context, or `KANDEV_WAKE_PAYLOAD_PATH` when the payload is too large for inline env.
9. Launch agent via the task starter (prompt + env, CWD = worktree). Skills are cleaned up automatically when the worktree is deleted at session end.

### Default instruction templates per role

Seeded on agent creation; users edit them in the Instructions tab.

- **CEO `AGENTS.md`**: persona ("You are the CEO. You lead the company, not do individual work."), delegation routing table (code -> CTO, marketing -> CMO, etc.), rules (directly handle first triage and small coordination/status concerns; delegate implementation work; post comments explaining decisions), subtask creation procedure, references to `./HEARTBEAT.md`.
- **CEO `HEARTBEAT.md`** (8-step checklist): read wake reason; if `task_assigned` triage directly and delegate only when independent evidence justifies it; if `task_comment` read and respond; if `task_children_completed` review and complete parent; if `approval_resolved` act on decision; if `heartbeat` check workspace status and reassign stalled tasks; post comments on all actions; exit.
- **Worker `AGENTS.md`**: persona ("You are a worker agent. You implement tasks assigned to you."), procedure (read task -> check blockers -> do the work -> post progress -> update status), rules (only work on assigned tasks, write tests, focused commits), and scope escalation to the CEO rather than recursive self-decomposition by default.
- **Reviewer `AGENTS.md`**: persona ("You are a reviewer. You review work done by other agents."), review checklist (correctness, quality, security, performance), approve/reject procedure, rules (be specific, suggest fixes, approve if meets requirements).

## API surface

### Agent CRUD

- `GET /api/v1/office/agents` - list agents in workspace.
- `POST /api/v1/office/agents` - create agent (UI or CEO).
- `GET /api/v1/office/agents/:id` - agent detail.
- `PATCH /api/v1/office/agents/:id` - update agent (permissions, name, budget, etc.).
- `DELETE /api/v1/office/agents/:id` - delete agent.

### Dashboard and runs

- `GET /api/v1/office/agents/:id/summary?days=14` - aggregate dashboard payload composing existing data (`office_runs`, `tasks`, `office_cost_events`, `office_activity_log`) into the precomputed shapes the four charts and costs view need. Fields:
  - `agent_id`
  - `latest_run` (SessionSummary-shaped, including run id)
  - `run_activity[]`: per-day `{date, succeeded, failed, other, total}`
  - `tasks_by_priority[]`: per-day `{date, critical, high, medium, low}`
  - `tasks_by_status[]`: per-day `{date, todo, in_progress, in_review, done, blocked, cancelled, backlog}`
  - `success_rate[]`: per-day `{date, succeeded, total}`
  - `recent_tasks[]`: `{task_id, identifier, title, status, last_active_at}`
  - `cost_aggregate`: `{input_tokens, output_tokens, cached_tokens, total_cost_cents}`
  - `recent_run_costs[]`: `{run_id, run_id_short, date, input_tokens, output_tokens, cost_cents}`

- `GET /api/v1/office/agents/:id/runs?cursor=&limit=` - cursor-paginated run list. Cursor = `(requested_at, id)` desc. Default limit 25, max 100. Used by both the full-page runs list and the recent-runs sidebar (fixed `limit=30`).

- `GET /api/v1/office/agents/:id/runs/:runId` - run detail: status, short id, invocation source, start/finish timestamps, duration, agent adapter/model, token + cost rollup, `session_id_before`/`session_id_after`, error message, tasks touched, invocation (adapter + cwd + command + env), events list, log offset.

### Permissions metadata

`/meta` includes a `permissions` array (each entry `{key, label, description}` for every permission in the table above) and a `permissionDefaults` object (per role, default value per permission) so the frontend renders the configuration UI without hard-coding the catalogue.

### agentctl CLI surface

Agents call the `kandev` command group on the agentctl binary instead of raw curl. Mutating subcommands use the signed runtime action surface under `/api/v1/office/runtime/…`; they never use generic Kanban or dashboard mutation endpoints. Auth reads `KANDEV_API_URL`, `KANDEV_API_KEY`, `KANDEV_RUN_ID`, `KANDEV_AGENT_ID`, `KANDEV_TASK_ID` from environment. Output is structured JSON by default; `--format text` for human-readable. Errors: non-zero exit + `{"error":"message","code":409}`. Task ID defaults to `$KANDEV_TASK_ID` when `--id`/`--task` is omitted.

```
# Task operations (singular)
agentctl kandev task get    [--id ID]
agentctl kandev task update [--id ID] [--status STATUS] [--comment BODY]
agentctl kandev task create --title TITLE [--description TEXT] [--parent ID] [--assignee AGENT_ID] [--project PROJECT_ID]

# Task operations (plural)
agentctl kandev tasks list         [--status S] [--assignee ID] [--project ID]
agentctl kandev tasks message      --id T-1 --prompt MSG
agentctl kandev tasks conversation --id T-1

# Projects
agentctl kandev projects list
agentctl kandev projects create --name NAME [--description TEXT] [--repository URL_OR_PATH ...] \
  [--lead-agent-profile-id ID] [--color COLOR] [--budget-cents N] [--executor-config JSON]

# Comments + memory + checkout
agentctl kandev comment add        --task ID --body BODY
agentctl kandev comment list       --task ID [--limit N] [--after COMMENT_ID]
agentctl kandev memory get         [--layer LAYER] [--key KEY]
agentctl kandev memory set         --layer LAYER --key KEY --content CONTENT
agentctl kandev memory summary
agentctl kandev checkout           --task ID

# Agents (CEO-only roster control)
agentctl kandev agents list   [--role ROLE] [--status STATUS]
agentctl kandev agents create --name N --role R [--budget-monthly-cents …] [--reason …]
agentctl kandev agents update --id A-1 [--name …] [--budget-monthly-cents …]
agentctl kandev agents delete --id A-1

# Routines
agentctl kandev routines list
agentctl kandev routines create --name N --task-title T --assignee A-1 \
  [--cron "0 9 * * MON-FRI"] [--timezone TZ] [--concurrency …]
agentctl kandev routines pause   --id R-1
agentctl kandev routines resume  --id R-1
agentctl kandev routines delete  --id R-1

# Approvals
agentctl kandev approvals list   [--status pending|approved|rejected]
agentctl kandev approvals decide --id AP-1 --decision approve|reject [--note …]

# Budget
agentctl kandev budget get [--agent-id A-1]
```

### MCP modes

Three MCP modes coexist:

| Mode | Tools | Token cost/turn | Used by |
|---|---|---|---|
| `ModeTask` | 27 (kanban + plans + walkthroughs + coordination + completion) | ~3-5K | Interactive kanban sessions |
| `ModeConfig` | 29 (workflows + agents + executors) | ~8-10K | Config setup sessions |
| `ModeOffice` | 12 (plans + tasks + rich output + decisions + completion) | ~1-2K | Office agent sessions |

Agent routing and Office ownership are independent. Workflow-level defaults, per-step agent profiles, and `runner` participants select the execution identity only; they never make a Kanban task Office-owned. A task is Office-owned only when it is linked to an Office project or its workflow matches the workspace's Office workflow.

`ModeOffice` includes:
- 4 plan tools (`create_task_plan_kandev`, `get_task_plan_kandev`, `update_task_plan_kandev`, `delete_task_plan_kandev`).
- `ask_user_question_kandev` (only meaningful when the user opens the task in advanced mode).
- `list_related_tasks_kandev`.
- 3 task-document tools (`list_task_documents_kandev`, `get_task_document_kandev`, `write_task_document_kandev`).
- `show_rich_output_kandev`, `record_step_decision_kandev`, and gated `step_complete_kandev`.

`ModeOffice` excludes kanban/config tools and workspace/workflow listing tools. Its first-turn context lists only registered tools, and Office mutations use `$KANDEV_CLI`.

### Skills are preferred over MCP tools

Skills are the preferred pattern for teaching agents office capabilities. A skill provides instructions in `SKILL.md` and the agent calls API endpoints via `$KANDEV_CLI`. This is cheaper than MCP tools: instructions read once per session, shell calls thereafter; MCP tool definitions add per-call overhead (tool schemas in context, structured I/O parsing on every invocation). The `kandev-protocol` system skill teaches CLI usage and replaces the earlier curl-based version. New office capabilities expose API endpoints, ship a skill that teaches the agent how to call them, and assign the skill to agents that need it.

## UI

### `/office/agents`

Agent list page: cards for each instance (icon, name, role, status indicator, current task if working, budget gauge, skill badges); "+" button to create a new instance (select profile, set name/role/skills/budget); sidebar "Agents" section shows a compact list of all instances with status dots and channel indicators (Telegram, Slack icons if configured); each card shows pending wakeup count and oldest wait time when the agent has a backlog ("3 pending, oldest: 12m ago").

### `/office/agents/[id]`

Real bookmarkable sub-routes; tab strip is `<Link>`s to each sub-route. Default redirects to `/dashboard`.

```
/office/agents/[id]
├── /                -> redirect to /dashboard
├── /dashboard       -> charts, latest run, recent tasks, costs
├── /instructions    -> instruction files
├── /skills          -> assigned skills
├── /configuration   -> permissions + model + executor
├── /runs            -> cursor-paginated run list
├── /runs/[runId]    -> run detail
├── /memory          -> memory entries
├── /channels        -> messaging channels
└── /budget          -> cost limits + spend
```

Every page is a Next.js Server Component that fetches initial data on the server (direct HTTP to the Go backend) and hydrates a Client Component with the response. Server Component owns the data fetch; Client Component owns interactivity (collapsibles, "Load more", live mode WS). Live mode is a strict enhancement: when a run is RUNNING, the Client Component subscribes to the run WS channel and merges appended messages/events into the SSR-supplied initial state.

#### Dashboard

- **Latest Run card** at the top: status badge, short run id (8 chars), invocation-source pill (`task_assigned`, `task_comment`, `manual_resume_after_failure`, etc.), one-line replied-to summary, relative timestamp, click-through to the run detail.
- **Four 14-day charts** in a 2×2 grid (4×1 on wide screens):
  - **Run Activity**: stacked bars (succeeded / failed+timed_out / other).
  - **Tasks by Priority**: stacked bars (critical / high / medium / low).
  - **Tasks by Status**: stacked bars (todo / in_progress / in_review / done / blocked / cancelled / backlog).
  - **Success Rate**: succeeded ÷ total per day, as a percentage bar or thin line.
- All charts are custom SVG flexbox bars (no chart library). 14-day window is fixed for v1.
- **Recent Tasks**: last 10 tasks the agent worked on, sorted by most recent activity. Identifier + title + status badge. Row click opens the task page.
- **Costs**: aggregate row (input / output / cached / total) plus per-run table for last 10 runs with cost (date / short run id / input / output / cost).

All dashboard data comes from `GET /api/v1/office/agents/:id/summary?days=14` (single round-trip).

#### Run detail

`/office/agents/[id]/runs/[runId]`.

- **Recent runs sidebar** (left): chronological strip of last ~30 runs, each row with status icon (animated when RUNNING), short run id, invocation-source pill, timestamp, one-line summary, optional token + cost. Active row highlighted.
- **Header strip** (main panel):
  - Status badge (queued / running / failed / completed / cancelled / scheduled_retry).
  - Adapter family + model (`claude_local · claude-sonnet-4-6`).
  - Time range: absolute start/end + relative + duration.
  - Token + cost summary (input / output / cached / total).
  - Action buttons by status: **Cancel** (RUNNING), **Resume session** + **Start fresh** (FAILED), **Retry** (scheduled_retry).
  - "Auth required" banner when the error indicates an expired token, with link to agent settings.
- **Session collapsible**: `session_id_before`, `session_id_after`, underlying ACP session id. "Reset session for touched tasks" action clears the resume token on each affected `(task, agent)` pair.
- **Tasks Touched table**: distinct tasks the agent acted on during the run. Each row links to the task. Sourced from `office_activity_log` rows whose `run_id` matches, plus the run's primary task.
- **Invocation panel**: adapter type, working directory, optional Details collapsible with command, env vars, prompt context.
- **Transcript**: embed the existing session-messages component (`AdvancedChatPanel` / `MessageList` from `apps/web/app/office/tasks/[id]/advanced-panels/chat-panel.tsx`), scoped to the run's `session_id`. It already supports messages, tool calls, status rows, scrollback, and live updates.
- **Events log**: structured run events (init, adapter invoke, completion, errors) with timestamp, level, stream (system / stdout / stderr).
- **Live mode**: when running, transcript and events stream in via the existing session WS channel filtered by `run_id`.

#### Other tabs

- **Instructions**: file list (`AGENTS.md` marked ENTRY, `HEARTBEAT.md`, `SOUL.md`, `TOOLS.md`) with byte sizes. Click to view/edit (markdown editor). "+" button to add custom instruction files. Default templates seeded by role on agent creation. `AGENTS.md` is required; others optional. Changes save to DB immediately.
- **Skills**: assigned skills with enable/disable toggles. Agent-created skills marked with an indicator. System skills show a "System" badge with the kandev release version (`system_version`); edit/delete affordances are hidden.
- **Configuration**: all permissions as labeled toggles with on/off state. Role defaults shown as baseline (dimmed label: "from role: worker"). User can toggle to override. `max_subtask_depth` is a number input. Model and executor settings on the same tab. Saves via `PATCH /agents/:id`.
- **Memory**: browsable entries grouped by layer (operating, knowledge, session). View, delete, clear all, export, search.
- **Channels**: configured messaging channels with status, platform icon, setup/edit.
- **Budget**: cost limits and current spend (see [costs](../requirements/costs.md)).

### `/office/workspace/skills`

Skill list (name, description, source type, which agents use each skill), inline editor for SKILL.md content with markdown preview, import flow for local path or git URL, assignment panel selecting which agent instances receive a skill. System skills are read-only with a "System" badge.

## Failure modes

- **Denied runtime action**: agent attempts an action it lacks capability or permission for. Runtime returns a forbidden error; no downstream service is called; no DB mutation occurs.
- **Out-of-scope task mutation**: agent attempts to mutate a task outside its claim scope. Backend returns 403; activity log records the rejection.
- **Privilege escalation attempt**: CEO tries to grant a permission it doesn't have when creating a new agent. Request rejected at the service layer; no agent created.
- **Subtask depth exceeded**: agent attempts to create a subtask deeper than `max_subtask_depth`. Creation rejected; agent receives the rejection in the response.
- **Hire rejection**: user rejects a hire request. The pending agent row is deleted; the requesting agent receives a wakeup with the rejection reason.
- **Budget exhaustion**: agent's budget reaches zero. Status auto-transitions to `paused` with `pause_reason="budget"`. Active sessions complete the current turn but no further prompts are dispatched. Surfaces as a banner on the agent card. See [costs](../requirements/costs.md).
- **Concurrency saturation**: agent at `max_concurrent_sessions`. Scheduler skips claiming wakeups for this agent; wakeups remain in `queued` indefinitely until a slot frees up. No retry, no expiry.
- **Stale MCP tool reference**: agent in `ModeOffice` calls a tool not in the mode (e.g. a kanban tool from an old skill). MCP server returns `"Tool not available in office mode. Use $KANDEV_CLI instead."`.
- **Missing Office launch context**: a generic task/session path attempts to start or resume an Office-owned task without the scheduler-injected CLI path, API URL, signed run token, task-bound identity, or run ID. Kandev fails closed before starting or relaunching the agent process and directs the caller to start or wake the task through Office. It never asks the user to configure or paste these credentials.
- **CLI auth failure**: agentctl call returns 401 because the JWT is expired or invalid. The CLI exits non-zero with structured error. Agent sees a clear failure and can surface it via comment.
- **CLI invoked outside Office**: `agentctl kandev` cannot find `KANDEV_API_URL` or `KANDEV_API_KEY`. The CLI exits non-zero and explains that these values are injected automatically for Office runs; regular task sessions use Kandev MCP tools.
- **Adapter without skill discovery**: agent type has no known `ProjectSkillDir`. Skill `SKILL.md` content appended to the system prompt as fallback.
- **Skill registry edit while session runs**: the running session is unaffected (file already written). Next session for that agent picks up updated content.
- **Worktree deletion**: when the worktree is deleted at session end, all injected skill directories are removed automatically. No explicit cleanup hook needed.
