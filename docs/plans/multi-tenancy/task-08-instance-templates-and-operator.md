---
id: "08-instance-templates-and-operator"
title: "Instance template API and operator role"
status: todo
wave: 3
depends_on: ["07-two-tier-config-storage"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 08: Instance Template API and Operator Role

## Acceptance

- The `operator` instance tier exists on `authn.Identity` and gates
  `/api/v1/instance/*`. `RequireOperator()` follows the shape of
  `RequireAdmin()` and composes with `RequireRealIdentity()`.
- An operator has **no** read access to any org's workspaces, tasks, sessions,
  transcripts, or secrets. A test asserts 404 for each, so the tier cannot
  quietly become a visibility role.
- `/api/v1/instance/templates/{kind}` supports list, create, update, delete for
  the eight kinds and writes only `org_id = ''` rows.
- A template body carrying a secret value, a sensitive env-var value, or an
  integration credential is rejected with
  `400 template_may_not_carry_credentials`, and nothing is written.
- The credential-free invariant is a pinning test over the template DTO field
  set, so adding a credential-bearing field to a templated kind fails the build
  until it is classified.
- On a single-org instance the auth setup wizard's first admin also receives
  `operator`.

## Verification

- `go test ./internal/org/... ./internal/auth/... -run 'TestOperator|TestTemplate'`
- `go test ./internal/... -run TestTemplateMayNotCarryCredentials`

## Files Likely Touched

- `apps/backend/internal/auth/authn/identity.go`
- `apps/backend/internal/org/controller_templates.go`, `service_templates.go`
- `apps/backend/internal/auth/service_setup.go`
- `apps/backend/internal/tenancy/effective.go`

## Inputs

- Spec: Permissions table, API surface, What (templates carry shape not
  credentials).
- Patterns: `authn.RequireAdmin` / `RequireRealIdentity`; the sensitivity
  classification in `internal/common/config/catalog.go`.

## Output Contract

Report the operator-cannot-read test matrix, the credential field pinning
test and the mutation used to prove it fails, RED/GREEN commands, and set this
task plus its plan checkbox to done.
