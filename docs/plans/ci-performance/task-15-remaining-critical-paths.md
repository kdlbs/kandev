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

No application behavior, paid capacity activation, trust expansion, loss of
unique regression coverage, automatic retries or delegation. An obsolete or
duplicate assertion may move to its existing contract keeper only with explicit
ownership and before/after verification. Preserve unrelated user changes.
Process experiments retain one Playwright worker per process and enforce an
explicit measured memory/CPU budget. No unsafe override or guard bypass is
allowed. The next four-process experiment below changes the planned-runner
budget only after its resource baseline; managed local full-suite limits stay
unchanged.

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

### Retain the open portable-config accordion

Candidate 27's only final E2E failure is the portable-agent configuration case.
Its original native steps record two clicks on the same Mock Codex accordion
trigger: first the regex opener, then the Not Configured title after selecting
the config bundle. The second click closes that group. A visibility assertion
can still see the exiting content; three attempts then spend 51-55 seconds
trying to click its disappearing auth checkbox, and the remaining attempt
observes unchecked before disappearance. Preserve the four failed attempts;
the final outcome is a timeout. The keyboard-shortcut scenario also retries
but passes finally and requires separate readiness investigation.

Remove only that duplicate accordion toggle. Require the original opener to
remain aria-expanded=true before interacting with the auth checkbox. Retain
all config/auth independence, persisted values, reload and original timeout/
retry assertions. This is a correction to scenario actions, not a product
change or an assertion deletion. Verify the original native identity and the
focused browser case with retries disabled after the Kubernetes run ends.
Do not overlap local runner ports or rebuild its inputs during that run.

### Bound disposal grace for fixture-owned Kubernetes leftovers

Candidate 26's container shard 1 takes 707 seconds without any container retry.
Native steps attribute 169.627 seconds to the Kubernetes cleanup fixture;
four hooks above 25 seconds account for 153.228 seconds. These hooks include
both backend reset and kubectl disposal, so do not attribute every second to
Pod grace. Cluster setup separately takes 174.270 seconds.

The cleanup helper deletes only its disposable Kind cluster's workload
namespace. Its callers are per-test reset and finally blocks after the
in-cluster and causal-failure assertions. Product stop/resume, Pod/PVC identity,
retention, label/UID deletion protection and causal-error checks run before
this disposal. They retain production Pod templates and deletion behavior.

Use only `--grace-period=1` for this cleanup kubectl command. Keep synchronous
`--wait=true`, its 90-second timeout and the following empty-namespace check.
Do not use force deletion or bypass finalizers. This is a command-option change;
prove it through the existing real Kind lifecycle cases instead of adding a
source-string test or test-only export. All assertions, test IDs, worker limits,
retries and deadlines remain fixed.

The first local baseline is invalid: an old generated build source marker
causes remote-helper identity mismatch. It stops with one failure, one
interruption and ten unrun cases; both owned clusters are removed. Preserve
that sample. Rebuild backend/helpers, regenerate the canonical six-artifact
identity for the exact compiled commit and repeat unchanged. The corrected
baseline passes all twelve executor cases in 6.8 minutes with retries disabled.
Native blobs retain the cleanup-step profile and exact outcomes.

Next measure the one-option candidate across all six Kubernetes shard-1 specs
under the managed single-worker guard, including the same four existing gated
Docker-workload skips. Compare its twelve executor cases directly with the
corrected baseline; preserve all other scenarios and source-bound native
inventory. Require lint/format/docs, focused real Kind validation and the next
complete hosted set before accepting any full-workflow improvement.


Local verification retains all six Kubernetes shard-1 specs: seventeen pass
and exactly the same four existing gated cases skip, with retries disabled.
For the identical twelve executor cases, native cleanup-step time falls from
137.231 to 109.062 seconds: 28.169 seconds saved (20.53%). The foreign-UID
case's cleanup falls from 61.047 to 32.744 seconds; other thirty-second waits
remain. Do not claim the full 153 seconds identified in hosted cleanup as
saved. The full six-spec run takes 7.6 minutes; it has a different scope from
the twelve-case 6.8-minute baseline, so those totals are not a speed comparison.

The corrected portable-config case passes locally in 8.4 seconds with retries
disabled. Native collection retains all 3,957 E2E IDs; strict lint/format pass.
Both changes leave production and unit-test inventories unchanged. Full hosted
coverage/timing and current-head review evidence remain required.

### Remove duplicate remote-repository layout cleanup

Candidate 26's full E2E fails remote repository creation's pasted-issue case in
`test.afterEach`, not its title or popover assertions. The original backend log
and error context report a real sidebar layout revision conflict from
`restoreSidebarLayout`; this suite already disables retries. Its unchanged
complete ten-case suite passes locally against a fresh managed build in 32.9s.
Keep the hosted failure separate from this local baseline.

Disposition D: remove only this suite's `initialLayout` snapshot and layout
restore hook/imports. The owning `testPage` fixture already resets the layout
before opening each fresh browser context and closes that context after use.
The extra afterEach restore runs while that browser can still persist its own
layout. Restoring a prior suite snapshot has no independent product oracle;
its second settings write conflicts with the fixture's existing ownership.
The same helper remains for its other callers. Do not add conflict retries or
ignore settings errors.

Disposition R: retain all ten native remote-picker cases and every existing
assertion, including the first case's explicit collapsed navigation, loading
geometry, all source selections/submissions and the pasted issue's title and
popover bounds. Keep suite retries zero and all deadlines unchanged. Verify
that the complete test-body suffix is byte-identical, native collection keeps
all 3,957 IDs, and the ten-case managed browser suite passes without retries.
No product behavior or API contract changes. Preserve the complete failed CI
sample and rerun full hosted selection only after focused validation.


The focused edited suite passes all ten cases in 31.0s with retries disabled,
against the same freshly built application as its unchanged baseline. Strict
ESLint/format pass, and native collection retains the exact 3,957 IDs. All ten
complete test-body suffixes remain byte-identical. Hosted validation is pending.

### Restore the unavailable pinned Go setup action

The twenty-fifth candidate fails two Windows jobs and the preview before
checkout/tests: GitHub cannot download setup-go commit
`924ae3a1cded613372ab5595356fb5720e22ba16`. Both archive formats return 404,
although the official v6/v6.5.0 refs and commit API still identify that SHA.
Do not classify those missing executions as coverage or a fast pipeline.

Use official setup-go commit `5064d95ba18d4a9dda7c4cc67adeb03aea1a2a14`,
whose ZIP and tar archives are available. It follows stable v7.0.0 by one
upstream commit and includes bundled dependency refreshes. It is not runtime-
identical to the old pin. The official v6.5.0/v7.0.0 action definitions retain
the same input defaults, outputs and Node 24 main/post entry points; v7's ESM
migration is supported by the already-used Node 24 runners. The observed Windows
runner is 2.337.0. Do not select the old pin's parent: its bundled dependencies
would roll back. The latest main contains additional unreleased behavior and
is outside this bounded change.

Replace the unavailable pin consistently in its six workflows. Keep Go version
files, cache options, permissions, matrices, commands and deadlines fixed.
Prove that exact structured delta and rerun affected workflow/pinning contracts.
Download and validate the new official archive before pushing. Actual Windows
compile/process execution and the full hosted set remain mandatory; no SDK
version override, release action or unpinned ref is introduced.

Local checks pass: backend workflow 25 cases, E2E workflow 27, preview seven,
release 65, pinning nine, and all 27 workflows SHA-pinned. The release suite's
first invocation lacked Node on PATH; the corrected existing Node 24 invocation
passes every case. Both official archive formats validate, and parsed workflow
comparison proves only fifteen setup-go references change across six files.
No application or test selection changes are introduced.

### Keep native blob evidence while replacing its failing transfer path

The twenty-fourth full set passes every test job but fails its reporter after
all three blob-download attempts fail identically. Each attempt spends about
27 seconds in the action's five internal retries; existing outer waits add
another forty seconds. A partial inner ZIP then fails the unchanged parser.
The complete E2E workflow takes 16m29s. GitHub CLI downloads all 34 artifacts
successfully, and all 118 native ZIPs pass CRC validation. Read-only local
reconciliation proves all 3,957 identities and zero final test failures; that
recovery does not change the failed hosted conclusion.

Use the existing GitHub CLI on the hosted reporter for this blob transfer only.
Retain three total download attempts and the existing ten/thirty-second waits,
isolate each attempt in a fresh staging directory, and publish files only after
the whole download succeeds. Flatten the artifact folders without overwriting
duplicate basenames or following links, preserving every native ZIP and shard
metadata file byte for byte. Empty, unexpected and colliding inputs fail closed.
Leave Build/runtime downloads and their action unchanged. Do not alter report
parsers, catalog provenance, full native identity reconciliation, HTML/JSON
reporting, test exits, shard counts, workers, deadlines or retry policy.

First fail a workflow contract that requires this transfer route and its token
scope, then cover successful binary transfer, clean retry isolation, exhaustion,
invalid/duplicate inputs and unchanged destination on failure. Wire the helper
checks into the existing unfiltered required contract workflow. Exercise the
helper against the actual twenty-fourth run, compare every resulting byte with
the recovered artifacts, and rerun the same raw native reconciliation. Run all
affected workflow/gate/pinning contracts and docs checks before one full hosted
measurement. No new application logic, coverage protocol or hosted job is needed.

The new workflow-route and required-test wiring checks fail before configuration
changes, then all 27 E2E contracts pass. Eight helper checks pass, including a
copy-interruption regression that first exposes partial publication. Publish a
fully copied staging directory through a rename so an interrupted copy leaves
the destination untouched. Three attempts and ten/thirty-second waits remain.
All five gate checks, nine pinning checks and the full 27-workflow pin scan pass.

The final helper downloads the actual failed run's 152 files (118 native ZIPs)
in 23.354 local seconds. Every byte matches the independently recovered set.
The unchanged parser reconciles all 3,957 identities with identical attempts,
statuses, outcomes and durations. This local smoke does not establish hosted
speed. A structured YAML comparison proves only reporter tool preflight and blob
transfer change; all other E2E steps, jobs, selections, permissions, deadlines
and generic artifact-action bytes remain unchanged. Application and test source
are unchanged from the all-test-job-passing twenty-fourth candidate.

### Preserve the winning Office execution on duplicate start refusal

The twenty-third full set's remaining file-panel failure exposes an existing
launch-ownership defect. Its native trace changes the Office session from
`IDLE` to `FAILED` with `execution already running`. A focused real
`StartTaskWithEnv`/SQLite reproduction confirms the general start caller fails
an already-running Office session after the executor refuses a duplicate
dispatch. No second agent is dispatched. The prepared-session start caller
already handles this sentinel without recording a failed launch.

Under Task 16's demonstrated-existing-contract repair scope, apply that same
refusal handling to the general start caller after settling only a turn owned
by the losing call. Return the original typed refusal. Preserve the live
session, execution identity, environment and existing failure handling for
genuine launch errors. Do not alter Office scheduling, retries, recovery UI,
credentials, lifecycle deadlines or CI selection. Keep the original native
file-panel scenario and all its assertions.

Remove that file-panel case's generic fresh-agent recovery fallback. It is
invalid for scheduler-owned Office tasks and masks the launch failure with
another launch attempt. The case retains its original navigation, visible
Files panel and exact `.gitkeep` tree-row assertion. Office onboarding launch
and the existing scheduler-only start/resume checks remain the launch owners;
generic recovery-dialog behavior belongs to its dedicated recovery scenarios.
Preserve the native case identity, original deadline and retries.

Convert the reproducer to a focused regression through the real start boundary;
assert the running state, native execution identity and single session, and
reject any duplicate manager dispatch. Retain the existing genuine-failure
regression. Verify both with the race detector, affected Go lint and all five
Office advanced-mode browser cases against a fresh backend build. Then measure
the next complete hosted set. No timing savings are claimed from local checks.

The real-guard reproduction fails before the three-line caller repair. All
three focused checks pass afterward, including the existing genuine-failure
and sequential Office-reuse cases. The identity assertion initially populated
only a session projection; the corrected fixture seeds the runtime ledger,
checks its initial identity and verifies it survives the refusal. The affected
Office/start cases pass with the race detector, and affected Go lint reports
zero issues. All five unchanged Office scenario identities pass against a fresh
backend/web build in 29.6 seconds with browser retries disabled. The exact
3,957-case native E2E inventory and affected-spec lint/format pass. The new
backend case is the only approved backend inventory addition; all prior cases
and outcomes remain required. Hosted timing and full coverage remain pending.

### Make the twenty-second run's setup checkpoints causal

Keep the two previous repairs: their hosted tool-completion and native lane
cancellation cases both pass on their first attempt. The next bounded change
addresses two final failures and a retrying sidebar case, without production
behavior changes, longer deadlines or more retries.

Quick Chat's baseline currently accepts an initial settled session before its
opening turn has completed. The failure screenshot shows the next slow prompt
before the opening response and a failed-send notice. Require a completed
persisted turn as well as the existing addressed-session settled state. First
prove that an empty/pending turn ledger cannot satisfy the baseline, and retain
active-session and session-identity checks. Keep both phone cancellation cases,
their original slow turn, touch controls, pending/disabled state and follow-up
send assertions. Opening-prompt behavior remains covered by its existing suite.

Preview feedback currently allows a five-second mock turn to finish while the
test opens a browser panel and captures feedback. Use the existing mock-turn
file gate to keep that real turn busy until the queue chip is observed. Release
it before checking delivery, and release again during cleanup. Preserve every
capture, persistence, attachment, queued-payload and settled-composer oracle.

The sidebar case's retry reads verbose accessibility text when checking the
compact visual label. Scope that check to the existing aria-hidden compact
span and keep the screen-reader-text assertion. Before reload, verify the
selected saved view has actually persisted through the existing settings API;
retain the reload and task-row assertions. This changes test checkpoints,
not settings synchronization or accessible output.

The three new readiness cases fail against the original helper, then all four
session-helper cases pass with the completed-turn fence. The pre-existing
localhost connection warning also occurs with the unchanged HEAD test; that
probe restores the edited file byte for byte. It is not a new test failure.

Both phone cancellation cases pass in 39.5 seconds without retries. The full
preview feedback case passes in 20.0 seconds with the gate, and the sidebar
presentation case passes without retries. An intermediate preview marker
assertion used a nonexistent test ID; the corrected exact visible agent text
establishes the same causal marker. No queue/capture or accessibility oracle
is removed. Full web typecheck, strict affected-file lint/format and all 3,957
native E2E identities pass. Frontend adds exactly three named readiness cases
in the existing session-helper file, for 26,553 expected cases and 2,956 files.
The prior nine gesture cases remain. Each approved addition list is checked
against its actual source hash. The twenty-third hosted set confirms all four
changed browser cases pass first-attempt, preserves every native E2E identity
and accepts exactly those frontend additions. Full E2E still fails a separate
Office file-panel case in 15m41s; Task 16 records that rejected measurement.

### Repair the twenty-first run's settled-tool and drag-release failures

Full E2E retains all 3,957 identities but fails two existing cases. The tool
completion screenshot contains the completed two-tool group; its old Terminal
locator describes the running group and can pass before the final projection.
Wait for the settled two-call group, retain the no-spinner oracle, and expand
it to verify both original Terminal rows. Do not remove tool-completion coverage.

The lane trace proves Escape removes the drag reserve, then mouse release
navigates to another card. The height poll is left on a task page. The current
sensor removes its click suppression after 50ms, while this Escape action takes
125ms. Preserve the existing adaptive-Kanban cancellation/height contract by
suppressing the native click belonging to the completed drag until it is
consumed or a new pointer gesture begins. Keep keyboard/assistive activation
and the next ordinary click/tap available; add no arbitrary suppression timer.
First prove the cancelled-gesture click fails focused component tests. Strengthen
the original browser case to cross the sensor's old timer boundary and assert
that native release stays on the board; retain growth, equal heights, no inner
overflow and shrink after deletion. Then verify the focused component suite,
both original browser cases, type/lint and existing phone navigation.

Mobile contract: the existing Kanban phone surface remains one focused column
with native scrolling, menu-based moves and direct card-tap navigation. Its
cards do not activate dragging. Reuse that composition, scroll owner, safe-area
and touch targets without new UI. The shared guard changes only a drag's
residual pointer click. Mouse/touch component cases and the existing
mobile-kanban-navigation scenario verify that a subsequent ordinary tap still
opens the task and phone navigation/viewport containment remain intact.

The focused cancelled-gesture checks fail against the original surface,
including release over another workflow's card. Both original browser cases
also fail locally with retries disabled: the settled group has no Terminal
button, and delayed mouse release navigates away from the board. The repair
then passes all ten component checks, including ordinary mouse/touch gestures,
keyboard activation, a missing synthetic click, secondary touch and unmount
cleanup. A test-local PointerEvent input was corrected to mark ordinary
pointer downs as primary; no production workaround was added for that input.

All twelve desktop tool/height/scroll/drag cases pass in 47.0 seconds, and both
existing phone navigation/scroll cases pass in 10.0 seconds, all without
retries. Full web typecheck, strict affected-file lint/format and documentation
checks pass. Native Playwright collection preserves all 3,957 identities across
projects, owning root specs and native test IDs. The nine new cases extend the existing one-case
file; expected frontend selection is 26,550 cases in the same 2,956 files.
The explicit addition list is reconciled against its tested source hash; no
other frontend identity or outcome change is permitted. Hosted speed and
final-head coverage still require the next complete measurement.

### Hosted validation of the retained implementation

The nineteenth full current-main set passes all 75 core jobs in 14m17s,
including all 3,957 E2E identities and 612 ACP top-level tests. Both repaired
leaf scenarios pass. A source-identical twentieth set also passes every
core job and preserves native identities/outcomes, but takes 16m05s. Keep
both results; one sub-fifteen-minute sample is not repeatability acceptance.
Task 16 records complete run IDs, queue/setup/report intervals, runner
occupancy and native report reconciliation. Further delivery measurements
retain this implementation and the existing capacity/resource boundaries.

### Repair the eighteenth run's fixture failures

Keep the single-producer implementation while repairing its two exposed leaf
failures. The cancellation fixture currently starts its interruption during task
creation, before page loading, so a loaded browser can miss the five-second
waiting phase. Add a prepared-session mode through the existing
`start_agent: false` and `session.launch` prepare APIs; load the page before submitting the
original interruption through the composer. Preserve the waiting-phase check,
zero started attempts, retained runtime, no native restore/replay, terminal
state and usable composer. Other scenarios keep their immediate-start mode.

The storage test checks persistence through bounded retention, rather than
requiring every batch to fit a loaded host's 250ms work budget. Expire the first
existing batch slot deliberately, then drive the same three-times-event-count
call bound. Expected budget exhaustion must retain a live outer context;
unexpected errors fail. Acquire the writer lease before observing settled
cursor/deletion state so an expired acknowledgement cannot hide a commit.
Check actual remaining inbox rows, completed-prefix accounting and acknowledged
result deltas. Preserve journal restart equality, canonical stdout/stderr/exit,
retired cursor and replay-expired fencing. Keep 2,000/10,000 updates, the 256-KiB
window, byte/event ceilings, existing loop bound, production deadlines and CI
retry policies.

The prepared-session unit first fails against the old fixture, then all four
helper cases pass. The original desktop cancellation case passes twice in
19.9 seconds with retries disabled; the existing phone running-cancellation
case passes in 13.6 seconds. No application UI changes. These browser checks
use current-branch production assets; complete hosted checks still validate
the actual current-base merge.

On the exact incoming-base backend source, the unchanged storage test first
passes three ordinary repetitions; this is not a natural local reproduction.
A deliberately exhausted batch then reproduces its original deadline assertion.
The repaired two stress cases pass three repetitions. A production mutation
advancing the retired cursor without deleting receipts fails the independent
row-count assertion (eleven expected versus twelve accounted), then both
scratch files are restored byte for byte. The final complete ACP package
passes all 612 top-level cases under race and atomic coverage in 30.539 seconds,
with no skipped cases. Scoped Go lint, full web typecheck, strict changed-file
lint and all 3,957 unchanged E2E identities pass. Hosted timing remains pending.

### Produce the native E2E catalog once per workflow

Candidate I removes repeated whole-suite discovery from the 34 planned parents.
Candidate H's full successful E2E result is 17m20s, versus 25m49s previously,
but occupancy is effectively unchanged (296.28 versus 295.30 runner-minutes).
All 3,957 identities remain selected. Earlier completed shard logs measure
23.4-25.8 seconds before dispatch, including repeated discovery and wrappers.
Do not attribute that entire interval to discovery or claim a hosted saving yet.

Build performs native Playwright collection once through its public reporter
API. Retain each project's actual TestCase.id and owning file suite, including
helper-declared, generated and skipped cases. Derive both cohort unit inventories
from those same rows. Publish the catalog in the existing manifest artifact,
bound to checkout SHA, workflow run ID and attempt. Planned CI parents require
that catalog, matching identity and a clean checkout, then validate the same
complete manifest and source hashes without importing all specs again. Local
runs continue to discover the working tree natively. No stale/missing snapshot
fallback is allowed in CI.

The required report step reconciles every actual project, owning spec and native
test ID against the published catalog. Missing, extra, duplicate or unidentified
cases fail the report; existing skips and retries retain their current policies.
Inspect native reporter types and exercise real multi-project/helper collection,
source/run/attempt mismatches, dirty or added spec files, missing catalogs and
report omissions before adoption. Keep jobs, process limits, browser isolation,
assertions, deadlines, permissions and required gates unchanged. Own
e2e/scripts catalog/planning/report consumers, their tests, E2E workflow and
this design record. Run focused Vitest, typecheck, strict changed-file lint,
workflow contracts, native identity comparison and full hosted measurement.

Local verification passes 63 focused catalog/planner/runner/report cases and
26 workflow contracts, including a real three-project Playwright collection
and no-browser execution. The actual retry-summary CLI accepts complete blobs,
rejects a duplicate blob and requires its catalog in CI. Replacing raw-attempt
reconciliation with aggregated cases makes the duplicate assertion fail;
restore source byte for byte afterward. A rerun consumer remains bound to the
successful Build's published producer attempt. Full web typecheck, strict
changed-file lint, required-gate/action-pinning and documentation checks pass.
The new native collector matches all 3,957 identities of the seventeenth
hosted run exactly. This is local correctness evidence; hosted timing and
occupancy for the single-producer change remain unmeasured.

### Compare four guarded processes within each existing normal job

Candidate H follows G's resource observation, without more GitHub jobs or a
larger fleet. The controlled sample runs on four CPUs with a 10-GiB cgroup,
two managed shards and unchanged clarification, LSP and thread-composer cases.
All 59 cases pass on their first attempt in 376.258 seconds. Its peak is
2,891,640,832 bytes (2.693 GiB), with no OOM event. Cgroup CPU demand averages
1.693 busy cores across the four-CPU sample. Use complete cgroup CPU
counters rather than assuming shell timing includes every orphaned descendant. The fifteenth hosted logs
confirm the existing three-process mode in shards 7, 10, 11 and 15.

If that baseline passes without memory pressure, compare the same complete
planned parent on the same four CPUs with three then four processes. Derive
planned capacity from a two-GiB reserve plus two GiB per browser/backend stack,
clamped to one through four; keep unknown memory at one. Observe the effective
cgroup as well as host available memory, and use their lower remaining budget.
The four-process mode requires four available CPUs; smaller CPU hosts retain
the prior spare-core rule. Containers remain single-process and managed local
entry points retain their existing three-shard maximum and memory thresholds.
The three-process control passes the complete 122-case parent in 405.543
seconds: 120 first passes, one retry-only pass and one existing skip. Peak
memory is 4,688,642,048 bytes (4.367 GiB), with zero swap and zero OOM/max-memory
events; complete cgroup CPU demand averages 3.335 cores. Its existing sidebar
sort retry is retained, not attributed to the process cap.

The four-process preflight is rejected before starting tests because current
host memory is below the required 10 GiB. Keep that rejection and the guard;
do not lower the threshold or override it to obtain a sample. Use one complete
hosted four-process request as the bounded experiment instead. It is not a
locally verified four-browser result. The same memory/CPU guard can reduce the
actual count, which native metadata and logs must report.
A separate fixed-admission replay of 14-28 parent jobs gives no consistent
normal-tail benefit from a smaller matrix across runs 14-16. Keep 28 jobs and
measure the four-process request first; no matrix change is adopted. Adoption still needs
complete native coverage, successful checks, measured resource headroom and
lower hosted occupancy/loaded elapsed; no local or partial sample establishes
the fifteen-minute goal.

Keep whole files, 28 normal jobs, six container jobs, independent databases,
backend isolation, one worker per process, all manifests/reports, artifact
identity checks, input handlers, deadlines and retry policies. Add tests that
first reject unsupported four-process requests, verify memory boundaries and
unknown/lower cgroup budgets, preserve exact disjoint selection, and retain
failure/cancellation/report handling with four children. No environment bypass,
new provider or paid runner is part of this candidate. Resource snapshots and
native timing accompany the hosted comparison; reject OOM, swapping pressure,
lost evidence or worse loaded feedback. Full fifteen-minute acceptance remains
three successful complete hosted sets; this experiment alone cannot satisfy it.

The initial two-process resource sample has passing native shard logs for
30 and 29 cases. Its shared blob output directory retains only the latter
report; preserve both logs and do not claim a complete 59-case structured
report. The planned-parent control uses distinct per-process blob directories
and retains all 122 native identities.

Four-process request validation passes 57 focused guard, partition, supervision,
cancellation and manifest cases, plus 25 workflow contracts, full web typecheck,
strict changed-file lint/format, native discovery, required-gate/action-pinning
and documentation checks. All 3,957 native identities remain unchanged.
Structural YAML comparison preserves all original jobs, commands, permissions,
gates and artifacts apart from the guarded request and optional observations.
Normal commit hooks pass without bypass.

Main revision `c3831c5bd057` adds session-scoped launch credentials. The exact
conflict-free synthetic merge with source candidate `dff3d4d82ba` passes its
five new Office/orchestrator credential regressions under race detection and
the existing readiness regression in its actual agent/runtime/agentctl package.
The initial backendapp selector compiled that package but ran no tests; retain
that limitation and the corrected explicit PASS. No incoming product code is
modified by this candidate. Hosted actual-merge identity remains to be checked.

Main then advances to `8c6f4b800b12` with background-work settlement. Its exact
synthetic merge with documentation head `2605ff4e75d7` passes all 45 selected
ordinary/race regressions across seven packages, with zero skipped cases.
These include the credential and readiness keepers as well as the new
background-work contracts. PostgreSQL execution stays required in hosted CI;
no local PostgreSQL execution is claimed. Frontend/E2E source and all 3,957
identities are unchanged by these incoming Go-only changes.

A separate temporary CDP probe passes all three navigation/idle/busy cases,
then removes its source byte-for-byte. Task navigation consumes approximately
6.7-6.9 cgroup CPU-seconds, mostly Chromium, versus 3.1 for the settings page.
Settled static pages consume 0.35-0.60 CPU-seconds over approximately three
seconds, while the busy page consumes 2.4. Sampling adds overhead; these are
investigation data, not an adopted speedup. If process density is insufficient,
inspect cold task-route mounting and loaded JavaScript next. Do not disable
animations, renderer/isolation behavior or unique browser assertions.

### Measure existing E2E runner headroom

Candidate G is observation only, before selecting another process budget.
Fifteenth-run full E2E takes 26m15s, with 12m33s dependency-ready delay for
the final shard. More GitHub jobs are not a demonstrated solution. An offline
whole-file replay preserves all 1,337 normal units and the three-process cap:
subdividing each parent's three static partitions into six or nine queued
chunks increases summed parent occupancy by 4.5-9.2% with a 5-10-second cold
process cost. Larger global matrices also add setup occupancy. Retain the
existing allocation; neither sensitivity model proves hosted timing.

Measure the existing normal jobs' resource envelope before proposing a higher
local process cap. Keep 28 normal jobs, six container jobs, one worker per
process, the exact guarded command, current resource guard, manifests, artifact
identity, retry policy, deadlines and required gates. Use Bash's built-in time
for the existing command's wall and reaped-child CPU counters, and capture
raw Linux cgroup CPU/memory and host budget snapshots before and after it.
Memory peak includes setup and is a conservative whole-container upper bound;
missing or unsupported counters are unknown, not zero. Cgroup CPU counters
are meaningful only after checking the recorded cgroup scope; they are the
CPU-use authority because shell timing can miss orphaned descendants.

Store the snapshots in a separate optional artifact. Keep native blob paths
and missing-report checks unchanged.
Measurement must neither replace native results nor hide the command's exit
status. Do not add a privileged sampler, package/image dependency, capacity
provider, retry, timeout or unsafe override. The observation itself changes no
budget; a larger request follows Candidate H and its derived guard. Verify the existing workflow tests,
required gates, action pinning, shell syntax, observer output, unchanged native
catalog and unchanged workflow selection. Hosted counters, CPU demand and peak
memory across representative heavy jobs determine whether Candidate H is
adopted or rolled back. Do not claim a time saving for instrumentation.

### Align runtime replacement checks with silent restoration

Candidate F: fifteenth-run shard 4, job 116142861188, fails the zero-retry
runtime-replacement case while expecting `recovery-stop-button` to be enabled.
Its error snapshot already shows a resolved recovery message and ordinary
composer. The active contract requires automatic confirmed-death restoration
and no obsolete recovery controls after commit
(AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1, .2 and .4). History
`135630f9bc` added silent restoration while retaining the former uncertain-phase
checks. Requiring a transient banner/control after runtime availability races
with correct completion; recovery must not be slowed to satisfy that assertion.

Unmodified baseline runtime/control pairs pass on desktop (21.561 seconds) and
phone (21.377 seconds). A controlled late observer waits for actual durable
restoration immediately before the original Stop expectation: that same
expectation fails with the same missing-control error in 19.156 seconds. No
production state is forged or delayed; the original test is restored byte-for-
byte afterward. Keep the real child kill, backend boot/page origin, native
conversation/execution/workspace, no replay/duplicate turns/messages, reload
and later-message assertions. Replace uncertain-phase UI expectations with the
existing durable restored-state helper and an explicit absence-of-Stop check.

| Former assertion | Retained primary owner |
| --- | --- |
| Retired runtime records uncertainty and keeps its submission identity | Native lifecycle `TestRuntimeReplacementUncertainSubmission`, `TestRuntimeReplacementRetiredDisconnectSurfacesUncertainOutcome`, and `TestRuntimeReplacementRecordsUncertaintyBeforePromptCompletionSignal` |
| Persistent uncertain notice, enabled Stop and actual stop request | Desktop/phone `durable-stream-recovery.spec.ts` state-only recovery cases, which hold recovery and exercise rejected Stop truthfully |
| Stop and phone target geometry after a real killed runtime with an independent block | `delivery-retention.spec.ts` / `mobile-delivery-retention.spec.ts` and `assertRetentionRecoveryControls` |
| Automatic native restoration, ordinary composer and no obsolete actions after commit | Existing desktop/phone runtime-replacement cases and `expectSilentRestartRestoration` |

Run the three native uncertainty keepers and desktop/phone restored-runtime plus
stable-control cases. Keep all Stop action/geometry keepers and every current
90/300-second bound and retry setting. This is an assertion ownership repair,
not a product change or a removed scenario; only Candidate E changes identities.
Verify the native catalog delta remains exactly its nine retired search cases
and three combined keepers. Full hosted failure evidence remains retained.

All three native uncertainty keepers pass in the fresh lifecycle fixture binary
(the local binary run is not race-enabled). Final desktop and phone runtime/
stable-control pairs each pass twice: four cases in 44.299 seconds and four in
40.497 seconds, with zero retries, skips or failures. The controlled late-
observer RED is retained separately. This verifies the stable restored-state
oracle and the existing state-only Stop owners; no production source changes.
The independent-block delivery-retention case also passes on desktop (49.3
seconds) and phone (49.5 seconds), using actual runtime death,
Stop click/tap and the original touch geometry. Full web typecheck, strict
ESLint/Prettier and documentation/specification validation pass. Runtime case
identities remain unchanged; only the explicit search consolidation changes
the catalog.



### Consolidate repeated panel-search setup

Candidate E: `tests/search/panel-search-shared.spec.ts` passes all twelve
baseline cases on their first attempt in 66.548 seconds. Each of session, Plan
and terminal repeats the same seeded task and real panel preparation for
S1/S7 (open, focus, empty input, counter), S3 (Escape) and S4 (Close button).
Consolidate those three cases into one control flow per concrete panel, keeping
all their assertions and both distinct close handlers. Reopen after Escape
before clicking Close. Keep the real agent/Plan seed and terminal printf/output
readiness from history `e96910ff9a`; no fake panel or new production seam.

Keep S5 query reopening as a separate case for all three panels. The earlier
12-to-3 proposal is rejected: session search explicitly clears a committed query
(AC-UI-SESSION-SEARCH-OWNERSHIP-001.2), whereas the current Plan and terminal
close handlers retain their query state. S5 currently closes before the bar's
debounce necessarily commits, so its general empty-query title does not prove
committed-query policy. Preserve those cases pending separate contract/assertion
reconciliation; do not invent a uniform policy through this latency cleanup.
Rich Plan matching and asynchronous session reply-ownership tests stay intact.

The intended delta is twelve cases to six: retire exactly nine S1/S7/S3/S4
identities and add three combined keepers. All other identities, including all
three S5 cases, remain. Expected full catalog: 3,963 to 3,957, with the same
normal/container project/file union and existing skips. Record the exact delta,
not an unexplained smaller catalog. Every original direct behavior assertion
must remain in order; only repeated fixture setup is shared.

Run the full consolidated file and record native attempt counts, retaining
the file's existing retry policy. For an independent coverage check, temporarily make only the shipped Close button callback a no-op:
the session keeper must pass its Escape checks then fail the button's expected
zero-bar assertion. Build and restore production source byte-for-byte in one
try/finally operation, rebuild the restored bundle, and repeat the complete file.
Reject the consolidation if an oracle/input handler disappears, the mutation
passes, saved source is not restored, or the measured saving is immaterial.
Do not change production behavior, retry policy or the existing 90-second bound.

The six-case trial passes in approximately 35.6 seconds versus 66.548 seconds
for the twelve-case baseline. After the Close-button mutation is rejected at
the keeper's post-click zero-bar assertion, production source is restored byte
for byte and the shipped bundle rebuilt. All six final cases pass twice in
70.385 seconds total, on their first attempt. The deliberately broken button
fails both attempts under the file's existing one-retry policy; CLI retry zero
does not override that per-describe setting. No retry policy was changed.

Native discovery reports no errors and exactly 3,957 cases. The explicit
Counter comparison removes the named nine S1/S7, S3 and S4 identities and adds
three combined keepers; every other identity and the complete project/file
union remain. Full web typecheck and strict affected-file ESLint pass. Hosted
selection and elapsed improvement still require a new complete workflow set.


### Native paste for oversized-message fixtures

Candidate D: complete thirteenth-run blob events attribute 83.108 seconds of
the desktop oversized-preview case and 82.623 seconds of its phone companion
to Playwright filling the 3,921-line composer. Their assertions protect bounded
previews, complete stored/downloaded bytes, session switching and follow-up
submission (AC-UI-BOUNDED-USER-MESSAGE-001.1 through .6). Retain those oracles,
the full generated input, desktop shortcut submission, phone touch submission,
reloads and all viewport/target checks. The local baseline passes all three
desktop cases in 1.3 minutes and its phone companion in 1.7 minutes, retries off.

Evaluate real clipboard paste for these two large inputs only. Grant clipboard
permissions to the isolated browser context, write the unchanged full source,
focus the existing ready composer and invoke its native paste shortcut. Await
the final marker in the editor before the original submission. Do not assign
DOM/store/editor internals or seed the oversized submitted message over an API.
The existing complete persisted-source/download assertions must remain exact.
Ordinary fill, typing, keyboard and touch paths retain their existing owners.
Reject this experiment if it changes saved bytes, bypasses production handlers,
fails a retained assertion or does not materially reduce measured execution.
No production change, permission bypass, retry or timeout increase is included.
Measure full files before considering any causal replacement of the phone's
independent fixed 30-second queue predecessor.

The native clipboard trial passes all original cases: the full desktop file
falls from approximately 78 seconds to 32.3 seconds and the phone file from
approximately 102 seconds to 52.0 seconds. The generated 3,921-line source,
complete persisted-source/download equality, switching, follow-up, geometry,
keyboard/touch submission and all prior assertions are retained. This is local
fixture evidence, not hosted full-workflow acceptance.

Next replace only the phone case's fixed 30-second queue predecessor with the
existing cancellation-aware `mockTurnGate`. Preserve its real running state,
queue-mode readiness, expanded preview/download checks, queue clearing and real
agent completion/session-done assertions. Release after the intermediate
assertions, and again in finally for failure cleanup. Keep the original
completion marker and every timeout. Do not change the already-short desktop
queue case, generic message entry or production code. Repeat complete files
with retries disabled before staging the coherent fixture batch.

Final verification repeats every desktop case twice (six passes in 63.282
seconds total) and the complete phone case twice (two passes in 44.727 seconds
total), with no retries, failures or skips. Oversized desktop bodies are 17.283
and 16.394 seconds; phone bodies are 17.829 and 20.796 seconds. Every original
expectation/assertion remains in order and the full 3,921-line generator is
unchanged. The native clipboard event reaches the shipped paste handler, and
complete persisted/downloaded byte comparisons still pass. Full web typecheck,
strict changed-file lint/format and native discovery also pass. Fifteenth-run
hosted evidence retains all four first-pass cases, with the large desktop body
at 38.861 seconds and phone at 27.357 seconds. That complete set fails the
independent obsolete recovery assertion and takes 26m15s; local fixture savings
do not establish whole-workflow acceptance.

History/design retain these scenarios as the integration owners for the original
large-log incident, not redundant small-string parser cases. Keep transcript,
pinned, queued, narrow fine-pointer and coarse-pointer proofs; this batch changes
only how the large log enters the editor and when one mock predecessor finishes.
Inspection of the production backoff ladder also retains the 140.9-second
capacity-exhaustion scenario: its five real waits are 5, 10, 20, 40 and 60
seconds, and one episode proves both phone and desktop behavior. Do not replace
that distinct default-scheduler integration with a faster synthetic timer seam.

### Prepared workspace experiment for layout-only scenarios

Candidate C: right-panel visibility's ten cases currently create an agent task
and wait for a complete initial turn before inspecting layout. All ten pass
without retries in 1.1 minutes locally. The UI-owned requirement explicitly
excludes task and terminal lifecycle; retain every existing layout assertion,
reload, preset, resize, keyboard/touch and real user-shell check.

Evaluate the existing production `POST /tasks` `prepare_session:true` path
for the nine non-keeper cases. It prepares a real session and workspace through
`IntentPrepare`/`LaunchWorkspace`, without starting an agent. Keep the desktop
hide/restore case on its original real-agent path as the completed-turn keeper.
Prepared sessions remain CREATED on navigation because the existing resume
policy excludes that state. No preference override is needed or retained.
No backend endpoint, fake environment, assertion, timeout or new seam is added.
Run the complete original file with one worker and no retries, compare its
test identities and body durations, and reject if an assertion loses its real
environment or terminal prerequisite. Relevant production owners are
`prepareTaskSession`, `LaunchSession`, and the unchanged right-pane toggle and
Dockview persistence. History retains layout, Git refresh, runtime notification
and durable-session updates. Extend only after the sample demonstrates a
material saving and its state/coverage differences are understood.

The first pair preserves all ten original identities and every original action
and assertion byte-for-byte after the setup options. Summed bodies fall from
64.4 to 42.8 seconds. A second prepared pass adds after-test assertions that
all nine prepared sessions remain CREATED with real environment IDs: all ten
pass, 45.1 seconds summed bodies. The unchanged native-touch phone companion
passes in 7.3 seconds full-run time. This small sample is not hosted attribution.

Next evaluate `openWideTask` in `dockview-resize.ts`: its 13 callers protect
pane widths/caps, resize/maximize/restore, sidebar sash and tab-strip geometry.
The complete four-file baseline passes all 16 cases in 1.1 minutes. Retain the
three cases with their own live/multi-task setup and every existing assertion;
only the shared helper may use the production prepare path. Assert CREATED
state and a real environment before handing the settled layout to a caller.
No source beyond that helper changes in this second experiment. Reject the
extension if startup is still required by a caller or the measured saving is
too small. The original right-panel live-agent and phone keepers stay intact.

The four-file prepared pass retains all 16 cases and reduces summed bodies
from 63.6 to 51.0 seconds. The final combined five-file run passes all 26 cases
in 1.7 minutes with no preference override, retries disabled, and unchanged
original interaction/assertion bodies. Prepared-session state and real workspace
checks pass before layout use; right-panel cases also retain those checks after
the complete flow. These are local fixture-cost results. Whole-workflow savings
and contention impact require the next complete hosted measurement.

### Shared-capacity investigation after the twelfth run

Full E2E run 38634314141 succeeds in 24m37s. Normal jobs require only change
detection and build, yet their starts span 20:47:47 through 20:55:47 UTC after
build completion at 20:47:45. All run on GitHub-hosted Ubuntu. Other PR workflows
overlap this interval; dependency-ready delay includes scheduling/provisioning
and cannot identify the exact account-wide capacity cause by itself.

Do not grow the matrix. A read-only diagnostic replans the exact 1,337 native
units/3,810 tests using this run's selected successful-main profile 38628885177
and evaluates its observed final test-body costs with three guarded processes.
The longest body allocation grows from 476.6 seconds at 28 jobs to 555.6 at
21, 651.2 at 18, and 791.4 at 14. Replaying observed ready-slot arrivals with
fixed setup overhead shows no material improvement from those smaller matrices.
This model excludes retry costs and changes to fleet scheduling, so it is not
a hosted comparison or an unconditional rejection of fewer jobs. It does not
justify adopting a new count. Retain the existing matrix until a candidate
reduces measured work or loaded completion time.

The last shard's recorded fixtures take 105.4 seconds in before-hooks and
48.1 in after-hooks across 154 attempts; these overlap child fixture timings.
It also has two 90-second timeouts waiting for a missing proceed button and a
94.6-second oversized-message case. Retain assertions and lifecycle coverage;
do not attribute those isolated observations to a proven whole-suite saving.
No test, fixture, resource guard or runner-placement change is adopted from
this investigation. The next full set measures warm host build/cache behavior
without application, test or workflow changes.

### Runtime viewport flow audit

Audit the three desktop-project cases `narrow fine-pointer runtime flow retains
phone controls` and `runtime settings controls respect the 767px/768px phone
boundary` in `agent-runtime-notifications.spec.ts`. Their initial local bodies
take 16.2/16.5/16.0 seconds, without retries. Each calls the same full flow as
`runtime notices, saved consent and native guidance work outside Settings`
(16.0s) and the native-touch `phone runtime notices lead to saved policy,
native guidance and version drawer` (16.7s). Baseline: all five pass.

Candidate C (evaluated and rejected): keep all three viewport cases, but consolidate their repeated
policy persistence, unknown-version/manual ownership, and retained activation
failure checks into the unchanged desktop and real-touch full-flow keepers.
Those checks exercise the same policy hook/status branch independently of
width; both keepers retain every assertion, including draft retention, no write
before Save, exactly one write, reload persistence, manual guidance, no manual
switch, explicit unknown state and failed activation messaging. Retain each
viewport's full compact settings disclosure/ordering/deep-link/focus checks,
native and grouped toast bounds/touch links, floating-indicator absence,
version dialog/drawer content and page-overflow assertions. Retain no accidental
policy writes in the focused layout checks. Backend notification/reconnect and
native-host fallback suites stay independent and unchanged.

History: runtime notification #4148, settings #4156 and grouped notices #4211.
Contracts: runtime notification AC001.1/3/5/6/7 and automatic opt-in AC002.1;
production owners are AgentRuntimePolicies, useRuntimeAutoUpdatePolicy,
settings shared control sizing and UpdateAvailableToastBridge. The fixture's
status and policy-write controls have no production callers. No seam or
production code is deleted. Main risk is accidentally losing a viewport-specific
check while extracting phases; compare the original full-flow assertions and
run all five original identities after the change. Measure execution separately
from fixture setup; local savings are diagnostic, not full CI attribution.

The extracted candidate passes all five identities without retries, full typecheck
and strict lint. Desktop keeper remains 16.0s; phone keeper is 16.5s versus
16.7s. Three layout bodies total 43.1s versus 48.7s, only 5.6s saved across
three cases (11.5%). Reject this candidate as too small for its added helper
complexity; restore both source files byte-for-byte to head 67a6c3cf430.
Final disposition R: retain the original cases and every existing assertion.
No test change from this experiment is adopted or attributed to hosted CI.

### Remove browser images from browser-free prerequisites

Tenth build job 116067615073 spends 63 seconds initializing its browser-based
container and only 20 seconds building backend/web. Report job 116074107123
spends 39 seconds initializing another browser container although blob merging
does not launch a browser. Those 102 seconds sit on the serial E2E dependency
path. Bootstrap the existing Go version, Node 24 and pnpm 9.15.9 directly on
the same GitHub-hosted Ubuntu runners for build and report only. Keep browser
and desktop jobs, every build command, artifact identity and report gate intact.

Resolve the installed pnpm store instead of assuming a container home. Give
the host store its own key to avoid restoring archives rooted at `/root`;
build publishes it and report only restores. Disable setup-go's automatic cache
because the existing Go cache action remains the sole owner and still checks
compiler/environment compatibility. Retain tool setup, first cold population,
uploads and queue time in hosted measurements; the 102-second gross opportunity
is not a promised net improvement. Verify bootstrap/version ordering, mandatory
build/report execution and unchanged required jobs before pushing one candidate.

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
mobile queue browser cases, retries disabled. Ninth hosted evidence confirms
the unchanged stress pair falls from 348.27 to 16.78 seconds and the ACP package
from 375.557 to 42.322 seconds. Whole-backend runner-minutes fall 14.0%, but
full-pipeline elapsed remains above fifteen minutes; do not claim completion.

### Preserve sidebar action state across reveal (tenth candidate)

Sixth-run navigation hierarchy retries lose the New Task dialog after its
Cancel button appears. The collapsed and revealed Kanban sidebar place the
navigation subtree under different React parents, so a pending hover timer can
remount the local dialog owner. Three original one-CPU repetitions pass; do not
claim a local reproduction until a controlled browser clock crosses that timer
while the original dialog is open.

Prove the existing portal/state-continuity contract
AC-UI-SIDEBAR-HOVER-001.3 without removing the native rail click or any assertion.
If confirmed, preserve component ownership through the visual transition,
retaining existing rail/expanded geometry and all hover, focus and saved-state
semantics. Do not disable hover, animation, pointer actionability or timeouts.
Unit checks must retain child state and hide divider controls in the rail;
browser checks cover navigation actions, hover/portals and split resizing.
The existing phone drawer (mobile-sidebar-hover-reveal) remains the mobile
entry point: tap controls, one drawer scroll owner and no horizontal overflow.
Verify that scenario without introducing a phone hover rail or changing the
saved desktop preference. The Bitbucket retry is investigated independently.

The controlled browser clock reproduces the disappearing dialog before the
fix, after the original rail click and visible-dialog assertion. Keeping the
same navigation and task owners mounted through reveal fixes that failure.
Three strengthened browser repetitions pass with retries disabled, including
the actual hover-revealed state. All 14 desktop navigation/hover/width-resize
cases, six customization/split-persistence cases and the existing phone drawer
case pass against a fresh E2E web build, also without retries. The phone drawer
remains within the viewport and navigation introduces no horizontal overflow.
The first phone command selected a nonexistent project and ran no tests; the
corrected `mobile-chrome` run supplies the passing evidence.

All 37 focused sidebar/hook unit tests and full web typecheck pass. Strict
changed-file ESLint and formatting pass. No existing test identity, interaction
or assertion is removed. Hosted retry and elapsed-time impact remain unmeasured.

### Bound Windows cache publication (review follow-up)

Exact-head review and ninth hosted evidence identify per-commit Windows build
cache publication and duplicate ownership through setup-go. The compile job
alone spends 137 seconds restoring and 106 seconds publishing its build cache.
Repository cache usage is 22,821,747,222 bytes across 37 entries at inspection;
do not infer a quota or change any cache setting.

Use a UTC daily generation for the existing suite/toolchain/architecture/module
identity, retaining the compatible restore prefix and one shared process-suite
key across four cohorts. Disable setup-go caching, retain module downloads in
one separate optional module-only cache, and make compiled-cache failures
optional. Only successful process cohort 1 publishes the shared process build
generation; other suites publish their own build generations. Only the compile
suite publishes the common module cache. Exact hits skip publication.
Every original build, vet, race test, native selection, deadline and
required gate must remain unconditional and unchanged. First update the existing
workflow contract to reject the SHA key and duplicate cache owner; verify it
fails before changing configuration. Remove the sidebar restore prefix that
cannot match its different cache path set. Correct stale process-count prose.
Keep the helper compilation job's compatible legacy fallback during migration;
defer the optional gate-helper refactor because it changes no timing or coverage.
Measure the next complete hosted set, including any first daily-generation cost.

The updated cache contract fails before the configuration change and all 25
backend workflow cases pass afterward. A separate structured YAML comparison
proves every Windows non-cache step, native command, matrix, deadline,
environment and permission is unchanged; all other backend jobs are unchanged
apart from the dead sidebar fallback. Required-gate, runner-placement and action
pinning checks pass, as do the full 27-workflow pin scan and docs/spec validation.
This Linux host has no PowerShell runtime; actual Windows restoration,
publication, UTC generation and the unchanged native tests remain hosted gates.
No Windows timing savings are claimed before that measurement.
