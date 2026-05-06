---
status: draft
created: 2026-04-27
owner: cfl
---

# Office: Agent Task Context & Instructions

## Why

When the scheduler wakes an agent to work on a task, the agent needs identity, context, instructions, and API access. Without these, it can't authenticate, doesn't know its role, has no procedure to follow, and wastes tokens re-reading context on every wakeup.

## What

### Two separate systems

**Instructions bundle** (per-agent, defines identity):
- `AGENTS.md` -- persona, delegation rules, operating procedure (injected into prompt)
- `HEARTBEAT.md` -- per-wakeup checklist (on disk, agent reads it)
- `SOUL.md` -- voice/tone guidelines (on disk, agent reads it)
- `TOOLS.md` -- living doc the agent updates with tools it discovers (on disk)
- Stored in the DB per agent instance, editable in the agent detail "Instructions" tab
- Exported to disk before each session so the agent can read sibling files

**Skills** (shared across agents, defines capabilities):
- `kandev-protocol` -- how to call the kandev API
- `memory` -- how to read/write agent memory
- User-created skills (code-review, deploy-runbook, etc.)
- Stored in the DB, managed in the Skills page, assigned to agents
- Written to the agent's session worktree (CWD) before each session

### Storage and export flow

Both instructions and skills are stored in the **database** (source of truth). Before each agent session, they are written to the agent's session worktree (CWD) for the agent CLI to discover:

```
DB (source of truth)
  ├── Agent instructions (per-agent)
  └── Skills (shared)
       │
       ▼  write to worktree before session
       │
  <worktree>/
  ├── .agents/skills/
  │   ├── kandev-<slug>/               # injected skills (kandev- prefix)
  │   │   └── SKILL.md
  │   └── ...
  └── (team-committed skills at .agents/skills/<slug>/ are also discovered)
       │
  instructions are stored alongside the session:
       │
  ~/.kandev/runtime/<workspace-slug>/instructions/<agentId>/
  ├── AGENTS.md
  ├── HEARTBEAT.md
  └── SOUL.md
```

Skills are isolated per agent session via worktrees. Instructions are exported to a shared runtime dir (path injected into the prompt so the agent knows where to find them). The `.git/info/exclude` file in the worktree is updated with `kandev-*` so injected skills don't appear as dirty files.

### Environment variables

Injected before each agent session:

| Variable | Value | Purpose |
|----------|-------|---------|
| `KANDEV_API_URL` | `http://localhost:<port>/api/v1` | Base URL for API calls |
| `KANDEV_API_KEY` | Per-run JWT | Bearer token authentication |
| `KANDEV_AGENT_ID` | Agent instance ID | Agent's own identity |
| `KANDEV_AGENT_NAME` | Agent name (e.g. "CEO") | Human-readable name |
| `KANDEV_WORKSPACE_ID` | Workspace ID | Scope for API calls |
| `KANDEV_TASK_ID` | Task ID | Which task to work on |
| `KANDEV_RUN_ID` | Wakeup request ID | Audit trail header |
| `KANDEV_WAKE_REASON` | Reason string | Why the agent was woken |
| `KANDEV_WAKE_COMMENT_ID` | Comment ID (if applicable) | Which comment triggered wake |
| `KANDEV_WAKE_PAYLOAD_JSON` | Inline JSON | Pre-computed task context |

### Instruction bundle delivery

**Same strategy for all agent CLIs** -- no adapter-specific delivery:

1. Read AGENTS.md content from `runtime/<ws>/instructions/<agentId>/AGENTS.md`
2. Append a **path directive** telling the agent where to find sibling files
3. Prepend the combined text to the user-turn prompt
4. Agent reads HEARTBEAT.md, SOUL.md from disk during the session (via cat, Read tool, etc.)

**Path directive** appended to AGENTS.md content:
```
The above agent instructions were loaded from {instructionsDir}/AGENTS.md.
Resolve any relative file references from {instructionsDir}.
This directory contains sibling instruction files: ./HEARTBEAT.md, ./SOUL.md, ./TOOLS.md.
Read them when referenced in these instructions.
```

This is universal -- every agent CLI can read files from disk. No need for `--append-system-prompt-file` or `--add-dir` or any adapter-specific flags.

**On session resume**: instructions are NOT re-injected (agent CLI retains them from previous session). Only the wake context is sent.

### Default instruction templates per role

**CEO** (`AGENTS.md`):
- Persona: "You are the CEO. You lead the company, not do individual work."
- Delegation routing table (code -> CTO, marketing -> CMO, etc.)
- Rules: always delegate, never implement, post comments explaining decisions
- Subtask creation procedure (POST /api/v1/tasks with parent_id)
- References to ./HEARTBEAT.md for per-wakeup checklist

**CEO** (`HEARTBEAT.md`):
- 8-step checklist:
  1. Read wake reason
  2. If task_assigned: triage and delegate
  3. If task_comment: read and respond
  4. If task_children_completed: review results, complete parent
  5. If approval_resolved: act on decision
  6. If heartbeat: check workspace status, reassign stalled tasks
  7. Post comments on all actions
  8. Exit

**Worker** (`AGENTS.md`):
- Persona: "You are a worker agent. You implement tasks assigned to you."
- Procedure: read task -> check blockers -> do the work -> post progress -> update status
- Rules: only work on assigned tasks, write tests, make focused commits
- Subtask creation for self-decomposition

**Reviewer** (`AGENTS.md`):
- Persona: "You are a reviewer. You review work done by other agents."
- Review checklist: correctness, quality, security, performance
- Approve/reject procedure via API
- Rules: be specific, suggest fixes, approve if meets requirements

### Wake payload (resume delta)

`KANDEV_WAKE_PAYLOAD_JSON` contains pre-computed context:

```json
{
  "task": {
    "id": "task-123",
    "identifier": "KAN-42",
    "title": "Add OAuth2 login",
    "status": "in_progress",
    "priority": "high",
    "project": "Backend",
    "blockedBy": [],
    "childTasks": ["KAN-43", "KAN-44"]
  },
  "newComments": [
    {"author": "CEO", "body": "Prioritize login flow first.", "createdAt": "..."}
  ],
  "commentWindow": {
    "total": 15,
    "included": 3,
    "fetchMore": false
  }
}
```

On fresh session: full task context. On resume: only new comments since last run.

### Agent detail page: Instructions tab

The agent detail page gets an **Instructions** tab (alongside Overview, Skills, Runs, Memory, Channels):

- File list: AGENTS.md (marked ENTRY), HEARTBEAT.md, SOUL.md, TOOLS.md with byte sizes
- Click a file to view/edit its content (markdown editor)
- "+" button to add custom instruction files
- Default templates provided per role on agent creation
- AGENTS.md is required (always exists), others are optional
- Changes are saved to the DB immediately

### Session preparation flow

When the scheduler processes a wakeup:

```
1. Resolve agent instance (from wakeup payload)
2. Check guard conditions (status, cooldown, checkout, budget)
3. Export agent instructions from DB to runtime dir:
   ~/.kandev/runtime/<ws>/instructions/<agentId>/AGENTS.md
   ~/.kandev/runtime/<ws>/instructions/<agentId>/HEARTBEAT.md (if exists)
   ~/.kandev/runtime/<ws>/instructions/<agentId>/SOUL.md (if exists)
4. Create or reuse session worktree (CWD for the agent process)
5. Write skills into worktree (path depends on agent type):
   Claude Code: <worktree>/.claude/skills/kandev-<slug>/SKILL.md
   All others:  <worktree>/.agents/skills/kandev-<slug>/SKILL.md
   Add kandev-* patterns to <worktree>/.git/info/exclude
6. Build prompt (same for ALL agent CLIs):
   - Read AGENTS.md content from runtime dir
   - Append path directive (pointing to instructions dir)
   - Prepend combined text to user-turn prompt
   - Add wake context (reason, task summary, new comments)
   - For CEO heartbeat: add workspace status section
7. Set env vars (KANDEV_API_KEY, KANDEV_TASK_ID, etc.)
8. Set KANDEV_WAKE_PAYLOAD_JSON with pre-computed task context
9. Launch agent via task starter (pass prompt + env, CWD = worktree)
   Skills cleaned up automatically when worktree is deleted at session end
```

## Scenarios

- **GIVEN** a CEO agent assigned a new task, **WHEN** the scheduler wakes it, **THEN** the agent's AGENTS.md with delegation rules is in the system prompt, HEARTBEAT.md is on disk at the instructions dir, env vars are set, and the wake payload contains the task details.

- **GIVEN** a worker agent being resumed for a task_comment, **WHEN** it's a resume session, **THEN** only the new comment is sent in the prompt (instructions not re-injected, agent CLI retains them).

- **GIVEN** a user editing the CEO's AGENTS.md in the Instructions tab, **WHEN** they save, **THEN** the DB is updated. The next time the CEO wakes, the updated instructions are exported to disk and used.

- **GIVEN** a reviewer agent woken for a review, **WHEN** the scheduler prepares the session, **THEN** the reviewer's AGENTS.md (review checklist) is in the prompt, its desired skills are written to the session worktree, and the wake payload contains the task's changes.

## Out of scope

- SOUL.md and TOOLS.md content for v1 (empty files created, content written later)
- Automatic TOOLS.md generation from API schema
- Per-task instruction overrides (all agents of a role share the same instructions)
