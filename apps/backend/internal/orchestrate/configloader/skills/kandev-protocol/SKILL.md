---
name: kandev-protocol
description: How to interact with the kandev orchestrate API
---

# Kandev Protocol

You are an agent managed by kandev. This document describes how to authenticate,
communicate, and coordinate with the orchestrator.

## Authentication

All API calls use a bearer token and a run-ID header for audit:

```
Authorization: Bearer $KANDEV_API_KEY
X-Kandev-Run-Id: $KANDEV_RUN_ID
Content-Type: application/json
```

Base URL: `$KANDEV_API_URL`

## Environment Variables

These are injected into your session automatically. Do not hardcode them.

| Variable | Purpose |
|----------|---------|
| `KANDEV_API_URL` | Base URL for all API calls |
| `KANDEV_API_KEY` | Bearer token for authentication |
| `KANDEV_AGENT_ID` | Your agent instance ID |
| `KANDEV_AGENT_NAME` | Your display name (e.g. "CEO") |
| `KANDEV_WORKSPACE_ID` | Current workspace scope |
| `KANDEV_TASK_ID` | Task you are working on (if applicable) |
| `KANDEV_RUN_ID` | Current run ID -- include on all mutating calls |
| `KANDEV_WAKE_REASON` | Why you were woken (see wake reasons below) |
| `KANDEV_WAKE_COMMENT_ID` | Comment ID that triggered the wake (if applicable) |
| `KANDEV_WAKE_PAYLOAD_JSON` | Pre-computed task context -- parse this first |

## Heartbeat Procedure

When you wake up, follow these steps in order.

### Step 1: Read wake reason

Check `$KANDEV_WAKE_REASON`. Possible values:

- `task_assigned` -- a new task was assigned to you
- `task_comment` -- someone commented on your task
- `task_children_completed` -- all child tasks are done
- `approval_resolved` -- an approval you requested was decided
- `heartbeat` -- periodic check-in (CEO agents only)

### Step 2: Parse wake payload

If `$KANDEV_WAKE_PAYLOAD_JSON` is set, parse it. It contains pre-computed context
so you don't need to fetch it from the API (saves tokens):

```json
{
  "task": {
    "id": "task-123",
    "identifier": "KAN-42",
    "title": "Add OAuth2 login",
    "description": "Implement OAuth2 login with Google provider...",
    "status": "in_progress",
    "priority": "high",
    "blockedBy": [],
    "childTasks": ["KAN-43", "KAN-44"]
  },
  "newComments": [
    {"author": "CEO", "body": "Prioritize login flow first.", "createdAt": "2026-04-27T10:00:00Z"}
  ],
  "commentWindow": {
    "total": 15,
    "included": 3,
    "fetchMore": false
  }
}
```

On fresh session: full task context. On resume: only new comments since last run.
If `commentWindow.fetchMore` is true, fetch older comments from the API.

### Step 3: Check blockers

If `task.blockedBy` is not empty, post a comment explaining you are blocked and exit.
Never work on blocked tasks -- the orchestrator will wake you when blockers clear.

```bash
curl -s -X POST "$KANDEV_API_URL/orchestrate/tasks/$KANDEV_TASK_ID/comments" \
  -H "Authorization: Bearer $KANDEV_API_KEY" \
  -H "X-Kandev-Run-Id: $KANDEV_RUN_ID" \
  -H "Content-Type: application/json" \
  -d '{"body": "Blocked by tasks: KAN-43, KAN-44. Waiting for resolution."}'
```

### Step 4: Do the work

Based on your role and the task description, implement what is needed.
Read your instruction files (HEARTBEAT.md, SOUL.md) for role-specific guidance.

### Step 5: Post progress comments

Always post a comment before changing task status. This creates an audit trail
and keeps other agents informed.

```bash
curl -s -X POST "$KANDEV_API_URL/orchestrate/tasks/$KANDEV_TASK_ID/comments" \
  -H "Authorization: Bearer $KANDEV_API_KEY" \
  -H "X-Kandev-Run-Id: $KANDEV_RUN_ID" \
  -H "Content-Type: application/json" \
  -d '{"body": "Implemented OAuth2 login flow with Google provider. Tests pass."}'
```

### Step 6: Update task status

Mark the task as done (or in_review if reviewers are assigned):

```bash
curl -s -X PATCH "$KANDEV_API_URL/orchestrate/tasks/$KANDEV_TASK_ID" \
  -H "Authorization: Bearer $KANDEV_API_KEY" \
  -H "X-Kandev-Run-Id: $KANDEV_RUN_ID" \
  -H "Content-Type: application/json" \
  -d '{"status": "done"}'
```

Use `"status": "in_review"` instead of `"done"` when the task has reviewers.
Use `"status": "blocked"` if you discover a blocker during execution.

### Step 7: Create subtasks (if needed)

If the task is too large, decompose it into subtasks:

```bash
curl -s -X POST "$KANDEV_API_URL/tasks" \
  -H "Authorization: Bearer $KANDEV_API_KEY" \
  -H "X-Kandev-Run-Id: $KANDEV_RUN_ID" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Implement Google OAuth provider",
    "description": "Add Google OAuth2 provider with PKCE flow",
    "parent_id": "'"$KANDEV_TASK_ID"'",
    "assignee_agent_instance_id": "worker-agent-id",
    "project_id": "project-id"
  }'
```

To find available agents for delegation:

```bash
curl -s "$KANDEV_API_URL/orchestrate/agents" \
  -H "Authorization: Bearer $KANDEV_API_KEY"
```

### Step 8: Exit

Your session will end after you finish. The orchestrator will wake you again
when relevant events happen (new comments, child tasks completing, etc.).

## API Reference

### Tasks

```
GET    /orchestrate/tasks/{id}           -- read task details
PATCH  /orchestrate/tasks/{id}           -- update status, priority, assignee
POST   /tasks                            -- create task or subtask
GET    /orchestrate/tasks/{id}/comments  -- list comments on a task
POST   /orchestrate/tasks/{id}/comments  -- post a comment
```

### Agents

```
GET  /orchestrate/agents                 -- list all agent instances (for delegation)
```

### Memory

Persist information across sessions. Use memory to remember decisions,
discovered tools, learned patterns, etc.

```
GET    /orchestrate/agents/{id}/memory/summary  -- read your memory summary
GET    /orchestrate/agents/{id}/memory          -- list all memory entries
PUT    /orchestrate/agents/{id}/memory          -- store a memory entry
DELETE /orchestrate/agents/{id}/memory/{entryId} -- delete a memory entry
```

Memory PUT body format:

```json
{
  "entries": [
    {"layer": "knowledge", "key": "oauth-providers", "content": "Google, GitHub supported"}
  ]
}
```

Layers: `operating` (how you work), `knowledge` (facts you learned), `session` (temporary).

## Critical Rules

1. **Always include X-Kandev-Run-Id** on mutating calls (POST, PUT, PATCH, DELETE).
   This header ties your actions to the current run for audit.

2. **Never retry a 409 Conflict.** This means another agent has claimed the task.
   Post a comment and move on.

3. **Always post a comment before changing task status.** The comment explains
   what you did; the status change signals completion. Never change status silently.

4. **Do not work on blocked tasks.** If `blockedBy` is non-empty, post a comment
   and exit. The orchestrator will wake you when blockers resolve.

5. **If you cannot complete a task,** post a comment explaining why (missing
   permissions, unclear requirements, dependency on external system) and exit.
   Do not set the task to done if it is not actually done.

6. **Parse KANDEV_WAKE_PAYLOAD_JSON first.** It contains pre-computed context.
   Only call the API for data not in the payload. This saves tokens.

7. **Keep comments concise but informative.** Other agents and humans read them.
   Include what you did, what changed, and any decisions you made.

8. **Respect your role.** CEO agents delegate, they do not implement.
   Worker agents implement, they do not delegate to peers.
   Reviewers review, they do not modify code.
