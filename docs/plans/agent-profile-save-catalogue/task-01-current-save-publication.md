---
id: "01-current-save-publication"
title: "Publish concrete saves against current inventory"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-SAVE-CATALOGUE-001
acceptance_criteria:
  - AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.1
  - AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.2
  - AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.3
  - AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.4
  - AC-AGENTS-PROFILE-SAVE-CATALOGUE-001.5
system_design:
  - ../../specs/agents/system-design/creation-catalogue.md
---

# Task 01: Publish concrete saves against current inventory

## Summary

Repair the active concrete profile save acknowledgement writer so deleted
membership stays absent and unrelated current inventory stays intact. Preserve
existing target response acceptance, canonical baseline and newer draft behavior.

## In scope

- Independently authored real editor/coordinator/API/WS/store/selector
  regression with causal RED and ordinary success/rejected PATCH controls PASS.
- Read current inventory after accepted response, skip absent owner/target,
  update only the target and merge only its option projection.
- Remove captured inputs only from save and its active caller; adapt existing
  payload tests, verify compatibility and record actual checks/results.

## Out of scope

No delete, duplicate, creation, dynamic, global handler, backend, API, selection
policy, tombstone, layout/copy/touch/navigation or server arbitration changes.
No protected ROOT reproduction access, foreign resource mutation, delegates,
new sessions or model changes. No implementation before later ROOT release.

## Acceptance

1. The plan's named causal rendered regression fails only after accepted save
   restores the deleted sibling before the fix, with both controls passing;
   the same independently authored case passes afterward.
2. Present target publication preserves all unrelated current membership and
   option metadata; missing owner/target is never inserted. Actual eligible and
   ineligible task/subtask choices retain their expected label/availability.
3. Existing revision acceptance, normalized baseline, in-flight drafts,
   validation, permissions, Cursor MCP, conflict/error and force semantics
   remain effective; all changed suites and scoped checks pass.

## Verification

Acquire ROOT's exclusive local lease before dependencies/tests/statics/hooks.
If workspace dependencies are missing after release, run
`(cd apps && pnpm install --frozen-lockfile)` once; do not install at design.
Use the existing mise Node 24 bin in PATH for package and coverage commands.
Mark this order `in_progress` only after explicit release.

Before production changes, independently write the three initial rendered tests
named in the plan and run only the new suite. Require one causal failure and
two controls passing; retain the native handle and actual terminal join:

```bash
(cd apps/web && pnpm exec vitest run components/settings/agent-profile-save-catalogue.test.tsx --maxWorkers=1)
```

After the minimum fix and bounded additional cases, run from repo root:

```bash
(cd apps/web && pnpm exec vitest run components/settings/agent-profile-save-catalogue.test.tsx components/settings/agent-profile-page-state.test.ts components/settings/agent-profile-page.test.tsx components/settings/agent-profile-reconciliation.test.ts components/settings/agent-profile-picker.test.tsx --maxWorkers=1)
(cd apps/web && pnpm exec eslint components/settings/agent-profile-page-state.ts components/settings/agent-profile-page.tsx components/settings/agent-profile-page-state.test.ts components/settings/agent-profile-save-catalogue.test.tsx --max-warnings 0)
(cd apps && pnpm exec prettier --check web/components/settings/agent-profile-page-state.ts web/components/settings/agent-profile-page.tsx web/components/settings/agent-profile-page-state.test.ts web/components/settings/agent-profile-save-catalogue.test.tsx)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Run the actual tracked-plus-untracked coverage preflight from repo root:

```bash
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'JS'
const fs = require('node:fs');
const cp = require('node:child_process');
const assert = require('node:assert/strict');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = args => cp.execFileSync('git', args, { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedPaths = [...new Set([
  ...paths(['diff', '--name-only', '-z', 'HEAD']),
  ...paths(['ls-files', '--others', '--exclude-standard', '-z']),
])];
const docs = [
  'docs/specs/agents/requirements/creation-catalogue.md',
  'docs/specs/agents/system-design/creation-catalogue.md',
  'docs/plans/agent-profile-save-catalogue/plan.md',
  'docs/plans/agent-profile-save-catalogue/task-01-current-save-publication.md',
];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const actual = validateCoverage({ changedFiles: changedPaths, fileContents });
console.log(JSON.stringify({ phase: 'actual', ...actual }, null, 2));
assert.equal(actual.ok, true);
assert.deepEqual(actual.errors, []);
const production = 'apps/web/components/settings/agent-profile-page-state.ts';
const prospective = validateCoverage({ changedFiles: [...new Set([...changedPaths, production])], fileContents });
console.log(JSON.stringify({ phase: 'prospective production reference', ...prospective }, null, 2));
assert.equal(prospective.ok, true);
assert.equal(prospective.status, 'covered');
assert.deepEqual(prospective.errors, []);
assert.deepEqual(prospective.workOrders, [docs[3]]);
assert.deepEqual(fs.readdirSync('docs/plans/agent-profile-save-catalogue').filter(p => /^task-.*\.md$/.test(p)), ['task-01-current-save-publication.md']);
JS
```

At design, actual docs-only paths are exempt; prospective production validates
the real requirement/AC/design/manifest/order references without claiming code
changes. At implementation, actual coverage must be `covered` and include all
actual changed paths. Read every command result; retain and join every session.
No broad test/build/backend or untouched mobile E2E run is required for this
shared state/data-only path under the mobile-parity exception.

Before publication, update both order status and manifest checkbox/results,
use normal commit hooks and the repo PR template/body file. Return the lease
after all owned commands join, then continue hosted validation in the same turn
as ROOT directs. Do not bypass hooks or independently merge.

## Files likely touched

- `apps/web/components/settings/agent-profile-page-state.ts`
- `apps/web/components/settings/agent-profile-page.tsx`
- `apps/web/components/settings/agent-profile-page-state.test.ts`
- `apps/web/components/settings/agent-profile-save-catalogue.test.tsx` (new)
- `docs/specs/agents/requirements/creation-catalogue.md`
- `docs/specs/agents/system-design/creation-catalogue.md`
- `docs/plans/agent-profile-save-catalogue/plan.md`
- `docs/plans/agent-profile-save-catalogue/task-01-current-save-publication.md`

## Dependencies

None. Existing completed creation/deletion plans are related contracts, not
work orders to reopen. ROOT's later explicit implementation release and local
lease are execution gates.

## Inputs

- [Owning requirement](../../specs/agents/requirements/creation-catalogue.md#concrete-editor-save).
- [Owning design](../../specs/agents/system-design/creation-catalogue.md#concrete-editor-save-publication).
- [Plan test mapping](plan.md#tests).
- `agent-profile-page-state.test.ts`, `agent-profile-reconciliation.test.ts`
  and `dynamic-agent-profile-editor-save-concurrency.test.tsx` are read-only
  test patterns; the dynamic implementation is outside scope.

## Risks

Mocked acceptance/store/WS/consumer would hide the defect. Rebuilding all
options would violate independent metadata preservation. Absent target handling
must not create a broader conflict or ordering policy. Seed settled real
capabilities; explicitly document any noncausal external isolation.

## Parallelism

`sequential`

## Results

Design checks passed on 2026-10-10: catalogue validation (368 decisions/1498
specifications), 36 spec-validator tests, full spec lint, actual four-path
docs-only coverage (`exempt`) and prospective production references (`covered`,
zero errors, exactly this order), plus whitespace. Native documentation session
57323 joined exit 0/terminal `81464b`; other checks completed synchronously.
The [manifest](plan.md#verification-results) records the handoff and size limits.
The limited docs lease was returned; ROOT then explicitly released implementation.

Implementation completed. Independent RED native60600 joined exit 1/`a1b433`: one
causal post-ACK failure, two controls PASS. Initial GREEN native37120 joined 0/
`a6d871`; final five affected suites native78946 joined 0/`243800`, 43 tests PASS
including eight new integration cases. Scoped ESLint zero warnings, four-file
Prettier check PASS, i18n native92510 joined 0/`9ee478`, and normal web typecheck
native69862 joined 0/`723f04` after correcting incomplete test wire annotations.
Docs native57608 joined 0/`b0f2bd` (catalogue, 36 tests, full lint, whitespace).
Actual tracked+untracked coverage `covered`, zero errors, eight paths and exactly
this order. The manifest records all check commands, fixture corrections and
receipts. No check handle remains running. Normal hooks/publication and hosted
validation follow; no hook bypass, adjacent writer or protected resource change.
