---
status: implemented
created: 2026-08-10
---

# Duration-aware E2E sharding and CI reliability

## Why

The E2E workflow currently uses Playwright's ordinal test-level sharding. It
keeps test counts roughly even, but not execution time. In the investigated
run, the normal matrix had fourteen shards ranging from 3m52 to 17m57, while
the container matrix had six shards ranging from about 3m to 9m59. The slowest
shards therefore determine most of the wall time even when the other runners
are idle.

The same investigation found seven retry groups in the normal PR run. The
retry policy made the final run green, but several failures represented
unstable test data, leaked state, or missing synchronization rather than
transient infrastructure. Container tests also remove globally labelled
containers, so unrelated daemon state can affect their result.

This capability makes shard assignment adapt to the suite as it grows. It
also turns the known reliability findings into deterministic regression work
and makes passed-after-retry results visible.

## What

The E2E workflow gains two duration-aware cohorts:

- **Normal:** the existing fourteen-shard Chromium and mobile-Chrome matrix.
- **Containers:** the existing six-shard Docker and SSH matrix.

Each run creates an ephemeral manifest for each cohort. The manifest contains
the explicit test files assigned to each shard, the estimated cost, and the
profile source used for that estimate. Playwright runs those files directly;
the first implementation keeps `workers: 1` and the existing project
boundaries.

The estimates come from a rolling timing profile produced by successful runs
on `main`. A later `main` run merges its new successful samples into the
profile, so adding tests and changing test durations update future plans
without a hand-edited shard file.

## Timing profile contract

The profile is an artifact, not a checked-in source file. Its stable test key
is:

```text
<playwright project> + <repository-relative spec path> + <full test title>
```

Each entry records the source file hash, recent successful samples, a p50 and
p75 duration, the last source commit, and the last successful run. The
collector retains a bounded recent sample history per test. Only a successful
first attempt is a baseline sample. Failed, timed-out, and retry attempts are
excluded so a flake or an exceptional timeout does not permanently inflate the
plan.

The latest successful `main` profile is authoritative. Pull request runs may
publish diagnostic timing artifacts, but they do not replace the `main`
baseline. The planner applies these fallbacks in order:

1. Use the entry's p75 for an unchanged test.
2. Apply a conservative changed-file multiplier to a matching entry whose
   source file hash differs from the profile.
3. Use a project/cohort fallback estimate for a new or unknown test.
4. If no profile can be downloaded, use a deterministic count-based plan.

Unknown, changed, and stale counts are reported with every plan. Profile
entries that are no longer discovered are ignored by planning and eventually
removed when the bounded history is compacted.

## Shard planner contract

The planner discovers the same Playwright test catalog used by the selected
cohort. It treats a project/file pair as the initial indivisible work unit,
sums the selected project costs for a file when projects share it, sorts units
by estimated duration descending, and assigns each unit to the currently
lightest shard. Ties use stable project/file ordering and the lowest shard
number. This makes a plan reproducible for the same catalog and profile.

Every manifest must pass these checks before it is uploaded:

- every discovered project/file unit appears exactly once;
- no manifest contains a unit outside the selected cohort;
- the declared shard count is exactly fourteen or six as appropriate;
- the sum of assigned estimates matches the per-shard totals;
- unknown and changed units are listed rather than silently omitted.

If a single file remains a material outlier after weighted file planning, a
later planner revision may split that file into `file:line` test selections.
That refinement is data-driven and does not change the initial manifest
contract.

## CI lifecycle

1. The build/planning job builds the existing artifacts and obtains the latest
   successful `main` timing profile using the workflow token.
2. It discovers the catalog, creates normal and container manifests, validates
   coverage, and uploads the manifests as a run-scoped artifact.
3. Each matrix job downloads only its cohort manifest and invokes a wrapper
   with the explicit file selections. It preserves the existing blob reports,
   project selection, retry count, timeout, and `workers: 1` behavior.
4. The report job merges the blob reports and emits the new timing profile,
   retry summary, prediction-versus-actual summary, and plan health counters.
5. Only a successful `main` result is eligible to become the next baseline.

If the `main` artifact is unavailable, the workflow continues with the
validated count-based fallback and records that fallback in the report. A
missing or malformed manifest is a hard planning failure, not permission to
silently revert one matrix shard to ordinal sharding.

## Reliability contract

The following findings from PR #2471 become focused E2E repairs:

- scope the disabled-integrations link assertion to an exact, unique locator;
- derive multi-PR review expectations from the dynamically created repository
  instead of a fixed repository name;
- clear or isolate restored task-create drafts before testing prompt
  autocomplete;
- replace the fragile config-management marker wait with a state-aware
  synchronization point;
- wait for mobile autopilot sidebar hydration/WS convergence before asserting
  its state;
- make Docker storage-maintenance cleanup task-owned and use unique labels or
  scoped counts instead of deleting or counting unrelated managed containers.

The desktop and mobile multi-PR failures share the same fixture defect and
must be fixed together. Each repaired failure gets a regression assertion that
fails for the observed cause, not only a larger timeout.

Playwright output also includes a retry summary containing the test key,
attempt count, final status, error context, and links to retry artifacts. The
normal PR lane may retain its current retry gate while the summary is being
rolled out. A scheduled or explicitly diagnostic lane runs with
`failOnFlakyTests: true` so passed-after-retry results can be treated as a
first-class reliability signal.

## Performance experiments

The default remains one worker per shard until measurements prove that higher
concurrency is safe for the worker-scoped backend fixture. A controlled
experiment measures `workers: 2` on the known heavy shards and records wall
time, CPU/memory pressure, and retries before considering a default change.

A second experiment compares the unified normal matrix with a separately
balanced mobile matrix. It must use the same timing profile and report setup
overhead before changing the default matrix. Build/startup work is measured as
a separate budget, including package installation, runtime image startup, and
Playwright browser extraction; caching or pre-baked images are adopted only if
they improve wall time without increasing flakes.

## Success criteria

- Three consecutive successful `main` runs show materially smaller shard-tail
  skew than the current baseline. The initial target is no shard above 1.25x
  the cohort median unless one indivisible test unit dominates it.
- A newly added test appears in the next generated catalog without editing a
  shard assignment file.
- A changed test file is marked warm and receives the conservative estimate
  until successful `main` samples replace it.
- Manifest validation proves complete, non-overlapping coverage for both
  cohorts on every run.
- The seven observed retry groups either have deterministic fixes or remain
  visible in the retry summary with actionable evidence.
- The timing profile can be unavailable without blocking a correct,
  deterministic fallback plan.

## Out of scope

- changing product behavior or the worker-scoped backend isolation model;
- making `workers: 2` the default before the experiment passes;
- committing per-shard test lists that require manual maintenance;
- moving timing data to an external database;
- globally deleting Docker resources to make a test pass;
- masking flakes by increasing timeouts or retries.

## Implementation status

Implemented in the CI workflow, E2E tooling, scoped Docker fixtures, and
focused reliability tests. The generated manifests, rolling profile, retry
diagnostics, and fail-closed runner are covered by unit/type checks and local
targeted E2E runs. The three-successful-main-run balance target remains an
operational check after merge.
