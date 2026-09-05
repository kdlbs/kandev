---
id: "01-index-pending-id-bundle-access"
title: "Index pending-ID bundle access"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/clarification-response-reliability.md"
---

# Task 01: Index pending-ID bundle access

## Outcome

Pending-ID-only clarification reads and claims use an additive expression index
on fresh and existing SQLite and PostgreSQL databases.

## In scope

- Add a dialect-owned DDL helper for
  `idx_messages_metadata_pending_id_lookup` with pending ID first, followed by
  `created_at` and `id`, and a non-null predicate.
- Create the index in the ordered, startup-critical message-index step with a
  new replay-safe name.
- Keep the existing session-first pending-ID index.
- Add SQLite fresh-schema, replay, existing-schema, and query-plan coverage.
- Add environment-gated real PostgreSQL index-definition, replay, and planner
  eligibility coverage.
- Confirm pending-ID bundle reads and the atomic claim use the exact dialect
  expression indexed by the new DDL.

## Exclusions

- No message-row backfill or dedicated clarification table.
- No removal or reordering of an existing index.
- No PostgreSQL concurrent index creation or multi-replica migration protocol.
- No clarification authority, status, or delivery changes.

## Traceability

- `REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.2`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.5`
- `docs/specs/tasks/system-design/clarification-response-reliability.md`

## Implementation acceptance

- SQLite and PostgreSQL initialize and replay the new partial expression index
  without altering existing messages or dropping the session-first index.
- A pending-ID-only bundle lookup with substantial unrelated history uses the
  new index instead of a full message-table scan in SQLite, and the real
  PostgreSQL suite proves native index eligibility.
- Index creation failure propagates from repository initialization and blocks
  startup.

## TDD sequence

1. Add failing dialect and repository tests for the DDL, fresh/replayed schema,
   existing database upgrade, and SQLite query plan.
2. Add the skipped-without-DSN PostgreSQL test for native index definition and
   planner eligibility.
3. Implement the dialect helper and wire it into
   `ensureMessageMetadataIndexes`.
4. Refactor duplicated pending-ID expressions only where required to keep query
   and index text aligned, then rerun both database suites.

## Likely files

- `apps/backend/internal/db/dialect/pendingindex.go`
- `apps/backend/internal/db/dialect/pendingindex_test.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/message.go`
- `apps/backend/internal/task/repository/sqlite/message_clarification_response.go`
- `apps/backend/internal/task/repository/sqlite/message_pending_index_test.go`
- `apps/backend/internal/task/repository/sqlite/message_pending_index_postgres_test.go`

## Dependencies

None.

## Verification

- `cd apps/backend && go test ./internal/db/dialect -run 'PendingIDIndex' -count=1`
- `cd apps/backend && go test ./internal/task/repository/sqlite -run 'PendingID.*Index|Index.*PendingID' -count=1`
- `cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/task/repository/sqlite -run 'Postgres.*PendingID.*Index' -count=1`

## Results

Implemented the additive pending-ID-leading partial expression index for both
drivers. SQLite fresh initialization, replay/upgrade preservation, and query
plan coverage pass. The environment-gated PostgreSQL index definition,
planner, replay, and preservation coverage passes when a test DSN is present.

Verification:

- `cd apps/backend && go test ./internal/db/dialect -run 'PendingIDIndex' -count=1`
- `cd apps/backend && go test ./internal/task/repository/sqlite -run 'PendingID.*Index|Index.*PendingID' -count=1`
- `cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/task/repository/sqlite -run 'Postgres.*PendingID.*Index' -count=1`
