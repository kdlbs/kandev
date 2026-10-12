# ADR-2026-10-05-managed-clone-recovery-operation-storage: Own recovery artifacts and progress on the server

**Status:** proposed
**Date:** 2026-10-05
**Area:** backend

## Context

Clone relocation creates adjacent snapshots and journals inside a task root.
Those files crowd a multi-repository Files view. A browser-local pending map
also loses progress on reload. The original request can continue after its
browser disconnects, leaving a second page offering the same repair action.

Existing claims establish exclusive authority. They do not prove runner liveness
or provide a durable terminal result. Claims are released after recovery.
Retained copies remain subject to the existing preservation contract.

## Decision

Keep new clone-relocation artifacts in the existing private recovery namespace
outside task roots. Persist their operation and slot identity before mutation.
Retain versioned readers for adjacent legacy records without moving live locks.
Only verified, recorded artifacts can be excluded from workspace presentation.

The task environment owns a durable latest-operation projection. Keep that
projection separate from exclusive claims and detailed filesystem journals.
Expose it through session hydration and notifications. Verify runner liveness
before projecting a persisted operation as running after a backend restart.

Use repository names as display labels. Keep canonical filesystem paths unchanged.
Retain existing file preservation, claim fencing, and all-slot admission rules.
Do not add automatic backup deletion or recovery retries.

## Consequences

- New artifacts no longer crowd working files.
- Legacy artifact names remain readable without a destructive upgrade migration.
- Progress survives browser loss and terminal status survives claim release.
- Operation revisions and ownership fences add database and projection work.
- Unknown legacy artifacts remain visible until their ownership can be verified.
- Retained backups still consume disk space; retention policy remains separate.

## Alternatives considered

- Hide every recovery-looking name in React. This can hide user files and leaves
  workspace search inconsistent with Files.
- Store progress only in browser storage. Other browsers and backend restarts
  cannot determine the current operation from that state.
- Treat a claim or journal timestamp as proof of running work. Abandoned records
  would appear active indefinitely.
- Rename active checkouts to remove suffixes. This changes paths used by runtimes,
  Git registration, editors, and recovery journals without improving preservation.
- Delete or bulk-move old backups on upgrade. This introduces new retention and
  crash-recovery risks without explicit authority.

## Related specifications

- [Experience requirements](../specs/tasks/requirements/managed-clone-relocation-experience.md)
- [Experience design](../specs/tasks/system-design/managed-clone-relocation-experience.md)
- [Preservation boundary](2026-09-27-managed-clone-relocation-boundary.md)
- [Implementation plan](../plans/managed-clone-recovery-experience/plan.md)
