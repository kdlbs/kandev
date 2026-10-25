# ADR-2026-09-06-opt-in-external-e2e-runners: Opt in selected Linux CI jobs to external runners

**Status:** accepted
**Date:** 2026-09-06
**Area:** infra, workflow, security

## Context

The E2E workflow creates fourteen normal Playwright shards, six host-Docker and
Kind shards, two Kubernetes compatibility jobs, and supporting build, report,
and gate jobs. Backend and frontend workflows also contain long Linux test
jobs. Recent timing work reduced shard imbalance, but GitHub-hosted runner wait
can still dominate the merge-queue critical path.

An external provider can add capacity. However, hard-coded labels make rollback
require another workflow merge. One label for all jobs also prevents low-cost
control jobs from using a smaller instance class.

Operators need one fast switch during a queue incident, plus a controlled way
to pilot only part of the eligible workload. They also need to keep approved
instance labels configured between incidents. Provider-wide routing moves jobs
outside the reviewed Docker, platform, and credential boundaries.

## Decision

Use `KANDEV_CI_EXTERNAL_ENABLED` as the activation switch. A value of `true`
activates configured external tiers for newly dispatched eligible jobs. Any
other value keeps eligible jobs on `ubuntu-latest`.

Use `KANDEV_CI_EXTERNAL_PERCENT` to control the fraction of eligible jobs that
the external fleet receives while burst mode is active. The default is `0`.
The planner treats `0` as all GitHub-hosted and `100` as all external jobs with
non-empty tier labels. Values from `1` through `99` are allocated
deterministically per job family. Matrix families receive exactly
`floor(N * percentage / 100)` external instances. Singleton families hash the
workflow run into a stable bucket and approach the percentage across runs. A
missing value means `0`; invalid or out-of-range values fail closed to
GitHub-hosted runners with a visible warning.

Store the complete external labels in two independent repository variables:

- `KANDEV_CI_RUNNER_LIGHT` for change detection and the required gate.
- `KANDEV_CI_RUNNER_STANDARD` for normal E2E shards and eligible frontend
  tests. Credential-bearing build/report and backend checkout paths remain
  hosted during the pilot.

If burst mode is active but a tier label is empty, jobs in that class use
`ubuntu-latest`. A non-empty invalid label remains visible as a queued or failed
job. Do not mix instance types inside the normal shard matrix.

Apply the same tier contract to eligible read-only Linux jobs in the E2E and
frontend workflows, plus the backend aggregate gate. Keep E2E build/report
jobs, backend checkout/static/test-shard/ambient jobs, architecture and
harness linters, the Playwright image-resolution job, Docker/Kind shards,
Kubernetes compatibility jobs, desktop E2E, Postgres service jobs, and Windows
jobs on `ubuntu-latest` during the initial pilot. Do not apply this variable to
`pull_request_target`, production, deployment, release, publishing, signing,
or other credential-bearing jobs.

Use Ubicloud x64 Ubuntu 24.04 as the initial provider. Configure standard-2 for
the light tier and standard-4 for the standard tier. Keep Premium Runners
disabled because Ubicloud controls them at the account level.

Set burst mode to `false` after the queue recovers. This change affects only
newly dispatched jobs. GitHub does not migrate queued or active jobs.

The external provider is part of the selected jobs' trust boundary. The pilot
adds no secrets or permissions, retains `persist-credentials: false`, and
keeps cache branch protection enabled. Expansion to protected workflows
requires a measured compatibility and security review.

## Consequences

- Operators can enable burst capacity or return to GitHub-hosted runners
  with one repository-variable change.
- Runner labels remain provider-neutral workflow data. Ubicloud and runner
  size remain operator-selected values.
- Operators can resize one tier without changing other job classes.
- Operators can set `KANDEV_CI_EXTERNAL_PERCENT` to `0`, `50`, or `100` to
  stage a pilot without editing workflow files.
- Empty tier labels provide a safe GitHub-hosted fallback during partial
  configuration.
- An invalid or unavailable configured label can leave new jobs queued until
  the operator clears the variable and reruns them. The workflow does not
  silently use another fleet.
- External jobs expose their short-lived GitHub token, source, artifacts, and
  logs to the approved provider under the existing read-only permissions.
- The pilot can reduce runner wait, but it cannot remove merge-group ordering,
  slow tests, cold caches, or artifact-transfer time.
- A planner job adds a small GitHub-hosted dependency before eligible jobs can
  start. Exact compute-share percentages across unrelated workflows are not
  promised.

## Alternatives Considered

### Hard-code Ubicloud labels

Rejected because enablement, size changes, and rollback each require a
workflow commit and merge-queue pass.

### Use one runner-label variable as the activation switch

Rejected because one label gives light and standard workloads the same
instance size. Changing the label also erases the configured paid-runner value
when the operator returns to GitHub-hosted capacity.

### Use tier labels without a separate activation switch

Rejected because each burst requires multiple variable changes. A partial
change can unintentionally split eligible jobs across paid and GitHub-hosted
capacity.

### Enable provider-wide premium routing

Rejected because it removes job-level selection and routes workloads
whose trust and host-dependency boundaries are outside this pilot.

### Move every Linux CI job in the first rollout

Rejected because host-Docker, Kind, Kubernetes compatibility, image-resolution,
desktop, service, and Windows jobs need separate compatibility evidence. Their
smaller initial capacity benefit does not justify widening the pilot boundary.

### Use a direct percentage expression in `runs-on`

Rejected because GitHub expressions provide comparisons and boolean operators,
but no random or modulo operator. A read-only planner job can produce a stable
assignment for matrix and singleton job families and can fail closed on invalid
operator input.

### Distribute an exact percentage of all workflow compute

Rejected because workflows contain different job-family sizes and runtimes.
The contract allocates matrix instances exactly and singleton jobs by stable
cohort, then measures real queue and execution behavior before any adaptive
policy is considered.

### Fall back automatically inside a workflow run

Rejected because GitHub chooses `runs-on` before job execution. Implementing a
second-fleet retry duplicates job graphs and complicates the required gate.
It can also run the same untrusted workload on an unreviewed fleet. Manual
variable rollback keeps the selected trust boundary explicit.
