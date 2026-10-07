---
created: 2026-10-05
status: implemented
requirements:
  - REQ-UI-PORT-FORWARDING-ACTIVE-FIRST-001
system_design:
  - ../../specs/ui/system-design/port-forwarding-active-first.md
legacy_specs: []
---

# Implementation Plan: Active-first port forwarding

## Overview

Ship one vertical slice: unify the port projection, surface active tunnels first,
and prove desktop/phone behavior. One sequential work order keeps row identity,
mutation behavior, translations, and rendered evidence together.

Inputs: [requirements](../../specs/ui/requirements/port-forwarding-active-first.md)
and [design](../../specs/ui/system-design/port-forwarding-active-first.md).

## Scope

### In scope

Active-first grouping/count, deterministic deduplication/order, tunnel-only
visibility, successful start/stop transitions, responsive actions, localization,
session isolation, focused tests, and a short public usage section at implementation.

### Out of scope

Transport/lifecycle changes, polling, persistence, bulk actions, new settings,
top-bar redesign, and new overlay types.

## Technical approach

Add a pure `port-forward-rows.ts` projection alongside the dialog; test its union,
deduplication, provenance, active partition, numeric sorting, and input immutability.
Wire it into `PortListSection`, retaining stable port keys under one React parent
and inserting keyed group headings. `port-forward-list.tsx` owns list presentation within the existing TS limits. Active URL/status precede proxy access.
Retain manual targets for stopped tunnel-only rows during the dialog visit.

Keep `useTunnelActions` as the mutation path and protect session-scoped results
and initial hydration against newer mutations. A session-keyed control owns the
map and dialog content. Merge initial hydration for untouched targets while
retaining locally changed targets and deletions.
Preserve existing API semantics and cancelled hydration response protection.
Extend `PortUrlActions` only for scoped phone/coarse-pointer layout and sizing.
Preserve proxy URL construction and capability-gated Browser navigation.

Add task copy in all seven real locale catalogs and regenerate pseudo; use the
Traditional Chinese generator. Add a short how-to subsection to
`docs/public/tasks-and-workflows.md` explaining the two groups, Open/Copy/Stop,
and the distinction between a proxy link and a dedicated tunnel. Public docs
were unchanged during planning and are updated alongside implementation.
No ADR is needed for this local presentation change within existing boundaries.

## ASCII UI preview

UI-00: Current desktop list, source-verified from `PortListSection`. Entry: task
port-forwarding control, dialog open with a low inactive detected port and a high
manual forwarded port.

```text
Port forwarding                             [Close]
Listening ports                           [Refresh]
3000  Detected                        [Start tunnel]
  Proxy URL                               [Copy/Open]
9000  Manual                           [Stop tunnel]
  Proxy URL                               [Copy/Open]
  Tunnel URL                              [Copy/Open]
Add port manually                     [port] [Add]
```

UI-01: Proposed desktop, same entry and mixed-port state.

```text
Port forwarding                             [Close]
                                          [Refresh]
Forwarded ports (1)
9000  Manual  Forwarding               [Stop tunnel]
  Tunnel: https://host:49152/              [Copy/Open]
  Proxy:  .../port-proxy/session/9000/      [Copy/Open/Browser]
Other ports
3000  Detected                        [Start tunnel]
  Proxy:  .../port-proxy/session/3000/      [Copy/Open/Browser]
Add port manually                     [port] [Add]
```

UI-02: Proposed phone, active task drawer enable or task top-bar control.

```text
+---------------------------------+
| Port forwarding         [Close] |
|                       [Refresh] |
| Forwarded ports (1)             |
| 9000  Manual  Forwarding        |
| Tunnel: https://host:49152/     |
| [Open] [Copy]     [Stop tunnel] |
| Proxy: .../session/9000/        |
| [Open] [Copy]                   |
| Other ports                    |
| 3000  Detected                  |
| [Start tunnel]                 |
| Proxy: .../session/3000/        |
| [Open] [Copy]                   |
| Add port manually              |
| [port number          ] [Add]  |
+---------------------------------+
```

The dialog title/dismiss controls remain fixed; the list/manual-add body is one
scroller. Phone URL/action stacking, group order, dedicated URL priority, status
text, count, and touch hit areas are structural. Spacing, example ports, icon
representation, and shortened URLs are illustrative. Browser actions follow
existing capability gating and are omitted on phones that lack that capability.
Use the existing dialog as temporary configuration/management, with `100dvh`
bounds and safe-area padding; retain the task-switcher drawer entry.

UI-03: State variants applying to both compositions.

```text
No active tunnels: Other ports -> rows -> Add port manually
Detection loading: Forwarded ports (1) -> known row; Refresh busy
No detected ports: known forwarded/manual rows stay; detection feedback below controls
Start pending: Other ports -> row with busy Start, no active promotion
Stop pending: Forwarded ports (1) -> row with busy Stop, no demotion
Action failure: same group/count/URL retained; existing error toast
```

UI-01/UI-02 cover AC .1-.3 and .6-.7; UI-03 covers AC .4-.5. Session-switch
scenarios cover AC .8. No layout animation or automatic scroll-to-top.

## Tests

All IDs below have prefix `AC-UI-PORT-FORWARDING-ACTIVE-FIRST-001`.

| AC | File / proposed test outcome |
| --- | --- |
| .1, .2, .5 | `components/task/port-forward-rows.test.ts`: sorts active before other; deduplicates all sources; retains tunnel-only targets; handles empty inputs without mutation |
| .2, .5 | `components/task/port-forward-dialog.test.tsx`: detection resolves before tunnel list; empty detection retains known forwards |
| .3, .7 | Same component test: localized active heading/status, count, dedicated URL precedence, proxy/Browser regression |
| .4, .7 | Same component test with real `useTunnelActions` and deferred API mocks: success/failure/pending start/stop ordering, stopped tunnel-only retention, focused row identity |
| .8 | Same component test: switch session during held list/refresh/start response; discard old data and protect newer mutations from initial hydration |

Use defect-specific behavioral RED assertions before implementation. The existing
dialog test mocks away `DialogHeader` and `useTunnelActions`; strengthen those
fixtures for new assertions rather than treating mocked mutation handlers as proof.

## E2E tests

Extend existing `e2e/tests/session/port-forward-dialog.spec.ts` (chromium) and
`mobile-port-forwarding.spec.ts` (mobile-chrome). Use a bounded, session-correlated
WS route helper `port-forwarding-helpers.ts` beside these specs to supply known
detected ports and a confirmed high-numbered tunnel. Follow
`e2e/helpers/ws-response-hold.ts`: intercept only the port requests for the seeded
session, preserve request/response identity and all unrelated traffic. Install
before navigation. Hold/release responses by an explicit controller rather than
sleeping. Do not depend on host listeners or fixed available local bind ports.

- Desktop: mixed detected/manual/tunnel sources, active rows before lower inactive
  rows, one row per target; Start/Stop and failure leave correct ordering/count
  (AC .1-.5). Keep existing proxy/Browser tests passing. Use a real tunnel in the
  existing lifecycle scenario; synthetic fixtures prove deterministic ordering.
- Phone: enter through task drawer, inspect active group, open/copy dedicated URL,
  stop then restart via taps, verify target order and no overflow (AC .1-.6).
  Check actual 44px hitboxes, long URL wrapping, internal scrolling to manual
  addition, safe-area clearance, and focus return (AC .6-.7).
- Desktop: keyboard traversal from opener, successful row move with focus retained,
  delayed hydration after a successful Start retains both the new and untouched
  existing forwards (AC .7-.8). Session replacement is owned by the integrated
  component test with deferred list, refresh, and Start responses; the browser
  case adds distinct WebSocket transport coverage for snapshot merging.

Use stable row test IDs and explicit action names/IDs. Replace the page object's
`.getByRole("button").first()` tunnel selector with a stable tunnel-control
selector if the new layout changes button order. Capture desktop/phone screenshots
through `test.info().outputPath(...)` and inspect them against UI-01/UI-02.

## Work orders

- [x] [Task 01: Active-first port management](task-01-active-first-port-management.md)

Execution: sequential; no delegation authorized. Estimated implementation and
targeted verification: 60-90 minutes with workspace dependencies installed.

## Verification results

Design-package checks on 2026-10-05:

- `python3 scripts/list-docs.py validate`: passed (351 decisions, 1354 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/port-forwarding-active-first`: passed;
  status inspection confirmed all four new package files are untracked/uncommitted.
- Catalog lookup found both new UI artifacts. `.github/scripts/pr-docs.cjs`
  `validateCoverage` returned `covered` with no errors using these four artifacts
  plus a prospective modification of `port-forward-dialog.tsx`; this validates
  work-order/plan/design/REQ/AC references, not implementation behavior. The
  actual docs-only change is exempt from that CI delivery gate.

Implementation authorized on 2026-10-06. The [work-order results](task-01-active-first-port-management.md#results)
record behavioral RED/GREEN evidence, 26 passing unit tests, passing final
coverage of all 16 desktop and two phone E2E cases, inspected screenshots, and
passing lint/type/i18n/documentation gates. The last full desktop run passed
15/16; its setup-crash case passed in isolation. Production-source builds passed
before environment-native crashes; the final freshness-checked desktop run
reused that same build. See the work order for exact commands and limitations.

## Risks

- Group-container remounts can lose keyboard focus and inline input state.
- Asynchronous tunnel reads can arrive after detection or overwrite a newer local mutation.
- Host listeners vary; deterministic E2E fixtures must preserve unrelated WS traffic.
- Narrow layouts can clip actions unless URL text owns a separate line.
