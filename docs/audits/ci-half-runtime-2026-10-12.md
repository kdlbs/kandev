# Halving E2E execution time

Status: implementation and verification in progress.

The user clarified on October 13 that the 50% target applies to E2E.
The reference remains draft PR #4440 head
`dc36fe3f709ebe35bce51dbdb203c66ae213f4eb`: its longest normal E2E test step
took 2,159 seconds, giving a target of 1,079.5 seconds. Frontend improvements
recorded below are useful separate work and do not count toward this target.
Workflow elapsed time and runner-minutes are separate measures; queue
variability must not be presented as an execution improvement.

PR #4440 merged as `b706496d5b2e54e1cebc1e13f38b516e041d8ec1`.
The E2E target remains open. The follow-up starts with the measured shared-PTY
cleanup bottleneck in [the implementation plan](../plans/shared-pty-close/plan.md),
then audits repeated E2E journeys without removing distinct regression coverage.

## Merged-parent E2E result

[Run 38388102356](https://github.com/kdlbs/kandev/actions/runs/38388102356),
attempt 1 at head `57ee8042d739574f173bff434273c2a49681f598`, passed before
PR #4440 merged. Its longest normal test step was 2161 seconds (36m01s),
compared with the fixed 2159-second reference. E2E execution is effectively
unchanged, and the 50% target is not achieved.

The 14 normal test steps accumulated 27316 seconds versus 27739 seconds in
the reference (about 1.5% lower). Workflow elapsed fell from 4084 to 3033
seconds, but the latest normal shard began 683 seconds after the trigger
instead of 1643 seconds. That improvement is largely queue/prerequisite
timing and must not be attributed to faster tests. These accumulated test-step
seconds exclude setup and container jobs; they are not total billed runner time.

The follow-up local measurements and coverage map are tracked in the
[PTY plan](../plans/shared-pty-close/plan.md) and
[workflow-switch audit](workflow-switch-consolidation-2026-10-13.md).

## Cost evidence

E2E run 38306765560 executed 3,881 tests, with three retries and no final
failures. Summed final-attempt duration was 28,579 seconds. In its slowest
normal shard, setup hooks consumed 260 seconds of 2,108 summed test seconds,
and leaf navigation steps consumed 378 seconds. Setup alone cannot halve it.

At the same head, frontend worker-phase sums were 1,758 setup seconds,
2,176 import seconds and 726 test seconds. Worker phases overlap, so these
cannot be added to derive wall-clock savings.

## Reviewed locale setup

[The per-file ledger](frontend-test-setup-2026-10-12-second.csv) records 897
additional explicit assignments from full locale setup to the English browser
project and two prior browser assignments returned to full setup, with unchanged
source hashes. Node assignments remain 468; browser assignments become 2,296;
100 files retain full setup at this 2,864-file snapshot.
New files still default to full setup. Isolation and worker budgets are unchanged.

The previous graph treated `lib/plugins/plugin-translations.ts` as requiring
all application catalogs. Its resource operations are confined to the
`plugin-<id>` namespace and catalogs supplied by the caller. The registry's
language-change subscription only notifies consumers; importing it neither
activates another language nor requires host catalog preload. The English
setup retains the real i18next instance, English catalogs and plugin resources.

The review traced each candidate's relative, alias, workspace, literal dynamic,
re-export and mock/importActual dependencies. There are no candidate locale
activation/resource calls outside the reviewed i18n definitions and plugin
resource owner, or unresolved computed imports. Shared date-formatting and
UI locale readers observe the initialized English locale. Monaco and CodeMirror
language identifiers, storage/resource models, LSP preferences and API caches
do not depend on translation catalog preload. All 89 distinct hinted modules
were classified at their owner boundary. Five candidates with direct catalog,
DOM-language, mocked-language or plugin-host multilingual hints stay on full
setup, along with actual locale activation and unresolved graphs. CodeMirror cursor
navigation also retains its original full project. Two icon-mocking suites,
context-chip and file-item, return to that unoptimized project. Their native
module-identity and mock-interop requirements were exposed by full execution;
none of their assertions changed.

## Loading experiments

A fixed 72-file sample contains 606 passing tests and four skips. The initial
unmodified configuration took 43.408 seconds including command startup; the
second baseline took 41.645 seconds. Icon dependency optimization initially
failed because its experimental cache was outside the web package and could
not resolve React. Moving only that scratch cache under web/node_modules
restored all outcomes (37.744 seconds). That failed run is not speed evidence.

The VM-thread experiment retained outcomes but took 48.152 seconds and was
rejected. The thread pool remains unchanged. A cold Node compile-cache run took
33.942 seconds; an explicit five-package optimizer sample took 33.992 seconds.
The combined configuration and reviewed locale assignments took 30.772 seconds
(27.18 seconds reported by Vitest). These sample results do not establish a
whole-suite or hosted percentage.

The full optimizer candidate took 744.93 seconds versus the existing 1,350.39
second full baseline, but failed: two CodeMirror assertions saw duplicate state
module instances, and two icon-mocking suites failed to collect five tests.
All 2,864 files remained selected. The seven absent/changed outcomes were
accounted for exactly; this failed run is not an accepted speed improvement.
A focused comparison failed with either locale selection when optimization was
on, and passed with optimization off. Excluding icons repaired the mocks but
excluding CodeMirror did not repair its identity failure. Global dependency optimization
was therefore rejected rather than changing tests or production code.

The replacement experiment parses JSON with Vite's supported `json.stringify`
option and disables generated named exports. Current catalog consumers use
default imports. The same 72-file sample plus the three regression files passed
617 tests with four skips in 33.240 seconds; unoptimized default JSON handling
took 38.526 seconds on that 75-file sample. The locale, environment, worker,
selection and regression guards then passed 16 files and 147 tests. The unbundled full experiment was stopped after more than ten minutes, with
1,510 files and 14,341 tests completed without failures, because its remaining
selection made the half-time target implausible. It is not a complete correctness
or speed result. JSON module format is only changed in Vitest config.

A narrower experiment applies the five-package optimizer only to explicitly
reviewed browser suites. The three sensitive files use the unoptimized full
project; Node suites and unknown new suites remain unoptimized too. That
75-file sample passed all 617 tests and four skips in 31.870 seconds. Complete verification of this scoped configuration passed all 2,864 files,
25,554 tests and the same four skips. Exact file/title/outcome multisets match
the existing full baseline. Vitest time fell from 1,350.39 to 731.89 seconds,
45.8% lower locally. This remains short of the local half-time reference and is
not a hosted Actions result. The ledger explicitly
records the two reversals from earlier browser assignments.

The workflow uses Node's content-sensitive bytecode cache in runner.temp; it is
not a test result cache and does not skip any test. Neither worker budgets nor
isolation changed.

## Config-mode MCP journeys

The complete original config-management spec passed 21 cases in 2.6 minutes,
without retries. [The coverage map](config-mcp-consolidation-2026-10-12.csv)
maps all 21 original cases to four named journeys: workflow/cross-resource,
agent/profile/MCP, task, and executor operations. All 32 original MCP calls and persisted-result assertions remain; original case
names remain as 21 test steps.

An initial four-journey version retained a separate turn for each scenario and
passed in 2.0 minutes, down from 2.6 minutes. The next candidate batches
independent operations into seven real turns across those four journeys.
Workflow targets remain distinct; its config session exists before any deletable
workflow. Agent/profile writes are all checked before restoring model/MCP inputs
for the deletion turn. Both profile lifecycles still observe creation before
sending deletion with the newly returned ID. Moving, archiving and deleting
operate on three different tasks. No state mutation can overwrite another
scenario's expected state before its assertion.

Each journey creates one real config-chat session through the dedicated HTTP
endpoint and sends subsequent prompts through the real message.add WebSocket
action. MCP calls still traverse the mock agent, agentctl server, config-mode
tool registry and backend handlers. Sessions, canonical markers and terminal
states are correlated before assertions. A journey navigates once; passive
chat/idle/marker locators replace reload/resume recovery in this proof.
Profile name/model and MCP configuration are restored in finally blocks where
the journey changes them. The task and executor journeys do not mutate them.

The extra anchor workflow used to keep a newly created config session out of a
deletable workflow is unnecessary once that journey's session already exists.
Other created workflows remain distinct, so deleting or updating one cannot
satisfy another scenario's assertions. Minimal and all-optional-field step
creation both remain. The cross-resource workflow, profile and MCP scenario
remains in the workflow journey. Backend mode-gating, schema, validation and
security tests remain independent owners in internal/mcp/server.

The fresh config-management pair passed without retries: 21 original cases in
159.247 seconds versus four batched journeys in 40.424 seconds (74.6% lower).
All 21 scenario steps ran, and the source-level MCP call multiset is identical
at 32 calls. This is a per-file reduction, not a whole E2E pipeline percentage.
The complete frontend suite and repeated sample checks pass as recorded above.
Hosted measurements remain pending.
No production application code changes are planned in this batch.

## Raw report integrity

Reports remain in ignored `apps/backend/.build/ci-half/`. The full frontend
baseline is the existing `apps/backend/.build/e2e-next/unit-full-candidate.json`.
Their SHA-256 values are:

- Baseline full frontend: `f6339cf62b31639f385e16e7972301ee4293062c0274764ba49cd2d6d8b1a20e`.
- Scoped full frontend: `41678c71801fcc151de73546a69ed011ab99a2c99247391018dbea5b9ed933cc`.
- `config-baseline-measured.json`: `7d2007d0a47d022b57ee9fe06b747595b1f43369390eedb40086a500b136a17c`.
- `config-batched-measured.json`: `9f3b2c4858ee94afa70b7d027dcc03048a8355b296006aee41c5a923a1cd9194`.

## Repeated sample and pool checks

Three alternating 75-file pairs all passed the same 617 tests and four skips,
with exact identity/outcome parity. Median command time fell from 39.882 to
30.506 seconds (23.5% lower). Each candidate used a fresh Node
compile-cache directory. This sample median is separate from the full-suite
45.8% local reduction.

A separate pool comparison passed with both pools but took 31.269 seconds in
threads and 37.731 seconds in forks. Forks were rejected; threads and all worker
budgets remain unchanged.

## Final local verification

- Full Playwright discovery: 3,971 configured cases, zero discovery errors. Only
  Chromium loses configured instances (2,730 to 2,713); mobile Chrome 1,059,
  auth 37, routing nine, containers 152 and Kubernetes compatibility one remain.
- Web typecheck, changed-file ESLint and Prettier pass.
- Frontend workflow contract: all nine tests pass. Documentation reference
  validation passes, and `git diff --check` is clean.
- Both complete reports and all six sample reports retain every frontend test
  identity and outcome. All 21 original config case names appear as executed,
  passing steps in the candidate JSON report.

No test was made optional, no retry count increased, and no worker budget or
isolation boundary relaxed. The 50% hosted execution target is still open.

## Port-forwarding setup consolidation

A further pass at `b81c562` compared the original 17 cases in
`session/port-forward-dialog.spec.ts` with nine keepers. All 17 original
scenario titles appear either as unchanged tests or executed named steps; the
[coverage map](port-forward-consolidation-2026-10-12.csv) records each owner.
The local guarded one-worker, zero-retry comparison passed both selections:
145.105 to 86.923 seconds, **40.1% less time for this file**. Neither run skipped
or retried a case. This is not an estimate of whole-suite savings.

The local-executor journey now checks default-hidden, enable and reload
persistence together. The remote-executor journey checks default-hidden,
enable and disable together. One remote dialog journey retains open/discovery,
manual badge, invalid input, duplicate input, exact session/port proxy URL and
Enter submission. Each manual-input scenario uses its original port value.
Only repeated session creation, navigation and already-established dialog
visibility checks disappear.

Independent cases retain actual tunnel survival across visibility changes,
delayed tunnel hydration, active-first ordering, failed stop, keyboard focus,
and opening a proxy in an existing versus a newly created Browser panel.
`mobile-port-forwarding.spec.ts` retains its drawer, tap, touch-target,
scrolling and viewport proofs. No shared fixture or production code changes.
The reviewed owners are `port-forward-dialog.tsx`, port visibility helpers and
the discovery, active-first and proxy/Browser-panel requirements. The previous
source history includes executor parity (#2704), active-first ports (#4265),
and durable continuity (#3598); their distinct boundaries remain covered.

A temporary mutation removed the input range guard while retaining numeric
parsing. The combined journey failed at the original invalid-port assertion:
port 99999 was visible. The production source was restored byte for byte and
the normal E2E application rebuilt before final verification.

## Further rejected loading experiments

Another fixed 75-file sample compared the accepted setup with additional
Markdown, drag/query and translation dependency bundles. Markdown bundling
failed seven suites at collection (`util.deprecate` was unavailable). Drag/query
and translation bundles passed identical outcomes but took 28.33 and 27.88
seconds versus 27.04 seconds without those additions. A separate React/testing
library bundle pair took 28.11 versus 27.48 seconds, too small a difference
relative to observed run variation to justify broader bundling. None of these
experiments changes the committed configuration.

The slowest reference normal E2E shard recorded no recovery reload or automatic
resume click from `SessionPage.waitForLoad` or `waitForChatIdle`. Its explicit
reloads verify persistence or reconnection, so removing recovery fallbacks
would not explain away that shard's runtime. Likewise, the million-event
protected-gap loop scans candidates beyond the pinned frontier; stopping at
the first stationary cursor would remove the retained-tail proof. Both remain
outside the measured reductions.

After restoring the range guard and rebuilding, all nine port-forwarding
keepers passed again in 85.420 seconds, without retries or skips. Changed-file
ESLint and formatting passed. Complete Playwright discovery found 3,963
configured cases with no collection errors (Chromium 2,705; mobile 1,059;
auth 37; routing 9; containers 152; Kubernetes compatibility 1). This follow-up
removes eight additional Chromium instances; all other project counts remain
unchanged. The whole-branch discovery reduction is 4,113 to 3,963 (150 fewer
configured instances), which is not a runtime percentage.

## Hosted frontend result at b81c562

[Actions run 38322865444](https://github.com/kdlbs/kandev/actions/runs/38322865444)
passed. Its Run tests step took **1,091 seconds (18m11s)**, versus 2,008 seconds
(33m28s) at the continuation reference: **45.7% less execution time**. It is
87 seconds above the 1,004-second target. Workflow elapsed fell from 2,595 to
1,448 seconds (43m15s to 24m08s, 44.2% lower); the main job occupied 1,421
seconds. These are separate wall-time measurements, not a median.

The earlier draft reference took 2,291 seconds, so this run is 52.4% faster
than that earlier point. That does not satisfy the stricter continuation
reference or establish a 50% whole-pipeline reduction.

The hosted suite passed 2,877 files and 25,668 tests with four skips, compared
with 2,872 files and 25,613 passes/four skips in the reference. GitHub tested
merge commit `59130a7012857ce21a0ef5dfb61adcb761688ec3`, combining this branch
with advanced base `371d9c18193af5893fbe70387fee9330f7dbb65b`. Log comparison
finds no removed files: five upstream additions and two common files with
additional cases account for the difference. Local same-source identity
comparisons above remain the controlled coverage evidence. Runner class,
worker budgets and test isolation remain unchanged.

## One capacity episode, two browser observers

The desktop and phone continuity files previously ran the same real backend
capacity-exhaustion sequence independently. The phone project now starts both
a Pixel 5 context and an explicit Desktop Chrome context against one episode.
Both observe the retry card and usable chat after exhaustion. The original
desktop error row, model selector, absent recovery controls, feedback count,
single persisted error, unchanged execution identity and six-prompt ACP trace
assertions remain. Phone overflow and composer assertions remain. Independent
mouse and touch cancellation tests still exercise their respective controls.

The real five-retry ladder (5, 10, 20, 40 and 60 seconds) is unchanged. A single
episode also checks that connecting a second viewer does not duplicate retry
dispatches. No production clock or retry budget changes, skipped case, shared
cross-test browser state, or mocked recovery response is introduced. The
secondary fixture depends on the existing per-test reset and closes its
context in `finally`; the original fixture disposes its task and profile.

The original desktop cancel/exhaust case and phone exhaustion case passed in
157.174 and 143.732 seconds. Their replacements passed in 12.819 and 146.741
seconds: **300.906 to 159.559 seconds combined, 47.0% lower** in fresh guarded
one-worker commands, with zero retries or skips. This is a selected cohort,
not whole-suite timing. The first candidate exposed Playwright inheriting the
phone project's device defaults for the secondary browser. Explicit Desktop
Chrome options corrected that; fine/coarse pointer and desktop width
assertions guard the two actual environments.

The reviewed contract is transient-turn-runtime-continuity, especially
AC-PLATFORM-TURN-CONTINUITY-003.3/003.4 (bounded retries and duplicate
viewer safety), with the existing capacity-continuation and durable-runtime
history preserved. See the [assertion ownership map](capacity-consolidation-2026-10-13.csv).

Both complete continuity files passed after the fixture correction: four
desktop cases in 48.881 seconds and four phone cases in 170.745 seconds, with
zero retries, skips or failures. Web typecheck, changed-file ESLint and
formatting passed. The hosted frontend file comparison retains all reference
files; its five additions are secrets initial-error/global-error, terminal
settings save, legacy session-stopped banner and delivery-recovery view.
The PR-archive-banner and composer-props suites also gain upstream cases.

## Clarification dismissal and restoration

Four desktop cases independently launched the same three-question blocked
conversation to check local Escape dismissal, unchanged agent state, normal
answering after restoration and preservation of a partial answer. One journey
now exercises all four as [named steps](clarification-consolidation-2026-10-13.csv):
dismiss from the second carousel step, restore and dismiss from chat focus,
restore and choose PostgreSQL, dismiss/restore with that answer, then complete
and submit PostgreSQL/Go/Docker. All original visibility, progress, selected
answer, idle composer, request-count and session-state assertions remain.
The 300ms no-rejection and 500ms no-resumption observation windows remain.

The original four passed in 30.353 seconds; the keeper passed in 12.196 seconds,
**59.8% lower for this cohort**, with no retry or skip. All four original titles
were verified as executed steps in the report. Unit coverage of keyboard scope,
Escape guards and local collapse remains independent; this journey still uses
the real clarification endpoint and blocked mock-agent conversation. Detached
questions, skips, focus conflicts, races, submitting guards and mobile tests
remain independent. Reviewed owners are ClarificationPanelSection and
ClarificationInputOverlay, with the clarification response reliability and
active lifecycle contracts preserving local dismissal versus explicit Skip.

A temporary mutation removed the keyboard Escape dismissal callback. The
keeper failed because the overlay stayed visible. The file's existing retry
policy also retried that deliberate failure; it failed again. Production source
was restored byte for byte and the normal E2E application rebuilt before the
complete file verification. No production change or retry-policy change is
included.

After restoring the owner, all 29 clarification cases passed in
235.678 seconds, without skips or retries. Changed-file ESLint,
formatting and documentation reference validation pass. Complete discovery
now reports 3,960 configured instances: Chromium 2,702; mobile 1,059; auth 37;
routing nine; containers 152; Kubernetes compatibility one. Relative to the
whole-branch starting inventory, 153 repeated/obsolete configured instances
have been removed; no runtime percentage is inferred from that count.

## Follow-up E2E run (2026-10-13)

PR #4457's first E2E run, `38405876470` at `f5466aafe8`, passed all normal and
container shards. The longest normal test step was 2,126s (35m26s), versus
2,161s on the merged parent and the fixed 2,159s original reference. This is
only a **1.53% reduction against the target reference**; 50% is not achieved.
The sum of 14 normal test steps fell from the merged parent's 27,316s to
25,078s (8.19% lower). Workflow elapsed increased from 3,033s to 3,454s
(50m33s to 57m34s) as the last normal job started 1,448s after the trigger,
versus 683s on the parent. Do not count queue changes as execution savings.

That run predates the portable poller correction and the Git/Quick Chat
consolidation. The first head's added native macOS PTY check failed because
its master file had no Go poller registration; `58eb30d5be` adds explicit
registration and a reproducing regression. Its native check remains pending
until the next push. The later local two-file audit preserves all 50 scenarios
in 44 cases and measures 381.425s to 316.745s (16.96% lower), including repair
of a branch-cleanup ordering defect that previously triggered a 30-second
reset failure and backend recovery. See the dedicated Git/Quick Chat ledger
for assertion mappings, mutation evidence and measurement scope.
