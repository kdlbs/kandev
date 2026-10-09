---
id: "01-list-read-lifetime"
title: "Correct coordinator list read lifetime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-004
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-004.8
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
---

# Task 01: Correct coordinator list read lifetime

## Summary

ROOT's later explicit release authorized the completed regression tests and
local list read publication correction in `useCoordinators`.
Use the [plan's scenario matrix](plan.md#tests) and
[owning design](../../specs/coordinator/system-design/list-publication.md#settings-list-read-publication).
Design creation does not authorize execution.

## In scope

- Local tokens shared by initial load/refresh and invalidated on workspace,
  null, owner and consumer-lifetime changes before cached early returns.
- Real-hook/store and real rendered-list RED/GREEN, with partial `fetchJson`
  transport substitution only. Author both new suites under describe prefix
  `Coordinator list publication`; cover every plan scenario and all observable
  rows/loaded/loading/error and link outcomes.
- Existing compatibility checks, exact verification, normal hooks and accurate
  statuses/results. Preserve cached reads, current error/Retry, flag admission
  and defaults. Mobile state-only assessment remains the plan's recorded choice.

## Out of scope

Generic framework, backend/API, rollout, mutation ordering, store redesign,
markup/copy/layout, browser/build/E2E, delegation, new sessions, model switches,
proof replay/copy/import/mutation, foreign caches/worktrees, optional polish.

## Acceptance

1. Faithful real-hook/AppStore and rendered-list causal tests fail on the
   original hook for return/overlap, while the distinct-workspace control passes.
   Capture RED assertion evidence; setup/resource failures do not count.
2. The local correction makes the complete scenario matrix pass, including
   obsolete success/failure/finally and current failure/Retry, without changing
   the existing slice, CRUD, admission or rendering contracts.
3. Exact checks below pass and original process groups are joined/gone. Update
   this order to done and the manifest to implemented only after evidence and
   requirement/design reconciliation; do not claim completion of the broader
   workspace-coordinator package. Stop/report limitations to ROOT.

## Files likely touched

- `apps/web/hooks/domains/settings/use-coordinators.ts`
- `apps/web/hooks/domains/settings/use-coordinators.lifetime.test.tsx` (new)
- `apps/web/components/coordinators/coordinators-list-publication.test.tsx` (new)
- `apps/web/hooks/domains/settings/use-coordinators.test.ts` only if its fixture
  needs routine compatibility adjustment for the local correction.
- This order and `docs/plans/coordinator-list-publication/plan.md` for results.
- Existing paired requirement/design status only when that document's full
  lifecycle supports promotion; this narrow order cannot certify the entire
  broader draft specification.

`coordinators-list-page.tsx`, `coordinator-add-page.tsx`, `state-provider.tsx`,
the coordinator slice and `coordinator-api.ts` are read-only inputs, not owned
production changes. No lockfile/package/config or generated-file edits.

## Verification

Run from repository root. Design runs only cheap docs gates. Product commands
below are for the later explicit implementation turn and permitted resource
window. Use the existing Node 24 environment; do not install alternate runtimes.
If dependencies are absent, perform exactly one install:

```bash
(cd apps && corepack pnpm@9.15.9 install --frozen-lockfile)
```

Retain that original handle/log/cutoff/PID/PGID; setup or resource failure ends
work and reports to ROOT without replay or cache cleanup. The one conditional
install completed successfully; do not repeat it.

For both RED and GREEN, run exactly this anchored causal selection:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run --project browser-locales hooks/domains/settings/use-coordinators.lifetime.test.tsx components/coordinators/coordinators-list-publication.test.tsx -t '^Coordinator list publication ' --maxWorkers=1 --no-file-parallelism)
```

Then run the existing compatibility selection once after GREEN:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run --project browser-locales hooks/domains/settings/use-coordinators.test.ts components/coordinators/coordinators-list-page.test.tsx components/coordinators/coordinator-add-page.test.tsx lib/settings/workspace-settings-tabs.test.ts -t '^(useCoordinators (fetching|mutations|load errors)|CoordinatorsListPage|CoordinatorAddPage|getWorkspaceSettingsTabs|workspaceSettingsHref) ' --maxWorkers=1 --no-file-parallelism)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/domains/settings/use-coordinators.ts hooks/domains/settings/use-coordinators.lifetime.test.tsx components/coordinators/coordinators-list-publication.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s env NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:ratchet)
```

If the existing hook fixture changes, add its exact path to the changed-file
eslint command. Run normal hooks during later authorized commit; never bypass
them. Typecheck's normal prehook generates release/changelog inputs: inspect
status and report unexpected tracked changes rather than expanding scope.

Cheap docs gates, independently rooted, apply at design and after implementation:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/coordinator docs/plans/coordinator-list-publication
git status --short -- docs/plans/coordinator-list-publication
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/coordinator/requirements/coordinators.md',
  'docs/specs/coordinator/system-design/coordinators.md',
  'docs/plans/coordinator-list-publication/plan.md',
  'docs/plans/coordinator-list-publication/task-01-list-read-lifetime.md',
];
const changedFiles = paths.map(filename => ({ filename, status: filename.startsWith('docs/plans/') ? 'added' : 'modified', additions: 1, changes: 1 }));
const fileContents = Object.fromEntries(paths.map(file => [file, fs.readFileSync(file, 'utf8')]));
const docs = validateCoverage({ changedFiles, fileContents });
const projected = validateCoverage({
  changedFiles: [...changedFiles, { filename: 'apps/web/hooks/domains/settings/use-coordinators.ts', status: 'modified' }],
  fileContents,
});
console.log(JSON.stringify({ docs, projected }, null, 2));
if (!docs.ok || docs.status !== 'exempt' || !projected.ok || projected.status !== 'covered') process.exitCode = 1;
NODE
```

At delivery, rerun the repository coverage evaluator against actual changed
paths, including production/test paths. Docs-only paths are legitimately exempt;
the projected hook path checks references and is not a hosted actual-path gate.
Retain each original command's handle, log, cutoff,
PID/PGID and terminal exit; join/prove gone before the next dependent check.
Timeout/setup/resource/unknown/out-of-scope: end work and report to ROOT. Routine
in-scope fixture/lint correction: affected checks only, no automatic broad replay.

## Dependencies and parallelism

None. `sequential`, same primary only. ROOT released the exclusive local-heavy
window after design ended; no delegation or new sessions.

## Inputs and risks

Read-only accepted proof/receipt/classification/log and identity are recorded in
[the plan](plan.md#evidence-and-scope). Do not run or import them. The existing
hook/string guard, real provider and slice, API transport, list body error/Retry
and card URLs are authoritative inputs. Risks: abandoned loading settlement,
same-ID owner replacement, and mocked fixtures masking the original causal bug.
Read-versus-mutation and independent concurrent consumer reconciliation are
excluded. No question barrier, title ownership or completion gate may be removed
from the live task plan.

## Results

Done for AC004.8. Existing broader paired draft specs remain draft; this narrow
order does not certify the whole coordinator feature.

| Check | Actual result / original receipt name |
| --- | --- |
| Corrected evaluator + missed diff/inventory | Passed: `corrected-preflight`, `missed-inventory`; docs exempt/ok and projected hook covered, not hosted evidence |
| One conditional frozen install | Passed: `frozen-install`, 2.27s |
| Original causal RED | `causal-red`, exit 1: 17 causal failures, four passing controls, one nested StrictMode fixture error |
| Corrected root StrictMode RED | `strict-fixture-red`, exit 1 on obsolete rows (fixture error is not RED) |
| Committed foreign-link RED | `commit-link-red`, exit 1 on A link under B URL before passive loading |
| Final causal GREEN | `commit-link-green`, 22 passed in two files after final production edits/formatting |
| Named compatibility | `compatibility`, 27 passed in four files, once; unchanged mocked presentation/CRUD controls not replayed |
| Changed eslint | `final-changed-lint`, passed for all three changed TS/TSX files; earlier size-only failure repaired by test-group split |
| Project typecheck | `final-project-typecheck`, passed using normal prehook + project command, 10.634s |
| i18n checks | `i18n-check` and `i18n-ratchet`, passed |

All listed original processes were actually joined and their process groups
proved gone. Exact argv, UTC cutoffs, PID/PGIDs, exits and logs are retained in
`/tmp/kandev-child57-<receipt-name>-receipt.json` and same-prefix `.log` files;
product commands used Node 24.21.0, pinned pnpm 9.15.9, 4 GiB and a supervisor
with the documented 120s TERM / 10s KILL bound (instead of shell `timeout`).
Vitest used browser-locales, anchored names, one worker and no file parallelism.
The first design evaluator spawn failure remains failed in the manifest history.

Actual changed-path coverage and normal active-hook receipts are retained in
the live task plan before publication. No browser/build/E2E or generic audit.
The PR must remain unmerged until hosted gates and ROOT's separate serial grant.
