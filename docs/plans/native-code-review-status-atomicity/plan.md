---
created: 2026-09-14
status: completed
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
system_design:
  - ../../specs/agents/system-design/native-code-review-status-atomicity.md
legacy_specs: []
---

# Implementation plan: Native review finding status atomicity

Review finding status changes must be safe when more than one client updates the
same finding at the same time. The repository owns one atomic transition that
reads the stored status, derives `resolved_at`, and returns the stored finding.
The service keeps validation and publishes the existing update event after the
transition succeeds.

## Work order

- [x] [Make review finding status transitions atomic](task-01-review-finding-status-atomicity.md)

## Scope

- Keep the existing review finding and WebSocket contracts.
- Use one repository transition for status and timestamp updates.
- Cover concurrent SQLite writers and an environment-gated PostgreSQL
  multi-connection path.
