---
id: "02-export-preparation"
title: "Prepare canvas exports"
status: done
wave: 2
depends_on:
  - "01-portable-packages"
plan: "plan.md"
requirements:
  - REQ-CANVASES-MARKETPLACE-001
  - REQ-CANVASES-MARKETPLACE-003
  - REQ-CANVASES-MARKETPLACE-006
acceptance_criteria:
  - AC-CANVASES-MARKETPLACE-001.2
  - AC-CANVASES-MARKETPLACE-001.3
  - AC-CANVASES-MARKETPLACE-003.1
  - AC-CANVASES-MARKETPLACE-003.2
  - AC-CANVASES-MARKETPLACE-003.4
  - AC-CANVASES-MARKETPLACE-006.4
system_design:
  - ../../specs/canvases/system-design/marketplace-sharing.md
---

# Task 02: Prepare canvas exports

## Summary

Prepare both downloadable formats from one active immutable release. Introduce
the bounded temporary preparation store that the install review will also use.

## In scope

- User/workspace-bound preparations, 15-minute expiry, admission accounting,
  request leases, cancel, startup cleanup, and safe failure cleanup.
- Export preparation with metadata, inventory and size preview. No screenshot input.
- Authenticated bundle/source downloads with safe headers.
- Reauthorization and active-release checks before prepare/download.
- Stable error codes, content-free diagnostics, and no executor dependence.

## Out of scope

Permanent share-profile settings, external publication, installation mutations,
frontend forms, and changes to canvas runtime state.

## Acceptance

- Export preparation and both downloads refer to the same validated snapshot;
  form/release changes invalidate it and unauthorized requests disclose nothing.
- Source/state exclusion, screenshot-free preparation, expiry, cancellation, interrupted
  streaming, quotas, and restart cleanup leave the active release unchanged.
- Preparation routes are absent when canvases are disabled; download access
  requires current human workspace authority even if an ID is known.

## Verification

```bash
(cd apps/backend && rtk go test -race ./internal/canvas -run 'TestCanvasExport|TestCanvasPreparation' -count=1)
(cd apps/backend && rtk go test -race ./internal/backendapp -run 'TestCanvasExport|TestCanvasPreparation|TestCanvasDistributionFeatureOff' -count=1)
rtk git diff --check
```

## Files likely touched

New:
- `apps/backend/internal/canvas/distribution.go`, `distribution_preparation.go`
- `apps/backend/internal/canvas/distribution_export.go`, `distribution_export_test.go`
- `apps/backend/internal/canvas/distribution_preparation_test.go`
- `apps/backend/internal/backendapp/canvas_distribution_routes.go`, `canvas_distribution_routes_test.go`

Existing integration points:
- `apps/backend/internal/backendapp/canvas_routes.go` and service composition
- `apps/backend/internal/plugins/webapp/package.go` artifact read interface

## Dependencies

Task 01's validated package and archive writers.

## Risks

Do not mistake opaque IDs for authorization. Cleanup must not race a live
download or delete a shared immutable artifact. Bound compressed outputs and
multipart input before memory allocation. Avoid collecting task workspace files.

## Parallelism

`sequential`

## Inputs

- Design: Validation and bounded preparation, Export preparation, HTTP contracts.
- `canvasEditService.loadEditableCanvas` for immutable artifact authorization.
- Existing canvas route/feature-off tests and safe attachment response patterns.

## Results

Implemented user-bound, expiring export preparations with quota and cleanup
handling, immutable active-release snapshots, metadata/inventory review, and
authenticated bundle/source downloads. The export path is screenshot-free and
does not depend on the executor.

Verification: the canvas export/preparation race tests passed 3 tests, the
desktop sharing E2E passed, and the mobile sharing E2E passed. The full relevant
backend suite and SQL guard also passed.
