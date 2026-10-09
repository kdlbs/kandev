---
created: 2026-09-17
status: implemented
requirements:
  - REQ-PLUGINS-TASK-DEPS-001
  - REQ-PLUGINS-TASK-DEPS-002
  - REQ-PLUGINS-TASK-DEPS-003
  - REQ-PLUGINS-TASK-DEPS-004
  - REQ-PLUGINS-TASK-DEPS-005
  - REQ-PLUGINS-TASK-DEPS-006
system_design:
  - ../../specs/plugins/system-design/task-dependency-projection.md
  - ../../specs/plugins/system-design/task-dependency-refresh.md
  - ../../specs/plugins/system-design/task-dependency-edge-ends.md
  - ../../specs/plugins/system-design/task-dependency-response-bounds.md
legacy_specs: []
---

# Implementation Plan: Expose task dependencies in the plugin and canvas data API

## Overview

A workspace canvas or gRPC plugin reading the Host Data API's `Task` read model
could not see a single dependency edge, even though the kanban board already
derives and ships a complete view (`blocked`, `blocked_reason`, `depends_on`,
`blocks`, `start_when_unblocked`) via `BuildDependencyViews`. This plan adds the
same seven-field, read-only projection to every plugin task read model across
the gRPC surface (proto, SDK, `grpcHostServer`), the canvas JSON surface
(`webapp_protocol_json.go`), and the shared derivation the two reuse
(`internal/task/service/service_dependencies.go`), landing it as a field on the
existing `api_read:tasks` capability with no new grant. It builds on the
plugin Host Data API's existing task read implementation (ADR 0043,
`docs/plans/plugins/host-data-api/`).

## Scope

See `task-01-dependency-projection.md` for the full field set, attachment
rule, withheld-verdict semantics, fan-out bound, edge-end redaction, and the
shared-derivation fixes this plan lands.

## Verification

- `go test ./internal/plugins/... ./internal/task/service/... ./internal/task/repository/sqlite/... ./internal/office/repository/sqlite/... ./pkg/pluginsdk/...`
- `golangci-lint run ./...` (backend)
- `gofmt -l` on touched files

## Work orders

- [x] [Task 01: Expose task dependencies in the plugin and canvas data API](task-01-dependency-projection.md)
