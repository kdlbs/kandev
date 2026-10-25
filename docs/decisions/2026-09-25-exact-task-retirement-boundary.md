# ADR-2026-09-25-exact-task-retirement-boundary: Isolate exact task retirement

**Status:** accepted
**Date:** 2026-09-25
**Area:** backend

## Context

Ordinary task archive and deletion may clean worktrees through broad recovery
paths. A damaged old task may be retired only after a specific replacement has
accepted independently verified preservation evidence.

## Decision

Provide a dedicated read-only preview and later fenced exact-retirement
operation. Every evidence source is a closed predicate; missing or contradictory
evidence blocks. The operation cannot call normal task delete/archive cleanup,
force-remove a worktree, prune a repository, or infer success from absence.

## Consequences

The operation can leave an old task intentionally retained while operators fix
evidence. It needs separate adapters and a durable ledger for cross-store
recovery, but it gives reviewers auditable isolation from replacement and
foreign resources.

## Alternatives Considered

- Add an override to ordinary archive/delete. Rejected because it reuses broad
  cleanup authority.
- Use operator database and Git commands. Rejected because it cannot provide
  shared authorization, replay protection, or crash recovery.
