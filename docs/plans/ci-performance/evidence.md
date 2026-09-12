---
id: "ci-performance-evidence"
title: "CI performance evidence"
status: done
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-004
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
---

# CI performance evidence

This report records the read-only sample collected on 2026-09-12. Every run
uses attempt 1 unless stated otherwise. Queue time is the interval before the
job's `started_at`; execution time is the job interval from `started_at` to
`completed_at`. Workflow elapsed time is the run interval from
`run_started_at` to `updated_at`. These intervals answer different questions
and are not added together.

The sample is diagnostic. It does not establish a repository-wide p90, and it
does not include a hosted run of the changed frontend workflow. No workflow was
dispatched and no repository variable was changed during this task.

## Run and job inventory

| Area | Run and source | Run interval | Representative job | Queue / execution evidence |
| --- | --- | ---: | --- | --- |
| Claude review approval | [34355720657](https://github.com/kdlbs/kandev/actions/runs/34355720657), PR event, `b912428c248643c69e3effbd1e5ce3a624971740` | 7h 33m 04s | No jobs or check runs | Approval expired. GitHub created no runner job, so execution is unknown and must not be reported as 7h33m of Claude work. |
| Claude review execution | [34025129928](https://github.com/kdlbs/kandev/actions/runs/34025129928), `pull_request_target`, `c0264a50170693cd3ec2f103b5df9c80710bfb6c` | 1h 26m 44s | `claude-review-fork`, job `101464631037` | Queue 1h 20m 22s; execution 6m 21s. The allowlist job completed at 10:55:37Z and the review job started at 10:56:51Z. |
| Frontend unit baseline | [34689871445](https://github.com/kdlbs/kandev/actions/runs/34689871445), PR event, `a97538b0ba003f44b2586be9e2df7a59389edc1a` | 34m 09s | `Run Frontend Lint, Tests, and Build`, job `103543614850` | Job execution 29m 54s. The `Run tests` step took about 25.5m. |
| Cargo Audit | [34027695660](https://github.com/kdlbs/kandev/actions/runs/34027695660), push, `d796cf131deee94c24e78a6254aa19d4550ccabd` | 46m 37s | `Cargo Audit`, job `101471499916` | Queue 43m 23s; execution 3m 13s. The cargo-audit installation was about 2.8m. |
| E2E | [34687600985](https://github.com/kdlbs/kandev/actions/runs/34687600985), PR event, `fc101191cf4c7ac969dccc335b39450a98ae4a40` | 41m 21s | `E2E Shard 8/14`, job `103538339673` | Normal shards took about 15.6m to 20.4m including job setup. Some shards waited up to about 10.1m after job creation. |
| Backend | [34692849510](https://github.com/kdlbs/kandev/actions/runs/34692849510), PR event, `29139024786d4d3b8154c65cc7a4f472627d47e3` | 24m 26s | `Backend (windows)`, job `103551104350` | Windows job execution 21m 43s. The broad Windows-sensitive race-test step took about 13.2m. Linux test jobs took about 9.1m and 9.8m. |

The Claude approval-expiration run has no job timestamps. The absence is the
evidence: a workflow run's `run_started_at` is not proof that a runner started.
The Claude action therefore receives a runner execution budget in Task 01,
while approval and queue waits remain separate metrics.

## Frontend baseline and setup evidence

The frontend job log for job `103543614850` reported:

- 1,986 test files passed.
- 17,033 tests passed and 4 skipped.
- Vitest wall duration 1,526.89s.
- Vitest phase totals of setup 1,862.11s, import 1,252.60s, environment
  706.35s, transform 585.30s, and test bodies 385.32s.

Phase totals overlap across workers. They are attribution data, not a sum that
can replace the 1,526.89s wall duration.

The same log ended with the cache action warning:

```text
Path Validation Error: Path(s) specified in the action for caching do(es) not exist, hence no cache is being saved.
```

The changed workflow now resolves `pnpm store path --silent` inside each
container job, includes the runner architecture and pnpm version in the key,
and keeps `pnpm install --frozen-lockfile` after the best-effort cache step.
Local resolution in this workspace returned
`/root/.local/share/pnpm/store/v3` with pnpm `9.15.9`. A hosted save followed by
a compatible restore is still an operational check and cannot be proved by
the local run.

The setup candidate completed the local selection and environment checks. Its
final full report is retained outside the repository at
`/tmp/kandev-ci-unit-candidate-final.json` during this session. A hosted
three-run comparison remains required before crediting a speed improvement.

## E2E fixture and retry evidence

Run `34687600985`, attempt 1, published the `e2e-timing-diagnostics` artifact
and the related retry and timing profiles. The artifact source identifies run
`34687600985` and SHA
`9b0df86afdd1e8bc7d444c716626ed4694ea8747` on branch `3587/merge`.

The retry summary contained 2,933 tests passed on the first attempt, 5 passed
after retry, 0 failed, 0 timed out, and 38 skipped. The five flaky tests give a
rate of 1.7 per thousand executed tests. The retry summary has no unexpected
failures, so the retry data is suitable for identifying follow-up targets while
the run remains a successful result.

The measured shard profile identified these tails:

| Cohort | Shard | Predicted | Actual |
| --- | ---: | ---: | ---: |
| normal | 8/14 | 1,085.23s | 1,108.98s |
| normal | 6/14 | 1,085.09s | 1,106.70s |
| normal | 3/14 | 1,084.52s | 1,065.22s |
| containers | 6/6 | 207.54s | 479.00s |
| containers | 3/6 | 201.92s | 414.84s |

The slowest repeated test bodies in the timing profile were concentrated in
`tests/pr/pr-detection.spec.ts`,
`tests/task/autopilot-mode.spec.ts`, and
`tests/session/long-prepare-panels.spec.ts`, with individual samples near
48 to 61 seconds. The timing profile alone cannot distinguish fixture setup,
server work, or test-body waits inside those samples.

The five retried tests were:

- `tests/office/org-chart.spec.ts`
- `tests/task/file-tree-sorting.spec.ts`
- `tests/task/parked-background-work.spec.ts`
- `tests/task/task-listing-view-preferences.spec.ts`
- `tests/pr/mobile-pr-ci-chip.spec.ts`

The next E2E investigation should correlate these test keys with fixture and
request traces before changing workers, retries, or waits. A reproduction can
start with the selected files under the Chromium project, using the normal
one-worker safety limit:

```bash
cd apps/web
pnpm e2e:run --project chromium --workers=1 \
  tests/pr/pr-detection.spec.ts \
  tests/task/autopilot-mode.spec.ts \
  tests/session/long-prepare-panels.spec.ts
```

## Windows backend evidence

The Windows log for job `103551104350` ran on `windows-latest` from 12:08:33Z
to 12:30:16Z. It recorded roughly 3m29s for `go build ./...`, 1m28s for
`go vet ./...`, and 13m10s for the broad race-test command covering:

```text
./internal/agentctl/server/process/...
./internal/agent/runtime/agentctl/launcher/...
./internal/common/ptyexec/...
./internal/backendapp/ownershiplock/...
./internal/system/storage/workspaces/...
```

The job saved the Go cache at 12:30:10Z with a Windows, Go 1.26.0 key. This
shows cache save activity but does not show that the cache transfer is the
main cost. The next measurement should split compile, vet, package race tests,
and cache transfer on repeated Windows attempts before changing package
ownership or shard boundaries.

Reproduction command from the captured job:

```bash
cd apps/backend
go test -race -v ./internal/agentctl/server/process/... \
  ./internal/agent/runtime/agentctl/launcher/... \
  ./internal/common/ptyexec/... \
  ./internal/backendapp/ownershiplock/... \
  ./internal/system/storage/workspaces/...
```

## Ranked follow-ups

1. **Frontend test setup and hosted sharding.** The frontend baseline has the
   largest measured test interval in this sample. First compare the local
   setup candidate and two-shard candidate with identical test identities.
   Retain the matrix only after three comparable hosted runs show at least a
   30% lower median frontend critical path and no more than 25% additional
   runner minutes. Current branch work supplies the selection, matrix, report,
   and gate contracts; hosted performance evidence is pending.
2. **pnpm cache save and restore.** The path-validation failure is confirmed.
   Verify a cold install, a later compatible restore, and a cache-service
   failure with a successful frozen install. Do not use a cache hit as a proxy
   for test speed until its step timing is recorded.
3. **Runner queue capacity.** Cargo Audit and the Claude review sample are
   dominated by queue or approval waits. Existing external-runner variables
   are currently `KANDEV_CI_EXTERNAL_ENABLED=false`,
   `KANDEV_CI_EXTERNAL_PERCENT=20`, with configured light and standard
   Ubicloud labels. The account quota versus provider capacity cause is
   unknown. Use the Task 06 pilot procedure before changing allocation.
4. **E2E fixtures and retries.** Profile the five flaky tests and the normal
   shard tails listed above. Preserve one worker per shard and the existing
   retry policy until fixture and request costs are attributable.
5. **Windows compilation and race tests.** Measure repeated Windows jobs and
   cache transfer before further backend sharding. The current data supports
   profiling, not a source-level optimization.

## Reproduction and provenance commands

The report was assembled with these read-only API commands. They preserve run
attempt identity and keep raw logs outside the repository:

```bash
gh api "repos/kdlbs/kandev/actions/runs/$CI_RUN_ID/attempts/$CI_RUN_ATTEMPT/jobs?per_page=100"
gh api "repos/kdlbs/kandev/actions/runs/$CI_RUN_ID/artifacts?per_page=100"
gh run download "$CI_RUN_ID" --repo kdlbs/kandev \
  --name e2e-timing-diagnostics --dir "$CI_ARTIFACT_DIR"
gh api "repos/kdlbs/kandev/actions/jobs/$CI_JOB_ID/logs" > /tmp/kandev-ci-job.log
```

Raw downloaded logs and artifacts are temporary investigation outputs. The
curated figures above are the retained evidence; the repository does not store
the raw downloads.
