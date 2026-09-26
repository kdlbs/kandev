---
id: "04-auto-approval-audit"
title: "Persist auto-approval decisions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.9
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003.11
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 04: Persist auto-approval decisions

## Summary

Keep the selected option ID, option kind, and automatic source in durable
permission history. A completed approval must not have only a status and an
ephemeral log as evidence.

## In scope

- Carry the decision fields through agentctl's permission event.
- Persist them in the task permission message and expose them on replay.
- Make loss of the approval event observable and recoverable, not silent.

## Out of scope

- Changing how an allow option is selected.
- New general-purpose event infrastructure.

## Acceptance

1. Reloaded/replayed permission history identifies the exact selected option and automatic source.
2. An event delivery failure cannot silently leave an approved call without durable decision evidence.
3. Human approvals keep their existing status and attribution.

## Verification

```bash
cd apps/backend
go test ./internal/agentctl/server/process ./internal/orchestrator ./internal/task/service -run 'Test(Permission|AutoApprove)' -count=1
```

## Files likely touched

- `apps/backend/internal/agentctl/server/process/manager.go`
- `apps/backend/internal/orchestrator/watcher/watcher.go`
- `apps/backend/internal/orchestrator/event_handlers_git.go`
- `apps/backend/internal/task/models/message.go`
- `apps/backend/internal/task/service/service_messages_permission_test.go`
- `apps/backend/internal/orchestrator/agent_permissions_test.go`

## Dependencies

None.

## Risks

The event can be emitted before task message creation. Preserve ordering or an
idempotent update keyed by request identity.

## Parallelism

`parallel-safe`

## Inputs

- [Requirement](../../specs/agents/requirements/agent-permission-control-integrity.md), `003.4`, `003.9`, `003.11`.
- [System design](../../specs/agents/system-design/agent-permission-control-integrity.md), auto-approve selection.

## Results

The agentctl permission event now carries the selected option ID, provider
option kind, and `auto_approve` source. The process manager reliably queues this
event before returning an automatic approval; if the stream is stopped, it
falls through to the user-answerable permission path. The lifecycle publisher
retries transient event-bus failures and logs all decision fields after a final
failure.

The orchestrator persists `permission_decision` and approved status in the same
permission-message create. Automatic message writes retry three times and use a
stable request-derived message ID, so retrying a delivery does not create a
duplicate transcript row. Human approvals retain their existing status and
`permission_resolution` attribution. Older events can recover the selected
kind from the option list and infer the fixed automatic source.

Verification passed:

- `go test ./internal/agentctl/server/process ./internal/orchestrator ./internal/task/service -run 'Test(Permission|AutoApprove)' -count=1`
- `go test ./internal/agent/runtime/lifecycle -run 'Test(PublishPermissionRequest|TestPublishPermissionRequest)' -count=1`
- `go test ./internal/integration -run 'Test(AutoApprovedPermissionDecisionPersistsInSessionReplay|AuthenticatedMCPAgentPermissionListResolveAndReplay)' -count=1`
- `go test ./internal/backendapp -run '^$' -count=1`
