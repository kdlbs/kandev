---
id: "12-org-lifecycle"
title: "Org lifecycle: create, suspend, delete"
status: todo
wave: 5
depends_on: ["08-instance-templates-and-operator", "11-executor-tenant-pinning"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 12: Org Lifecycle — Create, Suspend, Delete

## Acceptance

- `POST /api/v1/instance/orgs` creates an org with a unique lowercase slug, its
  storage root, and a first-admin invite bound to that org.
- Suspension revokes nothing but fails every session and token closed with
  `org_suspended`, stops running executions, and makes background cycles skip
  the org. It is fully reversible: resuming restores access with no data change.
- Deletion requires a body naming the org slug verbatim. It stops executions
  first; if a stop fails the deletion aborts, the storage root still exists, and
  the error names the execution.
- A successful deletion removes every org row across the tenancy registry,
  removes the org storage root, and revokes every session and token for its
  users. A post-delete sweep asserts no row anywhere still carries the org ID —
  this is the registry's payoff and must be driven from the registry, not a
  hand-written table list.
- The default org cannot be deleted while it is the only org.
- Org-scoped side tables without a database cascade are deleted explicitly from
  the org-deleted handler, and the create → delete lifecycle is covered.

## Verification

- `go test ./internal/org/... -run 'TestOrgCreate|TestOrgSuspend|TestOrgDelete'`
- `go test ./internal/org/... -run TestOrgDeleteLeavesNoRows`
- `KANDEV_TEST_POSTGRES_DSN=... go test ./internal/org/...`

## Files Likely Touched

- `apps/backend/internal/org/{service_lifecycle,controller,store}.go`
- `apps/backend/internal/tenancy/registry.go` (drives the delete sweep)
- `apps/backend/internal/auth/store/store.go` (session/token revocation)
- `apps/backend/cmd/kandev/e2e_reset.go`

## Inputs

- Spec: State machine, Failure modes, Permissions.
- Patterns: the workspace-deletion side-table rule in `apps/backend/AGENTS.md`
  (explicit deletion from the deleted handler, lifecycle test, E2E reset
  parity); ADR 0009 fail-closed GC semantics.

## Output Contract

Report the registry-driven post-delete sweep result, the abort-on-live-execution
proof, the E2E reset parity, RED/GREEN commands, and set this task plus its plan
checkbox to done.
