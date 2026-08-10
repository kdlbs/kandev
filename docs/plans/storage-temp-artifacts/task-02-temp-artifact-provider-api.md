---
id: "02-temp-artifact-provider-api"
title: "Temporary-artifact storage provider and API"
status: done
wave: 2
depends_on: ["01-temp-ownership-registry"]
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 02: Temporary-artifact storage provider and API

Expose the registered artifacts through the existing Storage overview and run contracts. Cleanup is
an explicit-only provider: scheduled maintenance and an unscoped full run must not delete temp roots.

## Acceptance

- `GET /api/v1/system/storage` reports `temporary_artifacts` bytes/counts and capability state while
  analysis remains read-only; active/recent and unsafe records are separated from stale candidates.
- `POST /api/v1/system/storage/run` accepts `resources: ["temporary_artifacts"]`, moves only valid
  stale roots into quarantine, and returns provider counts/warnings; the empty selection and scheduled
  paths do not perform temp cleanup.
- The new `temporary_artifact` quarantine type survives list/restore/purge flows, cross-device or
  partial failures remain visible and retryable, and existing activity-gate/force semantics remain.

## Verification

```bash
cd apps/backend && go test ./internal/system/storage/... ./internal/backendapp/... -run 'Temporary|TempArtifact|Storage|Quarantine' -count=1
```

## Files likely touched

- `apps/backend/internal/system/storage/types.go`
- `apps/backend/internal/system/storage/operations.go`
- `apps/backend/internal/system/storage/runner.go`
- `apps/backend/internal/system/storage/handler.go`
- `apps/backend/internal/system/storage/store.go`
- `apps/backend/internal/system/storage/store_migrations.go`
- `apps/backend/internal/system/storage/tempartifacts/provider.go`
- `apps/backend/internal/system/storage/tempartifacts/provider_test.go`
- `apps/backend/internal/system/storage/handler_test.go`
- `apps/backend/internal/system/storage/operations_test.go`
- `apps/backend/internal/system/storage/runner_test.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`
- `apps/backend/internal/backendapp/storage_test.go`

## Dependencies

Task 01.

## Parallelism

`sequential`; it owns shared storage contracts and backend composition.

## Inputs

- Plan Backend sections: Storage provider and API composition.
- Existing `ExplicitCleanupProvider` used by the Go-cache action.
- Existing workspace/Go-cache quarantine and overview-cache implementations.

## Output contract

Report the provider resource name, summary/result JSON shapes, default-versus-explicit dispatch
behavior, quarantine integration, exact focused test output, and synchronized task/plan status.

## Results

- Added the manual-only `temporary_artifacts` provider with read-only analysis, active/recent
  protection, fixed 24-hour stale eligibility, and same-filesystem quarantine moves.
- Added storage summary/capability JSON, explicit resource dispatch, provider results/warnings, and
  `temporary_artifact` quarantine restore/permanent-delete integration.
- Crash reconciliation covers rename-before-lifecycle-update and interrupted-rename cases; restored
  artifacts can be quarantined again with a fresh quarantine entry ID.
- Backend verification passed:
  `go test ./internal/system/storage/... ./internal/improvekandev/... ./internal/agent/hostutility/... ./internal/backendapp/... -count=1`.
