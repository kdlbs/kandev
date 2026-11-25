---
id: "01-record-agent-group"
title: "Record the agent process group"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.20
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.21
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
---

# Task 01: Record the agent process group

## Summary

agentctl reports the agent child's process-group birth identity in the authenticated
delivery descriptor. Lifecycle stores it in the durable prompt binding, the reconciliation
identity, the recovery metadata, and the silent-restore checkpoint, as optional fields on the existing
runtime identity.

## In scope

- An additive `agent_process` field on the agentctl delivery status, for a running agent child only.
- Shared descriptor decoding and copied client cache values.
- Optional `AgentGroupID` and `AgentBirthToken` fields on `processidentity.Identity`, carried
  through `original_runtime` in bindings and recovery models.

## Out of scope

- Termination evaluation (task 02), remote executors, and UI.

## Acceptance

1. A running local instance reports `agent_process` with PID = GroupID and a birth token. It
   is absent before start and after exit.
2. A durable prompt binding and the recovery metadata created afterwards carry the group
   identity inside `original_runtime`. Older JSON without the optional fields still decodes.

## Verification

```bash
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agentctl/server/api ./internal/agentctl/server/instance -run 'AgentProcess|DeliveryStatus')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/agent/runtime/lifecycle -run 'PromptTurnBinding|ReconciliationIdentity|AgentGroup')
(cd apps/backend && go test -trimpath -race -count=1 ./internal/task/models ./internal/agent/runtime/agentctl)
(bash scripts/lint-go-changed $(git diff --name-only -- 'apps/backend/*.go'))
```

## Files likely touched

- `apps/backend/internal/agentctl/server/api/delivery_protocol.go`
- `apps/backend/internal/agentctl/server/instance/instance.go`
- `apps/backend/internal/agent/runtime/agentctl/` (delivery status type)
- `apps/backend/internal/agent/runtime/lifecycle/durable_prompt_turn_recovery.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_reconciliation.go`
- `apps/backend/internal/agent/runtime/lifecycle/delivery_record_identity.go`
- `apps/backend/internal/task/models/` (`AgentDeliveryRecovery`, silent-restore checkpoint)

## Dependencies

None.

## Risks

- The agentctl binary ships separately, and mixed versions must tolerate the missing field.

## Parallelism

`sequential`

## Inputs

- `durable-restart-reconciliation.md`, section on the original owner.
- ADR `2026-11-02-agent-process-group-termination-proof.md`.

## Results

- agentctl: `process.Manager` records the child identity after lifecycle installation and
  adapter connection succeed. It clears the identity after `Wait` and reports it as
  `agent_process` in `journal.RecoveryDescriptor`. Failed startup publishes no identity.
  `TestAgentProcessIdentityTracksRunningGroupLeader` passes.
- lifecycle: `withAgentProcessGroup` adds the reported group to the prompt binding, and
  `boundAgentProcessGroup` reuses it for the reconciliation identity.
  `TestWithAgentProcessGroupRecordsReportedAgentGroup` and
  `TestBoundAgentProcessGroupReusesPromptBinding` pass.
- `go test -trimpath -race -count=1 ./internal/common/processidentity ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/agent/runtime/... ./internal/orchestrator ./internal/task/models ./internal/task/repository/sqlite`: all `ok` (lifecycle 227s, sqlite 381s, process 319s).
- `GOOS=darwin` and `GOOS=windows` `go vet` of the changed platform packages: passed.
- `bash scripts/lint-go-changed <changed Go files>`: 0 issues.
- `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`: passed.

Deviation from the plan: the agent group travels as two optional comparable fields on
`processidentity.Identity` (`agent_group_id`, `agent_birth_token`), set with `WithAgentGroup`.
There is no separate `OriginalAgentGroup` field, so the existing `original_runtime` plumbing
(bindings, recovery metadata, silent-restore checkpoints, journal requests) carries it
unchanged. Darwin group proof returns `ErrUnverifiableIdentity`, so Darwin keeps the
runtime-only rule.

Review corrections add failed-startup and client-cache ownership regressions. These tests remain
unrun under the review request. The original verification results do not cover these corrections.
