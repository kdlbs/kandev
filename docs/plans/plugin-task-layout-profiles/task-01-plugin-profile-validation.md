---
id: "01-plugin-profile-validation"
title: "Registry-independent plugin profile eligibility"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-LAYOUT-PROFILES-001
  - REQ-UI-TASK-LAYOUT-PROFILES-002
acceptance_criteria:
  - AC-UI-TASK-LAYOUT-PROFILES-001.5
  - AC-UI-TASK-LAYOUT-PROFILES-002.4
  - AC-UI-TASK-LAYOUT-PROFILES-002.5
  - AC-UI-TASK-LAYOUT-PROFILES-002.6
system_design:
  - ../../specs/ui/system-design/task-layout-profiles.md
  - ../../specs/ui/system-design/task-layout-plugin-panels.md
---

# Task 01: Registry-independent Plugin Profile Eligibility

## Summary

Make structurally correct generic plugin panels reusable without requiring a
live registration. Reject malformed plugin definitions before retirement
filtering/session normalization can discard or rewrite them, preserving the
existing profile and Agent guarantees.

## In scope

- `validateReusableLayout`/`validateGroup` plugin structural checks using the
  existing generic constants, parser, and ID helper.
- Prevalidation of plugin-intended raw definitions in authoritative tree/flat
  groups before component sanitization and session normalization.
- Global duplicate counting, existing Agent/empty-group/active-tab checks,
  immutable validation, unavailable-plugin compatibility/default resolution.
- Focused tests for override/custom-profile operations and nested structure.
- Preserve unchanged `LayoutState` metadata, especially `rootOrientation`,
  in `session-panels.ts`'s shared Chat rewrite and `serializer.ts`'s
  `filterEphemeral`, covering normalization, materialization, and workbench
  reusable-profile capture.
- Axis-correct `toSerializedDockview` sizing and capture: VERTICAL root children
  use height, recursion uses width as the orthogonal extent, and horizontal
  pin overrides/caps or inferred Files/Changes pinning cannot alter those rows.
  Preserve existing root-extent fields and nested weights; no schema migration.

## Out of scope

- Registry discovery/menu actions, rendering plugin Components, or runtime
  revocation policy changes; Task 02 owns the user-visible vertical flow.
- Plugin grammar changes, plugin-specific allowlists, backend APIs/schema,
  retired core Prompt History revival, and unrelated retired-panel behavior.

## Acceptance

1. A canonical generic plugin definition validates with an empty, loading,
   removed, or changing registry; profile compatibility and effective-default
   resolution retain that reference for custom profiles and built-in overrides.
2. Missing/empty identity parts under the existing parser, ID/component/tab/params
   mismatch, cross-group/tree duplicate identity, and Agent/group/selection
   violations fail without silently dropping plugin-intended panels or mutating
   input. Existing retired-component cleanup stays unchanged for other panels.
3. Focused RED/GREEN behavior tests prove the new eligibility and failure cases,
   including JSON/profile-operation round trip preserving nested placement,
   selections, params, geometry, and explicit root orientation through
   normalization, Agent materialization, and ephemeral filtering. Cover
   VERTICAL, HORIZONTAL, and missing-orientation legacy compatibility.
4. On a non-square VERTICAL host, serializer output and actual Dockview
   round trip retain unequal root-axis proportions and nested widths, including
   a Files/Changes/plugin child with an old pinned flag. Horizontal targets and
   preferences cannot affect vertical serialization. Explicit HORIZONTAL and
   omitted orientation retain current width/cap/default behavior.

## Verification

From the repository root, bootstrap once if this worktree has not been installed:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Do not repeat successful setup. Follow `/tdd`: run each new behavior test before
production edits and record the specific expected assertion failure, then run
it GREEN. Use the complete focused command after the change:

```bash
(cd apps/web && pnpm exec vitest run lib/layout/layout-profiles.test.ts lib/layout/layout-profiles.plugin-panels.test.ts lib/state/layout-manager/plugin-panels.test.ts lib/state/layout-manager/session-panels.test.ts lib/state/layout-manager/sizing.test.ts lib/state/layout-manager/serializer.test.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

Task 02 supplies runtime geometry guards/application, actual browser smoke,
full requested checks, and documentation. Do not describe these unit tests as
runtime/browser proof or mark the plan implemented when only this order passes.

## Files likely touched

- `apps/web/lib/layout/layout-profiles.ts`
- `apps/web/lib/layout/layout-profiles.test.ts` (unchanged) and new `layout-profiles.plugin-panels.test.ts`
- `apps/web/lib/state/layout-manager/session-panels.ts` and `.test.ts`
- `apps/web/lib/state/layout-manager/serializer.ts` and `.test.ts`: metadata,
  root/orthogonal-axis serialization, and capture pin inference.
- `apps/web/lib/state/layout-manager/sizing.ts` and `.test.ts` for local
  axis-correct root sizing without horizontal pin policy on vertical rows.
- `apps/web/lib/state/layout-manager/plugin-panels.ts` and `.test.ts` only if a
  small pure identity-shape helper removes actual duplication.
- This work order and `plan.md` for execution status/results.

## Dependencies

None. Task 02 depends on the structural eligibility established here.

## Risks

- `filterLayoutStateByComponents` currently runs before panel eligibility;
  malformed plugin references cannot be allowed to disappear through that path.
- A plugin-prefixed ID with a Chat component must not normalize into Agent.
- `isKnownPanelId` and `resolvePluginPanelDefinition` are registry-dependent;
  using either as a validity guard breaks boot/reload/uninstalled references.
- `column.tree` is authoritative when present; flat `groups` is a projection,
  not additional occurrences of those panels.
- Do not tighten `parsePluginPanelId` grammar beyond current helper semantics.
- Shared Chat rewrites and ephemeral filtering currently return only columns;
  preserve unchanged state metadata rather than assuming tree retention implies
  root-orientation retention.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/task-layout-profiles.md), especially
  REQ-UI-TASK-LAYOUT-PROFILES-002 and criteria .4-.6.
- [Plugin extension design](../../specs/ui/system-design/task-layout-plugin-panels.md),
  Structural validation and Preview/serialization/persistence sections.
- [Baseline design](../../specs/ui/system-design/task-layout-profiles.md).
- Existing layout-profile and plugin-panel tests; current retirement filter,
  session normalization, `groupsIn`, and built-in override conventions.
- User-provided installed-binary validator failure is ground truth, not a request
  to rerun the old failure. New TDD tests establish current-code RED separately.

## Results

Executed 2026-10-18 (TDD, one behavior at a time).

- RED 1 (`vitest run lib/layout/layout-profiles.test.ts`, appended to that file at the time; the cases now live in `lib/layout/layout-profiles.plugin-panels.test.ts` to stay under the 600-line file limit): 26 new plugin
  behavior cases failed on their assertions (`expected false to be true` for a
  canonical plugin panel, and `valid: true`/`unsupported-panel` instead of
  `invalid-panel` for malformed or Chat-component plugin IDs). GREEN: local
  `isPluginIntendedPanel`/`isCanonicalPluginPanel`/`pluginDefinitionIssues` in
  `layout-profiles.ts` run on the raw layout before component filtering and
  Agent normalization; `validateGroup` counts canonical plugin IDs in the shared
  duplicate map. `PLUGIN_PANEL_PREFIX` is now exported from `plugin-panels.ts`.
- RED 2 (`session-panels.test.ts`): explicit VERTICAL/HORIZONTAL orientation
  was `undefined` after normalization/materialization. GREEN: `rewriteReusableChatPanels`
  returns `{ ...state, columns }`.
- RED 3 (`serializer.test.ts`): VERTICAL root `size` was 700 not 1400, rows were
  `[1220, 180]` (horizontal pin policy) instead of `[150, 200]`/`[300, 400]`,
  `fromDockviewApi` inferred `pinned: true`, and `filterEphemeral` dropped the
  orientation. GREEN: `computeRootAxisSizes` in `sizing.ts` (VERTICAL uses
  height, orthogonal width, ignores pin overrides/caps/flags; HORIZONTAL and
  legacy unchanged), `serializeColumn` takes axis/orthogonal extents,
  `fromDockviewApi` only infers `pinned` for a HORIZONTAL root, and
  `filterEphemeral` returns `{ ...state, columns }`. A real Dockview (jsdom
  `createDockview`) round trip asserts root-axis proportions (300/400), nested
  widths (233/467/700) and no inferred pin.
- GREEN: `(cd apps/web && pnpm exec vitest run lib/layout/layout-profiles.test.ts lib/layout/layout-profiles.plugin-panels.test.ts lib/state/layout-manager/plugin-panels.test.ts lib/state/layout-manager/session-panels.test.ts lib/state/layout-manager/sizing.test.ts lib/state/layout-manager/serializer.test.ts)`:
  6 files, 112 tests passed (rerun after the final production edit).
- Regression: `vitest run lib/state lib/layout components/task/dockview components/settings/layouts`:
  179 files passed; the only failure is `lib/state/slices/task-overview.test.ts`
  "rejects expired read protection after overflow or a context change", a
  timing test that exceeded its budget under load and exercises no touched module.
  (`pnpm install --frozen-lockfile` was required first: `@json-render/core`
  was missing from the worktree's `node_modules`.)
- `python3 scripts/list-docs.py validate`: 386 decisions, 1646 specifications
  validated. `python3 scripts/lint-spec-files.py --all`: all passed.
- These are unit results, not browser/runtime proof; Task 02 owns runtime
  geometry guards and browser smoke.
