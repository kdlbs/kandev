---
id: "13-e2e-runner-processes"
title: "Reduce E2E execution within existing runners"
status: in_progress
wave: 9
depends_on: ["10-profile-e2e-fixture-costs"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-005
  - REQ-PLATFORM-E2E-DURATION-AWARE-SHARDING-001
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-005.1
  - AC-PLATFORM-CI-PERFORMANCE-005.5
  - AC-PLATFORM-E2E-DURATION-AWARE-SHARDING-001.1
system_design:
  - ../../specs/platform/system-design/e2e-duration-aware-sharding.md
---

# Task 13: Reduce E2E execution within existing runners

## Summary

Measure and, only if useful, enable bounded independent Playwright processes
inside existing normal E2E jobs. The maintainer's target is a 50% reduction in
the longest normal E2E test step from 2,159 seconds to at most 1,079.5 seconds.

## Scope and boundaries

Own planned-shard execution, stable fixture port slots, resource selection,
complete reports and normal-job wiring. Preserve every selected assertion,
fourteen normal jobs, six container jobs, one worker per process, retries,
timeouts, build identity checks and required aggregate behavior. Container
execution remains serial. Do not change provider allocation or add capacity.

## Acceptance

1. A same-code, same-selection comparison retains every test identity and
   records failures, retries, elapsed time, process count and memory evidence.
   Local evidence supports candidate selection; complete hosted runs decide
   the full-suite target.
2. Partition and supervisor tests prove exact selection, whole-file grouping,
   stable ports across worker replacement, disjoint reports, joining all
   children and failure propagation. A missing report fails closed.
3. Existing timing/merge consumers accept all reports and exactly one metadata
   record per original job. Full CI passes; its longest E2E step, summed step
   time and workflow elapsed time are reported separately.

## Verification

Run from the repository root, with no overlapping local E2E suites. Run the
managed resource guard unchanged; a denied process count is not bypassed.

```bash
(cd apps/web && pnpm exec vitest run e2e/helpers/backend-ports.test.ts e2e/scripts/planned-subshards.test.ts e2e/scripts/planned-processes.test.ts e2e/scripts/planned-processes-lifecycle.test.ts e2e/scripts/run-planned-shard.test.ts e2e/scripts/resource-guard.test.ts e2e/scripts/e2e-timings.test.ts)
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning.py
(cd apps/web && pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the same queue, transcript and workflow file selection sequentially with
one, two and (only if the guard permits it) three local shards on four CPUs.
Separately exercise the planned runner with blob reports, multiple projects
and real worker replacement. Validate merged test identity and parent metadata
before enabling the workflow. Preserve failed attempts rather than selecting
only successful samples.

## Files and dependencies

- `apps/web/e2e/scripts/run-planned-shard.*`
- `apps/web/e2e/scripts/planned-subshards.*` and `planned-processes.*`
- `apps/web/e2e/fixtures/backend.ts` and `helpers/backend-ports.*`
- `.github/workflows/e2e-tests.yml` and its existing contract tests
- Existing E2E resource guard, file-duration manifest and report consumers

## Results

Candidate investigation is in progress. The initial 86-case four-CPU serial
attempt passed 84 cases and failed Quick Chat restored-session submission and
local-rebase comparison focus. It is retained as failed evidence, not a passing
speed baseline. No parallel phase started. A fresh 42-case queue/transcript/
workflow comparison uses consistent stable-slot port allocation on both sides.
The second serial attempt passed 41/42; the workflow reuse/complete policy
case timed out while backend logs recorded persistence probe failures and
HTTP 503 responses. Observed host-available memory fell to 790,315,008 bytes
while the measured test tree peaked at 2,536,239,104 bytes summed RSS. This
shared-host attempt is not a controlled speed baseline or proof of a new
parallelism regression; its parallel phases never started.

The actual planned-runner probe passed all 14 desktop/mobile cases from four
files with two one-worker processes, zero retries/skips, stable ports across
project replacement and successful blob merging. Exact catalog counts and
unique identities matched. Sixty focused unit tests passed, including real
Unix process cancellation; removing signal handlers made that regression fail
and restoring them made it pass. Workflow contracts, changed-file lint and
web typecheck passed. The draft workflow requests three normal-job processes
subject to the unchanged resource guard and CPU cap. This is a hosted trial,
not an adopted or measured 50% improvement.

The same 14-case probe also passed with one and three processes on the same
four-CPU affinity, with identical test identities and zero retries/skips.
Parent test execution was 52.552 seconds serial and 32.822 seconds with three
processes. This single small sample supports the hosted trial, not the full
suite target. Blob merging passed in both modes. The resource guard admitted
three processes; peak RSS was not separately sampled for this short probe.

Full hosted E2E run 38416314881 at f68a9c867d74eee3406460e610577aa468acad5c
passed before concurrency changes: longest normal step 2,178 seconds, summed
normal steps 26,058 seconds. The 50% target remains open.
