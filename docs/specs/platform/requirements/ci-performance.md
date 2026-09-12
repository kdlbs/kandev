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

### REQ-PLATFORM-CI-PERFORMANCE-002: Reusable frontend dependency cache

**Intent:** Reuse compatible dependencies without making test results depend on cache availability.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-002.1:** A successful frontend dependency installation shall support cache reuse by a later compatible run. Evidence shall show an actual save and restore.
- **AC-PLATFORM-CI-PERFORMANCE-002.2:** A missing or unavailable cache shall permit a complete dependency installation. Dependency resolution shall continue to enforce the lockfile.

### REQ-PLATFORM-CI-PERFORMANCE-003: Complete frontend verification with less overhead

**Intent:** Reduce repeated test setup and shorten feedback without weakening verification.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-003.1:** Optimized frontend verification shall retain every selected test file and test case exactly once across its test partitions.
- **AC-PLATFORM-CI-PERFORMANCE-003.2:** Browser, multilingual, and production-environment regression tests shall retain their existing behavior. Test files shall retain isolated state.
- **AC-PLATFORM-CI-PERFORMANCE-003.3:** Required frontend verification shall fail when any selected partition or mandatory check fails or is cancelled. Deliberate change-based skips shall remain valid.

### REQ-PLATFORM-CI-PERFORMANCE-004: Attributable performance evidence

**Intent:** Distinguish useful optimization from queue variation or reduced coverage.

#### Acceptance criteria

- **AC-PLATFORM-CI-PERFORMANCE-004.1:** Performance reports shall distinguish job execution, queue delay, and workflow elapsed time. Missing job evidence shall appear as unknown execution.
- **AC-PLATFORM-CI-PERFORMANCE-004.2:** Optimization comparisons shall identify source, attempts, runner class, cache state, test counts, failures, retries, and sample size.
- **AC-PLATFORM-CI-PERFORMANCE-004.3:** Runner-capacity assessments shall preserve the existing eligible-job boundary. They shall report cost assumptions and rollback conditions before activation.

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
