---
created: 2026-10-09
status: implemented
requirements:
  - REQ-UI-PORT-FORWARDING-DISCOVERY-001
system_design:
  - ../../specs/ui/system-design/port-forwarding-discovery.md
legacy_specs: []
---

# Implementation Plan: Mobile port forwarding

## Overview

Move phone port management into session Panels and a native bottom drawer.
One sequential vertical work order delivers discovery, shared management state,
responsive composition, localization, and focused regression tests.

The user approved Panels beside session tools and requested implementation.
The primary session owns the implementation and validation; no delegation or
publishing is part of this package.

## Scope

### In scope

- Panels command even without canvases/plugins; remove phone navigation entry.
- Open management independently of the optional task-header shortcut.
- Phone Drawer with existing active-first content, state and port operations.
- Preserve wider launchers, plugin/canvas selection, and current panel state.
- Unit/E2E checks, translated copy and shipped how-to update.

### Out of scope

New runtime transports, automatic/durable tunnels, embedded phone browsers,
desktop Dockview panels, new runtime flags, and general dock redesign.

## Technical approach

`SessionMobileBottomNav` and `PluginPanelPicker` provide the command.
`SessionTaskSwitcherSheet` omits only the phone action. The visibility provider
retains one open state and a separate shortcut preference. `TaskPageInner` hosts
`PortForwardingManager` above responsive headers. Its controller runs runtime
reads while eligible and enabled/open, including with preference off. Shared
body state survives header preference and viewport changes under Drawer/Dialog
wrappers. Keep `PortListSection`, `useTunnelActions`, row projection, clipboard
helpers, hydration race guards and Browser capability gating.

No schema, API, `MobileSessionPanel`, plugin SDK, or Dockview persistence change.
The completed active-first package remains historical delivery evidence;
its current system-design phone-wrapper paragraph now describes this delivery,
without rewriting its previous run results. No ADR is needed for this local
presentation choice; existing system and runtime ownership remain intact.

## ASCII UI preview

### UI-01: Phone discovery, ready task with no plugins

Entry: session bottom dock with Chat selected. Before is source-based.

```text
BEFORE: app/task navigation
  Tasks
  [Port forwarding (checked/off)]
  [filters] [task list...]

AFTER: session dock
  [Chat] [Plan] [...] [Terminal] [Panels]

  Panels
  Task tools
  [network] Port forwarding              >
  (existing canvas/plugin entries, if any)
```

Panels exists without plugins. Existing conditional Review/Status remain;
other content entries are abbreviated here. Picker has a fixed header and
one options scroller. The row opens, never toggles. At 320px contain any dock
crowding locally with 44px targets. Maps to .2, .3, .9, .11, .14.

### UI-02: Phone manager with a forwarded port

Entry: UI-01 row, header shortcut off.

```text
  Port forwarding                     [X]
  ---------------------------------------
  | Forwarded ports (1)        [Refresh] |
  | 3000 Forwarding                     |
  | http://host:49152/                   |
  | [Open] [Copy] [Stop]                 |
  | Other ports                         |
  | 5173                         [Start] |
  | Add port manually                   |
  | [Port number........] [Add]         |
  | Show in task header           [off] |
  | Shortcut preference for this task.  |
  ---------------------------------------
                  safe area
```

Inset bottom Drawer; fixed title/Close; the `|` region is one content scroller.
Existing proxy details remain available but are abbreviated in the sketch.
Preference changes keep this surface open. Maps to .5-.13 and active-first.

### UI-03: Phone disabled, empty, and error states

```text
  Panels
  [network] Port forwarding    (disabled)
  Session unavailable. / Task archived.

  Port forwarding                     [X]
  [Refresh] No listening ports detected.
  [Port number........] [Add]
  Show in task header              [off]

  Saving: preference switch disabled.
  Failed save: prior state + error toast;
  management stays open.
```

Copy is illustrative and localized. Empty detection must retain known active
rows. Maps to .3, .8, .10-.12 and existing active-first empty/loading criteria.

### UI-04: Wider compatibility

```text
  Desktop [+] -> checkable Port forwarding
  Tablet task sheet -> checkable action
  Header [network, if enabled] -> Dialog
```

Maps to .1, .4-.8, .14. Desktop ordinary controls stay 28px; phone/coarse-pointer
actions stay at least 44px. Grouping, entry points, scroll ownership, and
preference separation are structural; sample data/spacing are illustrative.

## Tests

| Discovery criteria | Unit evidence to add or retain |
| --- | --- |
| .2, .3, .9, .14 | `session-mobile-bottom-nav.test.tsx`: no-plugin Panels, off/on direct open, disabled reasons, existing plugin/canvas selection |
| .5, .7-.9, .13 | `port-forwarding-visibility-provider.test.tsx`: independent open, preserve-open toggle, rollback, cross-client metadata, stale context/readiness result |
| .6, .10, .13, .14 | `port-forward-dialog.test.tsx`: header-off host and scoped controller; retain hydration/mutation and Browser tests; E2E covers draft continuity |
| Existing projection | `port-forward-rows.test.ts`: retain active-first grouping/deduplication |

## E2E tests

| Flow | Criteria | File/project |
| --- | --- | --- |
| No plugins: Panels -> manager; repeated open sends no preference PATCH; app/task-picker action absent; preference off/on with reload; Open/Copy/Start/Stop/manual addition | .2, .5-.12, .14 | `session/mobile-port-forwarding.spec.ts`, mobile-chrome |
| Disabled/archive/readiness, failed preference, expanded localized labels, focus handoff, keyboard clearance, geometry at 320px/Pixel 5/767px and 768px coarse-pointer; unit tests cover late task/session results | .3, .8, .11-.13 | phone spec (including localized labels and 768px tablet); Chromium spec covers narrow fine-pointer and resize |
| Remote topbar retains context using Panels instead of old toggle | .2, .3, .9 | `task/mobile-remote-repository-topbar.spec.ts`, mobile-chrome |
| Plugin selection after added task-tools group | .14 | `plugins/mobile-plugin-task-panel.spec.ts`, mobile-chrome; canvas component test |
| Existing launcher/persistence/errors/Browser; desktop -> phone -> desktop preference and manual draft continuity | .1, .4-.8, .13-.14 | `session/port-forward-dialog.spec.ts`, chromium |

Use existing API seeding and session-correlated `routePortForwarding`.
Observe real preference requests causally. Scope locators to the visible wrapper;
measure actual hit areas, containment, and one scroller. Capture and inspect phone
screenshots against UI-01/UI-02, including long content and keyboard viewport.

## Work orders

- [x] [Task 01: Move phone ports into Panels](task-01-phone-port-management.md)

## Verification results

Design validation on 2026-10-09:

- `python3 scripts/list-docs.py validate`: passed (368 decisions, 1494 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Catalog lookup found the discovery requirement and paired design.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: covered, no errors, using
  these four documents and a prospective change to `port-forward-dialog.tsx`.
  This validates package references, not implementation behavior.
- `git diff --check` and package status inspection: passed; one modified
  requirement and three new documents, all unstaged/uncommitted.

The design checkpoint changed no production or permanent test files. The user
subsequently approved this package and requested implementation.

Implementation validation on 2026-10-09:

- Task 01 is done: Panels command, shared phone Drawer/wider Dialog controller,
  task-wide optional header shortcut, stale-context guards, localization and docs.
- 72 targeted unit tests passed; fresh-build browser runs passed 15 mobile/tablet
  and 19 Chromium cases. Typecheck, focused ESLint/Prettier, full i18n checking
  and the new-copy ratchet passed.
- Explicit rendered checks cover 320px, Pixel 5, 767px fine/coarse pointers,
  768px coarse-pointer tablet, shortened keyboard viewport, pseudo-label geometry,
  44px switch hit area, both dismiss-focus origins and responsive draft continuity.
- Catalog/spec/public-doc checks and delivery-reference coverage passed.
  See the work order for exact commands, RED/GREEN evidence and run logs.
- Requirements are active; both current designs and the public how-to match.
  No delegation was used. Commit, push and ready PR publication were separately
  authorized after the implementation handoff.

## Risks

- Gating the entire host on the shortcut would block header-off access.
- Autofocus can target an unmounted picker row during drawer handoff.
- Shared wrappers must preserve manual drafts while hiding shortcut/resizing.
- Eight conditional dock entries can crowd 320px widths.
- Remote-topbar E2E references the removed action and must migrate too.
