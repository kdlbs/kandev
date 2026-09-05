---
status: active
system: platform
created: 2026-09-03
owners:
  - kandev
---

# PostgreSQL Domain Store Parity Requirements

## Overview

Kandev supports SQLite and PostgreSQL as database backends. The Platform system owns this shared operational guarantee.

Built-in domain services must remain available with either supported backend. A database dialect must not silently disable a configured service during startup.

## Terminology

- **Domain store:** A store that persists state for one built-in service in the shared Kandev database.
- **Replay:** A second schema initialization against the same database without a version change.

## Requirements

### REQ-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001: Portable domain stores

**Intent:** Operators can use all built-in persisted services when Kandev uses PostgreSQL.

#### Acceptance criteria

- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.1:** When Kandev starts with PostgreSQL, every enabled built-in domain store shall initialize successfully.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.2:** When schema initialization runs again, each domain store shall preserve its data and return no replay error.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.3:** When a service reads or changes PostgreSQL data, its result shall match the equivalent SQLite operation.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.4:** When a domain store cannot initialize, startup diagnostics shall identify that store and the database error.
- **AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.5:** When PostgreSQL is active, integration and automation endpoints shall not fail because their stores are unavailable.

## Out of scope

- Active-active backend replicas that share one PostgreSQL database.
- Automatic PostgreSQL backups or restores.
- A central migration framework.
- Changes to provider authentication or external provider APIs.
