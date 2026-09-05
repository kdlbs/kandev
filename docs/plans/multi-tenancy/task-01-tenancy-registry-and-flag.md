---
id: "01-tenancy-registry-and-flag"
title: "Tenancy registry and feature flag"
status: todo
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 01: Tenancy Registry and Feature Flag

## Acceptance

- `features.multiTenancy` and `features.multiTenancyTrustedStandalone` are
  registered in `internal/runtimeflags/registry.go`, restart-required, and
  `"false"` in every profile in `profiles.yaml`.
- Startup aborts with a named configuration error when `features.multiTenancy`
  is on and `features.auth` is off.
- `internal/tenancy/registry.go` classifies every live table as `instance`,
  `tenant-root`, or `descendant`, and each `descendant` entry records its FK
  path to `workspaces.org_id`.
- A completeness test enumerates the tables the running schema actually creates
  and fails on any table absent from the registry.
- The completeness test is mutation-verified: adding a throwaway table makes it
  fail, and the failure message names the table.

## Verification

- `go test ./internal/tenancy/... ./internal/runtimeflags/...` from `apps/backend`.
- `go test ./internal/backendapp/... -run TestStartupRejectsTenancyWithoutAuth`.

## Files Likely Touched

- `apps/backend/internal/tenancy/registry.go`, `registry_test.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `profiles.yaml`
- `apps/backend/internal/backendapp/` startup validation
- `apps/web/lib/` feature-flag types (flag metadata is contract-tested)

## Inputs

- Spec: Data model (tenancy classification), What (flag semantics).
- Pattern: `internal/runtimeflags/registry.go` typed registration; the
  `/runtime-feature-flags` skill's file-by-file checklist.

## Output Contract

Report the table classification counts, the mutation used to prove the
completeness test fails, RED/GREEN commands, and set this task plus its plan
checkbox to done.
