---
id: "10-office-routing-handoff"
title: "Office routing handoff"
status: pending
wave: 8
depends_on: ["06-logical-session-integration", "08-dynamic-profile-settings"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 10: Office routing handoff

- **Acceptance:** Persist Office `execution_agent_profile_id`, use the shared
  resolver, remove legacy routing endpoints, UI, seeding, runtime ownership,
  `StartTaskWithRoute`, and the full `RouteOverride` overlay chain. Prove all
  three legacy tables and rows survive upgrade unchanged and unread.
- **Files likely touched:** `apps/backend/internal/office/{scheduler,onboarding,dashboard,service,routing,repository/sqlite}/**`,
  `apps/backend/internal/orchestrator/task_operations.go`,
  `apps/backend/internal/orchestrator/executor/{executor,executor_execute}.go`,
  `apps/backend/internal/backendapp/{main,adapters}.go`,
  `apps/backend/internal/agent/runtime/lifecycle/{manager_launch,types}.go`,
  `apps/backend/internal/agent/registry/canonical_routing_providers.go`,
  `apps/web/app/office/**`, `apps/web/hooks/domains/office/**`,
  `apps/web/lib/api/domains/office-routing-api.ts`,
  `apps/web/e2e/tests/office/{office-routing-*,mobile-provider-routing-profiles}.spec.ts`,
  `apps/web/e2e/helpers/office-routing.ts`, and `AGENTS.md`.
- **Dependencies:** Tasks 06 and 08.
- **Parallelism:** sequential.
- **Inputs:** Spec Use in Office, Delivery and rollout, and legacy-row scenario,
  ADR Legacy Office routing data, Tasks 06 and 08, the existing Office routing
  package and route-override launch chain.
- **Output contract:** Report removed runtime owners and tests, retained schema
  evidence, flag-matrix behavior, files changed, exact commands and results,
  blockers, risks, and synchronized task and plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/office/... ./internal/orchestrator/... ./internal/backendapp/... ./internal/agent/runtime/lifecycle/... && cd ../../apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- app/office hooks/domains/office lib/state/slices/office`
- **Risks:** Do not drop, rewrite, or display legacy Office routing rows.
  Unrelated Office data and disabled-mode behavior must remain valid.

## Results

Not started in this implementation slice. The existing Office routing chain,
legacy runtime ownership, and retained-row verification remain unchanged.
