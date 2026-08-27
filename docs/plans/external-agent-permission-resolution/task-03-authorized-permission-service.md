---
id: "03-authorized-permission-service"
title: "Authorized permission service"
status: completed
wave: 3
depends_on: ["01-live-permission-contract", "02-permission-audit-claim"]
plan: "plan.md"
spec: "../../specs/agents/requirements/external-permission-resolution.md"
---

# Task 03: Authorized permission service

## Acceptance

- Lifecycle/executor carry the Kandev request generation and expose list/strict resolve without
  importing agentctl process state above the runtime seam.
- The orchestrator service authorizes task/session ownership before runtime or audit access, lists
  only live requests, derives the actor safely, claims before delivery, and finalizes every known
  outcome with stable domain errors.
- The existing web permission handler routes through the service with task/request identity while
  preserving approve, reject, cancel-without-reject-option, expired, and session-state behavior.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/orchestrator ./internal/orchestrator/handlers ./internal/backendapp
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/event_types.go`
- `apps/backend/internal/agent/runtime/lifecycle/events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_events.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/orchestrator/watcher/watcher.go`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/orchestrator/executor/executor_interaction.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/agent_permissions.go`
- `apps/backend/internal/orchestrator/agent_permissions_test.go`
- `apps/backend/internal/orchestrator/session_scope_matrix_test.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`
- `apps/backend/internal/backendapp/adapters.go`

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential. It fixes the reusable service contract consumed independently by MCP and web work.

## Inputs

- Spec: API/state/permissions/failure scenarios.
- Decision: authorize and audit above live runtime authority.
- Existing patterns: `authorizeTaskSessionPair`, `RespondToPermission`, lifecycle execution lookup,
  and `session_scope_matrix_test.go`.

## Risks

- Task-wide listing must not return partial results when one session lookup fails.
- Automation auto-rejection and no-reject cancellation require an internal path but must not widen
  the external option-only service contract.

## Output contract

Report service signatures/error mapping, authorization order evidence, web compatibility, exact
commands/results, files changed, blockers/risks, then update task/plan status.

## Results

- Added lifecycle/executor list, exact resolve, and generation-safe internal cancel operations over
  typed agentctl snapshots without exposing process-manager state above the runtime seam.
- Added `ListPendingAgentPermissions` and `ResolveAgentPermission`: strict task/session
  authorization and server-owned pairing precede runtime/audit lookup; claims precede delivery;
  every known delivery outcome is finalized with a stable domain error.
- Task-wide lists are deterministic, skip sessions with no live execution, and fail rather than
  return partial data on any other live enumeration error.
- Audit actor derivation distinguishes browser, PAT, automation, and synthetic callers while never
  retaining PAT token IDs. Existing web option selection now uses the strict service; the
  no-reject dismissal fallback uses the separate internal exact-generation cancel path.
- Passed:
  `env GOCACHE=/tmp/kandev-go-build GOMODCACHE=/tmp/kandev-go-mod /home/bazil/.local/share/mise/installs/go/1.26.0/bin/go test -tags fts5 ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/orchestrator ./internal/orchestrator/handlers ./internal/backendapp`.
