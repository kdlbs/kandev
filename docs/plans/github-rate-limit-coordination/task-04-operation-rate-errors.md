---
id: 04-operation-rate-errors
title: Operation-local rate errors
status: done
wave: 4
depends_on: [03-workflow-sync-backoff]
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-004
system_design: ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 04: Operation-Local Rate Errors

## Acceptance

- A failed managed GitHub operation can return a safe, structured rate object.
- The object includes the rate kind, resource, retry boundary, delay, and source.
- Successful operations omit quota and internal coordinator state.
- Task and Office profiles do not expose a separate GitHub rate diagnostic tool.

## Verification

- `cd apps/backend && go test ./internal/github ./internal/workflowsync ./internal/mcp/server -count=1`

## Results

Removed the task and Office GitHub rate snapshot tool. Provider and admission
errors keep the structured context that belongs to the affected operation.
Manual Workflow Sync returns safe rate details beside its existing error.
Internal scheduler, telemetry, and logs retain access to coordinator state.
