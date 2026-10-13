---
id: "13-e2e-runner-processes"
title: "Reduce E2E execution within existing runners"
status: in_progress
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
