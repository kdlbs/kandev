---
spec: ../../specs/platform/requirements/e2e-duration-aware-sharding.md
created: 2026-08-10
status: in_progress
---

# Implementation Plan: Duration-aware E2E sharding and CI reliability

## Overview

Replace ordinal E2E sharding with generated, duration-aware manifests while
keeping the current Playwright project boundaries and one worker per shard.
Build the timing profile from successful `main` runs, keep the generated
manifests ephemeral, and retain a deterministic count-based fallback. In the
same rollout, repair the seven retry groups identified during PR #2471,
publish passed-after-retry evidence, isolate container state, and run measured
experiments for worker concurrency, mobile project layout, and CI setup cost.

The baseline to compare against is the investigated run: normal shard 2/14
completed in 17m57 while shard 11/14 completed in 3m52; container shard 1/6
completed in 9m59 while the lightest shards took about 3m. Build time was
6m18, including roughly 4m48 for the backend and E2E web build. The plan must
report whether improvements come from balancing test work or from reducing
fixed setup overhead.

No production application behavior is changed. This is a CI/tooling and E2E
fixture package.

## Timing profile and catalog

Create a small, unit-tested timing module under `apps/web/e2e/scripts/`.

- Parse the merged Playwright blob results and normalize each result to the
  stable project/file/full-title key.
- Accept only first-attempt passing results as baseline samples. Exclude
  failures, timeouts, and retry attempts.
- Merge the current eligible samples into the previous `main` profile and
  retain a bounded recent history per key. Compute p50 and p75 values and
  record the source commit, source file hash, run ID, and timestamps.
- Discover the current project/file catalog for the normal and container
  cohorts. Keep the test matching rules in
  `apps/web/e2e/playwright.config.ts` authoritative rather than duplicating
  project selection in CI.
- Classify profile entries as unchanged, warm (source hash changed), unknown,
  or stale. Apply the changed-file multiplier and project/cohort fallback
  values from the spec, and expose the classification in planner output.

Likely files:

- `apps/web/e2e/scripts/e2e-timings.ts`
- `apps/web/e2e/scripts/e2e-timings.test.ts`
- `apps/web/e2e/scripts/plan-shards.ts` (shared catalog types may live here)
- `apps/web/e2e/scripts/plan-shards.test.ts`

## Weighted shard planner

Implement deterministic longest-processing-time bin packing for each cohort.
The planner should produce a versioned JSON manifest with:

- cohort and selected projects;
- exact shard count;
- profile source and fallback mode;
- per-shard explicit project/file selections;
- predicted seconds, unknown count, warm count, and unit count;
- the full catalog checksum and validation result.

Use project/file pairs as the first indivisible unit. If a file is selected by
more than one project, account for each project result in its estimate while
preserving a single unambiguous invocation. Sort by descending estimated cost
and use stable path/project ordering for ties. Validate exact coverage and no
overlap before the manifest can be uploaded.

Keep a seam for a future `file:line` unit. Add a planner diagnostic that
identifies files which dominate their assigned shard so test-level splitting
can be enabled only when measurements justify its complexity.

## CI lifecycle and runner integration

Update `.github/workflows/e2e-tests.yml` so the build/planning job:

1. resolves the latest successful `main` timing-profile artifact with
   `actions: read` permission;
2. falls back to an empty/count-based profile when that artifact is absent;
3. runs catalog discovery and creates `normal/1.json` through
   `normal/14.json` and `containers/1.json` through `containers/6.json`
   manifests;
4. validates and uploads those manifests as a run-scoped artifact.

Update both matrix jobs to download their cohort manifest and invoke a shared
runner. The runner must preserve the existing project filters, retries,
timeouts, blob reporter, output directories, `KANDEV_E2E_WS_ASSERT`, and
`workers: 1`. It must fail clearly for a missing shard manifest and must not
silently add ordinal `--shard` selection.

Update the report job to retain the existing merged report and upload:

- the merged timing profile candidate;
- retry/flakiness summary;
- predicted-versus-actual shard timing summary;
- plan validation and fallback counters.

Only successful `main` report artifacts may be consumed as the next baseline.
Document the artifact names, retention expectations, fallback behavior, and
local dry-run commands in `apps/web/e2e/README.md` and keep workflow comments
aligned with the new ownership model.

## E2E reliability repairs

Implement the observed repairs as focused regression changes:

- exact and scoped disabled-integration navigation locator;
- multi-PR review helper and desktop/mobile expectations based on the seeded
  repository name;
- task-create prompt autocomplete setup that clears or explicitly verifies
  restored session-storage drafts;
- config-management synchronization tied to the actual update completion
  state rather than a brittle marker-only wait;
- mobile autopilot assertion that waits for the sidebar state to converge after
  the API/WS transition;
- Docker storage-maintenance setup and assertions scoped to resources owned by
  the test, with unique labels and no global managed-container deletion.

Preserve the existing worker-scoped backend isolation. If an application
helper must change to expose a stable readiness signal, add the smallest
helper contract and cover its failure path.

## Retry observability and policy

Add a parser/summary path beside the timing collector that groups Playwright
results by stable test key and records attempts, final status, error category,
and links to trace/screenshot/video artifacts when present. Include counts for
passed-first-attempt, passed-after-retry, failed, timeout, and skipped.

Keep the current PR retry behavior during initial rollout, but make retry
groups visible in the job summary and artifact. Add a scheduled or explicitly
diagnostic workflow mode with `failOnFlakyTests: true`; it should demonstrate
that the suite can enforce a no-flake policy without changing the normal PR
gate as part of this package.

## Controlled performance experiments

After the manifest path is stable, add reproducible experiment commands and a
record format rather than changing defaults immediately.

- Run `workers: 2` on the known heavy normal shards and compare wall time,
  CPU/memory pressure, backend isolation, and retries against `workers: 1`.
- Compare the unified normal matrix with a separate mobile matrix using the
  same profile. Include browser/backend setup cost before judging the split.
- Measure the fixed costs observed in the run: build, pnpm install, runtime
  image startup, and `/ms-playwright` extraction. Benchmark caching or
  pre-baked images only after the test-work balance is visible.

The default remains `workers: 1`, the current project matrix, and the existing
setup path until an experiment records a repeatable improvement without a
flake or resource regression.

## Tests

### Unit tests

- timing result parsing, stable identity, retry exclusion, quantiles, bounded
  merge history, file-hash warm classification, and fallback selection;
- planner bin packing, stable ties, empty/unknown catalogs, project/file
  aggregation, changed-file multiplier, exact coverage, overlap rejection,
  and manifest schema validation;
- retry grouping and artifact-link extraction;
- container ownership label construction and scoped cleanup predicates where
  those helpers are extracted from the E2E fixtures.

Run the focused tooling tests with:

```sh
cd apps
pnpm --filter @kandev/web test -- e2e/scripts/e2e-timings.test.ts e2e/scripts/plan-shards.test.ts
```

### E2E and CI checks

Run each repaired group directly against the existing config and retain the
normal retry count while debugging. The container test requires a Docker
daemon and the container profile:

```sh
cd apps/web
pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts e2e/tests/review/review-multi-pr.spec.ts e2e/tests/task/task-create-prompt-autocomplete.spec.ts
pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium e2e/tests/settings/config-management.spec.ts
pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome e2e/tests/review/mobile-review-multi-pr.spec.ts e2e/tests/task/mobile-autopilot-mode.spec.ts
KANDEV_E2E_CONTAINERS=1 pnpm exec playwright test --config e2e/playwright.config.ts --project=containers e2e/tests/docker/storage-maintenance.spec.ts
```

The workflow validation must prove that both cohorts execute every catalog
unit exactly once from generated manifests and that report artifacts include
the profile and retry summary. The three-run success criterion in the spec is
an operational rollout check, not a local unit-test substitute.

## Implementation Waves And Parallel Candidates

Wave 1 — independent evidence and fixture repairs:

- [x] [Task 01: Build the rolling timing profile](task-01-timing-profile.md)
- [x] [Task 04: Repair deterministic E2E fixtures](task-04-deterministic-e2e-fixtures.md)
- [x] [Task 05: Repair asynchronous E2E synchronization](task-05-async-e2e-synchronization.md)
- [x] [Task 06: Isolate container E2E state](task-06-container-isolation.md)

These tasks touch separate primary areas and are parallel candidates, but the
primary implementation conversation should execute them sequentially unless
the user explicitly delegates work.

Wave 2 — planner:

- [x] [Task 02: Generate duration-aware shard manifests](task-02-shard-planner.md)

Depends on Task 01.

Wave 3 — workflow integration:

- [x] [Task 03: Wire manifests into the CI workflow](task-03-ci-manifest-lifecycle.md)

Depends on Tasks 01 and 02.

Wave 4 — reliability signal and measurement:

- [x] [Task 07: Surface retry and timing evidence](task-07-retry-observability.md)
- [x] [Task 08: Run concurrency and setup experiments](task-08-performance-experiments.md)

Both depend on Task 03. Task 08 also consumes the repaired container behavior
from Task 06.

Wave 5 — rollout documentation:

- [x] [Task 09: Document rollout and operating checks](task-09-rollout-documentation.md)

Depends on Tasks 03, 06, and 07 so the README describes the final contracts.

## Verification Results

Implemented and verified in the working tree:

- Timing, planner, retry-summary, runner, and helper tests: 5 files, 26 tests
  passed.
- Backend Docker-scope tests: `go test ./internal/agent/runtime/lifecycle
./internal/backendapp` passed, 1,456 tests across 2 packages.
- Web typecheck and focused ESLint passed; Prettier check and `git diff --check`
  passed.
- `make build-backend build-web-e2e` and
  `make -C apps/backend e2e-plugin-package` passed.
- Planner dry-run generated and validated 14 normal plus 6 container manifests;
  the live catalog contained 599 unknown normal units and 27 unknown container
  units. The heaviest container file was reported as a 240-second dominant
  indivisible unit (63.2% of its shard). The missing-manifest runner check
  failed closed as designed.
- Reliability groups passed: normal Chromium 5/5, config-management 21/21,
  mobile 3/3, containers 1/1. Two concurrent container runs also passed 1/1
  each, and no managed containers remained afterward.
- Task 08 completed its controlled local experiment on the rebuilt merged
  checkout. The heavy normal shard passed 191/191 three times with workers=1
  (median 458.237s, CV 0.014%). A workers=2 run was 268.715s but produced two
  flaky tests and exited 1 under `E2E_FAIL_ON_FLAKY=1`, so workers=2 is rejected.
- The unified-vs-mobile split simulation predicted 611.1s for the current
  unified shape versus 653.7s for the best separate allocation before setup
  overhead. Normal 14/container 6 remains the measured shape. CI setup was
  separately recorded: 13s install, 4m44s backend/web build, 33s browser
  extraction; cold-cache measurements remain a follow-up.
- The latest PR run [31469837346](https://github.com/kdlbs/kandev/actions/runs/31469837346)
  passed 42 checks at merged head `d150554a5` and reduced full workflow wall
  time from 28m30s on main run [31423650879](https://github.com/kdlbs/kandev/actions/runs/31423650879)
  to 22m39s (20.5%). Both runs used count fallback because the latest main
  run has no timing-profile artifact; the first post-merge main run remains
  the profile bootstrap check.

The three-successful-main-run balance criterion and cold-cache setup comparison
remain operational rollout checks after merge, not reasons to change the
shipping defaults now.

## Open Questions

No blocking questions remain for implementation. The initial defaults are a
bounded recent history, p75 planning, a conservative changed-file multiplier,
file-level units, and `workers: 1`. Task 01 may tune the history window and
Task 02 may tune the fallback estimate using real profile distributions, but
those changes must remain explicit in the profile metadata and tests.
