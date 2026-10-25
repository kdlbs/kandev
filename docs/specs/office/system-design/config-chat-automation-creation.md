---
status: current
system: office
requirements:
  - REQ-OFFICE-CONFIG-AUTOMATION-001
---

# Configuration Chat Automation Creation System Design

## Purpose and boundaries

Expose the existing `automation.Service.CreateAutomation` contract through a
configuration-chat MCP tool. Preserve Office ownership of automation persistence
and scheduling. This is an additive client, without a new storage layer or UI.
See [requirements](../requirements/config-chat-automation-creation.md) and
[automation targets](automation-target-modes.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-OFFICE-CONFIG-AUTOMATION-001 | Catalog, Request and response, Dispatch and authorization, Instructions and verification |

## Catalog

Add a separate configuration-only group to `Server.profileToolGroups` in
`internal/mcp/server/server.go`. Do not attach creation to
`registerConfigWorkflowTools`: that group is also composed by external and
fixed automation catalogs. Register `create_automation_kandev` in a new
`config_automation_handlers.go`, using the established `wrapHandler`, argument
validation, and `forwardToBackend` path.

Define `ActionMCPCreateAutomation` (`mcp.create_automation`) in
`pkg/websocket/actions.go`. Describe the tool as a non-idempotent mutation and
make its persisted enablement behavior explicit. Rebuilding tool validators and
mode transitions must include/remove it through the normal registry path.

## Request and response

Use `automation.CreateAutomationRequest` as the backend DTO. The MCP schema
exposes `workspace_id` and `name` as required, plus the current optional fields:
`description`, `workflow_id`, `workflow_step_id`, `agent_profile_id`,
`executor_profile_id`, `repositories`, `repository_ids`, `prompt`,
`task_title_template`, `max_concurrent_runs`, `continuation_policy`, `task_mode`,
`repository_mode`, and `triggers`. Preserve structured repository entries and
nested trigger config objects rather than accepting a JSON-encoded string.

Document supported trigger types from `automation/models.go` and their config
shapes. Scheduled triggers use `config.cron_expression`; reuse existing service
validation for cron and plugin/provider-specific configurations. Do not invent
required workflow/repository fields for hidden runs. Omitted values flow to the
service: hidden `automation_run`, no repositories, `new_task` continuation, and
one concurrent run. Trigger `enabled` remains caller-controlled and follows the
existing DTO's false zero value; the automation itself is created enabled.

Return `automation.CreateAutomationResponse`, including the existing one-time
webhook secret field. The response reflects persisted triggers, not the submitted
request. Preserve normal secret redaction on subsequent reads, and do not add
payload/secret logging. Reuse existing creation behavior rather than changing
secret delivery as part of this feature.

## Dispatch and authorization

Add a narrow automation-creation service dependency to MCP `Handlers`, with a
setter and guarded action registration in `registerConfigModeHandlers`.
Wire `p.services.Automation.Service` from `registerMCPAndDebugRoutes` in
`internal/backendapp/helpers.go`; handle an unavailable subsystem without a nil
panic or successful response.

Decode the typed request, call `CreateAutomation` with the original scoped
context, guard its possible nil result, and return the shared response shape.
Follow existing MCP error envelopes. Do not write directly to the automation
store or duplicate the automation service's validation rules.

The in-session scope resolver supplies authenticated user identity; preserve it
through the service's `authorizeWs` and related ownership checks. Workspace IDs
remain explicit and follow existing authorized-workspace semantics, rather than
silently rebinding to the chat workspace. The current principal surface resolver
does not distinguish configuration tasks from ordinary tasks: do not use it as
a configuration-mode assertion. Catalog exposure and authenticated resource
access are separate controls, as in existing configuration tools.

Keep the new action out of `automationSurfaceActions` so raw automation-run
requests are denied. Do not broaden external tool discovery. Existing transport
permissions and configuration-chat task/session identity remain intact.

## Persistence and failure behavior

Reuse the service without migrations. It validates configuration before writes,
but currently logs trigger storage failures after the automation insert and
returns the persisted record. This change does not redesign that behavior or
promise atomic creation. The agent must report returned state accurately and
must not blindly retry an uncertain creation, since there is no idempotency key.
Creation does not call the run endpoint; enabled triggers follow existing
scheduler behavior. Existing reads expose the saved record without a new event
or refresh protocol.

## Instructions and verification

Add an automation section to `config/prompts/config-context.md`: use existing
workspace, workflow, repository, agent and executor discovery, or settings
resource discovery; explain optional target settings, trigger enablement and a
scheduled example. Do not require a new confirmation flow. Update the creation
section of `docs/public/automation-and-mcp.md` when implementation ships.

Test configuration-only discovery, mode transitions, nested argument forwarding,
service delegation, malformed inputs, unauthorized workspaces/references, nil
service/results, and raw automation action denial. An integration test must run
an MCP call through real scoped dispatch and the real automation service/store,
then read the persisted automation and triggers via an existing read path.
This provides end-to-end evidence for the new MCP surface without requiring
nondeterministic model output or adding browser markup.
