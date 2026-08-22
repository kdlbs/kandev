---
id: "03-reach-and-scope-resolution"
title: "Reach predicate and scope resolution"
status: todo
wave: 1
depends_on: ["02-visibility-membership-storage"]
plan: "plan.md"
spec: "../../specs/workspaces/membership.md"
---

# Task 03: Reach Predicate and Scope Resolution

The load-bearing task. Review as a security change.

## Acceptance

- `workspaceVisibleTo` implements the spec's reach table exactly: owner, then
  explicit membership row, then `visibility = 'org'` for a non-`guest` org
  member, else unreachable. A `guest` never reaches an org-visible workspace
  without a row.
- `authz.ScopesFor(user, workspace)` implements the spec's resolution order and
  is the **only** place permissions are derived. An explicit membership row
  outranks the org default in both directions: it admits a guest and it narrows
  a member to `viewer`.
- Every guarded workspace action checks a scope through the resolver. A caller
  with `workspace.read` but not the action's scope gets **403**; a caller
  without `workspace.read` gets **404**.
- Resolution **fails closed**: a transient error loading the workspace or
  membership row yields no scopes. It never falls back to the org default role.
  A fault-injection test covers this, because it is the branch that silently
  widens access under load.
- Scopes are resolved **once per request** and carried. A board render issues a
  constant number of membership queries regardless of row count, proven by a
  query-count assertion, not a timing assertion.
- No effective-scope result is cached beyond the request.
- Every existing per-user isolation test passes **unmodified**.
- A reach-and-scope matrix covers every row of the spec's Permissions table
  across the full surface: workspace, task, session, transcript, diff, preview,
  terminal, environment, repository, attachment.
- The in-session agent MCP identity remains the **task owner**, not the last
  prompting member.

## Verification

- `go test ./internal/task/... ./internal/authz/... ./internal/mcp/... ./internal/gateway/websocket/...`
- `go test ./internal/task/service/... -run 'TestReachMatrix|TestScopeResolution|TestScopeQueryCount|TestScopeFailsClosed'`
- Mutation check: invert the membership branch, and separately make the
  fail-closed branch return the org default. Both must fail the matrix; a suite
  that still passes is not testing the resolver.

## Files Likely Touched

- `apps/backend/internal/task/service/service_access.go`
- `apps/backend/internal/authz/resolve.go`
- `apps/backend/internal/task/service/` per-action scope checks
- `apps/backend/internal/mcp/scope/scope.go` (assert unchanged behavior)

## Inputs

- Spec: Permissions (the reach table), Failure modes (403 vs 404, fail closed);
  `roles-and-scopes.md` Resolution.
- Patterns: the `*NotFound` no-leak convention and the "new user-facing service
  entry points must apply scoping" rule in `apps/backend/AGENTS.md`.

## Output Contract

Report the matrix surface count against the authorize-helper count, the
query-count assertion, both mutations and their failures, RED/GREEN commands,
and set this task plus its plan checkbox to done.
