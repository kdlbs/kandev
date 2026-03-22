# Manual Testing Guide — TaskEnvironment Decoupling (PR #419)

## Prerequisites

- A workspace with at least one workflow configured
- A working executor (local_pc or local_docker)
- An agent profile configured

---

## 1. Basic Task Creation (Regression)

Verify that existing task creation still works with the new TaskEnvironment model.

- [ ] Create a new task via the UI (sidebar "Task" button → fill dialog → submit)
- [ ] Verify the task appears in the kanban board and sidebar
- [ ] Start the task — verify the agent launches and streams messages
- [ ] Check backend logs for `persisting task environment` after launch
- [ ] Verify the task environment API returns data:
  ```
  GET /api/v1/workspaces/{wsId}/tasks/{taskId}/environment
  ```
  Should return `{ id, task_id, worktree_id, status: "ready", ... }`

---

## 2. Worktree Reuse Across Sessions

Verify that creating a second session reuses the existing worktree.

- [ ] Create a task and start Session #1
- [ ] Let the agent make at least one file change (or make one manually via terminal)
- [ ] Stop or let Session #1 complete
- [ ] Open the sessions dropdown (top of the chat panel) → click "New"
- [ ] Start Session #2 with a new prompt
- [ ] Check backend logs for: `reusing existing task environment worktree`
- [ ] Verify Session #2 can see files changed by Session #1 (same worktree)
- [ ] Verify no duplicate worktree was created (check `ls .kandev/worktrees/` or backend logs)

---

## 3. Git Status Persistence

Verify that git status is shared across sessions and persists without an active agent.

- [ ] Start a task, let the agent make changes (unstaged or staged files)
- [ ] Observe the git status panel shows diffs/changed files
- [ ] Stop the agent session
- [ ] **Git status should still be visible** — it's keyed by environment, not session
- [ ] Create a new session on the same task
- [ ] Verify git status shows the same changes (not empty, not duplicated)
- [ ] Switch between Session #1 and Session #2 in the dropdown
- [ ] Both sessions should show **identical** git status (same environment)

---

## 4. Session Lifecycle Actions

### 4a. Stop Session

- [ ] Start a task, let the agent run
- [ ] Open sessions dropdown → find the running session
- [ ] Click the stop button (red square icon) on the active session
- [ ] Verify the agent stops and session state transitions to completed/cancelled
- [ ] Verify the stop button disappears and resume/delete buttons appear

### 4b. Resume Session

- [ ] After stopping a session, click the resume button (green play icon)
- [ ] Verify the agent restarts with the same workspace
- [ ] If this is Session #2+, check that handover context is injected (see Section 5)

### 4c. Delete Session

- [ ] Stop a session first (cannot delete running sessions)
- [ ] Click the delete button (trash icon) on a non-active session
- [ ] Verify the session is removed from the dropdown list
- [ ] Verify the task and other sessions are unaffected

### 4d. State Guards

- [ ] While an agent is running, verify the stop button is visible
- [ ] While an agent is running, verify delete button is **not** visible
- [ ] On a completed session, verify stop button is **not** visible
- [ ] On a completed session, verify resume and delete buttons are visible

---

## 5. Session Handover Context

Verify that new sessions on a task with prior sessions receive handover context.

- [ ] Create a task, start and complete Session #1
- [ ] Create Session #2 on the same task
- [ ] Look at Session #2's first agent turn — it should contain injected context:
  - Session count: "This task has had 1 previous session(s)"
  - Instruction to check existing code before making changes
  - If a task plan exists: the plan content should be included
- [ ] Verify Session #1 (the first session) did **not** get handover context
- [ ] Create a task plan using the agent's MCP tool, then start Session #3
- [ ] Verify Session #3's handover includes the plan content

---

## 6. New Session Dialog

- [ ] Open the sessions dropdown on a task that has an existing session
- [ ] Click "New" to open the new session dialog
- [ ] Verify the dialog shows a prompt input (not the full task creation form)
- [ ] Verify task title / environment info is visible as context
- [ ] Submit with a new prompt
- [ ] Verify the new session is created and starts with the submitted prompt

---

## 7. Sidebar Task List

Verify the sidebar correctly displays task info using environment-keyed data.

- [ ] Check that tasks in the sidebar show correct session state indicators
- [ ] Check that diff stats (files changed count) display correctly
- [ ] Switch between tasks — verify the correct session loads
- [ ] Verify repository path / PR info still displays on task cards

---

## 8. Config Chat (From Main — Regression)

This feature was merged from main during rebase. Verify it works.

- [ ] Navigate to workspace settings or config panel
- [ ] Start a config chat session (if UI entry point exists)
- [ ] Verify the agent has config-specific tools (workflow management, not task planning)
- [ ] Verify the session is ephemeral and doesn't appear in the kanban board

---

## 9. Edge Cases

- [ ] **Task without sessions**: Create a task but don't start it. Verify:
  - No TaskEnvironment is created yet
  - `GET .../environment` returns 404
  - Sidebar shows the task without errors
- [ ] **Browser reload mid-session**: Reload the page while an agent is running. Verify:
  - Session reconnects via WS
  - Git status reloads from environment (not blank)
- [ ] **Multiple tasks**: Run two tasks simultaneously on different executors. Verify:
  - Each has its own independent TaskEnvironment
  - Git status doesn't leak between tasks
- [ ] **Async workspace preparation** (from main): Start a task on Docker executor. Verify:
  - Workspace prepares asynchronously
  - TaskEnvironment is created once preparation completes
