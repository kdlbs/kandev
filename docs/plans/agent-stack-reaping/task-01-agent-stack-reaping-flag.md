---
id: "01-agent-stack-reaping-flag"
title: "Runtime flag features.agentStackReaping"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../../specs/platform/agent-stack-reaping.md"
---

# Task 01: Runtime flag features.agentStackReaping

## Acceptance

- `profiles.yaml` declares `KANDEV_FEATURES_AGENT_STACK_REAPING` default
  `"false"` in prod, dev, and e2e for a controlled experimental rollout.
- `config.FeaturesConfig.AgentStackReaping` carries explicit mapstructure/json
  tags; the runtimeflags registry owns the public metadata; the frontend
  feature defaults declare the key.
- Orchestrator `ServiceConfig.AgentStackReaping` is wired from
  `cfg.Features.AgentStackReaping` in `backendapp.provideOrchestrator`.
- Orchestrator `ServiceConfig.AgentStackIdleTTL` is wired from the existing
  operator setting `cfg.Agentctl.IdleTimeout`.

## Verification

- `cd apps/backend && go test ./internal/runtimeflags ./internal/common/config ./internal/profiles`
- `cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts`

## Files

- `profiles.yaml`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/web/lib/state/slices/features/types.ts`
