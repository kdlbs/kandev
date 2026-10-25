---
id: "02-frontend-regression-coverage"
title: "Frontend regression coverage"
status: done
wave: 2
depends_on: ["01-contract-and-mapping"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/plugin-nav-sidebar-footer.md"
---

# Task 02: Frontend regression coverage

Prove, with component-level tests, that the desktop plugin rail, the phone
Plugins group, the desktop sidebar footer, and the phone Utilities group all
render `section: "insights"` plugin items correctly — placement, exclusion
from the `plugins` surfaces, ordering, test id, and the no-cap capacity
guarantee — without any component code change (they already read the
navigation manifest generically; task 01 is what makes the manifest resolve
`"insights"` correctly). No production code in `apps/web/components/**`
should need to change for this task; if you find one that does, that is a
signal task 01 missed something and the plan needs revisiting before
continuing here.

## Files to change

1. `apps/web/components/plugins/plugin-nav-items.test.tsx` (desktop rail) —
   add `it("keeps insights items out of the main plugin rail", ...)`,
   following the existing `"omits a nav item registered for a non-main
   section"` pattern (register one item with `section: "insights"`, assert no
   `plugin-nav-item-<id>` testid renders).

2. `apps/web/components/plugins/mobile-plugin-nav-section.test.tsx` (phone
   Plugins group) — extend the existing test
   `"omits non-main sections: integrations belongs to MobileIntegrationsSection,
   and settings is rendered nowhere"` with a third registration,
   `section: "insights"`, and confirm the assertion (`container.innerHTML` is
   empty) still holds with all three non-main sections registered. If adding
   the third registration changes what the test needs to assert, split it into
   a dedicated `it("also excludes insights-section items", ...)` case instead
   of forcing it into the existing one.

3. `apps/web/components/app-sidebar/app-sidebar-footer.test.tsx` (desktop
   footer) — the file mocks `useStaticDestinations` to return a fixed
   `stats`-only array; add:
   - A test that a plugin `insights` destination in that mocked array renders
     as an icon button with `data-testid="sidebar-plugin:acme:board-button"`
     (built from `sidebar-${destination.id}-button`, where `destination.id` is
     already the namespaced `plugin:acme:board` — no template change needed),
     accessible name equal to the destination's `label`, and that clicking it
     calls the mocked router's `push` with the destination's `href`.
   - A test (or two, expanded + collapsed) asserting the **Capacity and
     overflow** guarantee: mock `useStaticDestinations` to return `stats` plus
     eight synthetic plugin `insights` destinations, render `AppSidebarFooter`
     with `collapsed={false}`, assert eight `sidebar-plugin:*-button` testids
     are present (`getAllByTestId` on a regex-matched selector or by iterating
     the known ids) and the footer's outer container carries the wrapping-row
     classes it already applies when expanded. Render again with
     `collapsed={true}` and assert the same eight buttons are present with the
     column classes applied instead. Do not assert anything about visible
     height/clipping — the spec explicitly does not guarantee that (see
     **Capacity and overflow** / **Out of scope**).

4. `apps/web/components/navigation/app-nav-sheet.test.tsx` (`AppNavSections`,
   phone Utilities group) — the file currently mocks `usePluginRegistry` as
   `() => ({ getNavRegistrations: () => [] })`. Change this to read a mutable
   module-level array (e.g. `let navRegistrations: PluginNavRegistration[] =
   []`, reset to `[]` in the existing `beforeEach` blocks), then add a test in
   `describe("AppNavSections", ...)`:
   - Set `navRegistrations` to one `{ pluginId: "acme", id: "board", label:
     "Acme Board", path: "/plugins/acme", section: "insights" }`, render
     `<SectionsHost/>`, and assert the relative order of `Stats`, `Settings`,
     and `Acme Board` among `screen.getAllByRole("link")` matches that
     sequence (spec **Ordering**, phone scenario — the plugin row follows
     `Settings`, not `Stats`). Do not assert the full role-name sequence
     including Status/theme/Improve Kandev/Health — those are non-manifest
     controls the spec explicitly says a conformance test must not fail on.

## Acceptance

- All four test files above pass with the new/extended cases.
- No file under `apps/web/components/**` other than the test files listed
  changes. If one does, stop and reconcile with task 01 / the plan before
  proceeding — that would mean a surface is not as generic as `plan.md`'s
  Overview assumes.

## Verification

```
cd apps && pnpm --filter @kandev/web test -- components/plugins/plugin-nav-items.test.tsx components/plugins/mobile-plugin-nav-section.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx components/navigation/app-nav-sheet.test.tsx
```

## Files likely touched

- `apps/web/components/plugins/plugin-nav-items.test.tsx`
- `apps/web/components/plugins/mobile-plugin-nav-section.test.tsx`
- `apps/web/components/app-sidebar/app-sidebar-footer.test.tsx`
- `apps/web/components/navigation/app-nav-sheet.test.tsx`

## Dependencies

01-contract-and-mapping (needs `pluginDestinations()` to resolve `"insights"`
correctly before any of these assertions can pass).

## Parallelism

parallel-safe with 03-e2e-coverage — disjoint files (this task touches only
`apps/web/components/**` test files; task 03 touches the Go fixture bundle,
its packaged artifact, and Playwright spec files under `apps/web/e2e/**`).

## Inputs

- Spec **Scenarios** section (the desktop-rail-exclusion, phone-group-order,
  and eight-plugin capacity scenarios map directly to the tests above).
- Spec **Ordering** section's "What order means" subsection — a conformance
  test must constrain only the manifest rows' relative order, not the full
  visible row sequence.
- Existing patterns: `plugin-nav-items.test.tsx`'s `"omits a nav item
  registered for a non-main section"`, `mobile-plugin-nav-section.test.tsx`'s
  non-main-section test, `app-sidebar-footer.test.tsx`'s `useStaticDestinations`
  mock, `app-nav-sheet.test.tsx`'s `SectionsHost` helper.

## Output contract

Summary of every test added/changed, files touched, exact commands run with
outcomes, and any surface found to be non-generic (a blocker requiring a plan
update). Update this file's `status` to `done` and this plan's Wave 2
checkbox and **Verification Results** section in the same change.

## Results

No production code under `apps/web/components/**` changed — confirmed the
plan's assumption that every surface already reads the manifest generically.

- `plugin-nav-items.test.tsx`: added `"keeps insights items out of the main
  plugin rail"`.
- `mobile-plugin-nav-section.test.tsx`: added `"also excludes insights-section
  items: they belong to the Utilities group, not here"` (kept as a dedicated
  case rather than folding into the existing non-main-sections test, per the
  task's fallback instruction).
- `app-sidebar-footer.test.tsx`: changed the `useStaticDestinations` mock from
  a fixed array to a mutable `insightDestinations` (reset in
  `resetFooterState`), added `renderFooter(collapsed)` parameter, and added a
  new `describe("AppSidebarFooter plugin insights items", …)` block: testid +
  accessible name + click-navigates for one plugin destination, and two
  capacity tests (8 synthetic plugin destinations, expanded/`flex-wrap` and
  collapsed/`flex-col`, all 8 testids present in both).
- `app-nav-sheet.test.tsx`: changed the `usePluginRegistry` mock to read a
  mutable `navRegistrations` array (reset in all three `beforeEach` blocks),
  added a test asserting the Utilities group's manifest-row order is `Stats`
  → `Settings` → plugin `insights` item.

Commands run (from `apps/`):
- `pnpm --filter @kandev/web test -- components/plugins/plugin-nav-items.test.tsx components/plugins/mobile-plugin-nav-section.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx` — 3 files, 34 tests passed.
- `pnpm --filter @kandev/web test -- components/navigation/app-nav-sheet.test.tsx` — 1 file, 12 tests passed.
- `cd web && pnpm run typecheck` — passed.
- `pnpm --filter @kandev/web lint -- <8 touched files>` — 0 errors, 0 warnings after fixing the two warnings surfaced in task 01's files.
- `pnpm run i18n:ratchet` (from `apps/web`) — clean.
- Combined rerun: `pnpm --filter @kandev/web test -- lib/navigation/plugin-destinations.test.ts lib/navigation/resolve-destinations.test.ts components/plugins/plugin-nav-items.test.tsx components/plugins/mobile-plugin-nav-section.test.tsx components/app-sidebar/app-sidebar-footer.test.tsx components/navigation/app-nav-sheet.test.tsx` — 6 files, 71 tests passed.

One tooling note, not a code blocker: running `pnpm run test -- <files>` via
`cd web && pnpm run test -- ...` (as opposed to `pnpm --filter @kandev/web
test -- ...` from `apps/`) caused pnpm to forward a literal `--` into the
vitest invocation, which made vitest ignore the file filters and run the
entire suite (~15 min before it was killed). Use the `pnpm --filter @kandev/web
test -- <files>` form from `apps/` for targeted runs.

Files touched matched **Files likely touched** exactly. No blockers.
