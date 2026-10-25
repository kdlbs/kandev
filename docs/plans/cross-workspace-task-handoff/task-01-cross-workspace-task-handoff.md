---
id: "01-cross-workspace-task-handoff"
title: "Implement cross-workspace task handoff"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-START-001
acceptance_criteria:
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.2
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.3
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.4
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.5
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.6
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.7
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.8
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.9
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.10
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.11
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001.12
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.2
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.3
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.4
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.5
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.6
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.7
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.8
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.9
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001.10
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.2
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.3
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.4
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.5
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.6
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.7
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001.8
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.2
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.3
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.4
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.5
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.6
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.7
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001.8
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001.2
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001.3
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.2
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.3
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.4
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.5
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.6
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.7
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.8
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001.9
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.2
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.3
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.4
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.5
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001.6
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.1
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.2
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.3
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.4
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.5
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.6
  - AC-CROSS-WORKSPACE-TASK-HANDOFF-START-001.7
system_design:
  - ../../specs/cross-workspace-task-handoff/system-design/handoff-mechanism.md
  - ../../specs/cross-workspace-task-handoff/system-design/failure-modes-and-verification.md
---

# Task 01: Implement cross-workspace task handoff

## Summary

Add the `kandev task handoff` CLI subcommand and its backing Office runtime
route, with trusted authorization, target-workspace resolution, provenance,
idempotency, reverse-link integrity, profile resolution, and launch flow. An
earlier iteration of this delivery exposed the same capability as an
Office-only MCP tool (`handoff_task_kandev`); that surface was withdrawn
before merge in favor of the CLI/route mechanism described here, per the
`command-and-route-surface` requirement's "no MCP tool" constraint. Protect
handoff metadata from generic mutation paths and preserve an explicit agent
profile through deferred starts.

## Files and boundaries

- `apps/backend/cmd/agentctl/kandev_task.go` owns the `kandev task handoff`
  subcommand surface.
- `apps/backend/internal/office/runtime/` owns the route handler, action,
  authorization/capability derivation, and same-workspace refusal.
- `apps/backend/internal/task/` owns protected metadata and the reverse-link
  compare-and-set repository operation.
- `apps/backend/internal/orchestrator/` owns deferred session preparation and
  explicit profile retention.
- `apps/backend/internal/agent/` and `apps/backend/internal/office/` own the
  permission and Office capability inputs.

## Verification

Focused backend tests cover the CLI subcommand, the route handler,
authorization/capability derivation, task metadata, reverse-link CAS, and
deferred profile start paths. Specification catalog and lint checks validate
the linked requirement and system-design documents.

## Result

The handoff flow, metadata safeguards, deferred profile marker, and regression
coverage are implemented. Existing ordinary task creation and same-workspace
flows remain unchanged. No MCP tool, profile capability, or tool-group entry
backs this capability on any surface.
