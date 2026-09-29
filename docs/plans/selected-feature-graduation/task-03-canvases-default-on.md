---
id: selected-feature-graduation-03
title: Promote canvases
status: pending
wave: 3
depends_on: ["07-authoring-bundle-and-scaffold"]
plan: plan.md
requirements:
  - REQ-CANVASES-DEFAULT-AVAILABILITY-001
acceptance_criteria:
  - AC-CANVASES-DEFAULT-AVAILABILITY-001.1
  - AC-CANVASES-DEFAULT-AVAILABILITY-001.3
  - AC-CANVASES-DEFAULT-AVAILABILITY-001.4
system_design:
  - ../../specs/canvases/system-design/default-availability.md
---

# Promote canvases

## Summary

Make canvas routes, authoring, and task/workspace surfaces available by default
while retaining a working kill switch for one stable release.

## In scope

- Confirm the pending [canvas authoring-bundle work order](../plugin-backed-canvases-ux-follow-up/task-07-authoring-bundle-and-scaffold.md)
  and its parent plan are complete before starting promotion. That work remains
  owned by its existing package.
- Validate real publication, permission review, task/workspace data scope,
  iframe isolation, custom-origin hosting, runtime startup failure, and rollback.
- Set `KANDEV_FEATURES_CANVASES` true in every shipped profile; retain the
  backend/frontend disabled paths and override behavior. Update public status,
  canvas, security, and configuration pages for the new default.

## Out of scope

- Retiring the flag, auto-approving later grants, or requiring marketplace
  distribution before normal canvas use.

## Acceptance

1. A normal production-profile install can create and open a canvas without an
   override; an explicit false override still blocks the feature before side
   effects.
2. Package, grant, isolation, and failure-recovery tests pass with the feature
   available by default.
3. Desktop and phone users can reach the existing canvas journey, including
   loading and failed-runtime states, with no horizontal overflow on phone.

## ASCII UI preview

`UI-01` and `UI-02` from [the plan](plan.md#ascii-ui-preview):

```text
Desktop: Workspace sidebar > Canvases; Task > Canvas panel [Create canvas]
Phone:   Workspace navigation drawer > Canvases > canvas destination
```

The entry points and primary action are required. The phone uses its existing
navigation and dedicated destination, with one scroll owner and touch controls;
the text is illustrative and localized in the product.

## Verification

```sh
cd apps/backend && go test ./internal/profiles ./internal/runtimeflags ./internal/canvas ./internal/plugins ./internal/backendapp
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

- `apps/backend/internal/profiles/profiles.yaml` and profile/registry tests.
- `docs/public/canvases.md`, `docs/public/security.md`,
  `docs/public/feature-status.md`, `docs/public/configuration.md`.
- Existing canvas E2E and mobile specs if the production-default path needs
  a no-override scenario.

## Dependencies and risks

The authoring-bundle work order is still in progress. Its completion is an
external dependency, not part of this work order. Default exposure increases
the reach of agent-authored
code, so package isolation and grant review are promotion gates. This task
retains a working kill switch.

## Results

Pending.
