---
id: "02-virtual-profile-foundation"
title: "Virtual profile foundation"
status: completed
wave: 2
depends_on: ["01-runtime-rollout-flag"]
plan: "plan.md"
spec: "../../specs/agents/requirements/dynamic-agent-routing.md"
---

# Task 02: Virtual profile foundation

- **Acceptance:** Seed the permanent `dynamic` family before reconciliation,
  keep it out of concrete inference probing, expose computed profile kind, and
  persist versioned dynamic configuration plus ordered concrete candidates.
- **Files likely touched:** `apps/backend/internal/agent/agents/dynamic.go`,
  `apps/backend/internal/agent/registry/registry.go`,
  `apps/backend/internal/agent/settings/models/models.go`,
  `apps/backend/internal/agent/settings/dto/dto.go`,
  `apps/backend/internal/agent/settings/store/sqlite.go`,
  `apps/backend/internal/agent/settings/controller/reconciler*.go`.
- **Dependencies:** Task 01.
- **Parallelism:** sequential.
- **Inputs:** Spec Profile configuration and Data model, Task 01 flag contract,
  current agent registry, settings SQLite schema, and reconciler patterns.
- **Output contract:** Report the seeded family invariant, schema changes,
  files changed, exact commands and results, blockers, risks, and synchronized
  task and plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/agent/registry/... ./internal/agent/settings/...`
- **Risks:** The family must survive orphan cleanup without becoming launchable or receiving a default concrete profile.

## Results

Completed. Added the permanent non-launchable `dynamic` family, computed
profile-kind normalization, versioned dynamic configuration, ordered route
persistence, and reconciliation safeguards.

Verification:

- `go test -tags fts5 ./internal/agent/registry/... ./internal/agent/settings/...`

The command passed. Dedicated migration and E2E coverage remain pending.
