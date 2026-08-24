---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001
created: 2026-08-12
owners:
  - nova28
---
# Plugin nav items in the sidebar footer icon row System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-PLUGIN-NAV-SIDEBAR-FOOTER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
  [`docs/public/plugins-authoring.md`](../../../public/plugins-authoring.md). It is
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
