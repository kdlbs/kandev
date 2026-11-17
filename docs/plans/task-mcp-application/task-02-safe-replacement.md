---
id: "02-safe-replacement"
title: "Replace executions at safe boundaries"
status: done
wave: 4
depends_on:
  - 06-generation-binding
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-APPLY-001
  - REQ-AGENTS-MCP-APPLY-002
  - REQ-AGENTS-MCP-APPLY-003
acceptance_criteria:
  - AC-AGENTS-MCP-APPLY-001.2
  - AC-AGENTS-MCP-APPLY-001.3
  - AC-AGENTS-MCP-APPLY-001.4
  - AC-AGENTS-MCP-APPLY-001.6
  - AC-AGENTS-MCP-APPLY-002.3
  - AC-AGENTS-MCP-APPLY-003.1
  - AC-AGENTS-MCP-APPLY-003.2
  - AC-AGENTS-MCP-APPLY-003.3
system_design:
  - ../../specs/agents/system-design/task-mcp-application.md
---

# Task 02: Replace executions at safe boundaries

## Summary

Add a coordinator that claims safe sessions and replaces one provider generation inside the same runtime execution without starting a turn.
Keep adapter capability disabled until work order 03 supplies conformance evidence.

## In scope

Share admission with prompt, resume, cancellation, and workflow dispatch. Preserve workspace resources and delivery identities.
Implement failure stages, persisted provider-generation fencing, startup reconciliation, bounded retries, and human Retry authorization.
Extend the existing execution claim to include helper incarnation, source process generation, binding ID, and configuration revision.
Use work order 06 protocol receipts and current-binding pointers. The same execution ID cannot prove the source provider is unchanged.
Use a fake conforming provider for coordinator tests.

## Out of scope

Automatic interruption, Start fresh, and unsupported-provider enablement.

## Acceptance

1. Running, uncertain, question-blocked, provider-background-active, and stopped/completed sessions never undergo an automatic disruptive restart.
2. Eligible idle replacement preserves identity, independent workspace services, and queued input with no new prompt or completion side effect.
3. Crash-stage recovery and duplicate events never create overlapping processes or reset failed-attempt limits.

## Verification

Use TDD for changed logic. Run commands from the repository root. New test paths are targets created by this work order.

```bash
(cd apps/backend && go test ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'Test(BuiltinApplication|MCPProviderReplacement|ResumePromptQueue)' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator ./internal/agent/runtime/lifecycle -run TestBuiltinApplication -count=1)
bash scripts/lint-go-changed $(git ls-files --modified --others --exclude-standard -- '*.go')
```

## Files likely touched

- `apps/backend/internal/orchestrator/builtin_mcp_application.go (new)`
- `apps/backend/internal/orchestrator/builtin_mcp_application_test.go (new)`
- `apps/backend/internal/agent/runtime/runtime.go`
- `apps/backend/internal/agent/runtime/lifecycle/provider_replacement.go` (new)
- `apps/backend/internal/task/repository/sqlite/builtin_application_claim.go`
- `apps/backend/internal/orchestrator/task_operations.go`

## Dependencies

06-generation-binding

## Risks

A generic Stop path can cancel the session or remove workspace resources. Use the provider-only protocol and retain the runtime owner.

## Parallelism

Sequential. No delegation is authorized.

## Inputs

- [Requirements](../../specs/agents/requirements/task-mcp-application.md).
- [System design](../../specs/agents/system-design/task-mcp-application.md).
- [Plan and test mapping](plan.md).
- Scoped backend/web guidance and existing builtin MCP tests.

## Results

The automatic coordinator is wired from committed human task saves and a service-lifetime reconciliation worker. It recovers requests from SQL and uses the retained provider control seam. Session lifecycle and prompt admission serialize application. Queue insertion resumes after the durable dispatch barrier, while dispatch waits for acceptance.

Concurrent-save tests cover prepare, retirement, activation, and acceptance. Lost responses reconcile without another provider start. Running work, unsupported providers, pending questions, and unavailable evidence defer application. Reverts to the accepted selection settle without replacement. Explicit Retry is versioned and cannot replay a failed request automatically.

`builtin_mcp_application_test.go`, `builtin_mcp_application_concurrency_test.go`, `builtin_mcp_queue_test.go`, and the messagequeue admission tests own these checks. Final validation is recorded in the plan. No live user task was restarted.
