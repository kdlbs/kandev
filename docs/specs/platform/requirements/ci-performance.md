---
status: draft
system: platform
created: 2026-09-12
owners:
  - kandev
---

# CI performance requirements

## Overview

Maintainers need bounded review execution and shorter CI feedback without lost test coverage.
Platform owns these shared operational guarantees across repository workflows.
The CI automation system retains ownership of contributor authorization and approval rules.
The existing external-runner contract retains ownership of fleet selection and trust boundaries.

## Terminology

- **Execution:** The interval from a job start to its completion.
- **Queue delay:** The interval from job creation to job start.
- **Workflow elapsed time:** The interval from workflow creation to completion, including approval and dependency waits.
- **Comparable runs:** Runs with the same source snapshot, test selection, runner class, toolchain, and recorded cache state.

## Requirements

### REQ-PLATFORM-CI-PERFORMANCE-001: Bounded Claude execution

**Intent:** Stop stalled review jobs without changing contributor approval rules.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-001.1:** Each automatic or comment-triggered Claude job shall have a 30-minute execution budget. GitHub shall cancel jobs that exceed that budget.
- **AC-PLATFORM-CI-PERFORMANCE-001.2:** A timeout shall not count as a successful review. Existing authorization, trigger, credential, and runner boundaries shall remain unchanged.

### REQ-PLATFORM-CI-PERFORMANCE-002: Reusable dependency caches

**Intent:** Reuse compatible dependencies without making test results depend on cache availability.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-002.1:** A successful frontend dependency installation shall support cache reuse by a later compatible run. Evidence shall show an actual save and restore.
- **AC-PLATFORM-CI-PERFORMANCE-002.2:** A missing or unavailable cache shall permit a complete dependency installation. Dependency resolution shall continue to enforce the lockfile.
- **AC-PLATFORM-CI-PERFORMANCE-002.3:** Each E2E execution environment shall support compatible dependency reuse. Cache errors shall not skip installation or conceal dependency errors.

### REQ-PLATFORM-CI-PERFORMANCE-003: Complete verification with less overhead

**Intent:** Reduce repeated test setup and shorten feedback without weakening verification.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-003.1:** Optimized frontend verification shall retain every selected test file and test case exactly once across its test partitions.
- **AC-PLATFORM-CI-PERFORMANCE-003.2:** Browser, multilingual, and production-environment regression tests shall retain their existing behavior. Test files shall retain isolated state.
- **AC-PLATFORM-CI-PERFORMANCE-003.3:** Required frontend verification shall fail when any selected partition or mandatory check fails or is cancelled. Deliberate change-based skips shall remain valid.
- **AC-PLATFORM-CI-PERFORMANCE-003.4:** Partitioned Windows process verification shall retain every runnable native test, fuzz seed corpus, example, and nested subtest in the existing package selection exactly once, with race detection and the existing per-command and job budgets preserved.
- **AC-PLATFORM-CI-PERFORMANCE-003.5:** Windows process verification shall fail when native enumeration, complete disjoint selection, any selected command, or completion evidence fails. A failed or cancelled cohort shall block the existing required backend gate; other started cohorts shall still be joined. Existing independent native Windows checks shall remain required.
- **AC-PLATFORM-CI-PERFORMANCE-003.6:** Both existing frontend partitions shall use the same duration evidence. Missing or unusable evidence shall produce a complete deterministic fallback.
- **AC-PLATFORM-CI-PERFORMANCE-003.7:** Current discovery shall retain new and changed tests. Historical durations shall never authorize a test omission or substitute for current completion evidence.

### REQ-PLATFORM-CI-PERFORMANCE-004: Attributable performance evidence

**Intent:** Distinguish useful optimization from queue variation or reduced coverage.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-004.1:** Performance reports shall distinguish job execution, queue delay, and workflow elapsed time. Missing job evidence shall appear as unknown execution.
- **AC-PLATFORM-CI-PERFORMANCE-004.2:** Optimization comparisons shall identify source, attempts, runner class, cache state, test counts, failures, retries, and sample size.
- **AC-PLATFORM-CI-PERFORMANCE-004.3:** Runner-capacity assessments shall preserve the existing eligible-job boundary. They shall report cost assumptions and rollback conditions before activation.
- **AC-PLATFORM-CI-PERFORMANCE-004.4:** Reports shall distinguish dependency waits from delays after prerequisites finish. Attribution to capacity saturation shall require concurrent-job evidence.

### REQ-PLATFORM-CI-PERFORMANCE-005: Efficient verification within shared capacity

**Intent:** Reduce runner occupancy and scheduling stages within the existing fleet.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-005.1:** Each optimization comparison shall report total runner-minutes, execution critical path, scheduling delay, and workflow elapsed time separately. Failed and cancelled attempts shall remain visible.
- **AC-PLATFORM-CI-PERFORMANCE-005.2:** Frontend setup changes shall preserve the complete test identity multiset, outcomes, isolation, and environment guards. Their adoption shall require lower median execution and runner-minutes across three comparable samples.
- **AC-PLATFORM-CI-PERFORMANCE-005.3:** Additional frontend partitions shall reduce median execution critical path by at least 30%. Their runner-minute increase shall not exceed 10% against the optimized unsharded baseline. Representative loaded feedback shall not regress.
- **AC-PLATFORM-CI-PERFORMANCE-005.4:** Consolidated workflow stages shall preserve required check names and fail on mandatory failures, cancellations, missing evidence, or failed change detection. Deliberate irrelevant-change skips shall still conclude successfully.
- **AC-PLATFORM-CI-PERFORMANCE-005.5:** Performance acceptance shall use the existing runner capacity. More capacity, fewer assertions, disabled isolation, and increased retries shall not count as execution-efficiency improvements.
- **AC-PLATFORM-CI-PERFORMANCE-005.6:** Fixed-job optimizations shall preserve matrix sizes, runner placement, permissions, and the shared 60-job capacity constraint. Waiting time shall remain a separate measure.
- **AC-PLATFORM-CI-PERFORMANCE-005.7:** Rebalancing existing partitions shall reduce median execution critical path without increasing median runner-minutes across three comparable pairs. Reports shall retain loaded feedback observations.

### REQ-PLATFORM-CI-PERFORMANCE-006: Conservative selection for test-only pull requests

**Intent:** Avoid application E2E work when a pull request changes only inputs that cannot affect the application build or E2E execution.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-006.1:** A pull request containing only verified application-independent unit-test changes shall run the affected unit and static checks. It shall omit application E2E execution.
- **AC-PLATFORM-CI-PERFORMANCE-006.2:** Runtime, shared dependency, build, E2E fixture, E2E specification, workflow, ambiguous, or mixed changes shall retain the existing applicable verification. Missing comparison evidence shall select full verification.
- **AC-PLATFORM-CI-PERFORMANCE-006.3:** Change classification shall cover additions, modifications, deletions, and both sides of renames. An unrecognized path shall never prove that a change is test-only.
- **AC-PLATFORM-CI-PERFORMANCE-006.4:** Required E2E checks shall report the deliberate skip and its reason. Main-branch, merge-group, and manual verification shall retain their existing selection policy.

### REQ-PLATFORM-CI-PERFORMANCE-007: Fifteen-minute complete CI

**Intent:** Shorten the time to complete all application verification, rather than only one test step.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-007.1:** Acceptance shall record three complete successful hosted workflow sets with at most 900 seconds from the earliest backend/frontend/E2E workflow creation to the latest completion. Report every sample and intervening failed, cancelled, or partial attempt. This is a measured delivery target, not a guarantee against future runner queues.
- **AC-PLATFORM-CI-PERFORMANCE-007.2:** Every selected backend, PostgreSQL 16/18, native Windows, frontend, normal/container E2E, desktop and Kubernetes compatibility check shall remain required. Partitions shall preserve selected identities, assertions, platform coverage, race checks, isolation, retries and failure propagation.
- **AC-PLATFORM-CI-PERFORMANCE-007.3:** Evidence shall distinguish full-workflow elapsed time, execution, waiting and runner-minutes. Baseline/candidate comparisons shall identify source and selection differences; path-filtered or cancelled runs shall never establish achievement.
- **AC-PLATFORM-CI-PERFORMANCE-007.4:** Candidates may redistribute work across existing hosted jobs after measuring setup costs and available capacity. Paid capacity activation and trust-boundary expansion remain separate operator decisions. Reject coverage loss and loaded feedback regression; report any occupancy increase explicitly.

### REQ-PLATFORM-CI-PERFORMANCE-008: Reusable compilation caches

**Intent:** Reuse compatible compilation work while every required command still validates the current source.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-008.1:** Compilation reuse shall support refresh after source changes without a dependency change. Reuse shall retain compiler, platform, and build-environment compatibility.
- **AC-PLATFORM-CI-PERFORMANCE-008.2:** Missing, incompatible, unavailable, or unreadable caches shall leave required build and verification commands operational. Cache results shall never replace their exits.
- **AC-PLATFORM-CI-PERFORMANCE-008.3:** Cache publication shall retain branch access boundaries and have an explicit owner per shared snapshot. Evidence shall include publication and restoration costs.

## Out of scope

- Automatic approval, automatic reruns, or changes to contributor trust policy.
- Automatic paid-capacity activation or expansion to protected runner workloads.
- Lower test coverage, disabled isolation, increased retries, or increased E2E worker counts.
- Application UI changes, release build redesign, or changes to an externally managed dashboard.

## Related contracts

- [External CI runner capacity](external-e2e-runner-capacity.md)
- [Contributor automation](../../ci/requirements/unified-contributor-pr-automation.md)

## Implementation plans

- [CI performance](../../../plans/ci-performance/plan.md)
- [First-commit Unstage delivery dependency](../../../plans/unstage-all-before-first-commit/plan.md)
