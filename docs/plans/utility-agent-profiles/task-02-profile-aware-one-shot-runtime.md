---
id: "02-profile-aware-one-shot-runtime"
title: "Build profile-aware one-shot runtime"
status: completed
wave: 2
depends_on: ["01-persist-profile-bindings"]
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 02: Build profile-aware one-shot runtime

## Intent

Resolve one complete agent-profile launch snapshot and apply it consistently to host-sessionless
and task-session-bound utility prompts, including explicit non-interactive permission behavior.

## Acceptance

- Both one-shot paths apply the selected profile's agent, model, mode/config options, enabled flags,
  command prefix, resolved environment/secrets, strip-env rules, and auto-approval policy.
- Missing/ineligible profiles, bad prefixes, and unresolved secrets fail closed before dispatch;
  execution never retries with agent defaults or stripped-down configuration.
- ACP permission requests auto-approve only when the profile says so; otherwise they are rejected
  promptly with an actionable error and never enter the interactive timeout.

## Files likely touched

- `apps/backend/internal/agent/profileexecution/resolver.go`
- `apps/backend/internal/agent/profileexecution/resolver_test.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agent/hostutility/public.go`
- `apps/backend/internal/agent/hostutility/manager_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/utility.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_env.go`
- `apps/backend/internal/agentctl/server/utility/types.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor_test.go`
- `apps/backend/internal/utility/handlers/handlers.go`
- `apps/backend/internal/utility/handlers/handlers_test.go`
- `apps/backend/internal/utility/handlers/handlers_integration_test.go`
- `apps/backend/internal/backendapp/helpers.go`

## Dependencies

Task 01 profile-ID persistence and utility API contract.

## Parallelism

Parallel-safe with task 04 after task 01: this task owns backend/runtime files while task 04 owns
frontend files. Sequential execution remains the default.

## Inputs

- Spec: `What`, `Permissions`, `Failure modes`, and the first six `Scenarios`.
- Plan: `Shared profile-aware one-shot launch` and `Utility CRUD and execution preparation`.
- Existing patterns: lifecycle `resolveProfileLaunchTokens`, `StoreProfileResolver`,
  `resolveAgentProfileEnvVars`, hostutility warm instances, and agentctl
  `applySessionConfigOptions`/ACP permission handlers.

## Verification

```bash
cd apps/backend && go test ./internal/agent/profileexecution/... ./internal/agent/hostutility/... ./internal/agentctl/server/utility/...
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... -run 'Test.*Inference|Test.*Profile.*Env|Test.*Utility'
cd apps/backend && go test ./internal/utility/handlers/...
```

## Output contract

Report the launch snapshot shape, host/session parity, explicit permission semantics, secret/trust
boundaries, files changed, exact test results, blockers, risks, and synchronized task/plan status.
Do not update plugin/review consumers or frontend settings here.

## Results

Implemented profile snapshot dispatch for host and task-session utility calls. Profile model, mode, config options, environment, flags, command prefix, and auto-approval policy cross the agentctl boundary; disabled profiles reject permission requests. Verified with focused lifecycle, hostutility, agentctl utility, and utility handler tests.
