---
id: "01-conflict-diagnostic"
title: "Emit typed ownership conflict"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DESKTOP-ISOLATED-STARTUP-001
acceptance_criteria:
  - AC-DESKTOP-ISOLATED-STARTUP-001.1
  - AC-DESKTOP-ISOLATED-STARTUP-001.3
system_design:
  - ../../specs/desktop/system-design/isolated-startup.md
---

# Task 01: Emit typed ownership conflict

## Summary

Give the desktop shell reliable evidence that the backend rejected startup
because another process owns a home or database lock. Preserve the terminal's
current human-readable diagnostic.

## In scope

- Add a versioned, machine-readable diagnostic on the typed
  `ownershiplock.ConflictError` path only.
- Include a closed storage classification and the effective database path for
  the default in-home SQLite case, so desktop copy can identify the database
  without parsing the human launcher banner.
- Bound and sanitize optional owner fields, avoiding secrets and unrelated
  configuration values.
- Test home conflict, database conflict, and unrelated lock failures.

## Out of scope

- Changing lock acquisition or allowing two backends to use one target.
- Desktop UI or retry behavior.

## Acceptance

1. A real `ConflictError` emits one parseable marker with target kind, canonical
   path, storage classification, and optional owner details alongside the
   existing stderr sentence. The default SQLite case includes its path.
2. Non-conflict acquisition and configuration errors emit no marker.

## Verification

```bash
(cd apps/backend && go test ./internal/backendapp ./internal/backendapp/ownershiplock)
```

## Files likely touched

- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/main_test.go`
- `apps/backend/internal/backendapp/ownershiplock/lock.go` only if a marker
  helper needs an exported typed field

## Dependencies

None.

## Risks

The launcher forwards nested backend output. Keep the marker short enough to
survive its bounded recent-output path and avoid logging sensitive values.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/desktop/requirements/isolated-startup.md),
  [design](../../specs/desktop/system-design/isolated-startup.md), and the
  existing `ownershiplock.ConflictError` and startup tests.

## Results

Implemented the versioned `KANDEV_DESKTOP_CONFLICT_V1` marker for typed ownership conflicts only. The backend unit and subprocess tests cover home and external SQLite ownership conflicts, preserve the existing diagnostic, and reject invalid configuration and non-conflict ownership failures. The targeted backend packages pass.
