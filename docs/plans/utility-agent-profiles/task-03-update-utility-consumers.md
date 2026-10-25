---
id: "03-update-utility-consumers"
title: "Update utility execution consumers"
status: completed
wave: 3
depends_on: ["01-persist-profile-bindings", "02-profile-aware-one-shot-runtime"]
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 03: Update utility execution consumers

## Intent

Route plugin utility calls and native code review through effective profile IDs so no backend
consumer retains the raw agent/model bypass.

## Acceptance

- Plugin configuration still selects a utility-agent ID, but invocation resolves and runs that
  agent's effective profile; missing, disabled, or unconfigured selections remain typed
  `FailedPrecondition` failures.
- Native review precedence is explicit workflow profile, `code-review` override, then default
  utility profile; every branch passes a profile ID to session execution and host fallback.
- Repository search finds no production utility execution adapter that accepts a raw agent/model
  pair on behalf of a configured utility agent.

## Files likely touched

- `apps/backend/internal/plugins/host_utility.go`
- `apps/backend/internal/plugins/host_utility_test.go`
- `apps/backend/internal/plugins/host_data_wire_test.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/services_plugin_utility_test.go`
- `apps/backend/internal/review/resolver.go`
- `apps/backend/internal/review/resolver_test.go`
- `apps/backend/internal/backendapp/review_wiring.go`
- `apps/backend/internal/backendapp/review_wiring_test.go`
- `apps/backend/internal/workflow/engine/run_code_review_test.go`

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential after task 02 because it compiles directly against the new runtime interfaces and
adapters.

## Inputs

- Spec: plugin/code-review bullets in `What`, `API surface`, unavailable-profile failure modes, and
  stale/disabled scenarios.
- Plan: `Plugin and review consumers`.
- Existing patterns: ADR 0048 cycle-avoidance adapters, review source precedence, and
  `reviewInference` session-to-host fallback.

## Verification

```bash
cd apps/backend && go test ./internal/plugins/... ./internal/review/...
cd apps/backend && go test ./internal/backendapp/... -run 'Test.*Plugin.*Utility|Test.*Review'
cd apps/backend && go test ./internal/workflow/engine/... -run 'Test.*CodeReview'
```

## Output contract

Report updated consumer interfaces and precedence, typed failure mappings, search evidence for
removed raw-pair paths, files changed, exact test results, blockers, risks, and synchronized
task/plan status. Do not alter plugin manifest selection or public protobuf signatures.

## Results

Updated plugin invocation and native review resolution to pass profile IDs. Missing, disabled, stale, passthrough, and unconfigured bindings fail closed. Verified with plugin, review, workflow-engine, and backendapp tests.
