# ADR-2026-07-29-quarantine-retention-override: Make Quarantine Retention Overridable but Visible

**Status:** accepted
**Date:** 2026-07-29
**Area:** backend, frontend

## Context

Storage maintenance atomically moves owned task workspaces and Go caches into quarantine, but the
initial implementation enforced each entry's retention deadline without showing that deadline in
the Storage page. It also exposed an enabled delete action before the deadline and described
automatic deletion that no maintenance provider performed. Operators need predictable automatic
reclamation and a deliberate way to recover disk space immediately when they accept losing the
restore window.

## Decision

The persisted `storage_quarantine_entries.delete_after` timestamp remains the normal permanent
deletion boundary. Individual deletion, **Clear eligible**, and scheduled or full manual storage
maintenance delete only entries at or after that timestamp.

The Storage page shows the deletion-eligibility timestamp and whether automatic cleanup is enabled.
Scheduled maintenance and full manual **Run now** include a typed quarantine provider that
permanently deletes eligible entries. When scheduling is disabled, no separate quarantine sweeper
runs; eligible entries remain until a full manual run or an explicit quarantine action.

Install operators also receive a separate **Force clear all** action. It requires the stronger
server-validated confirmation phrase `DELETE ALL NOW` and may permanently delete active quarantine
entries before their retention deadlines. This override bypasses only the retention clock. Resource
ownership, path containment, symlink, state-transition, and Git worktree-pruning safety checks
remain mandatory.

Bulk deletion is best-effort across entries. Its durable System job reports deleted, protected, and
failed counts and bytes; any entry that fails remains visible and retryable.

## Consequences

- Operators can see the earliest safe deletion time and understand whether reclamation is automatic
  or requires a manual action.
- Normal maintenance restores the intended bounded quarantine lifecycle instead of retaining
  expired entries indefinitely.
- A deliberate install-wide override can reclaim all quarantine space immediately, at the cost of
  losing protected restore windows.
- Disabling scheduled maintenance also disables automatic quarantine deletion, so the UI and
  public documentation must make that dependency explicit.
- The force action needs stronger confirmation and regression coverage proving it cannot bypass
  ownership or path-safety validation.

## Alternatives Considered

- **Never allow retention bypass.** Rejected because an operator facing disk exhaustion needs an
  explicit way to trade recovery for immediate reclamation.
- **Make every delete action bypass retention after confirmation.** Rejected because it makes the
  normal row action undermine the configured safety policy.
- **Run a dedicated quarantine sweeper while scheduled maintenance is disabled.** Rejected because
  disabling destructive scheduling should stop unattended permanent deletion.
- **Keep quarantine deletion manual-only.** Rejected because expired data would continue to
  accumulate despite an enabled storage-maintenance schedule.
