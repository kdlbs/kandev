---
created: 2026-10-28
status: complete
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-006
  - REQ-PLATFORM-SESSION-SUBSCRIPTION-RECOVERY-001
system_design:
  - ../../specs/plugins/system-design/conversation-source-reconciliation.md
  - ../../specs/platform/system-design/session-subscription-recovery.md
legacy_specs: []
---

# Conversation receipt transport

Extract the retained conversation fixes from the reviewed dense-workbench work order 02.
The implementation uses current main's canonical message identities, segment boundaries,
shell projection, and PostgreSQL task-lock retry behavior.

## Work packages

| Work order | Status | Scope |
| --- | --- | --- |
| [01: Preserve receipts](task-01-preserve-receipts.md) | done | JSON transport, canonical snapshots, timestamp ordering, targeted verification |

## Boundaries

Receipts remain transient results of the source transaction. Publication follows commit.
Coalescing merges only complete, contiguous receipts for one session. Missing evidence
retains source recovery. Existing privacy, generation, subscription, and replay guards apply.
No schema, public API, permission, layout, or control changes are required.

The frontend change compares timestamps as instants with nanosecond precision.
Desktop and phone use the existing chat surfaces and share this normalization.

## Evidence limits

This extraction removes redundant recovery caused by lost receipts. It does not establish
smoother rendering, a heap-leak fix, a task-switch speed target, or Firefox performance gains.
Historical research timings do not measure this branch. The work order records current checks.
