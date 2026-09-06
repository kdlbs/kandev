---
status: current
system: platform
requirements:
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002
  - REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003
created: 2026-09-06
owners:
  - kandev
---

# External CI Runner Capacity System Design

## Purpose and boundaries

This design adds provider-neutral runner tiers to the normal Linux CI
workflows. Two repository variables store the approved Linux x64 labels. One
burst-mode variable activates those labels when GitHub-hosted capacity is full.
An optional percentage variable lets operators run a deterministic fraction of
eligible jobs on the external fleet during a pilot.

The tier labels remain configured while burst mode is inactive. An operator
can therefore activate or deactivate paid capacity with one variable change.
The operator can also change an instance type without a workflow edit.

The design changes only runner placement. The duration-aware planner, shard
manifests, build artifacts, reports, required gates, and workflow permissions
remain authoritative. Host-Docker, Kind, compatibility, image-resolution,
desktop, service, Windows, checkout-token, report-token, release, and
deployment jobs remain outside the initial pilot.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-001` | [Runner-selection contract](#runner-selection-contract), [Job placement](#job-placement), [Failure and recovery](#failure-and-recovery) |
| `REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-002` | [Security and trust boundary](#security-and-trust-boundary), [Rollout and observability](#rollout-and-observability) |
| `REQ-PLATFORM-EXTERNAL-E2E-RUNNER-CAPACITY-003` | [Percentage allocation](#percentage-allocation), [Workflow integration](#workflow-integration), [Failure and recovery](#failure-and-recovery) |

## Runner-selection contract

The repository stores these Actions variables:

| Variable | Purpose | Initial Ubicloud value |
| --- | --- | --- |
| `KANDEV_CI_EXTERNAL_ENABLED` | Activates configured external tiers only when its value is `true`. | Unset or `false` |
| `KANDEV_CI_EXTERNAL_PERCENT` | Selects the percentage of eligible job instances assigned to external capacity when burst mode is active. | Unset or `0` |
| `KANDEV_CI_RUNNER_LIGHT` | Selects capacity for change detection and required gates. | `ubicloud-standard-2-ubuntu-2404` |
| `KANDEV_CI_RUNNER_STANDARD` | Selects capacity for builds, reports, browser tests, and Linux test jobs. | `ubicloud-standard-4-ubuntu-2404` |

The workflow maps a light planner assignment with this `runs-on` expression:

```yaml
runs-on: ${{ needs.runner_plan.outputs.changes_runner == 'external' && vars.KANDEV_CI_RUNNER_LIGHT || 'ubuntu-latest' }}
```

Standard jobs use the same expression with `KANDEV_CI_RUNNER_STANDARD`. Matrix
jobs use `matrix.runner` instead of a singleton output. GitHub evaluates the
expression before it dispatches the job. If external capacity is not selected,
the expression selects `ubuntu-latest`.

Percentage rollout cannot be implemented safely with a direct `runs-on`
expression. GitHub expressions provide comparisons and boolean operators but no
random or modulo operator. A planner job therefore computes assignments before
eligible jobs start and exposes only validated runner values through job
outputs. Downstream jobs consume those outputs with `needs` and, for matrices,
`fromJSON`.

If burst mode is active, the expression selects the complete label for that
tier. An empty tier label uses `ubuntu-latest`. A non-empty invalid label stays
visible as a queued or failed job.

Variable changes affect jobs that GitHub dispatches after the change. GitHub
does not migrate queued or active jobs. To recover from an external incident,
set `KANDEV_CI_EXTERNAL_ENABLED` to `false` and rerun the workflow.

The workflow does not parse provider names or instance sizes. It also does not
mix instance types inside one shard cohort. Comparable runner capacity keeps
the duration-aware timing profile useful across normal shards.

## Percentage allocation

`.github/scripts/runner-plan.py` is a read-only planner that runs on
`ubuntu-latest`. It receives the workflow identifier, run identifier, job
family descriptors, burst switch, percentage, and tier labels. It validates
the percentage and emits JSON assignments plus a visible warning when the
percentage is invalid.

The planner applies these rules:

1. Any burst value other than `true`, or a percentage of `0`, assigns every
   eligible instance to `ubuntu-latest`.
2. A percentage of `100` assigns every eligible instance with a non-empty tier
   label to that tier. Empty tier labels still use `ubuntu-latest`.
3. For a matrix family with `N` instances, it hashes the workflow, job family,
   run identifier, and matrix key, sorts the keys, and assigns exactly
   `floor(N * percentage / 100)` instances to the external tier.
4. For a singleton family, it hashes the workflow, job-family key, and run
   identifier into a stable bucket. A rerun keeps the same allocation, and
   different workflow runs approach the configured percentage.
5. A missing percentage is treated as `0`. A non-integer or out-of-range
   percentage assigns all instances to `ubuntu-latest` and emits a planner
   warning. It never emits an unvalidated external label for invalid input.

Each job family is allocated independently. At 50 percent, the fourteen E2E
shards receive seven external assignments. Protected backend checkout jobs
remain hosted; the backend aggregate gate and frontend jobs are still
eligible. Singleton jobs use a stable hash cohort, so their share approaches
50 percent across runs. The planner does not promise an exact percentage of
total compute across workflows with different family sizes.

## Workflow integration

Each workflow with eligible jobs adds a small planner job on
`ubuntu-latest`. Existing change-detection jobs can own this planning step when
they already run for the workflow. Always-run workflows use a dedicated
read-only planner job. Eligible jobs depend on the planner only for their
runner assignment; their existing test dependencies, matrices, artifacts,
timeouts, permissions, and required conclusions remain unchanged.

The planner steps use the reusable composite action at
`.github/actions/plan-external-runners/action.yml`. The action owns the
validated environment wiring and `runner-plan.py` invocation, while each
workflow keeps only its job-specific output mapping. This keeps the job
contract explicit without copying planner logic between workflows.

The planner output is limited to the enum values `github` and `external`. The
workflow maps `external` to one of the configured tier variables. Protected
jobs keep explicit `runs-on: ubuntu-latest` and do not consume planner output.
Contract tests assert the output wiring and the protected-job boundary.

## Job placement

| Job | Initial runner selection | Rationale |
| --- | --- | --- |
| `E2E changes`, `e2e-gate` | Light tier with GitHub fallback | Control jobs are short but can wait many minutes for capacity. |
| `E2E build`, `e2e-report` | `ubuntu-latest` | They query GitHub history or download artifacts with the job token. |
| `e2e` | Standard tier with GitHub fallback | The fourteen normal shards need comparable CPU and memory and do not use explicit repository-token inputs. |
| `Backend changes`, `static_checks`, `test_shards`, `test_ambient_env` | `ubuntu-latest` | Their checkout action receives the short-lived job token. |
| `Backend test` gate | Light tier with GitHub fallback | The aggregate gate is short and has no checkout or service dependency. |
| `Frontend changes`, `frontend-gate` | Light tier with GitHub fallback | Control jobs are short and queue-sensitive. |
| `Frontend frontend` | Standard tier with GitHub fallback | The single frontend test job is a recurring queue bottleneck. |
| `Architecture lint`, action-pinning, harness-lint | `ubuntu-latest` | Their checkout action receives the short-lived job token. |
| `playwright_image` | `ubuntu-latest` | Retains the reviewed host-Docker and GHCR metadata path during the pilot. |
| `e2e-containers` | `ubuntu-latest` | Requires direct host Docker and creates Kind resources. |
| `e2e-kubernetes-compatibility` | `ubuntu-latest` | Requires direct host Docker and pinned Kind clusters. |
| `desktop-e2e` | `ubuntu-latest` | Retains the dedicated desktop container and toolchain boundary during the pilot. |
| Backend `postgres-boot`, Windows, and container/service jobs | `ubuntu-latest` | Preserve host-service, operating-system, and credential boundaries. |

The runner expression does not alter `needs`, `if`, the matrix, the container
image, the environment, the timeout, or artifacts. The gate keeps
`if: always()` and its current result rules.

## Workflow contract coverage

The E2E, backend, and frontend workflow contract tests own the executable
placement contract. Dedicated checks cover the architecture-lint,
action-pinning, and harness-lint workflows. They verify the exact
variable-backed `runs-on` expression for both eligible classes. They also
verify that every protected CI job uses `runs-on: ubuntu-latest` without the
burst, percentage, or tier variables.

The contract test also protects the provider-neutral behavior: it asserts the
variable names and GitHub fallback, not the Ubicloud labels. Provider
installation and live variable values remain operator-owned external state.

## Security and trust boundary

An external runner executes pull-request and merge-group code. It receives the
short-lived job token, source, artifacts, and logs for its job. Provider
isolation does not make the provider part of GitHub's hosted trust boundary.

The initial eligible jobs keep the workflow's existing top-level read-only
permissions: `contents: read`, `actions: read`, and `packages: read`. Checkout
continues to use `persist-credentials: false`. Jobs whose checkout, GitHub API,
or artifact action receives the short-lived token remain on `ubuntu-latest`.
The change adds no repository, environment, deployment, publishing, signing,
or production secret.

The initial Ubicloud installation uses clean ephemeral x64 VMs and just-in-time
runner registration. Keep the default branch protection if the Ubicloud
transparent cache is enabled. Do not replace the pinned `actions/cache`
references during this rollout.

Jobs triggered by `pull_request_target`, jobs with production or deployment
credentials, checkout-token paths, E2E build/report token paths, releases,
publishing, signing, and Windows builds are not eligible for burst mode.
Future container jobs can use a heavy tier only after a separate host-
dependency, performance, and security review.

Ubicloud Premium Runners remain disabled. Ubicloud controls Premium Runners at
the account level, so that setting cannot provide job-level tier selection.

## Failure and recovery

| Condition | Required behavior |
| --- | --- |
| Burst mode is absent, empty, or not `true` | GitHub dispatches every eligible job to `ubuntu-latest`. |
| Burst mode is `true` and the tier has an approved label | GitHub dispatches new jobs in that class to the configured fleet. |
| Burst mode is `true` and the tier label is empty | GitHub dispatches that job class to `ubuntu-latest`. |
| A non-empty tier label is unavailable or invalid | The job remains visibly queued or fails. No unreviewed fallback runs. |
| External provider has an incident | Set burst mode to `false` and rerun. GitHub does not migrate queued or active jobs. |
| External image differs from the GitHub image | The repository container images own most toolchains. The pilot must verify the host-only change-detection and gate behavior. |
| External cache is cold | Existing dependency and artifact paths continue to work. Cache state can affect duration but not correctness. |

## Rollout and observability

1. Merge the workflow while `KANDEV_CI_EXTERNAL_ENABLED` is absent or `false`.
2. Install the Ubicloud GitHub App only for the Kandev repository.
3. Keep transparent-cache branch protection enabled.
4. Disable Premium Runners in the Ubicloud account.
5. Configure `KANDEV_CI_RUNNER_LIGHT` as
   `ubicloud-standard-2-ubuntu-2404`.
6. Configure `KANDEV_CI_RUNNER_STANDARD` as
   `ubicloud-standard-4-ubuntu-2404`.
7. When the GitHub queue is full, set `KANDEV_CI_EXTERNAL_ENABLED` to `true`.
8. Observe at least three representative pull-request or merge-group runs.
   Record runner labels, queue delay, build duration, shard tail, report/gate
   delay, cancellations, retries, infrastructure failures, and cache behavior.
9. After the queue recovers, set `KANDEV_CI_EXTERNAL_ENABLED` to `false`.
10. If the pilot shows a regression, disable burst mode and rerun failed jobs.

Keep the two tier labels configured between bursts. Change the standard-tier
label to `ubicloud-standard-8-ubuntu-2404` only after CPU or memory evidence.

The existing timing diagnostics separate queue delay from setup and test
execution. The pilot succeeds when runner wait decreases without a higher job
failure, cancellation, retry, cache-verification, or artifact-transfer rate.
GitHub merge-group ordering remains an independent source of serialization and
is not attributed to runner capacity.

## Related decisions

- [Opt in selected Linux CI jobs to external runners](../../../decisions/2026-09-06-opt-in-external-e2e-runners.md)
- [Duration-aware E2E sharding uses rolling `main` timings](../../../decisions/2026-08-10-duration-aware-e2e-sharding.md)
- [Cache host-runner E2E browser provisioning](../../../decisions/2026-08-30-e2e-browser-cache.md)
