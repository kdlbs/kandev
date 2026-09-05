---
id: "02-org-entity-and-identity"
title: "Org entity and identity binding"
status: todo
wave: 1
depends_on: ["01-tenancy-registry-and-flag"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 02: Org Entity and Identity Binding

## Acceptance

- `orgs` and `org_os_users` tables exist with the spec's columns, replay-safe
  on fresh and existing databases, on SQLite and Postgres.
- `users.org_id` and `auth_invites.org_id` are added via idempotent
  `ADD COLUMN` migrations; the users email uniqueness becomes
  `UNIQUE (org_id, email)` through a table-rebuild migration with a replay
  regression test proving values and timestamps survive.
- `authn.Identity` carries `OrgID` and `Instance`; the auth middleware
  populates both from the authenticated user.
- A boot migration creates exactly one `is_default` org and assigns every
  existing user, workspace, secret, and config row to it. The migration aborts
  the boot when any row cannot be assigned.
- With the flag off, `OrgID` is empty, no org route is mounted, and the boot
  payload has no `org` key.
- A session or token whose `OrgID` names a missing or suspended org fails
  closed with `org_suspended`, distinct from a session challenge.

## Verification

- `go test ./internal/tenancy/... ./internal/org/... ./internal/auth/...`
- `KANDEV_TEST_POSTGRES_DSN=... go test ./internal/org/... -run TestMigration`
- `go test ./internal/auth/httpmw/... -run 'TestSuspendedOrg|TestIdentityOrg'`

## Files Likely Touched

- `apps/backend/internal/org/{models,store,service,provider}.go`
- `apps/backend/internal/auth/authn/identity.go`
- `apps/backend/internal/auth/httpmw/middleware.go`
- `apps/backend/internal/auth/store/schema.go`, `service_invites.go`, `service_setup.go`
- `apps/backend/internal/user/{models,store}/`
- `apps/backend/internal/tenancy/registry.go` (classify the new tables)

## Inputs

- Spec: Data model, State machine, Failure modes.
- Patterns: `internal/auth/store/schema.go` dialect substitution; ADR 0027
  replayable migrations; the destructive-cutover migration conventions in
  `apps/backend/AGENTS.md` for the users table rebuild.

## Output Contract

Report the migration's fresh-DB and replay results on both dialects, the
abort-on-unassignable-row proof, RED/GREEN commands, and set this task plus its
plan checkbox to done.
