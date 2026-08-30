---
id: "01-runtime-rollout-flag"
title: "Runtime rollout flag"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 01: Runtime rollout flag

- **Acceptance:** Register `features.dynamicAgentRouting` and
  `KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING` as mutable, experimental, high risk,
  and restart-required. Default prod, dev, and E2E to false. Prove that all
  disabled entry points fail before side effects while concrete launches remain
  unchanged.
- **Files likely touched:** `profiles.yaml`,
  `apps/backend/internal/common/config/config.go`,
  `apps/backend/internal/runtimeflags/{registry,config}*.go`,
  `apps/web/lib/state/slices/features/*`.
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** Spec Delivery and rollout, the `/runtime-feature-flags` checklist,
  `profiles.yaml`, and `runtimeflags/registry.go` definitions.
- **Output contract:** Report the flag identity, registry metadata, profile
  defaults, gated entry points, exact commands and results, blockers, risks,
  and synchronized task and plan status.
- **Verification:** `make -C apps/backend lint && cd apps/backend && go test ./internal/runtimeflags ./internal/common/config ./internal/profiles && cd ../../apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts && cd web && pnpm run typecheck`
- **Risks:** Every backend entry point must be gated. Frontend hiding alone is
  insufficient.

## Results

Completed. Registered `features.dynamicAgentRouting` and
`KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING` as mutable, experimental, high-risk,
and restart-required. Production, development, and E2E profile defaults are
off. Backend validation blocks disabled dynamic profile mutation and launch
before side effects while concrete profiles retain their existing path.

Verification:

- `go test ./internal/runtimeflags ./internal/profiles ./internal/common/config`
- `pnpm --filter @kandev/web test -- --run lib/state/slices/features/features-contract.test.ts`

Both commands passed. Full E2E flag-matrix coverage remains pending.
