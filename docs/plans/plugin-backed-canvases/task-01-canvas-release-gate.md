---
id: "01-canvas-release-gate"
title: "Canvas release gate"
status: done
wave: 1
depends_on:
  - "00-baseline-transition"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-001.7
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 01: Canvas release gate

## Summary

Add the typed canvas runtime flag before canvas code becomes reachable. Make
the backend authoritative and keep all shipped profile defaults off.

## In scope

- Add `features.canvases` and `KANDEV_FEATURES_CANVASES`.
- Add the backend config field, runtime-flag registration, and frontend key.
- Mark the flag as restart-required.
- Add fail-closed helpers for HTTP, WebSocket, SSE, MCP, background, boot, and
  frontend entry points.
- Add enabled and disabled contract tests.

## Out of scope

- Canvas schemas, routes, tools, services, and user interface implementation.

## Acceptance

- The registry, profiles, backend config, and frontend defaults use one flag
  identity.
- All `prod`, `dev`, and `e2e` profile defaults are `false`.
- The disabled path exposes no canvas capability or side effect.

## Verification

```bash
cd apps/backend && go test ./internal/runtimeflags ./internal/common/config ./internal/profiles ./internal/backendapp
cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts
```

## Files likely touched

- `profiles.yaml`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/runtimeflags/config.go`
- `apps/backend/internal/backendapp/**`
- `apps/backend/internal/mcp/server/**`
- `apps/web/lib/state/slices/features/types.ts`
- frontend route and navigation gates

## Dependencies

Task 00 provides a clean branch from `origin/main` with only the design
package.

## Risks

- A frontend-only gate can leave direct backend entry points available.
- MCP tool registration can expose canvas tools until restart.

## Parallelism

`sequential`

## Inputs

- Implementation baseline and feature-gate design section.
- Current runtime-flag registry and feature-contract tests.

## Results

Completed on 2026-08-27. Added the single `features.canvases` identity to the
backend config and registry, all shipped profile defaults, the frontend
defaults, and the startup environment audit. The flag is experimental,
high-risk, mutable, and restart-required. Focused runtimeflags, config,
profiles, backendapp, and frontend contract tests pass.
