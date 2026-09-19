---
status: active
system: platform
created: 2026-09-14
owners:
  - kandev
---

# Canonical PR watch and bounded storage Requirements

## Overview

A Kandev task can be resumed across many sessions and worked by parallel
sessions. Legacy `github_pr_watches` identity was keyed by session, so each
resumed or parallel session could create its own watch row for the same task,
repository, and branch. Duplicate rows multiplied GitHub polling, event
publication, and status-summary work, and completing a session could silently
stop monitoring a Review task.

Long-lived installations also accumulate large tool payloads, Git snapshots,
and plan revisions, and background integration loops retried broken
credentials every cycle. This change makes watch identity task-owned,
reconciliation idempotent, projection contention-safe, operational storage
bounded and retention-eligible, maintenance backup-gated, and provider
failures visibly backed off.

Platform owns this cross-cutting contract because it covers the shared
task-status summary projector, the install-wide storage and maintenance
guarantees, and the background integration loops' health. The integration
system's GitHub provider contracts and the task system's conversation history
consume and feed these behaviors.

## Requirements

### REQ-PLATFORM-PR-WATCH-IDENTITY-001: Task-owned pull request watch identity

**Intent:** One task, repository, and branch (or pull request) shall have at
most one canonical watch row regardless of how many sessions observed it,
and monitoring shall survive completion of the session that created it.

#### Acceptance criteria

- **AC-PLATFORM-PR-WATCH-IDENTITY-001.1:** A still-searching watch (no pull
  request number yet) shall be unique per task, repository, and branch; a
  discovered watch shall be unique per task, repository, and pull request
  number; `session_id` shall remain optional provenance that cannot create a
  second canonical watch.
- **AC-PLATFORM-PR-WATCH-IDENTITY-001.2:** The database shall enforce both
  identities with partial unique indexes, and watch creation and ensure paths
  shall deduplicate by task-owned keys so resumed or concurrent sessions reuse
  the one canonical watch.
- **AC-PLATFORM-PR-WATCH-IDENTITY-001.3:** Moving a searching watch to a
  branch or pull-request number that already has a canonical watch shall merge
  into that sibling instead of violating the unique constraint, and concurrent
  transitions of equivalent sessions shall coalesce to one destination.
- **AC-PLATFORM-PR-WATCH-IDENTITY-001.4:** The upgrade migration shall run
  transactionally after the existing database snapshot boundary, collapse
  legacy session duplicates preferring discovered rows while preserving the
  newest status, check, review, and comment watermarks, remove watches for
  missing tasks or detached repositories, clear provenance for missing
  sessions, and be idempotent on a second boot.
- **AC-PLATFORM-PR-WATCH-IDENTITY-001.5:** Active watch listing shall exclude
  invalid or orphaned watches but retain a non-archived Review task whose
  provenance session has completed.
- **AC-PLATFORM-PR-WATCH-IDENTITY-001.6:** Branch resolution shall be keyed by
  task and repository, so a branch observation for one repository cannot
  overwrite a still-searching watch of another repository of the same task.

### REQ-PLATFORM-PR-WATCH-IDENTITY-002: Unchanged watch state causes no writes or duplicate lookups

**Intent:** Reconciliation that observes no change shall not write rows or
publish events, and repeated poll cycles shall not amplify lookups with
session history.

#### Acceptance criteria

- **AC-PLATFORM-PR-WATCH-IDENTITY-002.1:** A branch observation equivalent to
  a watch's current state shall perform no insert and no watch update.
- **AC-PLATFORM-PR-WATCH-IDENTITY-002.2:** Pull request feedback and task
  pull request events shall publish only for durable relevant state changes;
  an unchanged sync shall publish nothing on both single-watch REST and
  batched GraphQL paths.
- **AC-PLATFORM-PR-WATCH-IDENTITY-002.3:** A task resumed across many
  sessions shall drive one branch or pull request lookup per canonical target
  per poll cycle, and a simulated sustained-load run shall keep the canonical
  watch set and per-cycle lookup counts constant across cycles.

### REQ-PLATFORM-TASK-SUMMARY-CONTENTION-001: Coalesce and rebase task summary projection under contention

**Intent:** Equivalent projection work for one task shall converge to one
effective persistence and publication, and a writer that genuinely loses a
compare-and-set race shall rebase and retry with bounded pacing instead of
failing the handler.

#### Acceptance criteria

- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.1:** Concurrent equivalent pending
  refreshes for one task shall be single-flight coalesced and result in at
  most one effective summary persistence and publication.
- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.2:** A writer whose
  compare-and-set attempt is rejected shall reload the authoritative summary
  state and rebase its derivation on it before retrying.
- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.3:** Retry pacing between rejected
  attempts shall use a bounded exponential backoff with jitter and respect
  context cancellation; the attempt bound shall remain unchanged and pacing
  shall be injectable so tests can run instantaneously.
- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.4:** Under sustained per-task
  contention where an external writer forces repeated compare-and-set losses,
  concurrent events for many tasks shall preserve each task's correct final
  summary and report zero exhausted-retry handler errors.
- **AC-PLATFORM-TASK-SUMMARY-CONTENTION-001.5:** Per-task in-process
  serialization and the semantic no-op short-circuit for equivalent derived
  summaries shall keep a semantically-equal refresh from touching the store or
  publishing a new revision.

### REQ-PLATFORM-BOUNDED-SESSION-HISTORY-001: Cursor-bounded history hydration

**Intent:** Normal session-history reads shall stay responsive by fetching
only the requested window and never eagerly materializing large tool
metadata.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-SESSION-HISTORY-001.1:** Session history list responses
  shall accept and return a stable cursor so each hydration reads only the
  requested window.
- **AC-PLATFORM-BOUNDED-SESSION-HISTORY-001.2:** Normal history hydration shall
  return lightweight tool metadata without the full payload; the stored detail
  of a retained tool payload shall be returned only by a scoped explicit
  request.
- **AC-PLATFORM-BOUNDED-SESSION-HISTORY-001.3:** A reader on a separate pool
  shall observe committed writer updates, so paginated history stays fresh
  across readers and writers.

### REQ-PLATFORM-BOUNDED-SESSION-HISTORY-002: Digest-backed operational payload storage

**Intent:** Large operational payloads shall be stored once, verifiably, and
retainably without enlarging every transcript row.

#### Acceptance criteria

- **AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.1:** A tool payload exceeding the
  configured inline threshold shall be compressed or externalized with a
  recorded digest, encoding, byte size, and internal reference; payload
  metadata keyed by payload identity shall be deduplicated between sessions.
- **AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.2:** Equivalent Git snapshots
  within the same session and content-digest group shall deduplicate;
  snapshots from different sessions remain distinct history.
- **AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.3:** Retention selection shall
  identify superseded tool payloads, redundant live and status Git snapshots,
  and obsolete plan revisions only as reportable candidates; selection respects
  configured retention and is non-destructive until maintenance executes, and
  human and agent conversational messages shall remain preserved by default.

### REQ-PLATFORM-DATABASE-MAINTENANCE-001: Backup-gated database maintenance command

**Intent:** The native maintenance database command shall report, execute,
and roll back bounded-storage cleanup only behind a verified backup, never
silently deleting conversation history.

#### Acceptance criteria

- **AC-PLATFORM-DATABASE-MAINTENANCE-001.1:** The command shall support a dry
  run and explicit retention settings, and report per-table candidate and
  reclaim estimates without contents or credentials.
- **AC-PLATFORM-DATABASE-MAINTENANCE-001.2:** Destructive execution shall fail
  closed when a verified backup is absent; the verified backup may be an
  operator-supplied path or a freshly-created snapshot.
- **AC-PLATFORM-DATABASE-MAINTENANCE-001.3:** Execution shall remove only
  selected redundant payloads, snapshots, and plan revisions inside
  transactions; compaction shall stage and validate its result (`VACUUM INTO`
  or equivalent) and atomically replace only after success.
- **AC-PLATFORM-DATABASE-MAINTENANCE-001.4:** The command shall report
  SQLite/WAL and table-storage measurements and rollback steps without
  database contents, credentials, or destructive default behavior.
- **AC-PLATFORM-DATABASE-MAINTENANCE-001.5:** Upgrade shall not silently delete
  task history; public operations documentation shall describe backup of
  `kandev.db` and `master.key`, image upgrade, migration and maintenance dry
  run, execute and compact, rollback, and post-release health checks.

### REQ-PLATFORM-PROVIDER-BACKOFF-001: Generation-aware provider failure backoff

**Intent:** Authentication and configuration failures on background
integration loops shall enter exponential backoff with fingerprint-aware
reset, so a credential or configuration change resumes probing on the very
next cycle.

#### Acceptance criteria

- **AC-PLATFORM-PROVIDER-BACKOFF-001.1:** A background loop observing an
  authentication or configuration failure shall open a per-target circuit
  keyed by the owning workspace or configuration and skip further provider
  calls for that target until its backoff expires.
- **AC-PLATFORM-PROVIDER-BACKOFF-001.2:** A non-secret connection fingerprint
  (status and credential generation) shall reset an open circuit when it
  changes, forcing an immediate probe after a credential rotate, reconnect, or
  re-auth; an empty or unknown fingerprint shall never reset an open circuit.
- **AC-PLATFORM-PROVIDER-BACKOFF-001.3:** While a circuit is open, the loop
  shall make zero additional provider calls for that target; after a reset or
  expiry, one probe resumes evaluation.
- **AC-PLATFORM-PROVIDER-BACKOFF-001.4:** Failures shall be classified as auth,
  config, or transient; a client capability fallback (for example, no GraphQL
  support) is deliberately not a failure and shall not open any circuit.
- **AC-PLATFORM-PROVIDER-BACKOFF-001.5:** Health and status output shall
  distinguish healthy, degraded, and disabled integration states, aggregate
  open circuits by failure class without per-workspace identifiers or secret
  material, and expose circuit skip, reset, and failure counters as
  bounded-label metrics (provider and class only).

## Out of scope

- Agent identity and runtime profiles, owned by the agent system.
- Executor runtime environments, owned by the executor system.
- Frontend settings presentation, owned by the UI system.
