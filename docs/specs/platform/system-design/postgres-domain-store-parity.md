---
status: current
system: platform
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
---

# PostgreSQL Domain Store Parity System Design

## Purpose and boundaries

The Platform system owns database portability because it is a shared startup guarantee. Integration and automation services consume this guarantee.

This design covers GitHub, GitLab, Jira, Linear, Sentry, Azure DevOps, workflow sync, and automation stores. It does not change provider behavior.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001` | [Schema contract](#schema-contract), [Query contract](#query-contract), [Startup contract](#startup-contract), [Verification strategy](#verification-strategy) |

## Components and responsibilities

- `internal/db/dialect` supplies SQL types and fragments for each supported database.
- `internal/db` supplies shared schema-introspection helpers.
- Each domain store owns its tables, ordered upgrade steps, and store methods.
- `internal/backendapp` creates the stores and keeps provider errors isolated from unrelated services.

The shared helpers replace local copies of `PRAGMA` and `sqlite_master` probes. This boundary keeps database detection out of provider packages.

## Schema contract

Each schema owner builds data-definition statements for the active driver. SQLite uses `DATETIME`. PostgreSQL uses `TIMESTAMPTZ`.

Boolean defaults and predicates use SQL boolean literals. Integer status fields remain integers when their model contract is numeric.

Schema introspection uses the current PostgreSQL schema. SQLite introspection uses its native catalog and table-info interface.

Each store applies the same ordered initialization on a fresh database and during replay. SQLite-only table rebuilds run only when SQLite needs them.

## Query contract

Store queries use `?` as the source placeholder syntax. The final database or transaction executor calls `Rebind` before execution.

Queries built with `sqlx.In` call `Rebind` after expansion. Store code does not send unresolved `?` placeholders to PostgreSQL.

Portable conflict handling uses `ON CONFLICT`. Store code does not use `INSERT OR IGNORE` in shared database paths.

SQL predicates compare boolean columns with `TRUE` or `FALSE`. Bound values use Go booleans for boolean columns.

## Startup contract

Provider initialization errors remain isolated. One unavailable provider does not stop unrelated Kandev services.

The PostgreSQL boot test must also assert service availability. A successful outer startup call is not sufficient because provider errors are nonfatal.

GitLab has a separate host store and task store. PostgreSQL evidence must cover the task store even when the outer service exists.

## Failure and recovery

A schema or query error returns through the owning store. Existing startup logs keep the store name and database error.

Schema replay does not remove data. PostgreSQL operators keep responsibility for database backups before an upgrade.

## Verification strategy

Each affected package uses `testutil.OpenIsolatedPostgres` for fresh-schema, replay, and representative store operations. SQLite package tests keep their current coverage.

The backend boot test creates the full service graph on PostgreSQL. It asserts availability for every affected service and store boundary.

Provider tests cover settings, watches, task links, and automation state where those concepts exist. These tests expose placeholder and boolean-type errors after startup.

## Related decisions

- [Replayable schema migrations across SQLite and Postgres](../../../decisions/0027-replayable-schema-migrations.md)
- [Workspace-scoped integration settings](../../../decisions/0030-workspace-scoped-integration-settings.md)
