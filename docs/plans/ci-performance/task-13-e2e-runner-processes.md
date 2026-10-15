---
id: "13-e2e-runner-processes"
title: "Reduce E2E execution within existing runners"
status: done
wave: 9
depends_on: ["10-profile-e2e-fixture-costs"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-005
  - REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-001
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-005.1
  - AC-PLATFORM-CI-PERFORMANCE-005.5
  - AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-001.1
system_design:
  - ../../specs/platform/system-design/ci-performance.md
  - ../../specs/platform/system-design/e2e-duration-aware-sharding.md
---

# Task 13: Reduce E2E execution within existing runners

## Summary

Measure and, only if useful, enable bounded independent Playwright processes
inside existing normal E2E jobs. The maintainer's target is a 50% reduction in
the longest normal E2E test step from 2,159 seconds to at most 1,079.5 seconds.

## Scope and boundaries

Own planned-shard execution, stable fixture port slots, resource selection,
complete reports and normal-job wiring. Preserve every selected assertion,
fourteen normal jobs, six container jobs, one worker per process, retries,
timeouts, build identity checks and required aggregate behavior. Container
execution remains serial. Do not change provider allocation or add capacity.

## Acceptance

1. A same-code, same-selection comparison retains every test identity and
   records failures, retries, elapsed time, process count and memory evidence.
   Local evidence supports candidate selection; complete hosted runs decide
   the full-suite target.
2. Partition and supervisor tests prove exact selection, whole-file grouping,
   stable ports across worker replacement, disjoint reports, joining all
   children and failure propagation. A missing report fails closed.
3. Existing timing/merge consumers accept all reports and exactly one metadata
   record per original job. Full CI passes; its longest E2E step, summed step
   time and workflow elapsed time are reported separately.

## Verification

Run from the repository root, with no overlapping local E2E suites. Run the
managed resource guard unchanged; a denied process count is not bypassed.

```bash
(cd apps/web && pnpm exec vitest run e2e/helpers/backend-ports.test.ts e2e/scripts/planned-subshards.test.ts e2e/scripts/planned-processes.test.ts e2e/scripts/planned-processes-lifecycle.test.ts e2e/scripts/run-planned-shard.test.ts e2e/scripts/resource-guard.test.ts e2e/scripts/e2e-timings.test.ts)
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning.py
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the same queue, transcript and workflow file selection sequentially with
one, two and (only if the guard permits it) three local shards on four CPUs.
Separately exercise the planned runner with blob reports, multiple projects
and real worker replacement. Validate merged test identity and parent metadata
before enabling the workflow. Preserve failed attempts rather than selecting
only successful samples.

## Files and dependencies

- `apps/web/e2e/scripts/run-planned-shard.*`
- `apps/web/e2e/scripts/planned-subshards.*` and `planned-processes.*`
- `apps/web/e2e/fixtures/backend.ts` and `helpers/backend-ports.*`
- `.github/workflows/e2e-tests.yml` and its existing contract tests
- Existing E2E resource guard, file-duration manifest and report consumers

## Results

Candidate investigation is in progress. The initial 86-case four-CPU serial
attempt passed 84 cases and failed Quick Chat restored-session submission and
local-rebase comparison focus. It is retained as failed evidence, not a passing
speed baseline. No parallel phase started. A fresh 42-case queue/transcript/
workflow comparison uses consistent stable-slot port allocation on both sides.
The second serial attempt passed 41/42; the workflow reuse/complete policy
case timed out while backend logs recorded persistence probe failures and
HTTP 503 responses. Observed host-available memory fell to 790,315,008 bytes
while the measured test tree peaked at 2,536,239,104 bytes summed RSS. This
shared-host attempt is not a controlled speed baseline or proof of a new
parallelism regression; its parallel phases never started.

The actual planned-runner probe passed all 14 desktop/mobile cases from four
files with two one-worker processes, zero retries/skips, stable ports across
project replacement and successful blob merging. Exact catalog counts and
unique identities matched. Sixty focused unit tests passed, including real
Unix process cancellation; removing signal handlers made that regression fail
and restoring them made it pass. Workflow contracts, changed-file lint and
web typecheck passed. The draft workflow requests three normal-job processes
subject to the unchanged resource guard and CPU cap. This is a hosted trial,
not an adopted or measured 50% improvement.

The same 14-case probe also passed with one and three processes on the same
four-CPU affinity, with identical test identities and zero retries/skips.
Parent test execution was 52.552 seconds serial and 32.822 seconds with three
processes. This single small sample supports the hosted trial, not the full
suite target. Blob merging passed in both modes. The resource guard admitted
three processes; peak RSS was not separately sampled for this short probe.

Full hosted E2E run 38416314881 at f68a9c867d74eee3406460e610577aa468acad5c
passed before concurrency changes: longest normal step 2,178 seconds, summed
normal steps 26,058 seconds. The 50% target remains open.

The three-process hosted trial at 3314a962 (run 38425537974) failed. All 3,935
first-attempt identities matched the prior complete run, but six normal shards
failed and seven tests remained failed after retries. Longest normal execution
was 1,657 seconds, summed normal execution 17,615 seconds, and workflow elapsed
time 2,481 seconds. This failed run is not a new best passing result. Among
3,354 matching first-attempt passing tests in the first artifact snapshot,
summed individual durations rose from 22,411.289 to 37,366.244 seconds, consistent
with substantial contention. The trial does not satisfy the 1,079.5-second goal.

The next candidate requests two processes with the same memory guard, CPU cap,
fourteen normal jobs, six container jobs, assertions, timeouts and retry budget.
Failed backend fixture setup now prints at most 8 KiB of its own process-log
tail after log streams close and before deleting the temporary root. This
preserves the underlying startup error without suppressing the original failure.
Its regression failed before implementation and passes afterward. Missing
frontmatter design references were repaired against the trusted PR validator;
no documentation waiver was used.

Local diagnosis retained a three-process, four-file attempt with 22 cases:
17 passed, one settings case timed out, and four managed-runtime cases failed
because this host did not expose the required OpenCode managed metadata. Those
four failures occur before the hosted failing assertion and are not equivalent
reproductions. The settings and workspace files together pass all three cases
serially in 27.840 seconds with no retries. A fresh two-process probe passed
all 26 cases from settings, workspace attachment, navigation and automation
files in 69.271 seconds, on four CPUs with zero retries/skips and two actual
processes. It excludes the host-incompatible managed-runtime metadata cases
and is not a paired speed comparison. All 69 focused runner/fixture unit tests,
17 workflow contracts, changed-file lint, web typecheck, documentation checks
and action pinning pass. Full hosted remeasurement remains pending. Do not
interpret fewer processes as a full-suite improvement until that evidence exists.

The new log capture exposed a standalone-agentctl bind failure in the
two-process trial, job 115423582334. The fixture isolated backend listeners
and per-task agentctl ranges but left standalone agentctl at its production
default 39429. Concurrent availability checks can both succeed before either
child binds. Allocate that listener at backendPort + 1000 and pass AGENTCTL_PORT
explicitly; no production launcher behavior changes. Container and SSH executor
launchers already set their own target ports explicitly.

Extend the allocation regression to include every standalone listener across
all supported offsets and slots. It fails twice before the fix and passes
afterward; 48 focused runner/fixture tests pass. A real two-process run passes
all 26 probe cases with no retries/skips in 71.347 seconds, under the unchanged
memory guard and four-CPU affinity. Both standalone listeners are observed
simultaneously at 19080 and 19081. Formatting, lint, typecheck and documentation
checks pass. This addresses the observed startup race; it does not claim to
resolve unrelated hosted failures or meet the full-suite runtime target.

The completed two-process trial at fb2f7828 (run 38432683722) also failed:
longest normal test step 1,466 seconds, summed normal steps 18,026 seconds,
workflow elapsed 2,950 seconds. All 3,935 first-attempt identities match f68.
Twelve normal shards and all six container shards passed; two normal shards
failed. Final failures are managed-runtime cancellation during retry backoff
and the mobile multi-repository Restore-first journey. Sidebar sampling passed
only after retries. These timings are not an accepted passing improvement.

The cancellation case now observes the persisted startup-retry marker before
calling the same real stop API, retaining CANCELLED state and both launch-count
checks across the full 3.5-second negative observation. Its companion still
asserts the browser retry banner. The full four-case file passes in the CI
runtime image with no retries; see the managed-runtime cancellation audit.
The exact mobile Restore-first case passes locally in 29.7 seconds, but its
hosted snapshot shows session recovery failure and a closed backend connection;
it remains an unresolved hosted observation, not a relaxed geometry assertion.

Unexpected test-body failures now also print the bounded backend log tail before
the automatic fixture cleanup, rather than capturing only fixture setup errors.
A new regression distinguishes passing/skipped/expected-failure outcomes from
unexpected failure, timeout and interruption. It fails before implementation;
all 49 focused tests pass afterward. A temporary real failing Playwright case
confirms the automatic fixture prints the backend log and preserves exit 1;
the temporary probe is removed. Fresh hosted validation remains required.

The next full two-process trial (run 38439998051) finishes with thirteen normal
shards and all six container shards passing. Longest normal execution is 1,503
seconds, summed normal execution 17,680 seconds, and workflow elapsed 2,589
seconds. Full project/file/suite/title/repeat identities account for 3,931
cases plus four executed consolidation steps against 3,935 baseline cases;
the same 40 cases are skipped. Final outcomes are 3,890 passed, 40 skipped and
one failed, with five retry attempts. Cancellation and Restore-first both pass.
The remaining mobile goal-clear assertion expires while the agent is still
running; its readiness repair and local verification are recorded in the
mobile goal response audit. This failed run does not satisfy acceptance.

A further local three-process trial is rejected. Before intentional shutdown,
64 matching first-attempt passing cases take 672.7 summed seconds versus 427.0
with two processes, and a Quick Chat detached-work cancellation case fails.
This partial sample is neither a complete timing comparison nor proof that
concurrency caused that failure. The workflow remains at two processes.

A temporary executable 2 GiB RAM-backed Docker /tmp mount is also rejected.
The same eight desktop/mobile cases pass without retries on both sides,
taking 101.692 summed seconds on ordinary storage versus 99.179 seconds on
RAM-backed storage (2.47%). One small sample does not establish a useful full
suite gain or justify the new memory bound. The runner is restored byte for
byte. No hosted capacity or storage configuration changes are adopted.

Session-dialog consolidation retains nine scenarios in six cases, with a
complete local comparison of 95.1 to 69.6 summed seconds. A separate attempt
to share runtime-summary notification setup between desktop and phone is
rejected after a reconnect failure in its complete file run. The original
notification tests and helper are restored. Both coverage decisions are
recorded in their respective audits. The longest-step target remains open;
the next complete hosted run must validate this batch before any passing
speed claim.


Run 38448127809 completes with twelve normal shards and all six container
shards passing; normal shards 6 and 12 fail. Longest normal execution is 1,520
seconds (29.60% below the fixed reference), summed normal execution 18,128
seconds. This is not a passing speed result. Executor routing uses an
ambiguous text locator and skips an expansion control before it renders.
Cancellation adds sequential assertion overhead to its two-second check;
another attempt finishes its short mock turn during page setup. The routing
and cancellation readiness audit records the focused repairs and evidence.
The complete artifact audit corrects the initial failure classification:
executor routing passes on its second retry; the mobile 50,000-file viewport
case is the third final failure. Its scroll helper correction is recorded
in the large Changes scroll audit; no size or geometry assertion is removed.

Threads consolidation retains twelve scenarios in nine cases and passes its
complete local comparison at 107.038 to 87.831 summed seconds. A persisted
hidden-column leak is reproduced by a browser regression before the fixture
repair; the repaired regression, all five preview-workflow cases and nine
Threads cases pass together without retries. Desktop and phone Quick Chat
response readiness also pass their five complete cases. These are local
results only. Fresh full hosted validation, review resolution and the
1,079.5-second E2E target remain open.

A further session/clarification batch retains forty scenarios in thirty-six
cases. Active-session deletion now carries its exact surviving-session and
same-task assertions in the Changes-retention journey. Dirty and removed-file
sibling hydration share setup while keeping separate reloads and real Git
event evidence. Clarification layout and custom-draft clearing run as named
steps in their existing journeys. The complete local baseline and candidate
both pass, with no retries/skips, at 378.013 and 336.768 summed seconds
(10.91% locally). The identity audit verifies all four moved scenarios.
See the session/clarification consolidation audit. This is a focused local
saving; full hosted validation and the 50% target remain open.

Hosted run 38472002693 exposes a separate empty-turn notice ordering failure:
the original fixture starts a three-second empty turn before navigating, but
the failed retry spends about eight seconds in navigation. A controlled
late-navigation probe reproduces the missed live hint; submitting the empty
command from an already-ready chat passes the same probe. The final real
browser journey passes three repetitions with no retries/skips and retains
the normal-output negative case as a named step. See the empty-turn live
notice readiness audit. Product behavior and assertion timeouts are unchanged.

### Git launcher overhead, October 14 continuation

The next bounded candidate removes a Node startup from ordinary POSIX Git
commands in the shared E2E fixture. It preserves the existing Node control
implementation whenever either control file exists, and checks those files on
every invocation rather than caching their initial absence. The Windows path
is unchanged. Ownership includes `fixtures/git-shim-launcher.ts`, its launcher
regressions and real-Git integration tests; fixture/shim comments explain the
new execution path. No product or public configuration changes are involved.

[The launcher audit](../../audits/e2e-git-launcher-overhead-2026-10-14.md)
records the RED, control-bypass mutation, preserved command/environment/stream
contract, rejected Go-concurrency probe and pending paired browser comparison.
Adopt only after the focused real-control and browser checks pass; the full
hosted passing-run target remains the acceptance gate.

The latest-base comparison passes all 26 unchanged test identities on both
launchers: execution 71.477s to 48.078s (32.74% lower), whole local invocation
91.922s to 68.044s (25.98% lower). An earlier original/candidate/original sequence
reproduces the direction. The audit separates a further sample that overlapped
typecheck. All 12 real-control desktop/mobile cases also pass against the fresh
latest-main build with no retries/skips. Eight launcher/integration tests,
changed-file lint/formatting, primary and merged typechecks and documentation
checks pass. This selects the candidate for hosted measurement; it does not
close the full-suite 50% acceptance gate.


### Complete launcher trial and coverage correction

Hosted run 38485034609 at 0b248b18 completes with a longest normal E2E step of
1,142 seconds (19m02), 47.11% below the fixed 2,159-second reference. Summed
normal execution is 13,298 seconds; workflow elapsed is 2,292 seconds. Eight
normal shards and all six container shards pass. Eight final cases fail and
17 retry attempts occur. This failed run is not an accepted speed result, and
its longest step remains 62.5 seconds above the 50% target.

The complete 34-blob audit accounts for 3,938 executed cases: 3,890 passed,
eight failed and the same 40 skipped identities. Comparing the complete list
with CLI selection exposes a pre-existing planner defect: 30 helper-declared
cases are attributed to helper definition files rather than their root spec
files and therefore never selected for execution. The planner now carries the
owning root spec through nested suites. The real Playwright listing regression
fails before the correction; all 35 planner, runner and subshard tests pass
with it. Restoring those 30 cases is required before final timing acceptance.

CI remediation preserves the legacy multi-repository manual-recovery branch by
holding background Git reads until the real recovery response, while disabling
automatic session start for that fixture. Both mobile and desktop journeys
pass three repetitions each (12 cases, zero retries). The routing failure is a
real browser race: a delayed action response overwrites a newer failure in the
same route generation. Six component regressions and both real desktop/mobile
fallback journeys pass after preserving the newer session projection.

The startup-retry case retains persisted state, launch counts, file survival
and final UI assertions; transient banner rendering is already asserted by its
component test. All four browser cases pass. PR automation fixtures now declare
the mock repository visible to their automation account. The markdown comment
journey waits for the actual new-file Changes update before selecting Files.
Remaining browser validation and full hosted acceptance are still in progress.


All 30 restored helper-declared browser cases pass on desktop and mobile with
one worker and no retries or skips. A separate real CLI listing of every
planned shard selects exactly 3,816 normal and 152 container identities, with
no missing or duplicate cases relative to the full catalog.


The live tool-completion regression exposes a database timestamp defect rather
than a slow render: the turn-end completion sweep writes SQLite
`CURRENT_TIMESTAMP`, truncating the new tool revision to whole seconds. History
recovery then treats completed rows as older than their pending live revisions.
A repository regression fails before changing the sweep to a UTC application
timestamp and passes afterward with the race detector. SQL guard and the
persistence conformance suite pass. PostgreSQL and fresh-build browser checks
are recorded with the final validation below.

Main advanced to 8d163e959a62a5766a0e6922473bef99f0157c82. The two Quick Chat
cancellation test conflicts retain the new main-side causal message waits and
this PR's independent desktop test execution with normal configured retries.
The focused checks below validate the resolved merge before its normal hooked commit.


The timestamp regression now passes against both SQLite and isolated real
PostgreSQL with the race detector. The live browser completion case also passes
on the rebuilt merged source. The PR automation fixture additionally keeps its
initial turn active, matching the existing mobile fixture, so the browser
journey tests queued repair and provider transitions without an undispositioned
mock repair completing and triggering another auto-fix round. The markdown,
completion and desktop/mobile requeue cases pass three repetitions each:
12 passed, zero retries/skips. All 53 focused component/helper/planner/runner
unit tests pass. Public docs need no change: the existing timestamp and routing
contracts are preserved; their implementation invariants are clarified in the
owning system designs.


The unchanged backend storage test passes its single targeted hosted retry;
the completed old-head CI snapshot has 44 passed, 15 skipped and seven failed
checks, all failures belonging to the E2E workflow already under remediation.
The previous waiter exits 1 after 26m35s with those known failures and the base
conflict; it is not left polling. Main then advances to 2f501583ff through the
native first-prompt readiness fix. Fresh hosted validation must use the actual
new-head merge with that current base; old passing checks are not reused.


The complete affected PR/Quick Chat file run passes 17 of 18 cases and exposes
a missing mock-provider check in the mobile automatic-merge retry fixture.
Seeding the actual successful provider check makes that case pass three times
without retries. The subsequent six-case mobile file passes five cases and
exposes an existing conflict-repair fixture race: a mock repair completes
without an outcome and legitimately starts another round. Both desktop and
mobile cases now keep the repair active while checking duplicate feedback;
all six repeated cases pass without retries. Assertions and test budgets stay
unchanged. All seven desktop PR cases and five resolved Quick Chat cases
already passed in the complete-file run. These results are focused validation,
not a claim that one complete hosted suite passed.

Web typecheck, zero-warning focused ESLint, formatting, changed-Go lint,
SQL guard, persistence conformance, specification validation, and harness
validation pass. The next acceptance gate remains the complete hosted run,
including restored helper-declared cases and current-base integration.


## Remaining E2E gap and focused consolidation

The next complete hosted measurement executes all 3,971 expected identities:
3,930 passed, one failed, and the same 40 skipped identities; five retries
remain counted. The longest normal step is 1,136 seconds (18m56), 47.38% below
the 2,159-second reference, still 56.5 seconds above the target. The normal
step sum is 13,027 seconds and total workflow elapsed is 3,834 seconds,
including substantial runner queue delay. The slowest shard has 282 passes
and no retries, so removing failed-attempt time alone cannot close this gap.

The sole persistent failure is the phone startup-retry progress notice: a
successful replacement can start before the browser renders the notice. The
fixture now holds that real replacement until the existing visible/in-viewport
assertions finish, with a bounded release gate and cleanup. Both new shell
fixture regressions fail before the hold and pass afterward; three repeated
phone journeys pass with no retries. The original same-session, attempt-count,
message, screenshot and overflow assertions remain.

The PR popover and transcript-scroll audit consolidates 29 standalone cases
into 13 without dropping a scenario: 10 stay standalone and 19 run as named
steps. Under identical one-worker limits, all cases pass before and after;
elapsed time falls from 183.174 to 115.903 seconds (67.271 seconds saved).
The first consolidation attempt missed a workflow action URL in its fixture;
the final version preserves that URL and output and passes every original
assertion. See `docs/audits/e2e-popover-scroll-consolidation-2026-10-14.md`.
This subset measurement does not prove the complete hosted target. Final
current-base validation, normal hooks and a fresh hosted run remain required.


Final local validation passes all six desktop/phone startup-retry cases with
zero retries, including cleanup of the held process. The 13 consolidated cases
pass and their blob step inventory accounts for all 29 original scenarios.
All 54 focused unit tests, web typecheck, formatting, zero-warning ESLint and
documentation checks pass. Main's intervening change is limited to literal
executor-placeholder expansion; validate that owner on the final merge tree.


### Hosted target reached; isolate no-progression scrolling before delivery

Run 38500882345 passes every normal/container shard. Longest normal step:
17m29s versus 35m59s, a 51.41% reduction; summed normal steps 12,823s.
All 3,955 expected cases execute once before retries, with 3,915 passed and
the same 40 skipped identities. Five first retries pass. Both phone startup
retry cases pass first attempt. The full E2E workflow passes in 50m46s including queue, build and report.

The consolidated scroll keeper is one of those retries. Restore clean-history
isolation for the no-progression pair while retaining all assertions; the
whole two-file audit then has 14 cases and all 29 scenarios. This gives back
one setup rather than making an unsupported claim that the live-turn and
no-progression preconditions are interchangeable. Repeat the entire scrolling
file without retries and remeasure the subsequent head in full CI.

The restored scrolling file passes all 21 cases over three full-file repetitions with retries disabled on current main. Transcript owner tests (35), current-main rich-output owner tests (28), typecheck, formatting/lint and documentation checks pass. The original shard passes 280 cases with the same three skipped identities, zero retries, omissions or duplicates. Next-head CI remains pending.

The old-head frontend test job was cancelled after its test step exceeded 45 minutes (prior run: 16m42s). Normal cancellation completed after five minutes; no force-cancel was issued. GitHub did not publish that job log, including in the full workflow archive, so the cause is unconfirmed. The next head requires a fresh passing frontend run.

### Review: attribute helper timings to their owning specs

Review comment 4266653399 identified that catalog discovery retained the owning
spec, but timing collection used the helper declaration's filename. Carry the
root suite's file through nested suites so future profiles can inform those
specs' shard estimates. Keep declaration-location fallback for reports without
a suite location.

The regression reproduces two specs sharing one nested helper: before the fix,
their timing entries collide; afterward, each owning spec receives its own
measured estimate. All 51 focused timing/planner/runner tests, zero-warning
ESLint and web typecheck pass. Replaying all 34 blobs from run 38500882345
reattributes 30 observations across ten owning specs; all 3,960 attempts retain
their result, retry identity, duration and attachments. This changes timing
collection only, with no test selection or assertion changes. Final-head CI
and review resolution remain required before merge.

## Final hosted result

PR #4457 merged as `0b208ce00a0ae00955696381006364dbc3b7424a` on
2026-10-14. Run 38510354669 retained all 3,958 selected identities (3,918 passes,
40 unchanged skips, seven recovered retries) across fourteen normal and six
container jobs. The longest normal test step was 1,048 seconds against 2,159
seconds baseline: 51.46% less. Whole E2E elapsed time was 1,994 seconds; this
result does not establish the subsequent fifteen-minute full-CI target.
