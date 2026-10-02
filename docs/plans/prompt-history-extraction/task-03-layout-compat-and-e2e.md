---
id: "03-layout-compat-and-e2e"
title: "Harden and prove layout compatibility"
status: done
wave: 3
depends_on:
  - "02-remove-core-projection"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-EXTRACTION-001
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.4
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.6
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.7
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.9
  - AC-PLUGINS-PROMPT-HISTORY-EXTRACTION-001.10
system_design:
  - ../../specs/plugins/system-design/prompt-history-extraction.md
---

# Task 03: Harden and prove layout compatibility

## Summary

Make the retired panel a non-event for saved layout state, and replace the
browser coverage the removed panel owned with the two behaviors that still need
end-to-end evidence: an upgraded saved layout that contains the retired panel
restores cleanly, and the replacement plugin still works over the retained Host
contracts on desktop and phone.

## In scope

- Rewrite the retired-panel case in
  `apps/web/e2e/tests/settings/layout-profiles.spec.ts`. The existing
  "adds Prompt History through the layout editor and restores it into a task"
  test asserts removed behavior and its `layouts.addPanel("Prompt History")`
  step no longer exists. Split it into:
  - the same layout-editor round-trip using a surviving reusable panel
    (`Todos`), so the editor's add-save-restore coverage is not lost;
  - a new case that seeds `saved_layouts` whose layout JSON contains a
    `prompt-history` panel next to canonical panels (`chat`, `todos`, `plan`),
    makes it the default, opens a new task, and asserts the workbench opens
    without a `Prompt History` tab while the seeded panels are present, sized, and
    usable. Assert that absence by tab role and label (`.dv-tab` with the text),
    not by a `prompt-history-panel` test id: Task 01 deletes that id from the
    whole web tree, so the id assertion could never fail on the very case that
    exists to prove the drop.
- Cover the settings route, not only the task route. Seed a saved
  profile/default that contains the retired panel, open
  `/settings/preferences/layouts`, and assert the observable the UI actually
  renders: the profile shows its default badge (`settings:default`) and does not
  show `settings:unavailable` (`components/settings/layouts/layout-profile-list.tsx`).
  Do not assert on the internal `legacy` status string: nothing renders it, so
  such a check would pass even after a demotion. Then open a task and assert the
  layout applies with the other panels present. Keep this whole seeded-profile
  flow in `apps/web/e2e/tests/settings/layout-profiles.spec.ts`, next to the
  rewritten retired-panel case; the `prompt-history-removed.spec.ts` case keeps
  the "+"-menu row assertions in a default task.
- Add `apps/web/e2e/tests/task/prompt-history-removed.spec.ts` (chromium): open
  the task workbench "+" menu and assert `add-panel-prompt-history-item` is
  absent while the rows that state actually renders are present. Assert row
  presence by role and label, and only for rows that exist in the seeded state:
  the built-in default preset opens Chat, Files, Changes, and the default
  terminal (`presets.ts`), and the Changes and Files rows render only while
  those panels are _closed_ (`dockview-add-panel-items.tsx`), so a default task
  shows neither. Either close them first, seed a per-environment layout without
  them, or assert presence for the rows that do render (Plan, VS Code, Browser,
  Todos, terminals) and use test ids only where they exist
  (`add-panel-prompt-history-item` for absence; the Todos, Changes, and Files
  rows carry no `data-testid` and are located by role and label).
  Do not assert the absence of a `Prompt History` tab in the default layout: the
  built-in preset cannot contain it, so that assertion cannot fail. Seed the
  per-environment layout with a retired panel instead when the case needs a
  pre-existing tab.
- Add `apps/web/e2e/tests/task/mobile-prompt-history-removed.spec.ts`
  (mobile-chrome, `mobile-` filename prefix per the existing convention): open
  the `Panels` sheet on a task that has a canvas or an enabled plugin panel and
  assert `mobile-prompt-history-option` is absent while the sheet still lists
  those entries, and assert the task still exposes Chat, Plan, Changes, Files,
  and Terminal navigation.
- Locate rows correctly: only the PR, PR-submenu, plugin, canvas,
  prompt-history, port-forwarding, MR, and review rows carry `data-testid` today
  (`dockview-add-panel-items.tsx`). Todos, Plan, Changes, Files, Browser, and
  VS Code rows have none, so locate them as menu items by role and label (for
  example `getByRole("menuitem", { name: "Todos" })`). Assert the removed row's
  absence with Playwright's locator API — the suite has no `queryByTestId`:
  `expect(testPage.getByTestId("add-panel-prompt-history-item")).toHaveCount(0)`
  is the (vacuous, since Task 01 deletes that id) companion, and
  `expect(testPage.getByRole("menuitem", { name: "Prompt history" })).toHaveCount(0)`
  is the assertion with power. Assert the mobile sheet's absence the same way,
  by label plus `mobile-prompt-history-option`.
- Harden both readers of the maximized-state blob.
  `applySavedMaximize` in
  `apps/web/components/task/dockview-layout-restore.ts` and
  `restoreMaximizeFromStorage` in `apps/web/lib/state/dockview-store.ts` (reached
  from the environment-switch path through `restoreIncomingMaximize`) each hand
  `maximizedDockviewJson` to `api.fromJSON` and keep `preMaximizeLayout` without
  filtering, so the retired identity reaches dockview on either route and the
  failure path deletes the blob with `removeEnvMaximizeState`, which drops only
  that environment's maximize blob and the `preMaximizeLayout` copy it holds; the
  environment's saved layout lives under its own key and stays in place. Move the serialized-layout
  sanitizer to a neutral module
  (`apps/web/lib/state/layout-manager/sanitize-serialized-layout.ts`) so the
  store can use it without importing a component module that already imports the
  store. Every consumer needs the same component-name set, and it must be
  **static**: `dockview-desktop-layout.tsx` is loaded through
  `dynamic(() => import(...), { ssr: false })`, and the settings route that
  validates saved profiles (`/settings/preferences/layouts` →
  `LayoutSettings` → `use-layout-settings.ts` → `lib/layout/layout-profiles.ts`)
  never imports it, so a set populated at that module's load would be empty
  there. A second new module therefore owns the renderable names —
  `apps/web/lib/state/layout-manager/renderable-components.ts`, exporting
  `RENDERABLE_COMPONENT_NAMES` and an `isRenderableComponent()` predicate built
  from the renderer's component map as it stands after Task 01 — and every
  consumer (the serialized sanitizer, the `LayoutState` filter, the profile
  normalizer, and the retained-column prune below) reads it. The names live in
  that module, not in the sanitizer. Prefer building `components` in
  `dockview-desktop-layout.tsx` from that list, or at minimum derive
  `DESKTOP_VALID_COMPONENTS` from it, and pin the invariant with an observable
  assertion (below). Keep the on-ready route parameterized as well:
  `tryRestoreLayout` already receives `validComponents`, so pass that value down
  into `applyFixupsWithMaximize` and `tryRestoreMaximizeOnly` instead. No
  registry, no load-order dependency, and no fail-open rule to reason about —
  the settings route and the task route use the same predicate.
  - Serialized `maximizedDockviewJson`: the moved `sanitizeSerializedLayout`
    (today's `sanitizeLayout`, renamed) with that set (it requires `panels` plus
    `grid.root`). The move has no re-export shim: `dockview-layout-restore.ts`
    stops exporting it and `dockview-layout-restore.test.ts` re-points its
    import with its cases.
  - `preMaximizeLayout` `LayoutState`: a filter over
    `columns[].groups[].panels` (and the optional tree) that removes panels whose
    component is not in the set, drops a group or column left empty _by that
    filter_, and repoints or clears a dangling `activePanel`. Do not drop
    pre-existing empty groups: they are live state that the current restore
    preserves, and an existing environment-switch case asserts a layout in that
    shape stays valid. `rewriteReusableChatPanels` in
    `lib/state/layout-manager/session-panels.ts` is the model for that
    null-propagation, not for the return shape: it returns `{ columns }`, while
    `LayoutState` also carries `rootOrientation`, which is live state
    (`toSerializedDockview` reads it, `fromDockviewApi` captures a vertical
    root, and the right-pane heuristic compares it with stored metadata). Return
    `{ ...state, columns }` so a maximize restore cannot silently flip the root
    orientation. Do **not** use
    `filterEphemeral` here: it is the capture-time ephemeral filter, its set
    (`KNOWN_PANEL_IDS` plus `STRUCTURAL_COMPONENTS`) omits `canvas`,
    `review-detail`, `file-editor`, `diff-viewer`, `commit-detail`, and the
    legacy aliases, and it deliberately preserves empty groups for split
    preservation. Filtering a live pre-maximize layout with it would drop
    panels the user still has, which AC 001.7 forbids.
    Skip the overlay per reader when the maximized group does not survive, and
    make the two different outcomes explicit: the on-ready maximize-only reader
    (`tryRestoreMaximizeOnly`, reached only when no usable per-environment layout
    exists) must apply the filtered `preMaximizeLayout` and return true with
    `preMaximizeLayout`/`maximizedGroupId` cleared — returning false there ends in
    the built-in default layout and discards the panels the blob holds; the
    environment-switch reader returns false so `performEnvSwitch` applies the
    environment's sanitized saved layout, which is normally the same pre-maximize
    layout. Keep the existing self-heal deletion only for a blob that still fails
    after sanitization.
- Prune the retained hidden-right-pane column. `HiddenRightPane.column` is a
  whole `LayoutColumn` with panel definitions, written with the per-environment
  layout and read back shape-validated only; without pruning, the retired
  component can reach the grid on restore or leave a "show right panels" control
  that cannot act when no renderable panel survives.
  `readHiddenRightPane` prunes the column on the way in and returns null when
  nothing survives. This function is the funnel for all three readers:
  `restoreMaximizeFromStorage` and the environment-switch handler in
  `apps/web/lib/state/dockview-store.ts`, plus `setupReadyDockview` in
  `apps/web/components/task/dockview-desktop-layout.tsx`, which populates the
  store on an ordinary page load. `restoreRightPane` also filters immediately
  before insertion as a guard for callers holding a captured pane. The next
  capture then persists the pruned column.
  Add four unit cases in `apps/web/lib/state/dockview-right-pane.test.ts`: a
  retired-only column returns null from `restoreRightPane`; a mixed column
  inserts only the survivors and preserves its geometry and active panel; a
  retired-only stored column reads as null and leaves the toggle unavailable;
  and a mixed stored column reads with only the renderable panels and its active
  panel repaired. A retired-only column cannot also assert survivor insertion,
  because the emptied column is dropped.
- Sanitize the two remaining stored payloads that still reach the renderer
  unfiltered, using the shared helpers above with the renderable-component
  predicate:
  - `apps/web/lib/state/dockview-env-switch.ts`: `getHealthyEnvLayout` only
    checks shape health, and the slow path in `performEnvSwitch` hands the
    stored per-environment layout straight to `restoreSerializedDockview` →
    `api.fromJSON`. Sanitize the serialized payload before restoring, using the
    same immutable local for the active-view replay and right-column width read;
    `replaceStaleSessionPanels` takes no payload and is unaffected. The fast path
    does not cover this: its fingerprint comparison only inspects structural
    components, so any structural difference (an extra Plan, terminal, or PR
    group, a pinned file editor) routes through the slow path even when the
    layouts otherwise match.
  - `apps/web/lib/state/dockview-store.ts` (`restoreCustomLayout`, both
    branches): `applyCustomLayout` receives the raw stored profile from
    `components/task/layout-preset-selector.tsx`, and
    `normalizeReusableSessionPanels` / `materializeReusableChatPanel` pass
    non-chat panels through unchanged. Filter the incoming layout before it is
    applied, so the profile-normalization change above is not the only guard —
    that one covers validation and default resolution, not this explicit apply.
- Make a saved layout profile tolerate the retired id. Today
  `validateReusableLayout` in `apps/web/lib/layout/layout-profiles.ts` reports
  `unsupported-panel` for any id outside `REUSABLE_PANEL_IDS`, so once Task 01
  removes `prompt-history` from that set, `resolveEffectiveDefaultLayout` throws
  the user's customized default away and falls back to the built-in default.
  Normalize the profile before validation by dropping panels whose **component
  is no longer renderable** — the same static predicate the apply-path filter
  uses — then dropping a group or column left empty and repointing or clearing a
  dangling `activePanel`, mirroring `normalizeReusableSessionPanels` (whose
  `rewriteGroup`/`rewriteColumn` null-propagation is the pattern for the emptied
  case). The empty-group clause is not optional: `validateGroup` rejects a group
  with no panels, so keeping one reruns the exact failure the normalization
  exists to prevent.
  Do not generalize the drop to "any id outside `REUSABLE_PANEL_IDS`":
  `validateGroup`'s `unsupported-panel` check must stay reachable for ids that
  are not reusable but whose component still exists — the dynamic
  `pr-detail|<key>` and `mr-detail|<key>` panels a captured layout can carry —
  and the existing case `rejects an unsupported panel` in
  `apps/web/lib/layout/layout-profiles.test.ts` (a `chat` panel beside a
  `pr-detail|owner/repository/123` panel) must keep passing as it stands. Bound
  the emptied-group drop and the `activePanel` repair to the panels the drop
  actually removed, so the file's `empty-group` and `invalid-active-panel` cases
  keep their meaning.
- Add the unit cases. No seeding is needed: the predicate is a module constant,
  so every route behaves the same in a test as in the app, and only the
  parameterized `tryRestoreLayout` cases pass a set of their own:
  - `apps/web/components/task/dockview-layout-restore.test.ts` (parameterized
    route): a saved layout containing the retired component drops only that
    panel; a maximized blob whose layout contains it is applied without it while
    the rest of the maximized layout survives, including an unchanged
    `rootOrientation`; and, on the maximize-only route, a blob whose maximized
    group was the retired panel alone restores the filtered `preMaximizeLayout`
    with no maximize state rather than falling through to the default layout.
  - `apps/web/components/task/dockview-desktop-layout.test.ts`: export the
    renderer's component names from that module (for example as
    `DESKTOP_COMPONENT_NAMES`) and assert they equal
    `RENDERABLE_COMPONENT_NAMES`, so the static set cannot drift from what the
    desktop renderer can instantiate. Pair it with the direction that is not
    tautological, and make the assertion discriminating: `renderPanel`
    (`apps/web/components/task/dockview-panel-content.tsx`) resolves a component
    through its alias map before looking up a renderer, and a miss returns the
    `unknownPanel` placeholder rather than throwing, so "does not throw" or
    "renders something" passes for a name with no renderer at all. Assert instead
    that no name in `RENDERABLE_COMPONENT_NAMES` renders the unknownPanel
    fallback, and that each alias renders the same node as its canonical target
    (`diff-files` as `changes`, `all-files` as `files`). Do not assert against
    `PANEL_RENDERERS` (unexported), and do not require alias entries there: the
    legacy names must stay in the list and keep resolving through the alias map,
    or stored legacy panels would stop restoring.
  - `apps/web/lib/state/dockview-env-switch-action.test.ts`, in the
    "switchEnvLayout — maximize+sidebar-switch regression" suite, which already
    mocks `getEnvMaximizeState` and asserts `preMaximizeLayout`,
    `maximizedGroupId`, and `rightPanelsVisible`: the maximize route on an
    environment switch, plus the slow path in `performEnvSwitch` applied to a
    stored per-environment layout containing the retired component, asserting it
    is dropped while the other panels survive and the switch does not fall back
    to the default layout. `dockview-store.test.ts` cannot host it:
    `restoreMaximizeFromStorage` is module-private and that file's fake api does
    not drive `switchEnvLayout`.
  - `apps/web/lib/state/dockview-preset-persistence.test.ts`, which already
    drives `applyCustomLayout`: applying a saved profile whose layout contains
    the retired component drops it on both the `columns` branch and the legacy
    serialized branch while the other panels apply. Keep that as the single home
    — the gate and Files list name only this path.
  - `apps/web/lib/layout/layout-profiles.test.ts`: a default profile whose
    layout contains the retired reusable id still validates, keeps its other
    panels, and resolves as the effective custom default rather than falling
    back to the built-in one. Seed the retired id as the _only_ panel of its
    group in one case, so the emptied-group clause is exercised; a seed that
    co-locates it with `chat` leaves no empty group and passes either way. Build
    that panel as a literal object
    (`{ id: "prompt-history", component: "prompt-history", title: "Prompt History" }`)
    rather than through the file's `reusableLayout`/`panel` helpers: `panel(id)`
    throws `Unknown panel: <id>` once Task 01 removes the registry entry, so the
    helper would fail for an unrelated-looking reason. Any panel whose component
    is absent from the renderer registry works the same way; an id that is merely
    non-reusable but still renderable is not dropped and does not exercise this
    path.
- Confirm the retained Host surface with the existing plugin suites; do not
  modify them unless they fail for a reason this package caused.
- Verify the retained Host design's coverage and inventory rows while sweeping:
  the prompt-derivation row must cite `apps/web/lib/turn-duration.test.ts` and the
  fixture parity suites, the panel-state and end-to-end rows must cite
  `tests/plugins/prompt-history-plugin.spec.ts` and
  `tests/plugins/mobile-prompt-history-plugin.spec.ts`, and the inventory must
  record that the panel requirement and design are deprecated and superseded.
  They already read that way; correct them only if the implementation leaves them
  stale or if they still cite the files Tasks 01 and 02 delete. That file has
  headroom; the Host prerequisite _requirement_ does not (see below).
- Keep `docs/specs/plugins/requirements/prompt-history-extraction-host.md`
  length-neutral: it has about 65 bytes of headroom against the 20 KiB
  requirement limit and no size-exception path, so any correction there must be
  a net reduction, with added detail going into the Host design instead.
- Confirm the documentation posture across the repository, not only
  `docs/public`: search `docs/**` for statements that core still owns the
  prompt-history surface or its projection, and correct any that this package
  invalidates (for example a system design that attributes the separate
  user-message request coordinator to core, or a renderer list that names the
  removed panel). Update a public page only if one names the removed panel.

## Out of scope

- Any change to the browser conversation façade, plugin panel registration,
  navigation, favorites, or alias rendering to make a test pass.
- Migrating stored layouts to the plugin's panel id, seeding plugin data into
  user profiles, or adding a compatibility panel.
- Re-adding any part of the removed panel or its specs.

## Acceptance

1. A saved layout, saved profile, per-environment layout, or maximized-state
   blob that contains the retired `prompt-history` panel restores without the
   panel, without an error or empty surface, and with every other panel, group,
   and size preserved — on every route that applies a stored payload (the
   on-ready restore, the environment-switch slow path, the preset dropdown, the
   custom-default build, both maximize readers, and the retained hidden-right-pane
   column). The environment-switch fast path applies no stored payload, so it
   needs no case; a maximized group that still has a surviving panel stays
   maximized. No user action is required, and no saved layout or profile is
   deleted during the upgrade.
2. With no plugin installed, the workbench offers no Prompt history row or
   option on desktop or phone, and every other surface still works.
3. The plugin fixture suites pass on both projects against the retained
   contracts, proving the replacement still has the API surface it needs.

## ASCII UI preview

Relevant excerpt of the plan's previews (see
[the plan](plan.md#ascii-ui-preview)):

```text
UI-03: Saved layout containing the retired panel (required behavior)

BEFORE                              AFTER
+-----------+-------------+         +-----------+-------------+
| Chat      | Prompt      |         | Chat      | Todos       |
|           | history     |         |           |             |
+-----------+-------------+         +-----------+-------------+
The stored entry is dropped on restore; other panels, sizes, and
maximize state are preserved, with no notice.
```

## Verification

Run from the repository root. E2E uses the managed runner; run the two projects
separately and do not pass all-worker overrides.

```bash
(cd apps/web && pnpm exec vitest run \
  components/task/dockview-layout-restore.test.ts \
  components/task/dockview-desktop-layout.test.ts \
  lib/state/dockview-right-pane.test.ts \
  lib/state/dockview-env-switch-action.test.ts \
  lib/state/dockview-preset-persistence.test.ts \
  lib/layout/layout-profiles.test.ts)
(cd apps/web && pnpm e2e:run --project chromium \
  tests/task/prompt-history-removed.spec.ts \
  tests/settings/layout-profiles.spec.ts \
  tests/plugins/prompt-history-plugin.spec.ts \
  tests/plugins/conversation-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-prompt-history-removed.spec.ts \
  tests/settings/mobile-layout-profiles.spec.ts \
  tests/plugins/mobile-prompt-history-plugin.spec.ts \
  tests/plugins/mobile-plugin-task-panel.spec.ts)
rg -n -i -U -e "prompt[ _-]?history" docs
```

The documentation search covers the whole `docs` tree, not only `docs/public`.
Expected matches are: this package's own files; the retained Host prerequisite
requirement and design, whose `REQ`/`AC` ids contain the words, and the designs
that cite those ids (`plugins/system-design/conversation-recovery.md`,
`conversation-source-reconciliation.md`, `docs/decisions/2026-09-16-conversation-source-reconciliation.md`);
the deprecated and superseded panel specification pair; the live specs this
package edited, whose link labels name it
(`ui/requirements/prompt-alias-rendering.md`,
`ui/system-design/prompt-alias-rendering.md`) and the canonical browser contract
`docs/plans/plugins/PLUGIN-API.md`; the decisions that link them; the
tasks-system passages that mean the durable `task_session_prompt_seq`
ordinal (`tasks/requirements|system-design/workflow-step-agent-start-ownership.md`,
`tasks/system-design/initial-task-brief.md`); `ui/requirements/bounded-user-message-rendering.md`
(plugin views); and historical plans. Within `docs/public`, only the
browser-conversation-facade passages describing `host.conversation` and the
Host-only stream may match. A match that claims core still owns the panel or its
projection is a defect.

## Files likely touched

- `apps/web/components/task/dockview-layout-restore.ts`
- `apps/web/components/task/dockview-layout-restore.test.ts`
- `apps/web/lib/state/dockview-store.ts`
- `apps/web/lib/state/dockview-right-pane.ts`
- `apps/web/lib/state/dockview-right-pane.test.ts`
- `apps/web/lib/state/dockview-env-switch.ts`
- `apps/web/lib/state/dockview-env-switch-action.test.ts`
- `apps/web/lib/state/dockview-preset-persistence.test.ts`
- `apps/web/components/task/dockview-desktop-layout.tsx`
- `apps/web/components/task/dockview-desktop-layout.test.ts`
- `apps/web/lib/state/layout-manager/sanitize-serialized-layout.ts` (new)
- `apps/web/lib/state/layout-manager/renderable-components.ts` (new)
- `apps/web/lib/layout/layout-profiles.ts`
- `apps/web/lib/layout/layout-profiles.test.ts`
- `apps/web/e2e/tests/settings/layout-profiles.spec.ts`
- `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts` (rewritten in Task 01)
- `apps/web/e2e/tests/plugins/mobile-plugin-task-panel.spec.ts` (rewritten in Task 01)
- `apps/web/e2e/tests/task/prompt-history-removed.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-prompt-history-removed.spec.ts` (new)
- `apps/web/e2e/tests/task/prompt-history-panel.spec.ts`,
  `prompt-history-auto-load.spec.ts`, `mobile-prompt-history-panel.spec.ts`,
  `apps/web/e2e/helpers/prompt-history-long-seed.ts` (deleted in Task 01)
- `docs/public/*` and any `docs/specs/**` statement invalidated by the removal
  (only where a document still describes core prompt-history ownership)

## Risks

The maximize change must not disable maximize restore for healthy blobs: assert
a blob with only surviving panels still restores maximized. A seeded profile can
be dropped for an unrelated reason (invalid shape, size validation, missing
session panel) and make the case pass vacuously. Seed
canonical panels next to the retired one and assert those panels are visible, so
a wholesale layout rejection fails the test. The plugin suites already cover
pagination, ordinals, durations, aliases, favorites, navigation, and mobile
placement; if one fails, fix the cause inside this package's scope rather than
weakening the assertion.

## Dependencies

Depends on Tasks 01 and 02.

## Parallelism

`sequential`

## Inputs

- [Extraction requirements](../../specs/plugins/requirements/prompt-history-extraction.md)
  and [design](../../specs/plugins/system-design/prompt-history-extraction.md).
- `apps/web/components/task/dockview-layout-restore.ts` (today's `sanitizeLayout`,
  which requires the caller to pass the component set) and
  `apps/web/components/task/dockview-desktop-layout.tsx`, which derives
  `DESKTOP_VALID_COMPONENTS`, and the neutral module's static
  renderable-component list, which this work order introduces and keeps equal to
  the map.
- Existing plugin coverage: `tests/plugins/prompt-history-plugin.spec.ts`,
  `tests/plugins/mobile-prompt-history-plugin.spec.ts`,
  `tests/plugins/mobile-plugin-task-panel.spec.ts`.
- `apps/web/e2e/README.md` for runner and project conventions.

## Results

Done. Commands run from the repository root.

### Unit evidence

```bash
(cd apps/web && pnpm exec vitest run \
  components/task/dockview-layout-restore.test.ts \
  components/task/dockview-desktop-layout.test.ts \
  lib/state/dockview-right-pane.test.ts \
  lib/state/dockview-env-switch-action.test.ts \
  lib/state/dockview-preset-persistence.test.ts \
  lib/layout/layout-profiles.test.ts \
  lib/state/dockview-env-switch-maximize-session-filter.test.ts \
  components/task/dockview-layout-restore-maximize-session.test.ts)
# 8 files passed, 135 tests passed
(cd apps/web && pnpm run typecheck)   # tsc --noEmit clean
(cd apps/web && pnpm run lint)        # eslint --max-warnings 0 clean
```

### Browser evidence

```bash
(cd apps/web && pnpm e2e:run --project chromium \
  tests/task/prompt-history-removed.spec.ts \
  tests/settings/layout-profiles.spec.ts \
  tests/plugins/prompt-history-plugin.spec.ts \
  tests/plugins/conversation-recovery.spec.ts)
# 11 passed

(cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-prompt-history-removed.spec.ts \
  tests/settings/mobile-layout-profiles.spec.ts \
  tests/plugins/mobile-prompt-history-plugin.spec.ts \
  tests/plugins/mobile-plugin-task-panel.spec.ts)
# 10 passed
```

### Documentation

- The whole-tree search matches 81 files. Every match is in an expected category:
  this package's own documents, the retained Host prerequisite requirement and design
  (whose REQ/AC ids contain the words) and the designs/decisions that cite them, the
  deprecated and superseded UI panel pair, historical plans, and the
  browser-conversation-facade passages in `docs/public`. The single invalidated
  statement was the facade ADR's consequence "Core prompt history remains until a
  separate plugin and extraction package prove parity and migrate saved built-in panel
  identities", now corrected to record that both landed and that saved identities are
  deliberately not migrated.
- The retained Host design already cited `apps/web/lib/turn-duration.test.ts` and the
  fixture parity suites for prompt derivation, the two plugin specs for panel states
  and end-to-end coverage, and already recorded the panel requirement and design as
  deprecated and superseded. Its requirements doc was updated to clarify the
  implementation boundary; it remains within the specification size limit.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`
  pass.
- Status promotions: the extraction system design is `current`, this plan is
  `implemented`, and the requirements doc stays `active`.

### Choices recorded

- The static renderable set lives in
  `lib/state/layout-manager/renderable-components.ts`, and
  `dockview-desktop-layout.tsx` now builds its `components` map from that list and
  exports `DESKTOP_COMPONENT_NAMES` from the map, so the registered set and the
  restore/validation predicate cannot diverge; the registry test pairs that equality
  with the direction that is not tautological (no renderable name resolves to the
  `unknownPanel` placeholder, and each legacy alias resolves to its canonical
  renderer).
- `maximizedGroupIdOf` and `serializedGridGroupIds` live in the neutral sanitizer
  module rather than in `dockview-layout-restore.ts`, because the store cannot import
  a component module that already imports the store.
- Overlay readers differ on purpose when the maximized group does not survive:
  `restoreMaximizeFromStorage` returns false so `performEnvSwitch` applies the
  environment's sanitized saved layout, while `tryRestoreMaximizeOnly` applies the
  filtered `preMaximizeLayout`, leaves `preMaximizeLayout`/`maximizedGroupId` null, and
  returns true, because that reader has no saved layout to fall back on.
- E2E adjustments made while proving the flows; none weakens an assertion:
  - The Todos panel is runtime preference-gated (`show_todo_list_panel`, default
    false), so the workbench removes a saved `todos` tab unless the preference is on.
    Both layout-profile cases enable it before opening a task.
  - The Todos panel's root test id is `todos-panel` when it has entries and
    `todos-panel-empty-state` on an empty session, so the "usable" assertion matches
    the panel surface either way.
  - The retired row/option absence checks are label-exact: the in-repo fixture plugin
    registers a panel titled "Prompt history fixture", which a substring match counts.
  - `dockview-preset-persistence.test.ts`'s existing "does not persist when legacy
    fromJSON restore throws" case now seeds a shape-healthy legacy payload so it keeps
    exercising the `fromJSON` throw path rather than the sanitizer's null path.

### Environment-switch hydration follow-up

- The first environment adoption can run before the task's session list has
  hydrated. `useEnvSwitchCleanup` now passes the list's loaded state and
  sessions already known to belong to another environment through
  `performLayoutSwitch`. Maximize overlay and pre-maximize restore preserve
  unknown sibling sessions until the list is authoritative, while filtering
  known foreign sessions.

- Authoritative session-list completion also triggers reconciliation when the
  session IDs are unchanged. The pass removes stale session panels from the
  saved pre-maximize layout and updates its environment maximize blob, while
  retaining active and listed sibling sessions.
- Restore can already filter the in-memory snapshot before the hydration effect
  runs. Reconciliation also checks the saved blob and rewrites its stale
  pre-maximize layout, so a reload while maximized does not restore the retired
  session again.
- Maximize-state persistence is deferred to the next animation frame. The
  writer reads the latest pre-maximize snapshot and releases
  `isRestoringLayout` only while the same environment remains maximized; a
  stale frame cannot re-persist panels or clear a newer environment's restore
  guard.
- Nine focused dockview suites pass: 153 tests, including unchanged-ID
  hydration, restore-time blob cleanup, deferred writes, and stale-frame
  ownership. `pnpm run typecheck`, targeted ESLint, and Prettier checks pass.
- The seven focused dockview restore suites pass: 134 tests across
  `dockview-env-switch-maximize-session-filter.test.ts`,
  `dockview-env-switch-action.test.ts`, `dockview-right-pane.test.ts`,
  `dockview-preset-persistence.test.ts`, `dockview-layout-restore.test.ts`,
  `dockview-desktop-layout.test.ts`, and `layout-profiles.test.ts`. This
  includes delayed-adoption preservation and maximize-exit persistence.
- `pnpm run typecheck`, targeted ESLint, and Prettier checks pass on the changed
  TypeScript files.
- The managed browser smoke was attempted with
  `pnpm e2e:run --project chromium
tests/layout/saved-layout-session-isolation.spec.ts`; backend compilation
  stopped before the browser launched because the worktree hit `disk quota
exceeded`.
