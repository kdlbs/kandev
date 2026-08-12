---
id: "01-normalize-empty-bindings"
title: "Normalize empty utility bindings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/utility-agent-profiles.md"
---

# Task 01: Normalize empty utility bindings

Repair legacy built-in rows before changing the settings save path.

- **Acceptance:** A built-in with an empty profile ID and `unconfigured` state is persisted as
  `inherit`, and a second migration run makes no further update.
- **Acceptance:** A concrete stale built-in binding and every custom unconfigured binding remain
  unchanged and fail closed.
- **Acceptance:** Migration preserves the row's existing `enabled` value and does not copy a concrete
  default profile ID.
- **Verification:** Add the failing migration tests first, then run:

  ```bash
  cd apps/backend
  go test -run 'TestMigrateLegacyBindings' ./internal/utility/service/...
  ```

- **Files likely touched:** `apps/backend/internal/utility/service/service.go` and
  `apps/backend/internal/utility/service/service_test.go`.
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** The legacy migration, failure-mode, persistence, and scenario sections in
  `docs/specs/agents/utility-agent-profiles.md`, plus
  ADR-2026-08-12-empty-utility-bindings-inherit-default.
- **Output contract:** Report files changed, the RED and GREEN test commands and results, idempotency
  evidence, preserved stale/custom behavior, and synchronized task/plan status.

## Results

Implemented idempotent normalization for empty `unconfigured` built-in rows. The migration now
persists `inherit` without changing `enabled`. Concrete stale built-in bindings and custom
unconfigured bindings remain unchanged.

- RED: `cd apps/backend && go test -run 'TestMigrateLegacyBindingsNormalizesEmptyUnconfiguredBuiltin' ./internal/utility/service/...`
  (failed as expected: `updated = 0, want 1`)
- GREEN: `cd apps/backend && go test -run 'TestMigrateLegacyBindingsNormalizesEmptyUnconfiguredBuiltin' ./internal/utility/service/...`
  (pass: 1 test)
- REFACTOR: `cd apps/backend && go test -run 'TestMigrateLegacyBindings' ./internal/utility/service/...`
  (pass: 5 tests)
- Generated artifacts: None.
- Cleanup: No temporary artifacts.
- Security or external side effects: None. Tests use the in-memory fake repository.
