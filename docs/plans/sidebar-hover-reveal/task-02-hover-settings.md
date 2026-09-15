---
id: "02-hover-settings"
title: "Configure sidebar hover activation"
status: done
wave: 2
depends_on:
  - "01-hover-reveal"
plan: plan.md
requirements:
  - REQ-UI-SIDEBAR-HOVER-001
  - REQ-UI-SIDEBAR-HOVER-002
acceptance_criteria:
  - AC-UI-SIDEBAR-HOVER-001.1
  - AC-UI-SIDEBAR-HOVER-002.1
  - AC-UI-SIDEBAR-HOVER-002.2
  - AC-UI-SIDEBAR-HOVER-002.3
  - AC-UI-SIDEBAR-HOVER-002.4
  - AC-UI-SIDEBAR-HOVER-002.5
system_design:
  - ../../specs/ui/system-design/sidebar-hover-reveal.md
---

# Task 02: Configure Sidebar Hover Activation

## Summary

Add two portable user preferences through the existing appearance save flow and
apply them to the implemented sidebar reveal. Use TDD for defaults, validation,
persistence, timer behavior and responsive settings interactions.

## In scope

Backend JSON settings, PATCH validation, DTOs and live projection; common frontend
mapping, appearance draft/save state, a labelled settings card and discovery
entries; configurable hover hook; five-locale copy and public documentation.

## Out of scope

New endpoints, schema migrations, new mobile navigation, other sidebar preferences,
changing the running main instance, deployment, commits and PR operations.

## Acceptance

- New and legacy settings produce enabled/500 defaults; false/zero and custom
  delays persist independently. Invalid writes are atomic and do not broadcast.
- Appearance controls support draft, validation, discard, save failure, reload and
  live synchronization; saved changes safely cancel old hover interaction.
- Desktop and phone browser checks prove UI-03/UI-04, custom/disabled behavior,
  explicit expansion, accessible controls and unchanged phone tap navigation.

## ASCII UI preview

UI-03/UI-04: Preferences > Appearance > Sidebar (AC-UI-SIDEBAR-HOVER-002.1-.5).
Full composition and scroll ownership: [plan](plan.md#ascii-ui-preview-1).

```text
Desktop: Show sidebar on hover [on]   Hover delay (ms) [500]
Phone:   Show sidebar on hover [on]
         Hover delay (ms)
         [500                    ]
         Mouse or trackpad only.
         [Discard] [Save changes]
Off:     Delay value retained; editor disabled.
Invalid: Inline range error; Save changes disabled.
```

Keep desktop controls compact and touch hit areas at least 44 px. Phone fields
stack; the existing page scroll and floating save remain. No local save button.
Copy is illustrative and must be localized.

## Verification

Run from repository root, sequentially. Workspace dependencies are already installed.
Write failing tests first; rebuild before browser GREEN checks. Use isolated E2E
fixtures, never the main :9998 instance. A warm Go cache is available below.

```bash
(cd apps/backend && GOCACHE=/tmp/kandev-sidebar-go-cache go test ./internal/user/...)
(cd apps/web && pnpm exec vitest run hooks/domains/sidebar/use-sidebar-hover-reveal.test.ts components/app-sidebar/app-sidebar.test.tsx components/settings/sidebar-hover-settings-card.test.tsx components/settings/appearance-settings-state.test.ts components/settings/general-settings.test.tsx lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts lib/state/slices/settings/settings-slice.test.ts lib/state/hydration/hydrator.test.ts hooks/use-ensure-user-settings.test.ts lib/settings-discovery)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint hooks/domains/sidebar/use-sidebar-hover-reveal.ts components/app-sidebar/app-sidebar.tsx components/settings/sidebar-hover-settings-card.tsx components/settings/appearance-account-sections.tsx components/settings/appearance-settings-state.ts components/settings/general-settings.tsx lib/ssr/user-settings.ts lib/types/http-user-settings.ts lib/state/slices/settings/types.ts lib/settings-discovery/catalog/preferences.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
GOCACHE=/tmp/kandev-sidebar-go-cache make -C apps/backend build
GOCACHE=/tmp/kandev-sidebar-go-cache make -C apps/backend e2e-plugin-package
make build-web-e2e
(cd apps/web && pnpm e2e:run --host --no-build --project chromium e2e/tests/settings/sidebar-hover-settings.spec.ts e2e/tests/layout/sidebar-hover-reveal.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-sidebar-hover-settings.spec.ts e2e/tests/layout/mobile-sidebar-hover-reveal.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/user/{models/models.go,dto/dto.go,service/service.go,store/sqlite.go}`
  and the existing request adapter found from `UpdateUserSettingsRequest` callers;
  new `sidebar_hover_settings_test.go` in user store/service/handlers.
- `apps/web/lib/{types/http-user-settings.ts,ssr/user-settings.ts,state/slices/settings/types.ts}`
  and corresponding mapper, hydration, WS and settings slice tests.
- `apps/web/components/settings/{sidebar-hover-settings-card.tsx,appearance-account-sections.tsx,appearance-settings-state.ts,general-settings.tsx}`
  and card/draft/general-settings tests.
- `apps/web/lib/settings-discovery/catalog/preferences.ts`, its existing catalog tests,
  and `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/settings.json`.
- Existing sidebar hook and AppSidebar with tests, the two new settings E2E specs,
  `docs/public/tasks-and-workflows.md`, and this package's statuses/results.

## Dependencies

Task 01 is done. Read scoped backend and web AGENTS.md before implementation.
No separate schema or rollout work is required; use existing account settings.

## Risks

Pointer update fields must preserve explicit false/zero. Numeric drafts must not
coerce empty text into zero. Cancelling a revealed panel must safely return hidden
focus. Rebase logic must preserve dirty fields during live updates. Complete
wire payloads and accepted revisions must be consistent across all transports.

## Parallelism

sequential

## Inputs

- Paired hover requirements and system design, especially the settings extension.
- ADR 0041 portable settings and ADR 0046 route save coordinator.
- Existing status-bar settings persistence and appearance draft tests.
- `e2e/tests/settings/mobile-general-settings.spec.ts` for phone save geometry.

## Results

Completed on 2026-09-14.

| Required verification | Result |
| --- | --- |
| `go test ./internal/user/...` with the documented GOCACHE | Passed all six packages |
| Additional `go test ./internal/settingscatalog` and focused backend boot-projection tests | Passed |
| Targeted Vitest command above, including ensure-user-settings fixture coverage | 223 tests passed in 17 files |
| Typecheck and targeted ESLint | Passed; zero lint warnings |
| Traditional Chinese and pseudo generation, `i18n:check` | Passed; new copy complete in all supported locales |
| Backend build, E2E plugin package and `make build-web-e2e` | Passed |
| Chromium settings and hover specs | 5 passed, retries disabled |
| Mobile settings and navigation specs | 2 passed, retries disabled |
| Spec catalog, full spec lint, public-doc validator tests and public docs | Passed |
| `git diff --check` | Passed |

RED evidence: backend tests rejected missing defaults and silently ignored invalid
patches; frontend tests exposed fixed timers and missing wire/draft state; browser
RED confirmed the old frontend lacked the new toggle. Browser verification then
caught focus restoration running on explicit collapse; a failing hook regression
proved that path before it was restricted to preference changes. Final tests cover
both focus return on settings changes and normal pointer-exit collapse.

The existing settings navigation expands the sidebar when entering settings;
the disabled-hover browser case explicitly collapses it after returning home.
Desktop input height is 28 px; phone input/switch targets are at least 44 px with
zero page overflow. Rendered desktop and phone screenshots match the card grouping,
labels, units, disabled state and shared save flow in UI-03/UI-04.

Generated locale runs were limited in the diff to this feature's new keys;
unrelated regenerated translations were retained as they were. Builds retained
existing dynamic-import/chunk-size warnings. E2E artifact freshness checks required
rebuilding the plugin fixture after locale generation. Browser sockets required
sandbox escalation and used disposable E2E instances.

The existing seeded Tailscale test instance was refreshed separately, preserving
its database and shutdown script. Main instance :9998 was not restarted or changed.


## Appearance heading follow-up

Use the shared icon-bearing section heading above the Sidebar card. The backend
and frontend default remains 500 ms; the isolated test account had a saved 100 ms
value and was reset to 500 ms at the user's request.

Follow-up verification: 67 tests in three settings/mapper suites passed; targeted
ESLint, frontend build, spec lint and whitespace checks passed. Browser checks on
the existing Tailscale instance confirmed the icon-bearing heading is outside the
card, the saved delay is 500 ms, and the phone viewport has no horizontal overflow.
