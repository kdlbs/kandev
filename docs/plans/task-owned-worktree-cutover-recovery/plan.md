---
spec: docs/specs/tasks/requirements/session-delete-resource-cleanup.md
created: 2026-08-09
status: implemented
---

# Fix Plan: Recover Task-Worktree Cutovers Without Blocking Users

## Overview

Repair the one-time task-owned-worktree cutover introduced by PR #2456. The
current migration treats historical session references and stale duplicate
representations as live ownership conflicts, so valid legacy databases refuse
startup. The repair keeps canonical `task_environment_repos` ownership,
ignores obsolete terminal/deleted references, preserves fail-closed behavior for
unresolved live ownership, and adds an upgrade regression fixture matching the
affected database.

Confirmed root cause: `loadLegacySessionWorktrees` loads deleted history and
`mergeSessionWorktree` reconciles it as an owner, while `mergeFlatEnv` rejects
path/branch drift even when the canonical repository row already owns the same
physical worktree. The failure occurs before the schema swap, so the existing
database is recoverable without destructive repair.

## Backend

### Migration source precedence

- Update `apps/backend/internal/task/repository/sqlite/worktree_ownership_normalize.go`
  and `worktree_ownership_targets.go` so deleted session rows and stale rows
  belonging to terminal sessions cannot create or override ownership when a
  canonical repository row exists.
- Preserve the canonical repository row's path and branch when the legacy flat
  environment carries the same physical worktree identity with stale metadata.
- Continue returning a diagnostic for non-terminal sessions or worktrees with
  no compatible canonical owner; do not silently choose between two live
  physical worktrees.

### Upgrade safety

- Keep the transaction, pre-upgrade snapshot, shadow-table validation, and
  rollback behavior unchanged.
- Ensure the repaired migration remains replay-safe after the legacy table is
  removed and does not touch filesystem or Git state.

## Tests

- **What:** A deleted historical session row beside a canonical repository row
  does not block cutover.
  **File:** `apps/backend/internal/task/repository/sqlite/worktree_ownership_migration_test.go`.
  **How:** Seed a legacy database with a canonical row, a terminal session,
  and a deleted session-worktree row; assert final schema and canonical owner.
- **What:** A stale flat environment path/branch does not override the
  canonical physical-worktree row.
  **File:** same migration test file.
  **How:** Seed matching worktree identity with differing legacy metadata and
  assert the canonical path/branch survives.
- **What:** A live unresolved conflict still fails closed and leaves legacy
  schema/data intact.
  **File:** same migration test file.
  **How:** Extend the existing conflicting-worktrees fixture with a
  non-terminal session and assert rollback.
- **What:** Terminal session references for `COMPLETED`, `FAILED`, and
  `CANCELLED` cannot override a canonical owner.
  **File:** same migration test file.
  **How:** Run the table-driven `TestCutover_IgnoresTerminalHistoricalSessionConflict`
  fixture and assert the canonical worktree remains selected for each state.
- **What:** An unexpected `CREATED` session-worktree reference without a
  canonical repository row is preserved for backfill.
  **File:** same migration test file.
  **How:** Run `TestCutover_PreservesCreatedSessionWorktreeWithoutCanonicalOwner`
  and assert the physical worktree is present after cutover.
- **What:** The full repository initializer can upgrade a representative
  legacy database.
  **File:** same migration test file.
  **How:** Run the package's focused cutover tests, then the backend repository
  test target required by the task.

## Public documentation

Update `docs/public/operations.md` and `docs/public/k8s.md` only after the code
repair is implemented. Document that one initializer must reach health, that a
failed pre-cutover startup leaves the database authoritative, and that rollback
uses a matching pre-upgrade binary plus backup. Do not instruct operators to
delete ownership rows manually.

## Implementation Waves And Parallel Candidates

Wave 1 — sequential:

- [x] [task-01-cutover-repair](task-01-cutover-repair.md) — done

Wave 2 — sequential:

- [x] [task-02-upgrade-docs](task-02-upgrade-docs.md) — done

## State classification

Use the existing session-state contract: `STARTING`, `RUNNING`, `IDLE`, and
`WAITING_FOR_INPUT` are open/resumable and must not be discarded as historical;
`COMPLETED`, `FAILED`, and `CANCELLED` are terminal for this migration. A
`CREATED` session should normally have no physical worktree reference, but the
test fixture must prove that an unexpected created-session reference is not
silently discarded without a canonical owner.

## Verification Results

- `GOCACHE=/tmp/kandev-go-cache go test ./internal/task/repository/sqlite -run
  'TestCutover_(CanonicalRepoWinsOverStaleFlatMetadata|IgnoresDeletedHistoricalSessionConflict)' -count=1` — passed.
- `GOCACHE=/tmp/kandev-go-cache go test ./internal/task/repository/sqlite -run
  'TestCutover_(IgnoresTerminalHistoricalSessionConflict|PreservesCreatedSessionWorktreeWithoutCanonicalOwner)' -count=1` — passed.
- `GOCACHE=/tmp/kandev-go-cache go test ./internal/task/repository/sqlite -run
  'TestCutover' -count=1` — passed.
- `GOCACHE=/tmp/kandev-go-cache go test ./internal/task/repository/sqlite -count=1` — passed.
- `make -C apps/backend test` — passed.
- `node scripts/validate-public-docs.mjs` — passed; 41 pages validated.
- `git diff --check` — passed.

## PR Fixup Results

- Remediation commit `44ffff81` added terminal-state coverage for
  `COMPLETED`, `FAILED`, and `CANCELLED`, the unexpected `CREATED` safety
  fixture, and cached historical ownership classification.
- Focused cutover tests passed after the remediation, and `git diff --check`
  passed.
- The actionable review thread was replied to and resolved after the cache
  fix; the exact-head unresolved thread count is zero.
- Exact-head PR state for `44ffff81`: checks snapshot complete, 4 passed, 0
  failed, 12 pending, 0 approval-required runs, 0 issue comments, and 0
  unresolved review threads. The PR is open and mergeable; GitHub reports
  `UNSTABLE` only because checks are still running.
- Pending checks are Backend (Windows), Backend Postgres, Backend Static
  Checks, Backend Tests (1/2), Backend Tests (2/2), five CodeQL analyses,
  `deploy-same-repo`, and the Cloudflare Pages preview deployment.
