---
id: "01-phone-port-management"
title: "Move phone port management into Panels"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PORT-FORWARDING-DISCOVERY-001
acceptance_criteria:
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.1
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.2
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.3
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.4
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.5
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.6
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.7
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.8
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.9
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.10
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.11
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.12
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.13
  - AC-UI-PORT-FORWARDING-DISCOVERY-001.14
system_design:
  - ../../specs/ui/system-design/port-forwarding-discovery.md
---

# Task 01: Move phone port management into Panels

## Summary

Deliver phone Panels discovery and a bottom drawer using shared port state.
Keep opening independent from the header shortcut preference, preserving wider
launchers, plugin/canvas selection and existing port operations.

## In scope

- Panels command and phone-only navigation-entry removal.
- Preference-independent management host and preserve-open shortcut switch.
- Shared body with Drawer/Dialog wrappers, focus restoration and geometry.
- TDD, focused unit/E2E tests, localization and public how-to update.
- Reconcile current active-first design's phone wrapper, retain historical results.

## Out of scope

New API/schema, transports/tunnel lifecycle, automatic forwarding, mobile panel
IDs, desktop Dockview panels, plugin SDK changes and generic UI redesign.

## Acceptance

1. A phone task with no plugins reaches ports through Panels with shortcut off
   or on. Selection sends no preference write or content-panel change. Old phone
   task-navigation action is absent; tablet retains its existing action.
2. One Drawer exposes existing port operations and the separate preference
   without state loss, duplicate owners, focus errors or unsafe geometry. Context
   changes close management and reject stale results.
3. The plan's scenario matrix passes, wider launchers/Browser and plugin/canvas
   selection remain intact, all copy is translated, and public how-to matches.

## ASCII UI preview

From [full previews](plan.md#ascii-ui-preview), UI-01/UI-02 (criteria .2, .9-.12):

```text
PHONE UI-01
  [Chat] [Plan] [...] [Panels]
  Panels
  Task tools
  [network] Port forwarding              >
  (existing canvas/plugin options)

PHONE UI-02
  Port forwarding                     [X]
  | Forwarded ports (1)                 |
  | 3000 Forwarding [Open][Copy][Stop]   |
  | Other ports / Refresh / Manual add  |
  | Show in task header           [off] |
  (safe area)

WIDER UI-04
  [+]/tablet task sheet: checkable action
  Header [network] -> existing Dialog
```

Fixed title and one scrolling body; preference changes keep phone management
open. Include UI-03 disabled/empty/error variants from the plan. Grouping,
navigation and scroll ownership are structural; labels/sample data illustrative.
Inspect rendered screenshots against these previews.

## Verification

Run from repo root. In a fresh worktree without dependencies, first run
`(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm exec vitest run components/task/mobile/session-mobile-bottom-nav.test.tsx components/task/port-forwarding-visibility-provider.test.tsx components/task/port-forward-dialog.test.tsx components/task/port-forward-rows.test.ts components/task/port-forwarding-visibility.test.ts components/task/dockview-add-panel-items.test.tsx components/task/use-tunnel-actions.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/mobile/session-mobile-bottom-nav.tsx components/task/mobile/plugin-panel-picker.tsx components/task/mobile/session-task-switcher-sheet.tsx components/task/port-forward-dialog.tsx components/task/port-forwarding-visibility-provider.tsx components/task/port-forward-content.tsx components/task/use-port-forward-management.ts components/task/use-tunnel-actions.ts components/task/task-page-inner.tsx e2e/tests/plugins/mobile-plugin-task-panel.spec.ts e2e/tests/session/mobile-port-forwarding.spec.ts e2e/tests/session/port-forward-dialog.spec.ts e2e/tests/task/mobile-remote-repository-topbar.spec.ts e2e/pages/session-page.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
PATH=/usr/local/go/bin:$PATH NODE_OPTIONS=--max-old-space-size=4096 GOMAXPROCS=2 GOMEMLIMIT=512MiB scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project mobile-chrome tests/session/mobile-port-forwarding.spec.ts tests/task/mobile-remote-repository-topbar.spec.ts tests/plugins/mobile-plugin-task-panel.spec.ts
PATH=/usr/local/go/bin:$PATH NODE_OPTIONS=--max-old-space-size=4096 GOMAXPROCS=2 GOMEMLIMIT=512MiB scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/session/port-forward-dialog.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

Run managed E2E commands sequentially against fresh production builds; load the E2E
resource-safety reference before executing. If extraction adds files, extend the
exact unit/ESLint commands accordingly. Cover 320px, Pixel 5, 767px, narrow
fine-pointer and 768px coarse-pointer explicitly. Include keyboard viewport
shrink and desktop/phone/desktop transition assertions. Do not use device presets
alone as proof of keyboard clearance or breakpoint behavior. A failed-test or
new test-only rerun can use `--no-build` while production sources are unchanged.

## Files likely touched

- `apps/web/components/task/mobile/session-mobile-bottom-nav.tsx` and `.test.tsx`.
- `apps/web/components/task/mobile/plugin-panel-picker.tsx` and
  `session-task-switcher-sheet.tsx`.
- `apps/web/components/task/port-forward-dialog.tsx` and `.test.tsx`,
  `port-forwarding-visibility-provider.tsx` and `.test.tsx`, `port-forward-content.tsx`,
  `use-port-forward-management.ts`, `use-tunnel-actions.ts` and its tests, and the
  stable host in `task-page-inner.tsx`.
- `apps/web/e2e/tests/session/mobile-port-forwarding.spec.ts`,
  `port-forward-dialog.spec.ts`, `apps/web/e2e/pages/session-page.ts`, and
  `apps/web/e2e/tests/task/mobile-remote-repository-topbar.spec.ts`.
- `apps/web/e2e/tests/session/port-forwarding-helpers.ts` if delayed-response
  controls need extension; the plugin E2E retains
  Panels after plugin disable because task tools remain available.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`.
- `docs/public/tasks-and-workflows.md` (how-to),
  `docs/specs/ui/system-design/port-forwarding-active-first.md`, discovery pair,
  this work order and `plan.md` lifecycle/results.

## Dependencies

None. Existing active-first behavior is implemented and is the shared baseline.

## Risks

Header-off host unmount, duplicate state owners, stale-session reopen,
autofocus into removed rows, and narrow dock crowding. The plan names specific
regression scenarios; do not add duplicate phone business logic.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/port-forwarding-discovery.md).
- [System design](../../specs/ui/system-design/port-forwarding-discovery.md).
- [Active-first design](../../specs/ui/system-design/port-forwarding-active-first.md).
- `apps/web/AGENTS.md`, `/mobile-parity`, `/tdd`, `/e2e`, `/docs-maintainer`.

## Results

Completed in the primary session on 2026-10-09 after the user's explicit
implementation request. The primary session owns delivery; no delegation was used.

- Phone Panels includes the port command with no plugin/canvas requirement.
  Phone task/navigation actions are removed; tablet and desktop launchers remain.
- One controller above responsive headers owns runtime state and the manual draft.
  Drawer and Dialog share the body; shortcut changes preserve an open phone visit.
  Session changes reset state and reject delayed runtime/preference results.
- Seven real locales and generated pseudo include the three new messages.
  The public how-to and active-first design now describe the implemented flow.

Validation:

- Behavioral RED/GREEN: initial discovery/preserve-open/context tests failed on
  the old behavior; the scoped-pending test also failed before implementation.
  Final targeted unit evidence: 72 tests across seven files passed (63 in six
  files plus the nine visibility tests in a separate invocation).
- Typecheck passed with the recorded 4 GiB Node heap; the default 2 GiB run
  exhausted its heap. Focused ESLint and Prettier checks passed.
- `i18n:check` passed for all catalogs; `i18n:ratchet` passed with seven changed
  tracked copy files. Full copy checking also covered the extracted source files.
- Fresh-build managed mobile run: 15 passed, exit 0,
  `/tmp/kandev-run.e2e.W6mCqUKX.log`. Covers both entry points, real preference
  persistence/rollback, no writes on repeated opening, active-first operations,
  proxy/manual actions, readiness loss, archive-and-reopen, focus, 320px/Pixel 5/
  767px geometry, shortened keyboard viewport, 768px coarse-pointer tablet,
  expanded pseudo labels, plugin selection and remote topbar.
- Fresh-build managed Chromium run: 19 passed, exit 0,
  `/tmp/kandev-run.e2e.zQiLhEZn.log`. Covers original desktop/remote behavior,
  Browser actions, late hydration, invalid/duplicate/manual ports, preference
  reload, desktop/phone/desktop draft continuity and 767px fine-pointer phone.
- The localized hit-area E2E failed before enlarging the switch's invisible
  vertical target to 44px; it passed in the final fresh-build mobile run.
  A screenshot-only rerun disabled capture-time animations: two existing cases
  passed, exit 0, `/tmp/kandev-run.e2e.vNSDGXr4.log`.
  Rendered phone screenshots were inspected against UI-01/UI-02: bounded inset
  drawer, fixed title, wrapped URLs, reachable actions and manual/preference body.
- Catalog validation, all spec lint, 36 spec-linter tests, public-doc validation,
  63 public-doc validator tests, delivery-reference coverage and whitespace checks
  passed. Required catalogs/specifications and plan lifecycle are reconciled.

Builds used host mode, one browser worker, `GOMAXPROCS=2` and `GOMEMLIMIT=512MiB`
inside the 8 GiB container. Test-only reruns reused the current build; final
mobile and Chromium evidence used fresh builds. Keyboard clearance uses a
shortened browser viewport, not an actual OS keyboard or a Safari device run.
