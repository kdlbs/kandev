---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001
created: 2026-08-12
owners:
  - nova28
---
# Plugin nav items in the sidebar footer icon row System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## API surface

The only contract that changes is the `section` field of `NavItem` and the new named
type for its values. That is a statement about the *contract*, not about the file
count: the field is declared in `apps/web/lib/plugins/types.ts` and mirrored in
[`docs/plans/plugins/PLUGIN-API.md`](../../../plans/plugins/PLUGIN-API.md), so those two must
change together, and it is *documented* for plugin authors in
[`docs/public/plugins-authoring.md`](../../../public/plugins-authoring.md), which the **What**
section requires updating in the same change. Three files, one contract. Read neither this
paragraph nor the Before/After snippet below as the exhaustive edit list — that list is
**Stale prose at the edit sites**. `PluginNavRegistration`
(`apps/web/lib/plugins/registry.ts`) extends `NavItem` and inherits the change, so it
needs no edit of its own.

Before:

```ts
interface NavItem {
  id: string;
  label: string;
  path: string;
  icon?: string;
  section?: "main" | "settings" | "integrations";
}
```

After:

```ts
export type PluginNavSection = "main" | "settings" | "integrations" | "sidebar-footer";

interface NavItem {
  id: string;
  label: string;
  path: string;
  icon?: string;
  section?: PluginNavSection;
}
```

`PluginNavSection` SHALL be exported from `apps/web/lib/plugins/types.ts` so a plugin
author writing TypeScript can name the type, and SHALL appear in
`docs/plans/plugins/PLUGIN-API.md` in the same form. Nothing else about the plugin API
changes: no new hook, no new registry method, no manifest field.

### Stale prose at the edit sites — correct it in the same change

Several of the files this change touches carry prose that contradicts the contract above.
It sits inches from the lines being changed, so leaving it is shipping a
self-contradictory frozen contract, and a builder who noticed but had no instruction
would be inventing scope. The spec therefore requires all seven sites to **end** in the
state described below, and names them so nobody has to guess.

**Read each entry as a required end state, not as a guaranteed edit.** Some of these were
already brought into that state by the first (pre-amendment) implementation that is
already on this branch, so the honest instruction differs per entry and is marked on each:

- **VERIFY-NOT-EDIT** — already correct on this branch as of this amendment. Confirm the
  text still matches and move on. Finding nothing to change here is the expected outcome,
  not a sign you are on the wrong baseline. Entries **2**, **3** and **6**.
- **EDIT** — genuinely stale, verified stale at the time this amendment was written.
  Entries **1**, **4**, **5** and **7**.

Entry 1 is mixed and is annotated in place: its `PluginNavSection` / `"sidebar-footer"`
content is an EDIT, while its conditional *"Hosts predating a `section` value…"* clause is
already satisfied and is VERIFY-NOT-EDIT.

**This list is the complete edit inventory for prose and fixtures.** Together with the
mapping in `pluginDestinations()`, the union in `types.ts`, the footer's overflow
rendering and its exported `MAX_INLINE_PLUGIN_FOOTER_ITEMS`, the locale key, and the new
test coverage, it is everything this change writes.

1. **EDIT** (mixed — see the clause note). `docs/plans/plugins/PLUGIN-API.md`, the comment
   immediately above the `NavItem` interface the Before/After snippet replaces. It SHALL
   state the degrade-to-`"main"` rule, describe where `"sidebar-footer"` renders (matching
   the mapping table), and name `PluginNavSection`. The interface itself still declares
   `section?: "main" | "settings" | "integrations" | "insights"` and is genuinely stale.
   The older *"Hosts predating a `section` value simply don't render items targeting it
   (additive change)"* line is already absent — that clause is VERIFY-NOT-EDIT; if it ever
   reappears it SHALL be removed, because it contradicts the degrade rule and is false
   about the shipped host, which drops nothing.
2. **VERIFY-NOT-EDIT.** `apps/web/lib/navigation/plugin-destinations.ts`, the
   `pluginDestinations` docblock. It SHALL say `section: "settings"` items are skipped
   and render on **no** surface — not that they render in the settings tree's
   `PluginSlot`, which is false; that slot is fed only by
   `registerComponent("settings-nav", …)`. Already in this state on this branch; confirm
   and leave it.
3. **VERIFY-NOT-EDIT.** `apps/web/lib/navigation/plugin-destinations.ts`, the comment on
   the `surfaces: SIDEBAR_AND_MENU` line. It SHALL state that plugin destinations never
   declare the `palette` surface, without naming a substitute API, because there is no
   plugin route into the palette (see **What**). It SHALL NOT cite `registerShortcut`, an
   identifier that does not exist anywhere in this repository. Already in this state on
   this branch; confirm and leave it.
4. **EDIT.** `apps/web/lib/plugins/types.ts`, the doc comment on the `section` field. It SHALL
   document all four values, matching the mapping table: `"main"` (default) as a
   top-level sidebar entry, `"integrations"` inside the sidebar's Integrations section,
   `"sidebar-footer"` as an icon button in the sidebar footer's icon row and as a
   labelled row in the phone menu's Utilities group, and `"settings"` accepted and
   rendered nowhere. It SHALL also state that the footer placement is subject to the
   inline budget and that an over-budget item is reached through the footer's overflow
   menu, so an author is not surprised by an icon that is present but not inline.
5. **EDIT.** `docs/public/plugins-authoring.md`, the `registerNavItem` row of the Frontend hook/API
   matrix. It SHALL list `sidebar-footer` and say where it renders, matching the mapping
   table, and SHALL NOT mention `insights`. This is the public authoring surface the
   **What** section's documentation SHALL points at, and it is the enumeration a plugin
   author actually reads.
6. **VERIFY-NOT-EDIT.** `apps/web/lib/navigation/plugin-destinations.test.ts`, the title of
   the palette exclusion test. It already reads *"keeps plugin items off the palette"*. If
   it ever reads *"…which plugins reach via shortcuts"* again, that trailing clause is the
   same false claim as item 3 and SHALL be removed; the title SHALL state the exclusion
   without naming a substitute route. The assertion itself is correct and SHALL NOT change.
   (This file does gain new cases — the `"sidebar-footer"` → `insights` mapping row and the
   `"insights"`-degrades-to-`plugins` row — and it is one module under `apps/web` in which a
   **Type surface** import may live. It is **not** where the capacity scenarios go:
   `MAX_INLINE_PLUGIN_FOOTER_ITEMS` and the inline/overflow partition live in
   `app-sidebar-footer.tsx` per **Capacity and overflow**, which a pure mapping unit test
   cannot see, so those scenarios belong to the footer component's own test file
   (`app-sidebar-footer.test.tsx`, which already exists) or to the e2e specs. Do not import
   the footer component into this file to make them fit. Either way that is new coverage,
   not a correction, and is not what this entry governs.)
7. **EDIT.** `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`, the
   `e2e-insights-tools` registration. Its `section` SHALL change from `"insights"` to
   `"sidebar-footer"`. Without this the e2e fixture item silently relocates to the
   plugin rail and the desktop/phone footer e2e specs fail — a real behavioural
   consequence of this amendment, not a cosmetic one.

Items 1 to 4 and 6 are prose corrections; item 5 is a documentation requirement; item 7
is a fixture correction with behavioural effect. None of items 1 to 6 changes behaviour,
a signature, an exported symbol, or a test assertion, and nothing here is an
implementation decision left open. Of the seven, **four require an edit** (1, 4, 5, 7) and
**three are verify-only** (2, 3, 6); a builder who finds nothing to change in the
verify-only three has done that part correctly.

**The mapping lives in exactly one module.** `pluginDestinations()` in
`apps/web/lib/navigation/plugin-destinations.ts` is the sole place a `NavItem.section`
value is translated into a manifest `NavSection` — it is the only reader of that field
anywhere in the web app. The section-to-placement table below becomes code there and
nowhere else, which is why every other navigation module except the footer component is
listed under **Out of scope**.

### Section-to-placement mapping

This table is the whole placement contract. `NavItem.section` value on the left,
resulting internal navigation section and rendered surfaces on the right. The desktop
column reads "footer icon button" for items within the inline budget and "footer
overflow menu item" beyond it; see **Capacity and overflow**.

| `NavItem.section` | Nav section | Desktop sidebar | Phone menu | Palette |
|---|---|---|---|---|
| `"sidebar-footer"` | `insights` | footer icon button, or footer overflow menu item past the budget | Utilities group row | no |
| `"integrations"` | `integrations` | Integrations section row | Integrations group row | no |
| `"settings"` | *(skipped)* | no | no | no |
| `"main"` | `plugins` | plugin rail row | Plugins group row | no |
| omitted | `plugins` | plugin rail row | Plugins group row | no |
| `"insights"` | `plugins` | plugin rail row | Plugins group row | no |
| any other string | `plugins` | plugin rail row | Plugins group row | no |

### Rendered identity

The footer builds each button's test id from the resolved destination id, which for a
plugin entry is the owner-namespaced `plugin:<encodeURIComponent(pluginId)>:<encodeURIComponent(itemId)>`.
The resulting attribute is therefore:

```
data-testid="sidebar-plugin:<pluginId>:<itemId>-button"
```

For example the plugin `kandev-plugin-rill` registering `{ id: "rill", section: "sidebar-footer" }`
yields `data-testid="sidebar-plugin:kandev-plugin-rill:rill-button"`. This is the
existing derivation applied unchanged to a plugin destination — the footer is not
special-cased — and it is stated here because it becomes public contract the moment
the first plugin uses it. **When `label` is present** the accessible name is
`NavItem.label` verbatim, untranslated, matching how every other plugin-supplied label is
treated. When it is absent the host substitutes the destination id; that case is a
pre-existing resolver behaviour and is stated under **Failure modes**, not here, because
it is not something this change introduces or may alter.

**An over-budget item carries the same test id.** When an item is reached through the
overflow menu rather than an inline button, its menu item SHALL carry the identical
`data-testid="sidebar-plugin:<pluginId>:<itemId>-button"` and the identical accessible
name. A conformance test therefore selects the same id either way; the only difference
is that the menu must be opened first, because the menu's content is not in the DOM
while closed. The overflow trigger itself carries
`data-testid="sidebar-plugin-overflow-button"`.

Phone Utilities rows carry **no** `data-testid`. The shared row renderer only emits a
plugin test id when the calling surface supplies its own prefix, and neither Utilities
caller does today. Conformance tests for the phone surface must select by visible label.
Adding a prefix there is deliberately excluded (see **Out of scope**).

## Ordering

### What "order" means in this spec: manifest rows only

**Every ordering statement in this document — in this section and in every scenario
below — constrains the relative order of the *manifest destination rows* only**, meaning
the rows a surface renders from `resolveDestinations`. Neither surface renders those rows
alone: both interleave bespoke, non-manifest controls around them. This spec makes no
ordering claim about those controls and changes none of them — they stay exactly as they
render today (see **Out of scope**). A conformance test must therefore assert the relative
order of the manifest rows and must not fail because a non-manifest control sits before,
between, or after them.

**The same manifest-rows-only reading governs every *presence* and *count* statement in
this document**, not just the ordering ones. "The footer shows X" and "exactly N buttons
are present" are claims about the manifest-derived buttons plus the one new non-manifest
control this change adds (the overflow trigger, whose presence and absence are normative
per **Capacity and overflow**). They are never claims about the footer's or the phone
group's full visible control set, because most of that set renders conditionally on state
this change does not touch — What's new on `releaseNotes.showTopbarButton`, the
Office↔Kanban switch on `officeEnabled`, the connection warning on connection state, the
user chip on auth mode, and the phone Health row on whether there are health issues. A
test that pins the complete visible sequence would fail on a profile flag this contract
says nothing about, which is a false failure, not a caught regression.

This is stated once, here, rather than repeated per scenario. The two surfaces and their
non-manifest neighbours are:

- **Desktop sidebar footer** (`app-sidebar-footer.tsx`). Before the `insights`
  destinations: the settings gear. After them: the doctor / Improve Kandev button, and
  then What's new, the Office↔Kanban switch, the theme toggle, the connection warning and
  the user chip, each rendered conditionally. The overflow trigger this amendment adds is
  itself a non-manifest control and sits at a fixed position defined below.
- **Phone menu Utilities group** (`UtilityNavSection` in `app-nav-sections.tsx`). Before
  the destination rows: the Status row, rendered only when the app status drawer is
  enabled. After them: the theme toggle, the Improve Kandev row, and the Health issues
  row, the last rendered only when there are health issues to show.

None of those is a destination, none comes from `APP_DESTINATIONS` or from a plugin, and
none except the new overflow trigger is added, removed or reordered by this change.

### The order itself

Within the `insights` section, on both the sidebar footer and the phone Utilities
group, manifest destinations render in this total order:

1. **First-party entries**, in their array position in `APP_DESTINATIONS`
   (`apps/web/lib/navigation/core-destinations.ts`). Today that is exactly one entry,
   `stats`. First-party always precedes plugin entries — the merged list is
   first-party-then-plugins by construction, matching how the Integrations section
   already orders its plugin additions.
2. **Plugin entries**, in plugin-registry registration order, i.e. the order
   `pluginRegistry.getNavRegistrations()` returns them.

The inline budget does not reorder anything. It **partitions** the plugin run at a fixed
index: the first `MAX_INLINE_PLUGIN_FOOTER_ITEMS` plugin entries render inline in that
order, and the remainder render in the overflow menu in that same order. Concatenating
the inline run with the menu's contents reproduces the full order above, unchanged.

Within a single `loadPlugins` pass, registration order is fully determined and needs no
tiebreak, because it is a single append-ordered array:

- Across plugins: `loadPlugins` iterates the boot payload's `plugins` array with a
  sequential `for … of` and awaits each plugin's `initialize()` before starting the
  next, so plugin *A* earlier in the boot payload has all of its nav items registered
  before plugin *B* registers any of its own. Bundle import latency does not reorder
  anything.
- Within one plugin: items appear in the order `initialize()` calls `registerNavItem`.

That determinism is scoped to one pass on purpose. `loadPlugins` can be **in flight
more than once at a time**: boot fires it without awaiting the result, and the settings
enable/update path calls it independently (`lib/plugins/host.ts` documents this race in
its own module comment). Generation fencing is per-`pluginId` — it stops a stale load
from clobbering a newer one for the *same* plugin — and it imposes no global order
across concurrent loads of *different* plugins. So a plugin enabled from Settings while
boot loading is still running lands wherever the two passes interleave, which is not
predictable from the boot payload order alone. This is the same family as the
re-enable rule below, it is pre-existing for every nav section, and no ordering control
is introduced to fix it.

Three consequences that are contract, not accident:

- A plugin disabled and re-enabled at runtime has its registrations removed and then
  re-appended, so its footer icon moves to the **end** of the plugin run of the
  insights row. The row does not re-sort to restore the boot order.
- Position is not alphabetical and is not influenced by `label`, `id`, or `path`. A
  plugin cannot choose its slot in the row, and cannot choose whether it is inline or
  in the overflow menu.
- Because the partition is by position, a re-enable that moves a plugin to the end of
  the run can move it **from inline into the overflow menu**, and can promote the plugin
  that was first in the overflow menu into the inline run. The item is still present and
  still reachable; only its affordance changed. This is the same registration-order
  consequence as the bullet above, and no stickiness or memory is introduced to avoid
  it.

No per-surface ordering override is introduced. The footer and the phone Utilities
group agree on the **relative order of `insights` entries**: `stats` first, then plugin
entries in registration order, on both surfaces. They differ only in affordance — the
phone renders every entry as a row, the footer may place a suffix of the plugin run
behind the overflow trigger.

They do **not** resolve the same list. The phone Utilities group resolves two sections in
one pass (`MOBILE_MENU_UTILITY_SECTIONS = ["insights", "utilities"]`), and the resolver
returns catalog array order followed by plugin entries — it does not group by section.
Because `stats` (`insights`) precedes `settings` (`utilities`) in `APP_DESTINATIONS`, the
phone group's **manifest rows**, in order and ignoring the non-manifest controls named
above, are:

```
stats, settings, <plugin sidebar-footer items in registration order>
```

so a plugin row lands **after** Settings on the phone, not adjacent to Stats as the
footer's `stats, <plugin …>` might suggest. Conformance tests for the phone surface must
expect that position, and must read it as a claim about the manifest rows only: the
Status row precedes them and the theme, Improve Kandev and Health rows follow them, so a
test asserting the group's complete visible row sequence would fail on controls this
change does not touch. Interleaving the two sections is existing behaviour and is not
changed here.
