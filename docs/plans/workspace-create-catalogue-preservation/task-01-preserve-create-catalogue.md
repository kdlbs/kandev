---
id: "01-preserve-create-catalogue"
title: "Preserve creation catalogue publication"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-SETTINGS-UPDATES-003
acceptance_criteria:
  - AC-WORKSPACES-SETTINGS-UPDATES-003.1
  - AC-WORKSPACES-SETTINGS-UPDATES-003.2
  - AC-WORKSPACES-SETTINGS-UPDATES-003.3
  - AC-WORKSPACES-SETTINGS-UPDATES-003.4
  - AC-WORKSPACES-SETTINGS-UPDATES-003.5
system_design:
  - ../../specs/workspaces/system-design/workspace-settings-updates.md
---

# Task 01: Preserve creation catalogue publication

## Summary

Independently prove and repair successful Add Workspace publication against the
initiating provider's current catalogue. Preserve independent current changes
and one accepted mapped identity, along with existing selection, form, and errors.
Execute sequentially only after ROOT's later reviewed-package implementation
interrupt plus exclusive global local-heavy lease in the original primary.

## In scope

- Read the owning pair, plan, scoped AGENTS, `/tdd`, and current source; repeat
  only the bounded creation consumer inventory at release.
- Independently author the [plan's named cases](plan.md#tests-and-traceability)
  using real Page/provider/createAppStore/useRequest/action/client/registered
  notifications and actual sidebar/management choices. Mock external transports
  only. Never access ROOT protected source proofs/fixtures or replay discovery.
- Change `workspaces-page-client.tsx` successful publication to use the existing
  current-provider reader, accepted mapper, and same-ID filtering. Keep read,
  map and setter synchronous after acknowledged creation.
- Maintain exact criteria .1-.5 and preserve merged Save/backend presence
  contracts. Record actual command results and lifecycle/delivery receipts.

## Out of scope

No backend/API/schema/store-framework/revision/permission/navigation/lifecycle
redesign, other create/delete/placement/save repair, mapper or WS handler edits,
runtime/harness changes, broad writer audit, browser/build/full suites/E2E/DB.
No native delegates, tasks, new tabs/sessions, model/profile switches or operator
approval prompt. Extra causal immediate glue requires a ROOT checkpoint.

## Acceptance

1. Independent additions/updates/deletions survive accepted creation in current
   store and real picker/management consumers. Before-ACK same-ID notification
   yields one accepted mapped row, with other descriptors/order intact.
2. Both notification settlement directions and ordinary/empty/blank/failure,
   selection/revision, descriptor/default and independent-store controls follow
   their existing behavior through actual product boundaries.
3. The bounded production diff and independently authored tests pass the exact
   released checks with original joined receipts; mobile/public-doc assessments
   still hold. Preserve all later RETURN/hosted/merge barriers.

## Fixture and causal evidence

Mount actual `WorkspacesPage` at `/settings/workspaces`, real `StateProvider`
(therefore real `createAppStore`), Toast/Tooltip providers, and actual sidebar
picker/routing. Observe `useAppStoreApi` without replacing it. Two isolation
roots must be independent; nested providers intentionally share their parent.
Drive real Add Workspace buttons/input/submit. Hold the actual POST fetch;
capture its exact trimmed JSON payload. Supply only legitimate section-count
GET responses from the real API clients and reject unexpected requests. Simulate
WS delivery using `registerWorkspacesHandlers` for the actual observed store.
No mocks of product components, hooks, actions, clients, stores, handlers,
mapper, router or UI primitives.

Before releasing held success, assert the independently delivered effect in the
store, actual open picker and management rows. After acceptance, assert all
three consequences plus the accepted row. Each claimed add/update/delete
regression must reach its relevant causal assertions on pre-fix production.
Same-ID deduplication is a control protecting the current-store correction and
may pass on the original captured-array source. Controls must pass independently; fixture exceptions and
unhandled/unexpected requests are not regressions. After-ACK stimuli verify
current registered handler semantics without promising target version arbitration.

Descriptor controls state expected fields directly, including caller scopes,
role/member count, owner/unit, description, four defaults, idle policy/defaults,
Office kind and timestamps. Selection controls observe current revision after
real selection, not a captured request-start identity. Management active-first
display and raw catalogue ordering have different existing contracts. Ordinary
choice selection follows the real picker and existing Home URL behavior; it
must not introduce a new navigation contract. Empty creation retains existing
first-row fallback and does not invent a revision increment.

Failure uses actual failed HTTP response and real toast; retain input/open form,
independent notification and selection, without publishing an accepted row.
Own all deferred transport promises and admitted count reads; settle and join
them during success/failure cleanup. Unmount providers and clean timers,
observations, fetch/history/cookies/guard state as needed. No sleeps, timeout
weakening, leaked background work, or copied ROOT proof source.

## Files likely touched

Expected production ownership:

- `apps/web/app/settings/workspace/workspaces-page-client.tsx`

Permanent regression ownership after release:

- `apps/web/app/settings/workspace/workspaces-create.integration.test.tsx`
- `apps/web/app/settings/workspace/workspaces-create.test-helpers.tsx` (optional fixture-size split)

Delivery ownership: existing Workspaces workspace-settings-updates requirement
and design, this work order, and `plan.md`. All other production/tests remain
read-only. Existing Save or unrelated test suites are not reopened.

## Dependencies

No task dependency. At design, `apps/node_modules` and `apps/web/node_modules`
are absent. Only after release and lease, verify the existing project-pinned
launcher from `apps/` is pnpm9.15.9. Use existing mise Node24.21.0 bin under Bash
`login:false`; a missing/mismatched launcher checkpoints ROOT before alternatives.
If dependencies remain absent, run ONE frozen install (no runtime replacement,
cache deletion or duplicate install):

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps && pnpm --version)
(cd apps && NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 10m pnpm install --frozen-lockfile)
```

## Verification after later release

DESIGN authorizes none of the following product commands. After explicit ROOT
release plus lease, run each command truly serially in its own retained original
native handle under Bash `login:false`. Record original initial/session/chunks,
subprocess cwd/argv/PID/PGID/environment/UTC/bounds/exit/raw logs, ACTUALLY JOIN,
and fresh exact own wrapper/groups absence. Do not infer status from duplicates.

Causal RED before production edit: independently authored cases and strict
controls, expected assertion failure(s), no unhandled/unexpected transport.
GREEN after the narrow publication edit uses the same bounded test command:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 6m pnpm exec vitest run app/settings/workspace/workspaces-create.integration.test.tsx --maxWorkers=1)
```

Run the task checks once after GREEN. Optional helper, if authored, must be
included by its exact path in changed-file ESLint/Prettier commands; it is
imported by the integration test. If absent, omit only that helper argument.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 5m pnpm exec eslint --max-warnings=0 app/settings/workspace/workspaces-page-client.tsx app/settings/workspace/workspaces-create.integration.test.tsx app/settings/workspace/workspaces-create.test-helpers.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 2m pnpm exec prettier --check app/settings/workspace/workspaces-page-client.tsx app/settings/workspace/workspaces-create.integration.test.tsx app/settings/workspace/workspaces-create.test-helpers.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 6m pnpm run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 5m pnpm run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --signal=TERM --kill-after=10s 5m pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before commit, run the actual changed-path preflight from repo root (HEAD diff
includes staged and unstaged paths; untracked files include the new work order):

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
  'docs/specs/workspaces/requirements/workspace-settings-updates.md',
  'docs/specs/workspaces/system-design/workspace-settings-updates.md',
  'docs/plans/workspace-create-catalogue-preservation/plan.md',
  'docs/plans/workspace-create-catalogue-preservation/task-01-preserve-create-catalogue.md',
];
const result = validateCoverage({
  changedFiles: changedPaths.map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(docs.map(filename => [filename, fs.readFileSync(filename, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
assert.equal(result.ok, true);
assert.equal(result.status, 'covered');
assert.deepEqual(result.errors, []);
assert.deepEqual(result.workOrders, [docs[3]]);
JS
```

This uses the actual repository validator and must accept the work order's plan
link, requirement/AC ownership and design declaration. Retain the changed-path
evidence for publication and reconcile it with the actual PR diff/head/base.
At design, document-only actual paths are exempt; a separately labelled
prospective production reference preflight must still validate the actual
package. No API publication or synthetic Git commit is needed for preflight.

No broad compatibility suite, backend check, full Vitest/E2E/build or DB run.
Routine causal fixture/lint/format repairs rerun only affected checks; record
their distinct original receipts. Resource/timeout/transport/unknown/out-of-scope
failure checkpoints ROOT before retry or alternatives. Active commit hooks run
normally later, without bypass. Do not replay passing tests for docs-only changes.

## Mobile and public documentation

[Creation verification/mobile/public assessment](../../specs/workspaces/system-design/workspace-settings-updates.md#creation-verification-mobile-and-public-documentation)
uses the pure state/data exception with actual shared picker tests. No rendered
composition/copy/control/touch/scrolling/navigation/breakpoint edits: no ASCII
preview, browser, screenshot, product build or mobile Playwright test planned.
Published Add Workspace/default/placement procedures and screenshots remain
accurate, so no public docs change is needed. Recheck the final diff; any
surface/boundary expansion checkpoints ROOT.

## Parallelism and standing gates

`sequential`. [All phase/resource gates](plan.md#phase-and-resource-gates) and the
own Kandev plan apply. DESIGN ends at the four-artifact handoff; later reviewed
INTERRUPT plus lease required. Normal ready publication then EXPLICIT GLOBAL
LOCAL-HEAVY RETURN and END; no local-heavy work without a fresh lease. HOSTED
OBSERVER/SEMANTIC HOLD until separate ROOT release, one original collector/full
current App347564 evidence; MERGE NONE until separate static compatibility grant.
Normal expected-head squash/noadmin, verified authoritative merge and owned joins/
cleanup are required for task completion. Preserve ROOT proof and managed/foreign
resources for ROOT archive. No child-to-ROOT interrupt, retries of optional queued
callbacks, delegates, new task/tab/session or model change. Critical parent
question ends the turn immediately.

## Inputs

- [Requirement 003 and criteria .1-.5](../../specs/workspaces/requirements/workspace-settings-updates.md).
- [Successful creation publication design](../../specs/workspaces/system-design/workspace-settings-updates.md#successful-creation-publication).
- [Named matrix and evidence limits](plan.md#tests-and-traceability).
- Read-only real Page/provider/action/useRequest/mapper/registered WS/picker/
  management clients and existing current-store Save idiom. Never ROOT proof source.

## Risks

See [plan risks](plan.md#risks): stale reads, same-ID descriptor loss, legitimate
display-order differences and swallowed transport/cleanup failures. No material
product question remains; unexpected boundary expansion checkpoints ROOT.

## Results

ROOT reviewed all four design artifacts and released implementation in this
same primary session. Reviewed hashes matched before the `in_progress` change.
The production correction reads the initiating provider after successful
creation and uses the existing mapper plus same-ID filtering; no other
production file changed.

The first independent fixture run had a management-link lookup mistake: its
expected query suffix differed from the actual overview link. After correcting
that lookup, original03 reached the before/after causal assertions for each
independent add, update, and deletion: three failed, ten controls passed.
Original04 then passed all thirteen cases with the production correction.
Neither fixture exceptions nor the first run are counted as qualified RED.

Original06 and original07 scoped ESLint found duplicate test literals. Constants
retained the exact values and assertions; original08 passed with zero warnings.
Original09 format check identified only the integration test after those
constant edits; original10 formatted it and original11 passed. Original12 web
typecheck and original13 i18n checks passed. These mechanical repairs did not
change the causal stimuli, expectations, product source, or test policy.

Original20 normal commit hooks caught the newly staged helper's synthetic form
input as untranslated copy. An explained `i18n-exempt` comment identifies the
test input used to verify trimming; its value and behavior remain unchanged.
The original hook failure is retained, and the affected guard plus normal
hooks are required to pass before publication.

Each original process is retained separately with native chunks/session,
actual subprocess join, cwd/argv/environment, PID/PGID, UTC interval, exit,
raw logs, and fresh own wrapper/group absence under `/tmp/kandev-child96-runs/`.
The own Kandev plan owns final local delivery/publication receipts and resource
return. Task01 remains in progress through the separate hosted/merge gates;
ready publication alone does not complete it. No browser/build/E2E/DB/backend
checks or protected ROOT source access occurred. Mobile state/data and accurate
public-doc no-change assessments remain applicable to the final bounded diff.
