---
id: "08-performance-experiments"
title: "Run concurrency and setup experiments"
status: in_progress
wave: 4
depends_on:
  - "03-ci-manifest-lifecycle"
  - "06-container-isolation"
plan: "plan.md"
spec: "../../specs/e2e-duration-aware-sharding/spec.md"
---

# Task 08: Run concurrency and setup experiments

## Acceptance

- A repeatable comparison records `workers: 1` versus `workers: 2` on the
  known heavy normal shards, including wall time, CPU/memory pressure, backend
  isolation failures, and retry counts; the default remains one worker unless
  evidence supports a later change.
- A repeatable comparison measures the unified normal matrix against a
  separately balanced mobile matrix, including project/setup overhead and
  actual shard tail; no matrix split is enabled without the report.
- Build, pnpm install, runtime-image startup, and Playwright browser extraction
  are measured separately, and any cache or pre-baked-image change is kept
  behind evidence of lower wall time without a reliability regression.

## Verification

```sh
cd apps/web
E2E_SHARD=2 /usr/bin/time -v bash e2e/scripts/run-planned-shard.sh \
  e2e/manifests/normal/2.json -- --workers=2
E2E_SHARD=2 /usr/bin/time -v bash e2e/scripts/run-planned-shard.sh \
  e2e/manifests/normal/2.json -- --workers=1
```

Generate `e2e/manifests/normal/2.json` with the Task 03 planner first. Run the
paired commands on the same runner class and record results in the experiment
report. The runner forwards the worker override after `--`, so both commands
execute the exact same duration-aware file selection. Do not compare unrelated
ordinal assignments.

## Files likely touched

- `.github/workflows/e2e-tests.yml` (diagnostic/manual experiment hooks only)
- `apps/web/e2e/README.md` (experiment procedure and result format)
- CI image/cache configuration if a measured setup optimization is approved

## Dependencies

Depends on Task 03 for reproducible manifests and Task 06 for safe concurrent
container behavior.

## Parallelism

Sequential after the default manifest workflow is stable. Experiments must be
isolated from the blocking PR lane.

## Inputs

- Current workers-one invariant and worker-scoped backend fixture.
- Heavy normal and container shard timing evidence from the investigation.
- Build and setup timings from the PR #2471 workflow run.

## Output contract

Report paired measurements, runner/environment details, flake/resource
observations, recommendation, and explicit statement of which defaults remain
unchanged.

## Implementation result

The README now contains the controlled experiment procedure and result format.
The runner now supports forwarding worker overrides for paired manifest-based
measurements. A local four-test probe passed with workers=1 in 35.5s and
workers=2 in 29.2s; both had zero retries and backend errors. This single probe
is diagnostic only, so the CI default remains `workers: 1` and the three-repeat
rollout check is still required before changing it.
