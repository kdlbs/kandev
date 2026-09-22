---
id: "06-adaptive-discovery"
title: "Back off idle searching watches"
status: done
wave: 6
depends_on: ["05-bounded-fallback"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.1
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.2
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.3
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.4
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-001.5
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 06: Back off idle searching watches

## Summary

Back off idle searching watches. Implement with TDD and preserve workspace identity.

## In scope

Add a due-target selector with an injected clock. Keep the scheduler tick at
1 minute. Searching intervals are 1 minute below 2 hours idle or while running,
15 minutes from 2 to below 24 hours, and 30 minutes thereafter.
New watches receive a next-tick attempt. Known PRs remain at 1 minute.

Load task activity in bulk through a narrow adapter. Reuse task-domain query
patterns without treating generic UpdatedAt or provider notifications as activity.
Include session state, human conversation, agent execution, and observed branch
commit/push activity. Missing evidence retains fast discovery. No provider reads
or remote Git scans are allowed for activity classification.

Use the fastest eligible member for shared discovery targets. Preserve due
state across reconciliation and restart using existing persisted activity and
LastCheckedAt. Passive sync must honor scheduling; explicit refresh bypasses
idleness only. Keep quota/auth admission intact.

Add TestSearchingWatchAdaptiveSchedule in a new poller_schedule_test.go.
Cover exact boundaries, restart, activity after a slow check, unknown activity,
all-idle and mixed shared targets, archive/delete, and external PR discovery
without a new commit. Add service-level passive/explicit refresh regressions.
Inspect LoadTaskLastActivity in task/repository/sqlite/task_status_summary.go
as a query pattern, not as an assumed clean activity signal.

Update `docs/public/integrations.md` during implementation to describe the
shipped freshness bounds. Keep the public guide unchanged during planning.
Run these additional checks after that documentation edit:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Out of scope

New UI controls, credential policies, global schedulers, and unrelated providers.

## Acceptance

- Meet every linked acceptance criterion with deterministic regressions.
- Preserve existing policy, scope, and error-handling contracts.
- Record the expected red test result and passing package results.

## Verification

Run from the repository root.

```bash
(cd apps/backend && go test ./internal/github -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/poller_schedule.go`
- `apps/backend/internal/github/poller_schedule_test.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/task/repository/interface.go` (only if the activity adapter needs a new task-domain query)
- `apps/backend/internal/task/repository/sqlite/task_status_summary.go` and adjacent tests

If the task activity query changes, also run:

```bash
(cd apps/backend && go test ./internal/task/repository/sqlite -run "Activity|Watch" -count=1)
```

- `docs/public/integrations.md`

## Dependencies

05-bounded-fallback.

## Risks

Provider writes can contaminate generic timestamps. Passive UI requests can defeat backoff.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-discovery-health.md)
- [Design](../../specs/integrations/system-design/github-pr-discovery-health.md)
- Source files and existing adjacent tests listed above. New helper/test files are created as needed.

## Results

Implemented adaptive passive discovery scheduling with a one-minute scheduler
tick. Searching watches check every minute while running or recently active,
then back off to 15 minutes after two hours of inactivity and 30 minutes after
24 hours. A newly created watch uses the newer of its creation time and the
authoritative task activity timestamp, so an old task does not make a fresh
watch enter a slow tier after its first check. Known pull requests remain on
the one-minute cadence, and activity, restart, archive, and deletion behavior
retain due state correctly.

Added a bulk task-activity projection and backend adapter. The projection uses
human conversation, agent execution, session state, and observed branch
activity, while excluding generic task timestamps, lifecycle messages, and
live-monitor snapshots. Shared discovery targets use the fastest eligible
watch. Passive refresh and its bounded per-watch fallback honor due admission;
explicit refresh remains immediate.

The task WebSocket sync boundary now carries an explicit refresh bit. Automatic
subscription, reconnect, and retry requests use passive due admission, while a
user refresh bypasses idle admission and still obeys quota and authentication
gates. Added deterministic scheduler, repository-query, production-shaped
handler, and passive/explicit refresh regressions. Verification passed:

```bash
(cd apps/backend && go test ./internal/github -count=1)
(cd apps/backend && go test ./internal/task/repository/sqlite -run "Activity|Watch" -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```
