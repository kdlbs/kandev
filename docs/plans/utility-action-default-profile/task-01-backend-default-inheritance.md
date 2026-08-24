---
id: "01-backend-default-inheritance"
title: "Repair backend default inheritance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 01: Repair backend default inheritance

Implement the amended utility-agent binding contract before changing the settings UI.

- **Acceptance:** Built-in rows in the inherited or legacy migration state without a concrete profile
  inherit the saved default. Empty `unconfigured` rows from older releases remain fail-closed, and
  custom unmatched rows remain `unconfigured`.
- **Acceptance:** Deleting an explicit profile preserves its stale ID and keeps execution
  fail-closed. Deleting the global default does not convert inherited built-ins into repair state.
- **Acceptance:** Plugin reads and utility execution use the same effective-default predicate.
- **Verification:** Add the failing service tests first, then run:

  ```bash
  cd apps/backend
  go test -run 'Test(MigrateLegacyBindings|PreparePromptRequest|ClearAgentProfileBindings)' ./internal/utility/service/...
  go test ./internal/utility/service/... ./internal/utility/profilebinding/...
  ```

- **Files likely touched:** `apps/backend/internal/utility/models/models.go`,
  `apps/backend/internal/utility/service/service.go`,
  `apps/backend/internal/utility/service/service_test.go`,
  `apps/backend/internal/backendapp/orchestrator.go`, and
  `apps/backend/internal/backendapp/services.go`.
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** The amended legacy migration, failure-mode, persistence, and scenario sections in
  `docs/specs/agents/requirements/utility-agent-profiles.md`, plus ADR-2026-08-12-built-in-utility-default-inheritance.
- **Output contract:** Report the backend files changed, exact test commands and results, stale-ID
  and default-deletion behavior, and synchronized task/plan status.

## Results

Implemented the shared inherited-built-in predicate, legacy migration normalization, default-backed
execution, stale explicit-ID preservation, and default-aware plugin/dependency reads. Empty
`unconfigured` rows remain fail-closed because older releases could erase deleted profile IDs.
Deleting a global default no longer rewrites inherited built-ins into repair state.

Verification:

- `cd apps/backend && go test -run 'Test(MigrateLegacyBindings|PreparePromptRequest|ClearAgentProfileBindings)' ./internal/utility/service/...` (pass: 13 tests)
- `cd apps/backend && go test ./internal/utility/service/... ./internal/utility/profilebinding/...` (pass: 17 tests)
- `cd apps/backend && go test -run 'TestPluginsUtilityAgentAdapter' ./internal/backendapp` (pass: 3 tests)
