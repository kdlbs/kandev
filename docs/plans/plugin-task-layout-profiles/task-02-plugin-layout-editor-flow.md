---
id: "02-plugin-layout-editor-flow"
title: "Plugin panel add, save, reload, and task application"
status: pending
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
(cd apps/web && pnpm exec vitest run lib/layout/layout-profiles.test.ts lib/state/layout-manager/plugin-panels.test.ts lib/state/layout-manager/session-panels.test.ts lib/state/layout-manager/sizing.test.ts lib/state/layout-manager/serializer.test.ts lib/state/layout-manager/applier.test.ts lib/state/dockview-store.test.ts lib/state/dockview-layout-builders-fixups.test.ts lib/state/dockview-pinned-enforce.test.ts components/task/dockview-layout-setup.orientation.test.ts components/task/dockview-layout-setup.measurement.test.ts components/settings/layouts/layout-editor-actions.test.ts components/settings/layouts/layout-editor-toolbar.test.tsx components/settings/layouts/layout-editor.test.tsx components/settings/layouts/use-layout-settings.test.tsx components/task/use-close-revoked-plugin-panels.test.ts components/task/plugin-task-panel.test.tsx)
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

Pending. Record exact RED/GREEN, final gate results, desktop/mobile discovery and
pass counts, real-package smoke/screenshots/teardown, public-doc changes, and
commit SHA. No runtime or implementation verification has run during design.
