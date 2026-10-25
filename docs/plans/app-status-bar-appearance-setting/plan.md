---
spec: docs/specs/ui/requirements/app-status-bar.md
decision: docs/decisions/2026-08-11-user-owned-status-bar-visibility.md
created: 2026-08-11
status: done
---

# Implementation Plan: Status Bar Appearance Setting

## Outcome

Graduate the stable App status bar out of runtime feature flags. Keep ordinary
status chrome off by default and give each user a portable **Show status bar** preference under
Settings > Preferences > Appearance. A successful Appearance save changes the
desktop/tablet bar or ordinary phone Status paths without restarting Kandev.

The WebSocket connectivity warning remains visible through its problem-only
fallback when the preference is off. Existing metrics, item-order, and LSP
location preferences remain independent.

## Fixed decisions

- The wire field is the top-level user setting
  `app_status_bar_enabled: boolean`; frontend state uses
  `appStatusBarEnabled`.
- Missing stored values and initial compatibility payloads default to `false`.
  PATCH and partial-live-update omission preserve the current value, while an
  explicit boolean is durable.
- The former `features.appStatusBar` and
  `KANDEV_FEATURES_APP_STATUS_BAR` identities are retired, never reused, and not
  migrated into user settings. Unknown persisted runtime override rows remain
  inert.
- The new control participates in the existing Appearance-page saved/draft
  model and shared **Save changes** coordinator. It does not create an
  independent save button or browser-storage fallback.
- Turning the preference off hides the ordinary desktop/tablet bar and phone
  Status entry points. It preserves:
  - the sidebar or connection-only drawer WebSocket warning fallback;
  - the pre-status-bar top-bar metrics fallback when metrics are enabled;
  - the Monaco toolbar fallback when LSP location is `status_bar`;
  - the independent agent-runtime availability alert.
- `system_metrics_display.show_in_topbar`,
  `system_metrics_display.simplified`, `app_status_bar_order`, and
  `lsp_status_location` keep their existing meanings.
- One relational migration adds an atomic per-user settings revision. No new
  endpoint, WebSocket action, plugin contract, or status-bar layout change is
  introduced.

## Backend

### Portable settings contract

- Add `AppStatusBarEnabled bool` to the user settings model and response DTO.
- Add `AppStatusBarEnabled *bool` to PATCH DTO, controller/service request
  mapping, and basic settings application so omitted and explicit false remain
  distinct.
- Set the canonical default to `false` in `defaultUserSettings`.
- Decode the stored JSON through a `*bool` field and overlay it only when
  present. This preserves explicit values while treating old rows as disabled.
- Include `app_status_bar_enabled` in JSON persistence and the
  `user.settings.updated` event.
- Add `appStatusBarEnabled` to the Go boot-state mapping.
- Add a `settings_revision` column that every settings mutation increments and
  returns atomically. Include the revision in boot state, PATCH responses, and
  `user.settings.updated` events.

The existing `users.settings` JSON blob carries the visibility field. The SQL
migration is limited to the independent revision counter used to order complete
settings snapshots.

### Runtime flag retirement

- Remove the App status bar field from `common/config.FeaturesConfig`.
- Remove its profile entry from
  `apps/backend/internal/profiles/profiles.yaml`.
- Remove its active runtime registry registration and add the exact key/env pair
  to `retiredRuntimeFlagIdentities`.
- Remove status-bar-specific config/profile/runtime tests and replace the
  registry metadata test with explicit graduation tests: the definition is
  absent and the retired identity is present.
- Do not delete existing runtime override rows. Registry handling for unknown
  keys already makes them harmless.

## Frontend

### Hydration and live settings

- Add optional `app_status_bar_enabled` fields to HTTP user-settings response
  and PATCH types.
- Add `appStatusBarEnabled` to `UserSettingsState`, with a frontend placeholder
  default of `false`.
- Extend the shared wire mapper so boot hydration, PATCH responses, and
  `user.settings.updated` all apply the field. An absent field preserves the
  current mapped value; the initial current value is the default-false state.
- Compare the atomic numeric revision at every boot, HTTP, and WebSocket
  ingestion path. Older snapshots cannot replace newer state, including when a
  queued status-order PATCH finishes after a newer live update.
- Keep backend defaults authoritative. Do not add localStorage, cookies, or a
  second compatibility mapper.

### Appearance control

- Add a controlled `AppStatusBarSettingsCard` under Preferences > Appearance.
- Use localized copy:
  - label: **Show status bar**
  - explanation: the preference shows connection, optional host metrics, and
    plugin status in the desktop/tablet bottom bar or phone Status drawer;
    important connection warnings remain visible when it is off.
- Give the switch row a minimum 44 px interactive height and reuse the current
  Settings card, label, switch, dirty marker, and focus treatment.
- Extend `createAppearanceSavedState`, the Appearance draft, the shared save
  PATCH, discard behavior, and post-save Zustand update with
  `appStatusBarEnabled`.
- Skip the backend PATCH when an Appearance save contains only local theme or
  menu changes.
- Add a settings-discovery target and aliases for status bar, status drawer,
  bottom bar, and appearance under the Preferences group.
- Add English locale keys and regenerate the pseudo locale. Do not hand-edit
  generated pseudo output.

### Surface gates

Replace `useFeature("appStatusBar")` and feature fixtures with the portable
setting in:

- `AppStatusSurfaceProvider`, which owns desktop/tablet versus phone
  presentation and the connection-only exception;
- `AppSidebarFooter`, which owns the desktop/tablet warning fallback;
- `TopbarMetrics`, which owns the old metrics fallback;
- `useLspStatusPlacement`, which passes the effective visibility to the pure
  placement resolver.

The pure resolver may retain the internal `appStatusBarEnabled` parameter. Its
input source changes from runtime features to user settings. Tests and E2E
fixtures must stop mutating the frontend feature slice for this behavior.

### Frontend feature contract

- Remove `appStatusBar` from `defaultFeatureFlags`.
- Update feature action/contract fixtures to match the backend
  `/api/v1/features` response after the config field is removed.
- Keep the generic backend/frontend feature-key parity test as the contract
  oracle.

## Mobile design contract

- **Desktop outcome:** a saved-on preference renders the shipped 24 px in-flow
  bar. Saved-off removes it and releases its ordinary contribution/metrics
  mounts.
- **Phone entry:** saved-on keeps the shipped native Status entry points and
  inset drawer. Saved-off removes healthy-state Status entry points.
- **Nearest exemplar:** `SystemMetricsSettingsCard` supplies the controlled
  Appearance switch pattern. `AppStatusDrawer` and
  `mobile-resource-metrics-display.spec.ts` supply phone drawer and settings
  interaction patterns.
- **Hierarchy:** the setting is an Appearance choice, not a System Feature
  Toggle. Its description names the desktop/tablet bar, phone drawer, and
  warning exception.
- **Presentation:** reuse the existing Appearance page and shared floating save
  affordance. Do not add a mobile-only page, modal, or direct-save switch.
- **Shared state:** backend-owned `appStatusBarEnabled` selects both responsive
  presentations. Mobile code only selects existing touch presentation.
- **Geometry:** the switch row and Status triggers remain at least 44 px; the
  existing drawer remains viewport-contained, safe-area-aware, and the sole
  internal scroll owner. No document horizontal overflow is introduced.

## Test strategy

### Backend unit and integration tests

- Store tests prove old/missing JSON defaults false, explicit true survives a
  repository round trip, encoding emits the complete field, legacy schemas gain
  the revision column, and concurrent writes receive distinct revisions.
- DTO tests prove response mapping plus PATCH omission versus explicit false.
- Service tests prove apply semantics and exact event payload.
- Boot-state tests prove `appStatusBarEnabled` is present and preserves false.
- Runtime registry/config/profile tests prove the active flag is gone and its
  exact identity is retired.

### Frontend unit and component tests

- Shared settings mapper tests prove default false, explicit true, and partial
  update preservation plus revision ordering across boot, HTTP, and WebSocket
  paths.
- Status-order tests prove a delayed older PATCH response cannot inherit a
  newer store revision or replace that newer snapshot.
- Appearance tests prove the controlled switch's accessible copy, dirty state,
  discard, PATCH payload, and post-save store update.
- Status surface provider tests prove on/off responsive mounting and
  connection-only bypass.
- Sidebar, top-bar metrics, and LSP tests prove their existing fallbacks now
  follow user settings.
- Feature contract tests prove `appStatusBar` is absent from the runtime feature
  response and frontend feature state.

### E2E

- Extend desktop `layout/app-status-bar.spec.ts` with the persisted Appearance
  opt-out/opt-in flow and retain geometry/order coverage.
- Add `layout/mobile-app-status-bar.spec.ts` under the `mobile-chrome` project.
  Prove switch/save/reload persistence, ordinary Status path absence while off,
  restored drawer access while on, 44 px controls, viewport containment, focus
  return, and zero horizontal overflow.
- Update desktop/tablet and mobile connectivity warning specs to establish the
  preference through user settings instead of feature-state mutation.
- Update agent-runtime availability coverage so its status-bar-off setup also
  uses user settings and the alert remains independent.
- Capture and restore every persisted baseline field changed by these specs in
  `afterEach`; the E2E worker does not reset user settings between tests.
- Use API PATCH only for setup/cleanup. Exercise the visible Appearance control
  for the product flow itself. Continue injecting transient warning severity
  through the E2E store bridge so Playwright does not wait ten seconds.

## Public documentation

Implementation updates these shipped-behavior pages in the same change:

- `docs/public/configuration.md`: remove the runtime flag/config/env entries and
  profile-default claims.
- `docs/public/operations.md`: describe **Show status bar**, no-restart behavior,
  metrics independence, and the connectivity warning exception.
- `docs/public/agents-and-profiles.md`,
  `docs/public/developer-tools.md`, and
  `docs/public/sessions-and-review.md`: replace feature-gated wording with the
  default-off user preference and responsive surface behavior.

No new public page or navigation entry is needed.

## Implementation waves

Execution is sequential in the primary conversation.

| Wave | Task | Status | Gate |
|---|---|---|---|
| 1 | [01 portable visibility contract](task-01-portable-visibility-contract.md) | Done | Default-false backend/frontend setting round trips without changing live gates. |
| 2 | [02 Appearance control and surface gates](task-02-appearance-control-and-surface-gates.md) | Done | UI saves the portable preference and every surface consumes it. |
| 3 | [03 retire runtime flag](task-03-retire-runtime-flag.md) | Done | Flag/config/profile identities are removed from active contracts and retired permanently. |
| 4 | [04 E2E, public docs, and verification](task-04-e2e-public-docs-and-verification.md) | Done | Desktop/mobile persistence, fallbacks, docs, and final checks pass. |

Tasks share settings and feature contracts, so none are parallel-safe.

## Verification order

If `apps/node_modules` is absent, run once before frontend checks:

```sh
(cd apps && pnpm install --frozen-lockfile)
```

Each task runs its focused commands. Final verification:

```sh
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/... ./internal/runtimeflags ./internal/common/config ./internal/profiles)
make -C apps/backend lint
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
(cd apps/web && pnpm e2e:run --project chromium tests/layout/app-status-bar.spec.ts tests/layout/ws-connectivity-warning.spec.ts tests/layout/agent-runtime-unavailable.spec.ts tests/settings/resource-metrics-display.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-app-status-bar.spec.ts tests/layout/mobile-ws-connectivity-warning.spec.ts tests/settings/mobile-resource-metrics-display.spec.ts)
git diff --check
```

The managed E2E runner must rebuild production assets for the first run. The
second project may use `--no-build` only when the first build completed from the
same final tree.

## Validation results

- Backend focused tests and lint passed.
- Frontend focused tests passed: 12 files and 114 tests.
- Frontend typecheck, lint, i18n check, and i18n ratchet passed. The i18n check
  reported only its advisory real-locale parity backlog.
- Public-doc tests passed: 58 tests; validation passed for 41 pages.
- Production-build desktop status bar, warning, agent-runtime fallback, and
  resource-metrics E2E passed: 9 tests.
- Mobile status bar, warning, and resource-metrics E2E passed: 4 tests.
- Older status-surface consumers now opt in explicitly; their desktop suite
  passed 16 tests and their mobile suite passed 29 tests.
- `git diff --check` passed.

## Risks

- A backend or frontend fallback that remains true would enable ordinary status
  chrome for users who never opted in.
- Updating only `AppStatusSurfaceProvider` would leave sidebar warnings,
  top-bar metrics, or LSP placement bound to a removed feature key.
- Removing the runtime registration without retiring its identity would permit
  a future flag to reinterpret stale persisted overrides.
- Reusing `show_in_topbar` would make metrics visibility hide unrelated
  connection/plugin status.
- Direct feature-store mutation in E2E would pass without exercising the new
  persistence path. Persisted setup without cleanup would leak into later specs.
- Saving Appearance while the current route owns the status bar changes shell
  geometry immediately. Tests must wait on the surface and floating-save state,
  not fixed delays.
