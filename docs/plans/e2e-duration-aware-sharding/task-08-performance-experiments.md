---
id: "08-performance-experiments"
title: "Run concurrency and setup experiments"
status: completed
wave: 4
depends_on:
  - "03-ci-manifest-lifecycle"
  - "06-container-isolation"
plan: "plan.md"
spec: "../../specs/platform/requirements/e2e-duration-aware-sharding.md"
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

## Experiment report (2026-08-11)

The README contains the controlled experiment procedure and result format, and
the runner forwards worker overrides after the manifest separator. The local
experiment used the post-merge checkout at `d150554a5`, a rebuilt backend/web
artifact, and the heaviest predicted normal shard (`normal/10.json`, 44 files,
191 tests). The runner image does not provide GNU `/usr/bin/time`, so Bash
`time -p` supplied wall/user/system time and a process-group sampler supplied
peak RSS and CPU for the monitored runs.

### Environment

- Node `v24.16.0`; pnpm `9.15.9`.
- AMD Ryzen 5 7640HS, 6 cores / 12 threads; 10 online CPUs reported to the
  process.
- 18 GiB RAM, 15 GiB available at measurement start, 8 GiB swap.
- Docker `26.1.5`, Linux `amd64`.
- Local build artifacts were rebuilt at the measured commit before E2E runs.

### CI comparison

The latest successful main baseline, [run 31423650879](https://github.com/kdlbs/kandev/actions/runs/31423650879), took 28m30s from workflow creation to completion. The post-conflict PR run, [run 31469837346](https://github.com/kdlbs/kandev/actions/runs/31469837346), took 22m39s and passed all 42 checks: a 5m51s (20.5%) full-workflow reduction.

The PR run's normal shard cohort ran from 07:45:35Z to the slowest job finish at 08:00:41Z (15m06s); the slowest normal job was 12m47s. The baseline's slowest normal job was 17m41s. The container tail improved from 9m26s to 5m14s. The PR retry summary recorded 2 tests that passed after one retry, 0 final failures, and 0 timeouts.

Both the PR run and the latest successful main run used the deterministic
count-fallback path: the latest main run, [31469080336](https://github.com/kdlbs/kandev/actions/runs/31469080336), has no `e2e-timing-profile` artifact. The first successful main run after this change lands will bootstrap the rolling profile; until then, the planner is implemented but the CI timing-profile path is not yet exercised.

### Worker experiment

| workers | repetitions | wall time | result | sampled peak RSS | decision |
| ---: | ---: | --- | --- | ---: | --- |
| 1 | 3 | 458.110s / 458.237s / 458.252s; median 458.237s; CV 0.014% | 191/191 passed, no retries | 874–948 MiB on 2 monitored runs | keep default |
| 2 | 1 | 268.715s | 189 passed; 2 flaky tests passed after retry; exit 1 with `E2E_FAIL_ON_FLAKY=1` | 1.32 GiB | reject |

The two-worker run failed the reliability gate with a passthrough-terminal
assertion and a `fetch failed` error. It is faster by 41.4%, but it does not
meet the zero-retry/zero-isolation-error requirement. The checked-in default
remains `workers: 1`; no candidate worker change was applied.

### Matrix, shard-shape, and setup measurements

- The current unified 14-shard profile simulation predicts a 611.1s maximum
  shard. The best separate matrix allocation was 11 Chromium + 3 mobile
  shards at 653.7s (+7.0%) before extra setup, backend, and report overhead.
  The split was not executed because it failed the 10% improvement threshold.
- Normal shard simulation produced maxima of 855.3s at 10 shards, 611.1s at
  14, and 534.8s at 16. Container maxima were 210.9s at 4, 154.8s at 6, and
  remained 154.8s at 8 because `docker-launch.spec.ts` is the dominant
  indivisible unit. Keep 14 normal and 6 container shards.
- On the CI runner, dependency install took 13s, backend/web build took
  4m44s, and the complete Build job took 6m18s. Container setup extracted
  Playwright browsers in 33s. Warm local measurements were pnpm install
  0.93s, browser install verification 0.57s, and a local runtime-image
  startup probe 0.45s. Cold cache misses and image pulls still need a runner
  measurement before changing setup or image strategy.

### Decisions

- **Adopt now:** keep the generated-manifest workflow, 14/6 matrix, unified
  normal/mobile projects, and `workers: 1`; retain retry and shard diagnostics.
- **Needs CI confirmation:** consume the first post-merge main timing profile,
  measure cold setup/cache behavior, and repeat any future worker candidate on
  the CI runner class.
- **Reject:** workers=2 for the current shared-backend fixture, and a separate
  mobile matrix based on the profile simulation.
