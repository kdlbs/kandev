---
id: "04-settings-update-ui"
title: "Add the Settings update UI"
status: done
wave: 3
depends_on: ["03-backend-update-pipeline"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
role: implementer
model_tier: default
---

# Task 04: Add the Settings update UI

## Acceptance

- Installed managed-agent cards expose a labeled update action with current to
  target version, queued/running/refreshing progress, bounded output,
  success/refresh-warning/failure states, and retry.
- HTTP rehydration and WebSocket snapshots/chunks keep update state accurate
  across disconnects without duplicating output; unmanaged agents omit the
  control and same-agent maintenance disables it.
- Desktop and phone layouts preserve the existing card composition, at least
  44 px touch targets, vertical reachability, and no horizontal overflow.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/settings/settings-slice.test.ts lib/ws/handlers/agents.test.ts components/settings/agent-runtime-update-control.test.tsx`
- `cd apps/web && pnpm run typecheck`
- `cd apps && pnpm --filter @kandev/web lint`

## Files likely touched

- `apps/web/lib/types/http-agents.ts`
- `apps/web/lib/api/domains/settings-api.ts`
- Existing API export barrels under `apps/web/lib/api/`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/state/slices/settings/settings-slice.test.ts`
- `apps/web/lib/state/store.ts`
- Existing settings state re-export files
- `apps/web/lib/types/backend.ts`
- `apps/web/lib/ws/handlers/agents.ts`
- `apps/web/lib/ws/handlers/agents.test.ts`
- `apps/web/app/settings/agents/page.tsx`
- `apps/web/components/settings/installed-agent-card.tsx`
- `apps/web/components/settings/agent-runtime-update-control.tsx`
- `apps/web/components/settings/agent-runtime-update-control.test.tsx`

## Dependencies

Task 03 supplies the HTTP DTOs, state machine, conflict response, retained-job
list, and WebSocket actions.

## Inputs

- Spec `What`, `API surface`, `State machine`, and desktop/mobile scenarios
- Plan `API, state, and WebSocket hydration` and
  `Installed-agent update control`
- Existing `useInstallAgent`, `InstallAgentCard`, `InstalledAgentCard`, settings
  slice, and agent WS handler
- Mobile parity contract: visible labeled action, 44 px target, document
  scrolling, bounded internal log, no horizontal overflow

## Output contract

Report intent/acceptance, base/head SHA, changed entry points, named spec
sections, risk tags (`settings-ui`, `state-hydration`, `mobile-parity`,
`accessibility`), exact targeted results, and uncertainties. Update only this
task file to `done`; do not edit `plan.md`.
