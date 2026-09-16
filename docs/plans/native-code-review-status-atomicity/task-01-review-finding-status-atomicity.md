---
id: review-finding-status-atomicity
title: Make review finding status transitions atomic
status: completed
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
acceptance_criteria:
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.2
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.4
system_design:
  - ../../specs/agents/system-design/native-code-review-status-atomicity.md
---

# Make review finding status transitions atomic

## Scope

Replace the review service read/update/read sequence with one repository
transition. The SQL update derives `resolved_at` from the status stored by that
same statement and returns the final finding row. Keep status validation and
`TaskReviewFindingUpdated` publication in the service.

## Verification

- Review service concurrent update coverage passes.
- SQLite repository concurrent transition coverage passes.
- The real PostgreSQL two-connection test runs when its environment flag is
  enabled and remains skipped by default.
- The public WebSocket payload remains unchanged.

## Result

The implementation is complete at commit `674dd00f9dae51d1982870ae88583390e1e3aa68`.
