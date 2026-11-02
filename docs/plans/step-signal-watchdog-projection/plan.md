---
status: done
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-OFFICE-STALL-VISIBILITY-001
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
  - ../../specs/office/system-design/stall-visibility.md
---

# Step-signal watchdog projection

Issue #4531's reporter corrected the original archive expectation: explicit
self-archive is supported and works. Current main still reloads full active
sessions, metadata, snapshots, and worktrees for each stuck-signal scan. The
scan consumes only four scalar fields and the pending signal. This package
repairs that bounded read amplification without changing task retention.

Existing behavioral authority is
REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001,
AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.1, and the exclusions in
REQ-OFFICE-STALL-VISIBILITY-001 (AC-001.1 through AC-001.5).
The owning design is
[completion signals](../../specs/tasks/system-design/workflow-explicit-completion-signal.md#watchdog-scan-projection).
The [Office design](../../specs/office/system-design/stall-visibility.md)
continues owning detection-only behavior.

One sequential [work order](task-01-project-watchdog-sessions.md) owns the
repository projection, consumer wiring, and regression evidence. The parent
explicitly authorized autonomous bounded, behavior-preserving fixes after
reproduction; delivery still requires parent actual-diff validation.

No automatic archive/cancel, prompt inspection, scratch deletion, catalog
deduplication, schema, or retention-default change is included. SQL JSON
extraction still has database parsing cost; synthetic measurements cannot
establish reporter-instance or production-load performance.

## Result

The narrow reader and consumer substitution are implemented and pass the
work-order checks. Repository and real-consumer regressions failed before
production edits and pass after them. The parent validated the complete patch
and independently repeated focused SQLite/watchdog and PostgreSQL checks.
Protected delivery is authorized; exact-head CI/review and merge evidence
remain external to this implementation record. Retention/storage suggestions
remain outside this partial repair; no issue closure is authorized.
