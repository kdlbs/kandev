---
status: draft
created: 2026-08-12
amended: 2026-08-12
owner: nova28
---

# Plugin nav items in the sidebar footer icon row

## Amendment log

**Amendment 1 (2026-08-12), from PR #2562 review.** The first version of this spec
exposed the host's internal `NavSection` value `"insights"` as the plugin-facing
`NavItem.section` value, and decided against any capacity policy on the footer row.
A maintainer review on the open PR raised both as contract defects. Both are accepted;
this document is the amended contract, and the two decisions are recorded in
**Plugin-facing section vocabulary** and **Capacity and overflow** with the reasoning
that replaces the original.

Status returns to `draft` because the amended contract is not implemented. The
already-merged-into-the-branch first version is implemented; the amendment is not. The
status returns to `shipped` when PR #2562 merges carrying this contract, and
`docs/specs/INDEX.md` tracks the same value.

**Amendment 1, spec-review round 1 corrections (2026-08-12).** The amended contract went
through an adversarial spec review (cross-vendor and cross-model legs) and came back
FIX FIRST. No acceptance criterion changed meaning; the review found document defects,
not contract defects. Corrections applied, listed so a later reader can tell them apart
from the contract itself:

- **Missing/`null` `label` is now stated.** `Rendered identity` claimed the accessible
  name is `NavItem.label` verbatim, which is false when a bundle omits `label` —
  `destinationLabel()` substitutes the destination id. Qualified there, and given its own
  bullet under **Failure modes**. Existing behaviour; nothing to build.
- **The inline budget's test obligation is now stated.** `MAX_INLINE_PLUGIN_FOOTER_ITEMS`
  is normatively `3` while **Why 3** says the number may move freely; those could not both
  hold for a test hard-coding `3`. Resolved by exporting the constant and requiring
  conformance tests to derive from it.
- **A `Type surface` scenario group was added** so the `PluginNavSection` named export —
  half of what this amendment delivers — is observed by something. Every other scenario
  tests runtime placement and would pass against an inline, un-exported union.
- **The edit inventory now marks each entry EDIT or VERIFY-NOT-EDIT.** Its preamble
  claimed every named file carries contradicting prose; entries 2, 3 and 6 were already
  corrected by the first implementation on this branch.
- **The overflow trigger's test id changed** from `sidebar-insights-overflow-button` to
  `sidebar-plugin-overflow-button`. Adopted from a reviewer's noted question rather than a
  finding: the first spelling minted the host's internal section name `insights` into a
  contract-ish identifier in the same document that removes that name from the plugin API.
  Host test ids are host-owned, so this was not a defect, but nothing implements the
  trigger yet, so the rename is free now and never again.
- **Stray tool-call markup** (`</content>`, `</invoke>`) committed at the end of this file
  was deleted.

**Amendment 1, spec-review round 2 corrections (2026-08-12).** A second adversarial review
(both legs again: cross-vendor codex, cross-model `scope-analyst`) re-verified roughly
thirty existence citations by grep and found **zero dangling references and zero false
claims** — the contract itself was not disputed. It returned FIX FIRST on four coverage and
document-accuracy defects, all closed here without changing what any acceptance criterion
means:

- **The `Type surface` "by name" scenario was unobservable by its own runner.** Both legs
  found this independently. TypeScript is structural, so `pnpm run typecheck` cannot tell a
  named alias from an identical inline union, and the scenario as written asked for a check
  no typecheck can perform. Rewritten so the observable is a **type-equality assertion**
  between `NavItem["section"]` and `PluginNavSection | undefined`, with a new paragraph
  saying plainly that spelling is not observable, that a builder must not invent an AST or
  source-text assertion, and that the declared-by-name requirement belongs to **API
  surface** and is settled in review.
- **The overflow trigger's label was normative but unobserved.** `sidebar:morePluginItems`
  was specified in **Capacity and overflow** while every capacity scenario selected the
  trigger by `data-testid` alone, so wiring it to a different existing key would have passed
  the whole suite. A scenario now pins the key.
- **Edit-inventory entry 6 pointed capacity coverage at the wrong file.** Its parenthetical
  sent the capacity scenarios to `plugin-destinations.test.ts`, a pure mapping unit test
  that cannot see a partition living in `app-sidebar-footer.tsx`. Corrected to name the
  footer component's own test file, with an explicit "do not import the footer component
  into this file" instruction.
- **The `P = 0` baseline scenario had no determinate expected set.** "Shows exactly the
  buttons it shows today" reads as a claim about every footer control, five of which render
  conditionally. The scenario now states three assertable clauses, and **Ordering**'s
  manifest-rows-only rule was widened from ordering statements to **presence and count
  statements** as well, so this class of false failure is ruled out once rather than per
  scenario.

Two outside-voice findings were refuted rather than adopted, for the second time and on the
same grounds: missing/`null`/empty `NavItem.id` and `NavItem.path` are pre-existing,
section-independent pass-throughs on which this change adds no branch and the builder writes
no code. Only the `label` case earned a **Failure modes** bullet, because only it needed a
*negative* instruction (do not add a fallback).

**Renamed from `plugin-nav-insights-section` to `plugin-nav-sidebar-footer` (2026-08-12),
at the owner's request.** The old slug named the host's *internal* nav section, `insights`
— the very coupling this amendment removes from the plugin API — and it read as though the
feature were specific to one (private, unreleased) Insights plugin rather than a placement
any plugin can request. The new slug matches the plugin-facing value `"sidebar-footer"`
exactly, so the spec slug, the public token and the authoring docs all use one word. Moved
with it: `docs/plans/plugin-nav-insights-section/` → `docs/plans/plugin-nav-sidebar-footer/`
(plan plus three task files) and the `docs/specs/INDEX.md` row. The internal section is
still named `insights` in `apps/web/lib/navigation/types.ts` and this document still calls
it that; only the plugin-facing and document-facing names changed.

## Why

A plugin whose page is a compact, glanceable dashboard has nowhere good to put its
entry point. `registerNavItem` can place a row in the sidebar's labelled plugin rail
or in the Integrations section, but not in the sidebar footer's unlabelled icon strip
(the gear / Stats / doctor / Office row) where Kandev's own at-a-glance destination,
Stats, lives. Plugin authors either accept a full-width labelled row that visually
outranks what they are contributing, or they get no placement that matches the shape
of their page.

## What

- `NavItem.section` SHALL accept a fourth value, `"sidebar-footer"`, in addition to the
  existing `"main"`, `"integrations"`, and `"settings"`. The four values SHALL be
  declared as a named exported type, `PluginNavSection`. See **Plugin-facing section
  vocabulary** for why the value is `"sidebar-footer"` and not the host's internal
  section name.
- A plugin nav item with `section: "sidebar-footer"` SHALL render as an icon button in
  the desktop sidebar footer's icon row, styled and behaving exactly like the
  first-party Stats button: icon only, tooltip and accessible name from
  `NavItem.label`, click navigates to `NavItem.path` — subject to the inline budget in
  **Capacity and overflow**, past which the item is reached through the footer's
  overflow menu instead of an inline button.
- The same item SHALL also appear as a labelled row in the phone menu's Utilities
  group, which is where the first-party Stats destination already appears below the
  `md` breakpoint. The sidebar is hidden on phones, so a footer-only placement would
  make the item unreachable there. The phone group is **not** subject to the inline
  budget; see **Capacity and overflow**.
- A `"sidebar-footer"` item SHALL NOT also appear in the sidebar's plugin rail or the
  phone menu's Plugins group. Choosing `sidebar-footer` moves the item; it does not
  add a second placement.
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
  value must degrade to the default placement rather than being dropped. **The literal
  string `"insights"` is one such unknown value and SHALL degrade to the plugin rail**
  — it is deliberately not an accepted alias; see **Plugin-facing section vocabulary**.
- Plugin footer items SHALL NOT appear in the command palette, unchanged from
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
- The desktop footer SHALL render at most `MAX_INLINE_PLUGIN_FOOTER_ITEMS` plugin
  buttons inline and reach any remainder through a single overflow menu. No item is
  dropped. See **Capacity and overflow** for the value, the derivation, and the exact
  rule.
- The public authoring documentation SHALL list `sidebar-footer` as an accepted
  `section` value and describe where it renders. The site is named, not left to be
  found: the `registerNavItem` row of the Frontend hook/API matrix in
  [`docs/public/plugins-authoring.md`](../../public/plugins-authoring.md). It is
  the fifth entry in **Stale prose at the edit sites** below.

## Plugin-facing section vocabulary

### Decision

`NavItem.section` is typed by a named, exported union that is the plugin's vocabulary,
not the host's:

```ts
export type PluginNavSection = "main" | "settings" | "integrations" | "sidebar-footer";
```

The host's internal `NavSection` (`"primary" | "plugins" | "integrations" | "insights"
| "utilities"`, `apps/web/lib/navigation/types.ts`) stays internal. `pluginDestinations()`
translates between the two, and remains the only place that translation happens.

### Why this, and why not `"insights"`

The indirection this amendment names **already existed**; it was only unnamed and
leaking in one place. `sectionFor()` in `plugin-destinations.ts` has always been the
translation layer, and `"main"` has always mapped to the internal section `plugins` —
so three of the four plugin-facing values already differed from, or were independent
of, the internal taxonomy. `"insights"` was the single member that was spelled with the
host's internal name, which is what makes the coupling look like contract. Renaming
that one member and naming the union closes the leak without changing any of the three
shipped values.

The value is spelled after the **placement a plugin author is asking for**, not after
the host's grouping. The host can rename, split, or merge `NavSection` members without
a plugin-visible break, which is exactly the freedom the reviewer asked for and which
the previous version did not provide.

One objection was considered and rejected: `"sidebar-footer"` names a desktop surface,
while the item also renders in the phone menu's Utilities group, where there is no
sidebar. That asymmetry is real but does not win. The phone placement is a host
**reachability guarantee** — the sidebar does not exist below `md`, so the host puts the
item where its first-party peer (`Stats`) already goes — and not a second thing the
plugin declared. A plugin author asking for the footer strip gets the footer strip, plus
whatever the host must do to keep it reachable on a phone. A deliberately neutral token
(`"quick-access"`, `"glance"`) would be surface-independent but would tell an author
nothing about where their icon lands without reading the docs, and would be a novel
coined term where `"sidebar-footer"` is self-describing. The doc comment on the field
states both placements, so nothing is hidden.

### `"insights"` is not accepted, and not an alias

Passing `section: "insights"` from a plugin bundle SHALL be treated as an unrecognised
value and degrade to the plugin rail / Plugins group, exactly like `"footer"` or
`"banana"`. It is not accepted, not aliased, and produces no warning beyond the
existing (absent) validation.

This is safe and deliberate. Nothing has shipped: PR #2562 is open and unmerged, and
the one motivating consumer, `kandev-plugin-rill`, has not switched its `section` yet
(it is still `"main"`). There is no plugin in the wild passing `"insights"`, so there
is no compatibility burden to carry. Accepting both would make the internal name a
permanent de-facto public alias, re-creating the coupling this amendment removes on the
same day it removes it.

The consequence to be explicit about: the in-tree e2e fixture bundle
(`apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`) currently registers
`{ id: "e2e-insights-tools", …, section: "insights" }`. Under this contract that item
would silently move back to the plugin rail, so the fixture SHALL be updated to
`section: "sidebar-footer"` in the same change. It is entry 7 in **Stale prose at the
edit sites**.

### No runtime validation is added

There is still no runtime validation of `section` anywhere in the stack: the web
registry's `registerNavItem` pushes the item verbatim, and the backend plugin manifest
does not describe nav-item sections at all.
`apps/backend/internal/plugins/manifest/validate.go` validates a different, unrelated
enum (`ui.pages[].surface`, one of `settings | task-panel | main-nav`), which is not
this field and is not in scope. Introducing a validator, a console warning, or a
developer-mode diagnostic for an unrecognised `section` is **out of scope** — the
degrade rule is the whole error behaviour.

## API surface

The only contract that changes is the `section` field of `NavItem` and the new named
type for its values. That is a statement about the *contract*, not about the file
count: the field is declared in `apps/web/lib/plugins/types.ts` and mirrored in
[`docs/plans/plugins/PLUGIN-API.md`](../../plans/plugins/PLUGIN-API.md), so those two must
change together, and it is *documented* for plugin authors in
[`docs/public/plugins-authoring.md`](../../public/plugins-authoring.md), which the **What**
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

## Capacity and overflow

**Decision: a bounded inline budget on the desktop footer, with a single overflow menu
holding the remainder. Nothing is dropped, and the phone surface is uncapped.**

### The rule

- `MAX_INLINE_PLUGIN_FOOTER_ITEMS = 3`. The constant SHALL live in
  `apps/web/components/app-sidebar/app-sidebar-footer.tsx`, next to the code that
  applies it, and SHALL be **exported** so conformance tests can import it. The export
  exists for the tests; nothing else imports it, and it is not part of the plugin API.
- The budget counts **plugin** `insights` destinations only — those with
  `source === "plugin"` on the resolved destination, after the `section: "settings"`
  skip. First-party `insights` entries (today, `stats`) are never counted, never
  overflowed, and always render inline.
- Let *P* be the number of plugin `insights` destinations resolved for the sidebar.
  - *P* = 0: the footer renders exactly as it does today. No overflow trigger.
  - 0 < *P* ≤ 3: all *P* render as inline icon buttons, in order. **No overflow trigger
    is rendered** — a "more" button holding nothing is worse than no button.
  - *P* > 3: the first 3 render as inline icon buttons, in order, followed by a single
    overflow trigger holding the remaining *P* − 3 items as menu items, in the same
    order.
- The overflow trigger sits immediately after the last inline plugin button and before
  the doctor / Improve Kandev button, so the insights block occupies at most 5 slots
  (`stats` + 3 inline + trigger).
- The same budget applies **collapsed and expanded**. A per-state threshold would make
  icons jump between inline and menu when the user toggles the sidebar, which is a
  worse surprise than a constant rule.
- The overflow trigger renders with the footer's existing icon-button treatment
  (`FooterIconButton`'s size, tooltip and hover behaviour) wrapped as a
  `@kandev/ui/dropdown-menu` trigger, using `IconDots` from `@tabler/icons-react`. It
  is named `InsightOverflowMenu`. Its label and tooltip come from a new i18n key
  `sidebar:morePluginItems` with the English value `More plugin items`. Per this repo's
  i18n rules the key SHALL be added to `src/locales/en/sidebar.json` and the `pseudo`
  catalog regenerated; the real-locale catalogs (`zh-cn`, `pt-pt`) are translated out of
  band and are **out of scope**.
- Each overflow menu item renders the destination's resolved icon followed by its
  `label` as visible text — the labels are the reason the menu is legible where an
  icon-only strip is not. Clicking a menu item navigates to its `href` exactly as the
  inline button does. Menu placement, keyboard navigation and focus behaviour are
  whatever `@kandev/ui/dropdown-menu` already provides at its defaults; no bespoke
  keyboard handling, side/align override, or focus management is added.

### Why a bound, when the first version rejected one

The first version rejected a cap on the grounds that *"silently dropping the N+1th
plugin's icon would remove a plugin's only entry point with no error surface and no way
for the author to discover it."* That argument is sound and still stands — **against a
cap that drops**. It does not reach an overflow menu, which drops nothing: every
registered item is reachable, in order, one click away, and the trigger is a visible
affordance rather than a silent truncation. Rejecting the drop-cap and then treating the
question as closed was the gap; the reviewer found it.

The defect the bound actually fixes is a **priority inversion**, not crowding. The
footer renders third-party content *before* the host's own controls, inside a container
that clips rather than scrolls: the footer is `shrink-0` inside
`<div data-testid="app-sidebar-content" class="flex min-h-0 flex-1 flex-col overflow-hidden">`
(`app-sidebar.tsx`). Unbounded, a large enough plugin run pushes the theme toggle, the
Office↔Kanban switch, the connection warning and the account chip past the clip, with no
scroll to recover them. Those are host controls a user cannot reach any other way from
the sidebar, and the plugin that displaced them is the thing they would need to reach
Settings to disable. Bounding the plugin block at 4 occupied slots makes that outcome
unreachable at any *P*.

An alternative was considered and rejected: **reordering** the plugin run to the end of
the footer, after the host controls, so clipping eats plugin icons first. It is a
smaller change and it does fix the inversion, but it splits `stats` from the plugin run
it introduces, breaks the desktop/phone ordering agreement this spec establishes in
**Ordering**, and still leaves the row unbounded — a tall enough run still clips, only
now it clips the plugins with no affordance to reach them. The bound is the better
contract.

### Why 3

The collapsed footer is the binding constraint: it is a non-wrapping `flex-col`, so each
icon costs a full row, where the expanded footer is `flex-wrap` and fits roughly seven
28px buttons per line at the sidebar's default width.

Collapsed pitch is 32px (`h-7` button, `gap-1`), except the connection warning at 36px.
Today's worst-case first-party collapsed column is eight controls — gear, `stats`,
Improve, What's new, Office, theme, connection, user chip — at roughly 260px. A plugin
budget of 3 inline plus 1 trigger adds four rows, roughly 130px, for a worst case near
390px plus padding. That leaves the nav list above it non-zero on any viewport taller
than about 600px, which is the shortest window the app is expected to be usable in.
3 is the largest budget that keeps the worst case under 400px, which is why it is 3 and
not 5.

The number is a layout constant, not a contract with plugin authors: a future change may
raise or lower it without renegotiating this spec, provided the rule shape (inline run,
then a single overflow trigger, nothing dropped) holds.

**What that freedom requires of the tests, stated so it is not invented.** The normative
content of this section is the *rule shape*, not the digit. `3` is the value at the time
of writing, and the scenarios below spell it out only to stay concretely readable.
Conformance tests SHALL therefore derive their expectations from the exported
`MAX_INLINE_PLUGIN_FOOTER_ITEMS` rather than hard-coding `3` — a boundary case is written
as `MAX_INLINE_PLUGIN_FOOTER_ITEMS` items for the at-budget case and
`MAX_INLINE_PLUGIN_FOOTER_ITEMS + 1` for the first over-budget case, not as `3` and `4`.
Changing the constant then moves the suite with it and leaves every scenario below true,
which is exactly the freedom the previous paragraph claims. A test that hard-codes `3`
would convert a layout constant into a frozen contract by accident, and would make the
two paragraphs contradict each other the first time anyone tuned the number. The one
exception is the *rule-shape* boundary itself: a test MAY hard-code that `P = 0` renders
no trigger, because zero is a property of the rule, not of the budget.

### The guarantee

The contract that replaces "render everything inline" is a **reachability** guarantee:

- Every plugin `insights` destination SHALL be reachable from the desktop footer in at
  most one extra click: inline, or as a menu item under the overflow trigger. None is
  dropped, hidden behind a count threshold, or made unreachable.
- The overflow menu's content is not in the DOM while the menu is closed. A conformance
  test for an over-budget item SHALL open the menu before asserting. This is the honest
  version of the guarantee, and it is what a test can assert.
- **Expanded**, the footer's container remains a wrapping flex row (`flex-wrap`), so the
  bounded run plus the host controls flow onto a second line rather than overflowing
  horizontally.
- **Collapsed**, the footer remains a non-wrapping vertical column (`flex-col`), now of
  bounded height.
- The **phone** Utilities group renders every plugin item as a labelled row, with no
  budget and no overflow menu. It is a scrolling sheet, not a clipped strip, so the
  failure mode the budget exists to prevent does not occur there, and adding a menu
  inside a menu would be strictly worse. This asymmetry is deliberate contract.

## Failure modes

- **Unknown or missing `icon` name.** The curated icon map falls back to the generic
  puzzle-piece glyph, as it already does for every other plugin nav placement. The
  button still renders and still navigates. The same applies to an overflow menu item.
- **Unknown `section` string** (an untyped bundle passing e.g. `"footer"`, or the host's
  internal `"insights"`). Treated as `"main"`: the item renders in the plugin rail /
  Plugins group. It is never dropped and never silently promoted to the footer.
- **Empty `label`** (the empty string). The button renders with an empty accessible name
  and an empty tooltip. `destinationLabel()` coalesces on nullishness, not on
  falsiness — `"" ?? id` is `""` — so an empty string survives to the button as an empty
  string. This is pre-existing behaviour shared with every plugin nav surface; this
  change neither introduces nor fixes it.
- **Missing or `null` `label`.** The accessible name and tooltip become the
  owner-namespaced destination id (`plugin:<pluginId>:<itemId>`), not an empty string.
  `NavItem.label` is typed `string` and therefore required, but plugin bundles are
  untyped JavaScript at runtime — the same reason an unrecognised `section` must degrade
  rather than be trusted — so a bundle can omit it. `destinationLabel()` in
  `apps/web/lib/navigation/resolve-destinations.ts` ends
  `return destination.label ?? destination.id`, and the resolver is out of scope for this
  change, so the footer button and the overflow menu item both inherit that substitution
  unchanged. A conformance test MUST NOT assert accessible-name-equals-`label` for an
  item that omits `label`; the observable outcome there is the destination id. This is
  pre-existing behaviour shared with every plugin nav surface; this change neither
  introduces nor fixes it, and a builder must not add a label fallback of their own to
  satisfy this spec.
- **`path` pointing at a first-party route.** Unchanged from today: the href is used
  verbatim, so a plugin can *link* to a first-party page but cannot *serve* one — the
  plugin route resolver runs after every static route.
- **Two registrations of the same `(pluginId, id)` pair.** Both append, producing two
  entries sharing one destination id, and both count against the budget. Pre-existing
  for all sections; not changed here.
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
  status filter, so a plugin that registers a `sidebar-footer` item and then hangs leaves
  that entry in the footer until it is next loaded or disabled, occupying a budget slot.
  This is pre-existing loader behaviour shared by every nav section; this change neither
  introduces nor fixes it, and a builder must not add rollback to satisfy this spec.

## Concurrency and idempotency

The plugin registry is a synchronous single-threaded singleton, so there is no torn
write and no two-writer case for the nav-item array itself: every `registerNavItem`
call completes before any other code runs. Reload generation guards prevent a stale
load from re-adding registrations after a newer generation claimed the same plugin.
Re-rendering the footer is a pure read of the registry; it performs no writes and is
safe to run any number of times. Applying the budget is a pure function of the resolved
destination list, so it is likewise idempotent and introduces no state of its own — the
inline/overflow partition is recomputed on every render and is never remembered.

What is *not* claimed is a global ordering guarantee across concurrent `loadPlugins`
invocations — see **Ordering**. Two passes can interleave, and the resulting position
of a plugin's icon, and therefore whether it is inline or in the overflow menu, depends
on that interleaving. Safety is guaranteed; boot-payload position is guaranteed only
within one pass.

Idempotency of a reload is already handled by the loader and is unchanged here: a load
revokes the plugin's prior registrations before re-running `initialize()`, so repeatedly
loading the same plugin converges to exactly one set of nav items rather than
accumulating duplicates. The exception is the partial-failure case in **Failure
modes**: registrations from a failed pass persist until that next load revokes them.

## Scenarios

### Type surface

These are compile-time observations, not runtime ones. They exist because the named
exported type is half of what this amendment delivers, and every other scenario in this
document exercises runtime placement only — a suite built from those alone would pass
against an inline, un-exported union, silently shipping the coupling this amendment
removes while `PLUGIN-API.md` documents a type that does not exist. `pnpm run typecheck`
is the runner.

- **GIVEN** a TypeScript module under `apps/web` writing `import type { PluginNavSection } from "@/lib/plugins/types"`, **WHEN** the web package typechecks, **THEN** the import resolves — the type is exported under that exact name from that exact module.
- **GIVEN** that imported type, **WHEN** each of `"main"`, `"settings"`, `"integrations"` and `"sidebar-footer"` is assigned to a `PluginNavSection`, **THEN** all four assignments typecheck.
- **GIVEN** that same type, **WHEN** the literal `"insights"` is assigned to a `PluginNavSection` under `// @ts-expect-error`, **THEN** the package still typechecks — the suppressed error is present, proving `"insights"` is rejected by the union rather than accepted. If `"insights"` were ever added back to the union the unused `@ts-expect-error` itself becomes the type error, so this scenario fails in both directions and cannot rot silently.
- **GIVEN** the `NavItem` interface in the same module, **WHEN** a type-equality assertion compares `NavItem["section"]` against `PluginNavSection | undefined`, **THEN** the assertion typechecks — and it stops typechecking the moment either side gains, loses, or renames a member, which is the drift this scenario exists to catch.

**What that last scenario does and does not observe.** TypeScript is *structural*, so
`section?: PluginNavSection` and an inline `section?: "main" | "settings" | "integrations"
| "sidebar-footer"` are the same type and no typecheck can tell them apart. The observable
is therefore **equality**, not spelling, and a builder SHALL NOT invent an AST walk or a
source-text match to assert the latter — that would be inventing a mechanism this spec
never asked for. Equality is the property that carries the guarantee: the two can only hurt
anyone by diverging, and divergence is exactly what fails. That the field is *declared* as
`section?: PluginNavSection` rather than as a repeated literal union is a single-source
requirement of **API surface** (see its After snippet), settled by reading the diff in
review, not by this scenario. The standard conditional-type equality helper
(`<T>() => T extends A ? 1 : 2` compared against `<T>() => T extends B ? 1 : 2`) is the
usual way to write it; the spec does not require any particular spelling or helper library.

### Placement

- **GIVEN** an active plugin `acme` that registers `{ id: "board", label: "Acme Board", path: "/plugins/acme", section: "sidebar-footer" }`, **WHEN** the desktop sidebar footer renders, **THEN** an icon button with accessible name `Acme Board` and `data-testid="sidebar-plugin:acme:board-button"` appears in the footer row, and clicking it navigates to `/plugins/acme`.
- **GIVEN** that same plugin, **WHEN** the desktop sidebar's plugin rail renders, **THEN** no row for `board` appears in it.
- **GIVEN** that same plugin, **WHEN** the phone menu's Plugins group (`MobilePluginNavSection`) renders, **THEN** no row for `board` appears in it. This is the mobile half of the same "moves, does not add" rule the previous scenario asserts for the desktop rail; both halves are contract, and a regression that left a `sidebar-footer` item also matching the `plugins` section would show up here and nowhere else.
- **GIVEN** that same plugin, **WHEN** the phone menu's Utilities group renders, **THEN** a row labelled `Acme Board` linking to `/plugins/acme` appears in it.
- **GIVEN** that same plugin, **WHEN** the command palette's Navigation group renders, **THEN** no entry for `board` appears.
- **GIVEN** a plugin item with `section: "integrations"`, **WHEN** the sidebar's Integrations section and the footer both render, **THEN** the item appears in Integrations and does not appear in the footer.
- **GIVEN** a plugin item with `section: "settings"`, **WHEN** any navigation surface resolves, **THEN** no destination for that item exists on any surface.
- **GIVEN** a plugin item with `section` omitted, **WHEN** the sidebar renders, **THEN** the item appears in the plugin rail and does not appear in the footer.
- **GIVEN** a plugin item with an unrecognised `section` string, **WHEN** the sidebar renders, **THEN** the item appears in the plugin rail and does not appear in the footer.
- **GIVEN** a plugin item with `section: "insights"` — the host's internal section name — **WHEN** the sidebar renders, **THEN** the item appears in the plugin rail and does not appear in the footer, because `"insights"` is not an accepted plugin-facing value.
- **GIVEN** no active plugin registers a `sidebar-footer` item, **WHEN** the footer renders, **THEN** its **manifest buttons** are exactly one — `stats` — no button carrying a `sidebar-plugin:…-button` test id exists, and no element with `data-testid="sidebar-plugin-overflow-button"` exists. Per **Ordering**, read this as a claim about the manifest-derived buttons and the overflow trigger only, not about the footer's full icon set: the bespoke controls around them (gear, doctor, What's new, Office↔Kanban, theme, connection warning, user chip) each render conditionally on state this change does not touch, so a test asserting the complete visible button sequence would fail on flags this contract says nothing about. Those three clauses *are* the normative *"P = 0: the footer renders exactly as it does today"* guarantee in **Capacity and overflow**, stated in the form a test can assert.
- **GIVEN** a plugin `sidebar-footer` item whose `icon` is a name absent from the curated icon map, **WHEN** the footer renders, **THEN** the button renders with the puzzle-piece fallback glyph and still navigates to `path`.

### Ordering

- **GIVEN** the first-party `stats` destination and a plugin `sidebar-footer` item, **WHEN** the insights section resolves for the sidebar, **THEN** `stats` is ordered before the plugin item.
- **GIVEN** two active plugins `acme` then `globex`, each registering one `sidebar-footer` item, and `acme` listed first in the boot payload, **WHEN** the footer renders, **THEN** `stats`, `acme`'s item and `globex`'s item appear in that **relative order among the `insights` entries**. Per **Ordering**, this constrains those three manifest buttons only and is not an assertion about the footer's full icon sequence; a conformance test must not fail because the settings gear precedes them or the doctor button follows them.
- **GIVEN** two active plugins that both register a `sidebar-footer` item with `id: "dashboard"`, **WHEN** the footer renders, **THEN** both icons render with distinct destination ids `plugin:<pluginA>:dashboard` and `plugin:<pluginB>:dashboard`.
- **GIVEN** the boot-order footer manifest run `stats, acme, globex`, **WHEN** `acme` is disabled and re-enabled, **THEN** the `insights` entries appear in the relative order `stats`, `globex`, `acme` — same manifest-rows-only reading as the boot-order scenario above, per **Ordering**.
- **GIVEN** that same plugin `acme`, **WHEN** the phone menu's Utilities group resolves, **THEN** its **manifest rows** appear in the relative order `Stats`, `Settings`, `Acme Board` — the plugin row follows the first-party `utilities` entry, not `Stats`. Per **Ordering**, this constrains those three rows only: the Status row renders before them and the theme, Improve Kandev and Health rows render after them, and a test must not fail on their presence.

### Capacity and overflow

The counts below are written against `MAX_INLINE_PLUGIN_FOOTER_ITEMS` at its current
value of 3 — read `3` as "the budget", `4` as "the budget plus one" and `8` as "well over
the budget". Per **Capacity and overflow**, conformance tests derive these from the
exported constant rather than hard-coding the digits.

- **GIVEN** exactly 3 active plugins each registering one `sidebar-footer` item, **WHEN** the expanded desktop footer renders, **THEN** all 3 plugin buttons are present inline in registration order and **no** element with `data-testid="sidebar-plugin-overflow-button"` exists.
- **GIVEN** 4 active plugins `p1…p4` each registering one `sidebar-footer` item, in that registration order, **WHEN** the desktop footer renders, **THEN** inline buttons for `p1`, `p2` and `p3` are present in that order, an overflow trigger with `data-testid="sidebar-plugin-overflow-button"` follows them, `p4`'s button is not inline, and opening the trigger reveals exactly one menu item, for `p4`, carrying `data-testid="sidebar-plugin:p4:<itemId>-button"` and accessible name equal to its `label`.
- **GIVEN** those same 4 plugins, **WHEN** the desktop footer renders, **THEN** the overflow trigger's accessible name and its tooltip are both the rendered value of the i18n key `sidebar:morePluginItems` (`More plugin items` in the `en` catalog). A conformance test SHALL derive the expected string from that key rather than hard-coding the English text, so re-wording the copy moves the test with it: what this scenario pins is the **key**, because wiring the trigger to some other existing key is the failure it exists to catch and every other capacity scenario selects the trigger by test id alone and would stay green through it.
- **GIVEN** those same 4 plugins, **WHEN** the overflow menu is open and `p4`'s menu item is clicked, **THEN** the app navigates to `p4`'s `path`.
- **GIVEN** 8 active plugins each registering one `sidebar-footer` item, **WHEN** the expanded desktop footer renders, **THEN** exactly 3 inline plugin buttons and one overflow trigger are present, opening the trigger reveals the remaining 5 in registration order, none of the 8 is dropped, and the footer container carries its wrapping-row classes.
- **GIVEN** those same 8 plugins, **WHEN** the **collapsed** desktop footer renders, **THEN** the same 3 inline buttons and one overflow trigger are present — the budget is unchanged by collapse — and the container carries its non-wrapping column classes.
- **GIVEN** those same 8 plugins, **WHEN** the phone menu's Utilities group renders, **THEN** all 8 appear as labelled rows in registration order, with no overflow menu and no truncation.
- **GIVEN** the first-party `stats` destination and 8 plugin `sidebar-footer` items, **WHEN** the desktop footer renders, **THEN** `stats` renders as an inline button and is not placed in the overflow menu, and the 3-item budget applies to the plugin items alone.
- **GIVEN** 4 plugins `p1…p4` with `p1` inline and `p4` in the overflow menu, **WHEN** `p1` is disabled and re-enabled, **THEN** `p2`, `p3` and `p4` are the inline buttons and `p1` is the sole overflow menu item, per the registration-order partition in **Ordering**.
- **GIVEN** a plugin whose `initialize()` registers a `sidebar-footer` item and then throws or exceeds the initialize timeout, **WHEN** the footer renders, **THEN** the already-registered entry is still present and still occupies a budget slot, because a failed initialize does not revoke registrations already made.

## Out of scope

- **Renaming the three shipped `NavItem.section` values** (`"main"`, `"integrations"`,
  `"settings"`). They are already independent of the host's internal `NavSection` names
  and are in use by shipped plugins; renaming them would be a breaking change for no
  gain. Only the new member is named after placement intent.
- **Accepting `"insights"` as an alias for `"sidebar-footer"`, or warning when a plugin
  passes an unrecognised `section`.** Both are decided against in **Plugin-facing
  section vocabulary**; the degrade rule is the whole error behaviour, and no runtime
  validation, console warning, or developer-mode diagnostic is added.
- **Backwards compatibility for `section: "insights"`.** Nothing has shipped that uses
  it: PR #2562 is unmerged and `kandev-plugin-rill` has not switched.
- **A capacity policy on the phone Utilities group.** It is a scrolling sheet of
  labelled rows; the clipping failure the desktop budget prevents does not occur there.
- **Capping, truncating, or overflowing first-party `insights` destinations.** The
  budget counts plugin entries only. If `APP_DESTINATIONS` ever grows a second
  `insights` entry, that is a first-party layout decision, not this contract.
- **Making the inline budget configurable** — by user setting, plugin manifest field,
  or runtime feature flag. It is a layout constant in the footer component.
- **Making the sidebar footer scroll, or changing the `overflow-hidden` sidebar
  container.** The budget removes the need; changing the container is a sidebar-layout
  decision, not part of opening a section to plugins.
- **Rolling back nav registrations made before a plugin's `initialize()` throws or
  times out.** Pre-existing loader behaviour across every section; see **Failure
  modes**.
- **A per-plugin or per-surface ordering control** (`order`, `priority`, or
  alphabetical sorting) for nav items, and any stickiness that would keep a plugin
  inline across a re-enable. Registration order stands for every section, including
  this one.
- **Changing the footer's or the phone group's other non-manifest controls.** The footer
  component gains exactly one new control, the overflow trigger, at the position stated
  in **Capacity and overflow**. No change to its bespoke buttons (gear, doctor, What's
  new, Office, theme, connection, user chip), to their styling or order, or to the phone
  Utilities group's bespoke rows (Status, theme, Improve Kandev, Health issues).
- **Changing the first-party catalog** (`core-destinations.ts`) or the resolver
  (`resolve-destinations.ts`). Neither needs to know a plugin item can be
  `sidebar-footer`; the budget is applied in the footer component, not in the manifest.
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
- **Real-locale translations** of `sidebar:morePluginItems` (`zh-cn`, `pt-pt`). Those
  catalogs are translated out of band and only warn in CI; `en` and `pseudo` gate and
  are in scope.
- **Any change to `kandev-plugin-rill`**, the private plugin that motivates this. It
  switches its own `section` from `"main"` to `"sidebar-footer"` after this ships, in
  its own repository.
