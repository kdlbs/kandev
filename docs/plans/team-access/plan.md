---
spec: docs/specs/auth/requirements/roles-and-scopes.md
created: 2026-08-22
updated: 2026-08-22
status: draft
---

# Team Access Plan — Roles, Scopes, and Workspace Visibility

> **Partly superseded.** Workspace visibility and the membership model it came
> with are replaced by
> [organization units](../org-units/plan.md). The scope registry, the human
> assignee, and actor attribution from this plan stand and are not revisited
> there. Work orders 02, 03 and 08 describe a reach model that no longer ships.

## Scope

Implement [org roles and scopes](../../specs/auth/requirements/roles-and-scopes.md) and
[workspace visibility and membership](../../specs/workspaces/requirements/org-units.md): a
named scope registry with fixed org and workspace roles, workspace visibility
(`org` / `private`) with an org-level default, membership as the exception and
narrowing mechanism, a human task assignee with takeover, and actor attribution
on every human action. Closes
[#2824](https://github.com/kdlbs/kandev/issues/2824).

**Independent of multi-tenancy and ships first.** Everything here works on a
single-org instance and on today's auth model. Organizations are the boundary
above; this is the model within one.

## Current State

Per-user segregation is implemented and funnels through one predicate:

```go
// internal/task/service/service_access.go
func workspaceVisibleTo(workspace *models.Workspace, userID string) bool {
    return workspace == nil || workspace.OwnerID == "" || workspace.OwnerID == userID
}
```

Every `authorize*` helper, the WS dispatch backstop, the MCP scoper and the
office route middleware route through it. That single choke point is the
leverage — and the reason this lands as a security review, not a refactor.

Authorization today is one bit: `authn.Identity.IsAdmin()`. There is no scope
vocabulary, and `RequireAdmin` is applied per route rather than per capability.

Three facts from the code correct assumptions in the issue:

1. **Task assignee is not reusable.** `tasks.assignee_agent_profile_id` names an
   Office *agent profile* (ADR 0005 Wave F), not a person. A human assignee is a
   new field.
2. **Reviewer/approver participants are agents too.**
   `workflow_step_participants` carries `agent_profile_id`, so those roles do not
   compose with human members. Out of scope.
3. **Per-human GitHub identity already exists.** `github_user_connections` is
   keyed `(workspace_id, user_id)` with per-user credential generations, so
   "Approve as X" works per human the moment a member can reach the workspace.
   `internal/github` does not change.

A fourth fact set the design: a workspace is **coarse**. It owns the default
executor, environment and agent profile, the task prefix and sequence, its
Kanban workflow, and integrations resolve a default workspace. A team has one or
two. That is why visibility is the primary mechanism and per-workspace
invitation is the exception — an invite list on a 1-workspace org is the org
roster retyped.

## Architecture

- **One scope registry, one resolver.** `internal/authz/` owns scope
  identifiers, descriptions, and the static role→scope tables, plus the single
  `ScopesFor(user, workspace)` resolver. No second permission map, no per-route
  admin bit surviving as a parallel mechanism. Completeness tests both ways:
  every guarded action names a registered scope, every registered scope has a
  call site.
- **Reach and permission are separate questions.** `workspaceVisibleTo` answers
  reach (owner / explicit row / org-visible). `ScopesFor` answers permission.
  Conflating them is how a `viewer` accidentally gets a shell.
- **Explicit rows outrank the org default, in both directions.** That single
  rule delivers guests-in-one-workspace and members-narrowed-to-viewer without
  a second mechanism.
- **Resolve once per request, never cache across requests.** The resolver is hit
  many times per board render, so the result is computed once per request and
  carried. It is never cached beyond the request: a cache is how a removed
  member keeps access for another minute.
- **Fail closed on resolution errors.** A transient failure loading a
  membership row yields no scopes. It must never fall back to the org default
  role, which is the branch that would silently widen access under load.
- **`workspaces.owner_id` stays authoritative** for the accountable owner; the
  `owner` membership row mirrors it and a consistency test asserts they never
  disagree.
- **Attribution is threaded, never inferred.** The acting `user_id` travels from
  the identity to the persisted row, with no owner fallback — that fallback is
  precisely the shared-login behavior this removes.
- **The upgrade never widens access.** Every existing workspace migrates to
  `private`. Opening the board is an explicit act.

## Backend Touch Points

- New package `internal/authz/`: scope constants, role tables, `ScopesFor`,
  registry completeness tests, gin/WS guards.
- `internal/task/service/service_access.go`: reach predicate learns visibility
  and membership; per-request resolution cache.
- `internal/task/repository/sqlite/`: `workspaces.visibility`,
  `workspace_members`, owner backfill, `tasks.assignee_user_id`,
  `task_session_messages.author_user_id`, `queued_messages.author_user_id`,
  `task_step_transitions.actor_user_id`.
- `internal/task/handlers/workspace_handlers.go`: visibility, member, and
  transfer routes.
- `internal/user/`: `users.role` value migration, `guest` role,
  `/api/v1/users/directory`.
- `internal/auth/`: `me` returns org scopes; self-role-change guard;
  last-owner guard.
- `internal/org/` or `internal/system/settings`: org default visibility.
- `internal/orchestrator/messagequeue/`: author attribution through the queue.
- `internal/gateway/websocket/`: fan-out to everyone who can reach the
  workspace, `workspace.access.updated`, subscription drop on access loss.
- `internal/mcp/scope/`: the agent's in-session identity stays the **task
  owner**, not the last prompting member — widening it would make an agent's
  reach depend on message timing.
- `cmd/kandev/e2e_reset.go`.

## Frontend Touch Points

- Workspace settings: Visibility control, Members section (add via directory
  picker, set role, remove, transfer ownership), gated on `scopes`.
- Org settings: default workspace visibility, and the one-time "make my
  workspaces org-visible" action.
- Task detail and kanban: human assignee with "Assign to me" as the takeover
  affordance; assignee available to existing sidebar filter dimensions.
- Transcript: author attribution on human messages, distinct from agent output;
  unattributed renders neutrally, never as the owner.
- Every control gated on the `scopes` array from the DTO, not on a re-derived
  ID comparison.
- Access loss while viewing: route out with a clear message rather than a
  stalled panel.
- All copy through `t()` in five locales; no em dashes; mobile parity for the
  Members section, role picker, and assignee control.

## Task Waves

| Wave | Tasks | Theme |
|---|---|---|
| 1 | 01, 02, 03 | Scope registry, visibility and membership storage, the reach predicate |
| 2 | 04, 05 | Management API and directory; WS fan-out and revocation |
| 3 | 06, 07 | Human assignee and takeover; actor attribution |
| 4 | 08, 09 | Frontend surfaces; E2E, docs, ADR |

## Tasks

- [ ] [01 — Scope registry and role mapping](task-01-scope-registry.md)
- [ ] [02 — Visibility and membership storage](task-02-visibility-membership-storage.md)
- [ ] [03 — Reach predicate and scope resolution](task-03-reach-and-scope-resolution.md)
- [ ] [04 — Management API, roles, and user directory](task-04-management-api.md)
- [ ] [05 — WebSocket fan-out and revocation](task-05-ws-fanout-and-revocation.md)
- [x] [06 — Human assignee and takeover](task-06-human-assignee-takeover.md)
- [ ] [07 — Actor attribution and audit](task-07-actor-attribution.md)
- [ ] [08 — Frontend team-access surfaces](task-08-frontend-surfaces.md)
- [ ] [09 — E2E, docs, and ADR](task-09-e2e-docs-adr.md)

## Risks

- **The reach predicate is load-bearing for the entire privacy model.** One
  mistake widens access everywhere at once. Mitigation: task 03 keeps every
  existing per-user isolation test unmodified, adds a matrix over the full
  reach table, and lands with a security review.
- **Two mechanisms is the real failure mode.** If `RequireAdmin` survives
  alongside scopes on any route, permissions drift. Task 01's completeness test
  fails on an unmigrated guarded action, and the migration of every existing
  `RequireAdmin` site is part of its acceptance.
- **`session.exec` vs `session.prompt` will be argued about.** Separating them
  is deliberate: prompting is bounded by the agent's permissions, a shell in the
  worktree is not. Do not collapse them for convenience.
- **N+1 on board render.** Without per-request resolution, a shared board issues
  a membership query per row. Task 03 ships a query-count assertion, not a
  timing assertion.
- **Attribution gaps read as fixed.** A missing `author_user_id` falls back to
  empty and renders as neutral rather than failing. Task 07 asserts attribution
  at the producer boundary per write path, not at one consumer.
- **Default-visibility regret.** An org that flips the default to `org` exposes
  future workspaces, not past ones. Existing workspaces stay `private` until
  changed, and the bulk action is explicit and one-time.
- **Interaction with multi-tenancy.** `org` visibility must mean the workspace's
  own org, and cross-org membership must be refused. Owned by the multi-tenancy
  plan, task 04.
