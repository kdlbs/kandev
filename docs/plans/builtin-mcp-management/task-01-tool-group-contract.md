---
id: "01-tool-group-contract"
title: "Package catalog and isolated registration"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-001
  - REQ-AGENTS-BUILTIN-MCP-002
  - REQ-AGENTS-BUILTIN-MCP-003
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-001.2
  - AC-AGENTS-BUILTIN-MCP-002.1
  - AC-AGENTS-BUILTIN-MCP-002.2
  - AC-AGENTS-BUILTIN-MCP-002.3
  - AC-AGENTS-BUILTIN-MCP-002.4
  - AC-AGENTS-BUILTIN-MCP-002.5
  - AC-AGENTS-BUILTIN-MCP-003.2
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
---

# Task 01: Package catalog and isolated registration

## Summary

Introduce the authoritative five-package catalog and disjoint MCP server views.
Keep legacy launch behavior intact until Task 02 integrates selection and trusted
delivery. This is a compatible expansion, with no writable policy API yet.

## In scope

Typed package IDs and validated versioned manifest; package descriptors; exact
tool/action membership; extraction of shared Core discovery/lifecycle tools;
separate registration/transport views; profile clone/normalize/equality/rebuild
preservation. Prove canonical name/schema parity with existing surface catalogs.
The package host shares the backend client and owns atomic updates/shutdown.

## Out of scope

Persisted selections, public writers, launching package mode, UI, new probes,
changing required workflow authority, ordinary catalog records, and sibling work.
Existing external/automation/coordinator/managed catalogs remain unchanged.

## Acceptance

1. Five package catalogs have unique ownership and the documented exact
   Configuration membership. Default package unions equal prior surface names
   and schemas. All 16 eligible Kanban combinations and both Office states pass.
2. Disabled package routes and cross-package HTTP/SSE session reuse cannot
   invoke handlers. Core has no hidden aggregate route in package mode. Legacy
   mode remains unchanged; choosing the mode is a trusted host operation.
3. Invalid manifests, unsupported versions, duplicate IDs, Core exclusion, and
   invalid surface grants fail closed. Clone/JSON/SetMode/SetProfile/provider and
   plugin rebuilds retain the manifest. Descriptor/action completeness is tested.

## Verification

Run from repository root after `/tdd` implementation:

```bash
(cd apps/backend && go test -trimpath ./internal/mcp/profile ./internal/mcp/server -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/api -run 'Builtin|MCPRoutes' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/mcp/profile ./internal/mcp/server -run 'Builtin|ServerProfile|ServerMode|SetPluginTools|SetMode|SetProfile' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with actual changed Go paths and record the
exact command in Results. Keep tests in bounded files. Run any changed suite not
matched by the focused patterns explicitly.

## Files likely touched

- `apps/backend/internal/mcp/profile/{profile.go,builtin_packages.go,builtin_packages_test.go}`.
- `apps/backend/internal/mcp/server/{server.go,config_handlers.go,tool_packages.go,builtin_packages_test.go}`
  and bounded package host/route files; existing server catalog tests.
- `apps/backend/internal/agentctl/server/api/` package route wiring and
  `builtin_mcp_routes_test.go` where the host integration requires it.

New filenames are proposals. Extract helpers instead of growing large functions.

## Dependencies

None. The old launch path remains usable until Task 02.

## Inputs

Full requirements and design sections Packages, Membership, Transport, and Binding.
Read existing server catalog/profile/route and sysprompt parity tests. Preserve
exact provider, title, question, transfer, automation, and managed policy gates.
Follow backend and scoped agentctl/API AGENTS.md.

## Risks

Existing registration groups overlap. Duplicating them across servers can replace
schemas or handlers. A new package must not inherit plugin tools by accident.
SSE session/message routing and SetMode reconstruction can bypass isolation.

## Parallelism

`sequential`

## Results

Completed on 2026-10-24. Added the explicit native tool catalog, package
metadata/actions, validated manifest carrier, isolated HTTP/SSE registries, and
rebuild preservation. Package mode remains inactive in normal launches until
Task 02 supplies trusted binding and enforcement.

TDD failures covered optional tools leaking into Core, missing package routes,
Configuration task registration, profile rebuilds, cross-package session IDs,
unknown native tools, plugin retention, manifest equality, and modern notification
compatibility. Each failure passed after its corresponding change.

Validation passed:

- `go test -trimpath ./internal/mcp/profile ./internal/mcp/server -count=1`
  from `apps/backend`: full profile and server suites.
- `go test -trimpath ./internal/agentctl/server/api -run 'Builtin|MCPRoutes' -count=1`.
- After the final transport change,
  `go test -trimpath ./internal/mcp/server -run 'Builtin|StreamableHTTPHandler|SupplementNotification' -count=1`.
- `go test -trimpath -race ./internal/mcp/profile ./internal/mcp/server -run 'Builtin|StreamableHTTPHandler|SupplementNotification|ServerProfile|ServerMode|SetPluginTools|SetMode|SetProfile' -count=1`.
- `git diff --check`.
- Changed-file Go lint: zero issues with CI-pinned golangci-lint 2.9.0. Command:

```bash
bash scripts/lint-go-changed \
  apps/backend/internal/mcp/profile/profile.go \
  apps/backend/internal/mcp/profile/builtin_packages.go \
  apps/backend/internal/mcp/profile/builtin_packages_test.go \
  apps/backend/internal/mcp/profile/builtin_catalog.go \
  apps/backend/internal/mcp/profile/builtin_descriptors.go \
  apps/backend/internal/mcp/profile/builtin_descriptors_test.go \
  apps/backend/internal/mcp/server/server.go \
  apps/backend/internal/mcp/server/builtin_packages.go \
  apps/backend/internal/mcp/server/builtin_configuration.go \
  apps/backend/internal/mcp/server/builtin_packages_test.go \
  apps/backend/internal/mcp/server/builtin_package_transport_test.go \
  apps/backend/internal/mcp/server/builtin_catalog_test.go \
  apps/backend/internal/mcp/server/notification_protocol_compatibility.go
```

Evidence uses local HTTP/SSE test servers and backend fakes. No installed-provider
or live runtime claim is made. The main runtime and protected preview ports were
not used.
