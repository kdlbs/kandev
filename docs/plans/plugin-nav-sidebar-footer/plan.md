---
spec: docs/specs/plugins/requirements/plugin-nav-sidebar-footer.md
created: 2026-08-12
status: superseded
---

# Implementation Plan: Plugin nav items in the sidebar footer icon row

> **SUPERSEDED BY AMENDMENT 1 (2026-08-12). Do not build from this file.**
>
> This plan and its three task files are the implementation record of the **first**
> version of the contract, which is already built and on this branch. The spec was
> amended afterwards (see `## Amendment log` in the spec), and this plan does **not**
> describe the amended contract. Two of its claims are now actively false:
>
> - it plans for the plugin-facing value `"insights"`; the accepted value is now
>   `"sidebar-footer"`, and `"insights"` deliberately degrades to the plugin rail;
> - it states no footer component changes are needed. The amendment adds an inline
>   budget (`MAX_INLINE_PLUGIN_FOOTER_ITEMS`) and an overflow menu to
>   `app-sidebar-footer.tsx`.
>
> The spec is the contract. Build the amendment from the spec, not from here. The
> `status: done` markers on the task files refer to the pre-amendment work only.

## Overview

The navigation manifest already has an `insights` `NavSection` — it is how the
first-party `stats` destination reaches the desktop sidebar footer and the phone
menu's Utilities group today (`apps/web/lib/navigation/core-destinations.ts`,
`apps/web/lib/navigation/types.ts`, `apps/web/lib/navigation/surface-policy.ts`).
Every surface that renders a section (the footer, the mobile Utilities group,
the plugin rail, the mobile Plugins group, the palette) already reads through
`resolveDestinations()` / `useStaticDestinations()`, which already merges
`pluginRegistry.getNavRegistrations()` in. The only place a `NavItem.section`
value is translated into that internal `NavSection` is
`pluginDestinations()` in `apps/web/lib/navigation/plugin-destinations.ts`, and
it does not have an `"insights"` case yet — an item with `section: "insights"`
today falls through to `"plugins"`.

So this is a narrow change: widen the `NavItem.section` union, add one branch
to `pluginDestinations()`, correct the six stale comments the spec's **Stale
prose at the edit sites** section names (they sit next to the lines being
touched and already contradict the widened contract), and add regression
coverage for the behavior this unlocks. No sidebar, footer, mobile-menu, or
palette component changes are needed — they inherit the new placement for
free because they were already built generically. No backend change: nav-item
sections are a frontend-only concept (`apps/backend/internal/plugins/manifest/validate.go`
validates an unrelated enum).

Order: (1) widen the contract and fix the mapping plus its adjacent stale
prose, since every other file depends on this being correct; (2) add
frontend regression tests proving the existing generic components render the
new placement correctly, including the ordering and capacity guarantees the
spec states as contract; (3) add a real end-to-end plugin registering an
`insights` item and prove it on both a desktop and a phone viewport. (2) and
(3) touch disjoint files and can run in parallel once (1) lands.

---

## Frontend

### Contract — `apps/web/lib/plugins/types.ts`

Widen `NavItem.section` from `"main" | "settings" | "integrations"` to add
`"insights"`, and extend the field's doc comment to describe all four values
(today only `"main"` and `"integrations"` are documented on this field —
`"settings"` is undocumented; the spec's stale-prose item 4 requires fixing
this in the same change): `"insights"` renders in the sidebar footer icon row
and the phone Utilities group, `"settings"` is accepted and renders nowhere.
`PluginNavRegistration` (`apps/web/lib/plugins/registry.ts`) extends `NavItem`
and needs no edit — it inherits the widened union.

### Mapping — `apps/web/lib/navigation/plugin-destinations.ts`

`pluginDestinations()`'s `section` mapping currently reads:

```ts
section: item.section === "integrations" ? ("integrations" as const) : ("plugins" as const),
```

Add the `"insights"` case so the ternary becomes a three-way mapping:
`"integrations"` → `"integrations"`, `"insights"` → `"insights"`, anything
else (`"main"`, omitted, or an unrecognised string) → `"plugins"`.
`"settings"` items are already filtered out before this line and stay
filtered out.

Also correct, in the same file:
- The docblock above `pluginDestinations()` (stale-prose item 2): it currently
  says `"settings"` items "render in the settings tree's `PluginSlot`" — false;
  that slot is fed only by `registerComponent("settings-nav", …)`. Replace
  with: settings-section items are skipped and render on no surface.
- The comment above `surfaces: SIDEBAR_AND_MENU` (stale-prose item 3): it cites
  `registerShortcut`, which does not exist in this repository. Replace with a
  statement that plugin destinations never declare the `palette` surface,
  without naming a substitute API.

### Docs — `docs/plans/plugins/PLUGIN-API.md` and `docs/public/plugins-authoring.md`

- `PLUGIN-API.md` (stale-prose item 1): update the `NavItem` interface
  Before/After snippet's `section` union to include `"insights"`, replace the
  false "Hosts predating a section value simply don't render items targeting
  it (additive change)" comment with the actual degrade-to-`"main"` rule, and
  add a line stating where `"insights"` renders.
- `docs/public/plugins-authoring.md` line 184 (stale-prose item 5, the one
  requirement rather than correction in this list): the `registerNavItem` row
  of the Frontend hook/API matrix currently reads *"section is main,
  integrations, or accepted-but-not-rendered settings"*. Add `insights` and
  state where it renders.

These three files plus `types.ts` are, per the spec's **API surface** section,
the complete contract-and-docs edit set.

---

## Tests

### `apps/web/lib/navigation/plugin-destinations.test.ts`

This is the sole module that reads `NavItem.section`, so it carries the
mapping-level coverage:

- **What:** a `section: "insights"` item routes to the `insights` `NavSection`
  on both `sidebar` and `mobileMenu` surfaces. **How:** extend the existing
  `pluginItems` fixture with an `insights`-section item and add a case
  alongside the existing `"main"`/`"integrations"`/`"settings"` assertions.
- **What:** an `insights` item does not also appear in the `plugins` section
  resolution (no double placement — the spec's "moves, does not add" rule).
  **How:** assert `pluginsIn("sidebar" | "mobileMenu", …)` excludes it, mirroring
  the existing `"routes main-section items…"` test.
- **What:** an `insights` item stays off the palette, like every other
  section. **How:** extend the existing `"keeps plugin items off the
  palette…"` test's fixture with an `insights` item and assert its id is
  absent; retitle the test per stale-prose item 6 (it currently claims
  plugins reach the palette "via shortcuts", which is false — there is no
  plugin route into the palette by any API).
- **What:** an unrecognised `section` string (e.g. `"footer"`) degrades to
  `plugins`, never to `insights` and never dropped. **How:** new test, one
  `pluginItems` entry with an out-of-union string cast, asserted present in
  `pluginsIn(...)` and absent from an `insights`-section resolution.
- **What:** two plugins each registering one `insights` item keep distinct
  destination ids and render in the order they were supplied. **How:** new
  test mirroring the existing `"keeps two plugins that pick the same item id
  apart"` test, using `section: "insights"`.

### `apps/web/lib/navigation/resolve-destinations.test.ts`

- **What:** the first-party `stats` destination precedes a plugin `insights`
  item when the sidebar resolves the `insights` section. **How:** new test,
  `resolveDestinations({ surface: "sidebar", section: "insights", pluginItems: […] })`,
  assert id order `["stats", "plugin:acme:board"]`.
- **What:** the phone Utilities group's manifest rows resolve as `stats`,
  `settings`, then the plugin `insights` item — the plugin row lands after
  Settings, not adjacent to Stats (spec **Ordering** section, phone scenario).
  **How:** new test extending the existing `"accepts several sections…"` test
  with a plugin `insights` item passed via `pluginItems`, surface `mobileMenu`,
  section `MOBILE_MENU_UTILITY_SECTIONS`.

### `apps/web/components/plugins/plugin-nav-items.test.tsx` (desktop plugin rail)

- **What:** an `insights`-section item is excluded from the plugin rail, like
  `"settings"` and `"integrations"` already are. **How:** new test
  `"keeps insights items out of the main plugin rail"`, following the existing
  `"omits a nav item registered for a non-main section"` pattern.

### `apps/web/components/plugins/mobile-plugin-nav-section.test.tsx` (phone Plugins group)

- **What:** an `insights`-section item is excluded from the phone Plugins
  group, matching the desktop rail (spec scenario: "moves, does not add" holds
  on both surfaces). **How:** extend the existing
  `"omits non-main sections…"` test with an `insights`-section registration
  alongside the existing `integrations`/`settings` ones, asserting the
  rendered output still excludes it (the test currently asserts the whole
  section is empty because its two fixture items are `integrations`/`settings`
  — verify the assertion still holds, or split into a dedicated case if adding
  a third non-empty-triggering item would change what's asserted).

### `apps/web/components/app-sidebar/app-sidebar-footer.test.tsx` (desktop footer)

The file already mocks `useStaticDestinations` to return a fixed `stats`-only
array (a deliberate seam: the manifest's own resolution is covered above, not
re-tested here). Add:

- **What:** a plugin `insights` destination renders as an icon button with
  accessible name = `NavItem.label`, `data-testid="sidebar-plugin:<pluginId>:<itemId>-button"`
  (falls out of the existing `sidebar-${destination.id}-button` templating
  once `destination.id` is the namespaced plugin id — no component change),
  and clicking it calls `router.push(destination.href)`. **How:** new test,
  mock `useStaticDestinations` to return `[stats, { id: "plugin:acme:board", label: "Acme Board", icon: IconChartBar, section: "insights", href: "/plugins/acme", source: "plugin", pluginItemId: "board" }]`.
- **What:** capacity guarantee — eight plugin `insights` destinations all
  render into the DOM with no cap, and the footer container carries its
  wrapping-row class when expanded and its column class when collapsed (spec
  **Capacity and overflow**). **How:** new test(s), mock eight synthetic
  plugin destinations, assert `getAllByTestId(/^sidebar-plugin:/)` has length
  8 for both `collapsed={false}` and `collapsed={true}` renders, and assert
  the container's class list matches the `flex-wrap` / `flex-col` expectation
  already encoded in `AppSidebarFooter`'s className logic.

### `apps/web/components/navigation/app-nav-sheet.test.tsx` (`AppNavSections` / phone Utilities group)

The file currently mocks `usePluginRegistry` to always return
`getNavRegistrations: () => []`. Change that mock to read a mutable
module-level array (reset in `beforeEach`) so individual tests can inject
registrations, then add:

- **What:** the phone Utilities group renders `Stats`, then `Settings`, then a
  plugin `insights` row, in that order (spec **Ordering**, phone scenario) —
  and the Status/theme/Improve Kandev/Health rows around them are unaffected.
  **How:** new test in the `describe("AppNavSections", …)` block, set the
  mutable registrations array to one `{ pluginId: "acme", id: "board", label:
  "Acme Board", path: "/plugins/acme", section: "insights" }`, render
  `<SectionsHost/>`, assert the relative DOM order of the three `Stats`,
  `Settings`, `Acme Board` links via their positions among
  `getAllByRole("link")`.

---

## E2E Tests

The shared Playwright fixture plugin (`apps/backend/cmd/plugin-fixture`,
packaged as `kandev-plugin-e2e`) registers one `section: "main"` nav item
today (`id: "e2e-hello"`, `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`).
Add a second `registerNavItem` call there with `section: "insights"` so real
install/boot/render exercises the new placement, then repackage
(`make -C apps/backend e2e-plugin-package`, which writes
`apps/backend/.build/kandev-plugin-e2e-1.0.0.tar.gz` — the same artifact every
existing plugin e2e spec installs from).

```js
registry.registerNavItem({
  id: "e2e-insights-tools",
  label: "E2E Insights Tools",
  path: "/plugins/e2e-hello",
  section: "insights",
});
```

Reuses the existing route/page (`/plugins/e2e-hello`) — this is a second nav
entry point into the same fixture page, not a new page, keeping the fixture's
existing route/component untouched.

- **Scenario:** installing the plugin and reloading shows its `insights` item
  as a footer icon button, and it does **not** also appear in the plugin rail.
  **File:** `apps/web/e2e/tests/plugins/plugins.spec.ts` (extend the existing
  `test.describe` block; reuse its `openInstallDialog`/`uploadPackage` helpers
  and the file's shared `afterEach` uninstall). **What to verify:**
  `testPage.getByTestId("sidebar-plugin:kandev-plugin-e2e:e2e-insights-tools-button")`
  is visible with accessible name `E2E Insights Tools`, clicking it navigates
  to `/plugins/e2e-hello`, and
  `testPage.getByTestId("plugin-nav-item-e2e-insights-tools")` has count 0.
- **Scenario:** on a phone, the same item appears as a labelled row in the
  Utilities group and does **not** appear in the Plugins group. **File:**
  `apps/web/e2e/tests/plugins/mobile-plugin-nav.spec.ts` (new test in the
  existing `test.describe("Mobile plugin navigation", …)` block, following
  the file's existing inline install pattern). **What to verify:** after
  install + reload + opening the menu sheet, a row with accessible name
  `E2E Insights Tools` is present and is not inside
  `getByTestId("mobile-plugin-nav-section")` (the Plugins group container).

---

## Verification Results

**Task 01 (contract and mapping) — done.** `pnpm --filter @kandev/web test --
lib/navigation/plugin-destinations.test.ts
lib/navigation/resolve-destinations.test.ts` → 2 files, 25 tests passed.
`pnpm run typecheck` (apps/web) → clean. `pnpm --filter @kandev/web lint --
<touched files>` → 0 errors/warnings. `pnpm run i18n:ratchet` → clean. See
task-01's `## Results` for detail.

**Task 02 (frontend regression coverage) — done.** `pnpm --filter @kandev/web
test -- <6 touched test files>` → 71 tests passed across 6 files. Typecheck,
lint, and i18n:ratchet all clean. No production component code changed — the
existing generic manifest-driven rendering absorbed the new section without
edits. See task-02's `## Results` for detail.

**Task 03 (E2E coverage) — done.** Rebuilt the e2e fixture plugin package and
the `kandev`/`mock-agent` binaries and web `dist/` this worktree lacked, then
ran the real Playwright suites: `plugins.spec.ts` (chromium) 12/12 passed,
`mobile-plugin-nav.spec.ts` (mobile-chrome) 5/5 passed, and
`plugins-docs-screenshots.spec.ts` 2 skipped (gated, pre-existing, confirms no
crash loading the new fixture bundle). See task-03's `## Results` for detail.

All three tasks are done. Plan status should move from `draft` to
`complete` once this is committed.

---

## Implementation Waves And Parallel Candidates

```
Wave 1:
- [x] [task-01-contract-and-mapping](task-01-contract-and-mapping.md)

Wave 2 (parallel candidates — user authorization required):
- [x] [task-02-frontend-regression-coverage](task-02-frontend-regression-coverage.md)
- [x] [task-03-e2e-coverage](task-03-e2e-coverage.md)
```

Wave 2 tasks touch disjoint files (unit test files under `apps/web/components`
and `apps/web/lib` for task 02; the Go fixture bundle, its packaged artifact,
and two e2e spec files for task 03) and both depend only on task 01's mapping
change landing first.

## Open Questions

(None — the spec's **Section-to-placement mapping** table and **Ordering**
section leave no implementation choice open.)
