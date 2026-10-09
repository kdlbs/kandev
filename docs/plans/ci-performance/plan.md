---
created: 2026-09-12
status: in_progress
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-001
  - REQ-PLATFORM-CI-PERFORMANCE-002
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
  - REQ-PLATFORM-CI-PERFORMANCE-006
system_design:
  - ../../specs/platform/system-design/ci-performance.md
legacy_specs: []
---

# Implementation Plan: CI performance

## Overview

Reduce CI runner-minutes and PR feedback time within approximately 60 shared concurrent jobs.
Continue the existing package instead of repeating its completed review-budget, setup-framework, and capacity-procedure work.
The October continuation reduces frontend setup, avoids independent Go-test-only E2E runs, and removes redundant scheduling stages.
Frontend sharding follows measurement of the cheaper setup. Additional E2E shards and paid capacity are not required.

The user authorized implementation after planning. The October changes passed
local validation; hosted performance acceptance remains open. Historical
September results remain separate from the current experiment.

## October delivery sequence

1. Task 07: add a read-only measurement helper and capture comparable baselines.
2. Task 03: expand reviewed frontend setup assignments, using the complete inventory and sample experiment.
3. Task 08: combine each workflow's bootstrap stages and combine E2E reporting with its required gate.
4. Task 09: exempt verified Go-test-only PRs from application E2E, preserving backend checks.
5. Task 04: compare one, two, and four frontend partitions; adopt the smallest passing candidate.
6. Task 10: measure remaining E2E fixture and container-planning costs, then define only justified follow-up work.

Task 02's cache save/miss evidence remains open but does not block setup work.
Tasks 01, 05, and 06 retain their completed historical status.
All work is sequential; this plan does not authorize subagents.

| Work | Intended benefit | Adoption evidence |
| --- | --- | --- |
| Frontend setup | Target 40% lower full-suite execution and 30% fewer frontend runner-minutes | One full-suite pair plus three stratified setup pairs; hosted full-suite samples remain pending |
| Go-test-only PR selection | Avoid an application E2E fan-out of roughly 550 runner-minutes for eligible revisions | Dependency audit, classification matrix, preserved required result |
| Bootstrap/report consolidation | Four fewer scheduled jobs per full workflow set; two fewer E2E queue boundaries | Workflow contracts, exact required names, failure-path evidence |
| Frontend partitioning | At least 30% shorter optimized execution path | At most 10% additional frontend runner-minutes; no loaded feedback regression |
| E2E follow-up | Identify measurable fixture/setup savings without removing lifecycle coverage | Attributed measurements and bounded follow-up proposal |

These are targets and acceptance gates, not claims of delivered hosted speedups.
Full runtime changes still run complete selected suites. Main and merge-group verification remain intact.
The local 32-file sample improved by 68.1%; it does not establish a whole-suite improvement.

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), `REQ-PLATFORM-CI-PERFORMANCE-001` through `006`.
- [System design](../../specs/platform/system-design/ci-performance.md).
- [Existing external-runner package](../external-e2e-runner-capacity/plan.md): complete, reused without reopening its implementation.
- [Existing E2E efficiency package](../e2e-ci-efficiency/plan.md): rollout evidence remains open.
- [External-runner ADR](../../decisions/2026-09-06-opt-in-external-e2e-runners.md).
- [Capacity-aware verification ADR](../../decisions/2026-10-08-capacity-aware-ci-verification.md).
- [October evidence](evidence-2026-10-08.md), [complete frontend inventory](frontend-audit-2026-10-08.csv), and [reproducible experiment](experiments/benchmark-setup.py).

## Investigation baseline

These observations are diagnostic samples from September 12, 2026. They are not reconstructed dashboard percentiles.

| Evidence | Observation | Implication |
| --- | --- | --- |
| [Claude run 34355720657](https://github.com/kdlbs/kandev/actions/runs/34355720657) | 7h33m04s; approval expired; zero jobs | An execution timeout does not solve approval waits. |
| [Claude run 34025129928](https://github.com/kdlbs/kandev/actions/runs/34025129928) | Review job queued 80.35 minutes and ran 6.35 minutes | Separate queue delay from review execution. |
| [Frontend job 103543614850](https://github.com/kdlbs/kandev/actions/runs/34689871445/job/103543614850) | Job 29.9 minutes; unit tests 25.5 minutes | Unit-test overhead is the primary measured frontend target. |
| Same frontend log | 1,986 files; 17,033 passed and 4 skipped tests | Preserve selection at the benchmark snapshot; current counts can grow. |
| Same frontend log | Cache directory missing; no cache saved | Resolve the real pnpm store path. |
| [Cargo run 34027695660](https://github.com/kdlbs/kandev/actions/runs/34027695660) | Queue 43m22s; execution about 3.2 minutes | Runner capacity dominates this sample. |
| [E2E run 34687600985](https://github.com/kdlbs/kandev/actions/runs/34687600985) | 41.4 minutes overall; browser jobs 15.6–20.4 minutes; queue up to 10.1 minutes | Profile fixtures and retries before further shard expansion. |
| [Backend run 34692849510](https://github.com/kdlbs/kandev/actions/runs/34692849510) | Windows job 21.7 minutes; sensitive-package step 13.2 minutes | Distinguish compile time from individual test time. |

Vitest reported 1,526.89 seconds wall time. Aggregate worker phases were setup 1,862.11s, import 1,252.60s, environment 706.35s, transform 585.30s, and tests 385.32s.
Those phases overlap across workers. Their sum is not elapsed time.
The actual cost of loading all locales remains a hypothesis until a controlled comparison isolates it.

## Scope

### In scope

- Thirty-minute execution budgets for both automatic review jobs and the interactive Claude job.
- Frontend pnpm store discovery, compatible keys, and cache save/restore proof.
- Explicit test environments and selective locale setup with unchanged test selection.
- Measured two/four-shard candidates and a stable required frontend gate.
- A focused E2E/Windows profile report and an operator-ready capacity pilot procedure.
- Completion of the frontend classification work across all 2,757 inventoried files, with unknown files retained until reviewed.
- Read-only CI cost measurement, four fewer scheduling jobs, and a narrow Go-test-only PR exemption.
- Measured E2E fixture and skip-context follow-up without speculative runtime changes.

### Out of scope

- Approval bypasses, automatic retries, new permissions, or provider model changes.
- Live repository variable changes, paid runner activation, workflow dispatch, or publication during planning.
- Release optimization, Cargo binary caching, unproven E2E cache repairs, and unidentified application fixes.
- Application UI changes, E2E worker-count changes, lower race coverage, or disabled test isolation.
- Editing the supplied dashboard without its source and metric definitions.

## Technical approach

Task 01 uses job-level timeout fields and existing Claude workflow contract tests.
Task 02 resolves the pnpm store in the actual frontend container.
Task 03 separates Node-compatible files from browser tests and isolates multilingual setup.
Task 04 uses native Vitest shards and the existing external-runner family schema.
The `frontend` job keeps static checks, the unsharded unit suite, and build until
the hosted adoption gate passes. A measured test matrix remains a candidate for
the unchanged public `Frontend Tests Passed` gate.
Tasks 05 and 06 produce bounded reports and procedures. They do not invent unmeasured code changes.
See the system design for candidate-retention thresholds and failure behavior.

### October implementation boundaries

Task 07 adds `.github/scripts/ci-efficiency.py` and fixture-based tests. It adds no always-on CI job.
Task 03 extends `REVIEWED_NODE_TEST_FILES` and `REVIEWED_BROWSER_TEST_FILES` in bounded, verified batches.
Unknown indirect dependencies keep the existing full-locale setup; all assertions and test isolation remain intact.
Task 08 moves allocation outputs into each `changes` job and makes `e2e-report` publish `E2E Tests Passed`.
Task 09 adds an explicit test-only reason after a valid, mode-aware diff. Unknown input selects full E2E.
Task 04 preserves all static checks and measures native Vitest partitions after setup optimization.
Task 10 profiles the existing reset, Git, browser, and container boundaries before proposing further changes.

The Windows process-cohort implementation and its first-commit Unstage companion stay unchanged.
The external-runner package remains implemented. Only its job inventory changes when Task 08 lands.
The E2E browser-cache package retains its separate three-main-run and forced-miss evidence requirements.
No public application behavior or rendered UI changes; this package needs no UI preview.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| 001.1–001.2 | `claude-code-review-workflow-contract_test.py`: budgets on three execution jobs, no success fallback, preserved trigger/permission blocks. |
| 002.1–002.2 | `frontend-tests-workflow-contract_test.py`: resolved store wiring and lockfile install; hosted cache miss/save/hit evidence remains pending. |
| 003.1 | `apps/web/scripts/vitest-project-selection.test.ts`: complete, disjoint file selection; unsharded versus merged two-shard identities. |
| 003.2 | `vitest-environment.test.tsx`, focused `lib/i18n` tests, worker-budget tests, production-mode focused run, typecheck, and full unit suite. |
| 003.3 | Frontend workflow contract cases for unsharded production-mode test placement, result handling, and deliberate skips. |
| 004.1–004.2 | [Curated report](evidence.md) with attempt-specific timestamps and local two-shard identity evidence; hosted comparable performance remains pending. |
| 004.3 | Existing runner planner and placement tests plus the runbook procedure with protected jobs, cost assumptions, and rollback. |
| 005.1 | `ci-efficiency_test.py`: overlap, timestamps, attempts, cancellation, incomplete evidence, and workflow identity fixtures. |
| 005.2 | Existing selection/worker/environment tests plus full-suite identity and outcome comparison after reviewed migrations. |
| 005.3 | One/two/four-partition hosted comparison, using the optimized baseline and the stricter 10% occupancy allowance. |
| 005.4 | Backend/frontend/E2E workflow contracts: merged bootstrap, complete joins, report failure, unexpected skips, and stable required names. |
| 005.5 | Event/change-class cost report; no added fleet or E2E workers in the candidate. |
| 006.1–006.4 | `ci-test-only-changes_test.py`: PR 4349, statuses/modes/renames, mixed changes, dependency exceptions, unknown inputs, and non-PR fallbacks. |

The new measurement, gate, and exemption tests are implemented. Existing paths were checked during planning.
No browser E2E is needed for CI-only behavior. Existing application E2E coverage remains unchanged.

## Work orders

- [x] [Task 01: Bound Claude execution](task-01-bound-claude-execution.md)
- [ ] [Task 02: Repair frontend dependency caching](task-02-repair-frontend-cache.md)
- [ ] [Task 03: Reduce frontend test setup](task-03-reduce-test-setup.md)
- [ ] [Task 04: Partition frontend verification](task-04-partition-frontend-verification.md)
- [x] [Task 05: Profile remaining CI costs](task-05-profile-remaining-costs.md)
- [x] [Task 06: Prepare the runner capacity pilot](task-06-prepare-capacity-pilot.md)
- [x] [Task 07: Measure execution and occupied capacity](task-07-measure-ci-efficiency.md)
- [x] [Task 08: Remove redundant scheduling stages](task-08-reduce-scheduling-stages.md)
- [x] [Task 09: Avoid application E2E for independent Go test changes](task-09-skip-go-test-only-e2e.md)
- [x] [Task 10: Attribute E2E fixture and planner costs](task-10-profile-e2e-fixture-costs.md)

## Execution and evidence

The October order above replaces the original six-task sequence. Existing completed results remain historical evidence.
Fresh worktrees need `(cd apps && pnpm install --frozen-lockfile)` before any pnpm command.
Use TDD for changed selection or workflow logic. Configuration tests must protect a meaningful failure boundary.

Remote benchmark evidence requires authorized workflow execution or naturally occurring runs of the changed workflow.
Local success alone cannot satisfy cache reuse or hosted performance criteria.
Record pending operational evidence explicitly. Do not mark its work order done or the package implemented until its required evidence exists.
A measured rejection can complete Task 04 through its documented no-sharding outcome.
Task 06 completes with a checked procedure; activation and pilot results remain outside its scope.

Collect three comparable baseline/candidate samples for setup and partition decisions, with all failed attempts recorded.
Use the same runner class, source snapshot, toolchain, selected identities, and declared cache state.
Serial local shards prove selection only. Record loaded queue/elapsed behavior from naturally occurring runs when available.
Use one bounded experimental rollout at a time; do not enqueue several duplicate full E2E baselines into the saturated fleet.
Performance rejection retains the prior safe configuration and records the measured reason.

## Verification results

October implementation validation completed locally on 2026-10-09:

- Frontend: 420 Node, 867 English-browser, and 1,470 full-locale files. New files
  retain full setup. No application test assertion or isolation policy changed.
- Full baseline and corrected candidate preserved all 2,757 files and 24,573
  test identities/outcomes (24,569 passed, four skipped). Report elapsed fell
  from 43m47s to 27m21s, 37.5% in one local pair. The 40% target was not met.
- Three stratified pairs preserved 96 files and 925 tests per run. Median
  command elapsed fell from 60.608s to 20.400s. This is sample evidence only.
- The first full candidate's DOMParser failure remains recorded. Its file was
  moved from Node to browser setup, preserving the original test assertions.
- CI measurement, conservative selection, and workflow/gate contracts passed
  106 tests. Actionlint, frontend guards, typecheck, ESLint, Prettier,
  architecture/spec/catalog checks, and documentation coverage passed.
- Zizmor has no new finding on changed workflows; one existing low-severity
  Windows CMD finding remains. Full-repository existing findings are separate.
- Four scheduling jobs were removed. Required check names match the active
  main ruleset, and missing planning output or report evidence blocks success.
- Hosted cache and performance acceptance remain open. One frontend job is
  retained; the two/four-shard experiment is deferred without claiming rejection.

See the [October evidence](evidence-2026-10-08.md) for report hashes, failed
attempts, complete inventory, E2E fixture attribution, and measurement limits.


October design validation completed on 2026-10-08:

- `python3 scripts/list-docs.py validate`: passed; 366 decisions and 1,475 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- All ten work orders passed requirement, acceptance-ID, design-ownership, plan-design, and dependency-path checks.
- The repository `pr-docs.cjs` validator accepted the actual changed paths as documentation-exempt; independent traceability checks covered the package links.
- Experiment script syntax and JSON checks passed; all 2,757 CSV paths exist and are unique.
- `git diff --check` passed; status inspection identified the complete unstaged package, including new work orders and evidence.

Local investigation experiment: six successful runs, each with 32 files and 236 tests; all identity/status multisets matched.
Median elapsed time was 27.004s baseline and 8.611s candidate on the local task VM.
No full-suite or hosted adoption result is claimed by this experiment.

Design validation completed on 2026-09-12:

- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Package link and acceptance-reference checks: passed.
- `git diff --check -- docs/specs docs/plans/ci-performance`: passed.
- `git status --short -- docs/specs/platform docs/plans/ci-performance`: all nine new package files and the platform index identified.

Production application behavior is unchanged. CI workflows, workflow contracts,
test configuration, and investigation-plan evidence changed as scoped.
Implementation checks pass locally. Hosted cache reuse and hosted performance
acceptance remain pending because no changed workflow was dispatched.

Implementation validation on 2026-09-12:

- Full Vitest candidate: 1,999 files, 17,180 passed tests, 4 pending, 0 failures.
- Production-mode focused Vitest: 12 files, 106 tests passed.
- Two local shards: 1,999 files and 17,136 assertion identities formed an exact disjoint union.
- Web typecheck, i18n check, ESLint, Prettier, and production build passed.
- Frontend, Claude, external-runner, runner-plan, and action-pinning contract tests passed.
- YAML parsing and `git diff --check` passed. `actionlint` is unavailable in this workspace.

## Risks

- Splitting setup can expose hidden DOM or locale dependencies. Default unknown files to existing setup until reviewed.
- Multiple Vitest projects can duplicate or omit files. Compare full identities and counts, not totals alone.
- More shards can increase cost and queue pressure. Retain only measured candidates within the design budget.
- General interactive Claude requests can exceed 30 minutes. Record timeout frequency before revising the budget.
- Hosted cache scope can prevent cross-branch hits. Prove reuse within compatible GitHub cache access rules.
- Existing companion plans retain their own statuses. This package does not claim their pending operational evidence.

## Documentation impact

Internal CI procedures change in `docs/ci-merge-queue.md` during implementation.
Public application behavior and `docs/public/**` remain unchanged.
