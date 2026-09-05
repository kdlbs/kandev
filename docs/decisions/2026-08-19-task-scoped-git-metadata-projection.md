# ADR-2026-08-19-task-scoped-git-metadata-projection: Task-Scoped Git Metadata Projection

**Status:** accepted
**Date:** 2026-08-19
**Area:** backend, security, workflow

## Context

Task-owned linked worktrees need Git metadata outside their writable checkout. Granting the source repository `.git` directory makes ordinary commits work, but also exposes common configuration, refs, and sibling worktree administration to a task agent.

## Decision

The lifecycle derives an ephemeral, typed Git metadata projection from server-owned task repository and worktree records. The shared `internal/worktree` resolver validates the checkout's Git metadata and the executor compiles only the owned linked-worktree directory, object store, and current ref dependencies into its native policy. The common Git root and common `worktrees` directory are never broad writable roots. Unsupported executors or agent runtimes fail closed before launch.

Agent authority and executor mount plumbing are separate typed sets. Agent policy grants the exact active ref, reflog, and corresponding `.lock` paths. A container executor may bind their parent directories writable only as backing support for Git's native create-and-rename protocol, and only when it also installs the exact-path inner agent policy. Mount-support paths never become agent filesystem-policy rules. If either layer cannot be attested, launch fails closed.

## Consequences

All launch, resume, recovery, and attachment paths share one authorization boundary and preserve Git's native shared locks. Executors need explicit capability and attestation support, and policy refresh can require an idle child restart or container recreation. Layered containers carry a writable backing mount that is intentionally broader than the agent rule, so bypassing or omitting the inner policy is unsupported rather than a degraded mode.

## Alternatives Considered

Granting each source `.git` directory was rejected because it exposes unrelated source and sibling metadata. Replacing linked worktrees with independent clones was rejected because it changes task ownership and lock semantics, duplicates storage, and expands the repair into a materialization migration. A Git broker remains a possible executor-specific mediation fallback, not the default.
