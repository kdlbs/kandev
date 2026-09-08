---
id: "02-plugin-web-app-package-foundation"
title: "Plugin web-app package foundation"
status: done
wave: 2
depends_on:
  - "01-canvas-release-gate"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ISOLATED-WEB-APPS-001
  - REQ-PLUGINS-ISOLATED-WEB-APPS-002
  - REQ-PLUGINS-ISOLATED-WEB-APPS-008
  - REQ-PLUGINS-ISOLATED-WEB-APPS-009
  - REQ-PLUGINS-ISOLATED-WEB-APPS-010
acceptance_criteria:
  - AC-PLUGINS-ISOLATED-WEB-APPS-001.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-001.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-001.5
  - AC-PLUGINS-ISOLATED-WEB-APPS-002.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-002.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-002.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-002.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-002.5
  - AC-PLUGINS-ISOLATED-WEB-APPS-002.6
  - AC-PLUGINS-ISOLATED-WEB-APPS-008.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-008.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-008.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-008.5
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-009.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-010.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-010.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-010.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-010.5
system_design:
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 02: Plugin web-app package foundation

## Summary

Add web-application manifests, scoped plugin instances, immutable releases,
grants, state, artifact limits, and durable cleanup. Preserve current plugin
behavior through an implicit global instance.

## In scope

- Add `ui.web_apps` parsing, validation, and JSON projection.
- Add safe static-package validation and immutable artifact storage.
- Add instance, release, grant, state, reservation, and cleanup-job storage.
- Add replayable SQLite and PostgreSQL migrations.
- Enforce workspace and installation artifact budgets.
- Reconcile missing, changed, and unsafe artifacts before runtime registration.
- Add package, storage, cleanup, and content-free metric tests.

## Out of scope

- Runtime URLs, iframe rendering, browser data routes, and canvas lifecycle.

## Acceptance

- Invalid packages create no valid release or extracted artifact.
- Concurrent byte reservations cannot exceed a storage limit.
- A missing or changed retained artifact becomes unavailable before execution.

## Verification

```bash
cd apps/backend && go test ./internal/plugins/... ./pkg/pluginsdk/... ./internal/persistence/... ./internal/system/backups/...
```

## Files likely touched

- `apps/backend/internal/plugins/manifest/**`
- `apps/backend/internal/plugins/pkgtar/**`
- `apps/backend/internal/plugins/webapp/**`
- `apps/backend/internal/plugins/instances/**`
- `apps/backend/internal/plugins/state/**`
- plugin artifact cleanup and storage ownership providers
- `apps/backend/internal/backendapp/**`

## Dependencies

- Task 01 provides the release gate for all later entry points.

## Risks

- A path or link error can escape immutable storage.
- A count and byte reservation can diverge after a failed transaction.
- A database-only restore can leave metadata without artifacts.

## Parallelism

`sequential`

## Inputs

- Package, instance, artifact, recovery, compatibility, and observability
  design sections.
- Current manifest, package extraction, storage, backup, and migration tests.

## Results

Completed on 2026-08-27. Added `ui.web_apps` manifest declarations and
validation, including per-app exact HTTPS `network_origins`, the bounded static
package validator, digest-addressed immutable artifact storage, startup
reconciliation, scoped instance/release/grant records, atomic canvas and
storage admission, and durable cleanup inventory.
The documented plugin, SDK, persistence, and backup package tests pass.
