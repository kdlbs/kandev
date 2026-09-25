---
status: current
system: platform
requirements:
  - REQ-PLATFORM-PR-WATCH-IDENTITY-001
  - REQ-PLATFORM-PR-WATCH-IDENTITY-002
  - REQ-PLATFORM-TASK-SUMMARY-CONTENTION-001
  - REQ-PLATFORM-BOUNDED-SESSION-HISTORY-001
  - REQ-PLATFORM-BOUNDED-SESSION-HISTORY-002
  - REQ-PLATFORM-DATABASE-MAINTENANCE-001
  - REQ-PLATFORM-PROVIDER-BACKOFF-001
---

# Canonical PR watch and bounded storage System Design

## Purpose and boundaries

Platform owns the cross-cutting contracts of this change: the task-status
summary projector, install-wide storage and maintenance guarantees, and
background integration loop health. The GitHub provider implementation
(`apps/backend/internal/github`) and the workflow-sync loop
(`apps/backend/internal/workflowsync`) realize the watch and backoff
contracts against their providers; conversation persistence
(`apps/backend/internal/task`) realizes the bounded-history contracts.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-PR-WATCH-IDENTITY-001` | Watch data model and migration; Reconciliation |
| `REQ-PLATFORM-PR-WATCH-IDENTITY-002` | Event publication; Sustained load |
| `REQ-PLATFORM-TASK-SUMMARY-CONTENTION-001` | Projection coalescing and backoff |
| `REQ-PLATFORM-BOUNDED-SESSION-HISTORY-001` | History hydration |
| `REQ-PLATFORM-BOUNDED-SESSION-HISTORY-002` | Payload storage and retention candidates |
| `REQ-PLATFORM-DATABASE-MAINTENANCE-001` | Maintenance command |
| `REQ-PLATFORM-PROVIDER-BACKOFF-001` | Provider failure backoff and health |

## Watch data model and migration

Searching rows are unique on `(task_id, repository_id, branch)` where
`pr_number = 0`; discovered rows are unique on `(task_id, repository_id,
pr_number)` where `pr_number != 0`. `session_id` is optional provenance
stored as `TEXT NOT NULL DEFAULT ''` (empty string when unknown), never a
unique-key component. The
transactional upgrade migration runs after the existing version-change
snapshot boundary, prefers discovered rows while merging the newest status,
check, review, and comment watermarks, removes orphaned rows, and is
idempotent.

## Reconciliation

Per cycle, resolve each task's per-repository branch (checkout branch first,
worktree fallback within the same repository), collapse equivalent sessions
to one target, and transition searching watches atomically. A transition that
collides with an existing canonical sibling merges into the sibling instead of
violating the unique constraint, so concurrent transitions of equivalent
sessions coalesce to one destination.

## Event publication

`GitHubPRFeedback` and `GitHubTaskPRUpdated` publish only when the observed
state differs from the watch watermarks (checks or review state change, or PR
update time advancing). A sync of unchanged state performs no watch writes and
publishes nothing on both single-watch REST and batched GraphQL paths.

## Sustained load

`apps/backend/internal/github/load_validation_test.go` seeds a task resumed
across fifty historical sessions plus concurrent tasks and drives simulated
poll cycles, asserting a constant canonical watch set, one batched branch
lookup per cycle, and bounded cycle latency.

## Projection coalescing and backoff

`apps/backend/internal/task/statussummary/projector.go` keeps per-task
in-process serialization, single-flight pending-refresh coalescing, and the
semantic-equal short-circuit. Rejected compare-and-set attempts are paced by a
small capped exponential backoff with up to +25% jitter
(`defaultCASRetryBackoff`, `casRetryJitter`, injectable through
`ProjectorConfig.RetryBackoff`) and rebase from authoritative state
(`rebaseProjectionStateFromCurrent`) before retrying. The sustained-contention
fixture applies a per-task alternating-reject store across many tasks and
asserts zero exhausted-retry handler errors.

## History hydration

`task_session_messages` keeps a lightweight metadata projection per message.
List endpoints read by cursor (keyset pagination), and tool payload detail is
fetched only through the explicit payload endpoint. No legacy full-row
hydration path remains. Separate reader and writer pools observe committed
writes.

## Payload storage and retention candidates

A payload above the inline threshold is stored in
`task_message_payloads` keyed by digest with compression encoding, byte
size, and an internal reference; message rows retain the lightweight
projection. Explicit detail loading verifies the digest before returning
content. Equivalent Git snapshots within one session and digest group share
one stored snapshot. Candidate queries for orphaned payloads, redundant
snapshots, and obsolete plan revisions expose counts and references without
deleting; retention windows protect HEAD revisions, revert ancestry, recency
defaults, and query limits.

## Maintenance command

`kandev maintenance database` (dispatched by
`apps/backend/internal/launcher/maintenance_database.go`) supports dry run,
explicit retention options, verified backup, and compaction.
`apps/backend/internal/maintenance` verifies the backup before destructive
work (failing closed otherwise), removes only retention-eligible candidates
in transactions, stages compaction with `VACUUM INTO`, validates, and
atomically replaces only after success. Rollback steps are printed; storage
gauges report database, WAL, and per-table logical sizes without contents or
credentials.

## Provider failure backoff and health

`apps/backend/internal/common/authcircuit` provides the shared `FailureClass`
(auth, config, transient), `Backoff`, and `State`. The GitHub PR-watch poller
(`poller_circuit.go`) holds a poller-scoped, mutex-guarded per-workspace
circuit consulted by `filterOpenCircuitWatches` before each cycle, refreshing
each present workspace's connection fingerprint once per cycle
(`Service.WorkspaceConnectionFingerprint`) and recording outcomes at all
three poll-path call sites through `classifyPollErr`. The workflow-sync loop
persists circuit state and fingerprint per config and skips due-but-open
configs. GitHub credential and missing-target failures also suspend workflow
sync until a credential or configuration change or an explicit Sync now
attempt. GitLab workflow sync keeps bounded auth/config circuit retries.
`apps/backend/internal/health` exposes aggregate health (counts by
class, no workspace identifiers or secrets), and bounded-label expvar counters
report skips, resets, and failures.

### Review-cleanup extension

The [review task cleanup package](../../integrations/system-design/github-review-task-cleanup.md)
extends this existing backoff contract to scheduled review-task feedback. It
does not change task-owned PR-watch identity or the manual cleanup endpoints.

Keep review-cleanup circuit state separate from the PR-monitor circuit. A
successful PR-monitor request must not clear a failed review-cleanup target.
Key shared authentication and rate-limit failures by workspace automation
connection. Keep a record-specific configuration failure isolated to that
review record. One inaccessible PR must not suspend unrelated work in the
workspace. Use non-secret connection generation and status as the workspace
reset fingerprint. Reset a record-specific circuit when its watch
configuration changes, excluding routine polling timestamps. Circuit state
remains in memory and is discarded on restart. Remove record-specific state
when its dedup row disappears.

Before resolving automation credentials for scheduled cleanup, inspect the
Core rate tracker and workspace circuit. If Core is exhausted or the workspace
circuit is open, skip credential resolution. After resolving credentials,
inspect record-specific admission before each feedback request. A failed
feedback fetch must reach the poller as a classified outcome so it can open
the relevant circuit.
After a shared authentication or rate-limit failure, stop the remaining
feedback requests for that workspace in the same cycle. Continue other
workspaces unless the shared Core tracker reports exhaustion. A record-specific
configuration failure stops only that record. Search success does not count as
feedback success and does not reset the cleanup circuit.

Use the existing `authcircuit.State`, `classifyPollErr`, and connection
fingerprint rules. Do not change the public cleanup response shape. Keep
manual cleanup and explicit watch actions outside scheduled circuit admission.
Aggregate review-cleanup skips and failures into bounded-label health metrics;
never use task, watch, PR, repository, or workspace IDs as metric labels.

## Related decisions

- [Keep PR watches task owned](../../../decisions/2026-08-31-task-owned-pr-watch-identity.md)
- [Install-wide storage maintenance uses typed ownership providers and quarantine](../../../decisions/0045-install-wide-storage-maintenance.md)
