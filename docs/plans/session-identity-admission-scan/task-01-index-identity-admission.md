---
id: "01-index-identity-admission"
title: "Index the session identity admission query"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-WRITE-COST-001
  - REQ-PLATFORM-DELIVERY-RETENTION-004
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-WRITE-COST-001.5
  - AC-PLATFORM-DELIVERY-RETENTION-004.3
  - AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3
system_design:
  - ../../specs/platform/system-design/durable-delivery-write-cost.md
  - ../../specs/platform/system-design/postgres-domain-store-parity.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 01: Index the session identity admission query

## Summary

Replace the `OR` across `LEFT JOIN` in `ensureAgentDeliveryIdentityAvailable`
with a direct, indexed lookup on effects. Session creation then stops scanning
`agent_delivery_effects` while it holds the SQLite writer.

## In scope

- `apps/backend/internal/task/repository/sqlite/agent_delivery_retention_admission.go`
  (`ensureAgentDeliveryIdentityAvailable` only).
- New test file
  `apps/backend/internal/task/repository/sqlite/agent_delivery_identity_admission_test.go`.
- Re-measure the final statement on the disposable snapshot copy and append the
  result to `evidence.md`.

## Out of scope

`ensureAgentDeliverySessionNotRetired`, other retention queries, schema or
index changes, transaction scope, health policy, and live-instance load.

## Acceptance

1. `TestAgentDeliveryIdentityAdmissionUsesIndexes` runs `EXPLAIN QUERY PLAN` on
   the production statement using the real SQLite factory. It fails if any plan
   row starts with `SCAN` for a delivery table. It fails before the change
   (`SCAN e`) and passes after.
2. `TestAgentDeliveryIdentityAdmissionRejectsRetainedReferences` returns
   `repoerrors.ErrAgentDeliveryIdentityRetiring` for a session referenced by
   each of the following:
   - a cleanup job
   - a submission
   - an inbox row
   - a cursor
   - an effect with that `session_id`
   - an effect with an empty `session_id` whose stream cursor names the session

   It returns nil for an unrelated session ID while those rows exist. The test
   passes both before and after the change. It runs against PostgreSQL when
   `KANDEV_TEST_POSTGRES_DSN` is set.
3. The final statement runs in under 100 ms cold on the snapshot copy and
   matches the original for the sampled IDs. This is recorded evidence, not a
   CI assertion.

## Implementation sequence

1. Mark this work order `in_progress`.
2. Add both tests. Reuse the package's existing file-backed repository helper
   and `assertRetentionSQLiteDiscoveryOnly`-style plan scanning. Run them, and
   keep the expected `SCAN e` failure from test 1.
3. Replace the last `UNION ALL` branch with
   `SELECT 1 FROM agent_delivery_effects WHERE session_id = ? AND session_id <> ''`.
   Drop the now-unused bind arguments.
4. Run the commands below.
5. Re-run the timing and equivalence script from `evidence.md` against
   `/tmp/kandev-contention-repro/kandev.db`. Record the results, then delete the
   copy.
6. Mark this work order `done` and update `plan.md`.

## Verification

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run 'AgentDeliveryIdentityAdmission|Retention|Retire|ManagedConversationAdmission' -count=1
go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -count=1
go test -trimpath -tags fts5 -race ./internal/task/service -run 'AgentConversation|ManagedConversationAdmission' -count=1
go run ./cmd/sqlguard ./internal
go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1
cd ../.. && bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/agent_delivery_retention_admission.go apps/backend/internal/task/repository/sqlite/agent_delivery_identity_admission_test.go
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
git diff --check
```

Set `KANDEV_TEST_POSTGRES_DSN` for PostgreSQL coverage when it is available.
Otherwise, record the skip.
