---
id: "03-dual-id-session-plumbing"
title: "Dual-ID session plumbing"
status: done
wave: 2
depends_on: ["01-routing-profile-contract"]
plan: "plan.md"
spec: "../../specs/office/routing.md"
---

# Task 03: Dual-ID session plumbing

**Acceptance:** scheduler, orchestrator, executor, and lifecycle launch contracts carry both IDs explicitly; task/session ownership remains keyed by Office identity; session and `executors_running` snapshots persist the execution profile; non-Office launch behavior remains compatible.

**TDD cases:** Office identity differs from execution profile; direct non-Office launch aliases IDs; session ensure/reuse keeps Office ownership; executor snapshot round-trip; restart recovers the concrete profile; cross-workspace execution profile rejected before launch.

**Likely files:** Office scheduler starter interfaces, `apps/backend/internal/backendapp/{main.go,adapters.go}`, orchestrator task operations, orchestrator executor request plumbing, task models/API DTOs, task SQLite session/executor schema and migrations, runtime launch types, and tests.

**Verification:** focused scheduler/backendapp/orchestrator/task repository/runtime contract tests plus migration replay coverage.

**Dependencies:** Task 01. Can run in parallel with Task 02 after the candidate contract is fixed.

**Output contract:** report the two-ID call chain, persistence changes, compatibility behavior, tests, and unresolved call sites.
