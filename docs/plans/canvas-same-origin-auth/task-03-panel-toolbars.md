---
id: "03-panel-toolbars"
title: "Unify panel toolbars and canvas chrome"
status: done
wave: 3
depends_on:
  - "02-proxy-verification"
plan: "plan.md"
requirements:
  - REQ-UI-PANEL-TOOLBARS-001
  - REQ-CANVASES-AGENT-WEB-APPS-006
  - REQ-CANVASES-AGENT-WEB-APPS-007
acceptance_criteria:
  - AC-UI-PANEL-TOOLBARS-001.1
  - AC-UI-PANEL-TOOLBARS-001.2
  - AC-UI-PANEL-TOOLBARS-001.3
  - AC-UI-PANEL-TOOLBARS-001.4
  - AC-UI-PANEL-TOOLBARS-001.5
  - AC-UI-PANEL-TOOLBARS-001.6
  - AC-CANVASES-AGENT-WEB-APPS-006.10
  - AC-CANVASES-AGENT-WEB-APPS-006.11
  - AC-CANVASES-AGENT-WEB-APPS-007.5
system_design:
  - ../../specs/ui/system-design/panel-toolbars.md
  - ../../specs/canvases/system-design/canvas-host-chrome.md
---

# Task 03: Unify panel toolbars and canvas chrome

## Summary

Extend the existing panel toolbar and migrate inconsistent primary headers.
Render one canvas header and retain state, actions, and recovery in their
appropriate locations. Finish the migration with real browser geometry checks.

## In scope

- Shared 30px desktop and 48px touch panel header shell; retain footer geometry.
- Audit every existing header caller and padding-sized primary task-panel header.
- Migrate all primary toolbar rows in the design inventory; document exceptions.
- Reconcile oversized editor controls, review wrapping, narrow browser fields,
  terminal header sizing, and overflow in other existing shell consumers.
- Add explicit embedded presentation to CanvasHostRoute and its Dockview caller.
- Remove the separate status toolbar; preserve one standalone page header.
- Move phone actions into that header and retain the existing inset drawer.
- Keep blocking status and Retry in the body; preserve polite announcements.
- Update existing component and browser selectors for the new state presentation.
- Update applicable public canvas guidance only if its descriptions/screenshots
  show the removed row; preserve translations across all five locales for new copy.

## Out of scope

Global page-header resizing, Dockview tabs, footers, chat composers, nested
per-file diff headers, third-party app chrome, runtime authorization changes,
and new telemetry or density preferences.

## Acceptance

1. All primary panel toolbars in the inventory use the shared geometry and retain
   accessible actions at narrow widths. Browser evidence covers each migrated family.
2. Embedded and standalone canvas views have one host header, no status-only row,
   and functioning actions, loading, Ready announcement, unavailable state and Retry.
3. Phone and coarse-pointer controls fit their rows with at least 44px targets;
   resize and status changes preserve runtime/editor state and focus.

## ASCII UI preview

UI-01 excerpt from [the plan](plan.md#ascii-ui-preview), covering all listed ACs.

```text
Desktop below Dockview tabs:
| Coordinator  [Releases] [...] | [Diff] [Review] [...] | 30px
| Canvas content or error      | Changes content       |

Phone standalone:
| Back  Coordinator      [...] | one navigation header
| Canvas content or error      |
| [Try again] when unavailable |
[...] -> existing inset actions/picker drawer
```

Panel toolbars in touch layouts use 48px; page navigation retains its separate
contract. Title truncation and action overflow must not create a second row.
Content owns scrolling; controls remain outside it. Ready has no visible strip.

## Tests

- Extend `canvas-host-components.test.tsx` and `canvas-host-route.test.tsx` for
  one header, single blocking-state message, action reachability, and announcements.
- Add `components/task/panel-primitives.test.tsx` for prop/ref forwarding and
  retained child interaction; do not use class assertions as geometry evidence.
- Extend affected existing editor, plan, files, changes and preview component tests
  for action preservation when secondary controls move into overflow.
- Add `e2e/tests/panel-toolbars.spec.ts`: measure border-box heights and adjacent
  top/bottom edges within 1px, at standard root font. Exercise every migrated
  family, narrow Dockview widths, long labels, multiple PRs, dirty editor state,
  disabled/busy actions, keyboard overflow access, and canvas Retry.
- Add `e2e/tests/mobile-panel-toolbars.spec.ts`: 390px, 767px, 768px and 900px
  coarse-pointer layouts. Assert computed styles, actual hitboxes, row containment,
  no document horizontal overflow, drawer action activation and focus return.
  Use a fine-pointer desktop context separately to prove the 768px boundary.
- Run the existing canvas suites alongside new tests. Replace every assertion
  targeting the removed visible status row; do not weaken startup assertions.

## Verification

Install workspace dependencies if missing. Rebuild the application before E2E.
The broader component directory command covers all migrated caller tests.

```bash
(cd apps/web && pnpm exec vitest run components/task components/editors components/settings/canvas-host-components.test.tsx components/settings/canvas-host-route.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
make build-web
make build-backend
(cd apps/web && pnpm e2e:run --project chromium tests/panel-toolbars.spec.ts tests/canvas/plugin-canvas.spec.ts tests/canvas/canvas-authenticated-proxy.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/mobile-panel-toolbars.spec.ts tests/canvas/mobile-plugin-canvas.spec.ts tests/canvas/mobile-canvas-authenticated-proxy.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/panel-primitives.tsx` and its new tests.
- All primary toolbar callers listed in the design's migration inventory.
- `apps/web/components/task/dockview-panel-content.tsx`.
- `apps/web/components/settings/canvas-host-route.tsx` and its tests.
- `apps/web/components/settings/canvas-host-frame.tsx`.
- `apps/web/components/settings/canvas-host-components.tsx` and its tests.
- `apps/web/e2e/tests/panel-toolbars.spec.ts` and `mobile-panel-toolbars.spec.ts`.
- Existing canvas E2E files with obsolete status selectors.
- Locale catalogs only when copy or accessible names change.

## Dependencies

Task 02 supplies the final authenticated startup behavior and browser fixture.
The shared geometry itself has no runtime-auth dependency; sequential delivery
avoids duplicating final canvas state assertions during integration.

## Risks

CSS overrides can defeat fixed sizes. Narrow panels need width-aware overflow;
a phone breakpoint alone is insufficient. Live-region changes must not produce
duplicate announcements. Header updates must not remount the canvas or editor.

## Parallelism

`sequential`

## Inputs

- [Toolbar design and source audit](../../specs/ui/system-design/panel-toolbars.md).
- [Canvas host design](../../specs/canvases/system-design/canvas-host-chrome.md#single-canvas-host-header).
- Existing `PanelHeaderBarSplit`, `MobileCanvasActions`, and mobile file viewer.

## Results

Implemented shared panel header primitives with 30px fine-pointer desktop and
48px coarse-pointer touch geometry, forwarded attributes and refs, width-aware
overflow, and pointer-sized caller controls. Migrated the primary Files,
Changes, Browser, terminal, file viewer, preview, and editor toolbar callers.

CanvasHostRoute now has explicit embedded and standalone compositions. Each
canvas has one host header, blocking states render in the body, and Ready is
announced through the existing accessible status channel. Mobile actions remain
in the existing inset drawer and the canvas body retains its scroll ownership.

Added desktop geometry coverage at standard and 768px fine-pointer widths and
mobile coverage at 390px, 767px, and 900px coarse-pointer widths. The tests
measure header height, adjacent content edges, hitboxes, focus/navigation, and
document overflow.

Checks passed:

- `pnpm exec vitest run components/task components/editors components/settings/canvas-host-components.test.tsx components/settings/canvas-host-route.test.tsx` (493 files, 4645 tests, 4 skipped)
- `pnpm run typecheck`
- `pnpm run lint`
- `pnpm run i18n:check`
- `make build-web`
- `make build-backend`
- Desktop browser coverage: 11 passed.
- Mobile browser coverage: 7 passed.
- `python3 scripts/list-docs.py validate`
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

Review remediation added phone-width media sizing for fine-pointer layouts,
actual control-containment assertions at 390px and 767px, and width-aware
overflow menus for canvas, Changes, Browser, and both editor toolbars. The
multiple-PR Changes header keeps its selector as the primary left action while
moving secondary actions into the menu. Dirty editor state keeps Save visible,
and the editor overflow test opens the menu and activates a secondary action.
The canvas lifecycle suite also verifies the neighboring narrow Dockview
header and its action menu.

The refreshed checks passed:

- Focused frontend tests: 17 files, 62 tests; the Monaco toolbar test now has 6 passing cases.
- Desktop panel-toolbar browser coverage: 5 passed.
- Mobile panel-toolbar browser coverage: 2 passed.
- Desktop canvas and authenticated-proxy browser coverage: 9 passed.
- Mobile canvas and authenticated-proxy browser coverage: 5 passed.
- `make build-web` and `make build-backend`
- `go test ./internal/plugins ./internal/plugins/webapp ./internal/mcp/canvasskill ./internal/backendapp`
- `pnpm run typecheck`, `pnpm run i18n:check`, and targeted ESLint
- Public-document validation, specification catalog validation, specification lint, and `git diff --check`
