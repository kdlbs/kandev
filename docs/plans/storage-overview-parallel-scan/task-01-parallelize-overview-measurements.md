---
id: "01-parallelize-overview-measurements"
title: "Parallelize overview measurements"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-overview-parallel-scan.md"
---

# Task 01: Parallelize overview measurements

## Acceptance

- Workspace, Go-cache, quarantine, and Docker measurements start concurrently after settings are
  loaded.
- A provider error does not prevent successful results from the other providers from reaching the
  existing `Summary` response.
- The API response, provider construction, and overview cache behavior remain unchanged.
- No measurement goroutine or shared result state produces a race or leak under cancellation.

## Verification

```bash
cd apps/backend && go test -race ./internal/backendapp ./internal/system/storage ./internal/agent/runtime/activity ./internal/agent/runtime/lifecycle
```

The new unit test must use barriers/channels to prove concurrent entry; it must not use a sleep-
based timing assertion.

## Files likely touched

- `apps/backend/internal/backendapp/storage_maintenance.go`
- `apps/backend/internal/backendapp/storage_maintenance_test.go` or the existing focused backend
  composition test file
- Any small test-only provider seams required by the composition test

## Dependencies

None.

## Inputs

- Spec: parallel overview measurements, failure modes, and unchanged API/persistence guarantees
- Existing `storageOverview.Summary` composition in
  `apps/backend/internal/backendapp/storage_maintenance.go`
- Typed providers in `apps/backend/internal/system/storage/workspaces/provider.go`,
  `gocache/provider.go`, `store.go`, and `dockerstore/provider.go`
- Existing cache and API tests in `apps/backend/internal/system/storage`

## Output contract

Return a compact handoff capsule with acceptance status, changed entry points, focused test results,
race-test results, risk tags, and any provider behavior that remains dominant in live timing.

## Result

Implemented the four-way fan-out with isolated result/error slots and preserved the existing
summary/cache contract. The barrier test proves concurrent entry without sleeps. These checks pass:

```text
go test ./internal/backendapp ./internal/system/storage/... ./internal/system/metrics ./internal/agent/runtime/activity ./internal/agent/runtime/lifecycle -count=1
go test -race ./internal/backendapp ./internal/system/storage/... ./internal/system/metrics -count=1
```

The remaining dominant cost is workspace traversal; provider traversal itself was intentionally
left unchanged in this task.
