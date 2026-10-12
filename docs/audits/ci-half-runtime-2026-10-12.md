# Halving test execution time

Status: implementation and verification in progress.

The user requested a 50% time reduction after draft PR #4440 reached
`dc36fe3f709ebe35bce51dbdb203c66ae213f4eb`. That head is the reference:
frontend Run tests took 2,008 seconds; the longest normal E2E test step took
2,159 seconds. The respective targets are 1,004 and 1,079.5 seconds. Workflow
elapsed time and runner-minutes are separate measures; queue variability must
not be presented as an execution improvement.

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
