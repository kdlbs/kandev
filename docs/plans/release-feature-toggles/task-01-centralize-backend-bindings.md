---
id: "01-centralize-backend-bindings"
title: "Centralize backend flag bindings"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/feature-toggles.md"
---

# Task 01: Centralize backend flag bindings

- **Acceptance:** Each runtime flag's key, environment variable, config reader,
  and config applier are owned by one internal registry registration while the
  public metadata/API shape remains unchanged.
- **Acceptance:** `OptionsFromConfig`, `ValuesFromConfig`, and
  `ApplyStatesToConfig` contain no per-feature map or switch, and Debug mode's
  implied environment behavior remains intact.
- **Acceptance:** Generic tests fail when a `FeaturesConfig` field lacks a
  registry/profile binding or when its binding mutates the wrong field.
- **Verification:** Red first with the completeness/round-trip test, then run
  `make -C apps/backend test`; run `make -C apps/backend lint` from the repo
  root.
- **Files likely touched:**
  - `apps/backend/internal/runtimeflags/registry.go`
  - `apps/backend/internal/runtimeflags/config.go`
  - `apps/backend/internal/runtimeflags/registry_test.go`
  - `apps/backend/internal/runtimeflags/config_test.go`
  - `apps/backend/internal/common/config/config_test.go`
- **Dependencies:** None.
- **Parallelism:** `parallel-safe` with Task 02 only; files are disjoint and
  there is no shared generated contract, schema, lockfile, or package config.
- **Inputs:** Feature Toggles spec `What`, `Persistence guarantees`, and
  `Scenarios`; ADR-2026-08-01-release-toggle-gating-contract; existing
  `runtimeflags` service/handler/store tests.
- **Output contract:** Report changed files, exact test command and counts,
  blockers/risks, external side effects (`None` expected), then update this task
  and `plan.md` statuses/results in the same conversation.

## Results

- Changed `apps/backend/internal/runtimeflags/registry.go` to keep metadata,
  typed config readers, and typed config appliers in one internal registration.
- Changed `apps/backend/internal/runtimeflags/config.go` to iterate those
  registrations for environment values, runtime values, and persisted state.
  Debug implied-environment behavior remains in its named applier.
- Added reflective registry/profile/config round-trip coverage and generic
  `FeaturesConfig` JSON/tag coverage.
- Review remediation made config, registry, and profile key comparison exact in
  both directions; validates registration metadata and binding isolation; and
  reserves graduated key/environment identities in an append-only tombstone
  set. The graduated plugins identity is the first tombstone.
- Review verification: `go test ./internal/runtimeflags ./internal/profiles`
  from `apps/backend` passed (37 tests across 2 packages); the full backend
  suite passed, and backend lint reported 0 issues.
- Verification: `go test ./internal/runtimeflags ./internal/common/config ./internal/profiles`
  from `apps/backend` passed (91 tests across 3 packages), `make -C apps/backend
  test` passed, and `make -C apps/backend lint` passed with 0 issues.
- External side effects: none.
