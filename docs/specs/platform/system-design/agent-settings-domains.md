---
status: draft
system: platform
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-006
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-007
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-008
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-009
---

# Core domain adoption for agent-accessible settings

## Purpose and boundaries

Platform owns settings access across interfaces, not the underlying domain models.
This design extends the [shared tool contract](agent-settings-parity.md) to the main Kandev settings domains.
Agent profiles are the first vertical slice, not the final scope.
The package does not require a new settings file or central persistence store.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-006` | Required domains, Completion boundary |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-007` | Domain adapters, Authorized targets |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-008` | Mutation semantics, Relationships, Visible results |
| `REQ-PLATFORM-AGENT-SETTINGS-PARITY-009` | Explicit exceptions, Lifecycle operations |

## Required domains

Backend paths below are relative to `apps/backend/`. Resource type names are proposed stable identities.
The table defines domain boundaries, not another runtime field allowlist.

| Domain and resource types | Source contracts and services | Required coverage |
| --- | --- | --- |
| Profiles: `agent_profile`, `agent_profile_mcp` | `internal/agent/settings/dto/`, `controller/`, `handlers/` | Concrete and dynamic profiles, launch fields, model choices, separate MCP document |
| Agent definitions: `agent` | `internal/agent/settings/handlers/handlers.go`, `controller/` | Editable name, enabled state, and custom-agent definition fields. Computed capabilities remain read-only. |
| Personal: `user_settings` | `internal/user/dto/dto.go`, `controller/`, `service/` | Portable task behavior, keybindings, terminal preferences, appearance, views, presets, utility defaults, and LSP preferences |
| Workflows: `workflow`, `workflow_step` | `internal/task/dto/requests.go`, `internal/workflow/controller/controller.go`, `service/` | Workflow names and prompts, step bindings, events, session policies, admission limits, and editable ordering settings |
| Workspace: `workspace`, `repository`, `repository_script`, `repository_set` | `internal/task/dto/requests.go`, `handlers/`, `service/` | Workspace defaults, repository identity fields permitted by the UI, branch policies, scripts, copy rules, and repository sets |
| Execution: `executor`, `executor_profile`, `environment` | `internal/task/dto/requests.go`, `internal/task/handlers/`, `service/`, `internal/agent/executor/` | Current executor types, profile preparation settings, supported connection fields, secret references, and environment definitions |
| Task: `task` | `internal/task/dto/requests.go`, `internal/task/service/`, `internal/mcp/handlers/` | Editable task metadata and permitted launch defaults. Session actions and workflow movement remain lifecycle operations. |
| Prompts: `prompt` | `internal/prompts/dto/dto.go`, `controller/`, `service/` | Saved name and content with existing built-in restrictions |
| Utilities: `utility_agent` | `internal/utility/dto/dto.go`, `controller/`, `service/`, `profilebinding/` | Prompt, name, enabled state, and canonical profile binding. Legacy migration fields do not become new controls. |
| Editors: `editor` | `internal/editors/dto/dto.go`, `controller/`, `service/` | Saved editor definitions and kind-specific settings. Installation detection remains read-only. |
| Notifications: `notification_provider` | `internal/notifications/dto/dto.go`, `controller/`, `service/` | Provider name, enabled state, event subscriptions, and validated provider settings |
| Issue integrations: `jira_settings`, `linear_settings`, `sentry_instance`, and their watch types | `internal/jira/`, `internal/linear/`, `internal/sentry/` | Noncredential defaults, filters, existing instance settings, watch bindings, and watch schedules |
| Code-host integrations: `github_settings`, `gitlab_settings`, `azure_devops_settings`, and their watch/preset types | `internal/github/`, `internal/gitlab/`, `internal/azuredevops/` | Repository scope, defaults, action/query presets, and issue/review watch settings |
| Automations: `automation`, `automation_trigger` | `internal/automation/models.go`, `service.go`, `handlers.go` | Existing workspace automation definitions, continuation policies, bindings, and trigger schedules |
| Runtime: `runtime_flag` | `internal/runtimeflags/registry.go`, `service.go`, `handlers.go` | Existing mutable overrides except authentication enablement. Reads include source, lock, and pending-restart state. |
| Storage: `storage_maintenance` | `internal/system/storage/settings.go`, `handler.go`, policy types | Schedule and retention policies through the current settings operation, without running cleanup |

Task behavior preferences belong to `user_settings`, not the current task.
Portable colors and task-view preferences remain personal even when their values contain task IDs.
Workspace integration settings do not become personal preferences because a browser owns the current workspace selection.
Sentry uses instance identity plus workspace identity. It must not collapse into a workspace singleton.

## Domain adapters

Each owner contributes metadata, typed request schemas, target rules, read projections, resource lookup, and mutation adapters.
Adapters use the shared catalog interfaces. Backend composition injects existing controllers and services.
The shared catalog never imports every domain or dispatches arbitrary HTTP routes supplied by callers.

Schema extraction follows the actual wire contract, not an untagged internal struct's Go field names.
Several task DTOs lack JSON tags. Their current HTTP request bodies need shared, tagged domain contracts before schema generation.
Existing camelCase integration keys remain camelCase where that is their public contract.
Canonical catalog keys remain stable, with `field_path` identifying the actual wire field.

Opaque JSON fields need domain-specific schemas or a documented typed-map boundary.
An `interface{}` or `json.RawMessage` alone is not evidence that arbitrary nested writes are safe.
Computed fields, compatibility inputs, workflow state, credential contents, and operational history receive explicit classifications.

One composition registry serves the exporter and production registration through the same metadata constructors.
The exporter has no credentials, live database, provider probes, or runtime services.
Its all-domain snapshot is `apps/web/lib/settings-discovery/contract.generated.json`.
The profile-only snapshot remains a derived subset for the existing profile work orders.

## Authorized targets

The shared target and lookup rules are defined in the main design.
Personal adapters call `user.Controller.GetUserSettings` and `UpdateUserSettings` with trusted context, without an arbitrary user ID.
Workspace resources use existing task-service authorization. Workflow steps use the workflow controller's scoped entry points.
Install-owned writes enforce the exact existing permission, not a universal catalog permission.
For example, profile mutation uses `org.config.manage`, while runtime overrides use `org.settings.manage`.

Static metadata is available without reading a saved resource.
Detailed values, resource names, references, and contextual choices require the same visibility checks as their domain UI.
Adapters reject missing identity when authentication is enforced, including services that otherwise interpret missing identity as an internal caller.
No generic endpoint permits a caller to set identity, ownership, tool profile, or authorization mode.

## Mutation semantics

Each update represents one domain operation. It preserves omission, explicit empties, nullability, defaults, and current replacement rules.
Maps and arrays are not universally deep-merged. Descriptions state their exact semantics.
Related fields can travel together in one patch, such as a utility profile binding and its supported policy fields.

Existing full-document operations require one of two explicit adapters:

1. A complete, validated document field with replacement semantics.
2. A domain-owned atomic patch shared by every writer.

An MCP handler must not reconstruct a full document from a redacted read.
Integration updates preserve omitted credentials through the existing domain secret-preservation path.
Changes to connection identity that reuse stored credentials retain the provider's existing reauthorization requirements.
Credential enrollment and authentication-method changes remain interactive-only in this delivery.

Runtime flags expose one resource per flag key. `changes.override` accepts a boolean or null to clear the stored override.
`Service.SetOverride` retains environment locking, registry mutability, and restart behavior.
The adapter adds the same organization-scope check as HTTP before calling that service.
The `features.auth` descriptor is discoverable but interactive-only for writes.
No shipped profile defaults, retired identities, or feature gate behavior change in this package.

Storage policies use a complete `changes.settings` document because the current route accepts a complete policy.
The adapter calls the same settings operation and `OnSettingsChanged` callback as the HTTP route.
The current dedicated-Docker acknowledgement remains an exact token in domain-validated `options.confirmations`.
Go-cache path adoption remains a separate action. A generic patch cannot set the private adoption permission.

Saving an enabled watch, automation, or maintenance schedule can affect future background work.
Descriptions explain that consequence. The domain's existing enablement, confirmation, and validation rules remain mandatory.
Saving does not imply an immediate run or create a new scheduler.

## Relationships

Each adapter validates references through the owning service, not through catalog metadata alone.

| Invariant | Required evidence |
| --- | --- |
| Workflow step references a usable profile | Missing, inaccessible, incompatible, disabled, and valid profile cases |
| Step event references another step | Same-workflow validation and rejection of a foreign step |
| Workspace default references an executor/profile | Authorized, compatible target and documented clearing behavior |
| Repository copy rule stays contained | Existing copyfiles validation rejects traversal and malformed specifications |
| Utility agent keeps a valid profile binding | Dependency conflicts, built-in restrictions, and legacy migration inputs |
| Watch references match its workspace | Wrong-workspace workflow, profile, repository, and instance cases |
| Task configuration respects running sessions | No hidden move, start, reassignment, or runtime rewrite |
| Runtime override respects deployment policy | Environment lock, unknown/retired key, null reset, and pending restart |
| Storage policy preserves adoption safety | No unconfirmed dedicated daemon or private Go-cache adoption |

When a valid mutation updates several rows, the existing domain transaction remains the atomicity boundary.
New partial-update paths need equivalent protection across all writers, not only MCP callers.
Tests exercise competing writes for any added read-modify-write path and verify rollback after an injected persistence failure.
This package does not add universal optimistic locking. Existing dynamic versions and domain concurrency checks remain intact.

## Visible results

Every domain work order includes its existing store subscription and editor reconciliation.
The profile implementation establishes clean, dirty, submitted, and conflicted states through the existing save coordinator.
Other editors reuse that behavior where applicable, without moving domain state into the coordinator.

Existing events remain authoritative. Adapters must not duplicate event publication.
When no suitable event exists, the domain adds a value-free invalidation after persistence.
Invalidations route to the owning user or workspace, or authorized install-setting readers. They never broadcast private values globally.
Refetches use target identity and request-generation guards. An invalidation during a pending read requires a fresh read.
Delivery order without a domain revision is not proof that one snapshot is newer.

Personal settings reuse `apps/web/lib/user-settings-sync.ts` and the shared wire mapper.
Workflow drafts reuse `use-workflow-draft-contributor.ts` and `workflow-dirty-state.ts`.
Executor profiles reuse the contributors under `components/settings/profile-edit/`.
New invalidation handlers feed these existing paths instead of updating only the page that initiated the change.

Phone and desktop keep their current settings routes, content scroll owner, and shared save controls.
Conflict recovery stays inline and uses the current discard action. No new overlay or navigation model is required.
Existing mobile profile, executor, notification, prompt, and utility settings tests provide the nearest shipped examples.
New copy uses all five locales. New recovery actions preserve touch targets, keyboard access, and safe-area clearance.
Each domain's browser scenario invokes real MCP, observes the UI, then verifies persistence after reload.

## Explicit exceptions

Every exception has a stable identity, owner, category, reason, and available recovery destination.

| Category | Included examples | Discovery behavior |
| --- | --- | --- |
| `client-local` | Settings-menu shape, chart-motion preference, pane sizes, collapsed navigation | Explain the device boundary and link to the control. No remote browser bridge. |
| `read-only` | Installation detection, runtime health, computed capabilities, active execution snapshots | Return authorized nonsensitive state only. Never promote computed response fields into updates. |
| `interactive-only` | Login, token enrollment, account authority, authentication enablement, credential-bearing connection identity changes | Explain the required user flow without reading credentials. |
| `action` | Install, restart, restore, immediate cleanup, resource creation/deletion, workflow reordering | Identify an existing explicit tool or the UI action. Never encode the action as an arbitrary setting. |
| `deployment-owned` | Startup YAML/environment, database location, launcher binaries, server ports | Derive safe metadata from `internal/common/config/catalog.go` and source provenance. No file or environment writes. |
| `plugin-owned` | Plugin-specific settings schemas and storage | Identify the plugin settings destination. No arbitrary plugin storage mutation or new plugin API. |
| `separate-product-contract` | Office organization, projects, channels, and autonomous workforce management | Identify Office's existing tools or UI. Workspace automations remain in scope. |

Host-owned keyboard overrides include the typed plugin-shortcut override map where it already belongs to user settings.
That does not expose arbitrary plugin-owned settings or infer action availability when the plugin is absent.
Transient form drafts, navigation history, setup progress, and internal migration fields are not user configuration controls.

New value reads redact declared credential paths. Search and resource lookup never inspect arbitrary saved values.
Free-form prompt and script bodies are returned only by explicit authorized value reads, not search snippets or logs.
The catalog does not claim that it can detect every secret a user writes into free-form text.
Legacy external raw-MCP-document reads retain their current exposure contract.

## Lifecycle operations

Four settings tools plus one generic resource lookup form the shared settings interface.
They do not replace explicit lifecycle tools or turn updates into silent upserts.
Existing create/delete/import/reorder tools remain available through their current domain owners.
New fields on adopted create tools use a validated `settings` object, with duplicate legacy arguments rejected.

For prompt, utility-agent, editor, and notification-provider setup, domain work orders add explicit create adapters where no MCP create tool exists.
These tools use parent selectors where required and one validated settings object.
The catalog describes their complete create schema on demand. The outer tool definitions do not grow with new fields.
Creation preserves built-in restrictions and does not install, authenticate, execute, or test external credentials.
Other new lifecycle families, including provider account enrollment and resource destruction, remain outside this package.

## Completion boundary

The inventory covers Settings routes, navigation definitions, independent UI payload types, backend mutation bodies, and persisted response fields.
Navigation entries alone cannot prove completeness because some entries represent sections instead of individual fields.
The first work order produces a checked inventory from these inputs, including missing navigation controls and explicit exception records.

CI compares independent inputs against the catalog and the inventory. It must detect a new route, domain, nested typed field, or editable control.
Negative fixtures add each kind of omission and remove a descriptor. Every fixture must fail the appropriate coverage assertion.
Dynamic maps count as typed contracts, not as one new setting per saved key or provider option.
The completion report separates supported fields, read-only fields, exceptions, and pending work by domain.
Every eligible field in the required domains must be supported. Pending work cannot be relabeled as an exception to pass delivery checks.
The report does not claim a coverage percentage until the independent inventory establishes its denominator.

## Related decisions

- [Domain-owned settings catalog](../../../decisions/2026-09-08-domain-owned-settings-catalog.md)
- [Portable personal settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
- [Runtime overrides](../../../decisions/0018-runtime-settings-overrides.md)
- [Workspace integration settings](../../../decisions/0030-workspace-scoped-integration-settings.md)
