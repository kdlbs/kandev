---
id: "02-align-shared-launch-and-docs"
title: "Align shared launch and docs"
status: done
wave: 2
depends_on: ["01-unify-setup-timeout-paths"]
plan: "plan.md"
spec: "../../specs/platform/setup-launch-timeout.md"
---

# Task 02: Align Shared Launch and Docs

## Acceptance

- Shared execution creation can cross 60 seconds and remains bounded by
  `AgentLaunchTimeout`.
- Manager shutdown and per-caller cancellation keep their existing coalescing
  behavior, and a deadline releases the activity lease.
- Public configuration and executor docs describe the one environment variable
  and the derived default launch limit.

## Verification

```bash
cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers && cd ../.. && git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution_test.go`
- `apps/backend/internal/backendapp/adapters_plugin_starter_test.go`
- `apps/backend/internal/orchestrator/task_operations_wait_test.go`
- `docs/public/configuration.md`
- `docs/public/executors.md`

## Dependencies

Task 01.

## Parallelism

`sequential`. This task verifies and documents Task 01's common timeout policy.

## Inputs

- Setup and Launch Timeout spec.
- Task 01's resolved `SetupScriptTimeout` and `AgentLaunchTimeout` values.
- Existing coalescing tests in `manager_execution_test.go`.
- GitHub issue #2574 and its 90-second setup reproduction.

## Output contract

Report the RED 90-second virtual-time regression, shared-launch change, public
documentation updates, exact verification results, residual risks, and update
this task plus `plan.md` status.

## Results

- Replaced the fixed shared execution-creation deadline with the setup timeout plus the fixed five-minute launch allowance, preserving caller cancellation and manager-stop behavior.
- RED verification first demonstrated that a setup operation extending past the old 60-second deadline was cancelled; the regression now passes with the new launch budget.
- Documented the environment variable, duration parsing, executor coverage, launch allowance, restart requirement, and unchanged setup failure semantics in the public executor/configuration docs.
- Verification: `cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/task/handlers ./internal/mcp/handlers` passed with 4,267 tests across 4 packages.
- Focused lifecycle regressions passed under `go test -race`; `git diff --check` passed.
