---
status: shipped
created: 2026-08-12
owner: nova28
---

# Plugin nav items in the sidebar footer icon row

## Why

A plugin whose page is a compact, glanceable dashboard has nowhere good to put its
entry point. `registerNavItem` can place a row in the sidebar's labelled plugin rail
or in the Integrations section, but not in the sidebar footer's unlabelled icon strip
(the gear / Stats / doctor / Office row) where Kandev's own at-a-glance destination,
Stats, lives. Plugin authors either accept a full-width labelled row that visually
outranks what they are contributing, or they get no placement that matches the shape
of their page.

## What

- `NavItem.section` SHALL accept a fourth value, `"insights"`, in addition to the
  existing `"main"`, `"integrations"`, and `"settings"`.
- A plugin nav item with `section: "insights"` SHALL render as an icon button in the
  desktop sidebar footer's icon row, styled and behaving exactly like the first-party
  Stats button: icon only, tooltip and accessible name from `NavItem.label`, click
  navigates to `NavItem.path`.
- The same item SHALL also appear as a labelled row in the phone menu's Utilities
  group, which is where the first-party Stats destination already appears below the
  `md` breakpoint. The sidebar is hidden on phones, so a footer-only placement would
  make the item unreachable there.
- An `"insights"` item SHALL NOT also appear in the sidebar's plugin rail or the
  phone menu's Plugins group. Choosing `insights` moves the item; it does not add a
  second placement.
- `"integrations"` and `"settings"` behaviour SHALL be unchanged: `"integrations"`
  items still render alongside the first-party integration links on both surfaces,
  and `"settings"` items are still skipped entirely by navigation, which means they
  render on no surface at all. A `section: "settings"` nav item is **not** what fills
  the settings tree's `PluginSlot`: that slot (`<PluginSlot name="settings-nav" />` in
  `settings-tree.tsx`) is fed only by `registerComponent("settings-nav", …)`, and a
  plugin that wants a settings page uses `registerSettingsRoute` / `registerComponent`.
  `section: "settings"` on a nav item is accepted and then dropped.
- Any other `section` value, including omitted/`undefined` and a string outside the
  documented set, SHALL continue to render in the plugin rail / Plugins group exactly
  as `"main"` does. Plugin bundles are untyped JavaScript at runtime, so an unknown
  value must degrade to the default placement rather than being dropped.
- Plugin `"insights"` items SHALL NOT appear in the command palette, unchanged from
  every other plugin section. This follows structurally, not from a rule this spec
  adds: `pluginDestinations()` stamps every plugin entry with `surfaces:
  SIDEBAR_AND_MENU`, and the palette's Navigation group is
  `useStaticDestinations("palette")` (`components/global-commands.tsx`), which filters
  on that surface. **Plugins have no route into the command palette at all today, by
  any API.** `registerKeybinding(id, handler)` is not one: it binds a handler to an id
  declared in the plugin's manifest `ui.keybindings[]`, which the host dispatches from
  a global capture-phase keydown listener (`hooks/use-plugin-shortcuts.ts`) — a
  keyboard binding, not a palette row. Adding a plugin→palette route is a separate
  feature (see **Out of scope**).
- There SHALL be no numeric cap on how many plugin items the footer row accepts. See
  **Capacity and overflow** below for the reasoning and the guarantee that replaces a
  cap.
- The public authoring documentation SHALL list `insights` as an accepted `section`
  value and describe where it renders. The site is named, not left to be found: the
  `registerNavItem` row of the Frontend hook/API matrix in
  [`docs/public/plugins-authoring.md`](../../public/plugins-authoring.md), which today
  reads *"section is main, integrations, or accepted-but-not-rendered settings"*. It is
  the fifth entry in **Stale prose at the edit sites** below.

## API surface

The only contract that changes is the `section` field of `NavItem`. That is a statement
about the *contract*, not about the file count: the field is declared in
`apps/web/lib/plugins/types.ts` and mirrored in
[`docs/plans/plugins/PLUGIN-API.md`](../../plans/plugins/PLUGIN-API.md), so those two must
change together, and it is *documented* for plugin authors in
[`docs/public/plugins-authoring.md`](../../public/plugins-authoring.md), which the **What**
section requires updating in the same change. Three files, one contract. Read neither this
paragraph nor the Before/After snippet below as the exhaustive edit list — that list is
**Stale prose at the edit sites**. `PluginNavRegistration`
(`apps/web/lib/plugins/registry.ts`) extends `NavItem` and inherits the widening, so it
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
interface NavItem {
  id: string;
  label: string;
  path: string;
  icon?: string;
  section?: "main" | "settings" | "integrations" | "insights";
}
```

There is no runtime validation of `section` anywhere in the stack: the web registry's
`registerNavItem` pushes the item verbatim, and the backend plugin manifest does not
describe nav-item sections at all. `apps/backend/internal/plugins/manifest/validate.go`
validates a different, unrelated enum (`ui.pages[].surface`, one of
`settings | task-panel | main-nav`), which is not this field and is not in scope. The
widening is therefore a TypeScript-and-docs change plus the section mapping; no
validator gains a new accepted string.

### Stale prose at the edit sites — correct it in the same change

Every file this change touches already carries prose that contradicts the contract above.
It sits inches from the lines being changed, so leaving it is shipping a
self-contradictory frozen contract, and a builder who noticed but had no instruction
would be inventing scope. The spec therefore requires all six to be corrected in the
same change, and names them so nobody has to guess.

**This list is the complete edit inventory for prose.** Together with the one mapping
line in `pluginDestinations()`, the union widening in `types.ts`, and the new test
coverage, it is everything this change writes.

1. `docs/plans/plugins/PLUGIN-API.md`, the comment immediately above the `NavItem`
   interface the Before/After snippet replaces, currently reads *"Hosts predating a
   `section` value simply don't render items targeting it (additive change)."* That
   contradicts the **What** requirement that an unrecognised value degrade to the
   plugin rail, and it is false about the shipped host, which drops nothing. It SHALL
   be replaced with the degrade-to-`"main"` rule. The same comment block SHALL also
   gain a line describing where `"insights"` renders, matching the mapping table.
2. `apps/web/lib/navigation/plugin-destinations.ts`, the `pluginDestinations` docblock,
   currently reads *"`section: "settings"` items are skipped — those render in the
   settings tree's `PluginSlot`, not as destinations."* The second clause is false; the
   slot is fed only by `registerComponent("settings-nav", …)`. It SHALL be corrected to
   say settings nav items are skipped and render on no surface.
3. `apps/web/lib/navigation/plugin-destinations.ts`, the comment on the `surfaces:
   SIDEBAR_AND_MENU` line, currently cites `registerShortcut` — an identifier that does
   not exist anywhere in this repository. It SHALL be corrected to state that plugin
   destinations never declare the `palette` surface, without naming a substitute API,
   because there is no plugin route into the palette (see **What**).
4. `apps/web/lib/plugins/types.ts`, the doc comment on the `section` field being
   widened, currently documents only two of the three values it already accepts
   (*"`"main"` (default) as a top-level sidebar entry, `"integrations"` inside the
   sidebar's Integrations section"*) — `"settings"` is undocumented. Widening the union
   without touching it would leave two of four values undocumented on the field's own
   declaration. It SHALL be extended to cover all four, matching the mapping table:
   `"insights"` renders in the sidebar footer icon row and the phone Utilities group,
   and `"settings"` is accepted and renders nowhere.
5. `docs/public/plugins-authoring.md`, the `registerNavItem` row of the Frontend hook/API
   matrix, currently reads *"section is main, integrations, or accepted-but-not-rendered
   settings"*. That is the public authoring surface the **What** section's
   documentation SHALL points at, and it is the enumeration a plugin author actually
   reads. It SHALL list `insights` and say where it renders, matching the mapping table.
   This is the one entry in this list that is a stated requirement rather than only a
   correction; it is listed here so the edit inventory is in one place.
6. `apps/web/lib/navigation/plugin-destinations.test.ts`, the title of the palette
   exclusion test, currently reads
   *`it("keeps plugin items off the palette, which plugins reach via shortcuts", …)`*.
   The trailing clause is the same false claim as item 3 — there is no plugin route into
   the palette, by `registerShortcut` or any other API — and this is the file the new
   `insights` coverage is added to, so the builder will be editing around it. It SHALL be
   retitled to state the exclusion without naming a substitute route. The assertion
   itself is correct and SHALL NOT change.

Items 1 to 4 and 6 are prose corrections; item 5 is a documentation requirement. None of
the six changes behaviour, a signature, an exported symbol, or a test assertion, and
nothing here is an implementation decision left open.

**The mapping lives in exactly one module.** `pluginDestinations()` in
`apps/web/lib/navigation/plugin-destinations.ts` is the sole place a `NavItem.section`
value is translated into a manifest `NavSection` — it is the only reader of that field
anywhere in the web app. The section-to-placement table below becomes code there and
nowhere else, which is why every other navigation module is listed under **Out of
scope**.

### Section-to-placement mapping

This table is the whole behavioural contract. `NavItem.section` value on the left,
resulting internal navigation section and rendered surfaces on the right.

| `NavItem.section` | Nav section | Desktop sidebar | Phone menu | Palette |
|---|---|---|---|---|
| `"insights"` | `insights` | footer icon button | Utilities group row | no |
| `"integrations"` | `integrations` | Integrations section row | Integrations group row | no |
| `"settings"` | *(skipped)* | no | no | no |
| `"main"` | `plugins` | plugin rail row | Plugins group row | no |
| omitted | `plugins` | plugin rail row | Plugins group row | no |
| any other string | `plugins` | plugin rail row | Plugins group row | no |

### Rendered identity

The footer builds each button's test id from the resolved destination id, which for a
plugin entry is the owner-namespaced `plugin:<encodeURIComponent(pluginId)>:<encodeURIComponent(itemId)>`.
The resulting attribute is therefore:

```
data-testid="sidebar-plugin:<pluginId>:<itemId>-button"
```

For example the plugin `kandev-plugin-rill` registering `{ id: "rill", section: "insights" }`
yields `data-testid="sidebar-plugin:kandev-plugin-rill:rill-button"`. This is the
existing derivation applied unchanged to a plugin destination — the footer is not
special-cased — and it is stated here because it becomes public contract the moment
the first plugin uses it. The accessible name is `NavItem.label` verbatim, untranslated,
matching how every other plugin-supplied label is treated.

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

This is stated once, here, rather than repeated per scenario. The two surfaces and their
non-manifest neighbours are:

- **Desktop sidebar footer** (`app-sidebar-footer.tsx`). Before the `insights`
  destinations: the settings gear. After them: the doctor / Improve Kandev button, and
  then What's new, the Office↔Kanban switch, the theme toggle, the connection warning and
  the user chip, each rendered conditionally.
- **Phone menu Utilities group** (`UtilityNavSection` in `app-nav-sections.tsx`). Before
  the destination rows: the Status row, rendered only when the app status drawer is
  enabled. After them: the theme toggle, the Improve Kandev row, and the Health issues
  row, the last rendered only when there are health issues to show.

None of those is a destination, none comes from `APP_DESTINATIONS` or from a plugin, and
none is added, removed or reordered by this change.

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

Two consequences that are contract, not accident:

- A plugin disabled and re-enabled at runtime has its registrations removed and then
  re-appended, so its footer icon moves to the **end** of the plugin run of the
  insights row. The row does not re-sort to restore the boot order.
- Position is not alphabetical and is not influenced by `label`, `id`, or `path`. A
  plugin cannot choose its slot in the row.

No per-surface ordering override is introduced. The footer and the phone Utilities
group agree on the **relative order of `insights` entries**: `stats` first, then plugin
entries in registration order, on both surfaces.

They do **not** resolve the same list. The phone Utilities group resolves two sections in
one pass (`MOBILE_MENU_UTILITY_SECTIONS = ["insights", "utilities"]`), and the resolver
returns catalog array order followed by plugin entries — it does not group by section.
Because `stats` (`insights`) precedes `settings` (`utilities`) in `APP_DESTINATIONS`, the
phone group's **manifest rows**, in order and ignoring the non-manifest controls named
above, are:

```
stats, settings, <plugin insights items in registration order>
```

so a plugin row lands **after** Settings on the phone, not adjacent to Stats as the
footer's `stats, <plugin …>` might suggest. Conformance tests for the phone surface must
expect that position, and must read it as a claim about the manifest rows only: the
Status row precedes them and the theme, Improve Kandev and Health rows follow them, so a
test asserting the group's complete visible row sequence would fail on controls this
change does not touch. Interleaving the two sections is existing behaviour and is not
changed here.

## Capacity and overflow

**Decision: no cap, no truncation, no overflow menu.** Every registered `insights`
plugin item is rendered.

A cap was considered and rejected: silently dropping the *N+1*th plugin's icon would
remove a plugin's only entry point with no error surface and no way for the author to
discover it, which is a worse failure than a crowded footer. The realistic ceiling is
also low — this section competes with a labelled plugin rail that remains the default,
and the motivating installation has one such plugin.

The contract that replaces the cap is a **rendering** guarantee, not a visibility one:

- The footer SHALL render a button for every `insights` destination into the DOM. None
  is dropped, truncated, collapsed into a "more" affordance, or hidden behind a count
  threshold. This is what an implementation can guarantee and what a test can assert.
- **Expanded**, the footer's container is a wrapping flex row (`flex-wrap`), so extra
  icons flow onto a second line rather than overflowing horizontally.
- **Collapsed**, the footer is a non-wrapping vertical column (`flex-col`). Extra icons
  make the column taller.

What this spec deliberately does **not** promise is that every icon is visible on
screen at any count. The footer is `shrink-0` inside
`<div data-testid="app-sidebar-content" class="flex min-h-0 flex-1 flex-col overflow-hidden">`
(`app-sidebar.tsx`), so a collapsed column tall enough to exceed the available sidebar
height is clipped by that ancestor rather than scrolled. That is pre-existing layout
behaviour shared with the footer's first-party buttons, and changing it would require
editing the footer or the sidebar container, which **Out of scope** forbids. Stating
the guarantee as "rendered, never dropped" keeps this section and Out of scope
consistent, and keeps the guarantee falsifiable.

If a real installation ever reaches a count where clipping matters, the remedy is a
separate change to the sidebar's layout (scroll or height policy) — not a cap here.

## Failure modes

- **Unknown or missing `icon` name.** The curated icon map falls back to the generic
  puzzle-piece glyph, as it already does for every other plugin nav placement. The
  button still renders and still navigates.
- **Unknown `section` string** (an untyped bundle passing e.g. `"footer"`). Treated as
  `"main"`: the item renders in the plugin rail / Plugins group. It is never dropped
  and never silently promoted to the footer.
- **Empty `label`.** The button renders with an empty accessible name and an empty
  tooltip. This is pre-existing behaviour shared with every plugin nav surface; this
  change neither introduces nor fixes it.
- **`path` pointing at a first-party route.** Unchanged from today: the href is used
  verbatim, so a plugin can *link* to a first-party page but cannot *serve* one — the
  plugin route resolver runs after every static route.
- **Two registrations of the same `(pluginId, id)` pair.** Both append, producing two
  footer icons sharing one destination id. Pre-existing for all sections; not changed
  here.
- **A plugin's bundle never loads at all** (import fails, or it does not call
  `registerKandevPlugin`). `initialize()` never runs, so no nav item is registered and
  no icon appears. The loader marks the plugin failed, moves on to the next one, and
  the footer renders the remaining icons in order.
- **A plugin throws or times out *partway through* `initialize()`.** Registrations are
  **not** rolled back. `initialize()` runs against a live registry, so every
  `registerNavItem` call it completed before the failure has already landed; on timeout
  the loader logs and marks the plugin failed, and on a throw it logs and marks the
  plugin failed, and neither path revokes registrations (`unregisterPlugin` runs at the
  *start of the plugin's next load*, not on failure). `getNavRegistrations()` applies no
  status filter, so a plugin that registers an `insights` item and then hangs leaves
  that icon in the footer until it is next loaded or disabled. This is pre-existing
  loader behaviour shared by every nav section; this change neither introduces nor
  fixes it, and a builder must not add rollback to satisfy this spec.

## Concurrency and idempotency

The plugin registry is a synchronous single-threaded singleton, so there is no torn
write and no two-writer case for the nav-item array itself: every `registerNavItem`
call completes before any other code runs. Reload generation guards prevent a stale
load from re-adding registrations after a newer generation claimed the same plugin.
Re-rendering the footer is a pure read of the registry; it performs no writes and is
safe to run any number of times.

What is *not* claimed is a global ordering guarantee across concurrent `loadPlugins`
invocations — see **Ordering**. Two passes can interleave, and the resulting position
of a plugin's icon depends on that interleaving. Safety is guaranteed; boot-payload
position is guaranteed only within one pass.

Idempotency of a reload is already handled by the loader and is unchanged here: a load
revokes the plugin's prior registrations before re-running `initialize()`, so repeatedly
loading the same plugin converges to exactly one set of nav items rather than
accumulating duplicates. The exception is the partial-failure case in **Failure
modes**: registrations from a failed pass persist until that next load revokes them.

## Scenarios

- **GIVEN** an active plugin `acme` that registers `{ id: "board", label: "Acme Board", path: "/plugins/acme", section: "insights" }`, **WHEN** the desktop sidebar footer renders, **THEN** an icon button with accessible name `Acme Board` and `data-testid="sidebar-plugin:acme:board-button"` appears in the footer row, and clicking it navigates to `/plugins/acme`.
- **GIVEN** that same plugin, **WHEN** the desktop sidebar's plugin rail renders, **THEN** no row for `board` appears in it.
- **GIVEN** that same plugin, **WHEN** the phone menu's Plugins group (`MobilePluginNavSection`) renders, **THEN** no row for `board` appears in it. This is the mobile half of the same "moves, does not add" rule the previous scenario asserts for the desktop rail; both halves are contract, and a regression that left an `insights` item also matching the `plugins` section would show up here and nowhere else.
- **GIVEN** that same plugin, **WHEN** the phone menu's Utilities group renders, **THEN** a row labelled `Acme Board` linking to `/plugins/acme` appears in it.
- **GIVEN** that same plugin, **WHEN** the phone menu's Utilities group resolves, **THEN** its **manifest rows** appear in the relative order `Stats`, `Settings`, `Acme Board` — the plugin row follows the first-party `utilities` entry, not `Stats`. Per **Ordering**, this constrains those three rows only: the Status row renders before them and the theme, Improve Kandev and Health rows render after them, and a test must not fail on their presence.
- **GIVEN** a plugin whose `initialize()` registers an `insights` item and then throws or exceeds the initialize timeout, **WHEN** the footer renders, **THEN** the already-registered icon is still present, because a failed initialize does not revoke registrations already made.
- **GIVEN** that same plugin, **WHEN** the command palette's Navigation group renders, **THEN** no entry for `board` appears.
- **GIVEN** the first-party `stats` destination and a plugin `insights` item, **WHEN** the insights section resolves for the sidebar, **THEN** `stats` is ordered before the plugin item.
- **GIVEN** two active plugins `acme` then `globex`, each registering one `insights` item, and `acme` listed first in the boot payload, **WHEN** the footer renders, **THEN** `stats`, `acme`'s item and `globex`'s item appear in that **relative order among the `insights` entries**. Per **Ordering**, this constrains those three manifest buttons only and is not an assertion about the footer's full icon sequence; a conformance test must not fail because the settings gear precedes them or the doctor button follows them.
- **GIVEN** two active plugins that both register an `insights` item with `id: "dashboard"`, **WHEN** the footer renders, **THEN** both icons render with distinct destination ids `plugin:<pluginA>:dashboard` and `plugin:<pluginB>:dashboard`.
- **GIVEN** the boot-order footer manifest run `stats, acme, globex`, **WHEN** `acme` is disabled and re-enabled, **THEN** the `insights` entries appear in the relative order `stats`, `globex`, `acme` — same manifest-rows-only reading as the boot-order scenario above, per **Ordering**.
- **GIVEN** a plugin item with `section: "integrations"`, **WHEN** the sidebar's Integrations section and the footer both render, **THEN** the item appears in Integrations and does not appear in the footer.
- **GIVEN** a plugin item with `section: "settings"`, **WHEN** any navigation surface resolves, **THEN** no destination for that item exists on any surface.
- **GIVEN** a plugin item with `section` omitted, **WHEN** the sidebar renders, **THEN** the item appears in the plugin rail and does not appear in the footer.
- **GIVEN** a plugin item with an unrecognised `section` string, **WHEN** the sidebar renders, **THEN** the item appears in the plugin rail and does not appear in the footer.
- **GIVEN** no active plugin registers an `insights` item, **WHEN** the footer renders, **THEN** it shows exactly the buttons it shows today, with `stats` as the only insights entry.
- **GIVEN** a plugin `insights` item whose `icon` is a name absent from the curated icon map, **WHEN** the footer renders, **THEN** the button renders with the puzzle-piece fallback glyph and still navigates to `path`.
- **GIVEN** eight active plugins each registering one `insights` item, **WHEN** the expanded desktop footer renders, **THEN** eight plugin footer buttons are present in the DOM, none is dropped or replaced by an overflow affordance, and the footer container carries its wrapping-row classes.
- **GIVEN** those same eight plugins, **WHEN** the **collapsed** desktop footer renders, **THEN** all eight buttons are still present in the DOM. Whether every one is within the visible sidebar height is explicitly not asserted; see **Capacity and overflow**.

## Out of scope

- **A numeric cap, truncation, or an overflow "more" menu on the footer row.** Decided
  against above; every destination is rendered.
- **Making the sidebar footer scroll, or changing the `overflow-hidden` sidebar
  container so a tall collapsed footer stays on screen.** Pre-existing layout behaviour
  that also affects the footer's first-party buttons; see **Capacity and overflow**.
  Changing it is a sidebar-layout decision, not part of opening a section to plugins.
- **Rolling back nav registrations made before a plugin's `initialize()` throws or
  times out.** Pre-existing loader behaviour across every section; see **Failure
  modes**.
- **A per-plugin or per-surface ordering control** (`order`, `priority`, or
  alphabetical sorting) for nav items. Registration order stands for every section,
  including this one. Introducing one is a separate change affecting all four sections
  and all three surfaces.
- **Changing either surface's non-manifest controls, or how it renders them.** No change
  to the footer component's markup or button styling, to its bespoke buttons (gear,
  doctor, What's new, Office, theme, connection, user chip), or to the phone Utilities
  group's bespoke rows (Status, theme, Improve Kandev, Health issues). Both sets are
  listed under **Ordering**; this change adds manifest rows beside them and moves,
  restyles and removes none of them.
- **Changing the first-party catalog** (`core-destinations.ts`) or the resolver
  (`resolve-destinations.ts`). Neither needs to know a plugin item can be `insights`.
- **Adding a plugin `data-testid` prefix to the phone Utilities rows.** Those rows are
  test-id-less for first-party and plugin entries alike today; adding one is a
  separate surface-contract decision, and label-based selection covers this feature's
  conformance needs.
- **Palette entries for plugin nav items** of any section, and more broadly **any
  plugin route into the command palette**. There is none today: the palette's
  Navigation group resolves `useStaticDestinations("palette")` and plugin destinations
  never declare the `palette` surface, while `registerKeybinding` binds a global
  keydown handler rather than a palette row. Building one is its own feature and is not
  a prerequisite for this change — the exclusion here is the status quo, kept.
- **Backend manifest changes.** `ui.pages[].surface` is a different enum for a
  different purpose and gains no new value.
- **De-duplicating repeated `registerNavItem` calls** for the same `(pluginId, id)`.
  Pre-existing behaviour across every section.
- **The `utilities` nav section.** Only `insights` is opened to plugins; `utilities`
  holds the first-party Settings entry and stays closed.
- **Any change to `kandev-plugin-rill`**, the private plugin that motivates this. It
  switches its own `section` from `"main"` to `"insights"` after this ships, in its
  own repository.
