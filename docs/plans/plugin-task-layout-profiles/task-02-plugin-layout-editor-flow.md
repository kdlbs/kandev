---
id: "02-plugin-layout-editor-flow"
title: "Plugin panel add, save, reload, and task application"
status: done
wave: 2
depends_on:
  - 01-plugin-profile-validation
plan: "plan.md"
requirements:
  - REQ-UI-TASK-LAYOUT-PROFILES-001
  - REQ-UI-TASK-LAYOUT-PROFILES-002
acceptance_criteria:
  - AC-UI-TASK-LAYOUT-PROFILES-001.4
  - AC-UI-TASK-LAYOUT-PROFILES-001.5
  - AC-UI-TASK-LAYOUT-PROFILES-001.9
  - AC-UI-TASK-LAYOUT-PROFILES-001.10
  - AC-UI-TASK-LAYOUT-PROFILES-002.1
  - AC-UI-TASK-LAYOUT-PROFILES-002.2
  - AC-UI-TASK-LAYOUT-PROFILES-002.3
  - AC-UI-TASK-LAYOUT-PROFILES-002.5
  - AC-UI-TASK-LAYOUT-PROFILES-002.6
  - AC-UI-TASK-LAYOUT-PROFILES-002.7
  - AC-UI-TASK-LAYOUT-PROFILES-002.8
system_design:
  - ../../specs/ui/system-design/task-layout-profiles.md
  - ../../specs/ui/system-design/task-layout-plugin-panels.md
---

# Task 02: Plugin Panel Add, Save, Reload, and Task Application

## Summary

Expose registered plugin task panels in the existing Layouts Add panel menu,
using generic identities and shared title/placement helpers. Prove the complete
built-in/custom profile workflow through persisted settings and task rendering,
including actual Prompt History on a disposable instance.

## In scope

- Reactive combined built-in/plugin menu, whole-layout duplicate filtering,
  plugin-only enabled state, localization, and revocation while menu is open.
- Extend `addReusablePanel` for generic plugin identity resolution using existing
  helpers, keeping capture/dirty state and selected/fallback placement behavior.
- Retain settings placeholders and touch click-open safety; no live task effects.
- Save/reload both override and custom profiles, nested structure/proportions,
  canonical titles/params, task selection/default/reset and environment precedence.
- Axis-aware application and horizontal-only root-width policy in store
  geometry, applier, restore fixups, environment sizing, state enforcement,
  and mounted setup including target refresh, constraints and sash bookkeeping.
  Prove non-square VERTICAL-root proportions/nested widths through explicit
  apply, fresh default, Reset Layout, reload and resize, with HORIZONTAL and
  legacy compatibility.
- Editor-owned canonical title retention across mounted-editor registration
  loss, seeded from saved input or newly added registration, and localized
  display/fallback reconciliation without registry-only dirtying.
- Plugin-bearing environment switches bypass component-only fast eligibility
  when either current or target contains generic plugin panels; use existing
  saved restore/default build for exact identities, placement and selections.
- Preserve resolved horizontal reusable-default targets through slow fresh-task
  fixups, scheduled enforcement and mounted automatic refresh; scope transient
  geometry to its build/environment without fake manual-width persistence.
- Focused consumer tests, managed desktop/mobile Playwright, real-package browser
  smoke, public documentation, requested broad checks, and Conventional Commit.

## Out of scope

- New plugin API/storage, plugin repository code, retired core Prompt History,
  a plugin-specific allowlist, mobile/tablet task-layout redesign, deployment,
  primary-instance restart, and automatic reopening of revoked runtime tabs.
- Reopening completed historical layout/PR Details work orders.

## Acceptance

1. Registered plugin choices update reactively, remain selectable when no
   built-ins are missing, exclude identities in any editor split, and add once
   in the selected/fallback split. Registration lost before selection adds
   nothing. Built-in, Agent, keyboard, and separate-touch-selection behavior
   remain intact; settings mounts only generic placeholders.
2. Save/reload and application of a built-in override and a custom profile
   preserve canonical identity/params/title, group/tab ordering, selections,
   nested splits, root-axis proportions and nested widths on non-square hosts,
   and render actual plugin content in its saved task placement. Fresh default,
   Reset Layout, reload and resize retain this geometry. Vertical root sash
   edits do not overwrite horizontal width preferences. Availability changes
   preserve the reusable payload; existing live revocation remains unchanged.
   In-app switches from a different plugin-bearing task to a fresh default,
   and back to a saved environment, restore the incoming plugin identity and
   exact structure/selection rather than retaining an equal component skeleton.
   A fresh horizontal override/custom default with canonical right groups and
   an in-cap non-stock ratio retains its resolved targets after final fixups,
   rAF/enforcement and mounted refresh, without modifying manual preferences.
3. Behavior-level unit/component and desktop/mobile browser coverage pass;
   the real Prompt History smoke, exact verification results/limits, public-doc
   updates, and user-requested Conventional Commit are recorded. No live user
   settings or primary process is touched.

## ASCII UI preview

[Full UI-01/UI-02 previews and states](plan.md#ascii-ui-preview).

### UI-01: Desktop Add panel and saved placement

```text
[Profiles] [Sidebar]     Preview: [Agent | <plugin placeholder>]
                         [ + Add panel ]
                         +--------------------------+
                         | <missing built-ins>      |
                         | Prompt History           |
                         | <other plugin panels>    |
                         +--------------------------+
                                  [Save changes]
```

Show Prompt History in the menu only while not already placed; use the selected
split when adding. If plugins are the only missing panels, the trigger remains
usable. Registry removal changes choices, not saved preview placement.

### UI-02: Phone settings touch selection

```text
< Settings       Layouts
[Profiles] [Sidebar]
[profile selection/actions]
[placeholder preview]
[ + Add panel ]    [Save changes]
+------------------------------------+
| <missing built-ins>                |
| Prompt History                     |
| <other plugin panels>              |
+------------------------------------+
```

Use the shipped Radix bottom-sheet treatment below 640px, safe-area containment,
44px touch rows, and internal menu scroll. Settings owns page vertical scroll.
Opening and choosing are separate taps; no horizontal document overflow or
alternate phone catalog. Phone settings edits desktop profiles only.
These structures cover 002.1-.3/.7-.8. Wording/spacing are illustrative, not
pixel specs or new untranslated labels.

## Verification

From the repository root. Task 01 bootstraps dependencies; do not reinstall if
successful. Read `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer` before
execution. Before exported signature edits, attempt LSP references; if still
unavailable, search all imports/callers explicitly and keep the API change local.

Write focused behavior tests before production edits, run RED with the expected
consumer failure, then GREEN. Do not use a mocked addPanel argument echo as proof
of placement or duplicate handling. Use the actual registry, DOM menu behavior,
profile hook state/baseline transitions, serializer, and real Dockview where
those boundaries are the regression.

The mounted-editor title regression must open/add under pseudo/non-English
locale, revoke registration without unmounting the editor, move/reorder another
tab, then capture/save/reload while unavailable. Assert the exact canonical
title plus identity/params/tree and fallback display. Cover the symmetric
initially-unavailable profile and registry-only transitions remaining clean.
Replace the current static placeholder mapping test with this behavior-bearing
`layout-editor.test.tsx`; do not re-pin exported placeholder wiring.
Orientation coverage must save/reload/apply a VERTICAL-root profile on a
non-square host with a Files/Changes/plugin child, historical pinned flag,
unequal nested sizes, and non-default selected tabs. Assert root-axis
proportions, nested widths, tree order and plugin placement after explicit
apply, fresh default, Reset Layout, task reload and container resize. Exercise
both state and mounted enforcement and vertical sash persistence; verify
existing horizontal manual-width preferences are not overwritten. Task 01
owns serializer/normalization changes. This task owns application, fixup,
constraint, enforcement and bookkeeping guards; HORIZONTAL/legacy controls
must preserve their existing caps and manual width behavior.

Add `dockview-env-switch.plugin-layout.test.ts` consumer integration using
actual Dockview and real comparator/capture/serialization modules. Do not mock
comparator booleans as in the existing switch suite. Cover Notes -> Prompt
History on a fresh environment with an identical component skeleton, return to
a saved different-plugin environment, same plugin IDs with different
root/nested placement, tab order and selected tabs, and either-side-only plugin
cases. Assert resulting canonical IDs/params, tree/orientation, groups, selected
tabs and content, plus unchanged portable profile storage. Include a plugin-free
control. Extend desktop settings E2E with in-app task navigation (not full page
reload) and the same default-versus-saved-environment transitions.

Extend the switch regression with actual default builders, fixups, both
enforcement boundaries and mounted automatic refresh (not mocked fixups).
On a measured 1200px horizontal Dockview, use saved center/right 960/240,
canonical Files/Changes/plugin right groups, and no manual width. After the
slow fresh-default switch and scheduled rAF work, assert measured right extent
240 rather than stock 360 and the retained center proportion. Cover override
and custom defaults, stock preset/manual override/saved-environment controls,
and unchanged portable/manual-width storage. Add the settled geometry assertion
to the in-app desktop Notes -> Prompt History transition.

Carry the reusable default's resolved targets into final fixups and existing
in-memory enforcement/refresh state. Actual manual width wins; otherwise the
capped reusable target wins over stock automatic geometry. Keep global sidebar,
saved-environment and code-preset policies; replace/clear transient source
ownership on environment/build changes. Do not infer a manual preference from
the profile or rely only on the pre-fixup resolved map as proof.

Final requested gates start with formatting, after RED/GREEN implementation:

```bash
make fmt
(cd apps/web && pnpm exec vitest run lib/layout/layout-profiles.test.ts lib/layout/layout-profiles.plugin-panels.test.ts lib/state/layout-manager/plugin-panels.test.ts lib/state/layout-manager/session-panels.test.ts lib/state/layout-manager/sizing.test.ts lib/state/layout-manager/serializer.test.ts lib/state/layout-manager/applier.test.ts lib/state/dockview-store.test.ts lib/state/dockview-layout-builders-fixups.test.ts lib/state/dockview-pinned-enforce.test.ts components/task/dockview-layout-setup.orientation.test.ts components/task/dockview-layout-setup.measurement.test.ts components/settings/layouts/layout-editor-actions.test.ts components/settings/layouts/layout-editor-toolbar.test.tsx components/settings/layouts/layout-editor.test.tsx components/settings/layouts/use-layout-settings.test.tsx components/task/use-close-revoked-plugin-panels.test.ts components/task/plugin-task-panel.test.tsx)
(cd apps/web && pnpm exec vitest run lib/state/dockview-env-switch.plugin-layout.test.ts lib/state/dockview-env-switch.test.ts lib/state/dockview-env-switch-pinned.test.ts lib/state/dockview-env-switch-right-width.test.ts)
make typecheck test lint
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/layout-profiles.spec.ts tests/plugins/plugin-task-panel.spec.ts tests/task/task-default-layout.spec.ts tests/layout/saved-layout-session-isolation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-layout-profiles.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run sequentially; managed E2E rebuilds production Vite/backend/fixture artifacts
and enforces one worker per shard. Confirm the intended specs are discovered.
Run any additional changed permanent suite explicitly and add its exact command
to Results. If requested formatting changes unrelated files, preserve the user's
changes and avoid staging unrelated output.

### Required actual browser smoke

Use [the plan's Real Prompt History protocol](plan.md#real-prompt-history-smoke).
The package must be real, not the in-tree prompt-history fixture. Acquire it
from the separate repository or a verified release; inspect its documented
package workflow if packaging is necessary, without plugin source edits.
Record package version/checksum, host revision, isolated ports/state paths,
browser actions, before/after saved payload/group geometry, actual seeded
prompt rendered in saved placement, screenshots, and teardown.

A temporary `tests/settings/plugin-layout-real-package-smoke.spec.ts` may reuse
`test-base` disposable worker and the normal upload UI. Run it with a fresh
production build:

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/settings/plugin-layout-real-package-smoke.spec.ts)
```

That path is temporary and is created only for this execution proof; remove it
after the run. If using an isolated interactive browser instead, record the
exact launch command and observed scenario, not an unexecuted planned command.
Never point at the primary instance/database, change live settings, or restart
it. Verify a phone viewport of the settings menu as well as the desktop task
panel; report genuine limits rather than treating the fixture as substitute
real-package proof.

### Documentation coverage and delivery

After actual smoke, update `docs/public/sessions-and-review.md` (existing how-to)
with registered plugin discovery, placeholders, save/reload/apply, and portable
placement surviving unavailable plugins. Avoid duplicate instructions; link from
`docs/public/plugins.md` only if useful. Leave generated historical CHANGELOG.md
unchanged; the authorized `feat:` Conventional Commit feeds its release entry.

Run the exported `validateCoverage` in `.github/scripts/pr-docs.cjs` against the
changed work orders and their plan, requirements, and design file contents;
record `ok: true` and accepted references. The design-phase preflight is not a
replacement for checking final implementation references.

Keep REQ-UI-TASK-LAYOUT-PROFILES-001 active. Record acceptance of the proposed
002 extension separately from execution status; passing checks alone does not
establish contract acceptance. Update this work order and plan with exact
results and actual smoke evidence. After both work orders pass, verify that
the designs match the accepted contract and exercised behavior before making
them current, and set the plan to implemented. Preserve old completed
companion-plan results as history. Use `/commit` and explicit `git add` paths
for the authorized commit. Do not push, create a PR, deploy, or call
step_complete; the user moves the task to Test.

## Files likely touched

- `apps/web/components/settings/layouts/layout-editor-toolbar.tsx`
- `apps/web/components/settings/layouts/layout-editor-actions.ts`
- `apps/web/components/settings/layouts/layout-editor-actions.test.ts`
- New `apps/web/components/settings/layouts/layout-editor-toolbar.test.tsx`
- `apps/web/components/settings/layouts/layout-editor.tsx`
- Replace `apps/web/components/settings/layouts/layout-editor.test.ts` with
  `apps/web/components/settings/layouts/layout-editor.test.tsx`.
- `apps/web/components/settings/layouts/use-layout-settings.test.tsx`
- `apps/web/lib/state/layout-manager/serializer.test.ts`
- `apps/web/lib/state/layout-manager/applier.ts` and `.test.ts`
- `apps/web/lib/state/dockview-store.ts` and `.test.ts`: every custom/default
  geometry resolver caller retains orientation instead of passing columns alone.
- `apps/web/lib/state/dockview-layout-builders.ts` and
  `dockview-layout-builders-fixups.test.ts`
  including resolved reusable-default targets at the final fresh-task fixup.
- `apps/web/lib/state/dockview-pinned-enforce.ts` and `.test.ts`
- `apps/web/components/task/dockview-layout-setup.ts` and
  `dockview-layout-setup.measurement.test.ts`; new
  `dockview-layout-setup.orientation.test.ts` for actual geometry/width-state
  transitions, not mocked call forwarding.
- `apps/web/lib/state/dockview-env-switch.ts`, `dockview-env-switch.test.ts`,
  `dockview-env-switch-pinned.test.ts`, `dockview-env-switch-right-width.test.ts`,
  and new `dockview-env-switch.plugin-layout.test.ts`. The new suite exercises
  real `layout-manager/comparator.ts`; comparator source remains unchanged.
- `apps/web/e2e/tests/settings/layout-profiles.spec.ts`
- `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts`
- `apps/web/e2e/pages/layout-settings-page.ts` if stable selectors need a helper.
- `apps/web/e2e/fixtures/test-base.ts` only if existing baseline cleanup is
  insufficient; preserve all existing reset semantics.
- `docs/public/sessions-and-review.md`, optionally `docs/public/plugins.md`.
- Paired requirement/design files, this work order, and `plan.md` for final status.

Canonical-title retention, orientation-safe geometry, plugin-bearing switch
eligibility, and horizontal fresh-default target preservation are required
scope, not conditional runtime follow-ups.
Keep save-hook and plugin-lifecycle source unchanged unless exercised behavior
proves a local acceptance fix is necessary.
No translation keys are expected: reuse existing host and plugin title helpers;
if new user-facing copy is required, add all required locale translations.

## Dependencies

Task 01 must pass before new plugin drafts can be accepted by the save/default
paths. This task owns the complete end-to-end delivery, not a menu-only patch.

## Risks

- An open menu can outlive registration; re-resolve at selection time.
- Registry updates can silently mutate a saved draft if preview reconciliation
  reuses live task cleanup. Keep those lifecycles separate.
- The capture helper falls back to localized Dockview titles when registration
  disappears. Retain canonical titles separately and test revocation in the
  still-mounted editor before another layout mutation; reload-only coverage
  cannot expose this ordering.
- The settings preview must never mount plugin task/session Components.
- Managed browser fixtures are worker-scoped: restore captured settings and
  plugin state in failure cleanup, not only successful assertions.
- Real package acquisition is required; report exactly what was tried if absent.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/task-layout-profiles.md).
- [Plugin extension design](../../specs/ui/system-design/task-layout-plugin-panels.md).
- [Baseline design](../../specs/ui/system-design/task-layout-profiles.md).
- Existing task plugin menu, generic identity/title helpers, registry subscription,
  settings preview/capture/save, fixture installer, LayoutSettingsPage, and
  existing desktop/mobile layout-profile and plugin lifecycle specs.
- `/home/clem/git/kandev-plugin-prompt-history` is a separate plugin source checkout,
  not a host code reference or a production-plugin destination in the monorepo.
- Reported installed-binary failure is accepted ground truth. Do not use the old
  `/home/clem/git/kandev` checkout or restore its retired built-in panel.

## Results

Executed 2026-10-18 after Task 01, TDD per behavior. Each RED below was run
before the production change and failed on the named assertion.

### Behavior RED/GREEN

- **Add panel (actions):** `layout-editor-actions.test.ts` (real Dockview)
  failed with `expected {} to deeply equal { pluginId, panelKey }` and
  `expected true to be false` for a revoked or unknown identity. GREEN:
  `addReusablePanel` resolves a built-in reusable definition or
  `resolvePluginPanelDefinition` at selection time, keeping duplicate prevention,
  selected/fallback group placement and the boolean result.
- **Add panel (menu):** new `layout-editor-toolbar.test.tsx`: five new cases
  failed (no plugin `menuitem`, trigger disabled with only plugin choices). GREEN:
  `useAddPanelChoices` combines missing built-ins with `getTaskPanels()`,
  canonical IDs via `pluginPanelId`, titles via `resolveTaskPanelTitle`, filtered
  against every placed identity, subscribed through `usePluginRegistry`. A touch
  tap that only opens the menu adds nothing (control kept).
- **Canonical titles:** new `layout-editor.test.tsx` (replaces the static
  `layout-editor.test.ts`; a mounted test now proves a placeholder per reusable
  panel). Under `ja`, a revoked registration saved `"プロンプト履歴"` in both the
  tree leaf and the flat projection and kept the localized tab. GREEN:
  `layout-editor-titles.ts` (editor-owned canonical map seeded from the input draft,
  refreshed from live registrations, pruned on removal, restored into tree leaves
  and flat groups on capture) plus `useDisplayTitleReconciliation`, which re-resolves
  tab titles under the capture guard so registry/locale changes never call
  `onChange`. Initially-unavailable ordering and registry-only transitions are
  covered symmetrically. `use-layout-settings.test.tsx` gained plugin dirty,
  save-baseline, failed-save and fresh-hydration cases; they pass against Task 01
  behavior (test-only contract coverage, no production change).
- **Orientation guards:** RED then GREEN, in a real Dockview, for: `applyLayout`
  (`[450, 250]` instead of rows `[300, 400]` with pinned targets/caps);
  `resolveCustomLayoutPinnedWidths` and `collectPinnedWidthUpdates`
  (VERTICAL returned a right width); `enforcePinnedTargets` (`resizeView` called twice
  on a VERTICAL root); and `dockview-layout-setup.orientation.test.ts` (rows
  forced to `[450, 250]`, a vertical sash stored manual right width 380, tracked width
  recorded, window resize created target 420). GREEN: `getRootOrientation`;
  guards in `applyLayout`, `applyLayoutFixups`, `enforcePinnedTargets`,
  `hasPinnedRightColumn` (setup enforcement, loose constraints, sash
  classification, drag-end persistence, tracking, automatic refresh) and
  fast-path `applyPinnedColumnSizes`. VERTICAL builds and fixups clear stale
  horizontal targets. HORIZONTAL and omitted-orientation controls kept.
- **Environment switches:** new `dockview-env-switch.plugin-layout.test.ts` (real
  Dockview, store, builders, fixups, comparator, capture, serialization). RED:
  6 failures (Notes kept instead of Prompt History, saved-different-structure not
  restored, right extent `[840, 360]` not `[960, 240]`, VERTICAL orientation kept
  horizontal). GREEN: `canUseFastEnvSwitch` rejects either-side generic plugin
  panels structurally; `setReusableRightTarget`/`resolveEnvRightWidth`
  (`pinned-targets.ts`, `right-width.ts`) carry the build's resolved ratio through final
  fixups, scheduled enforcement and the mounted refresh, scoped to one environment
  and released on environment switch, preset apply, or a VERTICAL root.
  Manual width, stock default, saved-environment and plugin-free fast-path controls
  are asserted. Mutation checks failed as expected when the fast-path guard or the
  refresh resolver was reverted. `dockview-layout-builders-fixups.test.ts` and
  `dockview-pinned-enforce.test.ts` gained owner/manual/VERTICAL cases written
  after that GREEN (unit-level cover for already-failing integration behavior).
- A `__KANDEV_E2E_PLUGIN_HOST__` hook (`lib/plugins/boot.ts`, installed only when the
  existing `__KANDEV_E2E_EXPOSE_STORE__` flag is set) lets E2E unload a plugin without
  leaving the page. No other route unmounts the Layouts editor while revoking, so this is
  the only way to drive the mounted-revocation ordering in a browser.

### Commands and results

- `make fmt`: completed; no unrelated file changed.
- Task 02 focused set (18 files, `lib/layout/layout-profiles.test.ts` ...
  `components/task/plugin-task-panel.test.tsx`, plus the new
  `layout-profiles.plugin-panels.test.ts`): 18 files, 232 tests passed after the final
  production edit. Env-switch set (`dockview-env-switch.plugin-layout`, `.test`,
  `-pinned`, `-right-width`) plus `scripts/vitest-project-selection.test.ts`: 5 files,
  53 tests passed.
- `NODE_OPTIONS=--max-old-space-size=10240 make typecheck` and `tsc --noEmit`: clean
  (the default 4 GB heap runs out of memory on this tree).
- `make lint`: backend `golangci-lint` 0 issues (run with `GOCACHE` and
  `GOLANGCI_LINT_CACHE` under `/tmp` because the shared Go cache volume was full);
  web ESLint `--max-warnings 0` clean; harness lint passes after folding the AGENTS
  note into an existing bullet (apps/web/AGENTS.md stays at 300 lines); spec lint and
  architecture lint pass.
- `pnpm run i18n:check`: passed (no new keys; every user-facing string reuses an
  existing catalog entry). It ran on Node 22.
- `make test`: not green in this environment. The backend part produced
  `[build failed]` for about 55 packages while the shared Go cache volume and `/tmp`
  were full, plus an `internal/common/config` test that reads this host's
  `KANDEV` port variable (`port = 38429`); `internal/delivery` re-run alone passed.
  The backend is untouched by this change. The full web run was 26,902 passed and
  32 failed in 17 files: one genuine regression (the reviewed-project list still named
  the deleted `layout-editor.test.ts`; `scripts/vitest-reviewed-projects.json` fixed and
  `vitest-project-selection.test.ts` passes); load timeouts that pass alone
  (7 browser files, 127 tests; 5 `scripts/lib` files); and environment gaps unrelated to
  this change (no Docker bridge for `http-git-server`, a changed Git message in
  `seed-repository-origin`, no `unzip` for `native-catalog`, `node:sqlite` missing on
  Node 22 for `multi-repo-managed-clone-recovery`). `make test-cli` passed; `make test-scripts` stopped in `scripts/pr-state` because
  `unzip` is not installed.
- `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`
  (48 pages), `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`:
  passed.

### Browser

- `pnpm e2e:run --project chromium tests/settings/layout-profiles.spec.ts
  tests/plugins/plugin-task-panel.spec.ts tests/task/task-default-layout.spec.ts
  tests/layout/saved-layout-session-isolation.spec.ts` (managed rebuild, one worker):
  24 passed. `pnpm e2e:run --no-build --project mobile-chrome
  tests/settings/mobile-layout-profiles.spec.ts` (artifacts from the preceding build):
  6 passed. Playwright discovery lists 13 desktop and 6 mobile layout-profile specs
  with zero discovery errors.
- Added desktop cases: add to the selected split and save a built-in override,
  reload, then apply to a task and see fixture content in the saved group; plugin-only
  Add panel with occupied-identity exclusion; disable/enable keeps the payload and
  editing; mounted-editor revocation under the pseudo locale then reload unavailable
  (canonical title in the saved payload); a VERTICAL custom profile through save,
  fresh default, Reset Layout, reload, container resize and a vertical sash drag with
  no manual right-width key; and an in-app Notes -> Prompt History switch and return
  with the saved in-cap 20% right column and an actual manual width winning. Added
  mobile case: tap opens the menu with no insertion, a 44px row, a separate tap adds,
  save/reload, no horizontal overflow.

### Real Prompt History package smoke

Temporary spec `tests/settings/plugin-layout-real-package-smoke.spec.ts` (removed after
the run) against the managed worker's disposable backend.

- Package: `kandev-plugin-prompt-history-1.4.1.tar.gz`, SHA-256
  `12f0ba1266f70074e69ef21c00cc0cae6bd02823c1c981cf864e9dc939b77c5d` (10,844,322
  bytes), built with `make package-host` from a copy of `/home/clem/git/kandev-plugin-prompt-history`
  at `6a1beac` (Release 1.4.1), linux/amd64 only, packed by this worktree's
  `plugin-pack` via a symlinked `../kdlbs-kandev`. Plugin source was not edited.
- Host: this worktree at `0294918f70` plus uncommitted changes (46 files), E2E backend
  binary SHA-256 `affd37b3...`, ports 18094-18109 (varied per run) with data under
  `/tmp/kandev-e2e-0-*`, never the user's `~/.kandev`; saved layouts verified empty
  before any write.
- Exercised: Add panel offered Prompt History; adding it into the Files group, a
  placeholder-only preview (`ph-plugin-scroll` absent), duplicate exclusion, floating
  Save appearing then clearing; built-in override save/reload (`files, changes,
  prompt-history`, canonical title and params); a duplicated custom profile with the
  plugin split below Files/Changes (nested `tree`, own group, saved and reloaded);
  a managed task with the distinct prompt `REAL-PH-SMOKE-ALPHA distinctive prompt`
  rendered by the real panel (`ph-plugin-row-1`) in the override group, after reload,
  and in the custom profile's own group after selecting it and after reload; disabling
  the package (choice gone, payload byte-identical, tab and title kept) then re-enabling
  and Reset Layout restoring placement and content; a 393 x 851 phone settings menu
  listing Prompt History (`04-phone-settings-menu.png`). Desktop and phone screenshots and
  `evidence.json` are in `/tmp/ph-smoke/shots`.
- Not covered by the real package: mounted-editor revocation (the fixture covers it, see
  above).
- Teardown: the package row was deleted, the worker backend stopped by the runner,
  and the copied plugin tree, private Go caches and temporary spec removed.

### Public documentation

`docs/public/sessions-and-review.md` describes plugin panels in Add panel (placeholder
preview, save/reload/apply, availability versus portable placement, existing-task
precedence, phone behavior). `docs/public/plugins.md` is unchanged (avoids duplicate
instructions); `CHANGELOG.md` is generated. `validateCoverage` against the final
documents: `ok: true`, `status: covered`, no errors; accepted references:
`task-layout-profiles.md` for REQ-001 and `task-layout-plugin-panels.md` for REQ-002.

### Contract status

REQ-UI-TASK-LAYOUT-PROFILES-001 stays active. REQ-002 is implemented and verified here
but its acceptance by the contract owner is a separate decision, so the draft statuses
of the requirement and both designs are left unchanged. Implementation commit: `af1485a39f`
(`feat(ui): add plugin task panels to reusable layout profiles`). Not pushed.

### Adversarial review remediation

Round 1 (Opus 5.5 reviewer, read-only) reported two findings, both accepted:

- P2: the plugin-free fast path ignored root orientation, so a VERTICAL default or
  saved environment was not applied after in-app navigation from a horizontal task,
  and a VERTICAL task leaked into a horizontal fresh environment. `canUseFastEnvSwitch`
  now rejects reuse when the live and target root orientations differ.
- P3: a plugin-free fresh environment reverted a custom default's right ratio to the
  stock ratio while the plugin path kept it. The fast path now owns the default's
  resolved right target for the new environment and passes that owner to the final
  fixups.
- RED: four new real-Dockview cases in `dockview-env-switch.plugin-layout.test.ts`
  (`expected 'HORIZONTAL' to be 'VERTICAL'` x2, `expected 'VERTICAL' to be
  'HORIZONTAL'`, `expected [840, 360] to deeply equal [960, 240]`). GREEN after the
  change; the mocked `dockview-env-switch-pinned.test.ts` factory gained the new export.
- Final focused run: `pnpm exec vitest run lib/state/dockview-env-switch
  lib/state/dockview-store.test.ts lib/state/dockview-layout-builders-fixups.test.ts
  lib/state/layout-manager components/task/dockview-layout-setup`: 23 files,
  244 tests passed. Changed-file ESLint, Prettier and web typecheck passed
  (`NODE_OPTIONS=--max-old-space-size=10240 pnpm run typecheck`).
- Browser verification rebuilt the backend, Vite bundle and fixture with
  `TMPDIR=/home/clem/.kandev/agent-scratch/plugin-layout-review/tmp` and
  `GOCACHE=/home/clem/.kandev/agent-scratch/plugin-layout-review/go-cache` after
  cache-volume exhaustion and a subsequent `/tmp` quota failure. Command:
  `pnpm e2e:run --host --project chromium tests/settings/layout-profiles.spec.ts
  tests/task/task-default-layout.spec.ts
  tests/layout/saved-layout-session-isolation.spec.ts`. Result: 16 passed,
  2 flaky (passed on retry), no terminal failures. Flakes were backend readiness
  timeout and API `503 persistence_unavailable`, not layout assertions.
- Document catalog, full specification lint and whitespace checks passed.
