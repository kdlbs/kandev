---
spec: docs/specs/platform/pr-watch-and-storage-bounds.md
created: 2026-08-31
status: draft
---

# Implementation Plan: Canonical PR Monitoring and Bounded Task Storage

## Overview

Replace session-scoped PR-watch ownership with task/repository canonical identities, migrate legacy data safely, and prevent unchanged reconciliation and PR status from producing writes or event fan-out. Then make summary projection resilient to real cross-writer contention and make GitHub authentication failures visibly back off.

Bound operational storage without deleting normal conversation history: separate heavy tool payloads from normal hydration, deduplicate snapshots, add explicit retention maintenance with verified backups and safe SQLite compaction, and expose the relevant health signals. This follows [ADR-2026-08-31-task-owned-pr-watch-identity](../../decisions/2026-08-31-task-owned-pr-watch-identity.md) and the existing [ADR 0045](../../decisions/0045-install-wide-storage-maintenance.md).

## Confirmed root cause

`github_pr_watches` is currently unique by `(session_id, repository_id, branch)`, and `Service.EnsurePRWatch` looks up the same session-level identity. `Poller.reconcileWatches` iterates session-level branch candidates while `refreshStaleBranches` resolves branch by task/session. A resumed or parallel session can therefore create its own row and drive duplicate polling even though `ListActivePRWatches` selects all non-archived task watches.

The task-status projector has an in-process per-task lock and semantic no-op check, but its fixed three-attempt CAS loop returns a handler error when another writer repeatedly wins. Duplicate PR events make that pressure much more likely. Message pagination exists in the repository/service, but legacy hydration paths still use full message reads and load inline metadata. Git snapshots and plan revisions have no equivalent payload-dedup/retention maintenance policy.

## Backend

### Canonical PR-watch schema and migration

Update `apps/backend/internal/github/store.go` and its migration helpers to replace the session key with canonical searching/discovered identities. Preserve `session_id` as provenance, validate task/repository ownership in active queries, and add transactional migration that snapshots first, resolves duplicates deterministically, preserves freshest status/timestamps, removes orphans, logs safe counts, and is idempotent.

Use the persistence snapshot implementation in `apps/backend/internal/persistence/snapshot.go`; do not introduce another backup mechanism. Reconcile existing multi-repository backfills and task cleanup behavior with the new lifetime contract.

### Reconciliation, polling, and events

Change `TaskBranchProvider` and `Poller` so branch resolution has one desired result per task/repository. Replace session-level ensure/reset operations with atomic task-level transitions. Batch/check only canonical watches and gate `GitHubPRFeedback` plus `GitHubTaskPRUpdated` publication on a durable, semantic state fingerprint that includes task, repository, PR, head SHA, and exposed status fields.

Classify GitHub auth/configuration failures separately from transient provider/rate-limit failures. Persist/report degraded integration state and use generation-aware exponential backoff/circuit reopening when credentials/configuration changes.

### Task status and observability

Extend `internal/task/statussummary` with per-task coalescing/single-flight pending refreshes and a contention policy: reload/rebase authoritative state, jittered bounded backoff for a real CAS race, and semantic success for equivalent pending work. Keep the existing revision contract in `docs/specs/platform/bounded-task-status-delivery.md`.

Add metrics/structured health through the existing health/debug metric wiring for canonical/searching/duplicate/orphan watch counts, per-canonical PR requests, CAS retries/exhaustion, event-handler failures, storage sizes, SQLite/WAL size, history hydration latency, queue depth, and active runtime count. Metric labels must not include credentials, branches with sensitive content, or unbounded IDs.

### Bounded storage and history hydration

Introduce a repository-owned heavy tool-payload representation, digesting and compressing/externalizing payloads above a configured threshold. Return a lightweight metadata projection by default and add explicit, scoped detail retrieval. Update all normal session hydration callers to cursor pagination; preserve stable ordering and authorization.

Deduplicate equivalent snapshot writes using a stable content fingerprint, and add retention selection for redundant snapshots, superseded tool payloads, and obsolete plan revisions. Retention is opt-in through maintenance and never targets ordinary human/agent messages.

### Operator maintenance and documentation

Add the database-maintenance command to the native Go launcher/backend command surface, reusing `internal/system/storage` ownership and `persistence.SnapshotSQLite`. It must plan/dry-run estimates by table, verify or create a backup, perform deletions transactionally, compact into a staged file, atomically replace only after validation, and print exact rollback instructions.

Document the operator workflow under `docs/public/**`, including backup of `kandev.db` and `master.key`, dry-run review, execute/compact, rollback, and post-release verification. Update relevant public configuration/operations references and backend guidance if command or metrics conventions change.

## Frontend

No new product interaction is required for the first iteration. If existing System health/status surfaces expose the new degradation and database-maintenance telemetry, add only localized status presentation following the current System-page patterns; otherwise the command and structured health contract are the supported operator interface.

## Tests

- **Canonical identity/migration:** `apps/backend/internal/github/store_*_test.go` and `service_pr_watch*_test.go` use duplicated legacy records, multiple sessions, branch changes, multi-repository tasks, and completed provenance sessions to prove uniqueness, idempotency, and Review monitoring.
- **Polling/events/auth:** `apps/backend/internal/github/poller_test.go`, batched-watch tests, and workflow-sync/GitHub health tests count GitHub calls/events and prove unchanged cycles are write-free, duplicate PR state emits once, and invalid credentials back off until generation changes.
- **Status projector:** `apps/backend/internal/task/statussummary/projector*_test.go` drives concurrent equal/different PR events and competing CAS writers; it asserts no exhausted retry handler error, monotonic revision, and correct final authoritative summary.
- **Storage/history:** task SQLite repository/service/controller tests build a large fixture, assert cursor-bounded lightweight history reads and explicit payload retrieval, digest deduplication, retention selection, and preservation of ordinary messages.
- **Maintenance:** command/system/persistence tests assert dry run makes no write, backup precondition fails closed, per-table estimates are stable, transaction rollback preserves data, staged compaction validates and atomically swaps, and the documented backup restores the fixture.
- **Load validation:** a deterministic multi-coordinator/task integration fixture runs ten simulated minutes and asserts no watch creation loop, no recurring projector failure, bounded lookup count, and stable handler latency.

## E2E Tests

No browser E2E is required unless task 06 adds a System-page health UI. In that case, add a desktop and mobile Playwright scenario that verifies the localized degraded GitHub state and links to the non-destructive maintenance instructions; run `mobile-parity` before implementing that UI task.

## Verification Results

Pending.

## Implementation Waves And Parallel Candidates

Wave 1 (sequential, shared schema and state contracts):

- [x] [task-01-canonical-watch-migration](task-01-canonical-watch-migration.md)
- [x] [task-02-idempotent-polling-events](task-02-idempotent-polling-events.md)
- [x] [task-03-contention-safe-projection](task-03-contention-safe-projection.md)

Wave 2 (sequential, depends on stable event/storage contracts):

- [x] [task-04-github-auth-backoff-observability](task-04-github-auth-backoff-observability.md)
- [x] [task-05-bounded-history-storage](task-05-bounded-history-storage.md)

Wave 3 (sequential, destructive operator surface depends on retention implementation):

- [ ] [task-06-database-maintenance-command](task-06-database-maintenance-command.md)
- [ ] [task-07-operator-docs-load-validation](task-07-operator-docs-load-validation.md)

No task is parallel-safe: the watch schema, projector/event semantics, storage representation, and maintenance command all share persistence and operation contracts.

## Risks

- Changing unique constraints on SQLite requires a table rebuild and careful preservation of all watch columns; migration must not assume completed sessions are disposable.
- A canonical discovered-watch uniqueness constraint needs an explicit searching-row representation so multiple branches remain possible before discovery.
- Heavy tool payload externalization must preserve authorization, integrity verification, backup behavior, and existing transcript/export consumers.
- Atomic compaction needs sufficient free storage for a staged copy and must coordinate database pools/WAL; failures must leave the live database usable.
- Exact retention defaults are intentionally operator-configured and disabled for destructive execution until an explicit maintenance run. The command must print its effective policies before it acts.

## Out of scope

- Automatic deletion of normal conversation messages.
- Hardware resizing or a container restart as a remediation.
- Disabling GitHub PR monitoring.
- A new third-party object-store integration for payloads.
