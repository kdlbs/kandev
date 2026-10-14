---
id: "16-complete-ci-delivery"
title: "Measure and deliver complete CI"
status: in_progress
wave: 12
depends_on: ["15-remaining-critical-paths"]
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

# Task 16: Measure and deliver complete CI

## Outcome and scope

Collect hosted attempts, exact coverage reports, workflow identities and occupancy. Keep one bounded rollout active; resolve valid CI/review failures and merge the final passing head.

## Exclusions

No new product behavior, paid capacity activation, trust expansion, assertion
removal, automatic retries or delegation. Repair a demonstrated existing-contract
regression only when needed to retain a failing CI assertion. Preserve unrelated user changes.

## Acceptance

Three complete successful workflow sets meet 900 seconds including waiting. Partial, cancelled and failed runs remain visible and do not count. All required checks and reviews pass at the final head before authorized squash merge.

## Files and dependencies

The backend/frontend/E2E workflow files, their registered contract tests and
CI performance evidence. Task 15 supplies the preceding measured result.

## Verification

```bash
python3 .github/scripts/ci-efficiency_test.py
scripts/pr-state --compact <PR>
scripts/pr-resolve list <PR>
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

Hosted evidence remains mandatory; local contracts do not prove speed.

Tenth candidate `cef9436c77d` completes all three workflows: backend
38626664788 succeeds in 702 seconds (693 execution critical path; 196.3167
runner-minutes), frontend 38626664893 succeeds in 759 seconds (751 execution
critical path; 29.6333 runner-minutes), and E2E 38626664950 fails in 957 seconds
(885 execution critical path; 302.1 runner-minutes). The full set is 15m57s,
failed, and does not count toward the three required successful samples.
All 3,963 E2E identities remain: 3,916 first passes, five retry-only passes,
two final failures and 40 existing skips. The two failed cases are keyboard
queue reorder and runtime replacement; keep their zero-retry policies.

Exact daily module/build-cache hits are proven in tenth build 116067615073;
it takes 185 seconds versus ninth's 487 seconds, with exact-hit saves skipped.
The Windows process cohorts restore the previous compatible source snapshot;
only cohort one publishes the new daily build key. All eight Windows suites
succeed. These step/cache observations do not establish a controlled complete
workflow speedup, and queue differences remain material.

Conflict merge `5222f0bfe36` retains canonical parent lookup and the branch's
same-ID selected-row fallback against main `b5705cfb069`. After fresh native,
fixture/plugin and pseudo-locale web builds, both previously failed E2E cases
pass twice without retries (four cases, 52.6s). This does not prove a flake
repair. The selected-parent desktop and real phone scenarios pass without
retries, alongside 26 focused parent unit cases, full typecheck and strict lint.
Normal native enumeration preserves all 1,337 hosted-tenth project/file IDs,
root-spec hashes and 3,810 test counts; containers retain 46 units/153 tests.
The next candidate removes browser images from build/report per Task 15;
23 E2E workflow contracts and existing join/placement/pinning checks pass.
The measured 102-second container-start opportunity is gross, not a net claim.

Eleventh structural head `67a6c3cf430` is rejected before E2E execution:
run 38632630783 fails workflow validation with zero jobs. Its job-level
Corepack environment used `runner.temp`, a context unavailable at job-env
evaluation. This invalid/partial attempt is not a timing sample or a passing
gate. Local Actionlint v1.7.12 reproduces both offending expressions; the new
bootstrap regression also fails against that head. Use a private temporary
Corepack directory literal on each isolated hosted runner instead. Both local
checks then pass. Retain the other workflows' terminal/partial evidence and
collect a new complete set after pushing the correction. The walkthrough
generator also exits unexpectedly after two seconds on the free provider;
retain that infrastructure failure separately from application CI.

Ninth candidate `5f47945b531` succeeds as backend 38620159334, frontend
38620159244 and E2E 38620159182. Backend succeeds in 1,181 seconds including
queue/reporting (796 seconds execution critical path; 211.2 runner-minutes).
Frontend succeeds in 1,002 seconds (949 seconds execution critical path;
28.1333 runner-minutes). E2E succeeds in 2,112 seconds (1,244 seconds execution
critical path; 308.1667 runner-minutes). The complete set therefore takes
35m12s, slower than the sixth set, and fails timing acceptance. All 3,963
selected E2E identities remain: 3,916 first-attempt passes, seven retry-only
passes, 40 existing skips and zero final failures. Its first daily-cache
population build takes 487 seconds, including 85 seconds of cache publication;
retain that cold cost in the complete sample. Logs prove successful module and
build-cache publication. Exact-key reuse is still unmeasured. PR #4489 is ready
and mergeable, not merged.

Ninth frontend artifacts preserve all sixth-set identities, adding only the
new mock-turn release helper test: 2,953 files, 26,447 tests, 26,443 passes and
four skips. The actual hosted ACP package falls from 375.557 to 42.322 seconds;
its two unchanged shell-snapshot stress cases fall from 348.27 to 16.78 seconds
combined. Backend runner-minutes fall 14.0%; the complete workflow still misses
fifteen minutes. The E2E manifest uses the same successful-main timing profile
38599930486, and retains every selected native unit and count.

The tenth candidate's sidebar retry repair has a deterministic browser red
before the fix and three passing repetitions afterward. The complete affected
desktop navigation/hover/resize/customization group and existing phone drawer
pass without retries, alongside focused unit tests, full typecheck and strict
lint. These local results do not establish a hosted speedup.

Sixth candidate `6e006a425` is pushed. Backend 38614506503, frontend
38614506677 and E2E 38614506377 run on hosted merge
`6b1b627b0ace6e462b78b2f66efc679457624fa8` over main `148c84f072`.
Its tree matches locally verified synthetic merge `5d5da539316c`: 33 parent/mobile
context tests, full web typecheck and affected race-enabled backend checks pass.
Current-main enumeration adds only two workspace-adoption scenarios, for
3,810 normal plus 153 container tests; no existing identity or count changes.
Backend passes in 865 seconds including queue/reporting, with 802 seconds
execution critical path and 245.6 runner-minutes. The single primary waiter
preserves this full attempt while local queue/cache follow-ups are prepared.
Frontend passes in 1,001 seconds (696 execution critical path; 26.75
runner-minutes). E2E passes in 1,921 seconds (1,037 execution critical path;
303.7833 runner-minutes). The complete set therefore takes 32m01s and fails
the fifteen-minute timing target. Its 34 reports account for all 3,963 E2E
tests: 3,916 first-attempt passes, seven retry-only passes and 40 existing skips.
Hosted native IDs, hashes and counts match the locally verified current-main
catalog; its actual timing profile is successful main run 38599930486, rather
than the older local planning profile. Preserve that scheduling difference.
The primary waiter confirms 85 passed/14 skipped, zero failed/pending and no
file conflict; its exit 1 reports base drift, separately checked on the same
hosted merge tree. Local queue/cache/tracing follow-ups remain unmeasured on CI.

Second candidate head `e2a66b8536` runs backend 38581852307, frontend
38581852072 and E2E 38581852055. Its tested merge is
`048610fb914664e1873c4ac671c66c1622bde238` over base `73c5ddb8c955`.
The initial head `6025705717` failed frontend argument forwarding and two cold
Windows compilation steps; the subsequent push superseded its remaining jobs.
Record that attempt as failed/partial, never complete timing acceptance.

For controlled frontend comparisons, a temporary branch
`perf/full-ci-frontend-baseline` starts at the exact tested merge. Commit
`15b7acfc46381193ab6f792a43ad2b9de6f0d3bd` restores the base frontend workflow
and adds only a JSON report, required artifact upload and workflow/checkout
identity recording. Both candidate and baseline application tree hashes are
`c3cfe17ff326562b01df5257bedbdf92fc23c511`. Its normal hooks passed with no
bypass. Dispatch 38582284835 is the first unsharded measurement. This branch
has no PR and will not merge; comparisons must retain event/cache differences.

Review thread 4271878983 correctly identified an overly broad frontend job
range in the runner placement contract. Narrow it to the static job and
explicitly check the new repository job's hosted boundary. Five placement
contract tests pass; thread disposition follows the verified fix commit.

The first complete frontend candidate succeeded in 825 seconds (13m45s), with
757 seconds of execution critical path and 30.15 runner-minutes. Its two reports
contain 26,411 tests (26,407 passed, four skipped) across all 2,948 files. The
normal repository artifact union is exactly 1,904 top-level tests, once each,
at merge source `048610fb914664e1873c4ac671c66c1622bde238`.

A matching temporary branch `perf/full-ci-frontend-candidate` points directly to
that tested merge. Dispatch-only frontend comparisons on the two temporary
branches preserve identical application trees and avoid PR-event differences.
The first PR result is useful loaded evidence; baseline reports and repeated
matched dispatch samples remain pending.

Third candidate `5949b632b0` passed the complete backend workflow 38587972467
in 1,134 seconds (18m54s), with 971 seconds of execution critical path and
243.32 runner-minutes. All four Windows process reports validate the same native
inventory: 864 names across two packages, split 216/216/216/216 exactly once.
Frontend 38587972660 failed only the existing Kubernetes SemVer workflow-string
contract, which still expected the old make target; update that assertion to the
new versioned native build command. E2E 38587972415 remains pending and cannot
count as complete acceptance. Its build improved from approximately eight to
six minutes.

Matched frontend dispatch pair two (38587875054 / 38587880385) passed with
identical 26,411 test identities and outcomes. Full elapsed was 1,725 versus
704 seconds; execution critical path was 1,540 versus 663 seconds; runner-minutes
were 25.6667 versus 26.5333 (+3.38%). Pair three is running as
38592146394 / 38592152123 on the same frozen application tree. The earlier PR
sample's higher occupancy remains retained and is not hidden by these pairs.

The fourth candidate's local verification covers canonical shared-manifest
identity/digest checks, isolated caches and failed-preparation cleanup, actual
worker-image command boundaries and cleanup, existing Kubernetes pins, native
Go allocation, cache/workflow contracts and unchanged process resource guards.
The real local Kubernetes worker-preset lifecycle passed for minimal, node-pnpm
and Python images (one test, 5.2 minutes including fixture setup; test body 37.7
seconds). With the real 44 MiB helper, preparation measured 21.894 seconds once
and 0.643/0.626/0.664 seconds for three isolated bundles. These local measurements
are diagnostic; the complete hosted acceptance gate remains unmet.

The same 3,808-test normal manifest partitions into 84 disjoint one-worker
processes across 28 jobs. Its longest whole-file estimate is 438.6825 seconds,
so the idealized 343-second average is not the expected tail. Native ordinary Go
selection remains exactly 407 packages in buckets 97/105/112/93, with the
repository package delegated once to its required native-cohort matrix.

### Third-candidate mobile failures and bounded remediation

E2E run 38587972415 failed in 2,076 seconds (34m36s), with 1,767 seconds
execution critical path and 377.43 runner-minutes. Preserve this failed attempt.
Its two failures are mobile empty-turn hints and non-active parent defaults.
The trace's sidebar response contains the selected parent's repository, but the
subtask dialog reads only the active Kanban collection. A browser connected
before the parent finishes can mask that missing data dependency through live
events. The dialog's initial repository effect then runs only on mount.

Keep the existing mobile composition and assertions. Pass the selected drawer
row into the existing subtask dialog as initial parent context; prefer a matching
current store row when available. Do not change persisted settings or form edits.
Make the existing E2E wait for parent completion before navigation to prove the
paged-row path without live creation events. Cover explicit selected-parent
context independently of the active task and preserve desktop callers.

The empty-turn notice is live-only. Use the same established desktop test
sequence on mobile: open a completed ordinary turn, then send the empty slash
command after chat readiness. Do not lengthen retries/timeouts or remove the
hint assertion. Verify both rendered mobile scenarios together after rebuilding.
This is CI regression remediation under the user's continuous delivery authority.

The strengthened selected-parent test failed locally before the fix, matching the
hosted missing-chip assertion. After the row handoff and a fresh production web
build, all three focused mobile scenarios passed in 20.3 seconds: the live hint
(using the phone Send button), ordinary Create Subtask, and completed non-active
parent defaults. Eleven existing component/context tests, changed-file ESLint,
formatting, full web typecheck and documentation validation passed. No UI layout,
copy, public API, or operator instructions change; this restores existing defaults.

Desktop Create Subtask also passed (one test, ten seconds including setup).
The normal commit hook enforced the component-length limit; extracting the
mobile target state/handler into a focused hook preserves the same row handoff.
The extraction passes strict lint and the seven mobile component tests.
Matched frontend candidate three passed all 26,411 tests in 1,074 seconds full
elapsed, 739 seconds execution critical path and 30.48 runner-minutes. Its paired
unsharded baseline remains running. The added waiting is retained in the result.

### Fourth hosted candidate and fifth-candidate remediation

Head `13aa34b8eedead8cb32bfa29aefb1e1dded96ebc` uses tested merge
`39c237ef2e5a4fbe32b643d456458701a9f284b1`. Frontend 38595327890 passed
in 588 seconds with 26,419 tests; the eight-test net increase is accounted for
by the new fixture/process cases. All three frozen frontend comparisons now
pass exact identities/outcomes. Median execution critical path is 1,576 to 682
seconds (56.73% lower); median runner-minutes are 26.2667 to 27.2 (+3.55%).

Backend 38595328037 attempt one failed a Go proxy HTTP/2 download before
repository cohort 1 began. All other jobs passed; full attempt elapsed was
1,136 seconds. Retry only that failed job once under the transport-failure
procedure. This is not a fresh complete performance sample.

E2E 38595327868 has three failing tests: coordinator permission answer and two
auto-scroll scenarios. The two mobile regressions now pass. The auto-scroll
failures match the selector remediation already authored in PR #4486 at
`fe7d97c84c72210151b93c3aee65b8c6d2a844cb`: exact output text for the
state-transition case and agent-message-body scoping for live markers. Reuse
that exact test-file patch, retaining every scroll assertion, deadline and
retry setting; verify the two scenarios locally. This does not adopt the
other PR's shared cache or image changes. Coordinator diagnosis remains open.

Coordinator diagnosis: a five-second delay of the application entry module
reproduces the missing card locally. The backend task remains RUNNING with
`status_summary.pending_action=permission` and a persisted permission request.
`useAllWorkflowSnapshots` mounts after the socket is already connected, accepts
the earlier boot snapshot, and never observes a connection-state transition to
refresh it. This is a missed boot-to-live update, not lost auto-approval config.
Permission waiting is represented by `pending_action`, not necessarily
WAITING_FOR_INPUT; an exploratory state wait was therefore discarded.

Retain boot data for immediate rendering, but refresh the shared snapshot on
initial mount when the live connection is already ready. Before connection,
retain the existing boot fast path; the connection transition still refreshes.
Strengthen the existing permission-answer E2E by holding the application entry
module until the backend reports the actual pending permission, then verify
all original answer/resume assertions. Add a focused hook regression for an
already-connected mount, preserved boot data during the read, and no rerender
fetch loop. Remove all temporary diagnostic logs and arbitrary startup delays.

Fourth E2E elapsed is 1,736 seconds (28m56s), with 1,725 seconds execution
critical path and 340.47 runner-minutes. It is failed evidence, not acceptance.
The backend's one permitted failed-job rerun succeeded; attempt-two reused-job
timestamps cannot form a fresh DAG timing sample, so retain attempt-one timing
and the successful recovery outcome separately.

The coordinator causal E2E failed at the original missing-card assertion before
the fix, even when the startup barrier waited on actual `pending_action` rather
than a fixed delay. The new already-connected hook case also failed before the
fix. Its surrounding snapshot/Coordinator suites pass 63 cases after the fix.
The container planning/runner suites pass 39 cases; native container selection
is unchanged at 46 units / 153 tests with the same file hashes and checksum.

The coordinator causal browser case passes after the snapshot fix (3.7 seconds).
The reused auto-scroll selectors fix the live-tail strict-mode failure, but the
combined state-transition test still fails locally because its direct scrollTop
assignment races catch-up motion from the preceding step. Reader intent is
registered through wheel/pointer/key events; direct assignment does not interrupt
that motion. Replace only the three reader-position setup assignments in that
scenario with a real upward wheel and an observed stable position, following the
existing inactive-session case's gesture. Preserve all enabled/disabled, freeze,
remount, and catch-up assertions and their deadlines.

The first combined local Kubernetes attempt is invalid: source-built binaries
still carried commit 5949 while the checkout/manifests used 13aa. The nested
backend explicitly rejected the remote-helper manifest identity. Stop that
owned run, retain its failure log, and rebuild native/remote helper artifacts
with the same current version and full commit before rerunning. Do not weaken
identity validation or count this attempt as timing/fixture acceptance.

With matching local artifacts and a fresh web build, all four focused browser
checks pass in 38.8 seconds: complete scroll state transitions, live upward-wheel
pause, clarification answer, and the causal delayed-startup permission answer.
Strict changed-file ESLint, full web typecheck, 49 planner/hook refactor cases,
21 E2E workflow contracts, seven measurement contracts, and documentation checks
pass. The real combined Kubernetes suite is being repeated with verified build
identity; hosted fifth-candidate measurements remain pending.

Normal shard 23 passed only after two 240-second setup failures. Its three
processes were enabled; the tail was not a resource fallback. Both failures
were a task-title hover card intercepting `openTaskSession` during the third
presentation task's setup. `startPresentationThread` already starts the task
through `createTaskWithAgent`, then redundantly opens the board and clicks each
card before waiting for the agent turn through the API. Remove only that
redundant navigation from the presentation-specific helper and its unused Page
argument. Keep real task creation, agent execution, completion waits, and every
Threads layout/settings/composer assertion. Existing dedicated board/session
navigation E2Es retain ownership of card opening. Verify the affected desktop
and mobile presentation suites; no global task-opening helper or product hover
behavior changes are part of this cleanup.


Final fifth-candidate native discovery preserves every fourth-run unit identity
and test count: 1,336 normal units / 3,808 tests and 46 container units / 153
tests. Only the eight edited normal spec file hashes change; the container
catalog checksum remains identical. The task presentation helper change does
not delete a scenario or assertion.

Fourth-run static-check logs confirm a complete Go/lint cache miss. That job
spent 233 seconds on catalog/SQLite conformance and 453 seconds on golangci-lint,
then approximately 172 seconds saving its cache. Treat the next restored-cache
result as a separate measured condition; do not attribute all cold-to-warm
improvement to source changes. Related PR #4486 remains open and owns the cache
refresh implementation; this candidate does not duplicate that work.


The combined Kubernetes suite passes with one worker and matching native/helper
artifacts: 18 passed and the four existing opt-in cases skipped in 10.7 minutes.
All seven files share one Kind cluster, including native Codex, in-cluster auth,
restart/ownership/storage/error recovery, concurrent task sessions, and all three
prepared worker images. The owned cluster is absent after teardown. This proves
fixture compatibility and cleanup, not a hosted latency win. The affected 33
Threads desktop/mobile checks are the remaining local gate before push.


The first affected Threads run completed in 5.8 minutes: 31 passed and two
translated-settings cases failed because the manually rebuilt local web bundle
used the production target, which excludes the pseudo QA catalog. Both failures
received English Grid text at the unchanged pseudo-locale assertion. Retain this
configuration failure, rebuild using the standard make build-web-e2e target used
by CI, and repeat both desktop/mobile display-settings files. No product or test
assertion change is justified by this missing local build flag.


After make build-web-e2e, all six desktop/mobile display-settings cases pass in
53.9 seconds without source changes or retries. Together with the first run's
31 passing cases, every one of the 33 affected Threads scenarios has passed; the
two configuration failures remain recorded above. All final source checks pass.
The next action is the normal hooked commit and one hosted candidate push; full
workflow acceptance, current review and merge remain pending.


### Fifth hosted result and sixth-candidate remediation

The fifth full set is complete and fails acceptance. Backend run 38604393643
failed at 1,179 seconds to the last job (1,181 to workflow update), with 966
seconds execution critical path and 230.57 runner-minutes. Frontend run
38604393508 passed at 1,291 seconds to the last job (1,390 to workflow update),
with 705 seconds execution critical path and 26.12 runner-minutes. E2E run
38604393338 failed at 1,885 seconds to the last job (1,886 to workflow update),
with 1,261 seconds execution critical path and 311.82 runner-minutes. Keep both
end timestamps visible; a workflow update is not always immediate job completion.
The complete set is about 31m26s including queue. No fifteen-minute success.

Backend bucket 2 exposes two test-fixture defects. The stable-window recovery
test shares the latest binding and callback between goroutines; CI's race report
pins those exact writes/reads. Pass each completed binding, callback and attempt
number through its existing start channel instead. The standalone deadline test
can consume its 10 ms deadline during HTTP enumeration before any loser stop is
dispatched. Complete enumeration before the timed collection phase, retain real
control-server stops, and withhold their result delivery until after that phase
returns. Preserve the 10 ms deadline, 100 ms prompt-return assertion, zero
recovered sessions, joint-failure retention and two-second background bound.
Move the latter test into a focused file rather than growing the large fixture.
No production recovery code or behavior changes.

Both repaired tests pass 30 race-enabled repetitions. Surrounding runtime
replacement/recovery and standalone recovery cases pass; scoped changed-package
Go lint reports zero issues. The unmodified original tests also passed 30 local
repetitions, so CI's concrete race trace and missing precondition are retained
as failure evidence rather than claiming deterministic local reproduction.

The optional sidebar compilation cache passes 25 backend workflow contracts,
five required-gate contracts and nine pinning tests; all 27 workflows retain
pinned actions. A structural comparison confirms that removing only the two
new optional cache steps yields the identical original workflow: matrices,
conditions, commands, artifacts and gates are unchanged. Documentation checks
validate 382 decisions and 1,610 specifications.

Fifth memory-1 runtime is 430.346 seconds, but its complete job took 943 seconds:
first Go test execution was at 17:18:35 after a 17:10:57 job start. Cold setup and
compilation therefore dominate its excess over the budget; the new cache still
requires hosted validation. The grouped Kubernetes job passed but took 842
seconds, and its setup plus the shared build/report path remains over budget.
Treat fixture affinity as an experiment needing further improvement.

E2E failures: normal shard 6 exhausts keyboard navigation in the long Threads
question case; three local repetitions pass and trace diagnosis is pending.
Normal shard 8 fails the worktree and local_pc survival tests because their
fixed /slow 12 response completes before restart assertions on a loaded runner.
Preserve running-turn assertions and replace the fixture's wall-clock race with
a causal release mechanism. The sixth-candidate repairs and local verification follow below. Hosted timing
for those repairs remains pending.


### Sixth-candidate local verification

Add the mock-only inline await_file directive, which holds a turn until its
unique worker-owned release marker exists and honors prompt cancellation.
Blocking/cancellation cases fail before implementation and pass afterward;
the full mock-agent package passes with race detection. The survival tests now
release the turn after every original in-flight restart assertion, and release
again in finally for cleanup. Scope output locators to agent message bodies so
the echoed script prompt cannot satisfy them. All identity, state, single-start,
normal completion and follow-up message assertions remain.

The keyboard diagnostic proves that returning from Back to SQLite previously
takes 94 forward Tabs after Back disables itself and focus falls to the body.
Native Shift+Tab reaches that same action in two presses. Use that return path
for the first option while retaining forward navigation, Space/Enter activation,
the ten-second bound and every geometry/hit-target assertion. The case passes in
10.5 seconds locally after the change versus about 17 seconds before it. Remove
all temporary diagnostic logging before final validation.

Build all three original worker recipes and their final harnesses before one
Kind image-load command. The pinned v0.32.0 loader exports one shared archive.
The command contract fails before the change, then confirms all six builds,
exactly three final tags in one transfer, and complete cleanup/failure propagation.
A real single-worker Kind lifecycle case passes in 4.7 minutes, exercising minimal,
Node/pnpm and Python images; its owned cluster is absent after cleanup. This
local subset proves compatibility, not full hosted timing acceptance.

The planner reserves one existing container job for the image-heavy fixture;
otherwise its setup-free timing would put eight additional units beside it.
The other five jobs retain exact discovery and control-plane fixture affinity.
Profile/fallback allocation tests fail before the reservation and pass afterward,
including one-shard fallback with the complete catalog. All 49 planner, runner,
subshard, worker-image and fixture-policy cases pass. Strict changed-file ESLint,
full web typecheck with a bounded heap and scoped Go lint pass. All nine final
browser repetitions pass in 1.6 minutes with diagnostics removed. Both profiled
and fallback native manifests preserve all 1,336 normal units / 3,808 tests and
46 container units / 153 tests; only the intentionally edited survival spec hash
changes. The worker-image fixture owns existing shard six in both plans, and
container catalog hashes/checksum are identical to the fifth candidate.
Documentation validation passes 382 decisions and 1,610 specifications. The next
hosted full set, current review and merge remain the delivery gates.
