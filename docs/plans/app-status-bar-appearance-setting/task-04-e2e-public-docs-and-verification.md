---
id: app-status-bar-appearance-04
title: Status bar E2E docs and verification
status: done
wave: 4
depends_on: [app-status-bar-appearance-03]
plan: docs/plans/app-status-bar-appearance-setting/plan.md
spec: docs/specs/ui/requirements/app-status-bar.md
decision: docs/decisions/2026-08-11-user-owned-status-bar-visibility.md
---

# Status bar E2E, public docs, and verification

## Inputs

Tasks 01 through 03, the desktop App status bar E2E, mobile Status drawer E2E,
connectivity warning E2E, agent-runtime availability E2E, and public
configuration/operations documentation.

## E2E sequence

1. Add a failing desktop Appearance persistence scenario to
   `apps/web/e2e/tests/layout/app-status-bar.spec.ts`.
2. Add failing phone coverage in the correctly named
   `apps/web/e2e/tests/layout/mobile-app-status-bar.spec.ts` and run it under
   `mobile-chrome`.
3. Convert warning and runtime-availability tests from feature-store mutation
   to backend-owned user settings.
4. Add baseline capture/restore before running the focused production-build
   suites.

## E2E implementation

### Persisted baseline discipline

- Read `app_status_bar_enabled` in `beforeEach` with
  `apiClient.getUserSettings()`.
- Restore it in `afterEach` with PATCH even after an assertion failure.
- Any touched test that mutates `app_status_bar_order` or
  `system_metrics_display` must capture and restore that field too.
- Use API PATCH for setup and cleanup. Use the visible switch and shared save
  action in the main behavior scenarios.

### Desktop and tablet

- On `/settings/preferences/appearance`, verify **Show status bar** starts from the
  backend value.
- Turn it off, confirm dirty state, save through the floating **Save changes**
  action, and wait for the ordinary bar to disappear without a restart.
- Reload and prove the switch remains off and the bar remains absent.
- Turn it on, save, and prove the 24 px bar returns with existing shell geometry
  intact.
- Retain modifier-drag order persistence coverage and restore its saved order.
- In `ws-connectivity-warning.spec.ts`, establish false through user settings
  and retain the sidebar plus sub-768 drawer warning fallbacks.
- In `agent-runtime-unavailable.spec.ts`, establish false through user settings
  and prove the independent persistent alert remains visible.

### Phone and coarse pointer

- Reach Appearance through the shipped mobile settings navigation.
- Turn the preference off and save. Prove ordinary native Status entry points
  and the general drawer are absent after navigation and reload.
- Turn it back on and prove a native Status trigger opens the existing inset
  drawer, closes with focus return, and persists.
- Assert the switch/trigger are at least 44 px, the drawer stays within the
  viewport, has one internal scroll owner, honors safe-area composition, and
  creates no document horizontal overflow.
- Update `mobile-ws-connectivity-warning.spec.ts` to use the false user setting
  while retaining phone and coarse-pointer tablet connection-only coverage.
- Continue injecting only transient connection severity through
  `__KANDEV_E2E_STORE__`; do not sleep through the three/ten-second thresholds.

## Public documentation

Update:

- `docs/public/configuration.md`
  - remove `features.app_status_bar`,
    `KANDEV_FEATURES_APP_STATUS_BAR`, and profile-default rows;
  - do not replace them with a config key.
- `docs/public/operations.md`
  - point users to Settings > Preferences > Appearance > **Show status bar**;
  - state that saves apply without restart;
  - explain that host metrics remain separately controlled and urgent
    connection warnings remain visible while ordinary status chrome is off.
- `docs/public/agents-and-profiles.md`
- `docs/public/developer-tools.md`
- `docs/public/sessions-and-review.md`
  - replace feature-gated wording with the default-off portable preference and
    the desktop/tablet versus phone presentation.

Review all changed user-facing copy through i18n rules. Public Markdown does not
use runtime translation, but it must use current terms and no Unicode em dash.

## Acceptance

1. Desktop and mobile users can turn the status surface off and on through
   Appearance, with shared-save dirty state and reload persistence.
2. The preference takes effect without backend restart.
3. Desktop/sidebar, narrow responsive, phone, and coarse-pointer warning
   fallbacks remain reachable while ordinary status chrome is off.
4. Host metrics and LSP fallback behavior remain independent.
5. Agent-runtime availability remains visible regardless of the preference.
6. E2E tests restore persisted baselines and pass from one shared worker state.
7. Public docs contain no live App status bar runtime flag/env instructions.
8. Backend tests/lint, frontend-focused tests/lint/typecheck/i18n, public docs
   validation, and production-build desktop/mobile E2E all pass.

## Verification

Install dependencies once if needed:

```sh
(cd apps && pnpm install --frozen-lockfile)
```

Run focused and static checks:

```sh
(cd apps/backend && go test ./internal/user/... ./internal/backendapp/... ./internal/runtimeflags ./internal/common/config ./internal/profiles)
make -C apps/backend lint
(cd apps && pnpm --filter @kandev/web exec vitest run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts hooks/use-ensure-user-settings.test.ts components/settings/app-status-bar-settings-card.test.tsx components/settings/general-settings.test.tsx components/app-status-bar/app-status-surface-provider.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx components/system-metrics/topbar-metrics.test.tsx lib/lsp/lsp-status-placement.test.ts components/app-status-bar/lsp-status-item.integration.test.tsx lib/state/slices/features/features-contract.test.ts app/actions/features.test.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Run desktop and mobile through separate managed-runner invocations:

```sh
(cd apps/web && pnpm e2e:run --project chromium tests/layout/app-status-bar.spec.ts tests/layout/ws-connectivity-warning.spec.ts tests/layout/agent-runtime-unavailable.spec.ts tests/settings/resource-metrics-display.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/layout/mobile-app-status-bar.spec.ts tests/layout/mobile-ws-connectivity-warning.spec.ts tests/settings/mobile-resource-metrics-display.spec.ts)
git diff --check
```

Do not use `--no-build` on the first E2E run. Use it on the second only when the
first run completed from the same final tree.

## Dependencies

Task 03. E2E and public docs must describe the final user-settings contract,
not the transitional state where the runtime flag still exists.

## Risks

- Settings persist across E2E tests; missing cleanup can make explicit-on tests
  order-dependent.
- Saving off from the Appearance route changes shell height immediately. Await
  surface and save-state locators instead of using delays.
- A phone test that opens the drawer directly can miss broken native entry
  points. Navigate through the shipped menu/top-bar path.
- Running both Playwright projects in one runner invocation can use the wrong
  project. Keep desktop and `mobile-chrome` separate.
- Removing configuration docs without adding the Appearance path leaves users
  no discoverable replacement.

## Output contract

Report desktop and mobile flows, warning/alert exceptions, persisted cleanup,
public pages updated, exact command results, and blockers. Mark this task and
the parent plan done only after every acceptance item is green.
