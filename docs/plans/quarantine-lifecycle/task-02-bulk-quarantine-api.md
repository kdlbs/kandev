---
id: "02-bulk-quarantine-api"
title: "Bulk quarantine API"
status: done
wave: 2
depends_on: ["01-backend-purge-engine"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 02: Bulk quarantine API

Expose eligible and forced bulk purge through the existing tracked System job contract.

## Acceptance

- `DELETE /api/v1/system/storage/quarantine` accepts only the two specified scope/confirmation
  pairs and returns `400` without starting a job for every mismatch.
- Accepted requests return a `storage-quarantine-delete` job whose result contains the complete
  typed purge summary; partial deletion failures produce a failed job without rolling back
  successful entries.
- Existing per-entry deletion still returns `409` before retention and cached overview data is
  invalidated whenever any entry is deleted.

## Verification

```bash
make -C apps/backend test
```

Completed: the storage API and operations tests passed before PR publication and are rerun with
the remediation checks after review fixes.

## Files likely touched

- `apps/backend/internal/system/storage/operations.go`
- `apps/backend/internal/system/storage/operations_test.go`
- `apps/backend/internal/system/storage/handler.go`
- `apps/backend/internal/system/storage/handler_test.go`

## Dependencies

Task 01.

## Parallelism

Sequential; Task 03 consumes the final HTTP contract.

## Inputs

- Spec: bulk API surface, permissions, failure modes, and scenarios
- Plan: Bulk HTTP operation
- Task 01 purge types and controller method

## Risks

- Route ordering must preserve collection delete and `/:id` delete behavior.
- Partial success must still invalidate the overview before the aggregate error finishes the job.

## Output contract

Report request validation, job result/error semantics, overview invalidation, files changed, exact
test results, blockers/risks, and update this task plus `plan.md` status.
