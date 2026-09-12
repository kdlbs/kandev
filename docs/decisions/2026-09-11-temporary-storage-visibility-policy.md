# ADR-2026-09-11-temporary-storage-visibility-policy: Separate temporary storage visibility from cleanup ownership

**Status:** accepted
**Date:** 2026-09-11
**Area:** backend, frontend, security

## Context

Before this decision, storage analysis omitted unregistered temporary files. Operators could not see
shared `/tmp` pressure in its classified categories, and scheduled runs always skipped the existing
artifact provider.

## Decision

Measure resolved service temporary folders read-only, including a distinct Unix `/tmp`.
Show this footprint separately from Total counted because it overlaps classified resources.
Do not infer cleanup authority from a path appearing in analysis.

Add an off-by-default policy for scheduled cleanup of registered Kandev temporary artifacts.
Keep explicit cleanup available independently of that option.
Preserve matching registry markers, inactive lifecycle, the 24-hour stale interval, and recoverable quarantine.
Keep unregistered files and legacy roots outside cleanup authority.

This decision amends only the manual-only restriction in
[the owned-artifact decision](2026-08-08-owned-temp-artifact-cleanup.md). That decision remains
the authority for artifact ownership, lifecycle checks, and quarantine safety.

## Consequences

Operators gain broader visibility and recurring cleanup of verified artifacts.
The general temporary footprint is informational and cannot be added to classified totals.
Large shared caches can remain visible without becoming removable through Kandev.
Cross-filesystem quarantine still fails without removing the source.
Quarantine moves bytes but frees no space until permanent deletion.

## Alternatives Considered

- Broad age-based cleanup: rejected because age and names do not prove inactivity or ownership.
- Automatic adoption of legacy roots: rejected because those roots lack trustworthy lifecycle records.
- Keep all temporary storage manual-only: rejected because registered artifacts already have a constrained ownership model.
- Add every temporary byte to Total counted: rejected because configured storage roots can overlap.
- Add configurable paths or shell cleanup commands: excluded because that creates a separate host administration contract.

## Related specification

[Temporary storage design](../specs/system-page/system-design/storage-temporary-folders.md)
maps the implemented behavior to requirements and completed work orders.
