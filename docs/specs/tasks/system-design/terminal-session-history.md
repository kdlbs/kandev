---
status: current
system: tasks
requirements:
  - REQ-TASKS-TERMINAL-SESSION-HISTORY-001
---

# Terminal Helper Session History System Design

## Purpose and boundaries

The task system owns terminal session cleanup, evidence retention, and the
archived-session projection. The web client owns only the explicit history
visibility preference and navigation to an archived conversation.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-TERMINAL-SESSION-HISTORY-001` | [Scoped cleanup](#scoped-cleanup), [Retention and history projection](#retention-and-history-projection), [History controls](#history-controls) |

## Scoped cleanup

The MCP `close_task_session_kandev` tool routes through
`internal/mcp/handlers/session_cleanup.go`. The backend derives task, session,
and workspace scope from the authenticated caller. Cleanup requires the caller
to be the current primary session and the target to be a terminal non-primary
session in the same task. A previously committed task-scoped receipt makes a
retry idempotent.

## Retention and history projection

`task.Repository.DeleteTaskSessionWithAttachments` checks the exact session
incarnation and retained-evidence guard inside one transaction. If evidence is
present, it requires the session queue to be empty, sets `task_sessions.archived_at`,
and records an `archived` cleanup receipt without deleting transcript rows. If
evidence is absent, the existing purge path removes disposable state and
records the corresponding cleanup receipt. A failure before commit leaves the
session and its evidence unchanged. The receipt's `queue_before_count` is zero
because cleanup commits only after verifying that no pending queue work remains;
the field records that disposition invariant rather than a rejected attempt's
queue size.

Session summary queries and the service fallback projection both carry
`archived_at` to the API. Primary promotion, resume candidate admission, and
session prompt admission reject archived rows. The session resumption hook
skips automatic status/recovery work and manual resume for archived history.
The history filter changes only which summaries are shown; it does not
reactivate an archived conversation.

## History controls

`SessionHistoryToggle` provides the localized Show/Hide history action for the
desktop session tabs, kanban session preview, and mobile session list. Desktop
and preview use a compact icon control with an accessible name. Mobile displays
the localized label in a full-width, touch-sized action. Archived rows sort
after active sessions and remain available for transcript navigation while the
history filter is enabled.

## Verification boundaries

Repository tests cover evidence-preserving archive, disposable cleanup, receipt
replay, and lifecycle guards. Browser tests cover desktop archive/reopen and
transcript continuity, kanban preview history, and mobile history visibility
and cleanup confirmation.
