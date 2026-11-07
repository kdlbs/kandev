---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-TASK-DISCOVERY-001
---

# Task MCP executor discovery design

## Ownership and requirement mapping

The executor system owns the catalogue and profile eligibility.
REQ-EXECUTORS-TASK-DISCOVERY-001 maps to the registration, data flow, and
compatibility sections below. Identity and admission remain governed by
[per-user ownership](per-user-ownership.md).

## Registration and tool fit

Expose the existing `list_executors_kandev` operation in `registerKanbanTools`
in `apps/backend/internal/mcp/server/server.go`. Use a small shared registration
helper with `registerConfigExecutorTools` in `config_handlers.go` so the same
read operation has consistent schema and guidance. Do not enable the entire
configuration-executors group for task profiles.

Use the existing empty-object raw schema with `properties: {}` and wrapped
`listExecutorsHandler`. Describe the nested profiles and their use with
`create_task_kandev`. Keep `executor_id` required on `list_executor_profiles_kandev`
and point its task-mode guidance to `list_executors_kandev`.

This reuses an established tool rather than defining a new operation.
Automation already exposes both operations. `list_agents_kandev` similarly
returns agents with profiles. Making `executor_id` optional would overload the
profile-list contract and require new branching or a second response shape.
Removing the profile tool would break existing callers. Adding executor data
to `list_agents` would mix independent inventories. The existing tool is the
smallest compatible fit; no new architecture boundary requires an ADR.

## Data flow and identity

The server forwards `mcp.list_executors` through the existing backend client.
`handleListExecutors` in `internal/mcp/handlers/config_executor_handlers.go`
calls `taskSvc.ListExecutors` and `ListAllExecutorProfiles` with the same context.
It groups profile DTOs by executor ID and returns `ListExecutorsResponse`.

`internal/task/service/service_resources.go` filters executor owners and uses
`visibleExecutorProfiles` for profile eligibility. Preserve the trusted MCP
scope resolver and both parent and profile ownership checks. Do not accept a
caller-supplied user identity. Preserve the existing DTO fields, totals, and
empty nested-profile representation. A visible executor with no visible profiles
remains visible; discovery is not a runtime health or readiness guarantee.

A caller can pass a returned profile ID to the existing task creation path.
Task MCP marks only caller-supplied profile IDs as explicit selections. For a
deferred task, the service checks current ownership for an explicit selection
before writing the task. A profile inherited from source or session metadata
continues through the existing deferred flow and is checked when launch is
admitted. There is no authorization cache implied by a listing.

## Compatibility and failure behavior

- Kanban task: add the read tool and retain the profile-list contract.
- Configuration and external: retain existing reads and mutations.
- Automation: retain its fixed catalogue; shared registration must not add tools.
- Office, coordinator, and managed conversation: retain existing membership.

Keep existing backend errors. If either listing fails, return an error rather
than a successful partial response. Empty executor inventories return `[]`
and total zero. No migration, new storage, metrics, or feature flag is needed.
Existing provider inventory synchronization remains unchanged.

## Evidence and delivery

`TestServerModeTask_RegistersCorrectTools` currently asserts the tool is absent.
`TestServerSurfaceAutomationHasFixedCoordinatorCatalog` already includes it.
The existing config handler demonstrates response forwarding. Add a task-mode
integration test through the real dispatcher, scope resolver, and service to
prove custom-ID discovery and ownership filtering.

[Implementation plan](../../../plans/task-mcp-executor-discovery/plan.md).
