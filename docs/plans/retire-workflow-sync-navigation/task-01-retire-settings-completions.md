---
id: "01-retire-settings-completions"
title: "Retire workflow sync settings completions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002
acceptance_criteria:
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.1
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.2
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.3
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.4
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.5
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.6
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.7
  - AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.8
system_design:
  - ../../specs/integrations/system-design/workflow-sync-settings-lifetime.md
---

# Task 01: Retire workflow sync settings completions

## Summary and release barrier

Implement committed hook-instance/workspace admission and settlement, including
local form-input callbacks and immediate dialog save/remove completion. Keep
admitted backend work and its caller outcomes truthful while preventing retired
UI feedback, pending settlement and global reload.

ROOT released implementation and the exclusive global local-heavy lease after
reviewing all four artifacts (private review-hash receipt
`/tmp/kandev-root-child59-design-review-hashes-20261006.json`). Status is
done after the task checks below. The implementation barrier and local-heavy lease are released;
merge remains blocked pending a separate ROOT interrupt. Stay in this primary
session/profile/executor. No delegation or new task/session/tab/model change.
The earlier design turn stopped before code, permanent tests, dependencies or
heavy checks.

## Scope and owned files

Production ownership is limited to:

- `apps/web/hooks/domains/settings/use-workflow-sync.ts` and, only if needed
  for its size limit, a private sibling workflow-sync lifetime/form helper.
- `apps/web/components/settings/workflow-sync-dialog.tsx` for immediate
  committed dismissal/retained-caller admission only.
- `apps/web/components/settings/workflow-sync-section.tsx` only for causally
  needed local owner plumbing; preserve the keyed dialog and layout.

Independently author these prospective permanent causal suites after release:

- `apps/web/hooks/domains/settings/use-workflow-sync.lifetime.test.tsx`.
- `apps/web/components/settings/workflow-sync-section.lifetime.test.tsx`.
- Shared causal fixture: `apps/web/hooks/domains/settings/workflow-sync.lifetime.test-helpers.tsx`.

The private production helper is
`apps/web/hooks/domains/settings/use-workflow-sync-lifetime.ts`; the existing
`workflow-sync-dialog.test.tsx` fixture adds the controller identity. The actual
section and API source stay unchanged.

Use existing adjacent tests as pattern/input, preserving their current outcomes.
Change an existing test only when the implementation requires it; run every
changed suite. Update the four package artifacts' status/results after verified
implementation. The historical GitLab-provider and authorization packages
remain separate and complete. Their prior results are not this work's coverage.

## Out of scope

Backend, DB/schema, API changes, authorization, provider/poller redesign,
transport cancellation, framework/global cache or sequence, generic stale
request overhaul, new same-workspace draft/save policy, new runtime flag,
cadence, translations, geometry, navigation redesign and optional polish.
No protected proof replay/copy/import/mutation/removal. Preserve managed
worktrees/dependencies and foreign resources.

## Acceptance

1. Hook and committed input admission/settlement satisfy .1-.4 and .8 across
   actual unmount/remount, retained-hook replacement and layout-before-passive
   boundaries; only the newest pending control owner may finalize that control.
2. Admitted save/remove still resolve truthful true/false after retirement;
   force remains void. Current payloads, errors/warnings, form resets, initial
   loads, status-only reads and conditional/current refresh satisfy .5-.6.
3. The real immediate section/dialog integration satisfies .7, including
   closed/reopened/replaced/unmounted dialog and successful first save/provider
   switch/current removal on desktop and phone. No hook outcome lie masks a
   stale caller; no own-save config change incorrectly blocks current dismissal.

## TDD and causal matrix

Read `/tdd` on implementation release. Author transport-boundary regressions
independently; do not read/copy/import either protected candidate as a fixture.
First obtain a causal red with real `useWorkflowSync`, `StateProvider`,
`ToastProvider`, real workflow-sync APIs, and the actual immediate consumer
where relevant. Mock only `fetchJson` and the necessary actual navigation
refresh boundary. No hook/provider/store/API-client mocks, predicate-only
tests, synthetic passing tests or timer sleeps. Use deferred transport responses
and assert admission requests/visible outcomes before resolving them.

All IDs below abbreviate `AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002`:

| Suite / stimulus | Required observations | AC |
| --- | --- | --- |
| Hook: removal/changed force admitted A; actually unmount A, mount B, settle A | B config/form/feedback/pending unchanged, no reload; removal true, force void; current controls reload | .1, .5, .6 |
| Hook: A-B and A-B-A in retained instance; keep all action/input callbacks | Old callbacks make no transport or form edits, original writes retain A payload, old A never regains publication | .1-.3, .5, .8 |
| Hook: initial and background GET success/reject, including resolve/invoke in a later layout effect before passive cleanup | No retired initial toast/loading finalizer/config/reset; no old timer admission; current initial/error and silent status-only read controls | .1, .2, .4, .6, .8 |
| Hook: unmount and workspace replacement before passive cleanup; StrictMode cleanup/setup; suspended/uncommitted B render | Retirement at commit boundary, fresh callback-bound activation without old callback/request resurrection, speculative render leaves visible A admitted | .1-.3 |
| Hook: two independent A/B views, same-workspace separate views and separate providers/stores | Local retirement only; surviving independent view still admits its own actions | .3 |
| Hook: overlapping current saves/forces; success and rejection finalizers in both orders; replacement while pending | Older finally cannot clear newest saving/syncing/loading; newest settles; no cross-operation config ordering introduced | .4, .6 |
| Hook: current save/error, removal/error, force success/warnings/response-error/reject/changed/unchanged | Existing bool/void results, current toasts/config/reset, proper reload counts; GET cadence retained | .5, .6, .8 |
| Real section/dialog: submit pending save/remove, close/reopen, change workspace or unmount, then resolve/reject; retain prior click/completion callbacks | No new request from retired caller, no old dismissal/retry publication; no retired toast/reload; true backend success observed independently | .1, .2, .5, .7 |
| Real section/dialog: current first save, existing save, provider switch and removal; failure then retry on desktop/phone | Current successful dialog closes despite own config/target change; failure stays open and existing removal retry works; existing draft/reset semantics | .6-.8 |

Use named scenarios from the manifest and `@covers` annotations for exact ACs
where useful. Assertions must observe product effects rather than the lifetime
token itself. Test actual unmount/remount separately from same-hook API cases
and state the evidence limit. Retain source audit of the real ancestor loader;
no claim of B overwrite through actual navigation. Clear/drain all owned
deferred requests, timers and rendered providers before each suite exits.

For the immediate dialog, audit the committed `onOpenChange` boundary too.
Guard dispatch and completion at the current open/controller activation, while
preserving removal's existing confirmation behavior. Do not couple save
dismissal to a key changed by the save's own returned config.

## Environment and process receipts

Every external original command below gets an upfront receipt with UTC start,
cwd, exact argv/environment, unique private log, PID/PGID, native exec handle,
deadline, and owned resource list. Run via Bash with `login:false` and explicit
Node24/pnpm9.15.9 PATH. Launch a tracked isolated process group; print its PID,
PGID and log before yielding. Record the tool's actual session_id before
polling, or record synchronous terminal completion if no handle was returned.
Never discard an original handle. ACTUALLYJOIN it and verify its PID/group gone
before another local-heavy command, including a repair rerun. Preserve output
byte-for-byte for parser consumption (raw tools or `rtk proxy`, no display
helper). Keep each tool wait at most 60s; no blocking 120s host call.

Use the existing binaries, without downloading another Node or pnpm:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS=--max-old-space-size=4096
```

Check Node major 24 and pnpm exactly 9.15.9. If either configured binary is
unavailable, report a ROOT checkpoint. After release, ONE conditional frozen
install is allowed only if workspace dependencies are absent. Run this once
from repo root, through the same receipt wrapper:

```bash
if [[ ! -f apps/node_modules/.modules.yaml || ! -x apps/web/node_modules/.bin/vitest ]]; then
  (cd apps && timeout --signal=TERM --kill-after=10s 120s pnpm install --frozen-lockfile)
fi
```

A failed/timeout install is a setup checkpoint; do not blindly retry it. Do not
remove dependencies, clear caches or touch foreign processes to recover.

## Exact local verification

Commands are listed in serial order, from repo root with the explicit environment
above. Execute each as its own tracked original, not one opaque shell chain.
For red, run the two newly authored causal suites only using the same Vitest
flags/time limit; classify failures as causal versus setup before changing code.
After green run the final selection once:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism hooks/domains/settings/use-workflow-sync.lifetime.test.tsx components/settings/workflow-sync-section.lifetime.test.tsx hooks/domains/settings/use-workflow-sync.test.ts components/settings/workflow-sync-dialog.test.tsx components/settings/workflow-sync-status-banner.test.tsx lib/api/domains/workflow-sync-api.test.ts src/settings-routes.workspace-data.test.tsx)
```

Changed-file lint and formatting (extend these explicit lists only with actually
changed private helpers/tests; run every changed suite):

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm exec eslint --max-warnings 0 hooks/domains/settings/use-workflow-sync.ts components/settings/workflow-sync-dialog.tsx components/settings/workflow-sync-section.tsx hooks/domains/settings/use-workflow-sync.lifetime.test.tsx components/settings/workflow-sync-section.lifetime.test.tsx)
(cd apps && timeout --signal=TERM --kill-after=10s 120s pnpm exec prettier --check web/hooks/domains/settings/use-workflow-sync.ts web/components/settings/workflow-sync-dialog.tsx web/components/settings/workflow-sync-section.tsx web/hooks/domains/settings/use-workflow-sync.lifetime.test.tsx web/components/settings/workflow-sync-section.lifetime.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
python3 scripts/list-docs.py specs --text workflow-sync --format paths
git diff --check
git status --short
```

Run commands independently even where a code block lists more than one. Project
typecheck's normal prehook may regenerate release/changelog support; inspect
that diff and do not retain unrelated generated churn. Do not bypass the hook.
Use the repository documentation validator on the actual diff, including the
changed work order and its linked plan/design/requirement. From repo root:

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const validator = require('./.github/scripts/pr-docs.cjs');
const artifacts = [
  'docs/specs/integrations/requirements/gitlab-workflow-sync.md',
  'docs/specs/integrations/system-design/workflow-sync-settings-lifetime.md',
  'docs/plans/retire-workflow-sync-navigation/plan.md',
  'docs/plans/retire-workflow-sync-navigation/task-01-retire-settings-completions.md',
];
const base = cp.execFileSync('git', ['merge-base', 'HEAD', 'origin/main'], {encoding: 'utf8'}).trim();
const changed = cp.execFileSync('git', ['diff', '--name-only', '-z', base], {encoding: 'utf8'}).split('\0').filter(Boolean);
const untracked = cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], {encoding: 'utf8'}).split('\0').filter(Boolean);
const result = validator.validateCoverage({
  changedFiles: [...new Set([...changed, ...untracked])],
  fileContents: Object.fromEntries(artifacts.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== 'covered' || result.errors.length) process.exitCode = 1;
NODE
```

This covers actual changed paths and validates actual contract references; it
does not import hypothetical path coverage after implementation. Verify local
Markdown links and frontend source identifiers, every REQ/AC definition, and
work-order/plan/design inclusion. The design-only prospective coverage run is
separate and clearly labeled in Results.

No local backend/build/E2E/full-suite gates are added. The mobile state-only
exception and public-doc audit are in the paired design; preserve phone and
desktop component controls in the causal suites. Reaudit public guide, README,
screenshot catalog and AGENTS against the actual final diff. If scope expands
to copy, geometry, API or backend behavior, checkpoint ROOT before proceeding.

Routine own causal-fixture or lint/format failures may be repaired and only
affected checks rerun under the same lease. Setup/resource/timeout/transport/
unknown/out-of-scope failures require a ROOT checkpoint; no blind retry,
cache wipe, foreign kill or replay of broad passing checks.

## Delivery after verified implementation

Mark this work order done and the plan implemented only after all task checks
pass with actual counts/results. Promote the new design to current; keep the
existing active requirement owner active. Standing authorization covers normal
commit/push/ready PR. Read the delivery skills then; use normal active hooks,
no bypass. Verify clean local HEAD equals remote PR head and every original is
joined/gone, expose `heavyRETURN`, and return the global local-heavy lease.

After heavyRETURN start exactly ONE tracked hosted observer original:

```bash
timeout --signal=TERM --kill-after=10s 91m scripts/pr-await "$WORKFLOW_SYNC_PR" --mode all-terminal --deadline-min 90 --format json
```

ROOT supplies/record the concrete PR number in `WORKFLOW_SYNC_PR`; log the
expanded argv and preserve the original native handle/deadline across findings
and actual corrective head changes. No duplicate collector/self-successor or
manual timed pr-state polling. If the observer exits on findings, preserve its
terminal receipt and checkpoint ROOT for the next authorized observation path;
do not silently create a replacement original. Timeout, lost output or
incomplete evidence is NO VERDICT. No moving-main rebase, synthetic passing
tests, optional changes or uncontrolled hosted reruns. Corrections require the
ROOT local-heavy lease again where applicable.

Before `MERGE READY END`, require six known required contexts plus actual
Backend/Frontend/E2E parents SUCCESS, fresh complete `errors[]`, clear
visible/hidden/actionable/human findings, and authenticated configured
CodeRabbit App347564 substantive FULL CURRENT-HEAD review of all changed files.
Inspect automatic coverage and skip/gap reasons before ONE necessary full
request; accept sufficient automatic review with no ACK/optional duplicate.
Read review bodies and hidden/actionable threads, not only rollup status.

Wait for separate ROOT SERIAL MERGE INTERRUPT and the serial merge lease.
Normal expected-head squash only. Independently verify actual merge commit,
tree, owned requirements/design/implementation and remote inclusion; join and
close only owned resources. Preserve managed worktrees/deps and foreign
resources. Task remains incomplete until actual verified merge and all owned
closure. ROOT owns archive/proof release/refill. Expose checkpoints directly
in conversation; ROOT polls and interrupts, so never depend on queued messages.

## Dependencies and parallelism

No work-order dependencies. ROOT review/release and leases are operational
barriers. `sequential`.

## Inputs and risks

Read the [manifest](plan.md), the
[owning requirement](../../specs/integrations/requirements/gitlab-workflow-sync.md#req-integrations-gitlab-workflow-sync-002-settings-lifetime),
the [design](../../specs/integrations/system-design/workflow-sync-settings-lifetime.md),
`apps/web/AGENTS.md`, relevant existing tests and the actual ancestor source.
Evidence paths/hashes and classification limits are in the manifest; retain
both original private proof sets unchanged.

Risks: retirement before passive cleanup, stale retained input callbacks,
StrictMode activation reuse, same-ID A resurrection, independent-view coupling,
masking admitted writes, own-save target transitions, old caller callback
publication and current pending controls. Resolve through causal transport tests.

## Results

Implemented and locally verified on 2026-10-06. No PR or merge yet. The
existing requirement remains active, this work order is done, its manifest is
implemented, and the bounded design is current.

| Check | Actual result / private receipt label |
| --- | --- |
| Conditional frozen install | One install, pnpm 9.15.9 / Node 24.21.0, exit 0; `install` |
| Independently authored RED | `red`: 33 checks, 15 failures / 18 passes; 12 causal failures, two fixture control failures and one superseded nested StrictMode scenario. That initial StrictMode scenario did not faithfully replay; the independent faithful RED below establishes the contract. |
| Faithful first-setup StrictMode RED/control | `strict-original-red`: original committed hook fails refusal after two setups/one cleanup; `strict-replay`: fixed hook passes that same faithful scenario with admitted/live controls. Own source restored with hash verified before continuing. |
| Speculative callback RED | `red-speculative`: uncommitted suspended B could invoke A's old activation; the private helper now returns an inactive producing owner before commit. |
| Required seven-suite selection | `final-tests`: 70/71 passed, one fixture failure from nested StrictMode placement. All five adjacent suites passed 38/38; affected repair runs below supersede the old new-suite fixture. |
| Final causal hook suite | `regression-final`: 29/29 hook checks (plus then-current 12/12 dialog checks). Includes A-B/A-B-A, retained inputs/actions, actual unmount, pre-passive layout retirement, Strict replay, speculative render, shared/separate providers, GETs, pending ownership and current outcomes. |
| Final real section/dialog suite | `dialog-final-green`: 15/15, real section/dialog/hook/providers/API; desktop/phone, close/reopen, unmount, B replacement success/rejection, committed caller replacement, first/existing/provider-switch save, removal success/failure/retry. |
| Changed lint | `lint` production/helper/adjacent files pass; `lint-tests` passes repaired test grouping; `lint-dialog-final` passes final additional consumer cases. No warning bypass. |
| Changed formatting | `format-check` and affected `format-dialog-checked` pass. Formatting repairs changed only owned files. |
| Project typecheck | `typecheck` and final consumer-extension `typecheck-final` pass with normal release/changelog prehook; no generated diff retained. |
| i18n | `i18n-check` and `i18n-ratchet` pass; no new copy. The unstaged ratchet sees two modified source files; the normal staged commit hook will also check newly added source. |
| Documentation | `docs-validate`: 357 decisions/1,411 specs; `docs-lint-tests`: 36/36; `docs-lint`: all files pass; `docs-catalog`: owner/design discovered; `docs-links`: 14 links, eight ACs, one work order; `docs-coverage`: actual 11-path diff covered, `errors: []`; `whitespace`: pass. Status/results changes receive affected documentation checks before commit. |

All receipts use `/tmp/kandev-child59-<label>.json` with UTC/argv/cutoff/log,
PID/PGID/native handle and ACTUALLYJOINED/PID-gone/group-gone evidence; raw logs
are private siblings. Every original completed before the next heavy command.
Earlier own fixture repairs fixed fake-timer setup, Radix tab dispatch,
asynchronous confirmation admission and test lint/format structure. These
failures were not advertised as independent causal proof. No environment setup/resource/
timeout/transport/unknown failure occurred; no cache wipe or foreign kill.

Admitted writes retain their original workspace/payload and bool/void outcomes;
retired UI effects and reloads are suppressed. Dialog close alone retires its
caller, while the still-current hook may finish and refresh normally. Current
own save/removal config resets preserve dismissal. Pending tickets do not order
configuration acknowledgements. The full app route is source-audited and its
existing test passes; new transport regressions do not claim full-route E2E or
actual ancestor B overwrite.

Public-doc and mobile audits remain accurate: no new copy/layout/API/operator
steps or screenshots. The state-only mobile exception uses real component
controls at widths 390/1024. Root/scoped AGENTS and provider/authorization owners
need no edit. Protected ROOT candidates/receipts remain read-only and untouched.
Ready PR publication and hosted evidence follow the delivery barriers above;
merge requires a separate ROOT serial lease and interrupt.


## Reviewed corrective checkpoint

ROOT accepted one current-head regression at `2fb6aad5`: successful removal
dismisses a dialog whose target changed while DELETE was pending. Task01 is
reopened only for causal real-consumer coverage and the smallest success guard
that distinguishes a genuine target change from removal's own config/reset.
The first hosted observer joined at its deadline with no verdict; its process
groups are gone. Acceptance receipt:
`/tmp/kandev-root-child59-observer-deadline-finding-acceptance-20261006.json`.

Exclusive local-heavy lease granted; merge lease absent. Run only affected dialog
and real-section regressions, changed lint/format, project typecheck/i18n,
documentation and actual coverage checks, then normal hooks and corrective push.
Resolve both accepted review threads individually. After clean aligned
heavyRETURN, ROOT authorizes exactly one new-head 90-minute all-terminal observer
under GNU 91-minute timeout/10-second kill, retained and joined before any
successor. No verdict on deadline or lost results; serial merge remains a
separate ROOT interrupt.


Corrective result: four independently authored real-section/dialog cases failed
causally at the committed review head, after admitted DELETE and genuine form
or background-GET target changes, on phone and desktop. Receipts
`correction-target-red` and `correction-read-red` each show two causal failures.
The minimal private pre-reset read callback snapshots the confirmation generation
without changing the removal boolean, transport, refresh or reset semantics.
Only success dismissal gains the missing genuine target check; failure retry
and open/controller retirement retain their existing guards.

`correction-dialog-green`: 28/28 in the two affected dialog suites, including
current reset/dismissal, removal retry, first-save/provider-switch and retired
closed/reopened controls. `correction-lint`, `correction-format-write`,
`correction-typecheck` and `correction-i18n` pass. All these originals joined
and their PID/groups are gone. No unchanged hook or adjacent suite was replayed;
no reinstall, backend or E2E check was run. Public guide, copy, API, layout and
mobile data-only audit remain unchanged. The four cases use real hook, providers,
section/dialog and API; only fetch transport and actual reload boundary are
mocked. Their matching existing control preserves current successful removal
and its own reset. Detailed original receipts remain private under
`/tmp/kandev-child59-correction-*.json` and `.log`.


Corrective documentation checks: `correction-docs-validate` validates 357
decisions/1,411 specs; `correction-docs-lint` passes all specs;
`correction-docs-links` checks 14 links, eight ACs and the one order;
`correction-docs-coverage` reports actual changed paths covered with `errors: []`;
`correction-whitespace`, `correction-format-check` and `correction-ratchet` pass.
The unchanged 36-test spec-linter suite was not replayed. Final normal hooks and
publication receipts are maintained in the live task plan.
