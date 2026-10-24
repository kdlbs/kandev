---
id: "02-theme-lifecycle"
title: "Import and no-runtime package lifecycle"
status: done
wave: 2
depends_on:
  - "01-theme-format"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-THEMES-002
  - REQ-PLUGINS-THEMES-003
acceptance_criteria:
  - AC-PLUGINS-THEMES-002.1
  - AC-PLUGINS-THEMES-002.2
  - AC-PLUGINS-THEMES-002.3
  - AC-PLUGINS-THEMES-002.4
  - AC-PLUGINS-THEMES-003.3
  - AC-PLUGINS-THEMES-003.4
system_design:
  - ../../specs/plugins/system-design/theme-packages.md
---

# Task 02: Import and no-runtime package lifecycle

## Summary

Install and manage validated definitions without running a plugin. Add reviewed
native/VS Code import APIs and an authoritative inventory that connected clients
can reconcile across updates, restarts and removal.

## In scope

Prepare/commit with bounded expiring immutable reviews and idempotent confirmation;
raw import wrapping/dedup; auth/admin/admission gates; existing record/install
lock integration; all no-runtime lifecycle paths including Sync, startup and
updates; atomic snapshot publication and old-version preservation. Add inventory,
content/schema read routes, authenticated boot descriptor projection, revision
notifications and stable error codes. Existing managed-plugin lifecycle stays intact.

## Out of scope

Marketplace category/build pipeline, frontend review UI, appearance application,
a personal theme library, automatic publication, and new permission grants.

## Acceptance

1. Only instance admins can mutate shared inventory; confirmation binds exact
   prepared bytes/conversion, base/metadata, principal, expiry and predecessor.
   Repeated confirmation/import does not duplicate or overwrite a different pack.
2. Install/update/enable/disable/uninstall/startup/Sync/auto-update never start a
   process or load a UI bundle for a theme. Failure retains the old record/content;
   successful revocation removes eligibility, not client preference authority.
3. Inventory/boot/content observations carry identity/revision/digest and reflect
   only committed valid records; successful changes notify clients, and disabled
   Plugins paths reject admission without side effects.

## Verification

From repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/plugins/...)
(cd apps/backend && go test -trimpath ./internal/backendapp ./internal/gateway/websocket -run Theme)
git diff --check
```

Name new tests with Theme so the focused boot/gateway command includes them.
Use narrow `bash scripts/lint-go-changed` with every changed Go path recorded
from this order. Test prepared-state limits, cancellation, stale actor/version,
double commit, storage failure, no-runtime mocks, restart and old binary regression.

## Files likely touched

- `apps/backend/internal/plugins/themes_handlers.go`, `service_themes.go`,
  review storage/snapshot helpers and focused new tests.
- `apps/backend/internal/plugins/service_install.go`, `service_lifecycle.go`,
  `service_sync.go`, update/auto-update and pruning paths, existing tests.
- `apps/backend/internal/plugins/handlers.go`, `store/` if record projection needs it.
- `apps/backend/internal/backendapp/boot_state.go` and new theme projection/test.
- `apps/backend/internal/gateway/websocket/` theme notification wiring/test and
  existing event bus contracts.

## Dependencies

Task 01 supplies validated canonical data, package profile and conversion.
No theme-specific executable fixtures are allowed.

## Risks

Publish-after-persist failure, replacing installed state from a stale review,
legacy activation assumptions and forgotten startup/update paths.

## Parallelism

`sequential`

## Inputs

Package design: APIs, lifecycle, admission and inventory projection. Existing
install/admin-route/lifecycle failure tests and authenticated boot patterns.

## Results

Implemented prepared import/commit, no-runtime lifecycle, validated snapshots, authenticated boot projection and WS invalidation.

- `go test -trimpath ./internal/plugins/...`: passed.
- `go test -trimpath ./internal/backendapp ./internal/gateway/websocket -run Theme -count=1`: passed.
- `go test -trimpath -race ./internal/plugins -run Theme -count=1`: passed.
- Focused install/theme regression tests after lifecycle helper extraction: passed.
- Narrow changed-file Go lint: passed (0 issues).
