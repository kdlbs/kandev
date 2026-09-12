---
created: 2026-09-12
status: in_progress
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-001
  - REQ-PLATFORM-CI-PERFORMANCE-002
  - REQ-PLATFORM-CI-PERFORMANCE-003
  - REQ-PLATFORM-CI-PERFORMANCE-004
system_design:
  - ../../specs/platform/system-design/ci-performance.md
legacy_specs: []
---

# Implementation Plan: CI performance

## Overview

Bound Claude execution, repair frontend dependency caching, and reduce repeated frontend test setup.
Benchmark test partitions after the setup repair. Then produce focused E2E, Windows, and capacity follow-up evidence.
Execute the six work orders sequentially. Live runner activation remains an operator action outside this package.

## Inputs

- [Requirements](../../specs/platform/requirements/ci-performance.md), `REQ-PLATFORM-CI-PERFORMANCE-001` through `004`.
- [System design](../../specs/platform/system-design/ci-performance.md).
- [Existing external-runner package](../external-e2e-runner-capacity/plan.md): complete, reused without reopening its implementation.
- [Existing E2E efficiency package](../e2e-ci-efficiency/plan.md): rollout evidence remains open.
- [External-runner ADR](../../decisions/2026-09-06-opt-in-external-e2e-runners.md).

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
The `frontend` job keeps static checks and build. A new test matrix feeds the unchanged public `Frontend Tests Passed` gate.
Tasks 05 and 06 produce bounded reports and procedures. They do not invent unmeasured code changes.
See the system design for candidate-retention thresholds and failure behavior.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| 001.1–001.2 | `claude-code-review-workflow-contract_test.py`: budgets on three execution jobs, no success fallback, preserved trigger/permission blocks. |
| 002.1–002.2 | `frontend-tests-workflow-contract_test.py`: resolved store wiring and lockfile install; hosted cache miss/save/hit evidence remains pending. |
| 003.1 | `apps/web/scripts/vitest-project-selection.test.ts`: complete, disjoint file selection; unsharded versus merged two-shard identities. |
| 003.2 | `vitest-environment.test.tsx`, focused `lib/i18n` tests, worker-budget tests, production-mode focused run, typecheck, and full unit suite. |
| 003.3 | Frontend workflow contract cases for matrix placement, report publication, result handling, and deliberate skips. |
| 004.1–004.2 | [Curated report](evidence.md) with attempt-specific timestamps and local two-shard identity evidence; hosted comparable performance remains pending. |
| 004.3 | Existing runner planner and placement tests plus the runbook procedure with protected jobs, cost assumptions, and rollback. |

Proposed test paths are implementation deliverables. Existing paths were checked during planning.
No browser E2E is needed for CI-only behavior. Existing application E2E coverage remains unchanged.

## Work orders

- [x] [Task 01: Bound Claude execution](task-01-bound-claude-execution.md)
- [ ] [Task 02: Repair frontend dependency caching](task-02-repair-frontend-cache.md)
- [ ] [Task 03: Reduce frontend test setup](task-03-reduce-test-setup.md)
- [ ] [Task 04: Partition frontend verification](task-04-partition-frontend-verification.md)
- [x] [Task 05: Profile remaining CI costs](task-05-profile-remaining-costs.md)
- [x] [Task 06: Prepare the runner capacity pilot](task-06-prepare-capacity-pilot.md)

## Execution and evidence

All work orders start `pending`. Dependency order is 01 → 02 → 03 → 04 → 05 → 06.
Fresh worktrees need `(cd apps && pnpm install --frozen-lockfile)` before any pnpm command.
Use TDD for changed selection or workflow logic. Configuration tests must protect a meaningful failure boundary.

Remote benchmark evidence requires authorized workflow execution or naturally occurring runs of the changed workflow.
Local success alone cannot satisfy cache reuse or hosted performance criteria.
Record pending operational evidence explicitly. Do not mark its work order done or the package implemented until its required evidence exists.
A measured rejection can complete Task 04 through its documented no-sharding outcome.
Task 06 completes with a checked procedure; activation and pilot results remain outside its scope.

## Verification results

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
