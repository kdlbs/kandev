---
spec: ../../specs/platform/requirements/e2e-duration-aware-sharding.md
created: 2026-08-30
status: in_progress
---
# Implementation Plan: E2E CI setup and flake efficiency

## Objective

Reduce the E2E critical path after the duration-aware planner has already
removed most test-work imbalance. This package reuses verified Playwright
browsers on host-runner container jobs and removes one recurring false flake
from the review-diff regression test. It preserves the fourteen normal shards,
the six container shards, one worker per shard, existing retries, and the
required aggregate gate.

## Evidence and diagnosis

The recent merged E2E stabilization changes are relevant inputs:

- [PR #3150](https://github.com/kdlbs/kandev/pull/3150) added a causal layout
  wait for the PR status badge hover. [PR #3151](https://github.com/kdlbs/kandev/pull/3151)
  then narrowed an automation-trigger notification assertion. Both are recent
  examples of synchronization fixes that should be preferred over longer
  waits.
- [PR #3133](https://github.com/kdlbs/kandev/pull/3133) removed shared-worker
  cleanup races and reinforced task-owned test state.
- A fair successful-main run, [run 33250014029](https://github.com/kdlbs/kandev/actions/runs/33250014029),
  had a roughly 16m28s normal-shard tail after the build completed. Its six
  container jobs repeated browser provisioning.
- In [run 33308951784](https://github.com/kdlbs/kandev/actions/runs/33308951784),
  the predicted-versus-actual normal shards stayed within the existing 1.25x
  cohort-median target and the container cohort was also balanced. The
  workflow's 75m44s end-to-end wall time was dominated by runner queue delay,
  so queue capacity is tracked separately from code-level setup savings.
- Container job logs show about 54 seconds to pull/copy Playwright browsers
  into `/tmp/ms-playwright` on each host runner.
- The same latest-main run recorded one passed-after-retry result in
  `tests/review/review-file-status.spec.ts`. Its final assertion counted two
  unrelated `Loading diff...` placeholders in lazy sections while checking a
  moved file that had already rendered. The test has also appeared in earlier
  flake history, so this is a reproducible synchronization defect rather than
  a reason to increase retries.

## Scope

### In scope

- Best-effort browser-cache restore and save in the existing host-runner
  container cohort, keyed to the exact checked-in browser source.
- A browser smoke verification that decides whether the restored path is safe;
  the current pinned-image extraction remains the fallback.
- Step-summary diagnostics for cache state, browser verification, and setup
  duration so future runs can measure the gain.
- A selection-scoped readiness assertion in the review-file-status E2E test.
- Workflow contract coverage, focused E2E coverage, and rollout evidence.

### Out of scope

- Changing the duration-aware planner, timing-profile schema, 14/6 matrix, or
  `workers: 1` default.
- Splitting mobile Chrome into a separate matrix.
- Increasing retries or timeouts, adding fixed sleeps, or weakening the E2E
  gate.
- Replacing the runtime image, moving browsers to an external service, or
  changing application behavior.
- Treating GitHub runner queue time as a test or sharding defect.

## Technical approach

The container jobs resolve the `runtime-latest` convenience tag to an immutable
sha256 digest, then restore `/tmp/ms-playwright` before the digest-pinned image
pull and copy step. The cache key includes the runner OS, browser source,
resolved image digest, and a run-specific primary-key suffix with a stable
digest-scoped restore prefix. A verified exact or prefix hit skips the
pull/copy step. A miss, stale, cache-service failure, or failed Chromium
verification runs the existing digest-pinned extraction path and can populate
the cache under a new primary key after successful setup. Cache actions are
best-effort and cannot turn a healthy fallback into a failed E2E job.

The review test anchors the final moved-file assertions to the selected file's
existing review header/section boundary. Other lazy sections may still show a
loading marker and do not affect the selected file's readiness.

## Work orders

### Wave 1

- [ ] [task-01-cache-host-runner-browsers](task-01-cache-host-runner-browsers.md)
- [x] [task-02-scope-review-diff-readiness](task-02-scope-review-diff-readiness.md)

The slices are independently verifiable, but execution remains sequential in
the primary session so workflow and test changes can be reviewed together.

## Rollout and success criteria

1. Run the workflow contract and action-pinning checks locally.
2. Run the focused review E2E test repeatedly with retries disabled.
3. Merge the workflow change and compare at least three successful `main`
   runs with the preceding setup and execution measurements.
4. Confirm warm container jobs report a verified cache hit and omit browser
   extraction. Confirm a forced key miss or unavailable cache uses the pinned
   image fallback and still passes.
5. Retain the change only if the container setup component decreases without
   new browser-verification failures, retries, or test failures. Continue to
   report queue delay separately.

The existing shard-tail target remains the acceptance threshold for test-work
balance. A cache change is not credited with a sharding improvement, and a
shorter queued workflow is not credited as a test-speed improvement.

## Verification strategy

The work orders own focused verification. The implementation handoff should
also run the repository specification lint after changing the design package:

```bash
python3 scripts/lint-spec-files.py --all
git diff --check
```

Remote rollout evidence belongs in this plan after the branch workflow runs.
Until then, the cache hit rate and setup timings remain open operational
measurements.

## Implementation results

- The container workflow now resolves an immutable runtime image digest,
  restores and saves a Playwright browser cache keyed by runner OS, browser
  source, digest, and a run-specific suffix, and uses a stable digest-scoped
  restore prefix. Cache verification gates the digest-pinned GHCR fallback,
  and setup state is written to the job summary. Cache errors remain
  non-blocking.
- The review-file-status regression now scopes the moved-file assertions to its
  selected diff section. A 30-repeat host run with retries disabled passed all
  30 repetitions.
- PR run [#3163](https://github.com/kdlbs/kandev/pull/3163) passed all 20 E2E
  shards and the full required check set. A container shard restored a
  digest-scoped prefix cache, verified Chromium, skipped image extraction, and
  reported about 9 seconds for browser setup. The preceding PR run exposed a
  transient GHCR metadata lookup failure; the workflow now retries that lookup
  three times before failing safely.
- Post-merge comparison across three successful `main` runs and a forced-miss
  fallback measurement remain open rollout work.

## Risks

- GitHub Actions cache transfer can cost enough time to erase the browser-copy
  saving. Step summaries must report restore and verification duration before
  declaring success.
- Concurrent first-run cache saves can race. The fallback remains valid and
  each run has a distinct primary key; the restore prefix selects the newest
  compatible successful entry.
- The runtime image may change without a browser-version change. Including the
  resolved image digest in the key is a conservative invalidation rule and
  binds fallback extraction to the same image identity.
- Hosted-runner queue capacity can dominate total workflow wall time. The
  rollout must compare job execution intervals and queue intervals separately.
