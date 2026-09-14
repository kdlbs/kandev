---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-PR-WATCH-IDENTITY-001
  - REQ-INTEGRATIONS-PR-WATCH-IDENTITY-002
---

# Canonical task pull request watch identity System Design

## Purpose and boundaries

Integrations owns GitHub watch persistence and the poll loop that observes
pull requests. This design defines the canonical watch data model, the
transactional upgrade migration, and the idempotent reconciliation behavior.
Task status projection consumes the resulting events; the projector's own
contention contract is owned by the platform system's
[bounded task status delivery](../../platform/requirements/bounded-task-status-delivery.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-PR-WATCH-IDENTITY-001` | Data model; Migration; Reconciliation |
| `REQ-INTEGRATIONS-PR-WATCH-IDENTITY-002` | Reconciliation; Event publication |

## Components and responsibilities

- `apps/backend/internal/github/store.go`: watch schema, partial unique
  indexes, task-owned lookups (`GetPRWatchByTaskRepoBranch`,
  `GetPRWatchByTaskRepoPRNumber`), collision-safe branch and pull-number
  updates, and the transactional `migratePRWatchesToTaskOwnership` upgrade
  migration running after the existing version-change snapshot.
- `apps/backend/internal/github/service_pr_watch.go`: creation and ensure
  deduplication by task-owned keys; `TaskBranchProvider`
  branch resolution keyed by task and repository; per-cycle reconciliation
  that emits one `TaskBranchInfo` per canonical target.
- `apps/backend/internal/github/poller.go`: active-watch listing, batched
  GraphQL branch search per workspace, per-watch REST fallback, and
  `detectPRForWatch` PR discovery that reuses the watch-owned client
  resolution and records the workspace's poll-circuit outcome.
- `apps/backend/internal/github/load_validation_test.go`: deterministic
  sustained-load fixture proving watch-set and lookup-count stability across
  simulated poll cycles.

## Data model

Searching rows are unique on `(task_id, repository_id, branch)` where
`pr_number = 0`; discovered rows are unique on `(task_id, repository_id,
pr_number)` where `pr_number != 0`. `session_id` is a nullable provenance
column. Watch rows carry status, check, review, and comment watermarks used to
decide durable change.

The upgrade migration snapshots before destructive work, resolves duplicates
deterministically preferring discovered rows while merging the newest
watermarks, removes orphaned rows, and is idempotent.

## Reconciliation

Per cycle, resolve each task's per-repository branch (checkout branch first,
worktree fallback within the same repository), collapse equivalent sessions to
one target, and transition searching watches atomically. A transition that
collides with an existing canonical sibling merges into the sibling instead of
violating the unique constraint.

## Event publication

`GitHubPRFeedback` and `GitHubTaskPRUpdated` publish only when the observed
state differs from the watch watermarks (checks or review state change, or PR
update time advancing). A sync of unchanged state performs no watch writes and
publishes nothing on both single-watch REST and batched GraphQL paths.

## Failure and recovery

A circuit-open workspace (authentication or configuration failure) is skipped
for the backed-off window; its credential fingerprint refresh resets the
circuit on rotate or re-auth. Detection only prunes a failed discovery target
once every watch consumer of that target is deleted. A client without GraphQL
support falls back to per-watch REST before outcome recording.

## Related decisions

- [Keep PR watches task owned](../../../decisions/2026-08-31-task-owned-pr-watch-identity.md)
