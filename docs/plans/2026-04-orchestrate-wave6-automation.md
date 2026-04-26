# Orchestrate Wave 6: Routines, Org Chart & Notifications

**Date:** 2026-04-26
**Status:** proposed
**Specs:** `orchestrate-routines`, `orchestrate-overview` (org chart)
**UI Reference:** `docs/plans/2026-04-orchestrate-ui-reference.md` (routines page, org chart, routine rows)
**Depends on:** Wave 4 (scheduler processes wakeups), Wave 5 (notifications wired)

## Problem

Agents can be woken by events, but there's no way to define recurring work (daily digests, weekly scans) or trigger work from external systems (webhooks). The org chart page needs to visualize the agent hierarchy. Notification integration needs to be finalized.

## Scope

### 6A: Routines (backend + frontend)

**Backend:**

**Repository** (`internal/orchestrate/repository/sqlite/routines.go`):
- Full CRUD for `orchestrate_routines`, `orchestrate_routine_triggers`, `orchestrate_routine_runs`
- `GetDueTriggers(ctx, now)` -- triggers where kind=cron AND enabled AND next_run_at <= now
- `ClaimTrigger(ctx, triggerID, oldNextRunAt)` -- atomic UPDATE WHERE next_run_at=old (prevents double-fire)
- `CreateRun(ctx, run)` -- insert with idempotency check
- `GetActiveRunForFingerprint(ctx, routineID, fingerprint)` -- for concurrency check
- `UpdateRunStatus(ctx, runID, status, linkedTaskID)`
- `ListRuns(ctx, routineID, limit, offset)` -- paginated run history

**Service** (`internal/orchestrate/service/routines.go`):
- `TickScheduledTriggers(ctx, now)`:
  - Query due triggers
  - For each: claim, compute next_run_at, dispatch
  - Catch-up policy: skip_missed (default) or enqueue_missed (with cap 25)
- `DispatchRoutineRun(ctx, routine, trigger, payload)`:
  1. Resolve variables (builtins: {{date}}, {{datetime}} + declared defaults + provided values)
  2. Interpolate title and description templates
  3. Compute dispatch fingerprint (hash of resolved template + assignee)
  4. Concurrency check:
     - `skip_if_active`: if active task for this fingerprint exists, mark run `skipped`
     - `coalesce_if_active`: mark run `coalesced`, link to existing task
     - `always_create`: proceed
  5. Create kandev task with `origin=routine`, `routine_run_id`, `assignee_agent_instance_id`
  6. Task creation triggers `task_assigned` wakeup via existing event subscriber (Wave 4)
  7. Mark run as `task_created`, link `linked_task_id`
- `FireManual(ctx, routineID, variableValues)` -- manual trigger with user-provided variables
- `SyncRunStatus(ctx, taskID)` -- when task reaches terminal state, update linked run status

**Webhook endpoint** (`internal/orchestrate/handlers/webhook.go`):
- `POST /api/v1/orchestrate/routine-triggers/:publicId/fire`
- Signature verification by signing_mode: none, bearer, hmac_sha256
- Parse payload as variable values
- Call `DispatchRoutineRun` with webhook source

**Cron evaluation:**
- Use a Go cron library (e.g. `robfig/cron/v3` for parsing, not scheduling)
- `nextCronTick(expression, timezone, after)` -- compute next fire time
- Run inside existing scheduler tick loop (5s interval is sufficient for minute-level cron)

**Variable interpolation** (`internal/orchestrate/service/variables.go`):
- Parse `{{name}}` in strings
- Resolution order: builtins -> declared defaults -> provided values
- `syncVariablesWithTemplate(routine)` -- auto-create variable declarations for new {{}} in templates

**Frontend** (`/orchestrate/routines`):
- Routine list: name, trigger type, schedule, status, last run, next run, assignee
- Click to view detail: config, run history table, edit controls
- Create routine form: name, description, task template fields, trigger config, concurrency policy, variables
- Pause/resume toggle
- "Run Now" button (with variable input form if required variables)
- Run history: status, trigger payload, linked task (clickable)

### 6B: Org Chart Page (frontend)

**`/orchestrate/company/org`:**
- Visual tree of agent hierarchy from `reports_to` relationships
- Each node: card with icon, name, role, adapter type (e.g. "Claude Code"), status dot
- Tree layout: top-down, CEO at root, children below
- Zoom/pan controls (+ / - / Fit buttons)
- Click a node to navigate to `/orchestrate/agents/[id]`
- Use a React tree/graph library (e.g. `reactflow` or a simpler tree layout component)
- Data: fetch agent instances, build tree from `reports_to` links

### 6C: Notification Wiring (backend)

**Wire `orchestrate.inbox_item` to notification service:**
- In `cmd/kandev/gateway.go`: subscribe to `orchestrate.inbox.item` events
- Call existing notification service `HandleEvent()` with event type `orchestrate.inbox_item`
- Build notification message: type, summary, deep link to `/orchestrate/inbox`
- Default: auto-subscribe Local and System providers to `orchestrate.inbox_item`
- Add `orchestrate.inbox_item` to notification events constant list

## Tests

- Routine CRUD
- Cron next tick calculation with timezone
- Variable interpolation: builtins, defaults, provided values
- Dispatch: skip_if_active, coalesce_if_active, always_create
- Catch-up: skip_missed vs enqueue_missed
- Webhook signature verification (bearer, HMAC)
- Idempotency on runs
- Manual trigger with variables
- Org chart tree building from flat agent list
- Notification event subscription

## Verification

1. `make -C apps/backend test` passes
2. Create a routine with cron trigger -> fires on schedule -> creates task -> agent woken
3. Create a routine with webhook trigger -> POST to URL -> task created
4. Manual "Run Now" with variables -> task created with interpolated title
5. Concurrency policies work (skip, coalesce, always_create)
6. Org chart renders agent hierarchy, nodes clickable
7. New inbox items fire browser notification
