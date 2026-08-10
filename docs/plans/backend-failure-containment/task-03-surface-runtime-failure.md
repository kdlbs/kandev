---
id: "03-surface-runtime-failure"
title: "Surface runtime failure and recovery"
status: done
wave: 2
depends_on: ["02-publish-agent-runtime-availability"]
plan: "plan.md"
spec: "../../specs/platform/agent-runtime-availability.md"
---

# Task 03: Surface runtime failure and recovery

## Intent

Hydrate the backend-owned runtime snapshot and show a persistent, accessible
shell-level explanation with the supported Kandev restart path.

## Acceptance

- Zustand hydrates and replaces the full `agentRuntime` snapshot from boot and
  `system.agent_runtime.status_changed` without modifying domain data.
- One non-dismissible `role="alert"` renders for unavailable state on every
  authenticated route, including when the App status bar feature is disabled.
- Copy explains the local runtime impact, saved-data safety, and required full
  restart without exposing raw diagnostics.
- Supported supervisors show a working **Restart Kandev** action and existing
  restart progress flow; unsupported or failed capability lookup shows localized
  manual guidance.
- Restart request errors leave the alert mounted and readable.
- A new available snapshot removes the alert; browser connection changes alone
  do not.
- All new user-visible copy is localized in every shipped locale.

## TDD sequence

1. Add store/hydration and system-event handler tests for unavailable, available,
   and unrelated reconnect updates; confirm RED before the state contract exists.
2. Add alert/provider tests with the App status bar disabled, retained route
   children, supported capability, unsupported capability, restart error, and
   recovery; confirm RED before rendering.
3. Implement the type, store state/action, handler, alert, provider placement,
   capability selection, restart flow reuse, and translations.
4. Rerun focused tests, typecheck, and i18n checks before refactoring.

## Files likely touched

- `apps/web/lib/types/agent-runtime.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/store.ts`
- `apps/web/lib/state/store-overrides.ts`
- `apps/web/lib/ws/handlers/system-events.ts`
- `apps/web/lib/ws/handlers/system-events.test.ts`
- `apps/web/components/app-status-bar/agent-runtime-unavailable-alert.tsx`
- `apps/web/components/app-status-bar/agent-runtime-unavailable-alert.test.tsx`
- `apps/web/components/app-status-bar/app-status-surface-provider.tsx`
- `apps/web/components/app-status-bar/app-status-surface-provider.test.tsx`
- `apps/web/src/locales/en/system.json`
- `apps/web/src/locales/pt-pt/system.json`
- `apps/web/src/locales/zh-cn/system.json`

## Dependencies

Task 02's final boot and WebSocket contract.

## Parallelism

`sequential` — this task consumes the backend contract and owns the shared shell
surface used by Task 04.

## Mobile parity

The nearest shell exemplar is the in-flow App status surface, while the nearest
error exemplar is the existing SPA/session recovery alert. Phone composition
stacks copy and action, keeps the action at least 44 px, remains inside the
route/status column, and adds no fixed or bottom surface. Unit tests assert
responsive classes and semantics; Task 04 verifies real viewport geometry.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run lib/state lib/ws/handlers/system-events.test.ts components/app-status-bar`
- `cd apps/web && pnpm run typecheck`
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet`

## Inputs

- Task 02 public snapshot and action.
- Existing `useRestartCapability`, `useKandevRestart`, and
  `RestartProgressDialog` behavior.
- App status surface shell geometry and mobile UI language reference.

## Output contract

Record focused unit, typecheck, and i18n results plus the final localized copy
keys and any deliberate change to alert placement.

## Results

Implemented the backend-shaped `agentRuntime` snapshot in the web store,
hydrated it from boot state, and replaced it on the global runtime WebSocket
event without clearing domain state. Added the in-flow localized alert outside
the App status bar feature gate, reusing the existing capability and restart
progress hooks. Supported supervisors expose `Restart Kandev`; capability
lookup failures render manual guidance, and restart errors leave the alert
mounted until an available snapshot arrives.

Focused validation passed:

- `pnpm --filter @kandev/web test -- --run lib/state/store.test.ts lib/ws/handlers/system-events.test.ts components/app-status-bar/agent-runtime-unavailable-alert.test.tsx components/app-status-bar/app-status-surface-provider.test.tsx` — 21 tests passed.
- `pnpm run typecheck` — passed.
- `pnpm run i18n:check && pnpm run i18n:ratchet` — passed; pseudo catalog regenerated and synced.

The alert uses localized English, Portuguese, Chinese, and pseudo-locale
entries, remains in normal shell flow on phone layouts, and keeps its restart
action at the required 44 px minimum.
