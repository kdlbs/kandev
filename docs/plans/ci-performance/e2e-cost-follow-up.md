# E2E fixture cost follow-up

## Evidence and method

Source: [PR 4352 E2E attempt 1](https://github.com/kdlbs/kandev/actions/runs/37826061394).
Downloaded its 20 existing `blob-report-*` artifacts through read-only API calls.
All embedded build URLs identify that run. The shared fixture sources match
this checkout; `git diff 730abee48936b7b3c804a8f365526e27a36302ca HEAD` is empty
for `test-base.ts`, `backend.ts`, and `docker-test-base.ts`.

Playwright already records fixture setup and teardown steps. Those measurements
replace the proposed temporary instrumentation for this first attribution pass.
No new E2E run, source instrumentation, timeout change, or worker increase was
needed. This avoids adding work to the saturated fleet and avoids measurement
hooks that alter timing. The analysis script reads ZIP contents without
extracting executable files:

```bash
python3 docs/plans/ci-performance/experiments/profile-existing-e2e.py \
  /tmp/kandev-ci-implementation/blobs /tmp/e2e-fixture-profile.json
```

The committed `experiments/e2e-fixture-results-2026-10-08.json` contains the
aggregates, selected file results, and slow page-setup records. Raw artifacts
remain outside the repository. Retries remain included. Fixture durations
include child operations; do not sum a parent with its children. Accumulated
time across concurrent shards is not workflow elapsed time. Percentiles use
nearest rank; sparse worker observations do not estimate a fleet distribution.

## Measured boundaries

| Boundary | Observations | Median | p95 | Accumulated time |
| --- | ---: | ---: | ---: | ---: |
| `testPage` setup | 3,902 | 93 ms | 2,631 ms | 3,295.845 s |
| `testPage` teardown | 3,902 | 93 ms | 328 ms | 494.006 s |
| `integrationCleanup` setup | 3,779 | 102 ms | 136 ms | 389.322 s |
| `integrationCleanup` teardown | 3,779 | 66 ms | 92 ms | 258.444 s |
| Backend fixture startup | 82 | 790.5 ms | 28,992 ms | 459.280 s |
| Browser fixture startup | 76 | 76.5 ms | 154 ms | 8.208 s |
| Kind cluster setup | 4 | 138,979 ms | 174,743 ms | 585.839 s |

Only four backend/browser worker-cleanup observations appear in the blobs.
That is incomplete teardown coverage, so this report makes no total worker
shutdown-cost claim. Individual reset HTTP calls, Git fetches, and browser
context creation are not separately attributed by these fixture boundaries.

Seven page setups exceed 10 seconds, totaling 214.702 seconds. They occur in
inbox failure, command-palette archive, quick chat, coordinator proposals and
copilot, mobile branch policy, and session sidebar-order files. They do not
explain most accumulated page setup: the broad p95 tail deserves investigation.

Focused coverage in the same completed run:

- `chat/message-queue.spec.ts`: 20 observed page setups total 34.669 seconds;
  integration setup and teardown total 2.832 seconds.
- `session/session-idle-parking.spec.ts`: page setup takes 95 ms. Its long test
  duration is not explained by the page fixture.
- `session/mobile-transient-turn-runtime-continuity.spec.ts`: four page setups
  total 191 ms. Its retry/backoff coverage remains meaningful execution cost.
- `docker/docker-launch.spec.ts`: four observed page setups total 208 ms.
  This fixture count is not the file's full test count; API-only tests need no page.
- `kubernetes/kubernetes-docker-workloads.spec.ts`: no executed fixture evidence.
  The full-worker scenario is opt-in and was skipped in the baseline.

## Decisions and bounded follow-ups

1. **Retain integration isolation.** The entire observed integration setup and
   teardown sum is 10.8 runner-minutes, about 2% of the 546.4-minute workflow
   occupancy. That is an upper bound, not a promised saving. Serial configuration
   deletes and Git-origin restoration protect state reused by later tests.
   Do not remove them or run conflicting resets concurrently.

2. **Profile the page-reset tail before changing it.** Owners are
   `apps/web/e2e/fixtures/test-base.ts` (`testPage`, `waitForSeedAgentProfile`,
   `runWithBackendRecovery`) and the backend E2E workspace-reset handler.
   A next experiment should time readiness, profile recovery, workspace reset,
   profile cleanup, settings reset, context creation, and page creation
   separately. Start with message-queue and one of the seven long-tail files.
   Retain every reset and verify consecutive-file isolation in both orders.
   Adoption requires three same-runner samples with fewer accumulated fixture
   seconds and no new failures or retries. The current data supports this
   investigation, not a speculative concurrent-reset implementation.

3. **Keep real lifecycle waits.** Session idle parking, LSP lease release,
   transient retry exhaustion, and coordinator stall detection exercise actual
   lifecycle boundaries. Their page setup is not the long wait. Equivalent
   deterministic-clock coverage would need a separate lifecycle design and
   integration proof; deleting or shortening these tests is not part of this change.

4. **Retain the container planner until execution contexts are measured.**
   Container shard 1 was predicted at 720 seconds but executed in 1.794 seconds
   because full-worker Kubernetes tests were skipped. Another container shard
   took about 15.3 minutes in its test step. A future context-aware timing profile
   must distinguish opt-in full-worker execution from skipped declarations,
   preserve all manifest identities, and test missing/stale profile fallback.
   Validate both contexts before changing the profile schema or packing policy.

5. **Keep one frontend job for this rollout.** The shared pool is saturated.
   Setup reduction saves runner-minutes without adding jobs. Two/four-shard
   activation still requires the documented 30% critical-path gain, at most 10%
   extra runner-minutes, and loaded-feedback nonregression on the optimized
   baseline. Historical local partition tests do not establish those conditions.

No fixture source or test assertion changed during this analysis. Docker and
full-worker acceptance remain unclaimed. More test parallelism and removed tests
are rejected as unmeasured ways to improve the displayed completion time.
