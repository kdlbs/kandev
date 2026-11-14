---
status: current
system: agents
requirements:
  - REQ-AGENTS-RETAINED-CHECKOUT-001
---
# Retained checkout preparation reconciliation system design

## Assigned-path reuse

After the normal session/worktree-ID lookup and before materialization refresh,
Manager.Create checks only the task/repository path derived from its existing
request and path rules. An absent path follows ordinary preparation. PR heads
and remote contributions keep their existing materialization contracts.

The existing target-path reservation covers inspection and registration.
Existing no-follow path handling and linked-worktree inspection retain their
roles. Repository membership compares filesystem IDs of the Git common
directories with os.SameFile; symbolic branch inspection compares the exact
requested name. No content or commit fingerprint is computed or compared.

An existing ownership marker is read without adding a new marker. A conflicting
task identity is rejected; absence alone is not called foreign. The existing
task worktree inventory identifies any row already owning the assigned path.
Its task/repository/branch slot must match uniquely, and its ID is retained.
When the request names an environment, the registered environment must match;
a mismatch returns the existing typed recovery error without changing the row.
An unregistered checkout needs an explicit CheckoutBranch; a generated branch
prefix is never used as ownership or branch evidence.

## Normal registration without checkout cleanup

A matching unregistered checkout is represented through buildWorktreeRecord
and the existing Store.CreateWorktree. The store resolves the session's
environment and keeps cleanup barriers, recovery claims and repository-slot
identity checks in its existing transaction. Environment-not-yet-resolved
handling remains the normal initial-materialization contract: the executor
registers the returned worktree as part of its subsequent environment write.

The fresh-checkout persistWorktree cleanup wrapper is not invoked: its error
compensation removes a newly created directory, while this directory predates
the request. Registration failure is returned with the retained checkout
untouched. Successful registration uses the ordinary session/repository/branch
cache key. No new receipt, owner claim or metadata-repair loop is introduced.

## Rollback ownership and canonical holder eligibility

Only a physical creation callback marks a worktree as created by this request.
Successful recreation of a missing registered directory emits it after the
record is persisted and cached. Retained registration never emits it.
Multi-repository rollback uses
that explicit result, including partially completed creation, so later failure
removes new directories and preserves retained checkout contents and identity.

Canonical slot selection uses the same active inventory eligibility as reuse,
including deletion, failed status and worktree presence, with exact workspace
placement. Multiple eligible holders or a sibling slot collision remain unsafe.

## Preserved outcomes

Reuse does not fetch, create or reset a branch, rerun setup, copy configured
files, remove an occupant or prune registrations. Existing checkout options
remain checked through the same gitcheckout.Check used by ordinary reuse.
After that check succeeds, retained reuse invokes the existing contribution
destination configuration, including its original remote-conflict errors.
A different repository/branch, an invalid checkout or a regular main checkout
is retained and reported as a typed recovery error. Unavailable evidence is
reported distinctly from a proved mismatch. Managed-main relocation and the
separate inventory-repair recovery contract are unchanged.


## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-RETAINED-CHECKOUT-001 | Assigned-path reuse; Normal registration without checkout cleanup; Preserved outcomes |
