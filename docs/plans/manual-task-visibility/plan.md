---
created: 2026-10-25
status: in_progress
requirements:
  - REQ-COORDINATOR-MANUAL-VISIBILITY-001
system_design:
  - ../../specs/coordinator/system-design/manual-task-visibility.md
legacy_specs: []
---

# Implementation Plan: Manual Session Task Lifecycle Visibility

## Outcome

Give an explicitly granted manual Coordinator session one compact,
same-workspace lifecycle read. Keep managed coordinator, relation, plugin,
document, and write surfaces outside this grant. The implementation is tracked
in [Task 01](task-01-session-bound-lifecycle-visibility.md).

## Delivery and dependencies

The task implements the grant, dispatch guard, durable lifecycle receipts, and
compact projection together because each layer is needed for one safe read.
The behavioral candidate is implemented. Native PostgreSQL verification and
independent QA remain open; this record does not mark those gates passed.

## Verification strategy

Run focused grant/dispatch and repository matrix tests on SQLite. Run the
PostgreSQL matrix against a fresh task-owned PostgreSQL 16 fixture before final
acceptance. Recheck the independent reviewer and QA against the final exact
commit. Keep PR publication, CI, and human acceptance as separate gates.
