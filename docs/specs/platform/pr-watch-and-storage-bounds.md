---
status: draft
created: 2026-08-31
owner: kandev
---

# Canonical PR Monitoring and Bounded Task Storage

## Why

Long-lived Kandev installations must remain responsive as tasks are resumed and agents generate rich tool and Git history. Duplicate PR monitoring and unbounded large operational payloads create database growth and background work unrelated to available host CPU, memory, or I/O capacity.

## What

- Kandev monitors each searching pull request once per task, repository, and branch, and each discovered pull request once per task, repository, and PR number.
- A Review task continues to receive PR monitoring after its originating session completes. Orphaned, archived-task, and invalid repository watches are not polled.
- Unchanged branch state causes no watch creation or watch update. A branch change transitions one searching watch atomically and never oscillates between equivalent session observations.
- PR feedback and task-PR events are emitted only for a changed task/repository/PR/head-SHA/status state. Equivalent events cause no task-summary revision or handler error.
- Task-status projection coalesces equivalent pending work per task, serializes projection per task, rebases from authoritative state after genuine contention, and uses bounded jittered backoff for a cross-writer CAS race.
- Session history APIs are cursor-paginated and normal history hydration excludes large tool-call metadata. Detailed tool payloads are retrieved only when explicitly requested.
- Large tool execution payloads are compressed or externalized with an integrity digest/reference. Human and agent conversational messages remain preserved by default.
- Equivalent Git snapshots are stored once, and configurable retention can remove only redundant snapshots, superseded tool payloads, and obsolete plan revisions.
- An operator-run database maintenance command reports a dry-run reclaim estimate by table, requires a current database backup before destructive cleanup, supports configured retention, safely compacts SQLite, and documents rollback. Upgrade does not silently delete task history.
- GitHub authentication and configuration failures enter a visible degraded/backoff state and resume probing after credentials or configuration change.
- Health/metrics expose watch cardinality and duplication, per-canonical-PR polling, projection contention and failures, storage and SQLite/WAL size, hydration latency, queue depth, and active runtime count.

## Data model

`github_pr_watches` is task-owned as defined by [ADR-2026-08-31-task-owned-pr-watch-identity](../../decisions/2026-08-31-task-owned-pr-watch-identity.md). Searching identity is `(task_id, repository_id, branch)`; discovered identity is `(task_id, repository_id, pr_number)`. `session_id` is nullable provenance and never a unique identity component.

Tool payload storage records digest, encoding, byte size, and an internal reference when a payload exceeds the configured inline threshold. `task_session_messages` retains a lightweight metadata projection sufficient for transcript rendering. Retention records report, but do not implicitly delete, superseded payloads, duplicate snapshots, and plan revisions older than their configured retention policies.

## API and operations surface

- Existing session-message list responses accept and return stable cursor pagination. Normal list/detail hydration returns lightweight tool metadata; a scoped explicit payload request returns the stored detail when it is retained.
- `kandev maintenance database` supports `--dry-run`, explicit retention options, `--backup <path>` or a verified freshly-created snapshot, `--compact`, and prints per-table candidate/reclaim estimates and rollback instructions. Destructive execution fails closed without a verified backup.
- Health/status output reports integration state (`healthy`, `degraded`, `disabled`) and the backoff reason without credentials. Debug metrics and structured logs expose the required watch, projection, storage, latency, queue, and runtime measures.

## Failure modes

- A watch migration or maintenance deletion is not started without a successful database snapshot. A failed transaction leaves all affected rows unchanged; a failed compaction leaves the original database in place and reports the staged artifact for removal or inspection.
- A missing task/repository relationship makes a watch ineligible for polling and eligible for the safe migration report; it never triggers a GitHub request.
- Invalid/unchanged GitHub credentials do not cause continuous polling. The integration reports degradation and only retries after its backoff elapses or the credential/configuration generation changes.
- A status-summary CAS loss reloads and rebases authoritative state. Equivalent queued refreshes succeed as no-ops and do not surface an event-handler error.

## Persistence guarantees

- Canonical watches, PR status watermarks, integration health/backoff state, and retention configuration survive restart.
- Conversational messages are not removed by upgrade or default retention. A retained compressed/external tool payload remains integrity-verifiable and is either retrievable or explicitly reported as expired.
- Database maintenance preserves a pre-operation backup and supplies a deterministic rollback procedure. SQLite compaction uses an atomically staged replacement or leaves the original file untouched.

## Scenarios

- **GIVEN** fifty historical sessions for one task, repository, and branch, **WHEN** reconciliation and polling run, **THEN** exactly one canonical searching or discovered watch exists and one GitHub lookup is issued for that identity.
- **GIVEN** a Review task whose provenance session is complete, **WHEN** its discovered PR changes, **THEN** Kandev continues monitoring and publishes one changed feedback/update event.
- **GIVEN** unchanged task/repository Git state, **WHEN** reconciliation runs repeatedly, **THEN** it creates no rows and updates no branch.
- **GIVEN** a canonical searching watch and a changed canonical branch, **WHEN** reconciliation runs, **THEN** one atomic transition occurs and later identical cycles make no write.
- **GIVEN** repeated equal PR events, **WHEN** the task-status projector receives them concurrently, **THEN** it emits at most one effective summary update, returns no exhausted-CAS handler error, and retains the correct final summary.
- **GIVEN** duplicated and orphaned legacy watch rows, **WHEN** upgrade migration runs after a snapshot, **THEN** it keeps one deterministic canonical row with the newest observed state, retains Review monitoring, removes only redundant/orphan rows transactionally, and a second run makes no change.
- **GIVEN** invalid GitHub credentials, **WHEN** workflow synchronization polls repeatedly, **THEN** it enters visible backoff and does not retry unchanged credentials continuously; a credential generation change resumes probing.
- **GIVEN** a multi-gigabyte fixture database, **WHEN** a client loads one history page, **THEN** the response is page-bounded, avoids eager large tool metadata, and records hydration latency.
- **GIVEN** a verified backup and a maintenance dry run, **WHEN** an operator requests retention cleanup, **THEN** Kandev reports candidates and estimated reclaim by table without deleting data; a subsequent explicit compact run can be rolled back from that backup.

## Success criteria

- The ten-minute multi-coordinator load test has no PR-watch creation loop, no recurring exhausted-CAS handler errors, and stable history/API latency.
- A canonical PR identity produces no more than one GitHub lookup per poll cycle.
- Maintenance output identifies reclaimable bytes for tool metadata/payloads, snapshots, and plan revisions before any destructive operation.

## Out of scope

- Increasing host hardware as a workaround.
- Disabling PR monitoring.
- Deleting human or agent conversation messages by default.
- Silent upgrade deletion beyond the documented canonical-watch migration.
- Retrofitting arbitrary external storage providers for tool payloads in this iteration.
