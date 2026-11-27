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

### Twenty-eighth candidate: bounded fixture disposal and retained accordion state

Use a one-second deletion grace only for disposable Kind workload cleanup,
retaining synchronous deletion, the original timeout and explicit empty-resource
confirmation. Product Pod templates, lifecycle calls, ownership/UID protection,
PVC retention and every test body remain fixed. The full six-spec Kubernetes
group passes seventeen cases with the same four existing skips and no retries.
The twelve matching executor cases retain identical passed IDs/outcomes;
cleanup steps fall from 137.231 to 109.062 seconds (28.169 seconds, 20.53%).
Other thirty-second reset waits remain. No full-workflow gain is claimed from
these component timings or from the differently scoped local run totals.

Replace the portable-config scenario's duplicate accordion-closing click with
an assertion that the original opener remains expanded. Preserve independent
auth/config selection, persisted values, reload, retries and deadlines. The
focused case passes in 8.4 seconds without retries. All 3,957 native E2E IDs,
26,553 frontend cases and 35,581 expected backend executions remain selected;
no product or unit-inventory changes accompany these two test-support edits.
Strict lint/format pass. Full hosted validation remains pending; acceptance 1/3.

The first local Kubernetes attempt is invalid: an old generated source marker
mismatches newly compiled binaries. It is interrupted after one failure, with
one interrupted case and ten unrun cases; both owned clusters are removed.
After a canonical fresh backend/helper build and six-artifact marker refresh,
the unchanged twelve-case baseline passes in 6.8 minutes with retries disabled.
Keep that setup failure separate from the valid before/after comparison.

### Twenty-seventh complete hosted result: portable-config timeout

Head `9576aed18de4` passes backend `38690664361` in 19m01s (214.42
runner-minutes) and frontend `38690664433` in 12m27s (29.77). Full E2E
`38690664202` fails in 16m02s (286.03), with an 855-second execution critical
path. Complete elapsed time is 19m01s and occupancy is 530.22 runner-minutes.
All 75 core jobs are selected. This failed, over-budget sample does not count;
acceptance remains 1/3. Backend Postgres cohort 2 runs for 1,114 seconds with
two seconds dependency-ready delay; do not attribute that execution to queueing.

Exact source/run/producer reports retain all 3,957 E2E identities: 3,907 first
passes, nine retry passes, one final timeout and forty existing skips. All ten
remote-picker cases pass first attempt. Native raw audit covers 118 reports,
3,972 attempts, fifteen retry attempts and sixteen error results with no parse
errors. Ten backend native cohorts retain all 35,581 expected executions and
outcomes. Frontend retains 26,553 cases in 2,956 files. One backend artifact
arrives after the initial download; collecting that missing named cohort then
completes the exact identity audit without replacing any existing evidence.

Only the portable-config case fails finally, on four attempts. Its second
click on the already-open Mock Codex accordion closes the target; native steps
record 51-55-second waits on the disappearing auth checkbox in three attempts.
The keyboard integration scenario retries but passes finally; no zero-flake
claim is made. Normal E2E tail shards incur 216-223 seconds scheduling/provisioning
delay of unknown cause. The report takes 68 seconds and correctly preserves
the failed test result.

Twenty-seven normal resource artifacts show four CPUs, 7.93-9.75 GiB lifetime
peaks and zero observed swap/memory events. Shard 18's optional artifact is
absent from the complete 72-artifact listing despite successful capture/upload
steps; its resource outcome is unknown, never zero. All mandatory tests and
native reports remain accounted for. Fresh exact-head evidence has 89 passed
checks, two failed E2E contexts, eleven ancillary skips and zero pending, with
no evidence errors or unresolved/hidden threads/current-head review bodies.
No hosted set or PR monitor remains live before candidate 28.

### Twenty-seventh candidate: remove duplicate sidebar restoration

Candidate 26's only test failure is the remote-picker pasted-issue case's
layout-restoration hook, which races the live browser's revisioned layout write.
Task 15 records the shared page fixture as the single owner of per-test reset.
Remove only this suite's layout snapshot and afterEach restore. All ten test
bodies, assertions, zero retries and deadlines are byte-identical. The unchanged
baseline passes ten cases in 32.9s against a fresh managed build; after cleanup
removal they pass in 31.0s with retries disabled. Strict lint/format and native
collection pass, preserving all 3,957 E2E identities. No production or unit-test
inventory changes are made. Full hosted timing/coverage remain pending.

### Twenty-sixth complete hosted result: bootstrap restored, cleanup failure

Head `ad685333e05e` passes backend `38688697619` in 11m46s (194.32
runner-minutes) and frontend `38688697677` in 11m04s (25.30). Every Windows
job executes and passes. Full E2E `38688697738` fails in 15m49s (283.63),
with a 938-second execution critical path. All 75 core jobs are selected;
complete occupancy is 503.25 runner-minutes. This failed, over-budget set does
not qualify; acceptance stays 1/3.

Exact source/run/producer reports retain 3,957 E2E identities: 3,909 first
passes, seven retry passes, one final failure and forty existing skips. Raw
audit covers 118 native reports and 3,967 attempts, with zero parse errors;
ten retry attempts and eleven failed/timed-out attempt results reconcile with
the final outcomes. All ten native backend cohorts retain 35,581 expected
executions/outcomes. Frontend retains 26,553 cases in 2,956 files. All 28 normal
resource snapshots retain four CPUs, zero swap/memory events and 7.98-9.12 GiB
lifetime peaks.

The failed remote-picker case reaches an afterEach layout restore and receives
a real 409 revision conflict; its retained product assertions are unchanged.
The suite already has zero retries. Container shard 1 passes without retry but
finishes last: 707 seconds total, including an eighteen-second checkout,
66-second pinned-image pull and 562-second test step. The required report takes
65 seconds afterward. Some normal shards wait 138-212 seconds after Build;
these scheduling causes remain unknown. Do not attribute this sample's tail to
the previous run's managed-PVC retry: all container cases pass first attempt.
Fresh exact-head evidence has 89 passed checks, two failed E2E contexts, eleven
ancillary skips and zero pending, with no evidence errors or unresolved/hidden
threads/current-head review bodies. No monitor remains live before candidate 27.

### Twenty-sixth candidate: restore the unavailable Go setup archive

Replace the unavailable setup-go pin consistently in six workflows with an
available official v7 commit. Task 15 records its exact compatibility boundary:
the Go SDK versions, inputs, cache settings, matrices, permissions, test commands
and deadlines remain fixed. The new action includes upstream runtime dependency
updates; it is not claimed byte-identical. ZIP and tar downloads validate, and
structured workflow comparison permits only the fifteen action-reference edits.
Backend (25), E2E (27), preview (7), release (65) and action-pinning (9) contract
cases pass, together with the full 27-workflow pinning scan. Release contracts
initially ran without Node on PATH; all 65 pass with the existing Node 24 toolchain.
Actual Windows compile/process execution and the complete hosted set remain
required. No application or test inventory changes accompany this repair.

### Twenty-fifth complete hosted result: E2E passes, Go bootstrap fails

Head `4ff6b68b448a` completes full E2E `38684915750` successfully in 15m31s
(289.33 runner-minutes), with a 923-second execution critical path. Its repaired
blob transfer takes fourteen seconds and all native reports and required gates
pass. The exact source/run/producer catalog retains all 3,957 E2E identities:
3,907 first passes, ten retry passes, zero final failures and the same forty
existing skips. Frontend `38684915678` passes in 12m52s (25.65 runner-minutes),
preserving 26,553 cases in 2,956 files.

Backend `38684915704` fails in 17m08s (205.70 runner-minutes). Windows compile
and process 4/4 cannot download the pinned setup-go archive before checkout or
tests. The ancillary preview fails at the same bootstrap. Those Windows
executions are missing and cannot count as retained coverage. All ten Linux
native-report cohorts retain 35,581 expected executions and outcomes, including
only the approved Office regression addition. Normal E2E resource snapshots
retain four CPUs, zero swap/memory events and 7.98-9.05 GiB lifetime peaks.

All 75 core jobs are selected; complete occupancy is 520.68 runner-minutes.
The full set takes 17m08s, fails and exceeds 900 seconds; acceptance stays 1/3.
Fresh exact-head evidence has 84 passed checks, four failed contexts, eleven
ancillary skips and zero pending, with no evidence errors or unresolved/hidden
threads. The next candidate repairs only the demonstrated archive failure;
performance work on otherwise acceptable jobs near seventeen minutes is out
of scope for this iteration, following the user's latest direction.

### Next candidate: native CLI blob transfer

Replace only the failing reporter blob download with the existing GitHub CLI.
Each of the same three attempts uses fresh staging and the same ten/thirty-second
waits. Validate all inputs and publish the complete directory through a rename;
never overwrite duplicate basenames or expose partially copied evidence.
All native parsers, catalog checks, merged JSON/HTML reports and required gates
remain unchanged, as do every test, retry policy, worker and selected job.

Eight helper cases, 27 E2E workflow contracts, five gate checks, nine pinning
checks and the full 27-workflow pin scan pass. The new route/wiring checks fail
before configuration changes; the interrupted-copy case fails before atomic
publication. The final helper retrieves all 152 actual files in 23.354 local
seconds, preserving every byte. The unchanged parser reconciles the same 3,957
native identities with identical attempts, outcomes and durations. This is
local transport validation, not a successful hosted timing measurement.
Application/native test inventories remain 26,553 frontend, 35,581 backend
executions and 3,957 E2E cases. Full hosted validation remains pending; acceptance
is still 1/3.

### Twenty-fourth complete hosted result: all test jobs pass, blob transfer fails

Head `345f440e1048` passes backend `38682043078` in 13m30s (211.85
runner-minutes) and frontend `38682042954` in 12m08s (26.37). Full E2E
`38682043063` fails only its required reporter in 16m29s (276.98), with
856 seconds execution critical path. Complete occupancy is 515.20 runner-minutes.
All 74 other core jobs pass; all 75 core jobs are selected.
The failed set exceeds 900 seconds and does not count. Acceptance remains 1/3.

The report's blob download fails all three attempts, leaving a partial ZIP.
GitHub CLI independently recovers all 34 artifact folders and 118 native ZIPs,
each CRC-valid. Local raw reconciliation with the exact source/run/producer
catalog retains all 3,957 identities: 3,913 first passes, four retry passes,
zero final failures and the same forty existing skips. This is local forensic
evidence; no successful hosted report or pipeline is claimed. All five Office
cases and all four mobile action-bar cases pass on their first attempts.

Frontend preserves all 26,553 expected identities/outcomes in 2,956 files.
All ten backend native-report cohorts preserve the prior 35,580 executions and
outcomes plus exactly the new source-bound passed Office regression, for 35,581.
The remaining required failure is report transport, not test execution. Fresh
exact-head evidence has 88 passed checks, one failed aggregate, ten ancillary
skips, zero pending checks, and no errors, unresolved/hidden threads or current
review bodies. Task 15 records the bounded transfer repair; no new set is active.

### Twenty-fourth candidate: preserve a live Office session on duplicate launch

The real executor-guard reproduction fails before the repair: the losing
Office start changes the live session to `FAILED` without dispatching another
agent. The general start caller now returns that typed refusal before failed
launch bookkeeping, matching the prepared-session caller. Three focused checks
pass afterward, including actual persisted execution identity, genuine failure
settlement and ordinary Office session reuse. The identity check initially
seeded only the session projection; the corrected fixture uses the canonical
runtime ledger and verifies its initial state before exercising the start.

Remove the file-panel case's unsupported generic fresh-agent recovery fallback.
Retain its native identity, navigation, visible Files panel, exact `.gitkeep`
row assertion, original timeout and retries. All five Office advanced-mode
cases remain, and native collection retains all 3,957 E2E identities. Strict
affected-spec lint/format pass. The temporary reproduction is replaced by one
permanent Go regression, for 35,581 expected top-level backend executions;
frontend remains 26,553 cases. The hosted comparison permits only this named
passed addition in its existing ordinary cohort, bound to the test-source hash.
The affected Office/start cases pass with the race detector, and affected Go
lint reports zero issues. All five Office browser cases pass against a fresh
backend/web build in 29.6 seconds with retries disabled. Documentation catalog,
spec lint and diff checks pass. Hosted timing and full coverage remain pending;
acceptance remains 1/3 and no next hosted set is running yet.

### Twenty-third complete hosted result: one Office failure

Head `aae902e2468e` passes backend `38678701709` in 12m35s (207.43
runner-minutes) and frontend `38678701852` in 12m40s (27.00). Full E2E
`38678701770` fails in 15m41s (278.93), with an execution critical path of
823 seconds. Complete occupancy is 513.37 runner-minutes. All 75 core jobs
are selected. The failed set exceeds 900 seconds and does not count;
accepted complete sets remain 1/3.

The source/run/producer-attempt-bound native reports retain all 3,957 E2E
identities: 3,906 first passes, ten retry passes, one final failure and the
same forty existing skips. Frontend preserves all previous identities and
outcomes plus exactly the approved nine gesture and three readiness cases:
26,553 cases in 2,956 files. All ten backend native-report cohorts preserve
35,580 identities/outcomes. Normal resource snapshots retain four CPUs,
zero swap or memory events and 7.93-9.24 GiB lifetime peaks.

All four browser cases changed in this candidate pass on their first attempts:
both phone cancellation cases, preview queueing and sidebar presentation.
The remaining failure is Office advanced mode's workspace-files case, on
both attempts. Its trace shows the existing session changing from `IDLE`
to `FAILED` with `execution already running`; the generic fresh-session
fallback then fails the scheduler-owned Office launch-context check. The
unchanged case passes locally in seven seconds. Diagnose the losing-launch
failure path before changing product code or replacing the file-tree oracle.

Exact-head checks are terminal: 87 passed, two failed contexts (the leaf and
required aggregate), ten ancillary skips and zero pending. Evidence is complete,
with no unresolved or hidden threads, current-head review bodies, or errors.
No new hosted set is active while the remaining failure is investigated.

### Twenty-third candidate: causal startup, queue and persistence checkpoints

The shared Quick Chat baseline now requires both a completed opening turn and
the existing addressed-session settled state. Preview uses the existing real
mock-turn gate until the queue chip appears, then releases it for delivery and
again on cleanup. Sidebar compact text is checked separately from its retained
screen-reader label; selection persistence is observed before testing reload.
There are no application changes, retry increases or longer deadlines.

Local verification passes four session-helper unit cases, both phone
cancellation cases, the complete preview case and the sidebar presentation
case, with browser retries disabled. Native collection preserves all 3,957 E2E
identities. Exactly three readiness unit cases join the prior nine gesture
additions, for 26,553 frontend cases in 2,956 files. Full type/lint/format and
docs checks pass. The complete hosted set and current-head gates/reviews remain
required; accepted fifteen-minute sets are still 1/3.

### Twenty-second complete hosted result: failed execution and report transport

Head `eaf927ca2136` passes backend `38674620312` in 11m35s (199.90
runner-minutes) and frontend `38674620360` in 12m22s (27.05). Full E2E
`38674620296` fails in 18m11s (285.00), including 140 seconds in a blob
download step that exhausts its existing retry policy. Complete occupancy is
511.95 runner-minutes; E2E execution critical path is 877 seconds. Acceptance
remains 1/3. Do not count the failed set or a locally reconstructed report.

All 34 uploaded blob artifacts are recovered read-only and their raw attempts
reconciled against the actual merge/run/producer-attempt catalog. All 3,957
identities remain: 3,905 first passes, nine retry passes, two final failures
and forty-one skips. The extra skip is the dependent second Quick Chat case
after its serial predecessor fails, not an intentional coverage exemption.
The hosted retry summary is missing because artifact download failed; this
local recovery does not change the hosted gate or its elapsed time.

Both preceding repairs pass first-attempt: settled tool completion in 11.257
seconds and delayed native drag release in 23.507 seconds. Final failures are
Quick Chat startup/cancellation and preview queueing. The sidebar presentation
case passes after two failures, exposing save-before-reload and compact versus
accessible-text assumptions. Task 15 records the causal fixture repairs.

Frontend preserves all previous identities/outcomes plus exactly nine approved
component cases (26,550 total, 2,956 files), bound to their tested source hash.
All ten backend native-report cohorts preserve 35,580 identities/outcomes.
Normal resource snapshots retain four CPUs, zero swap or memory events, and
7.85-8.98 GiB lifetime peaks. Exact-head checks finish with 86 passes, three
failed contexts, ten ancillary skips and no pending checks or evidence errors.
The three required contexts preserve the two shard failures and report failure.

### Next candidate: preserve the two failing browser contracts

Task 15's bounded repair suppresses a drag's residual native click across
workflow rows until it is consumed or a new primary pointer gesture starts.
Keyboard activation remains available and unmount removes the listeners.
The original lane test now crosses the old sensor timer boundary before
release, then retains every growth/shrink/height/overflow assertion. The tool
case waits for the settled two-call group, opens it, verifies both Terminal
rows and checks no spinner before and after expansion. The script still
omits tool results, preserving the turn-completion safety-net trigger.

Local verification passes ten component checks, twelve desktop browser cases
and two existing phone navigation/scroll cases, with browser retries disabled.
Full type/lint/format and docs validation pass. All 3,957 E2E native identities
remain; frontend adds exactly nine named component cases (26,550 total).
This is a locally verified repair, not another accepted hosted measurement.
Fifteen-minute acceptance remains 1/3 and the PR remains open.

### Twenty-first complete hosted result: preserve two browser failures

The evidence-only head `b3629e9a59a1` runs the same executable/config/test
source as the preceding two samples. Backend `38670906969` passes in 12m50s,
frontend `38670906967` passes in 12m09s, and E2E `38670906951` fails in 15m36s.
Runner occupancy is 207.37, 29.77 and 286.13 minutes (523.27 total). E2E
execution critical path is 773 seconds. This failed set is not accepted;
fifteen-minute acceptance remains 1/3.

All 3,957 native E2E identities match the source/run/producer-attempt-bound
Build catalog: 3,911 first passes, four retry passes, two final failures and
forty existing skips. Frontend preserves all 26,541 identities/outcomes and
the ten backend native-report cohorts preserve all 35,580 executions/outcomes.
All 28 normal resource outcomes have four available CPUs, no swap or memory
events, and lifetime cgroup peaks of 7.98-9.11 GiB.

The failures are the settled-tool locator in normal shard 22 and native drag
release in shard nine, each exhausting its existing retries. Task 15 records
their screenshots, trace-based cause and bounded repair scope. The required
aggregate preserves failure. The sole monitor finishes exit one after matching
terminal snapshots; the exact-head snapshot has 86 passes, three failed
contexts, ten ancillary skips, zero pending checks and no evidence errors,
unresolved/hidden threads or exact-head review bodies. Retain this failed
measurement before publishing a repaired candidate.

### Twentieth complete hosted result: successful repeat above budget

Head `330685e321c58` and actual PR merge `61c9e46c2995` have exactly the
same source tree as the accepted nineteenth run. All three complete
workflows pass: backend `38668854073` in 12m19s (208.73 runner-minutes),
frontend `38668853861` in 12m40s (26.55), and E2E `38668853888` in 16m05s
(277.62). Complete elapsed is 965 seconds and occupancy is 512.90
runner-minutes. Every one of the 75 core jobs succeeds. This repeat exceeds
900 seconds, so acceptance remains 1/3. Do not report only the favorable
14m17s sample or claim a sustained fifteen-minute ceiling.

The Build catalog binds the actual merge, run and producer attempt one.
All 3,957 E2E identities remain: 3,907 first passes, ten retry passes, forty
existing skips and zero final failures/timeouts. Frontend preserves all
26,541 case identities/outcomes and 2,956 files. The ten backend native-report
cohorts preserve all 35,580 top-level executions and outcomes exactly from
run 19: 34,956 passes and 624 existing skips. Both repository modes retain
all 1,932 native names, disjoint complete selection and source provenance.
Other mandatory Windows/PostgreSQL18/resource checks also succeed.

Build readiness is 158 seconds; the final-finishing normal shard 13 waits
132 seconds, runs 605 seconds, then reporting/completion adds 70 seconds.
E2E execution critical path is 826 seconds versus 802 previously. E2E
occupancy decreases by 1.85 runner-minutes while full elapsed increases by
108 seconds. Retain that scheduling/retry variation rather than attributing
all elapsed movement to code or inferring an account quota. All 28 resource
outcomes have four available CPUs, zero swap or memory events, and lifetime
cgroup peaks of 7.72-9.11 GiB. The preceding accepted run's range is
8.00-9.19 GiB, also without swap or memory events.

Continue sequential complete measurements on the retained implementation.
The maintainer accepts jobs around seventeen minutes and asks to focus on
full E2E; this repeat adds no new optimization of those close jobs. Required
final-head checks/reviews and the authorized merge remain outstanding.

### Nineteenth complete hosted result: first accepted set

The full current-main merge at `5d6ac7137d2e` (PR head `7877b26ecbf5`)
passes all three complete workflows: backend run `38666763988` in 12m46s,
frontend `38666764000` in 12m16s, and full E2E `38666763963` in 14m17s.
All 75 selected core jobs succeed, including PostgreSQL 16/18, every native
Windows cohort, both frontend shards, all 28 normal and six container E2E
shards, desktop smoke and both Kubernetes compatibility checks. Complete-set
elapsed is 857 seconds; occupancy is 200.65, 29.65 and 279.47 runner-minutes
respectively (509.77 total). Execution critical paths are 758, 730 and 802
seconds. Accepted complete sets are now 1/3; this is not final delivery.

Every one of the 3,957 native E2E identities matches the previous catalog:
3,910 first passes, seven retry passes, forty existing skips and zero final
failures/timeouts. Frontend retains all 26,540 prior case identities/outcomes
plus the deliberate continuation fixture regression case (26,541 total,
2,956 file identities). All 612 top-level ACP identities remain and pass;
the bounded storage case takes 7.58 seconds. The complete-set acceptance
receipt checks native source/run/producer-attempt provenance, job-name
multisets, mandatory outcomes and reports, not just a green status rollup.

The four ordinary Linux, three SQLite repository and three PostgreSQL16
repository artifacts contain 35,580 top-level executions: 34,956 passes
and 624 existing skips. Both repository modes enumerate the same 1,932
native names, select them exactly once across their three cohorts and
validate complete outcomes against that selection. All six manifests bind
to the actual tested merge. Other mandatory platform/resource jobs remain
covered by successful required outcomes and selector contracts.

E2E Build is ready after 2m41s. The final-finishing normal shard 26 waits
3m29s after Build and runs 7m14s; reporting/completion then adds 53 seconds.
The previous successful full E2E was 17m20s, so this sample is 3m03s (17.6%)
faster. E2E occupancy falls 16.82 runner-minutes (5.7%), with queue and retry
variation retained. This comparison does not establish a sustained ceiling.
The failed eighteenth sample remains visible and does not count.

The sole PR monitor finishes after matching terminal snapshots: 89 passed,
nine ancillary skipped, zero failed/pending, with complete evidence. The
post-wait exact-head snapshot has no unresolved/hidden threads or errors.
Exact-head review bodies are empty; that is not substantive review approval.
A normal integration of main into the PR branch produces head
`330685e321c58` with exactly the same source tree as this accepted PR merge,
so fresh measurements also cover the current-main source. No new speed
change, paid capacity, skipped coverage or retry increase accompanies it.

### Eighteenth complete hosted result

The complete set at `86ab60445c37` is not accepted: backend run
`38663231645` fails in 13m37s, frontend `38663231641` passes in 8m54s and
E2E `38663231680` fails in exactly 15m00s. Occupancy is 204.83, 21.57 and
286.38 runner-minutes respectively (512.78 total). E2E includes 2m20s to
Build readiness, 11m33s to the final container shard and 1m07s reporting.
All 3,957 selected native identities match: 3,906 first passes, ten retry-only
passes, one final failure and forty existing skips. Acceptance remains 0/3.

The catalog producer artifact is bound to actual merge
`c8b1ee07abab72633425b46f161efd9bb5b2cfc9`, this run and producer attempt one.
Normal shard nine dispatches in 2.155 seconds after its test step starts;
earlier observed parent setup took 23.4-25.8 seconds. E2E occupancy falls
9.90 runner-minutes from the preceding candidate, with workload/retry/queue
variation retained. All 28 normal resource snapshots show zero swap and memory
events, with a maximum lifetime cgroup peak of 9.13 GiB.

The two leaf failures are a storage fixture requiring success from every
250ms retention batch, and cancellation missing its waiting phase because the
fixture launches before browser startup. Their aggregates preserve failure.
The new report's exact native-attempt reconciliation succeeds. Current-head
checks finish with 85 passes, four failures and nine skips, no pending checks,
review threads or exact-head review bodies. Both leaf failures require repair
before another complete measurement; the failed timing is not delivery.

### Seventeenth complete hosted result

All three fully selected workflows pass at `d7aba03e20a3`: backend run
`38659682573` takes 16m05s, frontend `38659682488` takes 12m53s and full E2E
`38659682719` takes 17m20s. E2E includes 4m01s to Build readiness, 12m00s to
the final shard, then 1m19s reporting. All 3,957 native identities match:
3,905 first passes, twelve retry-only passes, forty existing skips and no
final failures. Required checks finish with 89 passes and nine skips; no
unresolved/hidden threads or exact-head review bodies are present.

The 8m29s elapsed improvement from the sixteenth run is not evidence of less
work: E2E occupancy is 296.28 versus 295.30 runner-minutes, and execution
critical path is 963 versus 872 seconds. Build is ready earlier and the final
shard waits 3m45s rather than 9m07s; its execution is 8m15s rather than 8m24s.
Retain the queue variation, twelve retries and unchanged occupancy alongside
the favorable elapsed result. All 28 normal jobs actually use four processes;
their cgroup lifetime peaks are 7.80-8.89 GiB, with zero swap or memory events.
The complete trio consumes 557.83 runner-minutes. Fifteen-minute acceptance
remains zero of three, and the PR remains open.

### Sixteenth complete hosted result

Head `dc4d1c4f7f4` passes all three complete workflows: backend 38652558394
in 1,201 seconds (209.18 runner-minutes), frontend 38652558464 in 853 seconds
(24.98 runner-minutes), and E2E 38652558454 in 1,549 seconds (295.30
runner-minutes). The green complete set takes 25m49s and 529.47 runner-minutes.
It remains above fifteen minutes; accepted complete sets are still 0/3.

All 3,957 hosted native test IDs match discovery, paired with their project
and owning root spec even when a helper declares the test. The only delta
from run 15 is exactly nine retired search IDs and three combined keepers.
There are 3,914 first passes, three retry-only passes, forty existing skips
and no final failures. Both repaired runtime cases and all six panel-search
cases pass on their first attempt. Stable uncertainty/Stop owners remain.
The normalized title keys are not a substitute for native identity: reporter
file-title prefixes vary, and declaration locations can name helper files.

E2E build is ready at 23:18:52 UTC, 7m10s after workflow creation, including
2m26s before change detection starts and a further delay before Build. The
final-finishing normal shard 16 waits 9m07s, runs 8m24s, and reporting adds
1m08s. Its actual test step takes 442 seconds. Run 14's corresponding final
job took 538 seconds versus 504 here, but build readiness was 223 seconds
earlier; the green full workflow is therefore slower. These are observed
scheduling/provisioning intervals, not proof of the account quota cause.

A current-head helper snapshot reports 89 passed, zero failed/pending checks,
no unresolved/hidden review threads, no actionable issue comments or evidence
errors, and no exact-head review bodies. The native monitor reaches terminal
exit zero after 1,906 seconds, with 89 passed and nine skipped checks, no
failures/pending checks and complete evidence. Candidate 17's local resource
control passes; its four-process preflight is guard-rejected before any tests
start. The next hosted four-process request is explicitly an experiment, with
full native/resource evidence required before adoption. This completed green
measurement remains retained.

### Fifteenth complete hosted result

Head `480259fac59` completes backend 38648580365 successfully in 837 seconds
(219.87 runner-minutes) and frontend 38648580366 successfully in 761 seconds
(26.93 runner-minutes). E2E 38648580304 fails in 1,575 seconds (26m15s;
299.80 runner-minutes). The complete failed set uses 546.60 runner-minutes and
does not count toward acceptance. All 3,963 native identities match run 14:
3,916 first passes, six retry-only passes, one final failure and forty existing
skips. The single failure is the runtime-replacement stale transient Stop
expectation described and reproduced in Task 15. Both failed status contexts
come from that shard and its aggregate required gate, not two test failures.

Build finishes 3m46s after E2E creation. Final-finishing shard 11 then waits
12m33s after dependencies, runs 8m51s, and reporting adds 1m05s. Scheduling/
provisioning cause is not inferred from elapsed delay. The execution critical
path is 848 seconds, but actual complete elapsed is 1,575 seconds. Accepted
complete sets remain 0/3. The native clipboard batch passes its original four
cases on their first attempt; large desktop and phone bodies are 38.861 and
27.357 seconds. These successful case results do not erase the full failure.

The native monitor reaches terminal with complete evidence: 87 passed checks,
nine skipped, two failed and zero pending. The next candidate consolidates
nine repeated search identities into three keepers and repairs the two runtime
assertions against the already established silent-restoration contract. Its
expected catalog is exactly 3,957, rather than unexplained test loss. Retain
this failed measurement and verify selection, required checks, reviews and
full hosted timing again on the new head.

### Fourteenth complete hosted result

Head `2c886763966` passes all three complete workflows: backend 38644787855
in 776 seconds (220.45 runner-minutes), frontend 38644787749 in 874 seconds
(29.90 runner-minutes), and E2E 38644787877 in 1,396 seconds (292.22
runner-minutes). The green full set takes 23m16s and 542.57 runner-minutes.
All 3,963 E2E identity counters match the thirteenth set: 3,920 first-pass,
three retry-only, forty existing skips and no final failures. Every phone
status-bar case passes on its first attempt, including the strengthened
canonical-only Proceed scenario. The repaired readiness test also passes.
One hosted pass is not a guarantee that every future timing or flake is fixed.

The E2E execution critical path is 835 seconds (13m55s), while full elapsed
includes much longer dependency-ready delays. Build finishes at 22:05:53 UTC,
3m27s after workflow creation. The final-finishing normal shard 4 starts at
22:15:31, after 9m38s waiting, then runs for 8m58s. Final reporting and workflow
completion add 1m13s. These scheduling/provisioning delays are retained in the
result; the account quota cause is not established. The faster local fixture
results do not establish full-CI acceptance. Accepted complete sets remain 0/3.

The native clipboard/causal-release batch in Task 15 is verified separately
and is not included in this hosted measurement. It retains every original
assertion and all 3,963 identities. Six desktop and two phone case repetitions
pass with retries disabled, along with full web typecheck, affected lint/format
and documentation checks. Commit only the reviewed test/support/documentation
scope, preserve this completed hosted result, and measure one next full set.
Keep current-head checks, review classification and mergeability external until
the new head's complete workflows are terminal.

### Recurring phone Proceed failure

Both complete E2E runs 38634314141 and 38638947962 spend two 90-second
attempts waiting for the phone Proceed button, then pass on the third attempt.
Do not remove or shorten its geometry/action assertions. Its production
`useNextWorkflowStep` selector reads only board memberships, while the task
route and detail already prefer `taskOverview.byId`. A selected task can remain
owned by its detail without remaining in a displayed board's membership.
The existing shared-task design makes that overview canonical; phone action
criterion AC-UI-MOBILE-TASK-ACTION-BAR-001.2 requires its workflow action.

Within this work order's existing-contract remediation scope, reproduce
canonical-only placement and canonical-final-step precedence in the focused
hook tests, then apply the same canonical-first lookup already used by
`useTaskRouteProjection`. Retain board/snapshot fallback and all progression,
visibility, ownership and move guards. No new store, endpoint or product
contract is introduced. Verify the existing full phone status-bar file with no
retries after a fresh web build, and retain repeated hosted evidence before
attributing either timeout to a delivered improvement. Do not claim the
selector explains every shared-runner delay.

The two new hook regressions fail before the selector correction and all 69
focused hook/action tests pass afterward. The existing browser case is
strengthened by removing board membership through the existing E2E store bridge
while asserting the active detail retains its canonical task. It first proves
Proceed is visible, then fails after membership removal against the old bundle.
After a fresh bundle with the correction, the full four-case phone status-bar
file passes twice: eight passes in 1.1 minutes with retries disabled. Every
original geometry/action assertion remains. Full web typecheck and strict
changed-file lint/format checks pass. Hosted timeout improvement is pending.

Thirteenth head `fa9a4135170` changes only the two delivery records. Backend
38638948095 fails at 747 seconds (214.53 runner-minutes); frontend 38638947951
passes at 638 seconds (25.55 runner-minutes); E2E 38638947962 passes at 1,268
seconds (314.83 runner-minutes). The complete set is 21m08s and failed, with
554.92 runner-minutes and zero accepted complete sets. E2E retains all 3,963
identities: 3,916 first-pass, seven retry-only and forty pre-existing skips.
The warm host build completes in 134 seconds, with 24 seconds backend/web
compilation versus 195 seconds cold. The final-finishing normal shard waits
346 seconds after build, then runs for 701 seconds; final reporting adds about
61 seconds. Maximum normal dependency-ready delay is 443 seconds. These delays
include scheduling and provisioning; the account quota cause is not established.
The difference from the preceding 24m37s E2E run is not controlled attribution.

Backend shard 2 fails `TestRuntimeReplacementKeepsBackendBoot` in 0.20 seconds:
the child endpoint file exists but is empty. The helper currently accepts any
successful read and exits its poll before checking for an empty payload. A
regular-file creation can precede its write; the unchanged three-second bound
has not expired in this failure. Reproduce the empty publication state using
the existing filesystem boundary, then make the test helper wait for a
nonempty publication. Keep the real child process, PID, boot ID, epoch, health,
ownership and recovery assertions; do not add retries or extend timeouts.
No application recovery behavior changes in this remediation. The real-file
publication regression fails against the old helper and passes after the guard;
the complete agentctl client package passes race tests in 6.505 seconds and
changed-file Go lint reports zero issues. All original recovery assertions and
the three-second publication deadline remain.

Twelfth head `4dd5ae57f32` succeeds in all three complete workflows on hosted
merge `74b4edcdc694`: backend 38634314044 takes 1,317 seconds and 229.55
runner-minutes; frontend 38634314162 takes 795 seconds and 26.8 runner-minutes;
E2E 38634314141 takes 1,477 seconds and 304.5167 runner-minutes. The complete
set is 24m37s and 560.8667 runner-minutes. It fails timing acceptance; zero of
the three required successful sub-fifteen-minute samples are established.
All 3,963 E2E identities remain: 3,915 first passes, eight retry-only passes,
40 existing skips and zero final failures. Every required check passes and
the exact-head snapshot has no unresolved review thread. Walkthrough generation
also succeeds on this head; the prior provider error is not a current failure.

The new host build's first cache population takes 294 seconds including job
cleanup. Native backend/web compilation takes 195 seconds; logs prove host
pnpm and Go cache publication. This cold run does not establish a net build
improvement over the earlier warm container build. E2E's final-finishing shard
25 starts 399 seconds after build completion and runs for 687 seconds. Another
normal shard starts 482 seconds after build. Those are dependency-ready delays,
not test execution; concurrent workflows from other PRs are observed, but the
API does not prove the exact capacity or provisioning cause. Reporting after
the final shard adds 65 seconds through workflow completion.

The user prioritizes full E2E and accepts jobs already near seventeen minutes;
do not continue shaving those backend jobs. Keep total work and shared capacity
visible rather than seeking a quiet-run best by increasing fanout. The next
measurement changes delivery records only: application, test, workflow and
runner selections stay byte-for-byte unchanged. Run one complete set at a time
on this same PR cache scope, and retain all waiting and warm-up costs.

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


### 2026-10-28 Windows persistence compilation allowance

PR #4534 head `60f315d5370857ff89824b1ff950bb3a326d9571`, backend run
`40675821815`, job `122779656390` fails at the ten-minute compilation-step
limit. Both module and compiled-package caches miss. Five of the six package
binaries finish; the task/service compiler receives cancellation at the step
limit. No compiler diagnostic or selected test failure precedes cancellation.
The previous successful head compiled the same package selection in 6m53s;
all seven sibling Windows suites pass at this head.

Increase only the persistence compilation allowance from ten to twenty
minutes. Retain the zero-match selector, exact package union, race/fts5 flags,
parallelism, memory limit, test deadlines, required backend gate and overall
ninety-minute job limit. Settings compilation retains its existing allowance.
The existing workflow contract first fails against the old allowance and then
passes with the correction. Actual Windows completion requires fresh hosted
CI; Linux contract validation does not prove Windows compilation timing.
