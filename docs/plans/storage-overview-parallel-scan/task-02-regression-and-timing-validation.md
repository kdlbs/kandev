---
id: "02-regression-and-timing-validation"
title: "Validate storage scan and idle behavior"
status: done
wave: 3
depends_on: ["01-parallelize-overview-measurements", "03-disk-capacity-progress", "04-workspace-dependency-cleanup"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-overview-parallel-scan.md"
---

# Task 02: Validate storage scan and idle behavior

## Acceptance

- Existing Storage E2E coverage still shows policy, history, and quarantine before a held overview
  response, the independent disk-capacity card renders before a held overview response, and the
  mobile Storage flow has no horizontal overflow.
- Cache, handler, operations, scheduler, activity-coordinator, disk-capacity, and workspace
  dependency tests pass after the changes.
- When a compatible post-change backend is available, a comparable cold overview request is timed
  on the same host/settings as the baseline and the independent disk request is timed separately.
  If the running backend predates the disk route, the baseline, deterministic fan-out barrier, and
  held-overview managed E2E are recorded as the accepted non-mutating evidence instead.
- The Aug 5 `skipped_busy` behavior remains explainable by active `execution_preparing` and
  `execution_running` leases; no cleanup is run while those resources are active.

## Verification

```bash
cd apps/backend && go test -race ./internal/backendapp ./internal/system/storage ./internal/agent/runtime/activity ./internal/agent/runtime/lifecycle
cd apps/web && pnpm e2e:run --project chromium tests/system/storage-maintenance.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-maintenance.spec.ts
```

For the live timing comparison, use a read-only cold-cache setup and the existing backend request
duration logs. Do not drive the user's live browser or mutate storage data to manufacture a scan.

## Files likely touched

- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- implementation task handoff or validation notes for the before/after timing result

## Dependencies

Tasks 01, 03, and 04.

## Inputs

- Existing progressive-loading, capacity, dependency-option, and mobile Storage E2E scenarios
- Existing cache, capacity, dependency-cleanup, and idle-gate tests
- Baseline cold request: approximately 103 seconds on the captured host; warm request: effectively
  instantaneous due to the successful overview cache

## Output contract

Return exact commands and results, available cold/warm timing evidence, mobile artifacts if a test
fails, idle-gate confirmation, and remaining optimization risks. Mark this task done only after
Task 01 is implemented and the regression evidence is complete.

## Result

The pre-change cold baseline was approximately 103 seconds on the captured host, with warm reads
effectively instantaneous from the 15-minute cache. The new deterministic fan-out test proves the
four measurements overlap, and the managed E2E progressive-loading test holds the overview while
the disk card and fast sections render. Chromium Storage E2E passed 6/6; mobile Storage E2E passed
4/4 after the touch disclosure flow was isolated from the existing scheduled-help tooltip.

The existing activity/lifecycle packages and storage race suite pass, along with backend lint and
the Windows disk-metrics compile check. Fresh desktop and mobile PR captures show the progress
card and independent loading state. The idle contract remains
unchanged: the Aug 5 `skipped_busy` entry correctly reported active `execution_preparing` and
`execution_running` resources. A same-dataset post-change live timing sample was not collected
because the currently running local backend predates this endpoint; its disk request returned 404.
The managed E2E evidence is the non-mutating substitute for independent capacity availability.
