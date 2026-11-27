---
id: "01-preserve-save-drafts"
title: "Preserve Office save drafts"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-CONFIG-SYNC-006
acceptance_criteria:
  - AC-OFFICE-CONFIG-SYNC-006.1
  - AC-OFFICE-CONFIG-SYNC-006.2
system_design:
  - ../../specs/office/system-design/config-sync-surfaces.md
---

# Task 01: Preserve Office save drafts

## Summary

Correct the Office form's save acknowledgment under the existing shared
[AC-UI-SETTINGS-MANUAL-SAVE-001.4](../../specs/ui/requirements/settings-manual-save.md)
contract. Advance the acknowledged baseline on success while retaining any
current full draft that differs from the submitted full snapshot. Prove the
actual rendered form/coordinator behavior and subsequent Save/Reset.

## In scope

- `handleSave` and immediately related `useOfficeConfigSyncForm` glue in the
  existing hook; no changes to other action/reset callers.
- Independently authored rendered coverage in the new test file named below.
- Exact behavioral matrix in [the plan](plan.md#tests), including enabled real
  inputs, full-snapshot equality, scalar/provider fields, baseline advancement,
  normalization, failure, next Save, Reset, and save-and-leave.
- Final lifecycle/results updates to this plan/design package after checks pass.

## Out of scope

- Shared save/navigation engine or policy, API/backend/schema/payload/layout/
  copy, Sync Now/Delete, initial load/poller/lifetime ownership, global revision
  tracking, or shared test infrastructure. Causal need outside this boundary
  requires ROOT direction before changes.
- Discovery-proof replay/copy/import/edit/chmod/release, native delegation,
  new persistent tasks/sessions/tabs, browser/build/full local suites.

## Acceptance

1. The first independently authored causal test fails before production changes
   on lost live input and real coordinator dirty state. On success, the hook
   always advances baseline and replaces editable state only when current and
   submitted full raw snapshots match, with queued edits considered.
2. The real rendered form preserves scalar/provider edits and existing provider
   clearing/validation/payload rules. Normalized unchanged success becomes clean;
   rejected saves preserve previous baseline and draft. Later explicit Save and
   Reset use the correct retained draft/acknowledged baseline.
3. With the real providers/coordinator/guard, newer edits keep the contributor
   dirty and save-and-leave cannot proceed; unchanged normalized save can
   proceed. All listed targeted checks actually terminate successfully, with
   original receipts preserved and package statuses/results synchronized.

## Desktop and phone

Pure state/data correction inside the shipped form; no rendered composition,
touch, scrolling, focus, navigation, or breakpoint changes. Follow
[the assessment](plan.md#desktop-and-phone-assessment) and the mobile-parity
state/data exception. Both viewports use the same logic; no new browser proof,
build, E2E test, or layout preview is required.

## Implementation sequence

1. After a later explicit ROOT reviewed-package implementation INTERRUPT, read
   this packet and TDD skill, recheck HEAD/owned hook blob, mark only this work
   order `in_progress`, and obtain the exclusive ROOT heavy lease before running
   any product command. Preserve managed worktree/deps/foreign processes/caches.
2. Write the new test independently. Render real `OfficeConfigSyncSection`,
   `StateProvider` (which invokes real `createAppStore`), `SettingsSaveProvider`,
   and real required Tooltip/Toast contexts. Observe coordinator state through
   its real public hook. Only external transports may be mocked; API module,
   hook, state providers, inputs, tabs, status card, coordinator, and navigation
   guard remain real.
3. Hold a POST response; assert exact submitted body and enabled Directory;
   edit Directory to `newer-unsaved`; settle the older ACK; assert both input
   retention and real dirty/leave state. Run the RED command and record actual
   failing assertions. Controls alone are not the required failure proof.
4. Add the bounded local acknowledgment helper and use it only on save success.
   Capture full raw snapshot before await; preserve payload rules; `setConfig`
   always advances; pure functional form update conditionally normalizes.
5. Complete the behavioral matrix. Drive actual inputs/buttons/provider tabs.
   Cover target fields, branch/path/interval/poll-enabled; both provider switches
   and cleared opposite fields; next Save body/clean transition; Reset without
   POST to the latest full saved baseline; failure/retry; an edit back to the old
   baseline; and normalized unchanged success. Keep newer form whole when any
   field changes, including a case where raw and canonical values differ.
6. For save-and-leave, call real `requestNavigation` with an observed proceed
   callback, use actual Save and leave action, and hold its POST while editing
   real enabled inputs. Assert no proceed after old ACK/newer edit and observe
   real `canLeave=false` in the coordinator flow. Include unchanged normalized
   positive control. Do not synthesize a contributor or fake guard result.
7. Run GREEN and targeted lint sequentially; then docs/diff gates. Record every
   actual command/terminal result, counts and failure reasons. Promote the new
   design from draft to current only after conformance is proved; existing
   requirement status is not delivery status. Mark task done/plan implemented
   only when required implementation evidence passes.

## Verification

Run from the repository root using Bash with login disabled and the existing
Node 24.21.0 toolchain on PATH. ROOT reviewed the five-file package and granted
the exclusive local-heavy lease on 2026-10-08. Under that lease first check
dependencies; only if missing use once
`(cd apps && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 install --frozen-lockfile)`.
No repeated installs, changed lockfile, all-worker overrides, or overlapping
heavy checks. Retain original native start/session/full actual terminal plus
owned PID/PGID/startticks; join before advancing. No timeout constitutes PASS.

RED, after new test exists and before source changes (expected causal failure):

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 app/office/workspace/settings/components/office-config-sync-save-drafts.test.tsx -t 'retains a newer directory draft and dirty contributor after an older acknowledgment')
```

GREEN, after implementation and complete coverage:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 exec vitest run --maxWorkers=1 app/office/workspace/settings/components/office-config-sync-save-drafts.test.tsx hooks/domains/office/use-office-config-sync.test.ts app/office/workspace/settings/components/office-config-sync-section.test.tsx lib/api/domains/office-config-sync-api.test.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/domains/office/use-office-config-sync.ts app/office/workspace/settings/components/office-config-sync-save-drafts.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the repository `.github/scripts/pr-docs.cjs` local `validateCoverage`
preflight using the actual changed work order, manifest, Office requirement,
and design; the following exact command checks coverage for the owned hook
path. Use the existing pinned toolchain in PATH (this shell's installed Node
was found under mise); do not install tools during design.

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/office/requirements/config-sync-surfaces.md',
  'docs/specs/office/system-design/config-sync-surfaces.md',
  'docs/plans/office-config-save-drafts/plan.md',
  'docs/plans/office-config-save-drafts/task-01-preserve-save-drafts.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({
  changedFiles: [...documents, 'apps/web/hooks/domains/office/use-office-config-sync.ts'],
  fileContents,
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered' || result.workOrders.length !== 1) process.exitCode = 1;
NODE
```

No CI status publishing from local preflight. Product tests may not
be replaced with mock-only hook/controller controls or imported discovery proof.
No broad local passing replay; rerun checks only for actual changes/findings.

## Files likely touched

- `apps/web/hooks/domains/office/use-office-config-sync.ts` (only production file).
- `apps/web/app/office/workspace/settings/components/office-config-sync-save-drafts.test.tsx`
  (new independently authored permanent coverage).
- `docs/specs/office/requirements/config-sync-surfaces.md` (policy link only).
- `docs/specs/office/system-design/config-sync-surfaces.md` (bounded design).
- `docs/specs/office/system-design/config-sync.md` (link to bounded design).
- This work order and its sibling manifest (statuses/results).

## Dependencies

None. Implementation requires the later explicit ROOT release; heavy checks
require its exclusive lease. Publication and SERIAL MERGE are separate barriers
in the durable task plan. No operator approval/model prompt or callback ACK gate.

## Risks

- Stale-closure comparison, partial equality, and normalized-payload comparison
  can silently lose newer drafts; tests must observe the actual fields and
  contributor together.
- Acknowledging only when unchanged would leave Reset targeting the wrong
  baseline; always advance config independently of draft replacement.
- An environment/transport/resource failure does not prove the defect or the
  fix. Checkpoint ROOT with original receipts rather than alter assertions,
  increase timeouts, race commands, or expand scope.

## Parallelism

`sequential`. Exactly one work order; no agents/tasks/sessions/tabs to create.

## Inputs

- [Office surfaces requirement](../../specs/office/requirements/config-sync-surfaces.md),
  [bounded design](../../specs/office/system-design/config-sync-surfaces.md),
  [shared save requirement](../../specs/ui/requirements/settings-manual-save.md),
  [ADR 0046](../../decisions/0046-settings-route-save-coordinator.md).
- Current real hook, section, shared save provider and guard, API payload tests;
  `agent-profile-page-state.ts` is existing local draft/ACK comparison prior art.
- ROOT proof receipt and discovery brief, read-only only; proof is not a test
  implementation resource.

## Results

Implemented after ROOT's later reviewed-package release under the exclusive
local-heavy lease. The independent causal RED reached live-input, route-dirty,
and contributor-dirty failures before the source change. Final GREEN passed
all four listed files, 44 tests including 20 rendered behavioral cases.
Affected ESLint (zero warnings), direct typecheck, i18n check and ratchet each
terminated with exit 0. Initial test selectors and lint organization/constants
were corrected without weakening assertions. All original command output and
owned process receipts are retained in the durable task plan and
`/tmp/kandev-child89-save-drafts-20261008/`; checks were sequential, joined, and
freshly absent before advancing. No resource/timeout failure, broad suite,
browser, or build occurred. See [the manifest](plan.md#verification-results).
Normal active hooks/publication, hosted checks/review, and separate ROOT merge
authorization remain delivery gates.
