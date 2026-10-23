# ADR-2026-10-23-attested-worktree-branch-continuity: Separate repair evidence from current branch identity

**Status:** accepted
**Date:** 2026-10-23
**Area:** backend, operations

## Context

A completed inventory repair records the checkout branch before and after its
metadata transaction. Normal Git operations can later rename or switch that
branch. Kandev updates the canonical environment row with the observed branch.
Resume currently rejects this valid change because it compares the row against
the historical repair branch forever.

The original [preservation decision](2026-09-01-preserve-checkouts-during-inventory-repair.md)
requires immutable repair identity and proof that repair preserved the checkout.
That requirement must survive normal work after the repair.

## Decision

A completed positive repair receipt proves a historical operation. Current
admission separately proves that the same owned physical checkout remains valid.
Allow branch evolution only when both proofs succeed. Keep exact original
branch matching for incomplete repair attestation and explicit repair retries.

For completed repairs, each admission requires a current reciprocal Git proof
within existing launch admission. It performs no repair or attestation write
and adds no environment-wide lock. Preserve task, workspace,
repository-slot, worktree, and canonical-path boundaries. Preserve the original
receipt bytes, request hash, and attestation. Current observations never replace
historical observations.

Apply the same rule to already-persisted positive receipts. Do not introduce a
branch-event ledger or require an unavailable historical event to recover an
otherwise provable checkout. Current inventory alone is insufficient: its branch
must agree with the actual registered checkout.

## Consequences

- Normal branch changes no longer permanently block repaired workspaces.
- A Git identity inspection is required for every completed repaired row. It
  does not scan file content or the staged index, even after branch changes.
- Missing, negative, contradictory, or cross-owner evidence remains blocked.
- Recovery needs no receipt migration and never changes local files or refs.
- This decision does not settle unknown prompt outcomes or authorize deployment.

## Alternatives considered

- Rewrite receipts whenever the branch changes. Rejected because it replaces
  historical preservation evidence and complicates idempotency.
- Remove branch checks entirely. Rejected because stale or forged current
  inventory could select the wrong branch.
- Persist a new branch-transition ledger. Deferred because current reciprocal
  identity already proves the supported outcome, including existing incidents.
- Reset to the original branch or delete the receipt. Rejected because either
  changes user work or removes the audit record.

## Related documents

- [Requirements](../specs/agents/requirements/workspace-inventory-repair.md)
- [System design](../specs/agents/system-design/workspace-inventory-repair.md)
- [Fix package](../plans/attested-worktree-branch-continuity/plan.md)
