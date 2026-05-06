# Office Wave 1: Shell, Data Model & Routing

**Date:** 2026-04-26
**Status:** proposed
**Spec:** `docs/specs/office-overview/spec.md`
**UI Reference:** `docs/plans/2026-04-office-ui-reference.md` (sidebar, layout, component patterns)
**Depends on:** nothing (foundation wave)

## Problem

No office infrastructure exists. We need the database tables, the `/office` page shell with sidebar, routing for all sub-pages, and the backend API stubs before any feature work can begin.

## Scope

### Backend: Database tables (SQLite, code-based migration)

All tables created in a new `internal/office/repository/sqlite/base.go` using `CREATE TABLE IF NOT EXISTS` pattern (matching `internal/task/repository/sqlite/base.go`).

**Tables to create:**

1. **`office_agent_instances`**
   - `id TEXT PRIMARY KEY`, `workspace_id TEXT NOT NULL`, `name TEXT NOT NULL`
   - `agent_profile_id TEXT`, `role TEXT NOT NULL` (ceo/worker/specialist/assistant)
   - `icon TEXT`, `status TEXT NOT NULL DEFAULT 'idle'` (idle/working/paused/stopped/pending_approval)
   - `reports_to TEXT` (FK self), `permissions TEXT` (JSON)
   - `budget_monthly_cents INTEGER DEFAULT 0`, `max_concurrent_sessions INTEGER DEFAULT 1`
   - `desired_skills TEXT` (JSON array of skill IDs)
   - `executor_preference TEXT` (JSON: {type, image, resource_limits, environment_id})
   - `pause_reason TEXT`, `created_at DATETIME`, `updated_at DATETIME`
   - UNIQUE(workspace_id, name)

2. **`office_skills`**
   - `id TEXT PRIMARY KEY`, `workspace_id TEXT NOT NULL`
   - `name TEXT NOT NULL`, `slug TEXT NOT NULL`, `description TEXT`
   - `source_type TEXT NOT NULL` (inline/local_path/git)
   - `source_locator TEXT`, `content TEXT`, `file_inventory TEXT` (JSON)
   - `created_by_agent_instance_id TEXT`
   - `created_at DATETIME`, `updated_at DATETIME`
   - UNIQUE(workspace_id, slug)

3. **`office_projects`**
   - `id TEXT PRIMARY KEY`, `workspace_id TEXT NOT NULL`
   - `name TEXT NOT NULL`, `description TEXT`
   - `status TEXT NOT NULL DEFAULT 'active'` (active/completed/on_hold/archived)
   - `lead_agent_instance_id TEXT`, `color TEXT`
   - `budget_cents INTEGER`, `repositories TEXT` (JSON array)
   - `executor_config TEXT` (JSON: {type, image, resource_limits, worktree_strategy, network_policy, environment_id, prepare_scripts})
   - `created_at DATETIME`, `updated_at DATETIME`

4. **`office_cost_events`**
   - `id TEXT PRIMARY KEY`, `session_id TEXT`, `task_id TEXT`
   - `agent_instance_id TEXT`, `project_id TEXT`
   - `model TEXT`, `provider TEXT`
   - `tokens_in INTEGER`, `tokens_cached_in INTEGER`, `tokens_out INTEGER`
   - `cost_cents INTEGER DEFAULT 0`
   - `occurred_at DATETIME`, `created_at DATETIME`

5. **`office_budget_policies`**
   - `id TEXT PRIMARY KEY`, `workspace_id TEXT NOT NULL`
   - `scope_type TEXT NOT NULL` (agent/project/workspace)
   - `scope_id TEXT NOT NULL`, `limit_cents INTEGER NOT NULL`
   - `period TEXT NOT NULL` (monthly/total)
   - `alert_threshold_pct INTEGER DEFAULT 80`
   - `action_on_exceed TEXT DEFAULT 'notify_only'` (notify_only/pause_agent/block_new_tasks)
   - `created_at DATETIME`, `updated_at DATETIME`

6. **`office_wakeup_queue`**
   - `id TEXT PRIMARY KEY`, `agent_instance_id TEXT NOT NULL`
   - `reason TEXT NOT NULL`, `payload TEXT` (JSON)
   - `status TEXT NOT NULL DEFAULT 'queued'` (queued/claimed/finished/failed)
   - `coalesced_count INTEGER DEFAULT 1`
   - `idempotency_key TEXT`, `context_snapshot TEXT` (JSON)
   - `requested_at DATETIME`, `claimed_at DATETIME`, `finished_at DATETIME`
   - INDEX on (status, requested_at)
   - UNIQUE on (idempotency_key) WHERE idempotency_key IS NOT NULL

7. **`office_routines`**
   - `id TEXT PRIMARY KEY`, `workspace_id TEXT NOT NULL`
   - `name TEXT NOT NULL`, `description TEXT`
   - `task_template TEXT NOT NULL` (JSON)
   - `assignee_agent_instance_id TEXT`
   - `status TEXT NOT NULL DEFAULT 'active'` (active/paused)
   - `concurrency_policy TEXT DEFAULT 'skip_if_active'`
   - `variables TEXT` (JSON), `last_run_at DATETIME`
   - `created_at DATETIME`, `updated_at DATETIME`

8. **`office_routine_triggers`**
   - `id TEXT PRIMARY KEY`, `routine_id TEXT NOT NULL` (FK)
   - `kind TEXT NOT NULL` (cron/webhook/manual)
   - `cron_expression TEXT`, `timezone TEXT`
   - `public_id TEXT`, `signing_mode TEXT`, `secret TEXT`
   - `next_run_at DATETIME`, `last_fired_at DATETIME`
   - `enabled INTEGER DEFAULT 1`
   - `created_at DATETIME`, `updated_at DATETIME`

9. **`office_routine_runs`**
   - `id TEXT PRIMARY KEY`, `routine_id TEXT NOT NULL`, `trigger_id TEXT`
   - `source TEXT NOT NULL` (cron/webhook/manual)
   - `status TEXT NOT NULL DEFAULT 'received'`
   - `trigger_payload TEXT` (JSON), `linked_task_id TEXT`
   - `coalesced_into_run_id TEXT`, `dispatch_fingerprint TEXT`
   - `started_at DATETIME`, `completed_at DATETIME`, `created_at DATETIME`

10. **`office_approvals`**
    - `id TEXT PRIMARY KEY`, `workspace_id TEXT NOT NULL`
    - `type TEXT NOT NULL` (hire_agent/budget_increase/board_approval/task_review/skill_creation)
    - `requested_by_agent_instance_id TEXT`
    - `status TEXT NOT NULL DEFAULT 'pending'` (pending/approved/rejected)
    - `payload TEXT` (JSON), `decision_note TEXT`
    - `decided_by TEXT`, `decided_at DATETIME`
    - `created_at DATETIME`, `updated_at DATETIME`

11. **`office_activity_log`**
    - `id TEXT PRIMARY KEY`, `workspace_id TEXT NOT NULL`
    - `actor_type TEXT NOT NULL` (user/agent/system)
    - `actor_id TEXT NOT NULL`, `action TEXT NOT NULL`
    - `target_type TEXT`, `target_id TEXT`
    - `details TEXT` (JSON)
    - `created_at DATETIME`
    - INDEX on (workspace_id, created_at DESC)

12. **`office_agent_memory`**
    - `id TEXT PRIMARY KEY`, `agent_instance_id TEXT NOT NULL`
    - `layer TEXT NOT NULL` (knowledge/session/operating)
    - `key TEXT NOT NULL`, `content TEXT`
    - `metadata TEXT` (JSON)
    - `created_at DATETIME`, `updated_at DATETIME`
    - UNIQUE(agent_instance_id, layer, key)

13. **`office_channels`**
    - `id TEXT PRIMARY KEY`, `workspace_id TEXT NOT NULL`
    - `agent_instance_id TEXT NOT NULL`, `platform TEXT NOT NULL`
    - `config TEXT` (JSON, encrypted), `status TEXT DEFAULT 'active'`
    - `task_id TEXT` (the channel task)
    - `created_at DATETIME`, `updated_at DATETIME`

14. **`task_blockers`** (junction table, in existing task schema)
    - `task_id TEXT NOT NULL` (the blocked task)
    - `blocker_task_id TEXT NOT NULL` (the blocking task)
    - `created_at DATETIME`
    - PRIMARY KEY (task_id, blocker_task_id)
    - CHECK (task_id != blocker_task_id)

15. **`task_comments`** (office async comments, separate from session messages)
    - `id TEXT PRIMARY KEY`
    - `task_id TEXT NOT NULL`, `author_type TEXT NOT NULL` (user/agent)
    - `author_id TEXT NOT NULL`, `body TEXT NOT NULL`
    - `source TEXT NOT NULL DEFAULT 'user'` (user/agent/channel)
    - `reply_channel_id TEXT` (for channel relay)
    - `created_at DATETIME`
    - INDEX on (task_id, created_at)

**System office workflow** (created on startup per workspace):
- Auto-create a system workflow named "Office" with `is_system=true` for each workspace
- Steps: Backlog (0), Todo (1, is_start_step), In Progress (2), In Review (3), Blocked (4), Done (5), Cancelled (6)
- No step events (no on_enter auto_start_agent -- the office scheduler handles agent lifecycle)
- Store the workflow ID on the workspace as `office_workflow_id` for quick lookup
- This ensures office tasks have valid workflow_id/workflow_step_id -- kanban board, `/t/[taskId]`, stepper, move operations all work unchanged
- Created in `internal/task/repository/sqlite/defaults.go` alongside existing default workspace/executor setup

**Task model extensions** (in existing `internal/task/repository/sqlite/base.go`):

Schema changes (workflow_id stays NOT NULL -- office tasks use the system office workflow):
- `ALTER TABLE tasks ADD COLUMN assignee_agent_instance_id TEXT`
- `ALTER TABLE tasks ADD COLUMN origin TEXT DEFAULT 'manual'` (manual/agent_created/routine)
- `ALTER TABLE tasks ADD COLUMN project_id TEXT`
- `ALTER TABLE tasks ADD COLUMN requires_approval INTEGER DEFAULT 0`
- `ALTER TABLE tasks ADD COLUMN execution_policy TEXT` (JSON)
- `ALTER TABLE tasks ADD COLUMN execution_state TEXT` (JSON)
- `ALTER TABLE tasks ADD COLUMN labels TEXT DEFAULT '[]'` (JSON array of strings)
- `ALTER TABLE tasks ADD COLUMN identifier TEXT` (e.g. "KAN-42", UNIQUE per workspace)

Workspace extensions:
- `ALTER TABLE workspaces ADD COLUMN task_prefix TEXT DEFAULT 'KAN'`
- `ALTER TABLE workspaces ADD COLUMN task_sequence INTEGER DEFAULT 0`
- `ALTER TABLE workspaces ADD COLUMN office_workflow_id TEXT` (FK to the system office workflow)

Service layer changes:
- `CreateTask` for office: set `workflow_id` = workspace's office workflow, `workflow_step_id` = "Todo" step
- `CreateTask`: auto-assign identifier from workspace sequence (`task_prefix + '-' + task_sequence++`)
- Office status changes = move task between steps in the office workflow (reuse existing MoveTask)
- Add circular dependency check on blocker insert

Identifier assignment:
- Only office tasks get identifiers (origin != manual, or project_id set)
- Existing kanban tasks keep identifier=NULL, no backfill needed
- workspace.task_sequence starts at 0, incremented atomically on each office task creation

**TaskSession extensions:**
- `ALTER TABLE task_sessions ADD COLUMN cost_cents INTEGER DEFAULT 0`
- `ALTER TABLE task_sessions ADD COLUMN tokens_in INTEGER DEFAULT 0`
- `ALTER TABLE task_sessions ADD COLUMN tokens_out INTEGER DEFAULT 0`

### Backend: Package structure

```
internal/office/
├── models/          # All office data types
├── repository/
│   └── sqlite/      # SQLite implementation
│       └── base.go  # Schema + migrations
├── service/         # Business logic (thin stubs initially)
├── controller/      # Request/response DTOs + thin wrappers
├── handlers/        # HTTP route registration (Gin)
├── dto/             # HTTP DTOs
└── provider.go      # DI provider function
```

### Backend: API stubs

Register routes under `/api/v1/office/` namespace:

```
# Agent instances
GET    /api/v1/office/workspaces/:wsId/agents
POST   /api/v1/office/workspaces/:wsId/agents
GET    /api/v1/office/agents/:id
PATCH  /api/v1/office/agents/:id
DELETE /api/v1/office/agents/:id

# Skills
GET    /api/v1/office/workspaces/:wsId/skills
POST   /api/v1/office/workspaces/:wsId/skills
GET    /api/v1/office/skills/:id
PATCH  /api/v1/office/skills/:id
DELETE /api/v1/office/skills/:id

# Projects
GET    /api/v1/office/workspaces/:wsId/projects
POST   /api/v1/office/workspaces/:wsId/projects
GET    /api/v1/office/projects/:id
PATCH  /api/v1/office/projects/:id
DELETE /api/v1/office/projects/:id

# Cost events
GET    /api/v1/office/workspaces/:wsId/costs
GET    /api/v1/office/workspaces/:wsId/costs/by-agent
GET    /api/v1/office/workspaces/:wsId/costs/by-project
GET    /api/v1/office/workspaces/:wsId/costs/by-model

# Budget policies
GET    /api/v1/office/workspaces/:wsId/budgets
POST   /api/v1/office/workspaces/:wsId/budgets
PATCH  /api/v1/office/budgets/:id
DELETE /api/v1/office/budgets/:id

# Routines
GET    /api/v1/office/workspaces/:wsId/routines
POST   /api/v1/office/workspaces/:wsId/routines
GET    /api/v1/office/routines/:id
PATCH  /api/v1/office/routines/:id
DELETE /api/v1/office/routines/:id
POST   /api/v1/office/routines/:id/run

# Approvals
GET    /api/v1/office/workspaces/:wsId/approvals
POST   /api/v1/office/approvals/:id/decide

# Activity log
GET    /api/v1/office/workspaces/:wsId/activity

# Inbox (computed view)
GET    /api/v1/office/workspaces/:wsId/inbox

# Agent memory
GET    /api/v1/office/agents/:id/memory
PUT    /api/v1/office/agents/:id/memory
DELETE /api/v1/office/agents/:id/memory/:entryId
GET    /api/v1/office/agents/:id/memory/summary

# Dashboard
GET    /api/v1/office/workspaces/:wsId/dashboard

# Wakeup queue (internal, for dashboard status)
GET    /api/v1/office/workspaces/:wsId/wakeups
```

Wire into `cmd/kandev/gateway.go` and `cmd/kandev/services.go` via Provider pattern.

### Frontend: `/office` shell

**New directory structure:**
```
apps/web/app/office/
├── layout.tsx                    # Office layout (sidebar + topbar)
├── page.tsx                      # Dashboard (server component)
├── page-client.tsx               # Dashboard (client component)
├── inbox/page.tsx
├── issues/
│   ├── page.tsx                  # Issues list
│   └── [id]/page.tsx             # Task detail (simple + advanced toggle)
├── routines/page.tsx
├── projects/
│   ├── page.tsx
│   └── [id]/page.tsx
├── agents/
│   ├── page.tsx
│   └── [id]/page.tsx
├── company/
│   ├── skills/page.tsx
│   ├── costs/page.tsx
│   ├── org/page.tsx
│   ├── activity/page.tsx
│   └── settings/page.tsx
└── components/
    ├── office-sidebar.tsx    # Full sidebar component
    ├── office-topbar.tsx     # Top navigation bar
    └── workspace-switcher.tsx     # Workspace dropdown (reuse existing components/task/workspace-switcher.tsx, same setActiveWorkspace + router.push pattern)
```

**Office layout** (`layout.tsx`):
- Replaces default sidebar with office sidebar
- Shows workspace switcher at top
- Renders office topbar
- Children render in main content area

**Office sidebar** sections:
- Workspace switcher (dropdown)
- New Issue button
- Dashboard, Inbox (with badge)
- Work: Issues, Routines
- Projects: expandable list with +
- Agents: expandable list with + and status dots
- Company: Org, Skills, Costs, Activity, Settings

**Zustand slice stub** (`lib/state/slices/office/`):
```
office-slice.ts     # Slice creator with default state
types.ts                 # All office types
```

**API client stubs** (`lib/api/domains/office-api.ts`):
- Functions for all endpoints above
- Typed with response types

### Frontend: Homepage link

Add "Office" link to the kandev homepage top navigation bar. Navigates to `/office`.

## Event types

Add to `internal/events/types.go`:
```go
// Office events
OfficeAgentCreated      = "office.agent.created"
OfficeAgentUpdated      = "office.agent.updated"
OfficeAgentStatusChanged = "office.agent.status_changed"
OfficeSkillCreated      = "office.skill.created"
OfficeSkillUpdated      = "office.skill.updated"
OfficeProjectCreated    = "office.project.created"
OfficeProjectUpdated    = "office.project.updated"
OfficeApprovalCreated   = "office.approval.created"
OfficeApprovalResolved  = "office.approval.resolved"
OfficeCostRecorded      = "office.cost.recorded"
OfficeWakeupQueued      = "office.wakeup.queued"
OfficeWakeupProcessed   = "office.wakeup.processed"
OfficeRoutineTriggered  = "office.routine.triggered"
OfficeInboxItem         = "office.inbox.item"
```

## Tests

- Backend: table creation test (open in-memory SQLite, run initSchema, verify tables exist)
- Backend: CRUD tests for each repository method (create, get, list, update, delete)
- Backend: handler tests for each endpoint (HTTP status codes, response shapes)
- Frontend: store slice tests (default state, basic setters)

## Verification

1. `make -C apps/backend test` passes
2. `cd apps && pnpm --filter @kandev/web typecheck` passes
3. Backend starts without errors, new tables created in SQLite
4. `/office` page renders with sidebar
5. All API stubs return 200 with empty/stub responses
