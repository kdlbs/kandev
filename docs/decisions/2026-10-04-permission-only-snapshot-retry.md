# ADR-2026-10-04-permission-only-snapshot-retry: Bound permission-only snapshot retries

**Status:** proposed
**Date:** 2026-10-04
**Area:** backend

## Context

The recovery copier drops special mode bits and applies umask to new files.
The full-mode manifest then rejects otherwise identical content.
A persisted blocked record prevents the explicit relocation action from retrying
with a corrected copier. Removing that record also removes operation continuity.

## Decision

Preserve supported modes and the owner identity required by set-ID bits.
Refuse preservation if the host cannot retain that identity.
Never apply setuid to a copied executable under a different UID.

Permit one compatibility transition from a failed pre-restoration snapshot to
a new snapshot only through a new explicit, stamp-fenced relocation request.
Require a complete comparison proving permission-only differences and unchanged
source, replacement, inventory, owner generation, and exclusive authority.
Retain the same operation ID and both the original and old snapshot.
Persist retry provenance before creating the new snapshot.
Keep generic blocked adoption, automatic resume, and later-stage failures refused.

## Consequences

- Future snapshots retain supported permissions without a weaker integrity check.
- A proven affected relocation can continue through the existing repair action.
- Content drift still requires separate manual investigation.
- Compatibility proof reads the retained file trees once more and needs cancellation.
- Retained failed snapshots consume disk space under the existing retention contract.
- An unprivileged host can refuse a set-ID ownership change that it cannot preserve.
- Retry provenance extends the adjacent JSON record, with no database migration.

## Alternatives considered

- Ignore modes in the manifest. This loses filesystem behavior and conceals copy defects.
- Delete or reset every blocked record. This discards evidence and admits unrelated failures.
- Create a new operation for each retry. This breaks durable claim and replacement identity.
- Repair every snapshot on startup. This grants mutation without fresh relocation authorization.
- Copy set-ID bits without their owner identity. This changes the executable or directory privilege semantics.

## Related specifications

- [Preservation design](../specs/tasks/system-design/worktree-metadata-recovery.md)
- [Relocation requirements](../specs/tasks/requirements/managed-clone-relocation.md)
- [Relocation design](../specs/tasks/system-design/managed-clone-relocation.md)
- [Implementation plan](../plans/workspace-recovery-permissions/plan.md)
