---
id: "01-refresh-definitions"
title: "Refresh builtin definitions and prove isolation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATION-RUNS-001.3
  - AC-OFFICE-AUTOMATION-RUNS-001.9
  - AC-OFFICE-AUTOMATION-RUNS-001.10
  - AC-OFFICE-AUTOMATION-RUNS-001.11
  - AC-OFFICE-AUTOMATION-RUNS-001.12
system_design:
  - ../../specs/office/system-design/automation-runs.md
---

# Task 01: Refresh Builtin Definitions and Prove Isolation

## Summary

Connect the builtin desktop definition list to existing periodic and foreground
refresh hooks. Preserve workspace/request isolation and saved navigation state.
Prove the correction through focused RED/GREEN tests and external-client browser
checks. Start only after the user's later explicit implementation request.

## In scope

- `AutomationsSection` refresh wiring, including folded counts.
- Mounted create/rename/delete, foreground recovery, and periodic retry coverage.
- Deferred workspace and request-order regressions through the existing hook.
- Real-provider integration for independent consumers and hidden saved nodes.
- Desktop external-client E2E and a phone catalog control.

## Out of scope

Backend changes, automation execution/configuration, shared-cache consolidation,
new events, saved-settings writes, visual redesign, translations, inactive mobile
fallback code, broad suites, delegation, and infrastructure changes.

## Acceptance

1. Permanent regressions fail on unchanged production for the recorded stale-list
   behavior, then pass after the minimum refresh wiring. They assert rendered
   create/rename/delete outcomes through real hooks while the section stays mounted.
2. Deferred requests cannot publish another workspace's rows or overwrite a newer
   result. Hidden nodes remain hidden, saved layout and expansion remain intact,
   and health refresh retains its existing gates.
3. The desktop external-client browser flow passes without reload or workspace
   navigation. The existing phone drawer updates through the catalog while open.
   All commands below complete with recorded results.

## Implementation sequence

1. Read the [reproduction](reproduction.md), owning criteria, design section, and
   `apps/web/AGENTS.md`. Apply `/tdd` and mark this work order `in_progress`.
2. Add permanent cases to the existing section suite. Prove RED for creation,
   rename, and deletion before changing production. The temporary reproduction
   used real hooks and replaced only transport results after the initial render.
3. Add the definition interval and foreground hook in `AutomationsSection`.
   Enable them while `listScope` is present and definitions are not loading.
   Retain summary refresh on `showing`.
   Keep the shared definition hook's ownership and request guards unchanged.
4. Complete deferred-response and real-provider integration cases. Add focused
   browser coverage using an external client. Use `/e2e` and `/mobile-parity`.
5. Run the exact checks. Record failures and corrections, update the plan and
   work-order status, and reconcile the owning specification lifecycle only when
   implementation conforms. Do not promote unrelated unfinished requirements.

## Regression matrix

| Boundary | Required observation |
| --- | --- |
| Expanded desktop, initially empty | External creation adds its row by the next successful interval read |
| Expanded desktop, existing row | External rename changes text on the same ID and deletion removes that row |
| Folded header | Definition count updates, no health-summary interval starts, expansion stays false |
| Rail collapse, mobile desktop branch, unmount | This section stops definition reads. Reopening resumes the existing initial read |
| Foreground recovery | Focus/visibility/online events request definitions through `useForegroundRefresh`; an obsolete workspace response remains rejected |
| Failed read then recovery | A subsequent successful interval restores current definitions without manual navigation |
| A pending, switch B, return A | No B row is rendered under A, including before effects and after late settlement |
| Same-workspace overlap | Newer rename or empty result wins, older success/failure cannot replace it |
| Two mounted consumers | Catalog and builtin update through their own hooks. A catalog refresh alone is not proof of builtin freshness |
| Hidden saved builtin | External changes do not mount it or alter `visible`, order, or expansion. Other catalog consumers may still read |
| Current phone route | Same open drawer receives the external row with a correct history link and no desktop-section polling |

Use deferred promises with distinct workspace IDs and names. Settle every promise
in cleanup. Keep transport mocks below the hooks. Use the real store/provider
and layout renderer for preference preservation. Capture rendered workspace
transitions to detect stale rows before passive effects.

## UI preview

Use [UI-01 and UI-02 in the plan](plan.md#ui-preview). The runtime instruction
prohibits ASCII previews. These are existing compositions, not layout changes.

UI-01: desktop Automations header, definition row links, Open automations.
An external creation replaces the empty state. A folded header keeps its count.
UI-02: phone navigation drawer with an Automations disclosure, existing catalog
rows, setup action, and Open automations. Retain the drawer's scroll owner.
These map to AC-001.3 and .10 through .12.

## Files likely touched

Production:

- `apps/web/components/app-sidebar/sections/automations-section.tsx`

Tests:

- `apps/web/components/app-sidebar/sections/automations-section.test.tsx`
- `apps/web/components/app-sidebar/automation-definition-refresh.integration.test.tsx` (new)
- `apps/web/components/runs/use-workspace-automations.test.ts`
- `apps/web/e2e/tests/automations-sidebar.spec.ts`
- `apps/web/e2e/tests/settings/mobile-sidebar-customization.spec.ts`

Read-only boundaries:

- `apps/web/components/runs/use-workspace-automations.ts`
- `apps/web/components/runs/use-live-refresh.ts`
- `apps/web/hooks/use-foreground-refresh.ts`
- `apps/web/hooks/domains/sidebar/use-sidebar-shortcut-catalog.ts`
- `apps/web/hooks/domains/sidebar/use-sidebar-layout-navigation.ts`
- `apps/web/components/app-sidebar/sidebar-layout-navigation.tsx`
- `apps/web/components/navigation/mobile-sidebar-layout-navigation.tsx`

Update this package and its linked requirement/design with actual results.
A production hook change beyond caller wiring needs evidence and a package update.

## Verification

Run each command from the repository root. A fresh worktree needs one frozen
install from `apps/`; this design session already installed dependencies.
Use one heavy command at a time. Retain and join each process handle.

Permanent RED, before the production change:

```bash
(cd apps/web && pnpm exec vitest run components/app-sidebar/sections/automations-section.test.tsx -t 'external definitions')
```

Name the new parameterized cases with `external definitions` so this command
collects all three. Require the expected DOM failures, not setup failures.

Focused GREEN and existing controls:

```bash
(cd apps/web && pnpm exec vitest run components/app-sidebar/sections/automations-section.test.tsx components/app-sidebar/automation-definition-refresh.integration.test.tsx components/runs/use-workspace-automations.test.ts components/runs/use-automation-summaries.test.ts components/runs/use-live-refresh.test.ts hooks/domains/sidebar/use-sidebar-shortcut-catalog.test.ts e2e/helpers/causal-waits.test.ts)
(cd apps/web && pnpm exec eslint components/app-sidebar/sections/automations-section.tsx components/app-sidebar/sections/automations-section.test.tsx components/app-sidebar/automation-definition-refresh.integration.test.tsx components/runs/use-workspace-automations.test.ts e2e/tests/automations-sidebar.spec.ts e2e/tests/settings/mobile-sidebar-customization.spec.ts e2e/helpers/causal-waits.ts e2e/helpers/causal-waits.test.ts)
(cd apps/web && pnpm run typecheck)
```

Browser commands build fresh assets and use isolated fixtures. Name each new
browser case with `external definitions`. Existing focused desktop coverage
runs with the new case. The mobile command selects only the new parity case.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/automations-sidebar.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-sidebar-customization.spec.ts -- --grep 'external definitions' --retries=0)
```

Use a separate WebSocket API client for normal create/update/delete requests,
or existing isolated API fixtures where they represent the operation faithfully.
Never invoke a mutation through the observed page's hook or store. Take a baseline
layout snapshot. Arm successful response waits before mutations and match the
expected row transition in each payload. Await that response before the default-
timeout DOM assertion. Keep a page
marker and section handle to prove there was no reload or remount. Assert the
actual builtin row, not the catalog result alone. Add a test deadline derived
from the ten-second cadence plus transport allowance. Do not use arbitrary sleeps.

Documentation gates:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

No copy changes are planned. If implementation introduces copy, update the
package scope and run the repository's i18n checks before completion.

## Dependencies

None. This work is sequential in the primary session.

## Risks

The catalog can produce successful responses while builtin rows remain stale.
Hidden-node assertions must inspect the real renderer. Fake-clock tests must
install timers before mount. Browser timing must observe completed definitions,
not summary activity. Preserve existing request generations under overlap.

## Parallelism

`sequential`

## Inputs

- [Automation runs requirements](../../specs/office/requirements/automation-runs.md)
- [Sidebar definition freshness design](../../specs/office/system-design/automation-runs.md#sidebar-definition-freshness)
- [Reproduction](reproduction.md)
- [Portable settings decision](../../decisions/0041-backend-owned-portable-user-settings.md)
- Existing section tests, catalog tests, and `automations-sidebar.spec.ts`.

## Results

Initial implementation evidence from 2026-10-23 follows. The PR review
remediation below supersedes the initial polling and browser-wait details.

- Permanent RED: eight expected failures across create/rename/delete, folded
  counts, foreground events, and failed-read retry. One lifecycle control passed.
- Focused GREEN: six files, 55 tests passed after the production patch.
- After test-only lint corrections: both changed component suites passed,
  30 tests total. Production code did not change after the first GREEN run.
- Focused ESLint passed with no errors or warnings after those corrections.
- Typecheck first exhausted Node's default heap. With an 8 GB heap, it found
  an incomplete integration-test settings fixture. The fixture now spreads the
  canonical defaults. Typecheck passed, and that suite passed all three cases
  again. Its final ESLint and Prettier checks passed.
- The first managed desktop run built all assets, but all five cases stopped
  at fixture readiness. The fixture bound to a non-loopback address. Set `KANDEV_SERVER_HOST=127.0.0.1` for subsequent test processes only.
- The next desktop run passed all four existing cases and external create/rename.
  The new deletion assertion exposed a test timing error: a catalog response can
  arrive just after the builtin interval. Corrected each DOM assertion to allow
  15 seconds from mutation completion, concurrently with the response wait.
- Desktop final run passed all five cases, retries disabled, one worker. The new
  external CRUD case took 31.3 seconds. Log: `/tmp/kandev-run.e2e.JBHyyv3h.log`.
- Mobile final run rebuilt assets and passed the selected external-creation case,
  retries disabled, one worker. The case took 12.0 seconds. Log:
  `/tmp/kandev-run.e2e.dwM4VAsN.log`.
- Each external change had a 15-second DOM deadline from mutation completion.
  All mounted-element, document, layout, link, and expansion assertions passed.
- Final ESLint and Prettier checks passed after the browser timing correction.
- Host-only test environment: Nix library directories in `LD_LIBRARY_PATH`,
  `NODE_OPTIONS=--max-old-space-size=8192`, and the loopback override above.
  Desktop retries use `--no-build` because all assets built successfully and
  subsequent changes affect only tests. No runtime or host configuration changed.
- Catalog/specification/diff gates passed after implementation.
- Owned E2E fixtures completed teardown. No live user automation or infrastructure
  was changed. No public documentation or localization change is needed.
- Plan and work order are done. The wider migrated specs remain draft because
  only the criteria listed in this work order were verified.

## PR review remediation

- Reproduce slow-read starvation with two deferred definition reads that each
  exceed the polling interval. Automatic interval and foreground refresh must
  pause while the section's current definition read is loading. Resume the
  interval after settlement. Keep the shared hook and explicit-refresh request
  ordering unchanged.
- Extend the existing causal WebSocket response wait with an optional payload
  predicate. Cover rejection of a correlated response with stale data. Desktop
  and phone creation waits must match the created name; rename and deletion
  waits must match their exact row transition before default-timeout assertions.
- RED: the slow-read test issued four requests before the first settled; the
  payload matcher accepted the stale response. Both failed for the expected
  reason before the changes.
- GREEN: seven focused suites passed all 86 tests, including the existing
  workspace, visibility, and request-order controls. Focused ESLint, web typecheck
  with an 8 GB heap, specification catalog/lint, and diff checks passed.
- The first desktop rerun passed the existing cases but the deletion wait failed:
  an empty backend slice is encoded as `null`, which the watcher previously
  normalized to `{}`. Added a failing null-payload regression, then preserved
  the raw response for the optional predicate while retaining the existing
  object-shaped payload for event consumers. All 31 helper tests passed.
- Final desktop browser run: five cases passed without retries. Log:
  `/tmp/kandev-run.e2e.yMS61cJz.log`. Final phone case: one passed without retries.
  Log: `/tmp/kandev-run.e2e.C8PDyV3O.log`. Both used one worker and existing
  isolated fixtures. Tests use default DOM assertion timeouts after the matching
  definition response.
- Desktop assets were rebuilt after the production fix. Test-only corrections
  and the phone run used `--no-build` with those same assets.
- Final combined run passed all 87 tests across seven focused suites.
  CI and review status for the pushed remediation will be verified separately.
