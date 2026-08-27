---
id: "01-preserve-database-path-provenance"
title: "Preserve database path provenance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-STARTUP-CONFIGURATION-PARITY-002
acceptance_criteria:
  - AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.1
  - AC-PLATFORM-STARTUP-CONFIGURATION-PARITY-002.3
system_design:
  - ../../specs/platform/system-design/startup-database-selection-continuity.md
---

# Task 01: Preserve Database Path Provenance

## Summary

Keep a launcher-derived default database path classified as a default when the backend reloads configuration. Preserve explicit environment, YAML, and dev-isolated paths and add the regression seam needed by persistence selection.

## In scope

- Stop synthesizing a public database-path override for the normal derived default.
- Preserve ambient explicit environment values, selected YAML configuration, and dev managed overrides.
- Add launcher tests for child source provenance and resolved-path display behavior.

## Out of scope

- Legacy database inspection or copying.
- SQLite schema and backup behavior.
- Public documentation.

## Acceptance

- A default launch child resolves `<home>/data/kandev.db` with `SourceDefault` and no generated `KANDEV_DATABASE_PATH`.
- Explicit environment and YAML paths retain their existing precedence and source.
- Dev-mode repository isolation still supplies its managed database path.

## Verification

```bash
cd apps/backend && go test ./internal/launcher -run '^(Test.*DatabasePath|TestBackendEnv.*Database)' -count=1
cd apps/backend && go test ./internal/launcher -count=1
```

## Files likely touched

- `apps/backend/internal/launcher/env.go`
- `apps/backend/internal/launcher/bootstrap_test.go`
- `apps/backend/internal/launcher/database_path_test.go`

## Dependencies

None.

## Risks

- Removing the generated override must not remove an inherited operator override or the dev launcher's later managed override.

## Parallelism

`sequential`

## Inputs

- `REQ-PLATFORM-STARTUP-CONFIGURATION-PARITY-002`
- `docs/specs/platform/system-design/startup-database-selection-continuity.md`
- ADR `2026-08-20-startup-configuration-source-parity`

## Results

- Added `TestBackendEnvDoesNotSynthesizeDefaultDatabasePath` and
  `TestBackendEnvPreservesExplicitEnvironmentDatabasePath`.
- Updated `backendEnvForConfig` so only an explicit environment database source
  is emitted as `KANDEV_DATABASE_PATH`; default and YAML sources remain typed.
- Focused check passed: `go test ./internal/launcher -run
  '^(Test.*DatabasePath|TestBackendEnv.*Database)' -count=1`.
- Package check passed with inherited harness variables cleared:
  `unset KANDEV_INTERNAL_CONFIG_FILE KANDEV_DATABASE_PATH; go test
  ./internal/launcher -count=1`.
