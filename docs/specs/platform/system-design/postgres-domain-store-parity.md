---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007
  - REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008
---

# Required Persisted Store Parity System Design

## Purpose and boundaries

The Platform system owns database portability, startup, readiness, and shared diagnostics. Domain systems own their data models and store operations.

This design covers built-in SQL stores that use the selected Kandev database. Filesystem stores and remote provider APIs are outside the catalog.

Local provider data is required persistence. Provider credentials, API reachability, and remote authentication remain degradable dependencies.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001` | [Schema and query contracts](#schema-and-query-contracts), [Conformance suite](#conformance-suite) |
| `REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002` | [Bootstrap contract](#bootstrap-contract), [External provider isolation](#external-provider-isolation) |
| `REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003` | [Required store catalog](#required-store-catalog), [Completeness enforcement](#completeness-enforcement) |
| `REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004` | [Conformance suite](#conformance-suite), [CI execution](#ci-execution) |
| `REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005` | [Dialect safety check](#dialect-safety-check), [Schema and query contracts](#schema-and-query-contracts) |
| `REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006` | [Upgrade fixtures](#upgrade-fixtures), [PostgreSQL version coverage](#postgresql-version-coverage) |
| `REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007` | [Runtime health](#runtime-health), [Caller errors](#caller-errors), [Diagnostics](#diagnostics) |
| `REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008` | [Contributor workflow](#contributor-workflow) |

## Required store catalog

`internal/persistence/requiredstores` owns the authoritative catalog. The package does not import domain stores or `backendapp`.

Each `Descriptor` contains these stable fields:

| Field | Purpose |
| --- | --- |
| `ID` | Stable kebab-case identity for startup, tests, logs, and diagnostics. |
| `OwnerPackage` | Go package that owns the schema and behavior. |
| `RequiredTables` | Tables used by the runtime health probe. |
| `DependsOn` | Store IDs that must initialize first. |
| `Capabilities` | Required conformance scenarios for booleans, timestamps, conflicts, and transactions. |

The catalog contains SQL schema owners, not each service facade. One schema owner can supply several interfaces.

The first implementation audits and registers these families:

- Core task, workflow, analytics, agent settings, user, notification, editor, prompt, utility, Office, terminal, quick-terminal, runtime-flag, authentication, and secret stores.
- Shared system settings, organizations, organization units, session hostnames, worktrees, message queue, telemetry contracts, schema metadata, storage jobs, share links, and delivery ledger.
- Plugin state, user state, web-app instances, and instance state.
- GitHub, GitLab, Jira, Linear, Sentry, Azure DevOps, workflow sync, Office configuration sync, and automation stores.
- Canvas SQL state and any other feature-gated SQL schema owner found by the bootstrap audit.

All SQL schemas initialize regardless of their feature flag. A feature flag controls service activation after the required persistence phase.

## Bootstrap contract

`backendapp` creates a `requiredstores.Tracker` from the catalog before it initializes repositories. Bootstrap reports each store through its catalog ID.

The tracker accepts only known IDs. It rejects duplicate completion, dependency-order errors, and missing results.

Bootstrap uses this sequence:

1. Open the configured database pool.
2. Initialize each catalog store in dependency order.
3. Record a sanitized success or error for each store.
4. Stop startup on the first required-store error.
5. Reject a catalog entry that bootstrap did not visit.
6. Run an immediate health probe for all initialized stores.
7. Build feature services and external provider clients.
8. Set readiness only after the aggregate required-store state is healthy.

The bootstrap listener keeps its existing behavior during this sequence. `GET /health` remains a liveness response, and `GET /ready` remains unavailable.

`provideRepositories`, `provideServices`, `provideOrchestrator`, `provideWorktreeManager`, and later store constructors use the same tracker. A store cannot hide in a secondary service phase.

## External provider isolation

Provider construction separates local store initialization from remote availability. Local stores initialize in the required persistence phase.

Services then receive initialized stores. Credential discovery, token validation, remote probes, and poller startup occur in the degradable provider phase.

A provider error updates only that provider status. It does not mark the local store or aggregate persistence state unhealthy.

This split applies to GitHub, GitLab, Jira, Linear, Sentry, Azure DevOps, and future external providers. Workflow sync and Office configuration sync accept a nil remote client.

Tests inject one unavailable provider at a time. They assert that all catalog stores remain healthy and unrelated providers remain usable.

## Runtime health

The tracker owns immutable initialization results and mutable runtime probe results. Each store state is `initializing`, `healthy`, or `unhealthy`.

An immediate probe runs before readiness. A background probe then runs every 15 seconds with a two-second timeout.

Each probe pings the shared writer and reader. It also checks each descriptor's required tables through `internal/db` schema helpers.

One probe error makes aggregate persistence unhealthy. A later successful probe restores runtime health without a restart.

Initialization errors remain fatal and cannot recover inside the same process. The process exits through the existing startup error path.

## Caller errors

`GET /health` remains a pure liveness endpoint. Persistence state does not change its status or body.

`GET /ready` returns `503` while startup is incomplete or aggregate persistence is unhealthy. A persistence response adds `reason: "persistence"` and sorted `store_ids`.

A middleware protects stateful HTTP routes and the WebSocket upgrade. If persistence is unhealthy, it returns this authenticated response:

```json
{
  "error": "required persistence is unavailable",
  "code": "persistence_unavailable",
  "store_ids": ["task"],
  "action": "Check the database connection and the persistence diagnostics, then retry."
}
```

The middleware excludes liveness, readiness, static assets, and authenticated persistence diagnostics. It does not expose raw database text.

## Diagnostics

`GET /api/v1/system/diagnostics/persistence` returns one row per catalog entry. Each row includes its ID, owner package, state, last check time, and sanitized error.

The endpoint also returns the database driver and aggregate state. It does not return credentials, DSNs, SQL text, row data, or database paths.

Structured logs use `store_id`, `driver`, `phase`, `state`, and `error_class`. Diagnostic bundles already include these backend logs.

## Schema and query contracts

`internal/db/dialect` owns shared schema rendering. A store supplies a template with explicit tokens for timestamp, boolean, identity, and current-time fragments.

The renderer fails on an unknown or unexpanded token. Store packages do not transform DDL through local `strings.Replace` calls.

SQLite timestamps use `DATETIME`. PostgreSQL timestamps use `TIMESTAMPTZ` unless a documented model requires a naive timestamp.

SQL boolean columns use Go booleans, SQL boolean literals, and dialect-safe defaults. Numeric status fields remain integers.

Portable conflict handling uses `ON CONFLICT`. Shared SQL does not use `INSERT OR IGNORE`.

Shared execution helpers rebind source `?` placeholders at the final database or transaction boundary. `sqlx.In` expansion occurs before rebinding.

Schema inspection uses `internal/db.TableExists`, `TableColumns`, and related helpers. Store packages do not query SQLite catalogs directly.

## Dialect safety check

`internal/db/sqlguard` parses Go source with the Go syntax tree. `cmd/sqlguard` supplies a stable local and CI command.

The check scans non-test Go source under `apps/backend/internal`. It reports the file, symbol, rule, and unsafe SQL fragment.

The first rule set rejects these patterns outside central dialect helpers:

- `PRAGMA` and `sqlite_master`.
- `INSERT OR IGNORE`.
- `julianday`, `datetime`, `strftime`, and SQLite current-time expressions.
- Direct `DATETIME` schema types.
- Integer defaults or comparisons on SQL boolean columns.
- SQL with raw `?` placeholders passed to direct execution without `Rebind` or a shared rebinding helper.

`internal/db/sqlguard/exemptions.json` stores exact file, symbol, rule, and reason entries for genuine SQLite-only code. Broad path globs are invalid.

The check fails on an unused exemption. It also fails when an exemption names a missing file or symbol.

Unit fixtures contain one positive and one negative case for every rule. They include transaction and `sqlx.In` rebinding cases.

## Conformance suite

`internal/testutil/storeconformance` supplies a reusable scenario runner. It opens an isolated SQLite database or PostgreSQL schema and applies the same assertion callbacks.

`internal/persistence/storeconformance` is the fixed aggregation package. Its test adapters import each schema owner and bind domain operations to the runner.

Each adapter provides these baseline scenarios:

- Fresh schema initialization.
- Schema replay with sentinel data preservation.
- Representative create, read, update, and delete behavior.

The catalog capabilities require more scenarios when a store uses them:

- Boolean false and true round trips.
- Timestamp insertion, ordering, null handling, and UTC normalization.
- Duplicate insertion and conflict-update behavior.
- Transaction commit and rollback behavior.

The same adapter assertions run against both engines. Engine-specific assertions can inspect schema types, but they cannot replace behavior assertions.

## Completeness enforcement

The aggregation package compares its adapter IDs with the catalog. Unknown, duplicate, and missing IDs fail before engine setup.

The comparison runs even when `KANDEV_TEST_POSTGRES_DSN` is absent. A developer cannot hide a missing adapter through a PostgreSQL skip.

Each adapter declares both `sqlite3` and `pgx`. The suite rejects a missing engine or a missing callback for a catalog capability.

Bootstrap tests compare tracker visits with the same catalog. Readiness tests compare diagnostic rows with the same catalog.

## Upgrade fixtures

Committed fixtures live under `internal/persistence/storeconformance/testdata/upgrades/<tag>/`. The directory contains SQLite and PostgreSQL schema files plus one manifest.

The manifest records the stable tag, source commit, engine, fixture checksum, and sentinel rows. The initial baseline is `v0.93.0`.

The PostgreSQL fixture represents a successful `v0.93.0` core boot and its known missing provider tables. This state reproduces the incident without pretending that failed tables existed.

Upgrade tests load a fixture, run current bootstrap order, run schema replay, and execute all conformance reads. Sentinel rows must remain byte-equivalent where the model contract preserves bytes.

A fixture update script accepts an explicit stable tag. It never derives the baseline from the current working tree.

## PostgreSQL version coverage

PostgreSQL 16 remains the full conformance target. PostgreSQL 18 runs the clean-boot and previous-stable upgrade tests that cover catalog and type behavior.

Both service images use immutable digests and the existing GHCR mirror process. The matrix documents its supported major versions beside the workflow.

The smaller PostgreSQL 18 job protects the incident environment without doubling all race tests. A support-matrix change must update this design and CI together.

## CI execution

The backend workflow invokes fixed commands. It does not search source text for `PostgresDSNFromEnv` or another marker.

The required gates are:

1. Catalog completeness and SQLite conformance without external services.
2. SQL dialect safety.
3. Full PostgreSQL 16 conformance and backend boot.
4. PostgreSQL 18 clean-boot and upgrade coverage.
5. Previous-stable upgrade coverage on SQLite.

The workflow asserts named top-level tests in each fixed package. A zero-test result fails the job.

## Contributor workflow

`docs/public/backend-development.md` documents the catalog and commands. It also explains the required and degradable startup phases.

The root and backend `AGENTS.md` files state that a new SQL store must add a catalog entry and one central conformance adapter.

The guide requires a fresh schema change, a replay change, a previous-stable upgrade path, and central dialect helpers in the same pull request.

## Security and privacy

Public readiness output contains stable store IDs only. It does not contain SQL, credentials, hostnames, database names, or raw errors.

Authenticated diagnostics sanitize driver errors before they cross the API boundary. Full errors remain in local structured logs.

## Related decisions

- [Required internal persistence fails startup](../../../decisions/2026-09-05-required-internal-persistence.md)
- [Replayable schema migrations across SQLite and Postgres](../../../decisions/0027-replayable-schema-migrations.md)
- [Startup Configuration Uses One Typed Source Model](../../../decisions/2026-08-20-startup-configuration-source-parity.md)
