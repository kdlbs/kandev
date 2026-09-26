---
id: "05-runtime-dispatch"
title: "Route task launch and follow-ups to Cursor Cloud"
status: complete
wave: 5
depends_on:
  - "04-scoped-mcp-callback"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-001
  - REQ-EXECUTORS-CURSOR-CLOUD-002
  - REQ-EXECUTORS-CURSOR-CLOUD-004
acceptance_criteria:
  - AC-EXECUTORS-CURSOR-CLOUD-002.1
  - AC-EXECUTORS-CURSOR-CLOUD-002.2
  - AC-EXECUTORS-CURSOR-CLOUD-002.5
  - AC-EXECUTORS-CURSOR-CLOUD-004.1
  - AC-EXECUTORS-CURSOR-CLOUD-004.4
  - AC-EXECUTORS-CURSOR-CLOUD-001.4
  - AC-EXECUTORS-CURSOR-CLOUD-002.6
  - AC-EXECUTORS-CURSOR-CLOUD-004.5
system_design:
  - ../../specs/executors/system-design/cursor-cloud.md
---

# Task 05: Route task launch and follow-ups to Cursor Cloud

## Summary

A full task start through backend composition creates exactly one remote conversation and initial turn without local workspace setup.
Use TDD for changed logic. Keep results pending until the listed checks pass.

## In scope

- Add the proposed RuntimeRouter and cursorcloud runtime; reserve locally in Launch and submit in StartExecution.
- Wire AgentManagerClient launch, prompt, lookup, and queue integration through the runtime boundary without fake agentctl instances.
- Resolve one attached GitHub repository and published ref. Freeze create payload, model, callback grant, and explicit PR choice.
- Use the durable create identity for initial-create reconciliation. Follow-up timeouts remain unknown and block subsequent dispatch.
- Keep local Cursor CLI execution unchanged, reject unsupported task modes and attachments, and preserve existing prompt/plan composition with cloud limits.

- Implement every AgentManagerClient row in the design compatibility table, plus PromptTurnIDSetter. Add a contract test covering every supported or rejected method.
- Keep chat agent.cancel mapped to CancelAgent. StopAgent and StopAgentWithReason terminate local execution after remote confirmation while retaining its detached conversation binding.
- Route normal workflow on_enter/auto_start, post-step prompts, and queue draining through the same journal. Deduplicate by originating operation/message identity and recheck step/question guards.

## Out of scope

- Work assigned to later tasks, unrelated refactors, and release promotion.
- Paid cloud execution during automated tests.

## Acceptance

- A full task start through backend composition creates exactly one remote conversation and initial turn without local workspace setup.
- Follow-ups use the same conversation, serialize through the queue, and never automatically repeat unknown submissions.
- Invalid repositories and incompatible profiles fail before network mutation; local dirty files and existing executor launches remain unchanged.

## Verification

Run from the repository root. New test paths are implementation outputs, not tests available during this planning turn.

```bash
(cd apps/backend && go test ./internal/agent/runtime ./internal/agent/runtime/cursorcloud ./internal/orchestrator/executor ./internal/backendapp -count=1)
```

### Evidence mapping

- 002.1, 002.2, 002.5: `internal/agent/runtime/cursorcloud/dispatch_test.go: TestCreateRecovery, TestSerializedFollowups, TestUnknownSubmission`.
- 004.1, 004.4, 001.4: `internal/orchestrator/executor/executor_cursor_cloud_test.go: TestCursorCloudLaunch, TestCursorCloudRepositoryAdmission, TestCursorCloudCompatibility`.

- 002.6: `internal/orchestrator/executor/executor_cursor_cloud_workflow_test.go: TestCloudWorkflowAutoStartSerialized, TestCloudQueueDrainOnce`.
- 004.5: `internal/orchestrator/executor/executor_cursor_cloud_contract_test.go: TestCloudManagerMethodMatrix, TestCloudFrozenSelection`.

## Files likely touched

- `apps/backend/internal/agent/runtime/runtime.go`.
- `apps/backend/internal/agent/runtime/router.go (new)`.
- `apps/backend/internal/agent/runtime/cursorcloud/ (new)`.
- `apps/backend/internal/orchestrator/executor/executor.go`.
- `apps/backend/internal/orchestrator/executor/executor_execute.go`.
- `apps/backend/internal/orchestrator/executor/executor_resume.go`.
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`.
- `apps/backend/internal/backendapp/agents.go`.

## Dependencies

04-scoped-mcp-callback

## Risks

The facade is incomplete and normal tasks use direct lifecycle methods. Test the composed task entry point, not only a standalone adapter.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/cursor-cloud.md).
- [System design](../../specs/executors/system-design/cursor-cloud.md).
- [Proposed runtime ADR](../../decisions/2026-09-25-managed-remote-agent-runtime.md).
- Source baseline and code patterns listed in the plan.

## Results

Completed. Validation passed: `(cd apps/backend && go test ./internal/agent/runtime ./internal/agent/runtime/cursorcloud ./internal/orchestrator/executor ./internal/backendapp -count=1)`. The composed launch test confirms Cursor Cloud skips local workspace preparation; request snapshots freeze the repository/ref/model/callback/MCP mode/PR selection; the contract test covers manager capabilities; and shared model tests pin active journal states.

### Review remediation

The local prompt fallback now preserves both `dispatchOnly` and `onDispatched` through the lifecycle callback API, including with Cursor Cloud disabled. Passed: `(cd apps/backend && go test ./internal/backendapp -run 'TestCloud|TestCursorCloud' -count=1)`.
