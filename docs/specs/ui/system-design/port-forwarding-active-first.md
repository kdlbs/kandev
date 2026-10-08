---
status: current
system: ui
requirements:
  - REQ-UI-PORT-FORWARDING-ACTIVE-FIRST-001
---

# Active-first port forwarding System Design

## Purpose and boundaries

Extend the existing port-management presentation rather than the tunnel runtime.
The [requirements](../requirements/port-forwarding-active-first.md) own ordering,
status, and responsive interactions. Existing task visibility and proxy-to-Browser
behavior remain governed by [discovery](../requirements/port-forwarding-discovery.md)
and [Browser access](../requirements/port-proxy-browser-panel.md).

## Requirement mapping

| Criteria under REQ-UI-PORT-FORWARDING-ACTIVE-FIRST-001 | Design section |
| --- | --- |
| .1, .2, .5 | Projection and ordering |
| .3, .6, .7 | Presentation and responsive behavior |
| .4, .8 | State transitions and response scope |

## Components and responsibilities

- `apps/web/components/task/port-forward-dialog.tsx`: `PortForwardButton` gates a
  session-keyed `SessionPortForwardControl` that owns `activeTunnels` hydration;
  `PortForwardDialogContent` retains
  detected/manual state, refresh, and existing action wiring. `PortListSection`
  consumes the derived rows rather than separately mapping detected and manual lists.
- `apps/web/components/task/port-forward-rows.ts`: a pure, dependency-neutral
  projection using `ListeningPort`, manual target numbers, and the active tunnel map.
  Keep formatting and translation out of it. `port-forward-list.tsx` owns
  list/row presentation within the existing function/file limits.
- `use-tunnel-actions.ts` remains the single mutation path.
  `port-forward-dialog-actions.tsx` retains Open/Copy and Browser capability gating.
- `lib/api/domains/port-api.ts` continues to supply `ListeningPort`, `TunnelInfo`,
  `port.list`, and existing tunnel list/start/stop requests. No wire or backend change.

## Projection and ordering

Build one non-mutating union keyed by numeric target port on every source change:
detected targets, explicit manual targets, and current `activeTunnels` keys.
Detected metadata wins over manual provenance; otherwise use the existing Manual
badge. Duplicate detected records retain the first record's metadata, matching
the supplied source order. Attach tunnel ports by target key, independent of detection.
Partition active versus other rows and sort each by numeric target ascending.

Return row data including target, optional address/process, provenance, and
optional tunnel port. Count forwarded rows from this same projection. The
refresh callback must not be the gate that makes tunnel-only targets appear;
late `listTunnels` hydration must update the union immediately.

Retain successfully stopped tunnel-only targets in dialog-local manual state
for the rest of that visit. Reuse the existing manual-state retention pattern,
reconciling it when active tunnel keys arrive, rather than persisting runtime history.
Do not mutate source arrays or Maps while sorting.

## Presentation and responsive behavior

Use the existing theme, typography, Badge, Button, Input, and Dialog primitives.
The scene is a developer checking a running app during focused work, so retain
the user's theme and use restrained existing semantic tokens. Add a Forwarded
ports heading/count only while the active group has rows; use Other ports for
the remaining detected/manual targets. Keep refresh reachable above the list.
Empty detection feedback belongs beside the detection controls and must not
replace known rows. Render no empty active group or ornamental nested section cards.

Forwarded rows show Forwarding text with a connected icon; the tunnel URL and
Open/Copy actions precede the proxy row. Preserve detected/manual badges and the
existing proxy-only Browser action. Stop stays explicitly reachable. On desktop,
rows remain compact and ordinary controls are 28px. On phones, put each URL on
its own line and actions beneath it, with at least 44px targets. Use the canonical
phone/coarse-pointer conditions rather than `sm:` alone for touch sizing.

The nearest shipped surface is the existing port dialog, exercised by
`e2e/tests/session/mobile-port-forwarding.spec.ts`. Retain its Dialog because this
is temporary configuration plus multi-action management, rather than a short
picker. Reuse fixed-header, `min-h-0` scroll-body, dynamic viewport, and safe-area
geometry from `components/task/mobile/mobile-picker-sheet.tsx` without changing
overlay type or introducing another state owner. One content scroller contains
both groups and manual addition. Long values wrap or truncate within their own
line; controls never shrink out of reach. Browser-panel availability stays
capability-based, including its current phone behavior.

Keep each `PortRow` under the same React parent with `key={port}` across both
logical groups; insert keyed headings in that list rather than migrating rows
between independently mounted group containers. This preserves row state and
focus as grouping changes. Maintain visible focus, localized accessible names,
Escape/dismiss focus restoration, and no reorder animation. Reuse the existing
mutation toast feedback rather than adding duplicate live announcements.

## State transitions and response scope

Confirmed active map entries determine grouping. Pending Start remains under
Other ports; successful Start adds the entry and promotes it. Pending Stop stays
forwarded; successful Stop removes the entry and demotes it. Errors leave the map
and grouping intact while `pendingTunnels` prevents repeated actions. The main toggle uses
`aria-disabled` with an action guard so pending operations retain focus.

Keep the cancelled-response guard in `SessionPortForwardControl`. Key
the control by session identity so detected/manual state cannot leak
when a session changes while the dialog is open. Ensure async refresh/mutation
results cannot write into a replacement session: use the existing effect
cancellation pattern and scope the active-map setter to its initiating session.
Prevent an initial tunnel-list response that began before a successful local
mutation from overwriting that newer map. Track successfully mutated targets
within the session owner and merge hydration for untouched targets, retaining
newer local values or deletions for touched targets. This preserves other
existing tunnels when a Start succeeds before the initial tunnel read resolves.
No additional fetches, subscriptions, polling, or tunnel lifecycle changes are needed.

Detection/list clients currently convert errors into empty arrays. Preserve that
API behavior in this scope: an empty port result cannot remove active-map rows.
Do not claim to detect externally stopped tunnels or independently verify service
health. The current session runtime remains authoritative within its existing reads.

## Localization, persistence, and security

Add new task-namespace copy in English, pt-pt, zh-cn, zh-hk, zh-tw, ja, ko, and
the generated pseudo locale. Generate the Traditional Chinese pair through
`pnpm run i18n:zh-hant`; use `t()` at render time with `count` where pluralized.
Use existing clipboard/URL helpers and access authorization. No settings,
database schema, permissions, logs, metrics, or runtime flag changes.

## Implementation Plans

- [Active-first port forwarding](../../../plans/port-forwarding-active-first/plan.md)
