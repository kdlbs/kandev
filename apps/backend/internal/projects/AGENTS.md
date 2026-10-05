# Agent Projects

`projects` owns Agent Project configuration, coordinator and worker policy,
shared context storage, and its workspace-scoped HTTP API. Office projects use
`internal/office/projects` and remain a separate domain.

- Treat `agent_project_id` as the only task identity for this domain. Do not
  reuse Office `project_id` or attach a workflow to a project task.
- Check the Agent Projects feature flag and workspace authorization before any
  read or write with side effects.
- Resolve worker workspace, parent, profile, executor, repositories, and branch
  from the stored project. Never accept those authority fields from MCP input.
- Keep project context under the canonical `agent-projects/<id>/context` root.
  Validate paths and remove task-owned context links without following them.
- Keep context outside Git worktrees and repository Changes projections.
- Preserve project context when a task is removed. Delete it only when the
  project delete request explicitly selects context removal.
- Provision missing `index.md` and `notes.md` starter files independently. Use
  exclusive creation and preserve every existing regular file, including an
  empty file or custom index.
- Keep the editor format check advisory. It inspects raw drafts but never
  serializes, rewrites, or rejects context content.

Requirements and system design live in `docs/specs/projects/`. Follow the
current implementation work orders in `docs/plans/agent-project-context-knowledge/`.
