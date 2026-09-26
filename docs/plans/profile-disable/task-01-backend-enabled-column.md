---
id: "01-backend-enabled-column"
title: "Backend enabled column for agent profiles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/profile-disable.md"
---

# Task 01: Backend enabled column

- **Acceptance:** `agent_profiles` gains an `enabled INTEGER NOT NULL DEFAULT 1` column via idempotent migration; existing rows backfill to enabled; the model/DTO/controller/handler persist and expose it; `PATCH /api/v1/agent-profiles/:id` with `{"enabled": false}` round-trips.
- **Acceptance:** A profile created through `CreateProfile` is enabled by default (explicit `Enabled: true`, not the Go zero value).
- **Acceptance:** Focused Go tests cover store create/update/scan round-trip, migration backfill, and the PATCH path.
- **Verification:**
  - `cd apps/backend && go test ./internal/agent/settings/store/... ./internal/agent/settings/controller/... ./internal/agent/settings/handlers/...`
  - `cd apps/backend && go build ./...`
- **Files likely touched:**
  - `apps/backend/internal/agent/settings/models/models.go`
  - `apps/backend/internal/agent/settings/dto/dto.go`
  - `apps/backend/internal/agent/settings/store/sqlite.go`
  - `apps/backend/internal/agent/settings/controller/profile_crud.go`
  - `apps/backend/internal/agent/settings/handlers/handlers.go`
  - new tests: `apps/backend/internal/agent/settings/store/sqlite_profile_enabled_test.go`, additions to `controller/profile_crud_test.go` and `handlers/agent_update_handlers_test.go` as appropriate.
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** spec sections "Data model", "API surface", "Scenarios"; plan "Backend" section. The store file already centralizes the read projection in `agentProfileSelectColumns` and documents the column-add rule — follow it.
- **Output contract:** Report red/green test evidence, migration name, exact SQL diff, changed files, targeted test results, risks, and task/plan status update.
