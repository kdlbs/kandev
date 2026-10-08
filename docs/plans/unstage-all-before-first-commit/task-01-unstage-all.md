---
id: "01-unstage-all"
title: "Restore first-commit Unstage all and settle delivery dependencies"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
  - REQ-PLATFORM-CI-PERFORMANCE-003
acceptance_criteria:
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.38
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.42
  - AC-PLATFORM-CI-PERFORMANCE-003.4
  - AC-PLATFORM-CI-PERFORMANCE-003.5
system_design:
  - ../../specs/platform/system-design/workspace-git-path-details.md
  - ../../specs/platform/system-design/workspace-git-status.md
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 01: Restore first-commit Unstage all and settle delivery dependencies

## Summary

Make empty-list Unstage work before the first commit using the smallest unconditional
Git argv correction. Independently authored real operator and registered HTTP regressions
must establish causal RED, then GREEN with selected-file and committed compatibility.

The initial implementation, process-cohort amendment and timestamp fixture are
published at8403. ROOT later reviewed/released the exact mode-transition fixture
correction in this SAME order; affected causal RED/GREEN, three controls and full
CHANGED lint passed. Normal documentation/hooks/publication and qualified heavy
return follow; new-head hosted gates remain pending. Historical passing controls
must not be replayed; no hosted rerun or merge authority follows.

## In scope

- Add bounded initial/all and committed/all operator/HTTP regressions from the plan matrix.
- Change only empty-list `GitOperator.Unstage` argv from `reset HEAD` to `reset --`.
- Correct the process/runtime command comments and the public reference Unstage row.
- Update this order, manifest and version-safe task plan with actual command outcomes.

## Out of scope

HEAD detection, history/tracker, Discard/Revert, literal-path helper/admission, environment,
command/resource policy, transport/schema, UI/copy/layout/browser/build/E2E, PostgreSQL,
broad local suites, native delegates, recursive tasks, extra sessions or model changes.
Do not access/replay/copy/alter the protected ROOT proof.

## Acceptance

1. Production Stage all followed by Unstage all in an actually unborn private repository
   clears every index entry and preserves exact working bytes/permissions, symbolic HEAD,
   refs and config; permanent RED proves the causal failure before the correction.
2. Committed all-files, explicit literal selection/sibling preservation, staged
   additions/mixed edits/deletion and invalid empty-entry behavior remain correct through
   production operator and actual registered HTTP requests. Selected independent
   multi-repository routing preserves every other repository's index and bytes.
3. Task-defined GREEN, scoped lint and documentation gates pass with every original
   process handle actually joined; update only causal comments/public reference and
   record evidence. Return the local-heavy lease explicitly before hosted collection.

## Initial implementation inputs and sequence (completed)

The manifest records the original owning AC.37/.38, real operator/registered HTTP
boundary and nine owned paths. ROOT's first reviewed release granted the original
lease. Independent initial/committed tests were authored, causal RED joined, only
empty-list argv corrected, anchored GREEN joined, and grounded API QF1003 corrected
with affected checks. Documentation gates and one conditional install passed.
Normal active-hook commit/push/READY publication completed and the lease was returned
before hosted collection. Exact original commands and receipts follow; this is
history, not authorization to repeat them. The amended release barrier is below.

## Verification

Run from the repository root under bash with login=false. Commands below are executable
bodies; launch each heavy body through a private owned receipt wrapper that records exact
PID/process-group, UTC start/cutoff, GNU timeout and actual exit. Retain every returned
session handle and poll that SAME handle to completion. One heavy process at a time.
No repeated passing test run and no duplicate launch to infer an earlier process outcome.

Use explicit installed tools:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
```

Permanent RED, after writing tests and before production change (expected exit 1):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 11m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestGitOperatorUnstageBeforeFirstCommit|TestGitOperatorUnstageAllCommitted|TestHandleGitUnstageBeforeFirstCommit|TestHandleGitUnstageAllCommitted)$' ./internal/agentctl/server/process ./internal/agentctl/server/api)
```

GREEN with the focused existing controls, after correction (expected exit 0):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 11m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestGitOperatorUnstageBeforeFirstCommit|TestGitOperatorUnstageAllCommitted|TestHandleGitUnstageBeforeFirstCommit|TestHandleGitUnstageAllCommitted|TestGitOperatorLiteralSelections|TestGitOperatorLiteralPathspecEnvironment|TestGitOperatorLiteralCaseSelection|TestHandleGitLiteralSelections|TestHandleGitStageAndUnstage)$' ./internal/agentctl/server/process ./internal/agentctl/server/api)
```

Scoped initial lint, serial, exact starting base:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run --concurrency=2 --allow-serial-runners --timeout=5m --new-from-rev=202d48bceb50ff839e1834d928d1677337fda672 ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/agentctl)
```

Original documentation/format gates, retained as historical commands. No command
in this section is authorized during the amended design-only turn:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

Coverage preflight: invoke the repository `.github/scripts/pr-docs.cjs`
`validateCoverage({changedFiles, fileContents})` export with this changed work order,
manifest, owning requirement and design, plus prospective owned production/test/doc
paths. Require `ok=true`, `status=covered`, `errors=[]`; use actual file contents,
not fabricated reference documents. This is a documentation gate, not a product run.

No package installation is needed for these Node/Python gates. After release only,
if apps/node_modules is absent, perform at most one pinned pnpm 9.15.9
`pnpm install --frozen-lockfile` from apps before active commit hooks.
No hook bypass or amend. No optional browser/build/E2E/PG or broad local verification.

A backend review fixup requires ONE full CHANGED lint from apps/backend against the
then exact PR base: `GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./... --new-from-rev="<exact-pr-base-sha>" --concurrency=2 --allow-serial-runners --timeout=5m`.
No backend replay for a docs-only fixup and no retry after a resource failure.

## Risks

- Fixture helpers that commit during setup can conceal the defect.
- Whole reset's existing ORIG_HEAD/reflog bookkeeping is a compatibility behavior;
  this local correction must not become a history-policy redesign.
- Assert Git index content, not byte-identical stat bookkeeping. Preserve all
  working bytes, including later unstaged edits to additions and existing files.
- Real HTTP manager fixtures own background work and must drain even on test failure.
- Unexpected Git/platform/resource behavior requires ROOT checkpoint before expansion.

## Parallelism

`sequential`. Same primary session owns all phases; no delegates.

## Results

ROOT released this reviewed package in the same primary after design END 02:15:53 /
WAITING 02:16:04 and granted the one local-heavy lease. Qualification is in
`/tmp/kandev-root-child77-reviewed-design-20261008.json`. The independently authored
permanent tests establish the supplied cause without accessing the protected proof.

The RED/GREEN and initial scoped-lint commands above ran exactly as documented.
Only initial all-files operator/HTTP subtests failed in RED; explicit, invalid and
committed controls and preservation assertions passed. GREEN passed all nine named
functions. Initial lint found one QF1003 in the new API test; changing its scenario
branch to a tagged switch required only these affected checks:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 11m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^TestHandleGitUnstageBeforeFirstCommit$' ./internal/agentctl/server/api)
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run --concurrency=2 --allow-serial-runners --timeout=5m --new-from-rev=202d48bceb50ff839e1834d928d1677337fda672 ./internal/agentctl/server/api)
```

All original native handles were actually joined, not replaced or inferred. Detailed
argv, environment, UTC cutoffs and fresh empty-group observations are in the owned
`/tmp/kandev-child77-unstage-20261008/<job>.receipt.json` files; native join receipts
are sibling `<job>.native.json` files and complete command streams are `<job>.log`.

| Job | Original native / actual join | PID/group | UTC start / cutoff | True exit / result |
| --- | --- | --- | --- | --- |
| red | 48487 / f1b3c5 | 1454939 | 02:21:15.145766 / 02:32:15.145766 | 1 expected; process 0.200s / API 0.522s; wall 47.301s |
| green | 18322 / 3b2a5c | 1459788 | 02:22:27.963898 / 02:33:27.963898 | 0; process 3.823s / API 2.412s; wall 29.081s |
| lint | 48848 / a0f838 | 1465065 | 02:23:07.295886 / 02:29:07.295886 | 1; one API QF1003; wall 67.369s |
| http-after-lint | 3402 / ba0c2e | 1470563 | 02:24:31.775496 / 02:35:31.775496 | 0; API 1.309s; wall 9.109s |
| api-lint | 59648 / d14501 | 1472818 | 02:24:56.769542 / 02:30:56.769542 | 0; 0 issues; wall 5.809s |
| docs | 42094 / aa0f99 | 1475147 | 02:25:47.278781 / 02:30:47.278781 | 0; wall 2.025s |
| install | 37590 / 03f521 | 1476963 | 02:26:08.222443 / 02:36:08.222443 | 0; pnpm 9.15.9; wall 1.997s |

All dates are 2026-10-08; all command groups were empty after joining. Documentation
commands passed: catalog 363 decisions/1437 specs; spec tests 36; full spec lint;
public-doc tests 62; public pages 47; actual-path coverage covered/errors empty;
whitespace check. The install was conditional on missing apps/node_modules and used
`corepack pnpm@9.15.9 install --frozen-lockfile` from apps, once, without a lockfile edit.
Production changed only empty-list argv and matching comments; public reference changed
only the Unstage row. No frontend/browser/E2E/PG or broad product checks were run.

Initial Unstage implementation and normal publication are done, with lease returned
before hosted collection. PR4313 is OPEN/BLOCKED at the frozen published SHA. The
amended dependency below is pending; no implementation or merge-ready claim applies.

## Amended dependency: reviewed implementation

Design ended 04:05:42. ROOT later reviewed and released this amendment in the SAME
primary, with ONE GLOBAL LOCAL-HEAVY lease; qualification is
`/tmp/kandev-root-child77-reviewed-amendment-20261008.json`. Preserve both failed
attempt logs, exhausted Backend Tests/Backend(windows,process) retry budget and
all joined originals. The published SHA exception permits only this reviewed
dependency correction. No install, third retry or merge authority.

### Owned files and acceptance

- `.github/workflows/backend-tests.yml`: process member only; retain job90m,
  two independent process/native matrix members, native steps and required gate.
- `.github/scripts/backend-tests-workflow-contract_test.py`: extend its existing
  registered contract; no new workflow/test registration service.
- `apps/backend/cmd/windows-process-tests/main.go`, `runner.go`, `runner_test.go`:
  fixed native enumeration, two selectors, original command starts/joins,
  separate diagnostics and fail-closed exact terminal coverage. Standard library
  only; this CI command is invoked from apps/backend in the Windows process member.
- `apps/backend/internal/agentctl/server/process/workspace_monitor_dirty_paths_test.go`:
  ONLY `requireMonitorRefresh` context/comment; retain every assertion/caller/case.
- The manifest, this SAME order and existing CI-performance requirement/design pair.
  Existing workspace requirement/design, production Unstage and new regressions stay intact.

Acceptance: native package/name inventory and two selectors form an exact disjoint
union, including process/probe, examples, fuzz seeds and every inherited subtest;
each command retains race/verbose/JSON/25m and every started original is joined.
Any command/selection/completion failure blocks the backend aggregate. The monitor
fixture joins the accepted bounded worker through the cached API, retaining exact
status/diff/repository/refresh/cache/no-op assertions plus cancellation/deadlock
controls. The current native suite, job90m and production60s contracts are unchanged.

### One sequential amended implementation sequence

1. Read current version, later reviewed ROOT release and owned diff; mark this SAME
   order in_progress, record lease and ensure every original observer/read is joined.
2. Write the workflow contract for the runner placement and unchanged native/gate
   boundaries. Run its RED against the existing single process command: expected
   assertion failure only for missing cohort runner/runner-test wiring. Author the
   focused helper tests and compiling minimal stubs; helper RED must be behavioral
   enumeration/partition/join/coverage failures, never a compiler failure.
3. Implement the fixed runner described by the CI design. Native `go test -list`
   JSON is the inventory authority; require success and package terminal records.
   Sort unique names, alternate, anchor/escape selectors, verify each package/name
   matches exactly once. Start both exact native commands before any Wait; a second
   Start failure still joins the first. Wait failures still join the sibling.
   Retain and publish both full logs, inventory/selectors, actual PIDs/times/exits;
   reject missing/duplicate/unselected top-level terminal records. No user-selectable
   cohort count, source-regex selection, weight maps or automatic retries.
4. Process-only workflow steps run helper tests (`go test -race -timeout 25m
   ./cmd/windows-process-tests`), then `go run ./cmd/windows-process-tests` in the
   existing Test Windows process package step. Keep native member and permissions,
   action pins, triggers, change detection and required gate unchanged. Existing
   apps/backend change detection and Linux package selection cover the new command.
5. In ONLY requireMonitorRefresh, remove the independent 10s context/cancel and
   pass `tracker.cancelCtxOrBackground()` to GetGitStatusWithDetails(..., false).
   Its existing job.done/error path and production60s bound justify settlement;
   no new observation or bypass of unavailable/superseded errors. Retain all cases
   and assertions. Hosted attempt2 is the actual causal RED for this fixture;
   do not create a slow ten-second replay just to reproduce it locally.
6. Run helper GREEN, workflow GREEN and the anchored monitor/lifecycle controls
   below once, sequentially. Then one full CHANGED lint against exact PR base and
   existing doc/coverage gates. Grounded in-scope fixture/compiler/lint fixes may
   run only affected checks. Resource/timeout/transport/unknown/scope failure
   checkpoints ROOT; no automatic retry or broad local suite.
7. Record actual outcomes, normal active-hook commit and authorized fixup push;
   preserve author body/managed regions, canonical association and five false flags.
   Join all original local handles, fresh-check owned groups and EXPLICITLY RETURN
   lease before ONE newly authorized collector. Its hosted current-head native list,
   complete cohort results, six required contexts, three parent workflows and full
   semantic review are delivery gates. Old FULL9 review proves only the old head;
   accept sufficient new automatic review, request once only for a real gap.
   No second hosted retry or merge without a separate specific ROOT grant.

### Exact released checks

Use the existing owned receipt wrapper, bash login=false and explicit Go1.26.0 /
Node24.21.0 PATH. Retain and ACTUALLY JOIN every original handle with PID/group,
UTC cutoffs and real exits. At most one heavy command at a time; no install is
needed because existing apps/node_modules and caches are preserved.

Workflow contract RED then GREEN (same anchored contract class, changes justify GREEN):

```bash
timeout --signal=TERM --kill-after=10s 60s python3 .github/scripts/backend-tests-workflow-contract_test.py BackendTestsWorkflowContractTest
```

Helper behavioral RED then GREEN (six planned top-level tests):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 11m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestCohortNativeListParsing|TestCohortPartition|TestCohortSelectors|TestCohortJoinAllStartedCommands|TestCohortCoverage|TestCohortRunnerCommand)$' ./cmd/windows-process-tests)
```

Parser cases: package-qualified Test/Fuzz/Example identities, no-test package,
duplicate within-package, unknown package, malformed stream, failed enumeration
and empty inventory. Selection: shuffled inventory deterministic, cross-package
same name, two nonempty cohorts, full union, no prefix match and complete nested
selection. Start/Wait contracts: both starts precede waits; second start failure,
first wait failure and both failures still join every successful start exactly once.
Coverage: pass/skip are terminal, nested records retain their parent, and missing,
duplicate, unselected or failed records/package/diagnostic reads never certify success.
These are CI-helper contract tests, not a substitute for actual hosted Git tests.

Changed monitor fixture and five focused deadline/cancellation/queue/Stop controls:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 11m go test -trimpath -tags fts5 -race -p=1 -count=1 -timeout=4m -run '^(TestMonitorTickDirtyExactPaths|TestWorkspaceTrackerEnrichmentValidationUsesBoundedBackgroundAdmission|TestWorkspaceTrackerStopWaitsForCorrectionObservation|TestWorkspaceTrackerQueuesSameFingerprintCorrectionBeforeFailedAttemptSettles|TestWorkspaceTrackerDetailsWaitCanCancelWithoutCancelingEnrichment|TestWorkspaceTrackerDetailsWaitRejectsSupersededSnapshot)$' ./internal/agentctl/server/process)
```

Full CHANGED lint ONCE, exact PR-base read must still match the recorded base:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./... --new-from-rev=202d48bceb50ff839e1834d928d1677337fda672 --concurrency=2 --allow-serial-runners --timeout=5m)
```

After release, use the existing catalog/spec/doc coverage and diff gates above with
ALL actual owned paths and amended documents. No public row changed, so no replay
of public-doc tests. No new action/security boundary, so no unrelated workflow
security audit. Native Windows listing/cohorts run only in authorized current-head
CI, not a Linux passing replay, synthetic merged test, browser/build/E2E or PG run.

### Amendment design result

Source/lifecycle and both exact failure profiles inspected; see manifest for
715/717 completed top-level identities and partial cohort timing distribution.
At design END 04:05:42 all prior collectors/reads were joined; the four documents
were unstaged/uncommitted and amendment checks had not run. ROOT later reviewed
that exact package and released implementation. Consult current task-plan version,
release and retained original handles after crash; results follow.


### Released local results / resource checkpoint

ROOT released after DESIGN END 04:05:42. Workflow behavioral RED failed only the
two absent runner wiring assertions; its other eleven contracts passed. Compiling
helper RED failed expected inventory, partition, selector, join, coverage and argv
behavior. GREEN passed all six helper functions, including actual native selector
subtests and native Start/Wait failure ownership; final affected helper run passed
2.037s. Workflow GREEN passed all thirteen contracts. The changed monitor fixture
and five bounded/cancellation/queue/Stop controls passed with race, 6.707s. Original
Unstage/API controls were not replayed. Production Unstage/new regressions unchanged.

The ONE full CHANGED lint command ran exactly against base202d48 with GOMAX2,
GOMEM1GiB, concurrency2, serial runners, CLI5m/GNU6m/kill10. Original48453
ACTUALLY JOINED a319cb, true exit124, group2232641 gone; start04:17:20.607279Z,
cutoff04:23:20.607279Z, end04:23:20.708161Z, wall360.101s. It emitted zero bytes
and NO lint verdict. Do not report zero issues or treat this as an assertion failure.
Documentation gates, commit/push and new hosted collector have NOT RUN.

All original local handles are joined and a fresh audit found every owned group
absent. Child77 EXPLICITLY RETURNS the global local-heavy lease before checkpoint.
Evidence: `/tmp/kandev-child77-unstage-20261008/amend-resource-checkpoint.json`,
`amend-*.{receipt,native}.json`, complete `amend-*.log` streams. HEAD remains frozen
a5ebc322..., changes uncommitted. No retry/fallback, observer or merge authority.
NEXT ROOT resource decision; a replacement requires a specific later grant.

ROOT subsequently granted exactly ONE identical warm-cache lint recovery; the original
124 remains failed/no verdict. Recovery allowance exhausted on this dispatch; retain
and join its original handle. No further automatic retry or broader validation.


### Authorized recovery and owned lint correction

ROOT explicitly granted one IDENTICAL full CHANGED warm-cache recovery after the
original timeout was joined. Original7359 joined325135 exit1, group2262278 gone,
04:25:35.178967Z to04:29:51.035982Z (255.857s), cutoff04:31:35.178967Z. It
returned six concrete findings, all in the new runner: readInventory cyclop18,
three unchecked file closes, one unchecked fixture write and a repeated action string.
This is a real failed lint verdict; the original124 remains failed/cause unproved.

Under standing owned-diagnostic authority, extracted listing event acceptance,
propagated close/write errors and named repeated event constants. Assertions and
policy unchanged. Only affected helper checks followed: original42054 joined50209d
exit0/group2278483 gone, six functions/race2.037s; helper-only lint87fca2 completed
original invocation exit0/group2280002 gone, zero issues/.771s. No third full lint,
monitor/Unstage/API passing replay, cache deletion or bound change. Exact commands
are in amend-helper-after-lint.job.json and amend-helper-lint.job.json; all raw
streams and native/UTC/PID receipts remain in the same owned evidence directory.
Local code checks now qualify; documentation/coverage gates and normal publication
remain pending. Global lease remains child77 under ROOT recovery direction.


### Local implementation qualified; hosted delivery pending

Catalog validation363/1437, specification-linter tests36 and all-spec lint passed.
Actual17-path coverage first found the manifest missing its already-owned main
workspace-status design. Added only that frontmatter reference; only affected
coverage/diff checks followed, b9dd20 originalcall/exit0/group2285516 gone,
covered/errors[]. No public-doc replay. Code implementation and scoped local
verification are complete. Normal hooks/fixup publication, new-head actual native
coverage/CI/review and separate merge grant remain delivery gates, not proved here.

### Additional native timestamp dependency: released validation

Normal active hooks, commit15166 joined234ce6/0, push83942 joined3a982f/0 and
publication91660 joinedb7c262/0 published the process amendment at `7bcbec5b...`.
Local/remote/upstream/PR heads matched, the worktree was clean, author body was
preserved and canonical association was complete/errors[] with all five flags false.
Fresh 39-receipt audit preceded the explicit local-heavy return. FULL ALL17 current
App347564 semantic review completed without actionable findings during CI.

At dependency release the SAME original collector57475 was live, group2301134, start04:38:09.542433Z,
inner90m cutoff06:08:09.542433Z, GNU91m cutoff06:09:09.542433Z/kill10. It has no
terminal verdict and must be actually joined before any later authorized replacement.
Windows process rerun budget remains exhausted; no native job rerun was granted.

Native Windows job113152567200/Backend Tests37728353083/attempt1 at frozen7bcb
failed the existing `TestRepositoryCheckoutDefaultsPresence` strict timestamp
assertion at `service_repository_checkout_controls_test.go:42`. Test bytes match
base202d48; the real update stamps `time.Now().UTC()`. Exact failed timestamps were
not logged, so clock-resolution/tie cause remains unproved. Original diagnostic25720
joined529265/0/group2363319 absent; complete raw log1023954bytes and metadata are
in `/tmp/kandev-child77-unstage-20261008/amend-native-failure-checkpoint.json`.

ROOT reviewed this concrete additional dependency and released AUTHORING ONLY.
Owned source is solely
`apps/backend/internal/task/service/service_repository_checkout_controls_test.go`,
inside `TestRepositoryCheckoutDefaultsPresence`. Before EACH payload's canonical
before-read, parameterized fixture DB `ExecContext` sets only `checkout-repo.updated_at`
to a deterministic historical UTC instant; require exactly one affected row and confirm
that baseline through the canonical repository read. Keep strict `UpdatedAt.After`,
all five payloads, omitted/null/explicit presence, returned field values, real mutation
and actual event assertions. All companion/control tests and production code remain
unchanged. No sleep, weakened comparison, test skip or clock seam.

ROOT subsequently released validation in this SAME primary after qualifying
child78's explicit heavy return; receipt is
`/tmp/kandev-root-child78-publication-return-20261008.json`. The SAME order is
in progress, with the sole global local-heavy lease granted to child77. The fixture
and documents are still unstaged/uncommitted; no result is claimed before execution.

1. Read current release/version and retain original collector57475; no duplicate.
2. Run only the changed test with the released count10/race command below and
   ONE full CHANGED lint at the exact unchanged PR base. Retain every original
   handle, PID/group, UTC cutoff and actual join/exit. No automatic resource retry.
3. Run the relevant normal documentation/diff/actual-path coverage gate for all
   actual changed files (18 after this fixture). No passing Unstage/API/helper/monitor
   or companion-test replay. Normal active hooks, no bypass/amend, then fixup publication.
4. Preserve author body/canonical association/five false flags; return any granted
   heavy lease only after all original local commands are joined/groups gone.
   Before corrected push, verify ownership, stop ONLY old observer57475 and actually
   join it, preserve its reports and prove its exact group gone; do not cancel hosted jobs.
   A new observer is authorized only after local publication/joins and lease return.
   The prior FULL17 review
   becomes historical; qualify new-head full actual-file review and natural hosted
   Windows coverage/required contexts/parents. No blind rerun or merge authority.

Exact released changed-test command, bounded to three minutes:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -count=10 -timeout=3m -run '^TestRepositoryCheckoutDefaultsPresence$' ./internal/task/service)
```

Exact released full CHANGED lint; original earlier124 and failed recovery
remain historical failures, not cleanliness evidence:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB timeout --signal=TERM --kill-after=10s 6m golangci-lint run ./... --new-from-rev=202d48bceb50ff839e1834d928d1677337fda672 --concurrency=2 --allow-serial-runners --timeout=5m)
```

Use existing Go1.26.0/Node24.21.0 PATH and bash login=false, no install or cache deletion.
The actual hosted failure is the RED evidence; later local count10 success alone
cannot certify native Windows correction. Resource/transport/timeout/unknown/scope
failures checkpoint ROOT without automatic retry. No production clock change,
passing test replay, hosted rerun or merge authority.

### Native timestamp local results; hosted delivery pending

The exact released count10 command passed, original75293 joinedaade8a/0,
group2382648 absent. Start05:00:37.288070Z/cutoff05:03:37.288070Z,
end05:02:07.723436Z: wall90.435s, package1.577s. ONE full CHANGED lint
then passed zero issues, original52635 joined1377ed/0/group2389256 absent,
start05:02:18.020205Z/cutoff05:08:18.020205Z/end05:03:57.151314Z,
wall99.131s. All existing strict/payload/field/event assertions are retained;
no production change or passing-control replay. Original earlier lint124 remains failed.

Before publication, ROOT-authorized ownership checks verified original57475's
exact argv, parent and private wrapper. TERM05:05:01.386936Z stopped only its
owned group. Original57475 actually joined26aa11; collector exit143/no verdict,
wrapper exit1 because its summary parser encountered the empty interrupted report
AFTER saving the truthful terminal receipt. Raw reports/progress are preserved;
fresh PID/group/wrapper check is empty. Last snapshot35PASS/1FAIL/25PENDING is
historical. Old process job113152567188 was still running when read; no complete
cohort report was available, and no hosted job was cancelled or awaited for this fixup.

Local implementation is done. Catalog363/1437, all-spec lint, actual18-path coverage
(covered/errors[]) and diff whitespace passed, original7210 joined415baa/0,
group2401993 absent, wall1.277s. Unchanged specification/public validator self-tests
and product controls were not replayed. Normal active-hook publication follows.
New-head native fixture RUNPASS,
complete process/probe inventory/two25m cohort joins/coverage, required six contexts,
three successful parents and substantive full actual-file review remain hosted gates.
The published-head review is historical after this fixup; no merge-ready claim yet.

### Immediate mode-transition scan dependency: authoring checkpoint

ROOT authorized DESIGN/AUTHORING ONLY for the same order after current-head
Windows process job113159794671/run37730879950/attempt1 failed the unchanged
`TestMonitorLoop_TransitionToFastTriggersImmediateScan` notification assertion.
Its notification wait is TWO seconds; 5.28 seconds is total case duration.
Base202d48/current8403 source bytes match. The raw stream shows no package timeout.
Inventory is process767 plus probe16 native top-level identities. Cohorts392/391
are disjoint and complete; their actual terminal identity coverage equals inventory.
Both started before either original Wait joined, exits0/1; reportComplete=false
truthfully reflects this sole assertion failure. Unstage regressions passed in both
cohorts; the corrected native timestamp fixture separately passed naturally on Windows.
These facts do not prove a transient failure or a successful full process suite.

Released ownership is only the body of this exact test in
`apps/backend/internal/agentctl/server/process/workspace_poll_mode_loop_test.go`,
plus this SAME order, manifest and owning CI design explanation. No production,
shared helper, workflow, Unstage regression or other fixture changes are authorized.
After paused initial scan, capture completed scan count and drain stale tickDone.
Within the original two-second bound after switching fast, observe monitorRunning
or a completed-count advance, including a tick shorter than the observation interval.
The two unchanged 30-second timers cannot establish this admission assertion.
Then join the actual admitted scan through tickDone/cancellation before retaining
the existing real subscription notification assertion and its two-second wait.

ROOT accepted the completion guard workspaceGitStatusObserveTimeout + 3*gitCommandTimeout,
currently90 seconds: status observation60 plus quick-state Git twice/file-list once10
each. This is an explicit fixture guard, NOT a normative whole-tick production bound;
queue admission and subprocess cleanup are additional. ROOT explicitly qualified
this owned TEST failure/cancellation guard in the later implementation release;
no latency, whole-tick deadline or failure-cause claim follows.
The notification remains a required real event; tick completion alone cannot pass.
Paused setup, file bytes, long fast intervals, subscription and tracker cleanup remain.

Sequential validation released by ROOT after the completed authoring turn:

1. ROOT reviewed the exact four-file diff at8403 and accepted the guard qualification.
   SAME order is in_progress under child77 sole global local-heavy release.
2. Negative control: create an owned disposable Go overlay of workspace_monitor.go
   that retains mode-change timer drain/fast check but returns false before its
   monitorRunning CAS/monitorTick call. Execute ONLY the changed test with that
   overlay and require the new admission assertion to fail at two seconds. Neither
   notification nor the regular30-second timer can turn it into success. Retain and
   join the original command, restore/remove only owned overlay artifacts. No permanent
   production mutation, fake monitor, protected-proof copy or passing-control replay.
3. Positive affected test count10 and focused unchanged PausedSuppressesNotifications,
   FastPolls and FastToPausedStopsPolling controls, exact commands below. Retain actual
   joins/PIDgroups/UTC bounds. No passing Unstage/API/helper/clock fixture replay.
4. ONE actual corrected-source full CHANGED lint at exactbase202d48, normal owning
   documentation/actual-path coverage and whitespace gates, active hooks/publication
   only after explicit release. Stop and checkpoint resource/unknown/out-of-scope
   failures; exhausted Windows hosted rerun budget and MERGE NONE remain intact.

Released affected positive command (explicit Go1.26.0/Node24.21.0 PATH, bash login=false):

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB timeout --signal=TERM --kill-after=10s 3m go test -trimpath -tags fts5 -race -p=1 -count=10 -timeout=3m -run '^TestMonitorLoop_TransitionToFastTriggersImmediateScan$' ./internal/agentctl/server/process)
```

Negative uses the same anchored test/count1 with `-overlay=<owned-absolute-overlay.json>`;
expected Go exit1 plus exact admission failure, not compiler/timeout/canned success.
Focused controls use the same flags/bounds/count1 and selector
`^TestMonitorLoop_(PausedSuppressesNotifications|FastPolls|FastToPausedStopsPolling)$`.
Full CHANGED lint uses the already recorded exactbase202d48/concurrency2/allowserial,
GOMAX2/GOMEM1GiB/CLI5m/GNU6m/kill10 command, no automatic retry.

Original observer64810/group2424195 remains intact with original inner cutoff06:39:57Z,
GNU06:40:57Z/kill06:41:07Z. All diagnostic reads actually joined; no new heavy handle.
Current8403/FULL18 review and natural clock-fixture PASS are retained current evidence;
they become historical only after an authorized correction is later published.
At AUTHORING_END the candidate was unstaged/uncommitted and no checks had run.
ROOT subsequently reviewed/released implementation; actual results follow.


### Mode-transition released local results

ROOT later reviewed the exact four-file candidate and accepted90 seconds solely as
an owned TEST failure/cancellation guard, not a production aggregate deadline or
queue/cleanup/failure-cause assurance. Its receipt is
`/tmp/kandev-root-child77-mode-fixture-review-20261008.json`; the later same-primary
release granted sole local-heavy implementation. No production/shared helper/workflow
or other fixture source changed.

ONE disposable Go-overlay negative/count1 failed the exact two-second admission
assertion: case2.08s/package2.126s, original72826 joined719e15/exit1/group2890115
absent, wall10.237s. It retained timer drain/fast mode check but returned before
CAS/tick; this is causal wake-property RED, not a compiler/cleanup/90s-timeout result.
Only owned overlay JSON/source were removed after join; production bytes preserved.
Affected positive/count10 passed race/package2.187s, original74101 joined610362/0,
group2892230 absent/wall9.806s. Exactly the three planned unchanged paused/fast
controls passed race/package2.499s, original86763 joined87e467/0/group2894939 absent,
wall4.157s. The single exact-base full CHANGED lint passed zero issues, original84190
joined044456/0/group2896471 absent/wall12.814s. No passing Unstage/API/helper/clock
replay or package/job timeout increase occurred. Natural new-head Windows evidence
is still required; local checks cannot prove the hosted failure's latency cause.

Documentation/actual19-path coverage, normal active hooks/publication and original
observer64810 join precede heavy return. Current8403/full18/native clock PASS and
failed process evidence become historical after correction publication. Required
contexts, three parents, full actual-file review and complete native cohort coverage
remain delivery gates; no hosted rerun or merge authority.


Documentation gates passed: catalog363/1437, all-spec lint, actual19-path delivery
coverage (covered/errors[]) and whitespace, original25938 joined6b4490/0,
group2901835 absent/wall1.401s. No unchanged validator/public/product test replay.
Public docs need no change for this fixture-only synchronization correction. Normal
active-hook publication follows; all hosted outcomes remain separately qualified.
