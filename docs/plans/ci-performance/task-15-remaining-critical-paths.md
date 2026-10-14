---
id: "15-remaining-critical-paths"
title: "Shorten remaining critical paths"
status: in_progress
wave: 11
depends_on: ["14-partition-independent-checks"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-007
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-007.1
  - AC-PLATFORM-CI-PERFORMANCE-007.2
  - AC-PLATFORM-CI-PERFORMANCE-007.3
  - AC-PLATFORM-CI-PERFORMANCE-007.4
system_design:
  - ../../specs/platform/system-design/ci-completion-budget.md
---

# Task 15: Shorten remaining critical paths

## Outcome and scope

Implement the repository-only native inventory launcher described in the complete
CI design. Separate the large repository package into three normal and three
PostgreSQL cohorts; retain ordinary package jobs and the explicit PostgreSQL
catalog. Required gates join all cohorts. Then inspect E2E setup/fixture/report
intervals and update this work order before changing E2E execution.

## Exclusions

No application behavior, paid capacity activation, trust expansion, assertion
removal, automatic retries or delegation. Preserve unrelated user changes.

## Acceptance

Preserve explicit database/platform catalogs and complete test evidence. No blanket test removal or resource-guard bypass. Candidate choice follows observed bottlenecks.

## Files and dependencies

The backend/frontend/E2E workflow files, their registered contract tests and
CI performance evidence. Task 14 supplies the preceding measured result.

## Verification

```bash
python3 .github/scripts/repository-test-shard_test.py
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

Baseline run 38524205425 repository JSON contains 1,571 passing top-level tests,
769.6 seconds summed test duration and 771.3 seconds package elapsed. Retention
names account for only 147.9 seconds, so a retention/rest split would remain
unbalanced. Three round-robin native cohorts distribute both existing and new
names without maintaining a guessed list. PostgreSQL logs show 1,047 seconds
for the same package, with no single top-level test dominating. Hosted
acceptance remains pending.

## E2E candidate selected before implementation

Using successful main profile run 38554759444 against the current catalog,
1,336 normal project/file units retain all 3,808 tests. With two guarded
processes per hosted job, predicted slowest process work is 1,027.8 seconds
at 14 jobs, 685.8 at 21 and 514.7 at 28. Twenty-one jobs leave insufficient
headroom for build/setup/reporting, so measure 28 normal jobs; keep six container
jobs and every other platform unchanged. These are model estimates, not results.

Keep all normal manifest and report completeness checks synchronized at 28;
reject old-count, duplicate and incomplete metadata. Execute independent backend
and web build commands concurrently within the existing build job, join both
original processes even on failure, and only then build the delivery fixture.
Do not change build identity, permissions, retry policy or resource guards.

Additional verification: focused plan/timing/runner/subshard unit tests; native
catalog union with both profile and fallback; E2E workflow contracts including
parallel build failure propagation. Hosted coverage and all workflow elapsed
measurements remain required before adoption.

### Local candidate verification

The real repository launcher enumerated 1,904 native top-level tests and completed
cohort 1/3: 635 selected, 536 passed and 99 environment/platform skips, in
260.377 seconds package execution. Complete inventory, selection, raw JSON and
atomic coverage are retained outside Git under
`apps/backend/.build/full15-local-repository-1/`. This is one local normal cohort,
not PostgreSQL or full hosted acceptance.

Seven launcher tests cover native enumeration, disjoint future-name selection,
missing/duplicate/unselected results, nested tests, fuzz seeds, runnable examples,
command failure propagation and a real CLI race/coverage run. Backend workflow
contracts cover required joins and exact catalog delegation. The E2E planner,
timing, process and subshard checks pass 51 cases; 19 workflow checks include
actual parallel build joins and independent child failures.

### Sidebar resource lane

The first candidate's job 115897903303 remained in its memory step after the
fifteen-minute full-workflow budget. Workflow edits select this lane, so it
cannot be omitted from acceptance. Divide its 84 isolated pool combinations
across three complementary selectors as specified in the design, preserving
preparation, maximum-input, alternating, child concurrency, native-memory bounds
and the independent populated database timing lane. Verify actual Go nested
selection with a future sort key before adopting it.

The first memory lane completed successfully in 26m40s. Its scenario sums are
lastActivityAt 216.94s, runningFirstActivity 212.73s, state 230.58s, updatedAt
162.52s, createdAt 159.48s, title 158.17s, custom 158.31s and alternating 19.07s.
Use memory-1 for the first two sorts, alternating and the standalone memory
checks; memory-2 for state/updatedAt; memory-3 for createdAt/title/custom. This
balances observed work while retaining future sort keys in memory-1.

Both the historical-profile and no-profile native E2E plans were generated and
validated. Their exact unit IDs, file hashes and test counts match: 1,336 normal
units/3,808 tests across 28 jobs and 46 container units/153 tests across six jobs.

### Measured E2E build-path follow-up

Candidate build job 115913753173 spent 320 seconds in backend/web compilation.
The three non-host agentctl targets occupied 120 seconds (14:38:06–14:40:06
UTC), before the Linux application could build. Native agentctl took 24 seconds,
Linux/amd64 helper 20 seconds, application 111 seconds, and delivery fixture
43 seconds. A subsequent remote-helper invocation linked those same targets
again. Backend artifact upload took 20 seconds and included unused non-host
binaries. The entire job took approximately eight minutes.

Move Linux/arm64 and both Darwin helper builds plus diagnostic CLI/winjob
compilation into a separate hosted job that starts after change detection and
remains required by the final E2E gate. The Linux browser build retains kandev,
native agentctl/mock-agent, Linux/amd64 remote helpers, the delivery fixture and
plugin artifacts. Invoke existing Make targets once; do not change Makefile or
release semantics. Compact-runtime fixtures consume the Linux helper to create
their canonical test manifest; SSH/Docker/Kind targets are Linux/amd64. Preserve
all prior platform compilation in the parallel job, without making browser
shards wait for unrelated binary targets.

Verify the exact target union against the previous make expansion, mandatory
required-gate outcomes for the new job, protected hosted placement, actual
parallel child joins/failures and full hosted E2E results. Do not attribute the
120-second estimate to a delivered full-pipeline improvement before measurement.

### Ordinary Go package balance

After removing the repository package, ordinary shard 1 passed in 8m04s, but
shard 2 was still executing after the fifteen-minute full-workflow budget. Its
preceding run had process 319.7s, ACP transport 197.9s, lifecycle 197.5s,
orchestrator 190.4s and backendapp 141.9s on that same shard. Use four stable
hash buckets for the remaining package inventory, still excluding only the
repository package owned by its required matrix. Preserve all commands, race
and coverage flags, per-package deadlines and artifact/report handling. Verify
the four-way union from native go-list output, including newly added packages.

### Container fixture preparation

Second-candidate container shard 6 took 784.506 seconds in Playwright while
its test bodies totaled 324.978 seconds. Worker fixtures included Kind startup
(144.901 seconds), worker-image preparation (194.988 seconds), and three backend
starts (88.508 seconds). These costs are absent from the test-body timing profile.
The compact-runtime fixture invokes the canonical release tool to compress four
identical helper inputs at gzip level nine on every backend/image preparation.
A real local 44 MiB helper reproduced 21.412 seconds per preparation.

Prepare one canonical compact-runtime manifest per container Playwright invocation
in global setup, in a uniquely owned temporary directory. Every worker retains
its own bundle, home and helper cache, verifies the canonical bundle, and checks
the shared manifest's version, source revision and Linux helper digest against
its current inputs. Remove the shared directory in global teardown, including
failed preparation cleanup. Non-container runs do no extra work; direct fixture
callers retain standalone generation. Do not reuse across invocations or change
release-tool validation, compression or application runtime behavior.

Also omit loading intermediate worker recipe images into Kind: tests reference
only their final harness images, whose layers include the recipes. Continue
building all three real recipes, adding the current mock agent, loading each
final image and disposing all owned tags. Prove final-image coverage and cleanup
through the Docker command boundary, then run the real hosted container suite.

Before implementation, add failing tests for single manifest generation across
isolated bundles, stale identity/digest rejection, teardown/failure cleanup, and
final image loading without intermediate transfers. Run focused Vitest, changed
file lint/format, and the full hosted E2E measurement before adoption.

### Incremental E2E Go cache snapshots

Third-candidate build 115935221355 improved to 362 seconds, with 203 seconds
in backend/web/delivery compilation. Its Go cache was an exact hit on the
dependency-only key, restoring an old immutable 790 MiB snapshot and preventing
the updated compilation cache from being saved. The delivery fixture alone
recompiled for 74 seconds. Add the tested source SHA to the two E2E Go cache
keys (browser build and parallel helper compilation), restoring first from the
same role/dependency prefix, then the existing role/general fallbacks. Go still
validates every cached object against source, compiler and flags. Preserve cache
paths, permissions and artifact identity checks; no binary-output shortcut.
Measure restore/save overhead as part of complete elapsed time, including the
first snapshot-producing run. Require the workflow contract to retain the
source key and dependency-first incremental restore order.

The same immutable-cache issue affects Windows: settings job 115935187357 spent
15:19:07–15:24:08 in cold package compilation, then completed its tests successfully.
Setup-Go reported an exact hit on its dependency-only key and did not save those
new build objects. Retain Setup-Go's existing module cache, and add a narrowly
scoped GOCACHE snapshot keyed by native Go version, suite, dependency digest and
source SHA. All four process cohorts share one suite cache because they compile
the same packages; do not multiply it by cohort. Resolve paths through `go env`,
retain existing test flags/deadlines, and measure cache upload/restore overhead.
Limit this experiment to the two E2E build roles and Windows build objects:
repository cache usage is already approximately 10 GiB, so broad per-job cache
duplication would risk eviction churn. No storage limit or billing changes.

### Four-bucket balance correction

Third-candidate backend run 38587972467 passed in 1,134 seconds, but ordinary
shard 4 remained the critical job (949 seconds, test step 790). Its package
elapsed sums were 1,595 seconds versus shard 3's 298; shards 1 and 2 summed
887 and 984. Move only backendapp (244.459), ACP adapter transport (237.853)
and coordinator (180.728) from bucket four to bucket three. This makes measured
package sums approximately 932/961 seconds for those two buckets, without
additional jobs. All other native packages retain stable hashing and the sole
repository exclusion. Verify disjoint native selection and explicit placement
of those three packages, including unknown future packages. Package sums overlap
in execution and are balancing evidence, not promised wall-time savings.

### Use the existing guarded third normal process

Second-candidate normal execution had a 502-second median and 850-second tail;
the next run again has a slow normal tail after the six-minute build. Keep 28
hosted normal jobs and request three independent one-worker processes through
the existing process-count guard. That guard already allows three only with
at least 12 GiB available memory and at least four CPUs, falls back to two/one
when constrained, and always keeps containers at one. Change no guard threshold,
Playwright worker setting, retry limit or matrix count. With the same historical
workload, the idealized process estimate falls from 515 to 343 seconds; contention
and fixture repetition make this only an experiment. Verify three-way disjoint
whole-file allocation and failure joins, and retain actual process count, retry
rate, job occupancy and complete elapsed time. Reject a resource/flake regression.

### Container fixture affinity (fifth candidate)

Fourth candidate run 38595327868 placed Kubernetes files on five separate
container jobs. Every one provisions the same worker-scoped Kind fixture; the
sixth job contains only the four normally opt-in Docker-workload cases. The
slowest successful container job took 733 seconds after the shared build.
Co-locate container-project `tests/kubernetes/` units as one atomic scheduling
group, then balance the remaining Docker/SSH units with the existing LPT rule.
Keep six jobs, one worker per container job, every native identity, optional
scenario gate and original estimate. Normal-project scheduling stays unchanged.

Before adoption, prove deterministic placement and exact coverage for profiled
and fallback inputs, including future Kubernetes files and project boundaries.
Run the real combined Kubernetes files to verify worker-fixture reuse and
per-test cleanup; use hosted elapsed measurements to judge the candidate.


### Sidebar native compilation cache (sixth candidate)

The fifth hosted attempt exceeds the fifteen-minute budget while resource-memory
jobs remain active and normal E2E jobs queue behind occupied hosted capacity.
The fourth completed memory-1 job took 648 seconds; its actual Go test package
used 414 seconds, with first test execution only after 223 seconds of container,
checkout, dependency and compilation setup. All four sidebar modes compile the
same native package without a Go cache.

Add an optional shared module/compilation restore for the sidebar resource lane,
with a role/dependency key and a compatible static-check fallback. Save only from
the successful memory-1 mode so the four jobs do not race to compress/upload the
same cache. Preserve native Go cache validation, all selectors and child-process
isolation, the fixed memory bounds, required job outcomes and artifacts. Cache
misses or service failures must still run the ordinary verification. Do not add
a cache-based condition to either measurement step.

Validate the existing workflow contracts, native sidebar selection fixture,
action pins and required gates. Measure full cold/warm hosted elapsed time;
compilation reuse is a candidate improvement, not an accepted latency result.
The separate PostgreSQL timing rebalance remains an unimplemented proposal until
the fifth attempt's completed artifacts justify its weights.


### Worker-image transfer and placement (sixth candidate)

The fifth grouped Kubernetes job passed but took 842 seconds, leaving the E2E
execution critical path at 1,261 seconds before queue delays. All three prepared
worker recipes share the same pinned Kandev base. Exporting/loading each final
image separately repeats that base transfer. Build every original recipe and
harness, then pass the three final tags to one pinned Kind load command. Keep
all images, original transfer timeout, lifecycle smoke assertions and owned-image
cleanup; transfer failure must propagate after cleaning every built image.

Reserve one of the existing container jobs for the image-heavy worker-presets
file. Its expensive worker setup is absent from per-test timing; independent LPT
alone assigns eight additional files beside it in the measured manifest. Balance
all remaining files across the other five jobs, retaining the shared control-plane
Kubernetes group. Keep six container jobs and one worker per job;
all native test identities, future-file discovery and optional gates remain.
Verify both manifest modes and exact catalog union, then run the real worker
lifecycle suite and measure complete hosted elapsed. Batch-loader support is
confirmed in the pinned Kind v0.32.0 command implementation.

### Queue edit synchronization (seventh candidate)

Fifth-run native report `blob-report-8` records the mobile later-entry edit
and held-head save cases at 46.825 and 43.989 seconds. Each starts a 30-second
mock response and waits for it to finish after establishing an edit lease.
The desktop equivalents use ten-second sleeps. Elapsed time is incidental:
the contract requires the predecessor to remain active until the editor holds
the intended queue row, then complete while that lease remains held.

Reuse the sixth candidate's cancellation-aware `e2e:await_file` primitive to
release the predecessor only after the existing editor and draft assertions.
Keep native UI gestures, FIFO delivery, queue counts, editor persistence,
save-triggered dispatch, touch layout and original deadlines. Release the
test-owned marker in `finally` so failed assertions cannot leave a held agent.
Other queue scenarios retain their current fixtures. Measure the same four
tests before and after with retries disabled; validate their unchanged catalog
identities and keep the sixth hosted set running to completion independently.

Local baselines pass at 38.4/36.4 seconds on mobile and 18.1/16.6 on desktop.
Three post-change repetitions each pass with medians 9.0/7.2 and 8.9/6.9
seconds: combined test-body time is 109.5 to 32.0 seconds (70.8% lower).
This is focused local evidence, not full-CI acceptance. The first trial exposed
an exact-text selector against concatenated agent chunks; retain its failure
and interrupted repetitions, scope the corrected locator to agent bodies,
and use the fresh twelve-case passing run for verification. Native catalogs
retain all identities/counts; only these two spec hashes change.

### Bound E2E build-cache publication (eighth candidate)

Sixth-run Build job 116026205121 finishes artifact publication at 18:26:21 UTC,
then spends 102 seconds saving its per-commit Go cache before dependent jobs
can start. Compilation itself takes 94 seconds. A source-only commit currently
recompresses the cache despite compatible cached compilation inputs.

Reuse the optional daily cache action and seven identity/failure tests from
PR #4486 at `fe7d97c84c72`, initially for the E2E build job only. Preserve
native Go content-based invalidation, installed compiler/toolchain/architecture
and recipe compatibility, separate module keys, previous-generation restore,
success-only save, and unconditional required build/verification commands.
Register its tests in CI and add changed-path coverage for the shared action.
Do not import that PR's unrelated workflow, image producer or specification
changes. Its `007.*` annotations describe a different unmerged contract; keep
this branch's existing IDs/meanings and describe local cache checks separately.

Daily first-population saves remain real costs. Keep cold/warm full-run evidence
and verify actual exact-key restores and skipped post-save on subsequent runs.
No capacity, cache-limit or cache-deletion changes are authorized by this step.

Local cache checks pass: seven identity/failure cases, 22 E2E workflow cases,
25 backend workflow cases, required gates, efficiency, runner placement and
SHA pinning. Structured comparison preserves every original non-cache command,
guard, artifact and gate. The real installed-toolchain identity enables caching;
a real Go source change in the same native cache produces the changed binary.
The new workflow contract fails before integration and passes afterward.
Hosted daily-generation population/reuse and full timing remain pending.

### Avoid discarded protocol serialization (ninth candidate)

Sixth-run ordinary backend cohort 3 takes 779 seconds. Its full native JSON
attributes 285.60 seconds to `TestShellSnapshotCoalescingStorageBound` and
62.67 seconds to `TestShellSnapshotDurableStorage`. The tests retain 10,000
and 2,000 source updates with a 256 KiB output window. Protocol tracing currently
marshals every normalized event even when its newly created span records nothing.

Profile those unchanged tests with CI's race/atomic coverage flags. If the
profile confirms discarded serialization as a material cost, return after span
creation when `IsRecording` is false, still ending the span. This is an internal
performance change: preserve every enabled trace attribute/event, truncation,
sampling decision and source update. First prove disabled and sampled-out spans
skip a real JSON marshaler, and recording spans retain the same payloads.
Keep all stress iterations and storage/correctness assertions, then repeat the
exact native workload and run the affected tracing/adapter tests and Go lint.

The race/atomic-coverage CPU profile confirms repeated JSON marshaling through
`TraceProtocolEvent` while debug tracing is disabled. The same two unchanged
stress tests pass in 153.702 seconds before the guard and 9.095 afterward
(94.1% lower), with the same 10.6% ACP statement coverage. Three further
repetitions pass all six cases: 41 coalesced snapshots and 11 durable events
remain within the original budgets, preserving complete output and retirement.
Disabled, sampled-out, sampled and record-only trace regression tests pass ten
repetitions; the disabled/sampled-out checks fail before the guard. All shared,
ACP and tracing package race tests pass; strict changed-package lint finds zero
issues. Fresh native and support builds pass four desktop queue/restart and two
mobile queue browser cases, retries disabled. Hosted impact remains unmeasured.
