---
id: "02-resolution-and-routing-persistence"
title: "Resolver and route persistence"
status: done
wave: 2
depends_on: ["01-routing-profile-contract"]
plan: "plan.md"
spec: "../../specs/office/routing.md"
---

# Task 02: Resolver and route persistence

**Acceptance:** each candidate carries `execution_profile_id`; provider/model are derived snapshots; disabled routing selects exactly the first effective-tier profile without fallback; enabled routing preserves ordered health-aware fallback; runs and route attempts persist and expose the execution profile.

**TDD cases:** wake-reason and agent tier precedence; disabled single-candidate behavior; enabled fallback order; excluded/degraded candidates; missing first profile; run/attempt round-trip and restart recovery.

**Likely files:** `apps/backend/internal/office/routing/resolver.go`, `apps/backend/internal/office/scheduler/dispatch_routing.go`, `apps/backend/internal/office/models/models.go`, `apps/backend/internal/office/repository/sqlite/{base.go,base_migrations.go,run_routing.go,route_attempts.go}`, dashboard route/run DTOs, and tests.

**Verification:** focused routing/scheduler/office repository tests; SQLite and Postgres replay migration tests where supported.

**Dependencies:** Task 01.

**Output contract:** report candidate semantics, persisted fields, migrations, tests, and any compatibility fallback used.
