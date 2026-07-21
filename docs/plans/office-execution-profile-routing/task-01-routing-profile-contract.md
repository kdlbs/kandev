---
id: "01-routing-profile-contract"
title: "Execution-profile routing contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/routing.md"
---

# Task 01: Execution-profile routing contract

**Acceptance:** routing JSON stores authoritative execution profile IDs; legacy keys decode safely; validation rejects deleted, cross-workspace, unlaunchable, or wrong-provider profiles; routing GET returns selectable profile summaries; deletion guards use the authoritative references.

**TDD cases:** canonical/legacy JSON round-trip; unique legacy provider/model migration; missing and ambiguous migration; provider-key mismatch; global and same-workspace compatibility; referenced-profile deletion.

**Likely files:** `apps/backend/internal/office/routing/types.go`, routing validators/profile lookup helpers, `apps/backend/internal/office/repository/sqlite/workspace_routing.go`, `apps/backend/internal/office/dashboard/handler_routing.go`, routing DTOs, settings deletion guards, and focused tests.

**Verification:** focused routing/repository/dashboard/settings Go tests, then `make -C apps/backend test` for affected packages.

**Dependencies:** None.

**Output contract:** summarize schema/API compatibility, migration outcomes, tests, files, and remaining risks.
