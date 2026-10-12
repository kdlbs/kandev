---
id: "01-preserve-delete-inventory"
title: "Preserve current inventories on accepted row deletion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-DELETION-CATALOGUE-001
acceptance_criteria:
  - AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.1
  - AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.2
  - AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.3
  - AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.4
  - AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.5
  - AC-AGENTS-PROFILE-DELETION-CATALOGUE-001.6
system_design:
  - ../../specs/agents/system-design/creation-catalogue.md
---

# Task 01: Preserve current inventories on accepted row deletion

## Summary

Remove only the accepted deletion target from each current slice in the actual
`ProfileRow`, retaining all other settings profiles and picker options. Prove
the reported race through a real row, app store, registered events and visible
task/subtask selector choices. This single work order owns the entire repair.

## In scope

- `ProfileRow.handleDelete` success branch, unused import and invariant comment.
- An independently authored deletion-inventory test alongside the row. The
  manifest's regression matrix is the minimum scenario set; existing row/admin
  controls remain verification inputs, not a fake substitute for live-state proof.
- Matching result/status receipts in this package and the existing agents pair.

## Out of scope

- Backend, API, global handlers, option contracts, fetch reconciliation,
  absent-owner materialization, selection changes and layout/copy redesign.
- Adjacent save/duplicate/creation/enablement fixes, `useProfileEnabledToggle`,
  executor specs and child102's connection-save work. Do not revert others.
- Reading, copying, deleting or executing ROOT's protected candidate test;
  accessing protected paused worktrees/volumes or pruning shared resources.

## Acceptance

1. After a deferred successful delete and real global created event for an absent
   settings owner, both current inventories lose only the target, and real task
   and subtask selectors retain the received choice and selected label. The new
   test first fails on that actual loss; ordinary-delete and failure controls pass.
2. Newer unrelated metadata, disabled eligibility, global/Office boundaries and
   overlapping completion orders remain correct. Assert full option values and
   current settings metadata, not just lengths or profile IDs.
3. Rejected, handled and conflict responses perform no local removal. Existing
   guided navigation, errors, cancellation/focus, permissions and mobile controls
   remain; the exact focused checks pass with recorded receipts.

## Implementation sequence

After ROOT's later explicit release, mark this order `in_progress`. Acquire the
actual local-heavy lease before bootstrap or verification. Read the current row
and owning pair; recheck source compatibility without rebasing solely for main
drift. Author `agent-profiles-section-delete-inventory.test.tsx` independently.

Mount actual `ProfileRow`, subscribed `createAppStore`/`useStore` or real
`StateProvider`, and actual `registerAgentsHandlers`. Keep production actions
and normalize the real event. Defer the external DELETE transport response.
Seed real auth and settled hook readiness; no setup/unhandled failure qualifies.
Render actual `CreateEditSelectors` with `AgentSelector` and actual subtask
`SelectorsRow`, fed by subscribed production option derivation. Assert an actual
selectable received label before ACK, then retention afterward; do not mock
those selectors or build a test-only inventory renderer.

Include a sibling, another owner's option, newer model/label/fallback/capability
metadata and a disabled option whose settings representation is older. Pass an
Office-scoped created event through the real unchanged handler and prove it
never becomes a global option. Exercise two deletion response orders. Use unique
event/profile IDs and cleanly settle pending promises/subscriptions/UI.

Run the targeted RED first, recording its assertion and passing controls. Then
replace only the flatten rebuild: independently filter target ID from current
settings profiles and current picker options after ACK. Keep existing synchronous
setters and slice metadata/version, with no await inside publication. Remove
unused `toAgentProfileOption` and fix its misleading comment. Keep all error and
confirmation paths intact. Do not add an abstraction or broaden merge policy.
Run GREEN and the exact checks, update both Results sections and mark `done` only
with actual evidence. Continue later delivery under the manifest's ROOT gates.

## ASCII UI preview

UI-01, from the [full preview](plan.md#ascii-ui-preview); .1, .2, .5 and .6:

```text
Desktop: Target row -> Delete icon -> [Cancel] [Delete]
Phone:   Target row -> [...] -> Delete -> [Cancel] [Delete]
After ACK (both): target absent; sibling and received global choice retained
Task/Subtask: [Retained selected label v] -> real eligible choices
```

Use existing confirmation shells and focus owner; the phone menu closes before
confirmation. Geometry is illustrative. No control, overlay, navigation, scroll,
safe-area or localized-copy changes. Preserve actual option labels and gates.
The narrow state/data-only mobile-parity exception applies. Real rendered row,
task/subtask selectors and affected row/admin/confirmation controls cover the
changed behavior. The existing phone E2E is read-only compatibility context;
no replay or rebuild is required. Actual viewport/layout/control changes require
a ROOT scope reassessment.

## Verification

Commands below run from repository root after explicit implementation release
and lease grant. Bootstrap once per fresh managed workspace, never concurrently
with another heavy owner. The managed Node directory also contains executable
pnpm; plain root PATH currently contains neither. Run commands sequentially,
retain every returned handle through interruptions and join the original to
terminal completion. A timeout is a failed receipt, never inferred success or
permission to duplicate a running command.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps && timeout --signal=TERM --kill-after=10s 20m pnpm install --frozen-lockfile)
(cd apps/web && timeout --signal=TERM --kill-after=10s 5m pnpm exec vitest run components/settings/agents/agent-profiles-section-delete-inventory.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 5m pnpm exec vitest run components/settings/agents/agent-profiles-section-delete-inventory.test.tsx components/settings/agents/agent-profiles-section.test.tsx components/settings/agents/agent-admin-gating.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 5m pnpm exec eslint --max-warnings 0 components/settings/agents/agent-profiles-section.tsx components/settings/agents/agent-profiles-section-delete-inventory.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 10m pnpm run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 5m pnpm run i18n:ratchet)
timeout --signal=TERM --kill-after=10s 1m python3 scripts/list-docs.py validate
timeout --signal=TERM --kill-after=10s 1m python3 scripts/lint-spec-files.py --all
timeout --signal=TERM --kill-after=10s 10s git diff --check
timeout --signal=TERM --kill-after=10s 10s git status --short -- docs/plans/agent-profile-delete-inventory
```

The single-file run is RED before production changes. The combined GREEN run is
the final row/controls result; do not repeat passing broad checks without a new
failure/change. No untouched mobile E2E replay or build gate. If an existing row test is
changed for a concrete deletion assertion, include it in the targeted eslint
command. Do not edit adjacent consumers to simplify test setup.

Local PR-documentation coverage preflight, deferred from design-only. Run from
repository root with the executable verified present in this managed workspace.
Enumerate actual tracked changes and all untracked paths from the admitted base,
including the new regression; do not substitute an expected file list.

```bash
timeout --signal=TERM --kill-after=10s 1m /home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node - <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const admittedBase = 'c176df170bf2ceee5ab4e3f55dc3b296ab187b55';
const git = args => execFileSync('git', args, {
  cwd: process.cwd(), encoding: 'utf8', timeout: 15000, maxBuffer: 4 * 1024 * 1024,
});
const tracked = git(['diff', '--name-status', '-z', '--no-renames', admittedBase, '--'])
  .split('\0').filter(Boolean);
if (tracked.length % 2) throw new Error('Malformed tracked diff');
const statuses = { A: 'added', M: 'modified', D: 'removed', T: 'modified' };
const byPath = new Map();
for (let i = 0; i < tracked.length; i += 2) {
  const status = statuses[tracked[i]];
  if (!status) throw new Error(`Unsupported diff status: ${tracked[i]}`);
  byPath.set(tracked[i + 1], { filename: tracked[i + 1], status });
}
for (const filename of git(['ls-files', '--others', '--exclude-standard', '-z', '--'])
  .split('\0').filter(Boolean)) {
  byPath.set(filename, { filename, status: 'added' });
}
const changedFiles = [...byPath.values()].sort((a, b) => a.filename.localeCompare(b.filename));
const docs = [
  'docs/plans/agent-profile-delete-inventory/plan.md',
  'docs/plans/agent-profile-delete-inventory/task-01-preserve-delete-inventory.md',
  'docs/specs/agents/requirements/creation-catalogue.md',
  'docs/specs/agents/system-design/creation-catalogue.md',
];
const documentPaths = new Set([
  ...docs,
  ...changedFiles.filter(c => c.status !== 'removed' &&
    c.filename.startsWith('docs/') && c.filename.endsWith('.md')).map(c => c.filename),
]);
const fileContents = Object.fromEntries([...documentPaths].map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered' || result.errors.length !== 0 ||
    result.workOrders.length !== 1 ||
    result.workOrders[0] !== 'docs/plans/agent-profile-delete-inventory/task-01-preserve-delete-inventory.md') {
  process.exitCode = 1;
}
NODE
```

## Files likely touched

- `apps/web/components/settings/agents/agent-profiles-section.tsx`
- `apps/web/components/settings/agents/agent-profiles-section-delete-inventory.test.tsx` (new)
- `apps/web/components/settings/agents/agent-profiles-section.test.tsx` (only if a concrete deletion compatibility assertion needs updating)
- `docs/specs/agents/requirements/creation-catalogue.md`
- `docs/specs/agents/system-design/creation-catalogue.md`
- This manifest and this work order.

Read-only inputs: actual actions/registered handlers, option contracts,
duplication merge precedent, task/subtask selectors, admin controls and existing
mobile row E2E. No production changes outside the row are authorized.

## Dependencies

No preceding work order. ROOT's later explicit implementation interrupt and
actual local-heavy lease grant are required execution boundaries. Publication
and merge follow the separate gates in [the manifest](plan.md#execution-and-delivery-constraints).
No native delegates, persistent workers, new tabs or model switches.

## Risks

- Pre-await snapshots or full option reconstruction would lose independent data.
- Fake-store existing tests do not establish event-to-picker correctness.
- Absent owners, equal timestamps and capability refreshes make revision merging
  broader than deletion needs. Retain unrelated options rather than arbitrate them.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/agents/requirements/creation-catalogue.md#list-row-deletion),
  `REQ-AGENTS-PROFILE-DELETION-CATALOGUE-001`, criteria .1-.6.
- [Design](../../specs/agents/system-design/creation-catalogue.md#accepted-list-row-deletion-publication).
- Qualified receipt and source paths named in the manifest; protected test excluded.

## Results

ROOT reviewed the corrected four-artifact package and explicitly released
implementation on 2026-10-10 to this same primary with exclusive lease103.

Independent RED14864 joined exit1: the actual task selector loses the received
label after ACK, with two passing ordinary/failure controls and no setup error.
The minimal correction filters only the accepted target ID from current options;
current settings filtering and all error/control branches remain intact.

Final affected GREEN34730 joined exit0: three suites, 30 tests (nine independent
row/store/WS/task+subtask-selector scenarios and 21 existing row/admin controls).
Failed intermediate fixture checks and their bounded original joins are recorded
in [the manifest receipts](plan.md#implementation-receipts). No protected
candidate was read/copied and no backend/handler/API/selection/layout was changed.

Docs/preflight43843 joined exit0, covering all six actual tracked/untracked paths
against the admitted base: ok, covered, errors=[], exactly this work order.
Catalogue 368 decisions/1496 specifications and full spec lint passed. One install
only; no build or untouched mobile E2E replay.

Final sequential check20538 joined exit0: scoped ESLint with zero warnings,
web typecheck and i18n ratchet passed. Implementation is done. Lease103 remains
held for normal hooks; publication and hosted readiness receipts continue in the
external task plan. Merge requires the separate ROOT expected-head grant.
Logs live in `/tmp/kandev-child103-implementation-20261010/`.
