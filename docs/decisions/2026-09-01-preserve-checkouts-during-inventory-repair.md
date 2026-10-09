# ADR-2026-09-01-preserve-checkouts-during-inventory-repair: Preserve checkouts during inventory repair

**Status:** accepted
**Date:** 2026-09-01
**Area:** backend, protocol, security, operations

## Context

A ready task environment can retain a valid Git checkout while its canonical
`task_environment_repos` row is missing or stale. Normal session resume then
fails the workspace-reuse inventory guard. Falling through to ordinary
materialization can discard the only durable pointer to dirty or untracked
work, while relaxing the guard can attach a session to the wrong repository,
branch, task, or workspace.

The accepted task-owned-worktree model requires an explicit repair path. The
path must distinguish server-owned metadata drift from filesystem damage or an
ambiguous ownership graph.

## Decision

Keep `validateReuseEnvironmentInventory` as the admission guard. A missing or
mismatched repository slot never authorizes materialization, cleanup, reset,
or branch creation.

A server-authorized task-scoped recovery action may repair one
`task_environment_repos` slot only after Kandev proves one reciprocal identity
from the task, workspace, task repository, task environment, session, runtime
snapshot, canonical repository path, and Git worktree metadata. Caller-supplied
paths, repository identities, branches, and environment identities are not
authority. A competing live writer, a symlink mismatch, cross-scope ownership,
duplicate candidates, or incomplete proof denies repair without mutation.

Before mutation, Kandev captures a preservation receipt containing non-secret
identity revisions, Git HEAD and ref containment, worktree status and content
hashes, and runtime state. One transaction inserts or updates only the proven
environment-repository row and appends the receipt. A task-scoped idempotency
key and request hash return the existing receipt for the same request and
conflict for a different request. The transaction uses expected revisions so a
concurrent metadata change fails closed.

After a committed repair, the orchestrator may make one resume or start
attempt under its existing per-task/session concurrency guard. A later launch
failure does not roll back the metadata repair, mutate the checkout, or replay
the transaction.

## Consequences

- Dirty, staged, and untracked content remains byte-for-byte unchanged.
- Operators receive stable result codes and an auditable, non-secret receipt.
- Missing or stale metadata becomes recoverable without weakening ordinary
  workspace admission.
- Remote and container-only workspaces remain manual recovery cases until an
  executor can provide equivalent reciprocal proof.
- The recovery table and idempotency contract become durable lifecycle state
  and must remain replayable on SQLite and PostgreSQL.

## Alternatives Considered

1. **Fall through to normal materialization.** Rejected because it can create a
   second checkout or discard the only path to unique workspace state.
2. **Trust a caller-provided path or repository tuple.** Rejected because it
   turns a task-scoped repair into a cross-workspace filesystem capability.
3. **Scan the tasks root for a plausible checkout.** Rejected because broad
   discovery is ambiguous and leaks unrelated workspace identity.
4. **Update the row without a durable receipt.** Rejected because operators
   could not prove what state was preserved or distinguish a retry from a new
   mutation.
5. **Clean or reset before validation.** Rejected because repair exists to
   preserve unique state, including dirty and untracked files.
