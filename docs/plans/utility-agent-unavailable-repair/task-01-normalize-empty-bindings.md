---
id: "01-normalize-empty-bindings"
title: "Normalize empty utility bindings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 01: Normalize empty utility bindings

Repair legacy built-in rows before changing the settings save path.

- **Acceptance:** A built-in with an empty profile ID and `unconfigured` state is persisted as
  `inherit`, and a second migration run makes no further update.
- **Acceptance:** A concrete stale built-in binding and every custom unconfigured binding remain
  unchanged and fail closed.
- **Acceptance:** Migration preserves the row's existing `enabled` value and does not copy a concrete
  default profile ID.
- **Acceptance:** A concurrent concrete repair cannot be overwritten by the startup normalization.
- **Verification:** Add the failing migration tests first, then run:

  ```bash
  cd apps/backend
  go test -run 'TestMigrateLegacyBindings' ./internal/utility/service/...
  ```

- **Files likely touched:** `apps/backend/internal/utility/service/service.go`,
  `apps/backend/internal/utility/service/service_test.go`,
  `apps/backend/internal/utility/store/interface.go`, `apps/backend/internal/utility/store/sqlite.go`,
  and `apps/backend/internal/utility/store/sqlite_migration_test.go`.
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** The legacy migration, failure-mode, persistence, and scenario sections in
  `docs/specs/agents/requirements/utility-agent-profiles.md`, plus
  ADR-2026-08-12-empty-utility-bindings-inherit-default.
- **Output contract:** Report files changed, the RED and GREEN test commands and results, idempotency
  evidence, preserved stale/custom behavior, and synchronized task/plan status.

## Results

Implemented idempotent, conditional normalization for empty `unconfigured` built-in rows. The
migration now persists only the binding-state field to `inherit` without changing `enabled`, and a
stale-row predicate prevents a concurrent concrete repair from being overwritten. Concrete stale
built-in bindings and custom unconfigured bindings remain unchanged.

- RED: `cd apps/backend && go test -run 'TestMigrateLegacyBindingsNormalizesEmptyUnconfiguredBuiltin' ./internal/utility/service/...`
  (failed as expected: `updated = 0, want 1`)
- GREEN: `cd apps/backend && go test -run 'TestMigrateLegacyBindingsNormalizesEmptyUnconfiguredBuiltin' ./internal/utility/service/...`
  (pass: 1 test)
- REFACTOR: `cd apps/backend && go test -run 'TestMigrateLegacyBindings' ./internal/utility/service/...`
  (pass: 5 tests)
- Concurrency guard: `cd apps/backend && go test -run 'TestMigrateLegacyBindingsDoesNotOverwriteConcurrentRepair' ./internal/utility/service/...`
  (pass: conditional repository fake preserves the concurrent explicit repair)
- SQLite predicate: `cd apps/backend && go test -run 'TestNormalizeEmptyBuiltinBindingUsesStalePredicate' ./internal/utility/store/...`
  (pass: first stale row updates, second repaired row is not changed)
- Package verification: `cd apps/backend && go test ./internal/utility/... ./internal/backendapp/...`
  (pass: 378 tests) and `go vet ./internal/utility/...` (pass).
- Fixup review: preserved the stale concrete profile ID assertion and added a conditional SQLite
  update so concurrent settings repairs cannot be overwritten.
- Generated artifacts: None.
- Cleanup: No temporary artifacts.
- Security or external side effects: None. Tests use the in-memory fake repository.
