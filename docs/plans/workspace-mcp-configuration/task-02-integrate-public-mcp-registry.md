---
id: "02-integrate-public-mcp-registry"
title: "Integrate the public MCP Registry"
status: done
wave: 2
depends_on:
  - "01-persist-workspace-mcp-catalog"
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.9
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.10
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.11
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 02: Integrate the Public MCP Registry

## Summary

Add a bounded backend aggregator for the public MCP Registry and a versioned
curated catalog. Expose searchable last-good discovery data and an explicit
review-to-install API that creates independent workspace definitions.

## In scope

- Implement the versioned Registry HTTP client with cursor pagination,
  `updated_since`, timeouts, response limits, and status parsing.
- Persist normalized registry entries and sync state in SQLite and Postgres.
- Add full and incremental single-flight sync, an hourly schedule, manual
  refresh, and last-good-cache fallback.
- Add a small embedded curated manifest with source and template versions.
- Add marketplace search, detail, refresh, and install handlers.
- Require cached-revision review, package or remote selection, transport
  validation, and secret bindings before catalog creation.
- Mark compatible remote and exact npm choices selectable. Keep unsupported
  Registry package types visible with a typed incompatibility reason.
- Prove discovery and installation never execute, download, or connect to an
  advertised server.

## Out of scope

- Marketplace UI.
- Private registries, subregistry configuration, rankings, and trust badges.
- Automatic updates to installed definitions.

## Acceptance

- Full and incremental sync converge across pagination, deprecation, deletion,
  timeout, malformed data, and retry cases without discarding the last good
  cache.
- Marketplace installation pins reviewed source metadata and creates a normal
  workspace definition without invoking a process or advertised endpoint.
- Package and remote choices remain distinct. The API never converts an
  unsupported package type into a caller-supplied shell command.
- Search and detail responses label curated, public, stale, degraded,
  deprecated, deleted, and publisher-supplied metadata accurately.

## Verification

```bash
cd apps/backend && go test ./internal/agent/mcpconfig/registry ./internal/agent/mcpconfig ./internal/agent/settings/handlers -run 'Test.*(Registry|Marketplace|Curated)'
```

Write fake Registry server tests first. The RED cases must cover cursor
pagination, incremental status changes, stale cache, and non-execution.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/registry/client.go`
- `apps/backend/internal/agent/mcpconfig/registry/client_test.go`
- `apps/backend/internal/agent/mcpconfig/registry/sync.go`
- `apps/backend/internal/agent/mcpconfig/registry/sync_test.go`
- `apps/backend/internal/agent/mcpconfig/curated.json`
- `apps/backend/internal/agent/mcpconfig/marketplace.go`
- `apps/backend/internal/agent/mcpconfig/marketplace_test.go`
- `apps/backend/internal/agent/settings/store/sqlite.go`
- `apps/backend/internal/agent/settings/store/postgres_schema_test.go`
- `apps/backend/internal/agent/settings/handlers/mcp_marketplace_handlers.go`
- `apps/backend/internal/agent/settings/handlers/mcp_marketplace_handlers_test.go`

## Dependencies

- Task 01 supplies workspace definitions, revisions, secret references, and
  sanitized catalog creation.

## Risks

- The public Registry preview can change availability or return large metadata.
  Keep API versioning and strict bounds at the client boundary.
- Curated entries can be mistaken for security approval. Product copy and API
  source fields must state the narrower template meaning.

## Parallelism

`sequential`

## Inputs

- Requirement section 002.
- System-design sections `Registry aggregator`, `Marketplace installation`,
  `Failure and recovery`, and `Security`.
- Public Registry aggregator documentation and preview trust model.

## Results

- Added a bounded, versioned Registry client with cursor pagination, incremental sync, status parsing, and timeout limits.
- Added persistent normalized Registry cache and sync state with single-flight refresh, hourly scheduling, and last-good degraded fallback.
- Added embedded curated entries and marketplace search, detail, refresh, and reviewed install handlers.
- Added exact-version package and compatible remote choice validation, unsupported package reasons, and non-execution tests.
- Verification passed through the MCP Registry package, settings handler tests, and desktop/mobile marketplace E2E coverage.
