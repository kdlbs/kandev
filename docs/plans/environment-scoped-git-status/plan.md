---
created: 2026-08-31
status: completed
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
system_design:
  - ../../specs/tasks/system-design/environment-owned-git-status.md
legacy_specs: []
---

# Implementation Plan: Environment-Scoped Git Status

## Overview

The Changes store is environment-keyed, but snapshot persistence and delivery
still use session identity. This mismatch lets an old sibling snapshot replace
a newer workspace observation.

The change moves snapshot authority to `TaskEnvironment`. It then carries that
identity through capture, hydration, WebSocket delivery, and frontend storage.

The implementation order starts with the schema and repository contract. The
capture, delivery, and frontend work depend on that contract.

The completed [projection-time repair](../environment-owned-git-status/plan.md)
remains a historical delivery record. This plan replaces its session-scoped
persistence boundary.

## Confirmed root cause

`task_session_git_snapshots` partitions and ranks current status by
`session_id`. Its rank gives an older `agent_completed` row priority over a
newer `live_monitor` row.

Sibling sessions can share one task environment. Each sibling therefore has a
different persisted winner for the same workspace.

Boot hydration publishes each winner with a session ID. The frontend maps each
session to one environment key, so the last sibling message replaces the
earlier message.

The fault needs no session deletion. Two live sibling records in one
environment are sufficient.

## Scope

### In scope

- Rebuild Git-snapshot persistence around `task_environment_id`.
- Keep `session_id` as nullable capture provenance.
- Select one current observation for each environment and repository.
- Make completion supersession and live upsert environment-scoped.
- Keep live agentctl status authoritative for a live environment.
- Deliver task environment and repository identity in each status payload.
- Apply status directly to the environment-keyed frontend store.
- Adapt task summaries and direct snapshot-table consumers.
- Add SQLite, Postgres, backend, frontend, and browser regressions.

### Out of scope

- Changes to agentctl polling or Git command behavior.
- Changes to the Changes panel layout, navigation, touch behavior, or copy.
- Environment-scoping of session commits or other Git event types.
- A rename of `task_session_git_snapshots`.
- Cleanup of the completed projection-time delivery record.

## Technical approach

### Persistence and migration

Add `task_environment_id TEXT NOT NULL` to the final snapshot shape. Change
`session_id` to a nullable foreign key with `ON DELETE SET NULL`.

Use a transactional shadow-table cutover for existing databases. Backfill the
environment from `task_sessions.task_environment_id` and remove unresolved
rows.

Partition legacy rows by environment and normalized repository. Keep the
newest row by `created_at`, file-detail tie-break, and `id`.

Create environment-based indexes after the table swap. Keep a session index
for provenance reads.

### Repository selection and writes

Replace session-ranked current-status methods with environment-ranked methods.
Keep explicit session-history methods for history and code statistics.

The current selector returns one row per environment and repository. It never
uses an older detailed row after a newer sparse row.

Use one environment lock for live upsert and completion supersession. This
lock makes the delete and insert sequence safe on Postgres.

### Capture and live events

Add `TaskEnvironmentID` to `models.GitSnapshot` and Git status event payloads.
Every status write must have a resolved environment.

Key the live-write throttle by environment and repository. Keep the session ID
only for logs and provenance.

An agent completion removes earlier live and completion rows for its
environment and repository in the insert transaction.

### Hydration and summaries

Hydration detects live executions by environment. If one exists, persisted
status cannot replace a failed live query.

If no execution exists, hydration reads the environment selector. It sends one
payload for each repository.

Task-card and summary rebuilds deduplicate environment IDs before snapshot
reads. Delivery ancestry reads environment ownership instead of capture-session
ownership.

### Frontend identity

Add required `task_environment_id` to `GitStatusUpdateEvent`. Ignore a status
event that does not contain the field.

Pass the delivered environment ID directly to `setGitStatus`. Do not map the
status event through `environmentIdBySessionId`.

Keep timestamp ordering for response races. Normalize missing `files` to an
empty object so a sparse live observation removes stale details.

## Tests

- `AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9`: migration and repository
  tests prove that session deletion keeps environment status.
- `AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9`: repository tests seed an
  older detailed sibling row and a newer sparse row. The newer row wins.
- `AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5`: backend hydration tests
  prove one environment result across sibling, inherited, and shared sessions.
- Both criteria: frontend tests prove that the delivered environment ID is the
  only status key.

## E2E tests

Extend `apps/web/e2e/tests/session/session-tab-management.spec.ts`.

The test creates two sessions in one environment. A newer sparse observation
removes files that an older observation contained.

The test reloads the task and waits for both sibling hydration messages.
Changes must not restore the removed files.

This flow covers both acceptance criteria in Chromium. The mobile surface uses
the same store and has no presentation change.

## Work orders

- [x] [Task 01: Migrate snapshot ownership](task-01-migrate-snapshot-ownership.md)
- [x] [Task 02: Scope snapshot capture](task-02-scope-snapshot-capture.md)
- [x] [Task 03: Deliver environment status](task-03-deliver-environment-status.md)
- [x] [Task 04: Apply delivered environment identity](task-04-apply-environment-identity.md)

Tasks run in this order. The schema, event contract, and frontend payload
contract prevent safe parallel implementation.

## Verification results

- Task 01 passed the SQLite repository and cutover tests. Postgres parity tests
  skip when no DSN is configured.
- Task 02 passed 10 focused orchestrator and lifecycle tests.
- Task 03 passed 4 environment-focused tests and 2,348 tests in the affected
  backend packages.
- Task 04 passed 26 focused frontend tests, web typecheck, the E2E build, and
  the targeted Chromium regression.
- The final seven-package backend run passed 7,537 tests.
- PR fixup passed 7,624 tests across eight affected backend packages. It also
  verified owner-environment summary hydration without sessions, delivery
  reads after session deletion, recovered live status identity, frontend
  typecheck and lint, and the targeted Chromium regression.
- The migration-order fix carried existing environment-owned snapshots through
  the worktree shadow swap, rehomed snapshots from losing environments, and
  rebound the Postgres foreign key to the shadow parent without cascading
  deletion. The final affected-package run passed 7,749 tests across nine
  backend packages, and backend lint reported 0 issues.

## Risks

- The table has direct readers in delivery and analytics packages. Each reader
  must use environment scope or nullable session provenance intentionally.
- A table rebuild can remove data after a partial cutover. Failure-injection
  tests must prove transaction rollback at every swap step.
- A sparse live row contains no file-detail map. Frontend normalization must
  clear old details without removing current totals.
- Shared environments can cross task boundaries. Summary and delivery joins
  must use the environment binding instead of `task_environments.task_id` only.
- Postgres permits concurrent writers. The environment lock must cover both
  live upsert and completion supersession.
