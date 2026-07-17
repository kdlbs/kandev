---
id: task-10
title: plugins feature flag (backend + frontend plumbing)
status: done
wave: A
depends_on: []
plan: docs/plans/plugins/plan.md
---

# plugins feature flag

## Title
Add a `plugins` feature flag across profiles.yaml, backend config, the runtime
flag registry, and the frontend feature-flag types — default prod OFF, dev/e2e ON.

## Inputs
- Pattern reference (`office` flag): `docs/decisions/0007-runtime-feature-flags.md`.
  Concrete touch points found in repo:
  - `profiles.yaml` `features:` block (`KANDEV_FEATURES_OFFICE` with prod/dev/e2e).
  - `apps/backend/internal/common/config/config.go:174` `FeaturesConfig` (add
    `Plugins bool mapstructure:"plugins" json:"plugins"`) + defaults (~line 333).
  - `apps/backend/internal/runtimeflags/config.go:31` (`"features.plugins": cfg.Features.Plugins`
    map entry + the apply switch ~line 40).
  - `apps/backend/internal/runtimeflags/registry.go` (add a registry entry mirroring
    the `features.office` entry: Key `features.plugins`, Label, Description, Risk).
  - Frontend: `apps/web/lib/state/slices/features/types.ts` (`FeatureFlags` add
    `plugins: boolean`), `features-slice.ts` (default `plugins: false`),
    `apps/web/app/actions/features.ts` / `lib/api/domains/features-api.ts` /
    boot payload mapping if features are hydrated there (grep `office` to find every
    site and add `plugins` alongside).
  - Backend boot payload that ships features to the SPA (grep `Features.Office` in
    `internal/webapp/`).

## Acceptance
1. `KANDEV_FEATURES_PLUGINS` in profiles.yaml: prod `"false"`, dev `"true"`, e2e `"true"`.
2. `FeaturesConfig.Plugins` parsed; default false.
3. Runtime flag registry entry present (togglable in Settings > System).
4. Frontend `FeatureFlags.plugins` typed + defaulted false + hydrated from backend
   wherever `office` is.
5. Existing feature tests updated; add a slice test asserting default `plugins: false`.

## Files
- `profiles.yaml`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/config.go`, `registry.go`
- `apps/backend/internal/webapp/payload.go` (+ wherever office feature is mapped)
- `apps/web/lib/state/slices/features/types.ts`, `features-slice.ts`, `features-slice.test.ts`
- `apps/web/app/actions/features.ts`, `apps/web/lib/api/domains/features-api.ts` (as grep reveals)

## Verification
- `go test ./internal/runtimeflags/... ./internal/common/config/... ./internal/webapp/...` (apps/backend)
- `cd apps/web && pnpm run typecheck && pnpm test -- features`

## Output contract
Report: every file touched, and confirm `office`-parallel sites all got a `plugins`
sibling (list them). Do not implement the settings page (task-12).

## Dependencies
None. (task-09 may add the `Plugins` config field first; if present, don't duplicate.)
