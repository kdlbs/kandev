---
status: draft
system: platform
created: 2026-09-03
owners:
  - kandev
---

# Required Persisted Store Parity Requirements

## Overview

Kandev supports SQLite and PostgreSQL for its built-in SQL stores. The Platform system owns this shared startup and compatibility guarantee.

Each required store must initialize before Kandev becomes ready. External providers can remain unavailable without changing the health of their local stores.

## Terminology

- **Required store:** A built-in SQL store that persists Kandev state in the selected database.
- **External provider:** A remote service, credential, or authentication flow that uses a required store.
- **Replay:** A second schema initialization against the same database without a version change.
- **Previous stable schema:** The committed schema fixture from the stable release that immediately precedes the current development version.
- **Store catalog:** The authoritative inventory of required stores and their conformance obligations.

## Requirements

### REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001: Portable domain stores

**Intent:** Operators can use all built-in persisted services with SQLite or PostgreSQL.

#### Acceptance criteria

- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.1:** When Kandev starts with PostgreSQL, every enabled built-in domain store shall initialize successfully.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.2:** When schema initialization runs again, each domain store shall preserve its data and return no replay error.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3:** When a service reads or changes PostgreSQL data, its result shall match the equivalent SQLite operation.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.4:** When a domain store cannot initialize, startup diagnostics shall identify that store and the database error.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.5:** When PostgreSQL is active, integration and automation endpoints shall not fail because their stores are unavailable.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.6:** When Kandev starts with SQLite, every required store shall initialize through the same store catalog used for PostgreSQL.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.7:** When a store uses booleans, timestamps, conflicts, or transactions, both database engines shall produce equivalent results.

### REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002: Required persistence startup contract

**Intent:** Kandev never reports readiness when required internal persistence is incomplete.

#### Acceptance criteria

- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.1:** When a required store or schema cannot initialize, Kandev shall not become ready and startup shall return an error.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.2:** When an external provider or its authentication is unavailable, Kandev shall keep unrelated providers and internal services available.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.3:** When one provider is unavailable, its local required store shall remain initialized and diagnosable.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-002.4:** Kandev shall not classify a required store initialization error as a degradable provider error.

### REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003: Authoritative store catalog

**Intent:** A new required store cannot bypass startup, health, or dual-engine coverage.

#### Acceptance criteria

- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.1:** Bootstrap, readiness diagnostics, and coverage tests shall consume one authoritative store catalog.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.2:** When a catalog entry lacks bootstrap wiring, SQLite coverage, or PostgreSQL coverage, CI shall fail.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.3:** A disabled feature shall not remove its required SQL schema from initialization or compatibility coverage.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-003.4:** When a catalog entry or coverage adapter becomes stale, CI shall identify the missing or unknown store ID.

### REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004: Store conformance suite

**Intent:** Equivalent store behavior is executable evidence, not an inference from package names or test markers.

#### Acceptance criteria

- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.1:** Each required store shall pass fresh initialization, schema replay, and representative create, read, update, and delete behavior on both engines.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.2:** The suite shall cover boolean values, timestamps, conflict handling, and transaction commit or rollback where the store uses those behaviors.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.3:** SQLite and PostgreSQL scenarios shall use the same behavioral assertions for each store.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.4:** PostgreSQL CI shall invoke a fixed conformance package instead of discovering packages from test source markers.

### REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005: Dialect-safe shared SQL

**Intent:** Unsafe shared SQL fails before merge.

#### Acceptance criteria

- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.1:** CI shall reject unapproved shared-store uses of `PRAGMA`, `sqlite_master`, `INSERT OR IGNORE`, SQLite date functions, and direct `DATETIME` types.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.2:** CI shall reject raw `?` placeholders sent to an executor without rebinding.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.3:** CI shall reject integer defaults or comparisons for SQL boolean columns in shared-store code.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.4:** A SQLite-only exemption shall name an exact scope, rule, and reason. CI shall reject stale or unused exemptions.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-005.5:** Shared schema and query code shall use the central dialect helpers instead of local text replacement.

### REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006: Supported upgrade paths

**Intent:** A clean install and the supported upgrade path remain valid on each database engine.

#### Acceptance criteria

- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.1:** Current code shall upgrade the previous stable SQLite and PostgreSQL schema fixtures without data loss.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.2:** After an upgrade, a schema replay shall succeed and the conformance scenarios shall pass.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.3:** Each fixture shall record its stable tag, source commit, and expected sentinel data.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.4:** CI shall cover the oldest and newest PostgreSQL majors in the documented support matrix where their behavior can differ.

### REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007: Required-store health

**Intent:** Operators and callers receive actionable persistence status.

#### Acceptance criteria

- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.1:** When required persistence is unhealthy, `GET /ready` shall return `503` and identify the affected store IDs.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.2:** Authenticated diagnostics shall show each required store state, last check time, and a sanitized error.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.3:** When a stateful caller reaches an unhealthy backend, it shall receive `503` with the stable code `persistence_unavailable` and a recovery action.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.4:** When runtime database health recovers, readiness and stateful requests shall recover without a process restart.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-007.5:** Required-store health shall not change the liveness contract of `GET /health`.

### REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008: Contributor workflow

**Intent:** Contributors can add or change a store without guessing the portability rules.

#### Acceptance criteria

- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008.1:** Contributor documentation shall define the catalog, conformance, upgrade-fixture, dialect-check, and local PostgreSQL workflow.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008.2:** The documented workflow shall state that package names and local test markers do not provide PostgreSQL coverage.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-008.3:** The documented workflow shall preserve independent degradation for external providers and authentication.

## Out of scope

- Active-active backend replicas that share one PostgreSQL database.
- Automatic PostgreSQL backups or restores.
- Migration of data from one database engine to another.
- Automatic schema repair after startup.
- Changes to external provider APIs or authentication policy.
- A change to the `GET /health` liveness contract.
