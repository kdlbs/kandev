---
id: "01-complete-profile-preparation"
title: "Deliver the complete workspace MCP profile"
status: cancelled
wave: 2
depends_on:
  - 00-restart-reproduction
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PROFILE-CONTINUITY-001
acceptance_criteria:
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.1
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.2
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.3
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.5
  - AC-AGENTS-MCP-PROFILE-CONTINUITY-001.6
system_design:
  - ../../specs/agents/system-design/mcp-profile-continuity.md
---

# Task 01: Deliver the complete workspace MCP profile

## Disposition

Cancelled after [Task 00](task-00-restart-reproduction.md).

This work order proposed a new `PUT /api/v1/mcp/profile` operation for monolithic workspace promotion.
Actual backend restart/resume reproduces the historical failure on v0.98.0.
The same flow passes on current main through the existing complete-profile package binding operation.

The earlier manifest-free component diagnostic bypassed normal current task profile resolution.
It did not establish a reachable current defect requiring another endpoint.
Adding that endpoint or another rejection rule would not improve the tested current flow.

## Scope

No implementation work remains in this order.
No endpoint, process-manager method, client method, lifecycle interface, or startup guard is to be added from the cancelled proposal.
Task 02 provides regression coverage of the existing path.

## Acceptance

The final package removes the proposed operation from the design and active dependencies.
The investigation records both the historical failure and current positive controls.

## Verification

See [the investigation](investigation.md) for the actual runtime comparison.
No proposed endpoint tests were implemented or run.

## Files touched

This work order and the plan/design corrections only.

## Dependencies

Task 00 supplied the evidence for cancellation.
Task 02 depends directly on Task 00.

## Risks

Reviving this proposal from a manually manifest-free fixture would address an unproved current production path.
A future failure needs new evidence before selecting a repair.

## Parallelism

`sequential`

## Inputs

- [Investigation decision](plan.md#investigation-decision).
- [Process evidence](investigation.md).
- [System design](../../specs/agents/system-design/mcp-profile-continuity.md).

## Results

Cancelled. No production or permanent test changes were made.
