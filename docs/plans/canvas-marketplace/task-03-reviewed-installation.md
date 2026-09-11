---
id: "03-reviewed-installation"
title: "Install reviewed canvas bundles"
status: done
wave: 3
depends_on:
  - "02-export-preparation"
plan: "plan.md"
requirements:
  - REQ-CANVASES-MARKETPLACE-001
  - REQ-CANVASES-MARKETPLACE-004
  - REQ-CANVASES-MARKETPLACE-006
acceptance_criteria:
  - AC-CANVASES-MARKETPLACE-001.1
  - AC-CANVASES-MARKETPLACE-001.4
  - AC-CANVASES-MARKETPLACE-004.1
  - AC-CANVASES-MARKETPLACE-004.2
  - AC-CANVASES-MARKETPLACE-004.3
  - AC-CANVASES-MARKETPLACE-004.4
  - AC-CANVASES-MARKETPLACE-004.5
  - AC-CANVASES-MARKETPLACE-004.6
  - AC-CANVASES-MARKETPLACE-006.4
system_design:
  - ../../specs/canvases/system-design/marketplace-sharing.md
---

# Task 03: Install reviewed canvas bundles

## Summary

Install an inspected static canvas as an independent workspace instance after
explicit permission approval. Preserve reviewed bytes and retry identity across
the database transaction.

## In scope

- Upload and direct-link preparation, authoritative package/permission review without required images,
  expected digest, bounded HTTPS fetch, redirect/DNS destination validation.
- Catalog-reference resolver interface; Task 04 wires actual catalog entries.
- `InstallPrepared`, workspace admission, atomic instance/release/grant creation,
  receipt/provenance persistence, and existing lifecycle invalidation.
- Idempotent retry, concurrent confirmation, restart replay, deletion handling,
  safe artifact compensation, and native-install authority isolation.
- Additive receipt schema, migration/replay fixtures, persistence descriptor
  updates where the existing canvas store's conformance contract requires them.

## Out of scope

Repository cloning, provider credentials, replacing/updating existing canvases,
frontend review controls, and native plugin auto-update changes.

## Acceptance

- Upload/link installs execute only inspected bytes after current workspace
  authorization and full permission approval, with no instance before confirm.
- Concurrent or retried confirmation creates one instance; a new review creates
  an independent copy. Receipt, provenance, grants, and release survive restart.
- Invalid URL/digest/package, revoked workspace, quota/DB failure, and disabled
  routes leave existing data unchanged; supported schema replay checks pass.

## Verification

```bash
(cd apps/backend && rtk go test -race ./internal/canvas ./internal/plugins/instances -count=1)
(cd apps/backend && rtk go test -race ./internal/backendapp -run 'TestCanvasInstall|TestCanvasDistribution' -count=1)
(cd apps/backend && rtk go run ./cmd/sqlguard ./internal)
(cd apps/backend && rtk go test -race ./internal/persistence/storeconformance -count=1)
rtk git diff --check
```

Run the same targeted canvas/instance tests with `KANDEV_TEST_POSTGRES_DSN`
set when the test database is available. Record skipped PostgreSQL evidence
explicitly; do not report SQLite replay as cross-dialect evidence.

## Files likely touched

- `apps/backend/internal/canvas/distribution_install.go`, `distribution_install_test.go` (new)
- `apps/backend/internal/canvas/distribution_download.go`, `distribution_download_test.go` (new)
- `apps/backend/internal/canvas/repository.go`, `service.go`, `types.go`
- `apps/backend/internal/plugins/instances/store.go` and focused transaction tests
- `apps/backend/internal/backendapp/canvas_distribution_routes.go`, route tests
- Existing canvas schema registration, storage cleanup, and conformance fixtures

## Dependencies

Tasks 01-02. Use a narrow catalog resolver interface with fixture data until Task 04.

## Risks

A sequence of independently committed create/publish/approve calls is not atomic.
Use the existing store transaction boundary and receipt uniqueness. PostgreSQL
locking must cover admission and concurrent retries. DNS checks must bind to the
actual connection destination, not only an earlier lookup.

## Parallelism

`sequential`

## Inputs

- Design: Install preparation, Commit and persistence, Authorization.
- Existing `canvas.PublishPackage`, `instances.Store` transaction/cleanup tests.
- `canvasHTTPHandler.authorizeWorkspace`, authentication scoping guidance.
- Existing native-plugin install tests as a compatibility boundary.

## Results

Implemented upload, direct HTTPS link, and catalog-reference preparation;
server-side digest/package/permission review; bounded public-destination
fetching; atomic workspace install integration; provenance and receipt
persistence; and retry-safe confirmation.

Verification: the canvas installation/export race selection passed 3 tests, the
full relevant backend suite passed 1,587 tests across 8 packages, and SQL guard
passed. Existing native plugin installation paths remained covered by the
16-test plugin E2E regression.

Review remediation: production confirmation now uses the canvas service's
single transaction for instance, release, grants, and receipt ownership. It
also reauthorizes retry paths, fails closed on receipt-store errors, and
compensates a newly-created artifact when the transaction cannot commit.
