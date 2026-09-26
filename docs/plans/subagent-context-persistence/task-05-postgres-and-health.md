---
id: "05-postgres-and-health"
title: "Postgres parity and health-query coverage"
status: pending
wave: 5
depends_on: ["04-orchestrator-wiring"]
plan: "plan.md"
spec: "../../specs/agents/requirements/subagent-context-persistence.md"
---

> **Current implementation boundary:** Postgres parity covers the final
> execution-aware schema and the historical-message backfill. The feature does
> not ship a compatibility rebuild for an earlier table shape; see
> `docs/decisions/2026-08-15-subagent-context-schema-boundary.md`.

# Task 05: Postgres parity and health-query coverage

Prove the dialect-sensitive statements behave identically on PostgreSQL, and pin
the AC-28 expected-versus-observed health comparison.

## Acceptance

1. Against `KANDEV_TEST_POSTGRES_DSN`, a fresh schema plus a migration replay
   produces the same table, the same backfilled rows, and the same two
   activation keys as SQLite. (AC-19)
2. The upsert's conflict clause is verified *behaviorally* on Postgres —
   fill-forward, write-once `settled_at`, sticky `is_async`, preserved original
   `turn_id` — not merely replayed. The backfill's JSON helpers are verified over
   `jsonb`, including `''` and `'null'` metadata rows and the boolean spelling
   difference (`'true'` vs `1`). (AC-19, AC-23b)
3. The AC-28 comparison is asserted both ways: the two counts agree after live
   writes, and seeding a `subagent_task` message the writer never saw makes them
   diverge, with `MAX(observed_at)` locating the divergence in time. (AC-28,
   AC-29)

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/subagent_context_postgres_test.go`
  — new.
- `apps/backend/internal/task/repository/sqlite/subagent_context_health_test.go`
  — new.

## Dependencies

Task 04 — the health check compares rows written by the live path.

## Parallelism

`sequential`.

## Inputs

- Spec AC-19, AC-23b, AC-28, AC-29, and § *Writer health* — in particular *why*
  the message count is a valid independent expectation: the message write is
  load-bearing for the UI and a break is noticed within one turn, whereas a
  broken context write is noticed by nobody. Keep that asymmetry in the test
  comment; it is the reason the check is real.
- ADR `docs/decisions/0027-replayable-schema-migrations.md` and
  `apps/backend/AGENTS.md` § *Schema & migrations*: "add an environment-gated
  PostgreSQL behavior test for every changed dialect-sensitive method (schema
  replay is insufficient)".
- Patterns: `postgres_schema_test.go` (env-gated fresh-schema + replay),
  `document_plan_postgres_test.go` (asserting `ON CONFLICT` updates rather than
  inserts on Postgres), `testutil.OpenIsolatedPostgres` /
  `testutil.PostgresDSNFromEnv` (`internal/testutil/postgres.go:45`).
- The AC-28 query is dialect-specific (`json_extract` vs `#>>`); reuse the
  helpers from task 01 rather than hand-writing it twice.

## Verification

```
cd apps/backend && gofmt -l internal/task/repository/sqlite && make lint && go test -run 'TestSubagentContextHealth' ./internal/task/repository/sqlite/... && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test -run 'Postgres.*Subagent|Subagent.*Postgres' ./internal/task/repository/sqlite/...
```

If `KANDEV_TEST_POSTGRES_DSN` is unset the Postgres run **skips rather than
fails**. A skip is not evidence for AC-19: record it explicitly as a skip in
`## Results` and state that AC-19 remains unverified locally, rather than
reporting the task fully verified.

## Results

Pending. Before marking the task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record whether the Postgres suite ran or skipped. Record
security/trust and external side-effect boundaries when applicable, or explicitly
state `None`.
