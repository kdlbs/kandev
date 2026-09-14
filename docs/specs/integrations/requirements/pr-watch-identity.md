---
status: active
system: integrations
created: 2026-09-14
owners:
  - kandev
---

# Canonical task pull request watch identity Requirements

## Overview

A Kandev task can be resumed across many sessions and worked by parallel
sessions. Legacy `github_pr_watches` identity was keyed by session, so each
resumed or parallel session could create its own watch row for the same task
repository and branch. Duplicate rows multiplied GitHub polling, event
publication, and status-summary work for one logical target, and deleting or
completing a session could silently stop monitoring a Review task.

Integrations owns this contract because it owns external pull request
synchronization and the watch rows that drive it. Task and UI systems consume
the resulting associations unchanged.

## Requirements

### REQ-INTEGRATIONS-PR-WATCH-IDENTITY-001: Task-owned pull request watch identity

**Intent:** One task, repository, and branch (or pull request) shall have at
most one canonical watch row regardless of how many sessions observed it, and
monitoring shall survive the completion of the session that created it.

#### Acceptance criteria

- **AC-INTEGRATIONS-PR-WATCH-IDENTITY-001.1:** A still-searching watch (no pull
  request number yet) shall be unique per task, repository, and branch; a
  discovered watch shall be unique per task, repository, and pull request
  number. `session_id` shall remain optional provenance that cannot create a
  second canonical watch.
- **AC-INTEGRATIONS-PR-WATCH-IDENTITY-001.2:** The database shall enforce both
  identities with partial unique indexes, and watch creation and ensure paths
  shall deduplicate by task-owned keys so resumed or concurrent sessions reuse
  the one canonical watch.
- **AC-INTEGRATIONS-PR-WATCH-IDENTITY-001.3:** Moving a searching watch to a
  branch or pull-request number that already has a canonical watch shall merge
  into that sibling instead of violating the unique constraint, and concurrent
  transitions of equivalent sessions shall coalesce to one destination.
- **AC-INTEGRATIONS-PR-WATCH-IDENTITY-001.4:** An upgrade migration shall run
  transactionally after the existing database snapshot boundary, collapse
  legacy session duplicates (preferring discovered rows and preserving the
  newest status, check, review, and comment watermarks), remove watches for
  missing tasks or detached repositories, clear provenance for missing
  sessions, and be idempotent on a second boot.
- **AC-INTEGRATIONS-PR-WATCH-IDENTITY-001.5:** Active watch listing shall
  exclude invalid or orphaned watches but retain a non-archived Review task
  whose provenance session has completed.
- **AC-INTEGRATIONS-PR-WATCH-IDENTITY-001.6:** Branch resolution shall be
  keyed by task and repository, so a branch observation for one repository
  cannot overwrite a still-searching watch of another repository of the same
  task, and multiple active sessions resolving to the same task, repository,
  and branch shall produce one watch target per reconciliation cycle.

### REQ-INTEGRATIONS-PR-WATCH-IDENTITY-002: Unchanged watch state causes no writes

**Intent:** Reconciliation that observes no change shall not write rows or
publish events, so repeated cycles do not amplify database and event work.

#### Acceptance criteria

- **AC-INTEGRATIONS-PR-WATCH-IDENTITY-002.1:** A branch observation equivalent
  to a watch's current state shall perform no insert and no watch update.
- **AC-INTEGRATIONS-PR-WATCH-IDENTITY-002.2:** Pull request feedback and task
  pull request events shall publish only for durable relevant state changes,
  coalesced by task, repository, pull request, head SHA, and status; an
  unchanged sync shall publish nothing on REST and batched GraphQL paths alike.
- **AC-INTEGRATIONS-PR-WATCH-IDENTITY-002.3:** Historical session duplication
  shall not affect lookup counts: a task resumed across many sessions shall
  drive one branch or pull request lookup per canonical target per poll
  cycle, and a simulated sustained-load run shall keep the canonical watch
  set and per-cycle lookup counts constant across cycles.

## Out of scope

- Status-summary projection contention semantics, owned by
  [bounded task status delivery](../platform/requirements/bounded-task-status-delivery.md).
- Storage retention and maintenance, owned by the
  [system page](../system-page/README.md).
