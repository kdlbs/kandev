---
status: active
system: tasks
created: 2026-04-28
updated: 2026-08-14
owners:
  - cfl
---
# Task Subtree Controls Requirements

## Overview

This document is the migrated task-system source for the capability. The source detail below remains authoritative while the system is migrated into separate requirement and design records.

## Requirements

### REQ-TASKS-SUBTREE-CONTROLS-001: Task Subtree Controls

**Intent:** Preserve the observable task or workflow behavior recorded by the legacy specification.

#### Acceptance criteria

- **AC-TASKS-SUBTREE-CONTROLS-001.1:** When a consumer uses this capability, the system shall provide the observable behavior and exclusions documented below.

## Migrated source detail

## Why

When a CEO agent delegates a feature into five or more subtasks — each assigned to a
specialist — the operator has no efficient way to intervene at the tree level. If the
wrong branch of work is being executed, or the team discovers a blocking external
dependency, every task must be individually opened and cancelled. There is no single
action that stops the entire family of work, no way to resume it as a unit, and no
aggregated view of what the tree has spent.

Concretely:

- A CEO task creates KAN-14 (architect), KAN-15 (frontend), KAN-16 (backend),
  KAN-17 (tests), KAN-18 (docs). The product owner decides requirements have changed.
  Currently they must cancel all five tasks manually, with no guarantee they caught every
  newly-spawned child.
- After the pause/cancel, the operator wants to know whether the abandoned work cost
  $0.40 or $40 before deciding whether to restore or start fresh.
- Separately, an agent asks a clarification question via `ask_user_questions`, then the
  agent's turn moves on (timeout or redirect). The pending question overlay stays open
  in the UI with no way to dismiss it as cancelled.

## What

### 1. Tree-wide pause

One operator action pauses all tasks in a subtree (root task and every descendant,
regardless of depth).

- Pause does **not** change task status. It creates a durable `office_task_tree_hold`
  record with `mode = "pause"`.
- The wakeup scheduler gate checks for an active pause hold before dispatching any
  wakeup whose `task_id` is a member of that hold. Gated wakeups are dropped (not
  queued for later — the agent will receive a new wakeup when resume fires).
- Running agent sessions are **immediately cancelled** — the system calls `cancelRun()` for each active session in the subtree and waits up to 1 second for cancellation to propagate. Agents do not finish their current turn.
- This is intentional: when the operator pauses, everything stops now. Agents may leave uncommitted work in the worktree, but the worktree state is preserved for resume.
- Resume creates the release record on the hold and removes the gate. Any new events
  after resume produce fresh wakeups normally.

**UI:** "Pause tree" button on the task detail view, visible only when the task has at
least one child task. Shows a count of tasks that will be affected (from the preview
endpoint). After pause, affected tasks display a "Paused" badge in the subtask list.

### 2. Tree-wide cancel

One operator action cancels every task in the subtree.

- All member tasks are set to `CANCELLED` state and `cancelled_at` timestamp.
- Execution locks (`checkout_agent_id`, `checkout_at`) are cleared for all members.
- The current task status of each member is snapshotted into
  `office_task_tree_hold_members.task_status` before the status is changed, enabling
  restore.
- Running agent sessions are **immediately cancelled** via `cancelRun()` with a 1-second propagation wait (same as pause). Any `queued` or `claimed` wakeup for a member task is cancelled. The scheduler will not dispatch new sessions for cancelled tasks.
- A `mode = "cancel"` hold record is created with all members recorded.

**UI:** "Cancel tree" button with a confirmation dialog showing affected task count and
any actively running wakeups. After cancel, the root task shows a "Cancelled (tree)"
badge.

### 3. Tree-wide restore

Reverses a tree-wide cancel: every member task is returned to the status it held
immediately before the cancel hold was created.

- Restore is available only when the tree has an active `mode = "cancel"` hold (not
  after individual manual cancellations, which have no snapshot).
- Restore reads `task_status` from `office_task_tree_hold_members` and writes each
  member back to that status.
- Tasks that were already `CANCELLED` before the hold was created (pre-existing
  cancellations captured with `skip_reason = "already_cancelled"`) are not restored —
  they remain cancelled.
- The cancel hold is released (marked `released_at`) on successful restore.

**UI:** "Restore tree" button visible only on the root task when an active cancel hold
exists. Replaces the "Cancel tree" button in that state.

### 4. Subtree cost summary

Aggregated cost across an entire task tree: root task plus all descendants.

- Uses a recursive CTE starting from the root to find all task IDs in the tree
  (including hidden/archived descendants), then joins with `office_cost_events`.
- Returns total cost in cents, individual token counts (input, cached-input, output),
  and the count of tasks contributing to the total.
- Available at any time, not just when a hold exists.

**UI:** A "Tree cost" card in the task detail sidebar, shown when the task has
subtasks. Displays total spend formatted as currency. Individual task cost (single-task
view) remains unchanged.

**API:** `GET /tasks/:id/tree/cost-summary`

### 5. Clarification dismissal

Pending clarification bundles can be skipped from task chat.

- The chat X control is labelled "Skip all questions" and posts the exact visible bundle to
  `POST /clarification/:id/respond` with `rejected=true`.
- When the in-memory waiter still owns the bundle, rejection unblocks `WaitForResponse` in the same
  turn and every still-pending question row becomes `status = "rejected"`; terminal siblings remain
  unchanged.
- When the current-turn bundle is detached, rejection still persists every still-pending question row
  as rejected, preserves terminal siblings, closes the overlay, and completes clarification cleanup
  without resuming the agent.
- A pending row from an older turn is superseded history. It cannot reappear as the active overlay,
  re-arm a task icon, block workflow completion, or resume the agent from a stale response.
- `POST /clarification/:id/cancel` remains a low-level cancellation operation for a request still in
  the in-memory `clarification.Store`. It closes `CancelCh`, persists `status = "cancelled"`, and
  publishes the existing cancellation event. It is not the chat Skip path.

The current-turn ownership and restart behavior are defined by
[Active Clarification Lifecycle](clarification-active-lifecycle.md).

---

## API

```
POST /tasks/:id/tree/preview        → affected tasks + active wakeups (dry-run, no state change)
POST /tasks/:id/tree/pause          → create pause hold
POST /tasks/:id/tree/resume         → release pause hold
POST /tasks/:id/tree/cancel         → create cancel hold, cancel wakeups, interrupt sessions
POST /tasks/:id/tree/restore        → release cancel hold, restore task statuses
GET  /tasks/:id/tree/cost-summary   → aggregated cost across subtree
POST /clarification/:id/cancel      → cancel a pending clarification question
POST /clarification/:id/respond     → answer or reject the visible clarification bundle
```

### Preview response

```json
{
  "task_count": 6,
  "tasks": [
    { "id": "...", "title": "...", "status": "IN_PROGRESS", "depth": 0 },
    { "id": "...", "title": "...", "status": "TODO",        "depth": 1 }
  ],
  "active_wakeup_count": 2
}
```

### Cost summary response

```json
{
  "task_id": "root-task-id",
  "task_count": 6,
  "include_descendants": true,
  "cost_cents": 4200,
  "tokens_in": 180000,
  "tokens_cached_in": 32000,
  "tokens_out": 8500
}
```

---

## Schema

### `office_task_tree_holds`

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | UUID |
| `workspace_id` | TEXT | FK to tasks.workspace_id (denormalized for queries) |
| `root_task_id` | TEXT | FK to tasks.id |
| `mode` | TEXT | `"pause"` or `"cancel"` |
| `release_policy` | TEXT | JSON; default `{"strategy":"manual"}` |
| `released_at` | DATETIME | NULL when hold is active |
| `released_by` | TEXT | Actor type:id string, e.g. `"user:abc"` |
| `released_reason` | TEXT | Human-readable reason |
| `created_at` | DATETIME | |

### `office_task_tree_hold_members`

| Column | Type | Notes |
|--------|------|-------|
| `hold_id` | TEXT | FK to office_task_tree_holds.id, cascade delete |
| `task_id` | TEXT | FK to tasks.id |
| `depth` | INTEGER | 0 = root |
| `task_status` | TEXT | Status snapshot at hold creation time (for restore) |
| `skip_reason` | TEXT | Why this task was excluded, if applicable (e.g. `"already_cancelled"`) |
| PK | `(hold_id, task_id)` | |

Index: `idx_tree_hold_members_task` on `(task_id)` — supports "is this task gated?" lookup during wakeup dispatch.

---

## Scenarios

**GIVEN** a root task has 3 child tasks all in `IN_PROGRESS` state, **WHEN** the operator clicks "Pause tree", **THEN** an `office_task_tree_hold` with `mode="pause"` is created, all 4 tasks are recorded as members, and the scheduler will not dispatch new wakeups for any member task until the hold is released.

**GIVEN** an active pause hold exists, **WHEN** a `task_comment` wakeup fires for a member task, **THEN** the wakeup dispatcher detects the active hold, marks the wakeup finished with `skip_reason="tree_paused"`, and does not launch an agent session.

**GIVEN** a pause hold exists on a 4-task tree, **WHEN** the operator clicks "Resume", **THEN** the hold is marked released and subsequent events produce fresh wakeups normally.

**GIVEN** a tree of 5 tasks with 2 having active wakeups, **WHEN** the operator clicks "Cancel tree", **THEN** all 5 tasks are set to `CANCELLED`, the 2 active wakeups are cancelled, task statuses are snapshotted for each member, and a `mode="cancel"` hold is created.

**GIVEN** an active cancel hold exists, **WHEN** the operator clicks "Restore tree", **THEN** each task is restored to its pre-cancel status (excluding tasks that had `skip_reason="already_cancelled"`), and the hold is released.

**GIVEN** a task tree has spent across 5 tasks, **WHEN** the operator opens the root task detail, **THEN** the tree cost card shows the aggregated total from `GET /tasks/:rootId/tree/cost-summary`, including token breakdown.

**GIVEN** an agent has posted a current-turn clarification bundle, **WHEN** the operator clicks the Skip (X) control, **THEN** `POST /clarification/:id/respond` rejects that exact bundle, every still-pending question updates to `status="rejected"` while terminal siblings remain unchanged, and an already-detached agent is not resumed.

**GIVEN** a detached clarification remains pending in an older turn, **WHEN** the session accepts newer work, **THEN** the old row remains in history but no active overlay, task icon, workflow barrier, or stale response derives from it.

---

## Out of scope

- Automatic release policies (time-based triggers, condition-based auto-resume).
- Partial subtree operations (pausing only a specific branch while leaving siblings active).
- Hold nesting (creating a hold on a task that is already a member of another hold).
- Bulk operations across multiple unrelated trees in a single API call.
- Cascading hold creation on tasks that are spawned after the hold is created (holds are
  point-in-time snapshots of the tree at creation time; new children are not
  automatically gated).