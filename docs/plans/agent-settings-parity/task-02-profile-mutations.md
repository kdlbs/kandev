---
id: "02-profile-mutations"
title: "Deliver profile mutation parity"
status: done
wave: 2
depends_on:
  - "01-profile-contract"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-002
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-003
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-005
acceptance_criteria:
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.7
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.8
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-002.9
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-003.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.5
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
---

# Task 02: Deliver profile mutation parity

## Summary

Deliver compact profile updates and a validated settings object on the explicit create tool.
Prove equivalent persistence, rejection, authority, and notifications across REST and actual MCP invocation.

## In scope

- Add `update_settings_kandev` with a stable envelope and registry-selected field validation.
- Use the shared `target` object and domain-validated `options`. Profile dependency confirmation uses `options.force`.
- Add a `settings` object to the explicit create tool. Reject duplicates against legacy top-level fields.
- Keep existing external tools as compatibility adapters over shared typed contracts.
- Return safe structured field errors and discovery references without echoing values.
- Preserve model inheritance, explicit false, empty replacements, and existing successful argument shapes.
- Enforce organization scope from trusted MCP context and retain auth-disabled compatibility.
- Preserve structured errors across direct and streamed backend clients.
- Keep dynamic versions and confirmed dependency overrides in the domain path.
- Verify the existing separate MCP-document operation and its event behavior.
- Share MCP-document validation through the controller while retaining the existing HTTP and MCP omission rules.
- Add one value-free `agent.profile.mcp_config.updated` invalidation after each accepted document write.

## Out of scope

- New discovery tools, deletion/duplication expansion, and new runtime permissions.
- An unrestricted settings setter or atomic profile-plus-MCP save.

## Acceptance

- Real MCP calls and REST calls produce equivalent persisted profiles and one notification for each accepted mutation.
- Invalid fields, unauthorized identities, stale dynamic versions, and unconfirmed dependency conflicts cannot change saved state.
- Structured failures retain recovery details across both backend clients. Existing valid tool calls remain compatible.

The compact-envelope fixture must remain byte-identical after a new setting is registered.
The backend validator must accept that setting while rejecting unknown keys and invalid nested values.

## Verification

Run from the repository root.

```bash
rtk proxy go -C apps/backend test ./internal/mcp/server ./internal/mcp/handlers ./internal/mcp/scope ./internal/agent/settings/controller ./internal/agent/settings/handlers
rtk proxy go -C apps/backend run ./cmd/settings-catalog --check
rtk git diff --check
```

Create `TestProfileSettingsParity` around registered tools and a real dispatcher/controller fixture.
Direct handler calls alone do not prove schema and forwarding parity.
Use the existing workflow event parity fixture as the reference pattern.

## Files likely touched

- `apps/backend/internal/mcp/server/config_handlers.go`.
- `apps/backend/internal/mcp/server/settings_update.go` and tests (new).
- `apps/backend/internal/mcp/handlers/settings_update.go` and tests (new).
- `apps/backend/internal/mcp/server/backend_client.go` and `dispatcher_backend_client.go` with tests.
- `apps/backend/internal/mcp/server/profile_settings_parity_test.go` (new).
- `apps/backend/internal/mcp/handlers/config_agent_handlers.go` and `config_mcp_handlers.go`.
- `apps/backend/internal/mcp/handlers/profile_settings_authority_test.go` (new).
- `apps/backend/internal/mcp/handlers/handlers.go` and backend composition, if required for trusted auth-mode wiring.
- `apps/backend/internal/agent/settings/controller/` for shared validation or typed error placement.
- `apps/backend/internal/agent/settings/handlers/handlers.go` for the MCP-document adapter and notification.
- `apps/backend/internal/events/types.go`, `apps/backend/pkg/websocket/actions.go`, and gateway event routing for document invalidation.

## Dependencies

Task 01 supplies typed contracts, schemas, and field semantics.

## Risks

- Gin middleware does not run for in-process MCP dispatch.
- An added domain event can duplicate an existing transport publication.
- A generic error string can discard dependency references during transport conversion.
- The streamed path and external endpoint must not diverge on authority or errors.
- An open `changes` envelope is safe only with mandatory domain validation before mutation.

## Parallelism

`sequential`

## Inputs

- System design: Mutation adapters, Authority, Shared requests.
- `workflow_step_parity_test.go`, `config_agent_profile_auto_approve_test.go`, and profile CRUD tests.
- ADRs for MCP validation, tool profiles, and utility dependency safety.

## Results

Completed on 2026-09-09.

- Added compact profile mutation routing through the existing profile and MCP configuration services.
- Preserved omission and credential redaction rules, authorization, and profile MCP invalidation events.
- Covered the mutation and sensitive-field paths with backend tests and the real MCP desktop/mobile flow.
