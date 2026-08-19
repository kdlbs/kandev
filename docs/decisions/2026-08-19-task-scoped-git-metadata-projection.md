# ADR-2026-08-19-task-scoped-git-metadata-projection: Task-Scoped Git Metadata Projection

**Status:** accepted
**Date:** 2026-08-19
**Area:** backend, security, workflow

## Context

Task-owned linked worktrees need Git metadata outside their writable checkout. Granting the source repository `.git` directory makes ordinary commits work, but also exposes common configuration, refs, and sibling worktree administration to a task agent.

## Decision

The lifecycle derives an ephemeral, typed Git metadata projection from server-owned task repository and worktree records. The shared `internal/worktree` resolver validates the checkout's Git metadata and the executor compiles only the owned linked-worktree directory, object store, and current ref dependencies into its native policy. The common Git root and common `worktrees` directory are never broad writable roots. Unsupported executors or agent runtimes fail closed before launch.

## Consequences

All launch, resume, recovery, and attachment paths share one authorization boundary and preserve Git's native shared locks. Executors need explicit capability and attestation support, and policy refresh can require an idle child restart or container recreation.

## Alternatives Considered

Granting each source `.git` directory was rejected because it exposes unrelated source and sibling metadata. Replacing linked worktrees with independent clones was rejected because it changes task ownership and lock semantics, duplicates storage, and expands the repair into a materialization migration. A Git broker remains a possible executor-specific mediation fallback, not the default.
