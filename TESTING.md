# Manual Testing Guide

This file tracks manual testing steps for changes in this PR.

---

## 1. New Agent Session Dialog — Prompt Required

**What changed:** "Start Agent" button is disabled until the prompt textarea has content.

**Steps:**
1. Open any task that has an existing session.
2. Click **New Agent** (or the + icon to start a new session).
3. Verify the **Start Agent** button is greyed out / disabled when the textarea is empty.
4. Type something in the textarea → button becomes active.
5. Clear the text → button becomes disabled again.
6. Test the **Copy previous prompt** context option — button should become active after the copy, and disabled again if you switch back to **Blank**.
7. Test **Summarize** — button should become active once the summary fills in.

---

## 2. New Agent Session Dialog — File Attachments

**What changed:** Users can now attach files to the prompt when starting a new agent session (drag-and-drop or paste).

**Steps:**
1. Open the **New Agent** dialog.
2. **Drag-and-drop** an image or text file onto the textarea. Verify:
   - A "Drop files here" overlay appears while dragging over.
   - After dropping, the attachment appears below the textarea with a preview/name.
   - The **Start Agent** button is now enabled (since a file counts as input? — note: button currently gates on text only, not attachments).
3. **Paste** an image from the clipboard (e.g. take a screenshot, paste with Ctrl+V into the textarea). Verify the attachment appears.
4. Click the remove (×) button on an attachment to remove it.
5. Submit a session with an attachment — the agent should receive the file as part of its initial prompt context.

---

## 3. MCP Subtask — Executor & Agent Profile Inheritance

**What changed:** When an agent creates a subtask via `create_task` MCP tool with `parent_id`, the subtask now inherits the parent's `agent_profile_id` and `executor_profile_id`.

**Steps:**
1. Create a task with a non-default **agent profile** and a custom **executor profile** configured.
2. Start the agent on that task.
3. Have the agent call `create_task` (via a script or natural language prompt that triggers it).
4. Navigate to the **subtask** that was created.
5. Open the subtask's session details / info panel and verify:
   - The **agent profile** matches the parent's agent profile.
   - The **executor profile** matches the parent's executor profile.
6. Confirm the subtask agent actually runs (isn't stuck in a "no executor" state).

---

## 4. MCP Subtask — Repository Inheritance

**What changed:** When an agent creates a subtask via MCP `create_task`, the subtask now inherits the parent task's repository associations. Previously, subtasks started with no repository and ran as "quick chat" sessions.

**Steps:**
1. Create a task that is linked to a **repository** (set a repo in the task create dialog).
2. Start the agent on that task with a prompt like: *"Create a subtask called 'Test Subtask'"*.
3. After the agent creates the subtask, navigate to it (kanban or task list).
4. Verify the subtask is associated with the same repository as the parent:
   - The subtask's session should show the repo/worktree path in the environment info.
   - The agent should have access to the codebase, not start in an empty directory.
5. Run `git status` or a file-listing command in the subtask — it should show the repo contents.

---

## 5. Create Subtask Dialog — Prompt Required

**What changed:** The "Create Subtask" button was already gated on `hasPrompt` (this is a pre-existing check) — verify it still works correctly.

**Steps:**
1. Open the **Create Subtask** dialog from the task sidebar.
2. Verify the **Create Subtask** button is disabled when the prompt is empty.
3. Type a prompt → button becomes active.
4. Verify submission works normally.

---

## 6. Sessions API — `executor_profile_id` Exposed

**What changed:** `GET /api/v1/tasks/{id}/sessions` now returns `executor_profile_id` on each session object.

**Steps (API check):**
1. Create a task with a custom executor profile and start a session.
2. Call `GET /api/v1/tasks/{taskId}/sessions` (via browser devtools or curl).
3. Verify the response includes `executor_profile_id` on each session object (non-empty for sessions that have one).

---

## Post-Rebase Additions

### 7. Task Directory Worktrees

**What changed (from main):** Sessions can reuse an existing `TaskEnvironment` worktree when resuming or creating a second session for the same task.

**Steps:**
1. Create a task linked to a repository. Start a session and let it run.
2. After the session completes, create a **new session** on the same task (via New Agent dialog).
3. Verify the new session reuses the same worktree path (check `worktree_path` in session details or devtools).
4. Confirm no duplicate worktree directories are created on disk.

### 8. Multi-Session — Primary Auto-Promotion

**What changed (from main):** When the primary session terminates, the next active session is automatically promoted to primary.

**Steps:**
1. Create a task and start **two sessions** on it.
2. Confirm one is marked as primary (star icon in the session tabs).
3. Cancel / wait for the primary session to complete.
4. Verify the second session is automatically promoted to primary without manual action.

### 9. Toast Suppression on Missing Branch

**What changed (from main):** Failure handlers can suppress the generic error toast (e.g. when the branch is missing).

**Steps:**
1. Create a task linked to a repository with a branch that doesn't exist yet.
2. Start a session — observe whether a toast error appears or if it is cleanly handled.
3. Verify no duplicate/misleading error toasts are shown when a known failure condition is detected.
