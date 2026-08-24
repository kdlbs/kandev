---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001
created: 2026-08-12
owners:
  - nova28
---
# Plugin nav items in the sidebar footer icon row System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
