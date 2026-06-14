---
status: draft
created: 2026-06-14
owner: tbd
---

# Feature Toggles

## Why

Users can already opt into runtime features through environment variables and
`profiles.yaml`, but those controls are invisible from the app and require
terminal access. Users need a settings page that explains experimental
features, lets them choose install-level overrides, and clearly handles the
restart required for startup-gated features such as Office mode and Debug mode.

## What

- Kandev adds a **Feature Toggles** settings page at
  `/settings/system/feature-toggles`.
- The page shows user-facing runtime toggles, not every profile/env knob.
- V1 exposes two user-facing toggles:
  - **Office mode** — enables the autonomous-agent Office surface.
  - **Debug mode** — enables local diagnostic/debug behavior, including agent
    message debug logs.
- Office mode is marked **Experimental** and includes a concise description of
  what it does and the risks of enabling it.
- Debug mode is a single user-facing toggle. Agent message debug logs are part
  of Debug mode, not a duplicate top-level toggle.
- Every v1 toggle requires restart before the changed value takes effect.
- The page distinguishes effective value, saved override, default value, and
  environment-locked value.
- Explicit environment variables remain authoritative. If an env var controls a
  flag, the UI shows the flag as locked and does not allow overriding it.
- Users can reset a saved override back to the profile/env default.
- After saving any restart-required change, the UI shows a persistent restart
  banner and offers a restart action when Kandev can safely restart itself.
- If automatic restart is unsupported, the UI shows manual restart guidance.
- `/api/v1/features` continues to return effective feature booleans for SSR
  gating and existing `useFeature()` callers.

## Tech stack

- Backend: Go, Gin HTTP handlers, SQLite persistence.
- Frontend: Next.js App Router, React, Zustand, `@kandev/ui` shadcn
  components.
- E2E: Playwright under `apps/web/e2e`.
- Runtime defaults: `profiles.yaml` embedded by `apps/backend/internal/profiles`.

## Commands

```bash
make fmt
make typecheck test lint
make -C apps/backend test
cd apps && pnpm --filter @kandev/web typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e -- settings/feature-toggles.spec.ts settings/mobile-feature-toggles.spec.ts
```

## Project structure

```text
apps/backend/internal/runtimeflags/      Runtime flag registry, store, service, handlers
apps/backend/internal/system/restart/    Restart capability/request support
apps/backend/cmd/kandev/                 Startup override application and route wiring
apps/web/app/settings/system/feature-toggles/page.tsx
apps/web/components/settings/system/feature-toggles-settings.tsx
apps/web/lib/api/domains/runtime-flags-api.ts
apps/web/e2e/settings/feature-toggles.spec.ts
apps/web/e2e/settings/mobile-feature-toggles.spec.ts
docs/decisions/0018-runtime-settings-overrides.md
docs/decisions/0019-restart-supervisor.md
```

## Code style

Use typed contracts at the backend boundary and keep env-var knowledge in the
backend registry rather than duplicating it in React components.

```go
type RuntimeFlagDefinition struct {
	Key             string
	EnvVar          string
	Kind            RuntimeFlagKind
	Label           string
	Description     string
	Stability       RuntimeFlagStability
	RiskLevel       RuntimeFlagRiskLevel
	RiskDescription string
	RestartRequired bool
	Mutable          bool
}
```

Frontend components render the API shape and avoid hardcoding flag-specific
logic except for layout/grouping.

## Data model

Feature Toggle overrides are install-scoped and stored in SQLite.

`runtime_flag_overrides`

| Field | Type | Constraint |
|---|---|---|
| `key` | `TEXT` | primary key, e.g. `features.office` |
| `value` | `INTEGER` | required, `0` or `1` |
| `created_at` | `DATETIME` | required |
| `updated_at` | `DATETIME` | required |

Absence of a row means "use the effective default". A `false` row is a real
override and must not be treated as missing.

## API surface

`GET /api/v1/runtime-flags`

Returns all user-facing runtime toggle states and metadata.

```json
{
  "flags": [
    {
      "key": "features.office",
      "kind": "feature",
      "label": "Office mode",
      "description": "Enables autonomous agent office workflows and related settings.",
      "stability": "experimental",
      "risk_level": "medium",
      "risk_description": "Office mode is still evolving. Workflows, routes, and background automation may change between releases and should be reviewed before relying on them.",
      "effective_value": false,
      "default_value": false,
      "override_value": true,
      "source": "override",
      "env_var": "KANDEV_FEATURES_OFFICE",
      "env_locked": false,
      "restart_required": true,
      "requires_restart_to_apply": true
    }
  ]
}
```

`PATCH /api/v1/runtime-flags/:key`

```json
{ "override": true }
```

`override` can be `true`, `false`, or `null`. `null` clears the saved override.
Unknown keys return `404`. Env-locked keys reject writes with `409`.

`GET /api/v1/system/restart-capability`

```json
{
  "supported": true,
  "mode": "cli",
  "reason": ""
}
```

`mode` is `cli`, `systemd`, `launchd`, or `unsupported`.

`POST /api/v1/system/restart`

Requests a restart. The response is flushed before shutdown starts so the UI
can enter a waiting state and poll `/health`.

`GET /api/v1/features`

Existing contract remains unchanged: effective feature booleans only.

## State machine

Runtime flag state:

```text
profile default -> override saved -> restart pending -> restarted/effective
               \-> override cleared -> restart pending -> restarted/default
```

Restart request state:

```text
idle -> requested -> backend unavailable -> backend healthy -> complete
                 \-> timeout -> manual recovery shown
```

## Permissions

Kandev is single-user in v1. Every user with settings access can view and change
Feature Toggles. Future multi-user support must restrict Feature Toggles,
especially Debug mode, to admins.

## Failure modes

- **Environment-locked flag** — UI disables the switch and shows the controlling
  env var. API rejects override writes with `409`.
- **Override store unavailable** — runtime flags API fails with a recoverable
  error; existing `/api/v1/features` continues to use startup-effective config.
- **Restart unsupported** — restart banner shows manual restart instructions
  and no dead action button.
- **Restart requested while one is pending** — API rejects or idempotently
  reports the pending restart.
- **Backend does not return after restart** — frontend times out and shows
  manual recovery guidance.
- **Debug mode enabled** — UI warns that local diagnostic endpoints and agent
  message logs may include sensitive prompt, file, and tool content.

## Persistence guarantees

- Saved overrides survive Kandev restarts.
- Effective runtime values are applied at backend startup.
- V1 toggles are startup-gated; changing a value does not affect already
  registered routes, constructed services, or running agentctl processes until
  restart.
- Environment variables always win over saved overrides.

## Testing strategy

- Backend unit tests cover registry metadata, override persistence, precedence,
  env locking, and pending-restart computation.
- Backend handler tests cover runtime flag APIs and restart capability/request
  behavior.
- Frontend component tests cover Office experimental copy, Debug mode copy,
  env-locked rows, save/reset actions, and restart banner states.
- Desktop and mobile Playwright tests cover the user workflow from settings
  navigation through saving a restart-required toggle.

## Boundaries

- Always: keep `profiles.yaml` as immutable shipped defaults.
- Always: keep env vars authoritative over DB overrides.
- Always: show restart-required state for v1 toggles.
- Always: include mobile coverage for the settings page.
- Ask first: adding a new external flag service or changing the auth model.
- Ask first: exposing mocks/e2e tuning as user-facing toggles.
- Never: make the Go backend fork an unmanaged replacement process for restart.
- Never: expose agent message debug logs as a separate top-level toggle in v1.

## Scenarios

- **GIVEN** Office mode is using the production default, **WHEN** the user opens
  `/settings/system/feature-toggles`, **THEN** Office mode appears off with a
  `Default` source, an `Experimental` badge, and risk text.
- **GIVEN** Office mode is off, **WHEN** the user enables it, **THEN** the
  override is saved and the page shows that restart is required before Office
  mode takes effect.
- **GIVEN** `KANDEV_FEATURES_OFFICE=true` is set in the environment, **WHEN** the
  user opens Feature Toggles, **THEN** Office mode appears on, env-locked, and
  cannot be changed from the UI.
- **GIVEN** Debug mode is off, **WHEN** the user enables it, **THEN** the page
  explains that local diagnostic endpoints and agent message debug logs will be
  enabled after restart.
- **GIVEN** a toggle override exists, **WHEN** the user clicks `Reset to default`,
  **THEN** the override is removed and the page shows restart-required state if
  the effective runtime value differs from the restored default.
- **GIVEN** restart capability is supported, **WHEN** the user clicks
  `Restart Kandev`, **THEN** the UI enters `Restarting...`, polls `/health`, and
  refreshes the page after the backend returns healthy.
- **GIVEN** restart capability is unsupported, **WHEN** a restart-required change
  is saved, **THEN** the UI shows manual restart instructions instead of a
  clickable restart button.
- **GIVEN** a mobile viewport, **WHEN** the user opens Feature Toggles, **THEN**
  all descriptions, badges, switches, reset buttons, and restart actions are
  reachable without horizontal scrolling.

## Success criteria

- A user can enable or disable Office mode from Feature Toggles and understand
  that restart is required.
- Office mode clearly communicates experimental status and risk.
- A user can enable or disable Debug mode without seeing duplicate agent-message
  debug toggles.
- Env-controlled flags are accurately shown as locked.
- Restart-required changes have a clear supported or manual restart path.
- Existing SSR feature gating keeps working through `/api/v1/features`.
- Desktop and mobile tests cover the core workflow.

## Out of scope

- Live-applying Office mode or Debug mode without restart.
- Per-user, workspace-scoped, or percentage rollout flags.
- Exposing mock providers, e2e tuning, or arbitrary env vars as user toggles.
- A third-party feature flag service.
- Multi-user admin enforcement in v1.
