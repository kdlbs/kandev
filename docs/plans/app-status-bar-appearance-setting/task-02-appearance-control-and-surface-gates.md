---
id: app-status-bar-appearance-02
title: Appearance control and surface gates
status: done
wave: 2
depends_on: [app-status-bar-appearance-01]
plan: docs/plans/app-status-bar-appearance-setting/plan.md
spec: docs/specs/ui/requirements/app-status-bar.md
decision: docs/decisions/2026-08-11-user-owned-status-bar-visibility.md
---

# Appearance control and surface gates

## Inputs

Task 01's default-false portable setting, the existing Appearance-page shared
save coordinator, `SystemMetricsSettingsCard`, `AppStatusSurfaceProvider`, and
the warning/LSP fallback contracts in the linked specs.

## TDD sequence

1. Add failing tests for the controlled Appearance card and shared save payload,
   discard, dirty state, and post-save store update.
2. Convert existing surface/sidebar/top-bar/LSP tests from feature fixtures to
   user-setting fixtures and make the desired on/off/fallback cases fail.
3. Implement the localized card, Appearance integration, and user-setting
   selectors.
4. Regenerate pseudo translations, run focused tests, then refactor duplicated
   visibility selectors or test setup only if needed.

## Implementation

### Appearance

- Create
  `apps/web/components/settings/app-status-bar-settings-card.tsx` as a
  controlled Settings card.
- Render a localized **Show status bar** switch and explanation. The row must
  remain at least 44 px tall and expose the existing `data-settings-dirty`
  marker.
- Add a Status bar section to
  `apps/web/components/settings/general-settings.tsx`.
- Extend `createAppearanceSavedState`, saved/draft state, the
  `general-appearance` contributor's PATCH payload, discard behavior, and
  post-save `setUserSettings` call with `appStatusBarEnabled`.
- Keep one shared **Save changes** action. Do not save on switch activation and
  do not add optimistic durable state before the API succeeds.
- Do not send an empty user-settings PATCH when the save contains only local
  theme or menu changes.
- Add `GENERAL_SETTINGS_TARGETS.appStatusBar` and a discovery control in
  `apps/web/lib/settings-discovery/catalog/preferences.ts`, under the
  Preferences Appearance page.
- Add English keys in `apps/web/src/locales/en/settings.json` and run
  `pnpm run i18n:pseudo`.

### Runtime consumers

- Replace the feature selector in
  `apps/web/components/app-status-bar/app-status-surface-provider.tsx` with
  `state.userSettings.appStatusBarEnabled`.
- Preserve `connectionOnly = !enabled && issueSeverity !== "none"` and mount no
  metrics or plugin contributions for that exception.
- Replace the feature selector in
  `apps/web/components/app-sidebar/app-sidebar-footer.tsx`; warning fallback
  appears only while the preference is off and severity is active.
- Replace the feature selector in
  `apps/web/components/system-metrics/topbar-metrics.tsx`; the existing top-bar
  metrics fallback remains available while the bar is off.
- Replace the feature selector in
  `apps/web/hooks/use-lsp-status-placement.ts`; keep the pure
  `resolveLspStatusPlacement` behavior and saved preference unchanged.
- Remove obsolete feature mocks from related drawer/LSP integration tests where
  they no longer affect rendering.

## Mobile design contract

- **Desktop outcome:** saved on mounts the existing 24 px bottom bar; saved off
  removes it after the shared save completes.
- **Phone entry:** saved on exposes existing native Status controls; saved off
  removes ordinary Status controls while warning-only controls remain possible.
- **Nearest exemplar:** the resource metrics card provides the controlled
  switch/dirty pattern. The shipped App Status drawer remains the responsive
  surface exemplar.
- **Hierarchy and presentation:** use a dedicated Status bar Settings section
  inside Appearance and the existing floating save action. No mobile-only
  dialog or duplicate setting.
- **Shared state:** one user-setting selector governs desktop, tablet, and phone.
  Breakpoint code only chooses presentation.
- **Geometry:** keep the switch row and native triggers touch-sized; make no
  shell, safe-area, or scroll-owner changes.

## Tests

- Add
  `apps/web/components/settings/app-status-bar-settings-card.test.tsx` for
  accessible label/description, controlled value, callback, and dirty marker.
- Add a focused
  `apps/web/components/settings/general-settings.test.tsx` for the shared save
  payload, successful store update, and discard behavior.
- Update:
  - `components/app-status-bar/app-status-surface-provider.test.tsx`
  - `components/app-sidebar/app-sidebar-footer.test.tsx`
  - `components/system-metrics/topbar-metrics.test.tsx`
  - `lib/lsp/lsp-status-placement.test.ts`
  - `components/app-status-bar/lsp-status-item.integration.test.tsx`
  - any drawer fixture that still injects `features.appStatusBar`.

Tests must prove:

1. Explicit true mounts the ordinary responsive status surface.
2. False removes ordinary bar/drawer access.
3. False plus an active connectivity issue mounts only warning fallback.
4. Metrics and LSP fall back to their old non-bar locations without overwriting
   their own saved settings.
5. Failed save leaves the confirmed Zustand value active and the Appearance
   draft dirty/retryable.
6. A delayed status-order PATCH response cannot replace a newer settings
   revision that arrived through another ingestion path.

## Verification

```sh
(cd apps && pnpm --filter @kandev/web exec vitest run components/settings/app-status-bar-settings-card.test.tsx components/settings/general-settings.test.tsx components/app-status-bar/app-status-surface-provider.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx components/system-metrics/topbar-metrics.test.tsx lib/lsp/lsp-status-placement.test.ts components/app-status-bar/lsp-status-item.integration.test.tsx)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
git diff --check
```

## Dependencies

Task 01. This task makes the runtime flag unused by product surfaces; Task 03
then removes its active contracts safely.

## Risks

- Updating the visible switch but omitting it from `createAppearanceSavedState`
  or the post-save store update can create false dirty states or delayed UI.
- Mounting the warning-only drawer through the general enabled path can run
  metrics/plugin effects while the user opted out.
- Translating copy at module scope or comparing translated text would violate
  i18n rules. Render copy through `t()` inside components.
- A small switch with an untappable surrounding row fails mobile parity even if
  desktop tests pass.

## Output contract

Report the Appearance payload and failure behavior, responsive gate results,
fallback coverage, generated locale changes, exact commands, and blockers. Mark
this task done only after every live consumer reads user settings and the flag
has no UI consumers.

## Validation results

- RED: preference-backed fixtures failed against the former feature selectors;
  the Appearance integration could not find **Show status bar**.
- GREEN: focused Vitest run passed 43 tests across the Appearance control,
  responsive surfaces, warning fallback, metrics fallback, and LSP placement.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `pnpm run typecheck`
  passed. Focused ESLint passed after extracting the Appearance section body
  and the added WebSocket case to respect repository size limits.
