---
id: "01-save-refresh-boundary"
title: "Preserve acknowledged sleep settings through status refreshes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-TASK-SLEEP-INHIBITION-001
acceptance_criteria:
  - AC-PLATFORM-TASK-SLEEP-INHIBITION-001.9
  - AC-PLATFORM-TASK-SLEEP-INHIBITION-001.10
  - AC-PLATFORM-TASK-SLEEP-INHIBITION-001.11
system_design:
  - ../../specs/platform/system-design/task-sleep-inhibition.md
---

# Task 01: Save and Refresh Publication Boundary

## Summary

Implement the narrow acknowledged-save boundary in the existing sleep hook.
Independently authored real transport-boundary regressions must prove both stale
publication suppression and unchanged current caller/save behavior. One bounded
sequential outcome; no implementation before the later ROOT reviewed-package
INTERRUPT.

## In scope

- Module-private store generation/current read and committed hook binding in
  `use-sleep-inhibition-settings.ts`, using `useAppStoreApi` and existing actions.
- Initial/visibility/poll/retry GET admission and all success/catch/finally
  publication; old flights may not block a new post-ACK read or clear its loading.
- Save admission, successful ACK fence and actual response/error propagation.
  Inactive retained save callbacks decline through existing cancellation type;
  already-admitted PATCH keeps its actual captured-store completion.
- Two new independently authored causal suites named below. Real
  `SleepInhibitionSettings` standalone/grouped with actual `SettingsSaveProvider`
  and `saveAll`; include actual TaskBehavior Runtime attention where relevant.
- Minimal immediate card completion guard only after a causal failing integration
  test demonstrates the hook boundary alone leaves a lifetime gap. This is the
  only conditional production scope; retain existing draft/revision semantics.
- Synchronize these four design artifacts with actual results/lifecycle only.

## Out of scope

Common save-provider, store/slice, API transport/shape, state-provider, General/
TaskBehavior routing/markup, backend/nativeOS/power policy, task admission,
authentication, runtime flags/defaults/cadence, UI copy/layout/navigation,
probe aborts, generic request framework and blanket writer audits. No installs,
production/permanent tests, browser/E2E/build/Go during design. Later no browser,
E2E, build or Go verification for this data-only change. Do not copy, replay,
import, edit or remove ROOT proof files or clean foreign/managed resources.

## Acceptance

1. Causal deferred tests fail before the fix for stale GET success/rejection
   after actual successful ACK (pre-save GET and GET during held PATCH), then
   pass with no response/error/loading regression and unchanged actual caller
   response. Cover opposite completion order and post-ACK GET controls.
2. Current loading/error/retry and failed-save behavior remain correct. Real
   card/provider tests preserve unsaved drafts, permitted in-save edits and
   current boolean revision/canLeave/failedIds behavior, plus grouped attention.
   No hook/provider/store mocks or ownership-predicate tautologies.
3. Shared-store views/remounts and independent stores, A-B/A-B-A, retained
   callbacks at commit before passive cleanup, closed dialog/unmount and
   StrictMode do not permit retired GET success/reject/finally or callbacks to
   disturb the newest view/pending owner. Admitted PATCH still completes on
   the backend/caller and captured store; no synthetic result or new mutation
   ordering policy. Exact checks pass and all owned processes are joined/gone.

## Causal regression matrix

| Interleaving | Required assertions |
| --- | --- |
| GET admitted, then PATCH; PATCH held, then ACK, then old GET success/reject | Request admissions, ACK response, saved enabled/error remain correct; real provider may block a card save while GET loads, so use direct hook for that pre-save ordering |
| Toggle/edit, real saveAll admits PATCH, visibilitychange admits GET, ACK, then old GET success/reject | Rendered checked/dirty/attention, actual store enabled/error/loading and saveAll canLeave/failedIds; no bypass of canSave |
| Same competing GET settles before ACK | Useful current GET settles; ACK ultimately wins; returned PATCH result preserved |
| ACK followed by new GET while old transport still pending | New transport admitted immediately; old success/reject/finally cannot clear new loading, error or response; then new GET succeeds/fails and retry recovers |
| Failed PATCH with current GET before/after rejection | Actual rejection/provider failure, unchanged save baseline/draft, useful read still publishes; no success fence on failure |
| Unsaved edit/current runtime refresh; allowed edit while save pending | Existing draft alignment and boolean revision behavior, not an invented edit counter |
| Two bindings sharing/nesting StateProvider; independent roots | Same-store ACK fences sibling read; latest admitted read owns loading; independent root stores unchanged |
| A-B/A-B-A binding; retained actions in replacement layout effect before old passive cleanup | Real provider identities, transport admission count, old GET terminal no-op; retired refresh no-op/save cancellation, current actions work |
| Actual card close/unmount/reopen and StrictMode | Deferred GET after retirement cannot revive state or clear newer loading; fresh mount reads work; admitted PATCH preserves captured-store actual result; all timers/listeners/deferred work settled |
| Current happy/error controls | No competing GET save, new GET after ACK, initial load failure/retry, visible polling and current failure with cached response; member read-only unchanged |

Use test helpers owned by the two new files only. Partially mock
`@/lib/api/client` exports `fetchJson` and `fetchJsonWithRetry`; retain other
exports. Other routes needed by the real Runtime consumer get bounded explicit
transport fixtures, not mocked domain hooks. Mock only necessary actual router/
navigation boundary in addition. Assert real endpoint/method/body/admission and
observable store/DOM/coordinator/caller outcomes. Test names or nearby comments
map to .9-.11. Settle deferred promises and unmount in finally; no real sleeps.

## Files likely touched

Mandatory production ownership:

- `apps/web/hooks/domains/settings/use-sleep-inhibition-settings.ts`

Mandatory new test ownership:

- `apps/web/hooks/domains/settings/use-sleep-inhibition-settings.test.tsx`
- `apps/web/components/settings/sleep-inhibition-save-refresh.test.tsx`

Conditional immediate production ownership, only after demonstrated gap:

- `apps/web/components/settings/sleep-inhibition-settings.tsx`

Artifact ownership:

- `docs/specs/platform/requirements/task-sleep-inhibition.md`
- `docs/specs/platform/system-design/task-sleep-inhibition.md`
- `docs/plans/sleep-inhibition-save-refresh/plan.md`
- `docs/plans/sleep-inhibition-save-refresh/task-01-save-refresh-boundary.md`

Existing card/provider/TaskBehavior/slice suites are validation inputs, not
arbitrary modification permission. The test selector below explicitly covers
both new suites and these regression controls.

## Dependencies and parallelism

None. `sequential`. Same primary session/profile/executor, no delegation,
newtask/session/tab/model change. ROOT must grant the global local-heavy slot
before install/Vitest/lint/typecheck/i18n/commit hooks. A later ROOT SERIAL MERGE
INTERRUPT independently authorizes merge; standing delivery permission alone
does not authorize that action.

## Verification

All commands run from repo root with `/bin/bash` and `login:false`. First use
read-only checks for exact HEAD/blob and tool versions; moving main is not
permission to rebase or replay evidence. After ROOT implementation interruption,
mark this order in_progress and use `/tdd`. Author the two suites independently,
run their causal red cases, then implement and run the targeted green selection.

Use existing Node 24 and pinned pnpm 9.15.9, explicitly in PATH:

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
node --version
/home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm --version
```

Use the absolute pnpm executable in every command, so an earlier PATH pnpm
cannot select another version. If Node 24 or that pinned executable is absent,
checkpoint ROOT instead of downloading a replacement. Exactly one conditional
frozen install is allowed if workspace dependencies are absent or the first
package command establishes missing setup; do not delete existing dependencies:

```bash
(cd apps && timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm install --frozen-lockfile)
```

For each original command, create an owned private receipt/log prefix outside
the repo. Before launch record UTC, cwd, argv and log path. Start one isolated
process group, record original PID/PGID and native tool handle immediately,
retain that same handle until the actual wait returns, then record exit/end
UTC and verify every original PID/group is gone. The tooling invocation itself
must use Bash login:false and the explicit PATH above. Do not begin the next
heavy command until ACTUALLY JOINED/gone. A yielded exec cell/session is not a
result; poll the retained original handle, without a duplicate command.
If a timeout terminates the group, join and verify gone; it is a ROOT checkpoint,
not a test verdict or authorization for an automatic retry. Never kill a
foreign group. Apply the same receipt rule to install, hooks and later observer.

Exact targeted run (outer 120s, kill-after 10s, Node 4GiB, one worker, no file
parallelism; browser-locales is the actual configured project for these suites):

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm exec vitest run --project browser-locales --maxWorkers=1 --no-file-parallelism hooks/domains/settings/use-sleep-inhibition-settings.test.tsx components/settings/sleep-inhibition-save-refresh.test.tsx components/settings/sleep-inhibition-settings.test.tsx components/settings/settings-save-provider.test.tsx components/settings/task-behavior-settings.test.tsx lib/state/slices/settings/settings-slice.test.ts)
```

Red run uses the same limits/project and only the two new files, selected by the
independently authored causal test names using `-t`; record the actual names and
argv before launching. Baseline tests must fail on intended assertions rather
than setup/transport errors. No ROOT candidate replay. Green commands below
run serially once; routine own-fixture or lint repairs rerun affected checks
only. Preserve normal active hooks, no HUSKY/SKIP bypass.

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm exec eslint --max-warnings 0 hooks/domains/settings/use-sleep-inhibition-settings.ts hooks/domains/settings/use-sleep-inhibition-settings.test.tsx components/settings/sleep-inhibition-save-refresh.test.tsx)
(cd apps && timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm exec prettier --check web/hooks/domains/settings/use-sleep-inhibition-settings.ts web/hooks/domains/settings/use-sleep-inhibition-settings.test.tsx web/components/settings/sleep-inhibition-save-refresh.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py specs --system platform --text task-sleep-inhibition --format paths
git diff --check
git status --short
```

If the conditional card guard is changed, add the following affected-file
commands (the targeted Vitest selection above already covers its integration):

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm exec eslint --max-warnings 0 components/settings/sleep-inhibition-settings.tsx)
(cd apps && timeout --signal=TERM --kill-after=10s 120s /home/jcfs/.local/share/mise/installs/pnpm/9.15.9/pnpm exec prettier --check web/components/settings/sleep-inhibition-settings.tsx)
```

Audit all four artifacts' repository Markdown links and untracked-file whitespace,
then call the actual `.github/scripts/pr-docs.cjs` exported `validateCoverage`
with current on-disk artifact contents and actual changed paths plus every
projected owned production/test path. Require `ok=true`, `status=covered`,
`errors=[]`, one work order and accepted references to the real paired platform
requirement/design. A docs-only exemption is not projected coverage. The design
receipt records the concrete preflight; rerun it against actual final changes
and refreshed artifact contents before delivery. Exact local preflight from
repo root (uses only actual on-disk documents and the validator, with declared
projected paths; does not create production or test files):

```bash
/home/jcfs/.nvm/versions/node/v24.18.0/bin/node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/platform/requirements/task-sleep-inhibition.md',
  'docs/specs/platform/system-design/task-sleep-inhibition.md',
  'docs/plans/sleep-inhibition-save-refresh/plan.md',
  'docs/plans/sleep-inhibition-save-refresh/task-01-save-refresh-boundary.md',
];
const projected = [
  'apps/web/hooks/domains/settings/use-sleep-inhibition-settings.ts',
  'apps/web/hooks/domains/settings/use-sleep-inhibition-settings.test.tsx',
  'apps/web/components/settings/sleep-inhibition-save-refresh.test.tsx',
  'apps/web/components/settings/sleep-inhibition-settings.tsx',
];
const actual = [...new Set([
  ...execFileSync('git', ['diff', 'HEAD', '--name-only', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean),
])];
const result = validateCoverage({
  changedFiles: [...new Set([...actual, ...projected])].map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered' || result.errors.length || result.workOrders.length !== 1 || result.acceptedReferences.length !== 1) process.exit(1);
JS
```

Check exact test selectors against actual collected project suites before
marking done; report counts,
not planned counts. Unexpected setup/resource/timeout/transport/unknown or
out-of-scope errors checkpoint ROOT; no blind retry/cache wipe/foreign kill/
broad passing replay. No separate QA/review/simplify/full verification pass.

## Delivery and closure barriers

After targeted checks, synchronize .9-.11/results, promote the draft design to
current only when implementation conforms, mark order done/plan implemented,
and use commit/push/PR skills for a ready PR under standing authorization.
Normal hooks and no bypass; build no new worktree, preserve dependencies.
Freeze the exact head except actual corrective findings. Record clean tree,
local/remote head alignment and every original joined/gone before explicit
`LOCAL-HEAVY RETURN` to ROOT.

Then run exactly one original 90-minute all-terminal hosted observer with GNU
91-minute outer timeout and kill-after 10s; retain its original native handle
and receipt through head fixups and findings. The actual helper exists at the
design base. After recording the original command identity, use this exact
argv from repo root, with `KANDEV_SLEEP_PR_NUMBER` set to the actual created PR:

```bash
timeout --signal=TERM --kill-after=10s 91m scripts/pr-await "$KANDEV_SLEEP_PR_NUMBER" --mode all-terminal --deadline-min 90 --interval-sec 60 --format json
```

If unavailable checkpoint ROOT; no duplicate collector or self-successor.
A completed original's receipt remains authoritative only for its observed
head. Use focused fresh exact-head snapshots after actual corrections; never
claim its old verdict applies to a new head or launch a replacement observer. Deadline/lost response is NO VERDICT. Require six known required
contexts (discover and record their exact names) AND actual Backend/Frontend/
E2E parents SUCCESS, fresh complete errors[], clear visible/hidden/actionable/
human findings, and authenticated CodeRabbit App347564 substantive FULL current
head/all changed files review. Accept sufficient automatic review; inspect gaps
before one necessary full-review request; no ACK-only/optional duplicate review
or uncontrolled hosted retry. Correct valid findings using affected checks and
normal hooks under ROOT's global-heavy scheduling.

Return `MERGE READY END`, then stop for the separate ROOT SERIAL MERGE INTERRUPT.
Only then normal expected-head squash; independently verify actual merge
commit/tree, owned contracts, remote inclusion and joined only-owned cleanup.
Do not update a queued head without documented authorized dequeue/restore.
Task stays incomplete until verified merge and owned closure. ROOT owns
archive, protected proof release and refill; do not remove proofs or managed
worktree/dependencies. No moving-main rebase/synthetic tests/optional polish.

## Inputs

- [Requirement .9-.11](../../specs/platform/requirements/task-sleep-inhibition.md).
- [Full design](../../specs/platform/system-design/task-sleep-inhibition.md),
  especially store lifetime, acknowledgement boundary and draft/save contract.
- Existing real card/API/provider/state source; nearby `useCoordinators`
  layout-retirement pattern and `agent-list-resource.ts` store-keyed WeakMap
  pattern, without adopting either as a new generic framework.
- [Original implemented companion](../task-sleep-inhibition/plan.md), unchanged.
- Protected ROOT proof classification and receipt references in [manifest](plan.md#evidence-and-source-boundary), read-only; no replay/copy/import.

## Risks

Ownership must cover all terminal writes and commit/lifetime admission without
changing actual admitted mutation outcomes. Component fixtures must respect
canSave and current boolean draft/revision semantics. New tests must test real
consumers rather than mirror the implementation. A new architecture, public
contract or out-of-scope required edit is a ROOT checkpoint.

## Results

ROOT reviewed all four artifacts; private review hashes are recorded at
`/tmp/kandev-root-child60-design-review-hashes-20261006.json`. Authoring release
was followed by exclusive GLOBAL LOCAL-HEAVY after child59's audited return.
Task 01 is done; hosted delivery and merge remain the separate task-level gates.

Independent transport RED produced nine causal failures and six passing controls.
The hook correction passed all 18 hook cases and existing controls. The full
82-test selection exposed one fixture query and a causal reopened-card baseline
gap. Minimal card alignment glue and the query repair passed all 18 affected
card tests. An added cached-error reopen Retry test failed before local callback
resolution at click time. Final affected GREEN passed all 37 tests in three
suites; the other 46 unchanged controls passed in the preceding full selection.
This is 83 distinct passing tests across recorded runs, not a claimed single
83-test invocation.

Final changed lint (zero warnings), format, normal typecheck with its generation
prehook, i18n check and ratchet passed. Own long test-group warnings were fixed
without weakening checks. Initial wrong-cwd lint had no verdict; ROOT received
the setup checkpoint before the inspected reviewed-cwd command. All originals
through 19 actually joined and their owned groups are gone. Receipts/log names
are indexed in the manifest. Final docs/coverage checks and normal hook evidence
are recorded before delivery. Merge is unauthorized until MERGE READY END and
the later separate ROOT serial interruption.
