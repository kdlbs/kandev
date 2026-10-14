---
id: selected-feature-graduation-06
title: Retire canvas flag
status: pending
wave: 6
depends_on:
  - selected-feature-graduation-03
plan: plan.md
requirements:
  - REQ-CANVASES-DEFAULT-AVAILABILITY-001
acceptance_criteria:
  - AC-CANVASES-DEFAULT-AVAILABILITY-001.2
  - AC-CANVASES-DEFAULT-AVAILABILITY-001.3
  - AC-CANVASES-DEFAULT-AVAILABILITY-001.4
  - AC-CANVASES-DEFAULT-AVAILABILITY-001.5
system_design:
  - ../../specs/canvases/system-design/default-availability.md
---

# Retire canvas flag

## Summary

After a stable default-on release, make canvas service and UI composition
unconditional while preserving every permission and isolation check.

## In scope

- Remove the release gate from canvas service construction, MCP authoring
  capability, HTTP/WS/SSE routes, notifications, task panels, workspace
  settings, sidebar, and phone navigation. Keep existing authorization,
  package, grant, and runtime checks.
- Remove the live config/profile/registry/frontend identity; append the exact
  retired key/environment pair. Seed false environment and stored overrides in
  a contract test and prove they cannot disable canvases.
- Remove stale feature-toggle instructions from public canvas, security,
  configuration, and feature-status docs.

## Out of scope

- A new permission model, marketplace requirement, package format, or canvas
  data migration.

## Acceptance

1. An authorized user can create, publish, open, and manage canvases without a
   feature override; unauthorized, ungranted, invalid, or failed-runtime paths
   still fail by their existing rules.
2. The old false identities are inert and absent from active feature surfaces;
   the key/environment pair is reserved in the retired registry.
3. Desktop and phone journeys, including loading/error states and review
   controls, remain usable on a normal production-profile installation.

## ASCII UI preview

`UI-01` and `UI-02` from [the plan](plan.md#ascii-ui-preview):

```text
Desktop: Workspace sidebar > Canvases; Task > Canvas panel [Create canvas]
Phone:   Workspace navigation drawer > Canvases > canvas destination
```

Phone uses the existing direct destination and touch controls. No permission
review action becomes hidden behind hover or a desktop-only panel.

## Verification

```sh
cd apps/backend && go test ./internal/canvas ./internal/plugins ./internal/backendapp ./internal/orchestrator ./internal/runtimeflags ./internal/common/config ./internal/profiles
cd apps && pnpm --filter @kandev/web exec vitest run lib/state/slices/features/features-contract.test.ts
make -C apps/backend lint
cd apps/web && pnpm run typecheck && pnpm run lint
git diff --check
```

After building backend, web E2E bundle, and E2E plugin package:

```sh
cd apps/web && pnpm e2e:run --project=chromium tests/canvas/plugin-canvas.spec.ts tests/canvas/canvas-host-origins.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts
```

## Files likely touched

- `apps/backend/internal/backendapp/services.go`, `main.go`,
  `canvas_routes.go`, and orchestrator canvas capability wiring.
- `apps/web/src/spa-routes.tsx`, sidebar/phone canvas navigation, task canvas
  panel, workspace settings, and feature slice/types.
- `apps/backend/internal/runtimeflags/registry.go`, config/catalog, profile
  YAML, and exact-key contract tests.
- `docs/public/canvases.md`, `docs/public/security.md`,
  `docs/public/configuration.md`, `docs/public/feature-status.md`.

## Dependencies and risks

Requires Task 03 to have shipped in a stable release with permission and
isolation evidence. Removing the toggle does not authorize new data access or
grant approval. Old explicit false values cease to disable the feature; the
release note must say so.

## Results

Pending.
