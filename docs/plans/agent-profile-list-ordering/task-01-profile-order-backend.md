---
id: "01-profile-order-backend"
title: "Profile order backend"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-LIST-ORDERING-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.1
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.2
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.4
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.5
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.6
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.8
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.9
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.10
  - AC-AGENTS-PROFILE-LIST-ORDERING-003.12
system_design:
  - ../../specs/agents/system-design/profile-list-ordering.md
---

# Task 01: Profile order backend

## Summary

Store a per-agent profile order, serve profiles in that order, and add the
permission-gated reorder endpoint with its WebSocket event.

## In scope

- `agent_profiles.sort_order` in the `CREATE TABLE`, the table-recreation path
  (`agent_profiles_new`, `srcHasSortOrder` copy), and an `Apply` migration;
  ordered `ListAgentProfiles`.
- Create `agent_profile_orders`; ensure the `agent-settings` required-store
  descriptor lists `agents`, `agent_profiles`, and `agent_profile_orders`, and
  the fixed conformance adapter verifies the schema.
- `Repository.ReorderAgentProfiles` (transactional, set-equality check, no
  `updated_at` write, revision upsert in `agent_profile_orders`) and
  `GetAgentProfileOrderSnapshots`, which returns each requested agent's ordered
  profile rows and revision from one read-only transaction. PostgreSQL uses
  `sql.LevelRepeatableRead`; SQLite relies on the WAL snapshot established by
  the transaction's first query. Update every `Repository` fake.
- Serialize Postgres reorders with a transaction-scoped advisory lock keyed by
  namespace and agent ID before the first membership read; retain SQLite's
  single-writer serialization.
- Apply the same per-agent lock in the transaction for global profile creates,
  duplicates, soft-deletes, and agent deletion with profile cascade. Profile
  deletion also locks the candidate owner and stable profile ID when its
  preliminary row is workspace-scoped, then rereads owner and scope under lock
  and retries if either changed. Full-row updates (`UpdateAgentProfile` and
  `UpdateAgentProfileWithDynamic`) take a profile-ID lock plus sorted locks for
  known global source/target agents; they reread ownership under lock and retry
  when the preliminary owner is not covered by the held lock set.
- `Controller.ReorderAgentProfiles`, `ErrProfileOrderStale`, Dynamic-agent
  rejection; the handler broadcasts the event.
- `PUT /api/v1/agents/:id/profiles/order` and `ActionAgentProfilesReordered`.

## Out of scope

- Any web code. Agent-card order. Workspace-scoped profiles.

## Acceptance

- A reorder persists, is returned by `GET /agents` in that order with an
  increasing `profile_order_revision`; both `GET /agents` and `GET /agents/:id`
  pair ordered profile rows and revision from one consistent database snapshot.
  Controller regression tests give the legacy `ListAgentProfiles` getter
  deliberately different rows and assert both GET paths use the combined
  snapshot rows and revision without calling the legacy getter. Later-created
  profiles list first, an untouched install keeps newest first, and reorder
  leaves `updated_at` and configuration unchanged. Fresh and replay migrations,
  legacy `CHECK(model)` table recreation, Postgres, and the tagged `v0.93.0`
  upgrade fixture work; required-store/conformance checks assert `agents`,
  `agent_profiles`, and `agent_profile_orders`, and the upgrade manifest verifies
  legacy `sort_order = 0`.
- A request that is not exactly the agent's global profiles returns `409
profile_order_stale`, a request for the Dynamic agent returns `400
profile_order_unsupported`, both change nothing; a caller without
  agent-configuration permission is refused; a changed order broadcasts
  `agent.profiles.reordered` from the handler and an unchanged order does not.
- Deterministic two-connection SQLite and Postgres tests exercise both lock
  winners for global create, `DuplicateAgentProfile`, soft-delete, and
  `DeleteAgent` cascade against reorder; assert final membership/order and
  HTTP/event outcomes, including `404` and no reorder event when agent deletion
  wins. Ownership-update tests race A→B against A→C and force the stale-owner
  path (pre-read A, concurrent A→C commit, retry with sorted locks {B,C}), then
  race workspace→global and global→workspace moves against reorder in both lock
  orders. Each scenario asserts final order/membership; mutation-first returns
  `409` or `404` as applicable and emits no reorder event; reorder-first returns
  success and broadcasts only the exact locked set.
- A deterministic SQLite/Postgres race pauses deletion after its preliminary
  workspace-profile read, promotes the profile from owner A to B, and reorders
  B before deletion retries under B's lock. Assert both delete lock sets, the
  committed reorder, and the final membership/order.
- Snapshot race tests use a package-private after-profile-query barrier in the
  same private implementation called by the production repository method. The
  SQLite case uses a file-backed WAL database with independent reader and writer
  connections (not `:memory:`): while the reader transaction is paused after
  its first query, the writer commits a reorder; the active result must be
  old-order/old-revision and the next snapshot new-order/new-revision. The
  Postgres case uses two connections and `sql.LevelRepeatableRead` for the same
  old/old then new/new assertions.

## Verification

```bash
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test ./internal/agent/settings/store -run '^TestPostgres' -count=1)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
make -C apps/backend test
make -C apps/backend lint
```

The Postgres line needs `KANDEV_TEST_POSTGRES_DSN`; record when it is unavailable.

Every DSN-gated Postgres test in this package uses the `TestPostgres` name
prefix so `-run '^TestPostgres'` exercises schema and concurrency coverage.

## Files likely touched

- `apps/backend/internal/agent/settings/store/sqlite.go`
- `apps/backend/internal/agent/settings/store/sqlite_profile_order.go`
- `apps/backend/internal/agent/settings/store/store.go`
- `apps/backend/internal/agent/settings/store/sqlite_profile_order_test.go`
- `apps/backend/internal/agent/settings/store/sqlite_migration_test.go`
- `apps/backend/internal/agent/settings/store/postgres_schema_test.go`
- `apps/backend/internal/agent/settings/controller/profile_order.go`
- `apps/backend/internal/agent/settings/controller/profile_order_test.go`
- `apps/backend/internal/agent/settings/store/postgres_profile_order_concurrency_test.go`
- `apps/backend/internal/agent/settings/store/profile_order_snapshot_test.go`
- `apps/backend/internal/agent/settings/controller/agent_crud_snapshot_test.go`
- `apps/backend/internal/agent/settings/controller/reconciler_test.go` (`fakeStore`)
- `apps/backend/internal/agent/settings/store/sqlite_profile_order_concurrency_test.go`
- `apps/backend/internal/agent/settings/handlers/handlers.go`
- `apps/backend/internal/agent/settings/handlers/profile_handlers.go`
- `apps/backend/internal/agent/settings/dto/dto.go` (`profile_order_revision`)
- `apps/backend/internal/agent/settings/controller/agent_crud.go`
- `apps/backend/internal/agent/settings/handlers/profile_order_handlers_test.go`
- `apps/backend/internal/agent/settings/handlers/agent_settings_org_scope_test.go` (route table)
- `apps/backend/internal/agent/settings/handlers/interim_settings_interlock_test.go` (route table)
- `apps/backend/internal/agent/settings/handlers/profile_duplicate_handlers_test.go` (`fakeSettingsRepo`)
- `apps/backend/internal/agent/runtime/lifecycle/profile_resolver_test.go` (`MockRepository`)
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/persistence/requiredstores/catalog.go`
- `apps/backend/internal/persistence/requiredstores/catalog_test.go`
- `apps/backend/internal/persistence/storeconformance/adapters.go`
- `apps/backend/internal/persistence/storeconformance/upgrade_test.go`
- `apps/backend/internal/persistence/storeconformance/testdata/upgrades/v0.93.0/manifest.json`
- `apps/backend/internal/testutil/postgres.go`
- `apps/backend/internal/testutil/postgres_dsn.go`
- `apps/backend/internal/testutil/postgres_dsn_test.go`
- `apps/backend/internal/agent/settings/store/postgres_profile_order_fixture_test.go`
- SQLite and Postgres tagged-upgrade/conformance fixture expectations

## Dependencies

None.

## Risks

- Other `store.Repository` fakes fail to compile; find them with an LSP
  references lookup on the interface.
- The table-recreation copy must include `sort_order` or a legacy upgrade resets
  every order.

## Parallelism

`sequential`

## Inputs

- System design sections Data and contracts, Persistence, Security.
- `apps/backend/AGENTS.md` table-rebuild migration rule.
- `controller/agent_crud.go` (`filterGlobalProfiles`), `controller.go`
  (`broadcastProfileUpdated`) for the broadcast pattern, and
  `profile_duplicate_handlers_test.go` for handler test setup.

## Results

The results below are historical contributor reports from before integration
with current main. They are not verification of the combined review head.

Implemented the schema, snapshot reads, reorder endpoint/event, ownership locks,
and concurrent membership/reorder coverage.

Scoped store, controller, handler, required-store, and store-conformance tests,
SQLite race tests, SQL guard, and backend lint passed. `make -C apps/backend
test` remains blocked by two failures in the unchanged
`internal/testutil/envscan_test.go` environment-scan tests; the expected
diagnostics were absent. `KANDEV_TEST_POSTGRES_DSN` was unset, so the
DSN-gated PostgreSQL migration and concurrency tests were not exercised.

Round-five review follow-up: workspace-scoped profile deletion now acquires the
candidate agent membership lock and stable profile-ID lock, rereads ownership
and scope, and retries if promotion changed them. The regression in
`sqlite_profile_order_concurrency_test.go` pauses a delete, promotes its profile
from agent A to B, commits a reorder of B, and asserts the retry locks and final
membership/order. The test failed before the fix because deletion completed
without entering the ownership barrier.

`TMPDIR=/home/clem/.kandev go test -race -trimpath ./internal/agent/settings/store`
passed. The focused
`go test -race -trimpath -run '^TestSQLiteWorkspaceProfileDeleteRetriesAfterPromotionAndReorder$' -count=1 ./internal/agent/settings/store`
passed, and changed-backend `golangci-lint run ./... --new-from-rev=b0dc2bef512eda8def545ba5cf2edc67bbedf73d --timeout=5m`
reported zero issues using the persistent Go cache outside the full managed
cache mount. `TestPostgresWorkspaceProfileDeleteRetriesAfterPromotionAndReorder`
was skipped because `KANDEV_TEST_POSTGRES_DSN` is unset.

## Integration review validation

The review fixup preserves current-main catalogue publication, enabled-intent
persistence, capability/runtime metadata, and Settings model-picker behavior.
It adds regression coverage for creation baselines during pending drags and
late acknowledgements, terminal rollback, the 15-second request timeout,
malformed timestamp handling, Office option preservation, and localized drag
announcements. Desktop handles use the shared 28 px size; phone and coarse
pointer handles retain 44 px hit targets. Backend regressions cover canceled
repository reads, equal/missing ranks, and create/delete after saved ordering.

Focused frontend tests run in a credential-free, nonroot, network-disabled
container. Broad frontend/backend, race, lint, build, typecheck, and rendered
desktop/mobile checks remain assigned to hosted CI. Host hooks and contributor
tooling are not executed during this review. The historical broad-suite
failures above remain visible; they are not attributed to unrelated tests
without exact source and log evidence.

## PostgreSQL fixture correction after integration

The final #4373 head was merged externally while its hosted checks were still
pending. Its PostgreSQL job subsequently failed: newly opened pool connections
lacked the isolated schema, and the membership matrix reused one schema across
subcases that seed the same unique agent names. These failures do not establish
a production ordering failure; the concurrency proof remains incomplete.

Set the owned schema in connection startup parameters so every physical
connection uses it, retaining the production connector and schema cleanup. Open
a new repository/schema inside each membership subcase, preserving both SQL
dialects and all existing lock, revision, retry, and membership assertions.
Add driver parsing coverage for URL and keyword DSNs, including existing
parameters and invalid URLs. Add a PostgreSQL regression holding eight physical
connections and checking their schema and distinct backend PIDs.

Bounded isolated driver tests reproduced missing startup parameters and then
passed after the correction. Private gofmt and static assertion-preservation
checks passed. No local PostgreSQL server or broad package suite was run.
Hosted PostgreSQL, store, race, lint, and full backend results must pass on the
corrective head before this work order can return to done. Public behavior and
documentation are unchanged by this fixture correction.

## Corrective CI synchronization

The corrective combined-head backend shard exposed a clarification retry test
that asserted message creation as soon as the pending request was registered.
Registration precedes durable reconciliation and message publication. The
foreign-owner retry and different-question retry fixtures now await their
creator count within the existing one-second deadline before retaining their
exactly-one-bundle and identity assertions. Production registration ordering,
timeouts, and assertion values are unchanged. The bounded local package proof
could not execute because its dependencies were absent from the isolated cache;
fresh hosted backend execution is required.

Additional file: `apps/backend/internal/mcp/handlers/handlers_ask_retry_test.go`.
