---
id: task-03
title: plugin_state SQLite store
status: done
wave: 1
depends_on: []
plan: docs/plans/plugins/plan.md
---

# plugin_state SQLite store

## Title
SQLite-backed plugin-scoped KV state store following the per-package
`initSchema()` convention.

## Inputs
- Spec `docs/specs/plugins/spec.md` → "`plugin_state` (SQLite)" and
  "Plugin state API". Table:
  ```sql
  CREATE TABLE plugin_state (
    id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL,
    scope TEXT NOT NULL DEFAULT 'instance', scope_id TEXT,
    state_key TEXT NOT NULL, value_json TEXT NOT NULL, updated_at TEXT NOT NULL,
    UNIQUE (plugin_id, scope, scope_id, state_key));
  ```
- Scopes: instance | workspace | task | agent. Plugins cannot read others' state
  (enforced by always filtering on plugin_id — the API layer passes it, task-09).
- Pattern reference: `internal/task/repository/sqlite/base_schema.go` (initSchema
  with `CREATE TABLE IF NOT EXISTS`) and how repos take a `*sql.DB`/db handle.
  Use the same db handle type the other stores use (`db.Pool` writer/reader or
  `*sql.DB` — match sibling stores; inspect `internal/sentry/store.go`).

## Acceptance
1. `StateStore` with `initSchema()` creating `plugin_state` (idempotent).
2. Methods: `Get(ctx, pluginID, scope, scopeID, key) (json.RawMessage, bool, error)`,
   `Set(ctx, pluginID, scope, scopeID, key, value)` (upsert on the UNIQUE key),
   `Delete(ctx, pluginID, scope, scopeID, key)`,
   `List(ctx, pluginID, scope, scopeID) ([]StateEntry, error)`.
3. `updated_at` set to RFC3339 UTC on write. NULL `scope_id` handled for instance scope.

## Files
- `apps/backend/internal/plugins/state/store.go`
- `apps/backend/internal/plugins/state/store_test.go` (in-memory or temp-file sqlite)

## Verification
- `go test ./internal/plugins/state/...` from `apps/backend`
- `make -C apps/backend lint`

## Output contract
Report: db handle type chosen (match siblings), upsert approach, scope_id NULL
handling. Stay within `internal/plugins/state/`.

## Dependencies
None.
