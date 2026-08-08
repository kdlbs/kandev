---
id: "01-temp-ownership-registry"
title: "Temp ownership registry and producer lifecycle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/storage-maintenance.md"
---

# Task 01: Temp ownership registry and producer lifecycle

Create the durable ownership contract needed before any `/tmp` cleanup can be exposed. The registry
must know the exact path and lifecycle state; a filename prefix or old mtime alone is never enough.

## Acceptance

- A registry-created artifact has a durable `storage_temp_artifacts` row and matching owner-only
  marker; missing, mismatched, symlinked, escaped, or unregistered roots are not returned as owned.
- Active leases/heartbeats protect roots, normal close removes or closes them safely, and restart
  reconciliation leaves uncertain or legacy paths untouched while making stale registered rows
  retryable after the 24-hour interval.
- Improve Kandev bundles and the host-utility parent use the registry; the old broad stale-bundle
  name/age sweep is removed without changing the inherited `TMPDIR`/`TMP`/`TEMP` behavior.

## Verification

```bash
cd apps/backend && go test ./internal/system/storage/... ./internal/improvekandev/... ./internal/agent/hostutility/... ./internal/backendapp/... -count=1
```

## Files likely touched

- `apps/backend/internal/system/storage/store_migrations.go`
- `apps/backend/internal/system/storage/store.go`
- `apps/backend/internal/system/storage/tempartifacts/registry.go`
- `apps/backend/internal/system/storage/tempartifacts/registry_test.go`
- `apps/backend/internal/improvekandev/bundle.go`
- `apps/backend/internal/improvekandev/bundle_test.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agent/hostutility/*_test.go`
- `apps/backend/internal/backendapp/helpers.go`

## Dependencies

None.

## Parallelism

`sequential`; Task 02 depends on the registry and storage schema.

## Inputs

- Storage Maintenance spec sections: Kandev-owned temporary artifacts, Data model, Failure modes,
  Persistence guarantees, and Out of scope.
- ADR 0045 and ADR-2026-08-08-owned-temp-artifact-cleanup.
- Existing Improve Kandev owner-marker validation and host-utility process-scoped temp parent.

## Output contract

Report schema/migration names, marker and lease invariants, producer lifecycle changes, exact test
results, any platform-specific liveness caveat, and update this task plus the plan checkbox only
after the focused backend tests pass.

## Results

- Added the replay-safe `storage_temp_artifacts` SQLite table and lifecycle store methods.
- Added the exact-root registry and owner-only `.kandev-temp-artifact.json` marker contract,
  including heartbeat, close, restart reconciliation with per-run identities, symlink/containment
  checks, and protected liveness handling.
- Improve Kandev bundle roots and host-utility parent roots now register with the service registry;
  the old broad stale-bundle sweep was removed while inherited temp environment behavior remains
  unchanged.
- Backend verification passed:
  `go test ./internal/system/storage/... ./internal/improvekandev/... ./internal/agent/hostutility/... ./internal/backendapp/... -count=1`.
