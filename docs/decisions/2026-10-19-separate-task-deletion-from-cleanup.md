# ADR-2026-10-19-separate-task-deletion-from-cleanup: Separate task deletion from physical cleanup

**Status:** accepted
**Date:** 2026-10-19
**Area:** backend, frontend, protocol

## Context

The keep-files option removes unusable tasks but does not fix normal deletion.
An empty ownership marker blocks preview and service admission. Damaged Git
metadata can block cleanup snapshot preparation. The user requested that normal
deletion also work in this state while keeping the independent retain option.

## Decision

Explicit consent to unknown workspace changes can admit normal task deletion
after its database inventory and cleanup obligations are durably saved. A
physical inspection failure blocks the affected destructive effect, not the
already authorized task-record deletion. Independent verified effects can
proceed. Pending and failed cleanup inventory remains protected from unrelated
collectors and survives task deletion and restart.

The existing cleanup worker owns retries and diagnostics. Normal deletion
continues to authorize safe cleanup. Retain mode continues to prohibit automatic
workspace cleanup. A physical error never silently selects retain mode.
Ownership and physical identity remain required for destructive I/O, including
on later retries. A new directory at an old path is not proof of identity.

This amends the normal-deletion admission rule in
[the retain-mode decision](2026-10-16-retain-resources-on-task-delete.md).
It preserves that decision's retention guarantees and the
[fail-closed GC boundary](0009-fail-closed-gc-semantics.md).
The [task deletion recovery design](../specs/tasks/system-design/task-delete-recovery.md)
defines consent, durable progress, protection, and committed outcomes.

## Consequences

Users can remove broken tasks without opting to keep every workspace forever.
Healthy independent resources can be reclaimed. Some damaged resources can
remain pending or failed until safe cleanup evidence is restored. Kandev must
report this accurately and keep the evidence after deleting task rows.
Database and authorization failures still block task deletion. A generic force
flag, guessed ownership, broad recursive deletion, and automatic Git repair
are not authorized by consent to unknown local changes.

## Alternatives considered

- **Only keep files:** Useful and retained, but does not satisfy the requested
  normal-delete behavior or reclaim healthy resources in a mixed selection.
- **Ignore all inspection failures:** Can delete another task's or a replacement
  checkout. Rejected because user consent is not ownership evidence.
- **Repair metadata before deleting:** Makes task removal depend on filesystem
  health and can replace authentic identity with a guess. Not a prerequisite.
- **Delete rows and log cleanup failures:** Loses resource ownership and retry
  evidence. Rejected in favor of the existing durable cleanup owner.
