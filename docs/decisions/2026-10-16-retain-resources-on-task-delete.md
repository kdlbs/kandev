# ADR-2026-10-16-retain-resources-on-task-delete: Retain resources during task deletion recovery

**Status:** accepted
**Date:** 2026-10-16
**Area:** backend, frontend, protocol

## Context

A damaged workspace can prevent task deletion. An empty ownership marker
blocked both preflight and service admission in a reported restart incident.
Removing a task record and deleting its workspace have different evidence
requirements. Filesystem inspection cannot be a prerequisite for an action
whose explicit outcome is to keep those files.

The existing [fail-closed cleanup decision](0009-fail-closed-gc-semantics.md)
still applies. Simply skipping the check would permit unsafe cleanup. Merely
skipping the current cleanup worker would leave orphan collection free to act
after the task row disappears.

## Decision

Introduce explicit retain mode for task deletion. It authorizes removal of the
task records while preserving workspace resources. Persist the retention
decision in the existing durable cleanup owner before deleting live rows.
Every destructive cleanup consumer must respect that retained decision.
Agent shutdown remains a separate durable obligation.

The original normal-deletion admission rule is amended by
[separate task deletion from cleanup](2026-10-19-separate-task-deletion-from-cleanup.md).
That explicit recovery path does not change retain-mode guarantees. Without
the new acknowledgement, normal deletion keeps its inspection and consent behavior.
Authorization, database integrity, and managed-task admission remain mandatory
in both modes. Unknown physical state is sufficient to keep files, but never
sufficient to remove them.

The [task deletion recovery design](../specs/tasks/system-design/task-delete-recovery.md)
defines the protocol, storage protection, and UI. This decision extends the
task-owned resource lifetime after task record deletion without replacing its
canonical ownership graph.

## Consequences

Users can remove broken tasks without editing markers or moving directories.
Retained resources can consume disk or provider capacity indefinitely. Their
retention evidence must outlive cleanup completion and workspace deletion.
Existing generic storage cleanup cannot silently reclaim them.

Keeping workspace files does not keep the deleted conversation. A failed agent
stop must remain visible as pending work. Older cleanup readers that ignore
retention cannot run against these snapshots. Shipping requires full consumer
coverage, not only a changed delete dialog.

## Alternatives considered

- **Force file deletion after an inspection error:** Reject because the error
  does not prove ownership or identify safe cleanup targets.
- **Keep task deletion blocked until repair:** Reject because damaged or
  unavailable workspaces make obsolete tasks impossible to remove through Kandev.
- **Automatically keep files on any deletion error:** Reject because it silently
  changes the requested cleanup outcome and can conceal unrelated failures.
- **Move the directory to quarantine first:** Reject as a prerequisite because
  rename, marker reads, and provider calls can fail for the same reason as
  inspection. Retention must be possible without touching the workspace.
