---
id: "04-core-route-engine"
title: "Core route engine"
status: in_progress
wave: 4
depends_on: ["03-dynamic-profile-management"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 04: Core route engine

- **Acceptance:** Implement fixed ordering, configured compatibility error actions,
  durable route attempts and state, the adapter binding-descriptor contract,
  HMAC fingerprint derivation, scope-specific circuits, exclusive half-open
  probe leases, and single-owner route generations without plugin dependencies.
- **Files likely touched:** `apps/backend/internal/agent/runtime/dynamic/**`,
  `apps/backend/internal/agent/runtime/routingerr/**`,
  `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`,
  `apps/backend/internal/task/repository/sqlite/{base_schema,base_migrations}.go`,
  `apps/backend/internal/task/repository/sqlite/*dynamic_route*`, and reusable
  logic migrated from `apps/backend/internal/office/routing/`.
- **Dependencies:** Task 03.
- **Parallelism:** sequential.
- **Inputs:** Spec Route selection, Shared health and probing, Data model, State
  machine, and circuit scenarios, Task 03, existing Office backoff and
  `runtime/routingerr` behavior.
- **Output contract:** Report descriptor producers for every supported concrete
  family, canonical fields, installation-key persistence, fallback isolation,
  resource scopes, schema changes, files changed, exact commands and results,
  blockers, risks, and synchronized task and plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/dynamic/... ./internal/agent/runtime/routingerr/... ./internal/task/repository/sqlite/...`
- **Risks:** Unknown bindings must isolate by profile. Raw secrets, literal
  environment values, command prefixes, and account IDs cannot enter keys or
  logs. A missing adapter descriptor cannot silently use provider-wide scope.
  The generic action map remains a compatibility boundary; the separate
  Provider Error Policies package owns its class-policy replacement.

## Results

In progress. Implemented fixed-order selection, semantic retry/next actions,
durable route state and attempts, HMAC installation-key fingerprints,
profile-scoped unknown bindings, circuit state and half-open probe leases, and
generation-checked claims. Added restart loading and atomic SQLite claim tests.
The rollout-blocker repair now connects concrete binding descriptors to route
candidates, opens qualifying circuits, releases probe leases on production
failures, and routes expired circuits through exclusive probes.

Verification:

- `go test -tags fts5 ./internal/agent/runtime/dynamic ./internal/task/repository/sqlite -run "Test(DynamicRoute|Engine|Circuit|SQLiteRepository)" -count=1`

The command passed. Family-specific descriptor producers, complete automatic
circuit close observability, and the broader observability migration remain
open.
