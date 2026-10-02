---
id: "01-direct-deletion"
title: "Direct cache deletion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-GO-CACHE-004
acceptance_criteria:
  - AC-SYSTEM-PAGE-GO-CACHE-004.1
  - AC-SYSTEM-PAGE-GO-CACHE-004.7
  - AC-SYSTEM-PAGE-GO-CACHE-004.8
  - AC-SYSTEM-PAGE-GO-CACHE-004.9
  - AC-SYSTEM-PAGE-GO-CACHE-004.10
  - AC-SYSTEM-PAGE-GO-CACHE-004.11
  - AC-SYSTEM-PAGE-GO-CACHE-004.12
system_design:
  - ../../specs/system-page/system-design/go-cache-reclamation.md
---

# Task 01: Direct cache deletion

## Summary

Replace quarantine rotation with bounded deletion of build artifacts inside the existing owned cache root.

## In scope

Provider deletion, root/marker preservation, adoption validation, path-race protection, partial results, and historical quarantine compatibility.

## Out of scope

Scheduling changes, lifecycle tracking, build flags, and UI.

## Acceptance

- Eligible cleanup deletes build data without a new quarantine entry or active_quarantine blocker.
- The ownership marker and root `fuzz` corpus remain untouched; fuzz bytes are excluded from threshold accounting.
- Symlinks, same-device and cross-device mounts, directory replacement, and unowned paths cannot redirect deletion.
- Concurrent writes, cancellation, and partial errors stay bounded and report only successful removal.
- Threshold discovery resumes across bounded passes, and deletion makes bounded progress after the threshold is established.

## Verification

Use TDD for changed logic. Commands start from the repository root.

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/system/storage/gocache -count=1)
(cd apps/backend && go test -race -tags fts5 ./internal/backendapp -run 'Test.*(GoCache|Quarantine)' -count=1)
```

Use real temporary trees and outside-root sentinels. Cover root replacement, symlink/junction where supported, missing entries, partial unlink errors, cancellation, and continuous writes. Preserve historical quarantine restore/delete tests.

## Files likely touched

- `apps/backend/internal/system/storage/gocache/provider.go`
- `apps/backend/internal/system/storage/gocache/direct_cleanup_test.go (new)`
- `apps/backend/internal/backendapp/storage_maintenance.go`
- `apps/backend/internal/backendapp/storage_quarantine_controller.go`

## Dependencies

None.

## Risks

Preserve path safety and the exact activity-admission scope. Busy deletion intentionally can fail active builds.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/system-page/requirements/go-cache-reclamation.md)
- [Design](../../specs/system-page/system-design/go-cache-reclamation.md)
- [Plan](plan.md), including compatibility matrix and existing regression suites.

## Results

Implemented bounded in-place deletion with ownership-marker and fuzz-corpus
preservation, mount-aware filesystem-boundary and symlink skipping, root/path
identity checks, cancellation, resumable threshold discovery, bounded deletion
progress, partial results, and unknown remaining-byte reporting. Historical
quarantine controllers remain unchanged and the Go-cache provider no longer
creates or reads quarantine intents.

Validation passed:

- `(cd apps/backend && go test -race -tags fts5 ./internal/system/storage/gocache -count=1)`
- `(cd apps/backend && go test -race -tags fts5 ./internal/backendapp -run 'Test.*(GoCache|Quarantine)' -count=1)`
