---
status: current
system: platform
requirements:
  - REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-001
  - REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-002
created: 2026-08-30
updated: 2026-08-30
owners:
  - kandev
---
# Duration-aware E2E sharding and CI reliability system design

## Purpose and boundaries

This design defines the operational boundary for Kandev's Playwright E2E
workflow. It keeps test work balanced, removes repeatable setup work from the
container-backed cohort where safe, and makes retry and timing evidence
actionable.

The design covers GitHub Actions workflow orchestration, E2E planning and
reporting, browser provisioning, and the synchronization contract of the
affected regression test. It does not change application behavior, the
worker-scoped backend fixture model, the required E2E gate, or the normal
matrix shape.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-001` | [Execution flow](#execution-flow), [Existing planning contract](#existing-planning-contract) |
| `REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-002` | [Browser cache contract](#browser-cache-contract), [Reliability and flake contract](#reliability-and-flake-contract), [Observability](#observability) |

## Design invariants

- The normal cohort remains fourteen shards covering Chromium and mobile
  Chrome, and the container cohort remains six shards covering Docker and SSH.
- Each shard remains single-worker until a measured concurrency change passes
  the existing isolation and retry criteria.
- A timing profile or browser cache accelerates planning or setup only. A
  missing, stale, or inaccessible optimization input has a deterministic
  fallback and cannot change the selected test set.
- A passing-after-retry result remains visible, even when the normal pull
  request lane is allowed to finish green.
- CI time analysis separates runner queue time from job setup and test
  execution time. Queue time is capacity evidence, not a test-synchronization
  signal.

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `.github/workflows/e2e-tests.yml` | Detect relevant changes, build artifacts, obtain timing data, run the two E2E cohorts, and publish reports. |
| `apps/web/e2e/scripts/e2e-timings.ts` | Maintain the bounded successful-`main` timing profile and classify profile fallbacks. |
| `apps/web/e2e/scripts/plan-shards.ts` | Discover the cohort catalog, create deterministic duration-aware manifests, and validate coverage. |
| `apps/web/e2e/scripts/run-planned-shard.ts` | Validate and execute one explicit manifest selection while preserving Playwright reporting and retry settings. |
| `apps/web/e2e/scripts/retry-summary.ts` and `flake-report.ts` | Group attempts by stable test key and publish retry and historical flake evidence. |
| `.github/docker/ci-base/Dockerfile` | Define the pinned Playwright browser source baked into the runtime image. |
| `apps/web/e2e/tests/review/review-file-status.spec.ts` | Verify the selected review file's status using a selection-scoped readiness assertion. |

## Existing planning contract

The build job remains the only producer of run-scoped normal and container
manifests. It obtains the latest eligible successful-`main` profile, discovers
the catalog from the authoritative Playwright configuration, and produces
explicit file selections. The planner continues to use longest-processing-time
bin packing at project/file granularity with deterministic tie-breaking.

The normal and container matrix jobs download only their cohort manifest. They
do not add ordinal Playwright sharding as a hidden fallback. An unavailable
profile uses the existing count-based plan and reports that mode. An invalid
manifest fails before test execution.

The report job remains responsible for merged reports, the next timing-profile
candidate, retry summaries, and predicted-versus-actual shard diagnostics.
The aggregate gate retains its current cancellation and failure semantics.

## Execution flow

1. The change-detection job decides whether the E2E workflow is required.
2. The build job installs dependencies, builds the backend and web artifacts,
   packages the plugin fixture, resolves the eligible `main` profile, and
   creates validated manifests for both cohorts.
3. Normal matrix jobs download artifacts and run their explicit manifest
   selection in the existing runtime image.
4. Container matrix jobs restore the browser cache described below, use the
   pinned image extraction fallback when needed, then run the explicit
   Docker/SSH manifest selection on the host runner.
5. The report job merges blobs, records setup and execution diagnostics,
   creates the retry and flake summaries, and publishes a new profile only
   when the run is an eligible successful `main` result.
6. The required gate evaluates the final job results.

## Browser cache contract

The container-backed jobs currently copy `/ms-playwright` from the pinned
runtime image to `/tmp/ms-playwright` on every host runner. The cache removes
that repeated copy after the first successful warm-up without replacing the
image as the source of truth.

The existing host job uses the following sequence:

1. Resolve the runtime image manifest to a sha256 digest. If resolution fails,
   stop before using a mutable image reference.
2. Restore `/tmp/ms-playwright` from the GitHub Actions cache before the
   digest-pinned Docker pull and copy step.
3. Use the runner operating system, Playwright browser source, resolved image
   digest, workflow run ID, and attempt number in the primary key. Supply a
   stable prefix ending before the run-specific suffix as the restore key. A
   browser-source or image change therefore cannot reuse an incompatible
   entry, while a failed entry cannot win an exact-key lookup forever.
4. On a matching exact or prefix hit, set `PLAYWRIGHT_BROWSERS_PATH` to the
   restored path, verify that Chromium is usable, and skip the image pull and
   copy.
5. On a miss, stale entry, cache-service error, or failed verification, pull
   the resolved digest, run the existing image extraction and browser smoke
   check, and save a successful fallback under the current run's new primary
   key. Cache restore and save failures are reported as setup diagnostics and
   do not fail a test whose digest-pinned fallback succeeds.

The cache contains browser binaries only. It does not contain repository
source, credentials, test results, or generated application artifacts. A
cache hit cannot alter the manifest, project filters, retries, timeout, or
worker count. The first run after a key change is expected to pay the current
provisioning cost.

## Reliability and flake contract

The retry summary continues to use the stable project, repository-relative
file, and full-title key. It records attempt count, each attempt's status,
error category, and links to available trace, screenshot, or error-context
artifacts. The historical report retains recent successful-main entries and
cross-checks repeat offenders against the Playwright result data.

The explicit diagnostic lane sets `failOnFlakyTests` so a test that passes only
after retry fails that lane. The normal pull request lane may retain retries,
but its report must still expose the passed-after-retry result.

For the review-file-status regression, the selected moved-file header is the
readiness anchor. Assertions about the moved-file explanation and the absence
of its own loading marker are scoped to that file's section. Loading markers
for other lazy-loaded files in the dialog are not evidence that the selected
file is unready and must not fail the test.

This synchronization uses existing DOM state and causal Playwright assertions.
It does not add a fixed sleep or enlarge a timeout to mask the race.

## Timing and setup data

The report preserves these distinct measurements:

- runner queue and job start delay, derived from workflow and job timestamps;
- dependency installation;
- runtime image startup and browser provisioning;
- Playwright test execution per shard;
- report merge and aggregate-gate delay.

Predicted-versus-actual shard data remains keyed by cohort and shard number.
It includes the planning mode, unknown and warm counts, target duration, and
actual duration. The container summary emits the resolved image digest,
logical cache state, setup mode, restore/verification/extraction/save outcomes,
and total browser setup elapsed time. These values let setup savings be
compared with the test and queue budgets without claiming per-step durations
that the workflow does not measure.

## Failure modes and recovery

| Condition | Required behavior |
| --- | --- |
| Timing profile is absent or unusable | Use the validated count-based plan and report the fallback. |
| Manifest is missing, malformed, incomplete, or overlapping | Fail the planning or shard job before test execution. |
| Runtime image digest cannot be resolved | Fail before cache restore or image use; never fall back to the mutable convenience tag. |
| Browser cache hit | Verify the restored browser path, then skip image extraction only when verification succeeds. |
| Browser cache miss, stale key, cache outage, or failed verification | Use the digest-pinned runtime image extraction and smoke check, then save under a new run-specific key when possible. Test selection and correctness remain unchanged. |
| Cache save is unavailable | Finish the successful test run and report that the next run will need the fallback path. |
| Test passes after retry | Preserve the final pass, attempt details, error category, and diagnostic artifacts; fail the explicit diagnostic lane. |
| Selected review file is loaded while other lazy sections still show placeholders | Pass the selection-scoped assertion; unrelated placeholders are ignored. |
| Runner queue is long | Preserve job timestamps and execution diagnostics so capacity delay is not mistaken for shard imbalance. |

## Security and workflow constraints

- Keep all workflow actions pinned to their existing reviewed commit SHAs.
- Use the existing read-only package and artifact permissions. The browser
  cache must not require a new secret or broaden pull-request credentials.
- Resolve `runtime-latest` to a sha256 digest and use the digest-pinned image
  reference for fallback extraction. Restore only the exact browser path used
  by the host job and use the digest-scoped cache prefix as a compatibility
  boundary. Do not restore arbitrary files into the workspace or Docker state.
- Preserve the repository's workflow rules for untrusted pull requests,
  checkout credentials, and container cleanup.

## Observability

The workflow retains the existing `e2e-timing-diagnostics`,
`e2e-retry-summary`, `e2e-flake-history`, timing-profile, manifest, blob, and
merged-report artifacts. The container jobs add a short step summary with
cache state and setup timings. The report continues to make the following
questions answerable from one run:

- Did the run use a timing profile or count fallback?
- Was the slowest shard caused by test execution or setup?
- Which immutable image digest and cache state did the run use, and did the
  cache hit remove the extraction step?
- Did any test pass only after retry, and is it a repeat offender?
- Was the total wall time dominated by runner queue capacity?

Three successful `main` runs remain the rollout sample for the shard-tail
target. A cache change is retained only if it reduces the container setup
component without increasing retries, failed browser verification, or test
failures.

## Out of scope

- changing the fourteen normal or six container shards;
- enabling `workers: 2` by default;
- splitting the normal matrix into separate mobile and desktop cohorts;
- changing the Playwright retry count, test timeout, or required gate;
- replacing the rolling profile with a service or checked-in shard lists;
- hiding retry groups by increasing waits or weakening assertions;
- changing application code solely to accommodate this CI optimization.

## Related decisions and delivery plan

- [Duration-aware E2E sharding uses rolling `main` timings](../../../decisions/2026-08-10-duration-aware-e2e-sharding.md)
- [Browser cache for host-runner E2E provisioning](../../../decisions/2026-08-30-e2e-browser-cache.md)
- [E2E CI efficiency plan](../../../plans/e2e-ci-efficiency/plan.md)
