---
status: draft
created: 2026-05-10
owner: kandev
---

# Office Task Handoffs

## Why
Office coordinators need to plan work across multiple agents before every artifact exists. A coordinator may create a full task tree first, then let planning, implementation, and review agents exchange context through parent documents or shared workspaces without relying on hidden chat history.

## What
- Parent tasks can act as the default home for shared specs, plans, and coordination documents for their child tasks.
- Child tasks can read parent-owned documents without copying document content into every task description.
- Child tasks can write parent-owned coordination documents by default.
- Agents can list related tasks, including parent, children, siblings, blockers, and blocked tasks, and can read/write allowed task documents through MCP or CLI tools.
- Cross-task document handoffs reuse the existing blocker mechanism: a consumer task is blocked-by the producer task and reads the resulting documents from its prompt context after wake-up. No separate "required documents" data type — the blocker IS the readiness gate.
- A parent task can define a default child workspace policy for its task tree: children inherit the parent workspace or create their own workspace.
- A parent task can define a default child ordering policy: children are created with dependency edges for sequential work or without those edges for parallel work.
- Each child task can override the parent workspace policy and run in the parent workspace, a new workspace, or an explicit shared workspace group.
- When a user creates a subtask from an existing Kanban task detail dialog, the dialog lets them choose whether the subtask inherits the parent materialized workspace or creates a new workspace from selected repositories, local folders, or remote URLs.
- Subtasks that inherit a parent materialized workspace are represented through the same shared workspace group model used by Office-created tasks.
- Shared workspaces are visible in the UI, including source task, member tasks, materialized path/environment, and whether tasks are ordered by dependencies.
- Workspace sharing does not lock execution in v1; sequential behavior is expressed through task blocker/dependency edges.
- Agents get clear prompt context telling them whether to read parent documents, source-task documents, or shared workspace files, but document bodies are not injected inline.
- Archiving or deleting a task releases it from shared workspace membership and can trigger cleanup when no active, non-archived members remain.
- Archiving a parent task cancels active descendant runs, recursively archives descendant tasks, releases their shared workspace memberships, and then evaluates workspace cleanup.
- Deleting a parent task cancels active descendant runs, recursively deletes descendant tasks, releases their shared workspace memberships, and then evaluates workspace cleanup.
- Kandev-owned materialized workspaces are cleaned up after the last active member is archived or deleted and no active sessions reference the workspace.
- Kandev does not snapshot workspace contents before cleanup; files in cleaned Kandev-owned folders, clones, or worktrees are intentionally discarded.
- Unarchiving a task with a cleaned Kandev-owned workspace recreates a fresh materialized workspace from stored source configuration when possible.
- User-owned local folders and existing local checkouts are never deleted by workspace cleanup.

## Scenarios
- **GIVEN** a coordinator creates a planning task and an implementation task with the implementation blocked-by the planner, **WHEN** the planner writes `spec` and `plan` documents up to the parent and completes, **THEN** the blocker resolves and the implementation task wakes; its prompt names the parent's available document keys so the agent fetches them via the task document tool.
- **GIVEN** a child task reads documents from its parent, **WHEN** the child agent starts, **THEN** the prompt names the parent task and available document keys and instructs the agent to fetch them with the task document tool.
- **GIVEN** an implementation task needs a sibling planning task's documents, **WHEN** the implementation agent starts (after its blocker on the planner resolves), **THEN** it can list related sibling tasks and fetch the planning task's `spec` and `plan` documents.
- **GIVEN** a parent task policy says children inherit the parent workspace and run sequentially, **WHEN** the coordinator creates child tasks, **THEN** the child tasks reuse the parent materialized workspace and receive dependency edges that order their execution.
- **GIVEN** a user opens a Kanban task detail page, **WHEN** they create a subtask, **THEN** they can choose to inherit the parent task workspace or create a new workspace by selecting repositories, local folders, or a remote URL.
- **GIVEN** the user chooses to inherit the parent workspace for a Kanban subtask, **WHEN** the subtask launches, **THEN** it reuses the parent task's materialized workspace through a shared workspace group.
- **GIVEN** a child task overrides the parent policy to use a new workspace, **WHEN** it launches, **THEN** it uses the normal workspace creation path, such as a new git worktree or its own plain folder.
- **GIVEN** two tasks share a workspace group without dependency edges, **WHEN** the scheduler starts both tasks, **THEN** they may run concurrently in the same materialized workspace because v1 does not enforce workspace locks.
- **GIVEN** a user opens a task that shares a workspace, **WHEN** they view the task detail page, **THEN** they see a shared-workspace indicator, source/member tasks, and the active branch/path.
- **GIVEN** several tasks share a Kandev-created git worktree, **WHEN** all non-archived members are archived or deleted and no active sessions reference it, **THEN** Kandev removes/prunes the worktree and records the cleanup result.
- **GIVEN** a task uses a user-owned local folder or existing checkout, **WHEN** the task is archived or deleted, **THEN** Kandev releases task membership but does not delete the folder or checkout.
- **GIVEN** an archived task had a Kandev-created plain folder that was cleaned, **WHEN** the task is unarchived, **THEN** Kandev recreates an empty folder and does not restore previous files.
- **GIVEN** an archived task had a Kandev-created git worktree or remote clone that was cleaned, **WHEN** the task is unarchived, **THEN** Kandev recreates a fresh worktree or clone from stored repository, branch, and remote configuration.
- **GIVEN** an archived task's workspace cannot be recreated from stored configuration, **WHEN** the task is unarchived, **THEN** the task becomes active with a workspace-requires-configuration status visible to the user.
- **GIVEN** a parent task has active or pending descendant tasks, **WHEN** the user archives the parent task, **THEN** Kandev cancels active descendant runs, archives every descendant task, releases their workspace memberships, and runs cleanup rules.
- **GIVEN** a parent task has descendant tasks, **WHEN** the user deletes the parent task, **THEN** Kandev cancels active descendant runs, deletes every descendant task, releases their workspace memberships, and runs cleanup rules.

## Out of scope
- Preventing parallel editing of the same dirty workspace.
- Automatic guessing between task documents and repo files.
- Moving provider/model fallback into this feature; provider routing is specified separately in `../office-provider-routing/spec.md`.
- Replacing existing task documents or task plans.
- Building workspace locks or active-holder recovery.
