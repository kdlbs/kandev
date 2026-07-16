---
id: task-16
title: Manifest ui.bundle / ui.styles fields
status: done
wave: A
depends_on: [task-01]
plan: docs/plans/plugins/plan.md
---

# Manifest ui.bundle / ui.styles fields

## Title
Extend the plugin manifest to declare a frontend JS bundle for native UI plugins.

## Inputs
- Existing `internal/plugins/manifest/` (task-01): `Manifest`, `UISection`, `UIPage`.
- `PLUGIN-API.md` loading model: bundle served at `/api/plugins/{id}/bundle`,
  proxied to the plugin's declared bundle path.

## Acceptance
1. `UISection` gains `Bundle string` (yaml `bundle`, json `bundle`) — a path on the
   plugin process serving an ES module, e.g. `/ui/bundle.js`; and
   `Styles []string` (optional CSS paths).
2. `Validate`: if `ui.bundle` is set it must be a root-relative path (starts `/`).
   `ui.pages` remain valid but are now optional/secondary (native routes come from
   the bundle at runtime).
3. Helper `(*Manifest) HasUIBundle() bool`.
4. Tests cover: valid bundle path, invalid (non-root-relative) bundle path, manifest
   with bundle + no pages passes.

## Files
- edit `apps/backend/internal/plugins/manifest/manifest.go`, `validate.go`
- `apps/backend/internal/plugins/manifest/manifest_test.go` (extend)

## Verification
- `go test ./internal/plugins/manifest/...`; `golangci-lint run ./internal/plugins/manifest/...` (apps/backend)

## Output contract
Report the added fields + validation rule + helper. Stay within `internal/plugins/manifest/`.

## Dependencies
task-01 (done).
