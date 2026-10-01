# ADR-2026-09-24-agent-project-identity-and-context: Separate Agent Projects and context ownership

**Status:** accepted
**Date:** 2026-09-24
**Area:** backend

## Context

`tasks.project_id` already identifies an Office project and is inherited by
child tasks. Office classification, routing, and cost attribution depend on
that meaning. Kandev worktrees also use direct task roots under the configured
tasks base path with ownership markers and cleanup rules. Agent Projects need
a different lifecycle and a shared filesystem store independent of any one
task's checkout.

## Decision

Give Agent Projects their own table and `agent_project_id` task membership.
Do not reuse `tasks.project_id` or Office project rows. Store canonical context
under `agent-projects/<project-id>/context` outside task worktrees. A task-local
`context` symlink may point to that store on a supported local executor, but
the project record and context service are authoritative. Project task roots
continue to use the existing worktree manager's direct-child layout; the main
Files panel presents a virtual `Context` / `Workspace` root.

The first release supports host-local worktree execution using selected remote
repositories. Cross-machine synchronization requires a separate transport
contract before remote executors are admitted.

## Consequences

Office data and current task routing keep their existing meaning. Context
survives worker deletion. Existing database-only backups do not contain these
files; project data backup or export needs a separate contract. Symlinks alone
do not provide remote sharing, so unsupported executors fail admission. Project
deletion needs its own guarded context cleanup path. The virtual Files root
needs explicit authorization and path containment for both backing stores.

## Alternatives Considered

- Reuse `tasks.project_id`: smaller schema change, but it would classify Agent
  Projects as Office work and corrupt inherited routing and cost semantics.
- Nest task roots under a project-named directory: intuitive disk layout, but
  it conflicts with worktree path, ownership marker, and cleanup assumptions.
- Copy context into each worker: works across machines, but concurrent edits
  diverge and later turns cannot trust a single shared `notes.md`.
- Use only a symlink with no project store contract: simple locally, but no
  identity, authorization, or safe remote execution boundary.
