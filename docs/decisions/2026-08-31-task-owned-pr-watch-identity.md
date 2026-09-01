# ADR-2026-08-31-task-owned-pr-watch-identity: Keep PR watches task owned

**Status:** accepted
**Date:** 2026-08-31
**Area:** backend, GitHub, workflow

## Context

Agent sessions are attempts within a task, not independent ownership domains for a pull request. A session-scoped PR-watch key lets resumed or concurrent sessions create duplicate watches for the same task branch. Each duplicate polls GitHub and emits equivalent events, which creates unnecessary task-status projection contention.

Review work can outlive the session that initially discovered the pull request, so deleting watches solely because their provenance session has completed would stop required monitoring.

## Decision

`github_pr_watches` is task-owned. A searching watch is canonical by `(task_id, repository_id, branch)` and a discovered watch is canonical by `(task_id, repository_id, pr_number)`. `session_id` remains optional provenance only and must not participate in canonical identity, polling fan-out, or task-status publication.

The active-watch query admits watches for existing, non-archived tasks, including Review tasks whose originating session is complete, and excludes watches without a valid task/repository relationship. Reconciliation resolves one desired branch per task/repository and performs an atomic transition only when it differs from the canonical searching watch. Polling and publication coalesce state by task/repository/PR and observable PR state.

Upgrade cleanup is a one-time, idempotent, transactional migration. It requires a database snapshot before destructive work, retains a deterministic canonical row, prefers discovered rows, preserves the newest status and feedback watermarks, and removes only duplicate or orphaned watch rows.

## Consequences

- Multiple historical or concurrent sessions no longer multiply GitHub requests, feedback events, or status-summary work.
- A watch is retained based on task lifecycle instead of session completion, preserving Review monitoring.
- Session-level callers must be migrated to task/repository identity and cannot rely on a separate watch per session.
- Legacy duplicate cleanup has a documented backup and rollback requirement.

## Alternatives Considered

- **Keep session-scoped watches and deduplicate only at polling time.** Rejected because duplicate persistent rows still create reconciliation writes and every future consumer must reproduce deduplication.
- **Delete every completed-session watch.** Rejected because a Review task can legitimately require monitoring after its original session finishes.
- **Use only `(task_id, repository_id)` as the key.** Rejected because multi-branch tasks require independent searching watches until a PR is discovered.
