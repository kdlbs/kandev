---
status: active
system: platform
created: 2026-09-06
owners:
  - kandev
---

# External CI Runner Capacity Requirements

## Overview

The merge queue can wait on GitHub-hosted runner capacity even when test jobs
are balanced. Repository operators need a reversible external runner option
for normal Linux CI workloads. This option must preserve required gates, test
selection, and privileged workflow boundaries.

The platform system owns this contract because runner capacity is a shared
operational guarantee across CI workflows. The CI automation system continues
to own contributor trust gates for privileged pull-request automation.

## Terminology

- **Eligible Linux CI job:** A read-only Linux job in the E2E or frontend
  workflows, plus backend aggregate gates that do not receive checkout or
  report tokens. Credential-bearing checkout, report, service, and platform
  jobs are protected and stay on GitHub-hosted runners.
- **Light tier:** External capacity for control jobs that run for seconds.
- **Standard tier:** External capacity for build, report, browser-test, and
  Linux test jobs that need more CPU or memory.
- **Burst mode:** The operator-controlled state that activates configured
  external runner tiers for new jobs.
- **Protected CI job:** A host-Docker, Kind, Kubernetes compatibility, image,
  desktop, Windows, credential-bearing, release, or deployment job that stays
  on a GitHub-hosted runner during the initial rollout.
- **Percentage rollout:** A deterministic allocation of eligible job
  instances between GitHub-hosted and configured external capacity.

## Requirements

### REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001: Reversible tiered Linux CI runner selection

**Intent:** Let repository operators configure runner tiers once. They can
then activate or deactivate external capacity with one repository variable.

**User story:** As a repository operator, I want to use an approved external
runner fleet, so that I can reduce merge-queue wait across Linux CI.

#### Acceptance criteria

- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.1:** When the burst-mode
  variable is not `true`, every eligible Linux CI job shall select
  `ubuntu-latest`. Configured tier labels shall remain unchanged.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.2:** When burst mode is
  `true` and the percentage is `100`, each eligible Linux CI job shall use its
  configured light or standard runner label.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.3:** When burst mode is
  `true` and a required tier label is empty, that job class shall select
  `ubuntu-latest` for every percentage allocation.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.4:** Changing a tier label
  shall change the instance type for newly dispatched jobs in that class. It
  shall not require a workflow edit or a percentage change.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.5:** Changing burst mode, a
  tier label, or the percentage shall preserve each workflow's matrix, test
  selection, artifacts, dependencies, timeouts, permissions, and required
  conclusion.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.6:** When a non-empty tier
  label cannot accept a job, that job shall remain visibly queued or fail. The
  workflow shall not silently select an unapproved fleet.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001.7:** The same burst switch and
  tier labels shall apply consistently to eligible E2E, backend-gate, and
  frontend jobs. Protected lint workflows shall remain GitHub-hosted.

### REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002: Initial runner trust boundary

**Intent:** Limit the first external-runner pilot to jobs with reviewed
permissions and runtime dependencies for that provider.

#### Acceptance criteria

- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.1:** During the initial
  rollout, protected CI jobs shall select `ubuntu-latest`
  regardless of burst mode or configured tier labels.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.2:** The external-runner
  option shall not add secrets or broaden `GITHUB_TOKEN` permissions. It shall
  not persist runner state. It shall not move credential-bearing jobs to the
  external fleet.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002.3:** Operators shall be able
  to identify each job's class, configured tier, and selected fleet. They shall
  compare queue delay, execution time, and reliability before expansion.

### REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003: Deterministic percentage rollout

**Intent:** Let operators test an external fleet with a controlled fraction of
eligible jobs while the remaining jobs use GitHub-hosted capacity.

#### Acceptance criteria

- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.1:** When burst mode is not
  `true`, the percentage shall have no effect and every eligible job shall use
  `ubuntu-latest`.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.2:** When burst mode is
  `true` and the percentage is `0`, every eligible job shall use
  `ubuntu-latest`. When it is `100`, every eligible job with a non-empty tier
  label shall use that tier.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.3:** When the percentage is
  between `0` and `100`, the planner shall allocate each job family
  deterministically between GitHub-hosted and external capacity. A rerun of
  the same workflow run shall keep the same allocation.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.4:** For a matrix family with
  `N` instances, the planner shall allocate `floor(N * percentage / 100)`
  instances to external capacity. A single-instance family shall use a stable
  allocation across workflow runs that approaches the configured percentage.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.5:** A missing percentage
  shall be treated as `0`. A malformed or out-of-range percentage shall fail
  closed to `ubuntu-latest` and emit a visible planner warning. Neither case
  shall select an external label.
- **AC-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003.6:** Percentage allocation
  shall not change job names, matrix values, artifacts, dependencies, test
  selection, timeouts, permissions, or required conclusions.

## Out of scope

- Automatically activating burst mode from queue metrics.
- Automatically retrying an unavailable external runner label on GitHub-hosted
  capacity within the same workflow run.
- Weighted allocation by historical runtime or queue metrics.
- Moving Docker/Kind shards, Kubernetes compatibility, image resolution,
  desktop E2E, Postgres service jobs, Windows jobs, or credential-bearing jobs
  in the initial rollout.
- Adding a heavy tier before measurements show that a workload needs it.
- Guaranteeing an exact percentage across workflows with different job counts
  in one workflow run.
- Enabling an external provider's global or premium-fleet routing setting.
- Selecting ARM runners for x64 E2E binaries and containers.
