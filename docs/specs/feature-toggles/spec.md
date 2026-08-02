---
status: draft
created: 2026-06-14
updated: 2026-08-02
owner: tbd
---

# Feature Toggles

Decision: [ADR-2026-08-01-release-toggle-gating-contract](../../decisions/2026-08-01-release-toggle-gating-contract.md)

Implementation plan: [Release Feature Toggles](../../plans/release-feature-toggles/plan.md)

## Why

Kandev needs to merge medium and large changes into `main` without making an
unfinished or risky behavior available in the next release. Operators also
need a simple way to enable a gated feature on one installation, test it with
real work, disable it again if necessary, and understand when restart is
required. Adding and later removing a temporary release toggle must not require
duplicating flag identity or config-binding code across many files.

## What

- Kandev provides install-wide runtime toggles under
  **Settings > System > Feature Toggles** at
  `/settings/system/feature-toggles`.
- Runtime toggles have two uses:
  - **Release toggles** temporarily protect unfinished or risky behavior while
    it bakes on selected installations. They are removed after graduation.
  - **Operator configuration toggles** are supported long-lived installation
    choices such as Authentication or Debug mode.
- A new release toggle defaults to `false` in the `prod`, `dev`, and `e2e`
  profiles. A feature spec may deliberately choose a different non-production
  default, but `prod` remains off until the default-on rollout.
- An administrator can save an installation override from Feature Toggles.
  Explicit environment variables remain authoritative and lock the UI control.
- Every toggle change requires restart before its effective runtime value
  changes. Unsupported supervisors show manual restart guidance.
- `/api/v1/features` exposes the startup-effective feature booleans used by SSR
  and `useFeature()` callers. Missing or unavailable frontend flag data fails
  closed to `false`.
- Feature UI is absent while its release toggle is off. A guessed route returns
  `404` when the feature owns a route subtree.
- The backend is authoritative. A disabled feature cannot be invoked by direct
  HTTP, WebSocket, MCP, background-job, or agent-tool entry paths even if a
  stale or modified client sends its enabled-only request shape.
- Gates live at the narrowest composition or capability boundary that covers
  each entry path. Internal helpers do not independently re-read the flag when
  their caller has already enforced the same boundary.
- The disabled path preserves the behavior that existed before the gated PR.
  Shared refactors, startup initialization, schema migrations, and data-model
  compatibility are not considered protected merely because a later behavior
  branch is gated.
- Data written while a feature is enabled remains readable and inert when the
  feature is disabled. Each gated feature documents any stronger rollback
  behavior in its own spec.
- Runtime flag definitions bind their registry key, environment variable,
  config read, and config apply behavior in one backend definition. Frontend
  feature names and default-false values have one declaration. Graduated flag
  keys and environment variables move to an append-only retired-identity set.
- A release toggle follows this lifecycle:
  1. merge with every shipped profile default off;
  2. enable selected installations with an override or explicit environment;
  3. ship one default-on release while retaining the kill switch;
  4. remove the live flag and legacy behavior after the default-on release is
     proven, while retaining its retired key and environment identity.
- Removed flag keys and environment variables are never reused. Unknown
  persisted override rows are ignored so upgrades and downgrades do not
  destructively rewrite operator state.

## Data model

Feature-toggle overrides are installation-scoped and stored in
`runtime_flag_overrides`.

| Field | Type | Constraint |
|---|---|---|
| `key` | `TEXT` | primary key, for example `features.office` |
| `value` | `INTEGER` | required, `0` or `1` |
| `created_at` | `DATETIME` | required |
| `updated_at` | `DATETIME` | required |

Absence of a row means “use the effective default.” A `false` row is a real
override. Rows whose keys are absent from the running binary's registry are
ignored and are not deleted automatically.

## API surface

### `GET /api/v1/runtime-flags`

Returns the registered operator-facing toggle states and metadata.

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
      "risk_description": "Office mode is still evolving.",
      "effective_value": false,
      "default_value": false,
      "override_value": true,
      "source": "override",
      "env_var": "KANDEV_FEATURES_OFFICE",
      "env_locked": false,
      "restart_required": true,
      "requires_restart_to_apply": true,
      "mutable": true
    }
  ]
}
```

### `PATCH /api/v1/runtime-flags/:key`

The request body is `{ "override": true | false | null }`. `null` clears the
saved override. Unknown keys return `404`; environment-locked keys return
`409`. Mutation requires an administrator identity. When authentication is
disabled, the synthetic install identity is an administrator.

### `GET /api/v1/features`

Returns startup-effective feature booleans keyed by their public JSON names.
It remains a public bootstrapping endpoint and contains no risk metadata or
saved override details.

Restart capability and restart-request contracts remain defined by
[Restart supervisor ownership](../../decisions/0019-restart-supervisor.md).

## State machine

```text
default off
  -> selected-install override on
  -> default on with kill switch retained
  -> live flag removed, identity retired, new behavior permanent
```

At any retained-flag stage, an explicit environment value has precedence over
a saved override. A saved value that differs from the current process value is
`restart pending` until a new backend boot applies it.

## Permissions

- Authenticated administrators and members can read runtime-toggle state and
  risk metadata; only administrators can mutate installation-wide overrides.
- `/api/v1/features` remains public because the SPA needs it before authenticated
  route composition; it exposes booleans only.
- MCP and agent callers do not receive a generic flag-mutation capability.

## Failure modes

- **Environment lock** — the UI disables mutation and identifies the controlling
  environment variable; the API returns `409`.
- **Override store unavailable** — the runtime-flags API reports a recoverable
  error; startup-effective behavior remains unchanged.
- **Frontend flag fetch fails** — client and SSR gates use their all-false
  defaults and do not expose the feature.
- **Disabled feature request** — the authoritative backend boundary rejects the
  request without mutation or side effects. Route-owned surfaces use `404`;
  extensions to an existing endpoint use a stable client error defined by that
  feature.
- **Restart unsupported or unsuccessful** — manual restart/recovery guidance is
  shown and the running process retains its old effective values.
- **Unknown persisted override** — the row is ignored and hidden, but retained
  for downgrade compatibility. Its key cannot be assigned to a future feature.
- **Feature disabled after enabled data exists** — the data remains readable but
  cannot trigger enabled-only execution. The feature's own spec defines any
  feature-specific resume or cleanup behavior.

## Persistence guarantees

- Saved overrides survive restart and version upgrades.
- Effective values are resolved at backend startup with precedence:

  ```text
  explicit environment > saved install override > active profile > Go zero value
  ```

- Applying or clearing a saved override does not live-mutate already registered
  routes, constructed services, or agent processes.
- Unknown override rows are non-authoritative and non-destructive.

## Scenarios

- **GIVEN** a new release toggle, **WHEN** Kandev boots in `prod`, `dev`, or
  `e2e` without an explicit override, **THEN** the feature is off.
- **GIVEN** a release toggle is off, **WHEN** an administrator saves an on
  override and restarts Kandev, **THEN** the feature is enabled only on that
  installation and the page reports `Saved override` as its source.
- **GIVEN** a release toggle is off, **WHEN** a direct client sends an
  enabled-only HTTP, WebSocket, MCP, or agent-tool request, **THEN** the backend
  rejects it without persisting or dispatching enabled-only work.
- **GIVEN** a release toggle is off, **WHEN** a user opens the application,
  **THEN** the new UI is absent and the pre-feature workflow remains usable on
  desktop and mobile.
- **GIVEN** `KANDEV_FEATURES_<NAME>=true`, **WHEN** the Feature Toggles page
  opens, **THEN** the toggle is on, environment-locked, and cannot be changed
  from the UI.
- **GIVEN** the backend cannot provide feature data during SSR, **WHEN** the SPA
  renders, **THEN** every unknown or unavailable feature is treated as off.
- **GIVEN** a release toggle has baked on selected installations, **WHEN** its
  production default changes to on, **THEN** operators can still override it
  off for that release.
- **GIVEN** one default-on release has succeeded, **WHEN** the toggle graduates,
  **THEN** the legacy behavior and live flag declarations/gates are removed,
  the identity is retired, and the new behavior remains enabled.
- **GIVEN** a removed key still has a persisted override, **WHEN** Kandev boots,
  **THEN** the row is ignored and no unrelated feature adopts its value.
- **GIVEN** a phone viewport, **WHEN** an administrator opens Feature Toggles and
  changes an override, **THEN** the same save, reset, lock, risk, and restart
  information is reachable without horizontal scrolling.

## Success criteria

- A risky feature can merge to `main` without changing default production,
  development, or E2E behavior.
- An operator can enable that feature on one installation and turn it off again.
- Direct and stale clients cannot bypass a disabled flag.
- Adding or removing a frontend-visible toggle changes one backend definition,
  one typed config field, one profile entry, and at most one frontend feature
  declaration before its actual gates and tests.
- CI detects incomplete or malformed registry/config/profile/frontend
  contracts in either direction.
- A graduated toggle leaves no live legacy branch or reusable stale key or
  environment variable.

## Out of scope

- Per-user, workspace-scoped, percentage, or cohort rollouts.
- Live-applying runtime toggles without restart.
- A third-party feature-flag service.
- Protecting incompatible migrations, unsafe shared initialization, or
  unconditionally executed refactors with a late behavior check.
- Exposing mocks, E2E tuning, or arbitrary environment variables as user
  settings.
