---
id: "01-backend-default-inheritance"
title: "Repair backend default inheritance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/utility-agent-profiles.md"
---

# Task 01: Repair backend default inheritance

Implement the amended utility-agent binding contract before changing the settings UI.

- **Acceptance:** Built-in rows without a concrete profile inherit the saved default, including
  legacy empty or ambiguous rows. Custom unmatched rows remain `unconfigured`.
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
  `docs/specs/agents/utility-agent-profiles.md`, plus ADR-2026-08-12-built-in-utility-default-inheritance.
- **Output contract:** Report the backend files changed, exact test commands and results, stale-ID
  and default-deletion behavior, and synchronized task/plan status.

## Results

Implemented the shared empty-built-in predicate, legacy normalization, default-backed execution,
stale explicit-ID preservation, and default-aware plugin/dependency reads. Deleting a global default
no longer rewrites inherited built-ins into repair state.

Verification:

- `cd apps/backend && go test -run 'Test(MigrateLegacyBindings|PreparePromptRequest|ClearAgentProfileBindings)' ./internal/utility/service/...` (pass: 11 tests)
- `cd apps/backend && go test ./internal/utility/service/... ./internal/utility/profilebinding/...` (pass: 15 tests)
- `cd apps/backend && go test -run 'TestPluginsUtilityAgentAdapter' ./internal/backendapp` (pass: 3 tests)
