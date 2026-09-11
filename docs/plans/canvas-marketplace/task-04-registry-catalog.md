---
id: "04-registry-catalog"
title: "Publish canvas catalog entries"
status: done
wave: 4
depends_on:
  - "03-reviewed-installation"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-004
  - REQ-CANVASES-MARKETPLACE-005
  - REQ-CANVASES-MARKETPLACE-006
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-002.1
  - AC-PLUGINS-MARKETPLACE-002.2
  - AC-PLUGINS-MARKETPLACE-002.3
  - AC-PLUGINS-MARKETPLACE-002.4
  - AC-CANVASES-MARKETPLACE-002.1
  - AC-CANVASES-MARKETPLACE-002.2
  - AC-CANVASES-MARKETPLACE-002.3
  - AC-CANVASES-MARKETPLACE-002.4
  - AC-CANVASES-MARKETPLACE-004.1
  - AC-CANVASES-MARKETPLACE-005.1
  - AC-CANVASES-MARKETPLACE-005.2
  - AC-CANVASES-MARKETPLACE-005.3
  - AC-CANVASES-MARKETPLACE-005.4
  - AC-CANVASES-MARKETPLACE-006.4
system_design:
  - ../../specs/plugins/system-design/marketplace.md
  - ../../specs/canvases/system-design/marketplace-sharing.md
---

# Task 04: Publish canvas catalog entries

## Summary

Extend the existing repository registry and catalog with validated canvas entries
and registry-owned preview URLs for canvases and plugins. Wire registry selection into the same reviewed
installation service.

## In scope

- Optional pointer kind, exact release asset selection, production inspector,
  matching package/repository/version and required hash/source.
- Shared `previews: [{url, alt}]` on official/custom entries, required for
  canvases and optional for plugins; preserve URL order into the generated index.
- Additive catalog descriptor, kind filter, safe degraded-source behavior.
- No package screenshot extraction, image fetching, or Pages media staging.
- Per-workspace instance annotations after authorization and source-cache lookup.
- Catalog source/ID/version/digest resolution for Task 03's prepare endpoint.
- PR validation with read-only job permissions and no package code execution.

## Out of scope

Submitting a real canvas registry entry, publishing this branch, changing native
plugin package rules, a new catalog service, or automated repository creation.

## Acceptance

- Registry tests reject canvas listings without image URLs and packages without
  source, mismatched identity/version, ambiguous assets, or invalid archives.
  Package metadata/hash comes from the archive; images come from the registry.
- Catalog preserves preview URL order independently of package versions; failed
  sources and old native entries remain
  compatible. Workspace annotations cannot leak through shared cache entries.
- Catalog installation checks selected source/version/hash server-side and
  enters the existing review path. No production canvas executes during indexing.

## Verification

```bash
(cd apps/backend && rtk go build -o bin/canvas-package ./cmd/canvas-package)
rtk bash -lc 'KANDEV_CANVAS_PACKAGE_INSPECTOR=apps/backend/bin/canvas-package node --test plugin-registry/build-index.test.mjs plugin-registry/canvas-index.test.mjs'
(cd apps/backend && rtk go test -race ./internal/plugins/marketplace ./cmd/canvas-package -count=1)
(cd apps/backend && rtk go test -race ./internal/plugins -run Marketplace -count=1)
(cd apps/backend && rtk go test -race ./internal/backendapp -run 'TestCanvasInstall|TestCanvasDistribution' -count=1)
rtk git diff --check
```

The new Node test file uses local fixture responses and the built inspector.
Do not run a publishing workflow or contact external repositories for test data.

## Files likely touched

- `plugin-registry/schema.json`, `build-index.mjs`, `build-index.test.mjs`
- `plugin-registry/canvas-index.mjs`, `canvas-index.test.mjs` (new)
- `.github/workflows/plugin-registry-index.yml`
- `apps/backend/internal/plugins/marketplace/types.go`, `catalog.go`
- `apps/backend/internal/plugins/marketplace/canvas_catalog_test.go` (new)
- `apps/backend/internal/plugins/marketplace_handlers.go`, handler tests
- `apps/backend/internal/backendapp/canvas_distribution_routes.go` resolver wiring
- Canvas receipt projection in `apps/backend/internal/canvas/repository.go`

## Dependencies

Tasks 01-03. The inspector must be available on the trusted registry base before
accepting the first contributor canvas pointer; the feature PR itself only adds
local test fixtures and runs candidate-code tests without publishing privileges.

## Risks

Do not parse nested canvas YAML with the current shallow regex helper. Never
choose a source archive because it is the first tarball. Preserve native catalog
semantics and ensure the built inspector is from trusted registry code when
processing contributor pointers. Media/index deployment must stay coherent.

## Parallelism

`sequential`

## Inputs

- Design: Registry publication, Catalog projection, Install preparation.
- Proposed complete schema: `registry-entry.schema.json` beside this work order.
- Existing `buildEntry`, `parseManifestFields`, `build-index.test.mjs`.
- Existing marketplace catalog/source-store tests and Pages workflow.

## Results

Implemented canvas registry entries, ordered shared previews, exact release
asset inspection, archive digest enforcement, kind-aware catalog projection,
degraded-source handling, and catalog installation resolution. Native plugin
entries remain compatible and may omit previews.

Verification: the registry build-index suite passed 11 tests, the proposed
schema parsed successfully, the relevant marketplace/backend suite passed, and
desktop/mobile marketplace E2E passed 2 tests total. The existing plugin
marketplace regression passed 16 tests.
