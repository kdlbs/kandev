# ADR-2026-09-05-required-internal-persistence: Required internal persistence fails startup

**Status:** accepted
**Date:** 2026-09-05
**Area:** backend

## Context

Kandev stores internal state in built-in SQL stores. Many stores support both SQLite and PostgreSQL.

Some provider constructors currently combine local schema initialization with remote provider setup. Bootstrap treats their errors as nonfatal.

This behavior lets a database error look like an unavailable provider. Kandev can then report readiness while required internal state is unavailable.

Package-specific PostgreSQL tests do not close this gap. A package without a PostgreSQL test marker can remove itself from CI discovery.

## Decision

All built-in SQL stores are required internal persistence. One catalog lists their identities, dependencies, health probes, and conformance capabilities.

Bootstrap must initialize every catalog entry before readiness. A store or schema error stops startup and keeps readiness unavailable.

Feature flags control service activation, not SQL schema initialization. External credentials, authentication, and provider reachability remain degradable after local stores initialize.

Fixed CI packages compare bootstrap wiring and dual-engine conformance adapters with the catalog. CI does not discover PostgreSQL coverage from source markers.

`GET /health` remains a liveness endpoint. `GET /ready` and authenticated diagnostics expose aggregate required-store health without raw database errors.

## Consequences

Database defects stop a rollout before Kandev accepts stateful traffic. Operators receive the failed store identity and a recovery action.

An unavailable provider still affects only that provider. Its local store remains available for settings, diagnostics, and later recovery.

Each new SQL store adds catalog, bootstrap, health, SQLite, PostgreSQL, and upgrade obligations. This adds test work but removes self-selected coverage.

Feature-disabled schemas can exist in the database. This cost is small and keeps later feature activation inside the tested startup contract.

## Alternatives Considered

### Keep provider initialization nonfatal

Rejected. This option cannot distinguish a remote provider error from a required local schema error.

### Discover PostgreSQL packages from test markers

Rejected. A package without a marker can exclude itself from the test job.

### Keep separate inventories for bootstrap and tests

Rejected. Separate lists drift and cannot prove completeness.

### Make every provider error fatal

Rejected. A remote provider outage must not stop unrelated Kandev services.
