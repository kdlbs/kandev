# ADR-2026-10-14-session-bound-task-visibility: Bind compact lifecycle reads to a task session

**Status:** accepted
**Date:** 2026-10-14
**Area:** backend

## Context

A manual Coordinator conversation needs the lifecycle disposition of tasks that have left its board view. The Office CEO's workspace task-tree capability has a different principal and also permits relation inspection. A task title, agent profile, or caller-supplied MCP scope cannot establish the manual conversation's authority.

## Decision

A trusted backend may record a versioned compact-visibility grant in one persisted manual task session. The grant names that task, session, and workspace exactly. Profile discovery reads the persisted session; each in-session MCP dispatch resolves the task and session again and binds the hidden request envelope to that principal. The new capability exposes only `get_task_visibility` and its existing compact, workspace-bound lifecycle projection. It does not confer Office relation, document, global, foreign-workspace, or write access. Missing, revoked, malformed, or mismatched grants fail closed. The specific grant is activated separately from shipping this capability.

## Consequences

The backend can authorize an attended manual Coordinator without relabeling its task or broadening every Kanban session. A restart retains the grant, and clearing it revokes subsequent dispatches. Runtime activation and deployed task-disposition checks remain separate operations.

## Alternatives Considered

- Give all Kanban sessions a workspace task-tree read: rejected because it broadens relation access to unrelated callers.
- Treat the manual task as an Office or managed Coordinator task: rejected because it changes its established lifecycle and principal.
- Trust the MCP payload's caller and scope fields: rejected because an agent can supply those fields.
