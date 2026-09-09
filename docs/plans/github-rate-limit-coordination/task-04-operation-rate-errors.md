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
- Kanban and Office task profiles expose a task-bound, zero-provider-call
  `get_github_rate_limit_kandev` snapshot of cached primary observations,
  observed secondary state, quota principal, and admission decisions.

## Verification

- `cd apps/backend && go test ./internal/github ./internal/workflowsync ./internal/mcp/server -count=1`

## Results

Restored the task and Office GitHub rate snapshot tool as a read-only,
task-scoped view of locally cached coordinator state. Provider and admission
errors continue to keep structured context that belongs to the affected
operation. Manual Workflow Sync returns safe rate details beside its existing
error.
