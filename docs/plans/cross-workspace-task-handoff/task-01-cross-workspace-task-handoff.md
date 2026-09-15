---
id: "01-cross-workspace-task-handoff"
title: "Implement cross-workspace task handoff"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CROSS-WORKSPACE-HANDOFF-001
  - REQ-TASKS-CROSS-WORKSPACE-HANDOFF-002
acceptance_criteria:
  - AC-TASKS-CROSS-WORKSPACE-HANDOFF-001.1
  - AC-TASKS-CROSS-WORKSPACE-HANDOFF-001.2
  - AC-TASKS-CROSS-WORKSPACE-HANDOFF-001.3
  - AC-TASKS-CROSS-WORKSPACE-HANDOFF-001.4
  - AC-TASKS-CROSS-WORKSPACE-HANDOFF-002.1
  - AC-TASKS-CROSS-WORKSPACE-HANDOFF-002.2
system_design:
  - ../../specs/tasks/system-design/cross-workspace-task-handoff.md
---

# Task 01: Implement cross-workspace task handoff

## Summary

Add the Office MCP handoff tool and its trusted authorization, resource
resolution, provenance, idempotency, reverse-link, activity, and launch flow.
Protect handoff metadata from generic mutation paths and preserve an explicit
agent profile through deferred starts.

## Files and boundaries

- `apps/backend/internal/mcp/handlers/` owns validation, authorization,
  idempotency outcomes, and activity dispatch.
- `apps/backend/internal/task/` owns protected metadata and the reverse-link
  compare-and-set repository operation.
- `apps/backend/internal/orchestrator/` owns deferred session preparation and
  explicit profile retention.
- `apps/backend/internal/agent/` and `apps/backend/internal/office/` own the
  permission and Office capability inputs.

## Verification

Focused backend tests cover the handler, task metadata, reverse-link CAS, and
deferred profile start paths. Specification catalog and lint checks validate
the linked requirement and system-design documents.

## Result

The handoff flow, metadata safeguards, deferred profile marker, and regression
coverage are implemented. Existing ordinary task creation and same-workspace
flows remain unchanged.
