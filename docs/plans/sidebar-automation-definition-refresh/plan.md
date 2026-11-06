---
created: 2026-10-23
status: done
requirements:
  - REQ-OFFICE-AUTOMATION-RUNS-001
system_design:
  - ../../specs/office/system-design/automation-runs.md
legacy_specs: []
---

# Implementation Plan: Refresh Sidebar Automation Definitions

## Overview

Keep the mounted builtin Automations section current after external definition
changes. One sequential work order adds the missing refresh wiring and focused
regressions. The user authorized implementation with a later explicit
continuation request.

Office owns this correction because its automation definitions supply these
navigation rows. The requirement already owns that surface. Add AC-001.10
through AC-001.12 rather than creating a parallel UI requirement.
Clarify AC-001.3: the folded header already fetches definitions for its count,
while health summaries require expanded rows. Preserve that distinction.

The user's accepted scope resolves the material choices: external CRUD,
bounded freshness, workspace isolation, preserved settings, and phone parity.
Use the established ten-second interval. No material product question remains.

## Confirmed cause

The builtin section and shortcut catalog use independent definition hooks.
Only the catalog refreshes definitions periodically. The builtin refreshes
summaries and retains its original definition snapshot.

[Reproduction](reproduction.md) records three failing component cases and an
isolated browser failure. After external creation, a response contained the new
ID, but the same desktop section still had zero rows at 42.7 seconds.
Reload restored the row. The phone catalog path received a later external
creation while its drawer remained open.

Current main `c15867d270e3cddcae5334262ba5b31a6ef9ecce` has identical relevant
source. Earlier automation-list-publication work explicitly excluded sidebar
freshness. The implemented automation-sidebar-running-indicator package covered
health refresh, not definition refresh. Neither authorizes this new patch.

## Scope

### In scope

- Periodic and foreground definition reads owned by `AutomationsSection`.
- Folded header count freshness, expanded rows, and existing health refresh gates.
- External creation, rename, deletion, stale-response and workspace protection.
- Hidden-section and saved preference controls, plus the current phone path.
- Permanent focused component/hook tests and isolated browser regressions.

### Out of scope

- Shared cache consolidation, new backend events, API or database changes.
- Automation configuration, scheduler, hourly monitor, Telegram, or infrastructure.
- UI layout, styling, copy, navigation destinations, and settings migrations.
- Inactive mobile fallback repair, broad QA, unrelated verification, or delegation.

## Technical approach

Follow [Sidebar definition freshness](../../specs/office/system-design/automation-runs.md#sidebar-definition-freshness).
Destructure the definition refresh separately from the summary refresh.
Register its existing live interval while `listScope` is present and definitions
are not loading. Use the same gate and scope for the foreground hook. Keep summary polling on `showing`.

Keep `useWorkspaceAutomations` request generations and render-time workspace
mask. Do not add a timer to the hook or consume the flat settings store.
Hidden builtins remain excluded by the saved-layout renderer. No refresh may
write preferences. Existing catalog reads remain independent.

No new architecture boundary or ADR is required. ADR 0041 and the saved-sidebar
contract retain preference ownership. The public-docs assessment found no new
labels, actions, API, or user instructions. Internal records suffice.

## UI preview

This is a content-freshness correction inside the existing composition. The
user's runtime instructions prohibit ASCII previews. Use these named structural
views and the recorded browser evidence instead.

**UI-01, desktop, expanded:** Automations header, automation row links, then
Open automations. Before the fix, external creation leaves the setup empty state.
After the fix, the new row replaces that empty state on the next successful
refresh. Rename changes the existing row text. Deletion removes its row.
The section stays expanded. A folded header retains its count position.

**UI-02, phone, open drawer:** workspace picker and primary navigation, followed
by the Automations disclosure. Its existing row links precede setup and Open
automations actions. The phone uses catalog refresh and retains one drawer
scroll owner. No geometry, focus, safe-area, or target-size change is planned.

These views map to AC-001.3 and .10 through .12. Existing localized labels and
style tokens remain authoritative. Better-ui review requires no polish changes
for this data correction.

## Tests

- `automations-section.test.tsx`: real hooks and transport mocks prove mounted
  create/rename/delete, folded counts, rail/mobile gating, and foreground recovery.
- `use-workspace-automations.test.ts`: deferred A/B/A and same-workspace overlap
  prove newest-result authority, including newer empty and obsolete failures.
- `automation-definition-refresh.integration.test.tsx`: real provider, catalog,
  builtin section, and saved-layout renderer. Mock transport and unrelated domain
  adapters only. Prove the two consumers independently update, hidden stays
  hidden, preferences stay unchanged, and workspace transitions show no foreign
  row. Do not mock the definition, live, or foreground hooks.
- Existing live-refresh, summaries, and catalog suites remain focused controls.

## E2E tests

Extend `tests/automations-sidebar.spec.ts` on project `chromium`. Create through a
separate client while the visible section remains mounted. Rename and delete the
same ID through that client, checking each transition. Keep a document marker,
section element identity, URL, expansion, and saved layout snapshot.

Add one focused `external definitions` case to
`tests/settings/mobile-sidebar-customization.spec.ts` on `mobile-chrome`.
Keep the existing drawer open during creation and verify the new row's history
link. This protects the existing phone path without adding another poller.

Arm `watchWs` before initial navigation and each external mutation. Match the
expected creation, rename, or deletion in the response payload before asserting
the row with its default timeout. Correlated responses with stale data must not
complete the causal wait. A timer deadline is a failure bound, not a success signal.
Do not reload, switch workspaces, remount, or mutate the observed browser's store
to make an external-change assertion pass. Run only the focused cases and files.

## Work orders

- [x] [Task 01: Refresh builtin definitions and prove isolation](task-01-refresh-definitions.md)

One work order. No dependencies. Sequential execution in the primary session.

## Verification results

The initial evidence below is historical. PR review remediation and its current
verification are recorded in Task 01.

Diagnosis: three expected component failures and isolated desktop creation
failure. Phone creation control passed. Details are in [reproduction](reproduction.md).
Implementation completed on 2026-10-23 after the user's continuation request.
The production diff adds periodic and foreground definition refresh in
`AutomationsSection`. Shared hooks, saved preferences, and mobile composition
remain unchanged.

- Permanent RED: eight expected failures before the production patch.
- Focused GREEN: six suites, 55 tests passed. The final settings-fixture correction
  passed all three integration cases again.
- Focused ESLint and Prettier checks passed. Web typecheck passed with an 8 GB
  Node heap after correcting the integration fixture to use canonical defaults.
- Isolated desktop browser run: five tests passed with retries disabled. The new
  external create/rename/delete case took 31.3 seconds and retained the section
  element, document, URL, expansion, and saved layout throughout.
- Isolated phone browser run: one selected test passed with retries disabled.
  The external creation case took 12.0 seconds and retained the open drawer,
  section element, and document. Both projects enforced one worker.
- Each changed browser row had a 15-second deadline from mutation completion:
  the ten-second cadence plus transport/rendering allowance. These case durations
  include setup and assertions; they are not first-appearance measurements.
- Final documentation catalog, specification lint, and diff checks passed.

See [Task 01 results](task-01-refresh-definitions.md#results) for environment and
test-setup corrections, exact commands, and logs.

Design validation passed on 2026-10-23:

- `python3 scripts/list-docs.py validate`: 384 decisions and 1,616 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- PR documentation `validateCoverage` with the proposed production path and
  complete document package: `covered`, no errors.
- Package file links, `git diff --check`, and new-work-order status checks passed.

Task 01 is done. Implementation and verification are complete. The broad automation-runs
requirement and design documents retain their existing draft status because this
work verifies only AC-001.3 and .9 through .12, not every migrated behavior.

## Risks

- Catalog network traffic can look healthy while the builtin remains stale.
  Assert builtin DOM rows rather than request counts alone.
- Adding refreshes inside the shared hook would duplicate catalog polling.
- A hidden builtin does not imply that all other catalog consumers stop reading.
- Browser background throttling and transport failure prevent a strict wall-clock
  delivery guarantee. Foreground recovery and successful reads define the bound.
- Slow or reordered responses must retain the hook's existing workspace and
  newest-request guards. Do not broaden this repair into cache ownership.

## PR review remediation

The initial browser timing strategy above is superseded by payload-matched
causal response waits. A new deferred-read regression also exposed polling
starvation: a new automatic read could replace each slow response before it
published. The section now pauses automatic refresh while its hook is loading,
then restarts the interval after settlement. The shared request-order and
workspace guards remain unchanged. Follow-up verification is recorded in
[Task 01](task-01-refresh-definitions.md#pr-review-remediation).
