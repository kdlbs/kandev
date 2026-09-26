---
id: "01-backend-purge-engine"
title: "Backend purge engine"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 01: Backend purge engine

Implement the typed quarantine purge engine and add it to full storage maintenance.

## Acceptance

- Eligible purge deletes only entries at or after `delete_after`; forced purge attempts all active
  entries while bypassing no ownership, path, state, symlink, or pruner validation.
- Purge continues across entry failures and returns complete deleted/protected/failed counts and
  bytes while leaving failed entries retryable.
- Scheduled and full manual runs include the `quarantine` provider; explicit resource selections
  remain limited to the selected provider.

## Verification

```bash
make -C apps/backend test
```

Completed: `go test ./internal/system/... ./internal/backendapp` passed before PR publication;
the remediation reruns the focused storage packages after the review fixes.

## Files likely touched

- `apps/backend/internal/system/storage/quarantine_purge.go`
- `apps/backend/internal/system/storage/workspaces/provider.go`
- `apps/backend/internal/system/storage/workspaces/provider_test.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`
- `apps/backend/internal/backendapp/storage_maintenance_test.go`
- `apps/backend/internal/system/storage/runner_test.go`

## Dependencies

None.

## Parallelism

Sequential; Task 02 consumes the purge types and controller contract.

## Inputs

- Spec: quarantine behavior, state machine, failure modes, and persistence guarantees
- Plan: Typed purge result, resource deletion safety, and maintenance provider composition
- ADR: `docs/decisions/2026-07-29-quarantine-retention-override.md`

## Risks

- Retention bypass must not flow into path/ownership validation.
- Provider cancellation must leave unprocessed entries active and retryable.

## Output contract

Report purge types and dispatch, provider ordering/selection behavior, files changed, exact test
results, blockers/risks, and update this task plus `plan.md` status.
