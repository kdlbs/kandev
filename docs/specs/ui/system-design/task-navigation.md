---
status: current
system: ui
requirements:
  - REQ-UI-TASK-NAVIGATION-001
---

# Task navigation system design

## Purpose and boundaries

This design extends existing URL and client-navigation primitives. It does not
introduce a router or route catalog. UI owns the reusable interaction; task
identity and dependency semantics remain in the task system.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-TASK-NAVIGATION-001 | URL authority, Link authority, Enforcement, Responsive behavior |

## URL authority

`apps/web/lib/links.ts::linkToTask` remains the only first-party workbench URL
builder. Keep the current `(taskId, layout?)` call shape compatible. Add an
options overload containing `layout?`, `sessionId?`, and `searchParams?`
(`URLSearchParams`) for current canvas and inbox contexts. Clone caller-owned
parameters; explicit layout/sessionId values replace matching keys. Preserve
other keys and repeated values. Encode the raw ID as one path segment exactly
once. Callers pass raw IDs, never pre-encoded IDs. Test both overloads.

Route matching can still recognize `/t/` and `/tasks/`. Compatibility redirects
use the builder for their destination. `/tasks` listings, `/office/tasks/`,
API paths, external URLs, and stored user-authored content are separate contracts.
Do not globally rewrite strings or historical content.

## Link authority

Add `components/routing/task-link.tsx::TaskLink`, a thin wrapper around existing
`components/routing/app-link.tsx`. Accept `taskId`, builder options, normal
anchor presentation props, and `onNavigated`; omit `href` from public props.
The wrapper calls `linkToTask`, forwards its ref, and owns no task/store state.
Existing compliant `Link href={linkToTask(id)}` consumers may remain; new direct
task anchors should prefer TaskLink. Programmatic navigation continues through
`useRouter().push(linkToTask(...))`.

Extend AppLink with an optional `onNavigated` callback called inside the
existing successful `pushNavigationState` callback, after the location event.
It must not run for cancelled guards, prevented clicks, external navigation,
modified clicks, or alternate targets. TaskLink delegates all click decisions
to AppLink. Do not duplicate the router's guard or browser-modifier logic.

DependencyRow uses TaskLink. The chip supplies a stable close callback through
DependencyLists, invoked only after committed same-tab navigation. Keep browser
Back semantics and anchor accessibility. Missing-task and access errors continue
through the existing destination route.

## Enforcement

Add a local ESLint rule and register it as an error on production TS/TSX in
`apps/web/eslint.config.mjs`. Test it using the existing local-rule pattern.

Reject hand-built root-relative task-detail URL expressions outside links.ts:
string literals with a nonempty task segment, interpolated templates, and string
concatenation beginning `/t/` or `/tasks/`. Permit bare prefix constants used
for route recognition; exclude tests, E2E, generated files, comments, API and
Office paths. Replace constructed comparison URLs with linkToTask too.

Also reject native JSX anchors whose href resolves to a task route or a
linkToTask call, and window.location assign/replace/href writes with such a
value. Resolve import aliases and simple local const aliases. Report a clear
replacement suggestion. Rule fixtures cover direct, aliased, concatenated,
and conditional forms plus benign route matching and API paths.

This is bounded static enforcement, not whole-program URL analysis. Unknown
runtime strings remain outside its proof. Do not claim all possible dynamic
links are statically verified. Add a wiring test that evaluates the real ESLint
configuration on a synthetic new production component so new directories are
covered. Start at zero violations by migrating current builders in the same
work order; no growing baseline or broad per-directory exemptions.

## Responsive behavior

Keep the current dependency popover on fine pointers and useTouchDrawer on
coarse pointers. Reuse shared Drawer safe-area handling and the single internal
list scroller. The nearest mobile navigation exemplar is
`components/task/mobile/session-task-switcher-sheet.tsx`; the dependency chip
already uses the appropriate brief-choice drawer. Task selection completes the
choice and goes to the workbench, so successful navigation dismisses the drawer.
Keep its header and close action, constrain long lists to the viewport, and
apply 44px row sizing only on touch surfaces. No new persisted state or copy.

## Validation

Component tests prove canonical hrefs, click interception, guard cancellation,
callback timing, modifier handling, and encoding. Desktop and Pixel 5 E2E
follow links in both dependency directions, verify target task content, dismiss
the disclosure, and exercise Back. Set a unique window property before clicking
and assert it survives; also observe main-frame document requests. A correct URL
alone is not evidence against reload. Check touch targets and drawer containment.

## Related contracts

[Mobile task navigation](../requirements/mobile-task-navigation.md) owns broader
task actions and mobile board navigation. This contract adds shared task-link
transport and URL construction across desktop and mobile consumers.

## Related decisions and alternatives

[Navigation manifest boundaries](../../../decisions/2026-08-04-navigation-manifest-boundaries.md)
retains top-level destination ownership.
[Settings save coordination](../../../decisions/0046-settings-route-save-coordinator.md)
retains dirty-navigation authority. No new route ownership registry is needed.
A URL helper alone cannot prevent raw anchors from reloading. A component alone
cannot cover imperative redirects. Reusing both existing authorities with a
thin wrapper and lint enforcement addresses these distinct failure modes.

## Implementation plans

- [Task link navigation](../../../plans/task-link-navigation/plan.md)
