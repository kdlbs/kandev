---
id: "01-contract-and-mapping"
title: "Contract and mapping"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/plugins/requirements/plugin-nav-sidebar-footer.md"
---

# Task 01: Contract and mapping

Widen the `NavItem.section` contract to accept `"insights"`, teach
`pluginDestinations()` to route it to the `insights` `NavSection`, and correct
the stale prose the spec's **Stale prose at the edit sites** section names
(items 1–4 and 6 are corrections; item 5 is the one net-new documentation
requirement). This is the only functional-behavior change in the whole
feature — every rendering surface already reads `NavSection` generically and
needs no edit (see `plan.md`'s Overview).

## Files to change

1. `apps/web/lib/plugins/types.ts` — `NavItem.section`:
   ```ts
   section?: "main" | "settings" | "integrations";
   ```
   becomes
   ```ts
   section?: "main" | "settings" | "integrations" | "insights";
   ```
   and its doc comment (currently documents only `"main"` and `"integrations"`)
   is extended to cover all four: `"insights"` renders in the sidebar footer
   icon row and the phone Utilities group; `"settings"` is accepted and
   renders on no surface.

2. `docs/plans/plugins/PLUGIN-API.md` (around line 323–327) — the `NavItem`
   snippet's `section` union gains `"insights"`, matching step 1. The comment
   block above it currently reads:
   ```
   // section: "main" (default) renders as a top-level sidebar entry;
   // "integrations" renders inside the sidebar's Integrations section alongside
   // the first-party integration links (GitHub, Jira, ...). Hosts predating a
   // section value simply don't render items targeting it (additive change).
   ```
   The last sentence is false — the shipped host drops nothing; an
   unrecognised value degrades to `"main"`'s placement. Replace it with that
   degrade-to-`"main"` rule, and add a line for where `"insights"` renders
   (sidebar footer icon row / phone Utilities group).

3. `docs/public/plugins-authoring.md` line 184 — the `registerNavItem` row of
   the Frontend hook/API matrix currently reads *"section is main,
   integrations, or accepted-but-not-rendered settings"*. Add `insights` to
   the list and state it renders in the sidebar footer icon row and the phone
   Utilities group.

4. `apps/web/lib/navigation/plugin-destinations.ts`:
   - The `section` mapping line inside `pluginDestinations()`:
     ```ts
     section: item.section === "integrations" ? ("integrations" as const) : ("plugins" as const),
     ```
     becomes a three-way mapping — `"integrations"` → `"integrations"`,
     `"insights"` → `"insights"`, anything else (including omitted or an
     unrecognised string) → `"plugins"`. `"settings"` items are already
     filtered out one line above this mapping (`.filter((item) =>
     (item.section ?? "main") !== "settings")`) and stay filtered.
   - The docblock directly above `pluginDestinations()`:
     ```
     /**
      * `section: "settings"` items are skipped — those render in the settings tree's
      * `PluginSlot`, not as destinations.
      */
     ```
     The second clause is false: that slot is fed only by
     `registerComponent("settings-nav", …)`. Replace with: settings-section
     items are skipped and render on no surface.
   - The comment above `surfaces: SIDEBAR_AND_MENU`:
     ```
     // Plugins reach the palette through `registerShortcut`, not as nav
     // commands, so plugin destinations stay off that surface.
     ```
     `registerShortcut` does not exist anywhere in this repository. Replace
     with a statement that plugin destinations never declare the `palette`
     surface, without naming a substitute API (there is none — see spec
     **What**).

5. `apps/web/lib/navigation/plugin-destinations.test.ts`:
   - Retitle the test currently named
     `"keeps plugin items off the palette, which plugins reach via shortcuts"`
     to drop the false trailing clause (same falsehood as the `registerShortcut`
     comment above). The assertion body does not change.
   - Extend the `pluginItems` fixture with an `insights`-section entry and add
     assertions: it resolves to the `insights` section on both `sidebar` and
     `mobileMenu`; it is absent from `pluginsIn(...)` (`"plugins"` section)
     results on both surfaces; it is absent from the palette result (extend the
     retitled test's fixture and assertion).
   - Add a test for an unrecognised `section` string (e.g. a value not in the
     union, cast through the fixture): it appears in `pluginsIn(...)` and is
     absent from an `insights`-section resolution.
   - Add a test mirroring the existing "keeps two plugins that pick the same
     item id apart" case, using `section: "insights"` for both registrations.

6. `apps/web/lib/navigation/resolve-destinations.test.ts` — add:
   - A test that `resolveDestinations({ surface: "sidebar", section:
     "insights", pluginItems: [...] })` orders the first-party `stats` entry
     before a plugin `insights` entry.
   - A test that resolving `surface: "mobileMenu", section:
     MOBILE_MENU_UTILITY_SECTIONS` with a plugin `insights` item in
     `pluginItems` returns ids in the order `["stats", "settings",
     "plugin:acme:board"]` — the plugin row lands after `settings`, not
     adjacent to `stats` (spec **Ordering**, phone scenario).

## Acceptance

- `NavItem.section` accepts `"insights"` in `types.ts`, mirrored in
  `PLUGIN-API.md`'s Before/After snippet.
- `pluginDestinations()` maps `section: "insights"` items to the `insights`
  `NavSection` and nothing else changes about `"main"`/`"integrations"`/
  `"settings"`/unrecognised-string handling.
- All six stale-prose locations from the spec's **Stale prose at the edit
  sites** list read correctly (five corrections plus the one documentation
  requirement in `plugins-authoring.md`).
- New and updated tests in `plugin-destinations.test.ts` and
  `resolve-destinations.test.ts` pass.

## Verification

```
cd apps && pnpm --filter @kandev/web test -- lib/navigation/plugin-destinations.test.ts lib/navigation/resolve-destinations.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/plugins/types.ts`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/public/plugins-authoring.md`
- `apps/web/lib/navigation/plugin-destinations.ts`
- `apps/web/lib/navigation/plugin-destinations.test.ts`
- `apps/web/lib/navigation/resolve-destinations.test.ts`

## Dependencies

None.

## Parallelism

sequential (wave 1 — task 02 and task 03 both depend on this landing first).

## Inputs

- Spec sections: **What**, **API surface** (including **Stale prose at the
  edit sites** and **Section-to-placement mapping**), **Ordering**.
- Existing pattern: the `"integrations"` branch already in
  `pluginDestinations()` is the template for the new `"insights"` branch.

## Output contract

Summary of the mapping change and every stale-prose correction made, the
files changed, exact test commands run and their outcomes, and any blockers
or risks (e.g. if a consumer outside this list is found to depend on the old
three-value union). Update this file's `status` to `done` and this plan's
Wave 1 checkbox and **Verification Results** section in the same change.

## Results

Widened `NavItem.section` to `"main" | "settings" | "integrations" | "insights"`
in `apps/web/lib/plugins/types.ts` (doc comment extended to cover all four
values). Mirrored the union and the degrade-to-`"main"` rule in
`docs/plans/plugins/PLUGIN-API.md`. Added `insights` to the `registerNavItem`
row of `docs/public/plugins-authoring.md`'s Frontend hook/API matrix.

`pluginDestinations()` in `apps/web/lib/navigation/plugin-destinations.ts` now
routes `section` through a new `sectionFor()` helper: `"integrations"` →
`"integrations"`, `"insights"` → `"insights"`, everything else → `"plugins"`.
Corrected the docblock above `pluginDestinations()` (settings items render on
no surface, not in a `PluginSlot`) and the comment above `surfaces:
SIDEBAR_AND_MENU` (no `registerShortcut` API exists; plugin destinations never
declare `palette`).

Retitled `plugin-destinations.test.ts`'s palette test to
`"keeps plugin items off the palette"` (dropped the false "which plugins reach
via shortcuts" clause) and extended it to also assert an insights item is
excluded. Added: `insightsIn()` helper; a test routing insights items to the
insights section on both surfaces; a "moves, does not add" test against the
plugins group; an unrecognised-section-string degrade test; and an
insights-section two-plugin ordering/identity test. Added two tests to
`resolve-destinations.test.ts` (in a new `describe("resolveDestinations with
plugin items", …)` block, split out to stay under the 100-line
`max-lines-per-function` limit) proving `stats` precedes a plugin `insights`
item on the sidebar, and the mobile utility group orders `stats`, `settings`,
then the plugin item.

Commands run (from `apps/`):
- `pnpm --filter @kandev/web test -- lib/navigation/plugin-destinations.test.ts lib/navigation/resolve-destinations.test.ts` — 2 files, 25 tests passed.
- `cd web && pnpm run typecheck` — passed, no errors.
- `pnpm --filter @kandev/web lint -- lib/navigation/plugin-destinations.ts lib/navigation/plugin-destinations.test.ts lib/navigation/resolve-destinations.test.ts lib/plugins/types.ts` — 0 errors, 0 warnings (fixed two `sonarjs`/`max-lines-per-function` warnings found on first pass: extracted an `ACME_BOARD_ID` constant, split the new plugin-item tests into their own `describe` block).
- `pnpm run i18n:ratchet` — clean (no new hardcoded strings; 2 modified files, 0 added).

Files touched matched **Files likely touched** exactly. No blockers.
