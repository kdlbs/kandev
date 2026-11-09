# ADR-2026-11-08-worktree-refresh-lock-retries: Bounded worktree refresh lock retries

**Status:** accepted
**Date:** 2026-11-08
**Area:** backend, operations

## Context

A required PR refresh stopped after one Git lock failure.
The failed fast-forward updated the index and files but left the branch ref unchanged.
A disposable Git reproduction showed the same state and recovered through a normal fast-forward retry.

The platform prohibits generic retries after authentication or service failure.
Known lock contention needs a narrower rule without weakening required refresh or credential selection.
Delivery is complete; the [plan](../plans/git-refresh-lock-recovery/plan.md) records the results.

## Decision

Workspaces owns a bounded retry policy for its existing backend base-fetch and fast-forward-pull commands.
Every attempt uses the shared classified Git runner and the selected credential route.
The first admitted attempt starts one execution budget for the operation.
Only recognized lock contention permits another attempt.

Normal Git re-execution handles a partial fast-forward.
Kandev does not infer that a failed command left the checkout unchanged.
It does not remove unknown locks, reset local content, or force-update refs.

This rule amends the platform retry restriction only for this workspace operation.
It does not authorize authentication retries, transport fallback, or broader session restarts.
The [recovery design](../specs/workspaces/system-design/worktree-base-refresh-recovery.md) defines the scope and limits.

## Consequences

Brief contention can recover within the existing launch attempt.
Persistent contention remains a visible preparation failure with a retry instruction.
The existing required-refresh and local-first decisions remain authoritative.
Existing repository mutexes remain useful, but external Git writers can still cause contention.

## Alternatives Considered

- Fail immediately: retains the reported failure despite a short-lived lock.
- Retry every Git failure: obscures permanent errors and expands authentication behavior.
- Remove old lock files automatically: file age cannot establish ownership or exclude another writer.
- Add a global lock manager or repair journal: expands the system beyond the demonstrated failure.
- Replace refresh with a new fetch-only algorithm: changes base selection and checkout behavior beyond this repair.
