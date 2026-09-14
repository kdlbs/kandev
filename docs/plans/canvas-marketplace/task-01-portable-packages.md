---
id: "01-portable-packages"
title: "Define portable canvas packages"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CANVASES-MARKETPLACE-001
  - REQ-CANVASES-MARKETPLACE-002
  - REQ-CANVASES-MARKETPLACE-006
acceptance_criteria:
  - AC-CANVASES-MARKETPLACE-001.1
  - AC-CANVASES-MARKETPLACE-001.2
  - AC-CANVASES-MARKETPLACE-001.3
  - AC-CANVASES-MARKETPLACE-001.4
  - AC-CANVASES-MARKETPLACE-002.1
  - AC-CANVASES-MARKETPLACE-006.4
system_design:
  - ../../specs/canvases/system-design/marketplace-sharing.md
---

# Task 01: Define portable canvas packages

## Summary

Define the canvas distribution profile in the existing plugin manifest and
static validator. Preserve editable source with immutable releases and expose
a reusable offline inspector for registry validation.

## In scope

- Typed distribution metadata, source modes, checksum validation, compatibility,
  exactly one canvas app, and rejection of other plugin contributions.
- Screenshot-free bundle/source validation; registry previews are not manifest fields.
- Source-only project subtree, bounded authoring transfer, immutable retention,
  Quick Chat materialization, and runtime denial of distribution paths.
- Deterministic bundle/source archive writers and production offline inspector.
- Minimal embedded authoring reference changes needed to keep source modes valid;
  Task 07 owns the full sharing instructions.

## Out of scope

HTTP downloads, registry fetching, canvas installation, UI, and native installer changes.

## Acceptance

- Distribution round-trip fixtures retain matching static/project source and
  application assets while malformed, incomplete, disguised native, and oversized
  bundles fail. No screenshot or preview URL is required.
- Runtime requests cannot read the inert source subtree through plain,
  encoded, or normalized aliases; editing retains original build inputs after
  the author executor is unavailable.
- The offline inspector and host use the same validator and a shared descriptor
  fixture. Native managed-plugin installation tests remain unchanged and pass.

## Verification

Run from repository root. Start with focused RED cases, then run these blocks
after implementation. The new test files/methods are named in the plan.

```bash
(cd apps/backend && rtk go test -race ./internal/plugins/manifest ./internal/plugins/webapp ./internal/plugins/pkgtar ./cmd/canvas-package -count=1)
(cd apps/backend && rtk go test -race ./internal/backendapp -run 'TestCanvas.*(Source|Scaffold|Edit)' -count=1)
(cd apps/backend && rtk go test -race ./internal/agentctl/server/api -run Canvas -count=1)
(cd apps/backend && rtk go test ./internal/mcp/canvasskill -count=1)
rtk git diff --check
```

## Files likely touched

Existing:
- `apps/backend/internal/plugins/manifest/manifest.go`, `validate.go`
- `apps/backend/internal/plugins/webapp/package.go`, `runtime.go`
- `apps/backend/internal/backendapp/canvas_authoring.go`, `canvas_routes.go`
- `apps/backend/internal/agentctl/server/api/workspace_canvas_source.go`
- `apps/backend/internal/mcp/canvasskill/files/references/manifest.md`

New:
- `apps/backend/internal/plugins/manifest/distribution.go`
- `apps/backend/internal/plugins/webapp/distribution.go`, `distribution_test.go`
- `apps/backend/internal/backendapp/canvas_distribution_source_test.go`
- `apps/backend/cmd/canvas-package/main.go`, `main_test.go`
- Adjacent focused archive-writer tests and shared static/project fixture files.

## Dependencies

None. Follow the existing plugin-backed canvas runtime contract.

## Risks

A file-extension exception must not broaden runtime serving. Preserve the
current package byte/file budgets, source transfer fencing, and authoring core
inventory. Build-source declarations cannot prove that arbitrary builds succeed;
fixtures must prove both supported source modes and their documented commands.

## Parallelism

`sequential`

## Inputs

- Design: Package profile, Source retention, Validation.
- Existing `webapp/package_test.go`, `runtime_test.go`, authoring/edit tests.
- `apps/backend/AGENTS.md` and both scoped agentctl AGENTS files before changes.
- Existing canvas authoring guide and one-core-read inventory contract.

## Results

Implemented the typed canvas distribution profile, strict manifest/package and
source validation, checksum coverage, deterministic bundle/source archives,
runtime source isolation, bounded source retention, and the offline inspector.
The authoring reference and fixture preserve static/project source behavior.

Verification: the package/manifest/webapp/pkgtar/inspector race suite passed 210
tests, the agentctl canvas-source race suite passed 6 tests, the backend canvas
source/scaffold/edit race suite passed 7 tests, the authoring skill suite passed
7 tests, and the relevant backend suite passed 1,587 tests across 8 packages.
