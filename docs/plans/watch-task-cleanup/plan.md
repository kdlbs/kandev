---
created: 2026-09-22
status: complete
requirements:
  - REQ-INTEGRATIONS-WATCH-CLEANUP-001
  - REQ-INTEGRATIONS-WATCH-CLEANUP-002
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-001
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-002
system_design:
  - ../../specs/integrations/system-design/watch-task-cleanup.md
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
  - ../../specs/integrations/system-design/github-workflow-attention.md
legacy_specs: []
---

# Implementation plan: Watch task cleanup

## Overview

Prevent historical tasks from consuming provider quota. Implement GitHub
eligibility first, GitHub batch termination second, and sibling filters third.
Then reduce cleanup reads, bound fallback, add adaptive discovery, and cache
Actions observations. All seven sequential work orders are complete.

## Source documents

- [Requirements](../../specs/integrations/requirements/watch-task-cleanup.md)
- [System design](../../specs/integrations/system-design/watch-task-cleanup.md)
- [Discovery requirements](../../specs/integrations/requirements/github-pr-discovery-health.md)
- [Discovery design](../../specs/integrations/system-design/github-pr-discovery-health.md)
- [Actions requirements](../../specs/integrations/requirements/github-workflow-attention.md)
- [Actions design](../../specs/integrations/system-design/github-workflow-attention.md)

## Confirmed root cause and reproduction

GitHub cleanup list queries read historical deduplication rows without task
eligibility checks. Service gates fetch remote state before engagement checks.
Fetch errors become false eligibility results, so the batch continues.

A temporary in-memory SQLite reproduction executed the actual SQL extracted
from both GitHub review list methods. Both returned task IDs `active`,
`archived`, `deleted`, and an empty reservation. Only the first and last should
be cleanup candidates. No production database or provider API was accessed.

GitLab review and issue queries also lack task eligibility filters. Azure
DevOps review and work-item queries filter watch generation only. Both
providers fetch upstream state before checking task engagement.

The current GitHub feedback helper requests PR data, reviews, comments, checks,
and optional workflow attention. The exact request count differs from the
reported older implementation, but historical-row amplification remains.

## Scope and compatibility

Preserve cleanup policies, lifecycle-prompt protection, empty GitHub
reservations, explicit watch deletion, and reset behavior. Ignore historical
records without changing deduplication. No UI or schema changes are required.
Unarchived Done tasks remain eligible. Archive races after query selection
remain outside the atomicity guarantee.

## Work orders

- [x] [Task 01: Filter GitHub cleanup candidates](task-01-github-candidates.md)
- [x] [Task 02: Stop GitHub cleanup on rate limits](task-02-github-rate-limits.md)
- [x] [Task 03: Filter sibling provider cleanup candidates](task-03-sibling-candidates.md)

- [x] [Task 04: Use minimal PR reads for cleanup](task-04-minimal-cleanup.md)
- [x] [Task 05: Bound per-workspace PR fallback](task-05-bounded-fallback.md)
- [x] [Task 06: Back off idle searching watches](task-06-adaptive-discovery.md)
- [x] [Task 07: Cache Actions observations with bounded expiry](task-07-actions-cache.md)

## Polling and cache values

| Function | Shipped behavior | Bound/details |
| --- | --- | --- |
| PR poller scheduler tick | 1 minute | Fixed scheduler tick |
| Searching, running or idle below 2 hours | 1 minute | Fast discovery |
| Searching, idle 2 to below 24 hours | 15 minutes | Activity restores next-tick eligibility |
| Searching, idle at least 24 hours | 30 minutes | Never permanently stops |
| Known PR background checks | 1 minute | Fixed cadence |
| Review-watch discovery and cleanup | 5 minutes | Eligible tasks only |
| Issue-watch discovery and cleanup | 5 minutes | Eligible tasks only |
| Status/search response TTL | 30 seconds | Outer response cache |
| Explicit feedback response TTL | 8 seconds | Explicit refresh invalidates |
| PR sync freshness window | 30 seconds | Passive status freshness |
| Actions changing, empty, or attention-required | 30 seconds | Run and job observations |
| Actions all complete, no attention required | 5 minutes | Same-SHA reruns appear after expiry |
| Actions runs/jobs capacity | 512 entries each | In-memory bounded caches |
| Actions batch concurrency / total budget | 4 / 5 seconds | Preserved |
| Fallback target budget | 5 per workspace, 10 globally | Rotating per cycle |

Activity restores next-tick eligibility. Explicit refresh bypasses idle and
Actions cache delays but never quota/auth admission. Poll ticks can add up to
1 minute to the interval. Missing activity information selects fast polling.
A completed-SHA external rerun may take up to 6 minutes to appear automatically.

Retain discovery rate-limit retry delays of 1, 2, 4, 8, then 15 minutes when
no provider retry time is available. Existing provider retry deadlines are
capped at 15 minutes by discovery health. Auth/config circuit backoff starts
at 2 minutes, caps at 6 hours, and uses 25 percent jitter. The poller rate-limit
sleep cap remains 10 minutes per wait; it does not clear the exhaustion state.

## Tests

Each work order lists new regression names, existing test locations, and exact
commands. Use store-backed service tests to prove candidate selection reaches
provider-call behavior. Browser tests add no evidence for this backend-only
change. Run each regression before production edits and record its expected
failure. Then run the affected package suites.

## Verification results

- `python3 scripts/list-docs.py validate`: passed, 299 decisions and 1110 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `git diff --check`: passed.

Implementation is complete. Production, permanent test, specification, plan,
and public documentation changes are present in the working tree and remain
uncommitted.

## Risks

- Existing tests often attach fictitious task IDs without task rows. Update
  active fixtures rather than weakening eligibility.
- GitHub by-watch queries also support explicit watch deletion. Preserve its
  unfiltered inventory through existing task-ID methods.
- A batch can stop only after an error becomes visible. Concurrent requests
  within its current feedback fetch may already be running.
- Ignored historical deduplication rows remain stored. This fix bounds polling,
  not storage retention or quota consumed by other active work.

## Documentation impact

Internal requirements, designs, and work orders describe the expanded scope.
No controls change. The public integration guide documents delayed idle
discovery and bounded Actions freshness as shipped behavior.
