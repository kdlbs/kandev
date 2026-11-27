---
id: "01-task-session-ownership"
title: "Scope fallback and request state to the selected task"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-NAVIGATION-RESPONSIVENESS-001
acceptance_criteria:
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.8
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.9
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.10
  - AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.11
system_design:
  - ../../specs/ui/system-design/task-navigation-responsiveness.md
---

# Task 01: Scope fallback and request state to the selected task

## Summary

Implement the existing resolver's task-keyed fallback/request ownership and
prove the first new-task render and obsolete settlements with real hook/store
coverage. Prove the reachable consumer URL/active-store boundary without changing
selection policy, transport, store actions, or mounted component composition.

## Inputs

- [Requirements `.8`-`.11`](../../specs/ui/requirements/task-navigation-responsiveness.md).
- [Mounted fallback design](../../specs/ui/system-design/task-navigation-responsiveness.md#mounted-task-session-fallback-ownership).
- [Plan evidence and caller audit](plan.md#accepted-evidence-and-current-source).
- Read scoped web guidance and `/tdd` on the later implementation turn.
- Read-only ROOT proof and receipts listed in the plan. Never replay or edit them.

## In scope

- Existing hook's local fallback/request state and coherent ownership projection.
- New real-hook and actual-consumer tests, WS transport mocked plus ROOT's
  explicit marker-scoped viewport fixture below.
- Results/status synchronization in this package after actual checks.

## Out of scope

Store/cache/backend/API/membership/primary-policy/freshness/framework changes;
consumer production edits, navigation/layout/copy/schema changes, remounts;
new workers/tasks/sessions/models; browser/build/E2E/full-suite/backend checks.

## Acceptance

1. Every `.8`-`.11` case has real-hook/StateProvider/createAppStore evidence,
   including settled Alpha/Beta and null/different-task reopen RED before GREEN.
   Old success and failure cannot change current result or clear current loading.
2. Store primary-or-first and first-response fallback, timeout, return interface,
   current success/empty/failure/no-client/null/loading and independent instances
   remain compatible. No extracted-predicate-only or implementation-string tests.
3. Real `KanbanWithPreview` integration exercises the reachable store/URL path
   with missing higher-priority metadata and proves the primary-metadata control.
   All exact checks below pass under ROOT's sole heavy lease; no extra gates.

## Files likely touched

- `apps/web/hooks/use-task-session.ts`
- `apps/web/hooks/use-task-session.test.tsx` (new)
- `apps/web/components/kanban-with-preview.session-ownership.test.tsx` (new)
- This plan/work order and owning requirement/design pair for accurate results.

`KanbanWithPreview`, `PreviewSessionTabs`, `session-sort`, StateProvider and store
source are read-only integration dependencies. Do not edit production consumers.
No package/lock/config/generated source edits are part of this task.

## Dependencies and release barriers

None within the package. Wait for a LATER ROOT implementation INTERRUPT to this
same primary session/profile and a single GLOBAL LOCAL-HEAVY lease. No design
turn install/tests/heavy checks/commit/push/PR. Preserve source/proof identities
and managed worktree, dependencies, caches, foreign processes and refs.

After release, inspect dependencies without mutation. Only if absent/unusable,
perform ONE pinned frozen install from apps, while holding the heavy lease:

```bash
(cd apps && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 300s corepack pnpm@9.15.9 install --frozen-lockfile)
```

No second install or automatic timeout/resource/transport recovery. Checkpoint
ROOT for bounded direction. Active normal hooks, never bypass hooks.

## Verification

Run sequentially from repo root after release; retain and actually join every
returned handle, including failures. Capture command, cwd, exact exit, log and
RED/GREEN names/counts. No passing broad replay. The RED run uses the same new
permanent suites before the smallest production correction; ROOT's archived
proof is not executed. Deferred promises must settle in cleanup without
beforeEach returning a mock, and no fixture teardown errors count as RED proof.

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 exec vitest run hooks/use-task-session.test.tsx components/kanban-with-preview.session-ownership.test.tsx --project browser-locales --testNamePattern='^(task session fallback ownership|Kanban preview task session fallback ownership)( |$)' --maxWorkers=1 --no-file-parallelism)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 300s corepack pnpm@9.15.9 exec eslint hooks/use-task-session.ts hooks/use-task-session.test.tsx components/kanban-with-preview.session-ownership.test.tsx --max-warnings 0)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 300s corepack pnpm@9.15.9 run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 timeout --kill-after=10s 180s corepack pnpm@9.15.9 run i18n:ratchet --base 8776877048ea029b8f861b4d658be719dfbadff2)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

Typecheck's existing pretypecheck generator may create ignored required inputs;
inspect status and do not include generated artifacts in the correction.
Localization has no new copy, but changed-file enforcement still runs. Docs
validation is a reference check, not a public-doc rewrite or validator test suite.
Test coverage is the explicit AC matrix in plan.md; no instrumentation/full-suite
coverage run. Both named suites must collect and execute, not silently skip.

Run actual changed-path PR documentation-reference coverage from repo root:

```bash
node - <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const validator = require('./.github/scripts/pr-docs.cjs');
const paths = [...new Set([
  ...cp.execFileSync('git', ['diff', '--name-only', '8776877048ea029b8f861b4d658be719dfbadff2'], {encoding:'utf8'}).trim().split('\n'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], {encoding:'utf8'}).trim().split('\n'),
].filter(Boolean))];
const docs = [
  'docs/plans/task-session-fallback-ownership/plan.md',
  'docs/plans/task-session-fallback-ownership/task-01-task-session-ownership.md',
  'docs/specs/ui/requirements/task-navigation-responsiveness.md',
  'docs/specs/ui/system-design/task-navigation-responsiveness.md',
];
const result = validator.validateCoverage({changedFiles: paths,
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]))});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exit(1);
NODE
```

During design, use these cheap catalog/spec/reference/whitespace checks only.
Additionally supply the planned hook path to validateCoverage as a clearly
labelled prospective runtime-coverage check so a docs-only exempt result cannot
be mistaken for linked runtime coverage. Later delivery uses actual changed paths.
Routine in-scope assertion/fixture/lint corrections rerun affected checks only.
Resource, timeout, transport, unknown or out-of-scope blockers checkpoint ROOT.

## Parallelism

`sequential`. No delegation. No concurrent install/test/lint/typecheck/heavy hook
with another child; ROOT alone releases the global lease.

## Risks

Render-time gating is essential; effect-only clearing is insufficient. Do not
weaken cleanup or impose a new same-task retention/freshness rule. Consumer
fixtures can mask the defect with primary metadata or membership validation;
assert the actual store/URL boundary and describe visible-chat limits honestly.

## Results

Implementation started after ROOT's later release. The following initial
blocked fixture receipts are preserved history; the renewed release and local
verification results below supersede that checkpoint.

- One conditional pinned frozen install: handle 29916 joined exit 0, 933 reused,
  zero downloaded, 2 s; `/tmp/kandev-child40-install.log`. No second install.
- Initial permanent hook run 73069 joined exit 1: 14 tests, two causal first-Beta
  wrong-Alpha REDs, 12 controls pass, 3.03 s. Consumer file creation used a wrong
  cwd-relative path, so only hook suite collected. Not both-suite evidence.
- Exact both-suite run 46679 joined exit 1, 7.90 s: hook 2 RED/12 pass, consumer
  import missing ignored release-notes input. Not consumer RED.
- Exact approved initial typecheck 72704 joined exit 2: its existing prehook
  generated ignored release-notes/changelog; new consumer fixture lacked color
  and complete UserSettingsState. Corrected using actual production defaults.
  Typecheck has not yet passed and needs rerun after the final test/source edit.
- Exact both-suite run 57427 joined exit 1, 11.43 s: hook 2 causal RED/12 pass;
  consumer 3 collected but missing real ToastProvider. Added actual ToastProvider
  and CommandRegistryProvider, no mocked provider/selection logic.
- Affected consumer run 36179 joined exit 1, 12.99 s: missing Beta card because
  fixture omitted workflow/snapshot selection data. Supplied actual store shapes.
- Affected consumer run 76449 joined exit 1, 13.02 s: all three consumer tests
  still fail at missing Beta card. Real virtualized column task list uses
  `useVirtualizer` with measured viewport; Happy DOM supplies zero dimensions,
  yielding no card rows. No causal consumer result or passing metadata control.

Logs are `/tmp/kandev-child40-red.log`, `-red-both.log`,
`-typecheck-initial.log`, `-red-ready.log`, `-red-consumer.log`, and
`-red-consumer-ready.log`. The two new test files remain provisional; no GREEN
or permanent consumer proof claimed. Existing Happy DOM HTTP404 fallback is
caught by the plural-session hook and withholds chat; only WS is mocked.

All owned handles actually joined, none live. LOCAL-HEAVY returned to ROOT at
this checkpoint. Await bounded ROOT direction for the real consumer layout
fixture and a renewed heavy lease. No automatic geometry/virtualizer mock,
browser run, package install, widened production behavior, or transport recovery.
The accepted ROOT proof remains untouched. Design-only results above remain
historical; implementation checks are pending.

## ROOT bounded viewport-fixture release

ROOT renewed the exclusive GLOBAL LOCAL-HEAVY lease after reviewing the joined
checkpoint and real virtualizer source. The prior blocked results above remain
historical. Task 01 resumes in the same primary session/profile.

One explicit test-environment exception to transport-only mocking supplies
`offsetHeight=600` and `offsetWidth=320` only on real elements matching the
existing `data-testid="kanban-column-scroll"` marker. Preserve original getters
for every other element, install before mount, and restore after every test.
The real virtualizer, cards, provider/store, consumer, selection/actions, hook
and URL updates remain active. No other geometry/framework mocks are authorized.

First run only the affected consumer suite with the exact existing project,
anchored name, worker and memory limits. Require two causal switching REDs and
the current-primary control PASS before changing production. Retain the prior
hook RED receipt without replay. If the scoped viewport still fails to expose
real cards or reveals a distinct runtime boundary, join and checkpoint ROOT
without expanding the fixture. Then apply the smallest hook correction, run the
approved affected GREEN/checks, and normal publication. No MERGE lease.

### Joined scoped viewport RED

Consumer-only run handle 7792 actually joined exit 1, 11.88 s, log
`/tmp/kandev-child40-red-consumer-viewport.log`. The scoped getters expose the
real cards. Both direct switch and close/different reopen fail causally at
Beta's active store retaining `alpha-session`; the primary-metadata control
passes. Three cases execute, no setup/teardown errors. Existing caught plural
HTTP404 logs remain honest. No wider fixture change was needed. The prior
meaningful hook RED is retained rather than replayed. Production correction
and the exact approved two-suite GREEN are next.

## Local implementation results

The correction changes only the existing hook's local snapshot and render-time
projection. Requests retain per-effect cleanup, the 10000 ms transport timeout,
first-list fallback, and store marked-primary-or-first priority. Same-task
close/reopen retains settled fallback while a new read is pending. No consumer
production, store, API, layout, schema, package, or backend edits.

- Exact two-suite GREEN 54386 actually joined exit 0, 14.08 s, 17 tests PASS.
- Prettier completed exit 0 on the three TS/TSX files.
- Changed ESLint 12415 joined exit 1 (five warnings); 26587 joined exit 1
  (one remaining duplicate literal warning). Routine test constants and ternary
  corrections addressed those findings. Final 2570 joined exit 0.
- Approved typecheck 73307 joined exit 0; normal ignored generators only.
- i18n check 34154 and ratchet 33851 both joined exit 0, no new product copy.
- After those fixture-only lint corrections, exact affected GREEN 87620 joined
  exit 0, 13.09 s: both suites execute, 17 PASS. This supplies current real
  active-store and URL isolation, current Beta settlement, metadata precedence,
  plus the 14 hook contract cases. No browser/full-suite replay.

Logs: `/tmp/kandev-child40-green.log`, `-green-final.log`, `-eslint.log`,
`-eslint-corrected.log`, `-eslint-final.log`, `-typecheck-final.log`,
`-i18n-check.log`, `-i18n-ratchet.log`. Earlier failure receipts remain retained.
All local handles are actually joined. Task 01 remains in progress until actual
delivery gates and owned-handle joins finish. LOCAL-HEAVY is exclusively held
through normal publication, then returned. No MERGE lease; no claim of completed
delivery from local success or PR publication. ROOT proof remains untouched.

### Documentation validation before publication

Catalog validation exit 0 (351 decisions, 1,368 specs), all-spec lint exit 0,
public-doc validator exit 0 (47 pages), and `git diff --check` exit 0. Actual
seven-path PR documentation-reference coverage reports `covered`, errors empty,
with the changed runtime hook linked to this one work order and the owning
requirement/design. All command handles terminal. Public docs need no content
change; this restores the documented behavior without changing UI composition,
copy, commands, or settings. State-only mobile exception needs no screenshots
or browser/E2E runs. Normal active hooks and ready publication are next;
hosted gates and ROOT's separate merge lease remain pending.
