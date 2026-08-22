---
id: "01-scope-registry"
title: "Scope registry and role mapping"
status: todo
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/auth/roles-and-scopes.md"
---

# Task 01: Scope Registry and Role Mapping

## Acceptance

- `internal/authz/registry.go` declares every org and workspace scope from the
  spec with its identifier and human description, plus the static org-role and
  workspace-role mappings. One registry; no parallel map, switch, or per-route
  constant elsewhere.
- Gin and WS guards take a scope, not a role: `RequireOrgScope(scope)` and a
  workspace-scoped equivalent.
- **Every existing `RequireAdmin` route is migrated** to a named org scope.
  Migrating them is part of this task, not a follow-up: two mechanisms in the
  tree is the failure mode this design exists to avoid.
- A completeness test fails when a guarded action names an unregistered scope,
  and a second one fails when a registered scope has no enforcement call site.
  Both are mutation-verified: adding a bogus scope and removing a call site
  each produce a named failure.
- `users.role` accepts `owner | admin | member | guest`; the migration maps
  `admin -> admin`, `member -> member`, and promotes the instance's first admin
  to `owner`. `guest` is never assigned by migration.
- Last-`owner` demotion/disable and self-role-change are refused.
- `GET /api/v1/authz/scopes` returns id + description for UI labels.
- With `features.auth` off, the synthetic identity holds every scope, so
  behavior is byte-identical to today.

## Verification

- `go test ./internal/authz/... ./internal/auth/... ./internal/user/...`
- `go test ./internal/authz/... -run 'TestScopeCompleteness|TestNoUnenforcedScope'`
- `KANDEV_TEST_POSTGRES_DSN=... go test ./internal/user/...`

## Files Likely Touched

- `apps/backend/internal/authz/{registry,scopes,guards}.go` + tests
- `apps/backend/internal/auth/authn/identity.go`
- `apps/backend/internal/user/models/models.go`, store, migration
- Every current `authn.RequireAdmin()` call site

## Inputs

- Spec: `docs/specs/auth/roles-and-scopes.md` — Scope registry, Role mappings,
  Permissions, Failure modes.
- Patterns: `internal/runtimeflags/registry.go` typed registration with
  completeness tests; the existing `RequireAdmin` / `RequireRealIdentity`
  composition; `docs/specs/auth/self-actions-guard.md`.

## Output Contract

Report the scope count, the `RequireAdmin` call-site count before and after
(after must be zero), both completeness-test mutations and their failure
messages, RED/GREEN commands, and set this task plus its plan checkbox to done.
