---
id: "01-mcp-discovery-guidance"
title: "Compact MCP discovery guidance"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-DISCOVERY-001
  - REQ-AGENTS-MCP-DISCOVERY-002
  - REQ-CANVASES-AGENT-WEB-APPS-001
acceptance_criteria:
  - AC-AGENTS-MCP-DISCOVERY-001.1
  - AC-AGENTS-MCP-DISCOVERY-001.2
  - AC-AGENTS-MCP-DISCOVERY-001.3
  - AC-AGENTS-MCP-DISCOVERY-001.4
  - AC-AGENTS-MCP-DISCOVERY-001.5
  - AC-AGENTS-MCP-DISCOVERY-001.6
  - AC-AGENTS-MCP-DISCOVERY-002.1
  - AC-AGENTS-MCP-DISCOVERY-002.2
  - AC-AGENTS-MCP-DISCOVERY-002.3
  - AC-AGENTS-MCP-DISCOVERY-002.4
  - AC-AGENTS-MCP-DISCOVERY-002.5
  - AC-CANVASES-AGENT-WEB-APPS-001.10
  - AC-CANVASES-AGENT-WEB-APPS-001.11
  - AC-CANVASES-AGENT-WEB-APPS-001.12
system_design:
  - ../../specs/agents/system-design/mcp-tool-discovery-guidance.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 01: Compact MCP discovery guidance

## Summary

Replace the partial catalog with discovery instructions and essential workflow
guidance. Add canvas authoring instructions only for eligible task sessions.

## In scope

- Use the proposed discovery wording and essential-section table in the design.
- Add a canvas option to `KandevContextOptions` and resolve it through the
  executor's existing `resolveTaskSessionMCPProfile` path.
- Cover prepared-session launch, normal launch, and workflow context reset.
- Preserve marker canonicalization, user content, task/session identities,
  question barriers, title ownership, completion, autopilot, and delegation.
- Preserve coordinator interrupt/stop/restart guidance and its condition.
- Use TDD for option and producer changes. Add small dedicated test files.

## Out of scope

- Tool registration changes or duplicate feature-flag state.
- Canvas locale changes, live-session restarts, or unsolicited messages.

## Acceptance

1. A missing callable tool triggers search/catalog guidance, while directly
   available tools need no redundant discovery. The raw template is at most
   2,800 UTF-8 bytes, compared with the 3,482-byte baseline.
2. All task prompt producers advertise canvas operations exactly when the
   resolved session profile exposes them. Recorded/dispatched text agrees.
3. Existing mandatory rules and MCP inventories remain intact. Tests cover
   flag-off, Office, configuration, automation, and passthrough cases.

## Verification

From `apps/backend`:

```bash
rtk go test ./internal/sysprompt ./internal/prompts ./internal/mcp/profile ./internal/mcp/server -count=1
rtk go test ./internal/orchestrator ./internal/orchestrator/executor -run 'Test.*(CanvasPrompt|MCPDiscovery|KandevContext|StartCreatedSession|AutoStart.*Context|MCPProfile)' -count=1
```

Name new producer cases with `CanvasPrompt` so the focused command selects
every new case. Include `StartTask` and reset paths within those cases.
Record normal and canvas-enabled rendered sizes in Results. Run the failing
new tests before implementing their behavior.

## Files likely touched

- `apps/backend/config/prompts/kandev-context.md`
- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/mcp_discovery_test.go` (new)
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/canvas_prompt_test.go` (new)
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/canvas_prompt_test.go` (new)

## Dependencies

None.

## Risks

The formatter currently has no resolved profile input. Keep any query narrow
and reuse the existing resolver. Do not infer eligibility from client identity
or add a second canvas flag owner.

## Parallelism

`sequential`

## Inputs

- Both linked requirements and designs.
- `TestFormatKandevContext_TitleToolFollowsCapability` and question barrier tests.
- `wrapCreatedSessionPrompt`, `applyLaunchPromptContext`, and workflow reset.
- Typed MCP profile and canonical tool-name ADRs linked by the agent design.

## Results

- Compacted `kandev-context.md` to a 2,743-byte UTF-8 template while retaining
  task/session identity, question barriers, title ownership, completion gates,
  autopilot, delegation, plan, rich-output, and coordinator guidance.
- Added conditional canvas authoring guidance and reused the executor's typed
  MCP profile resolver for normal launch, prepared-session launch, workflow
  context reset, and the first direct message path. Office, configuration,
  automation, and passthrough sessions remain excluded.
- Added discovery, profile, prepared-launch, reset, and prompt-rendering
  regression coverage. Recorded and dispatched reset prompts use the same
  resolved canvas capability.
- Rendered sizes are 4,758 bytes for the ordinary task context and 5,179 bytes
  with canvas guidance enabled.
- Verification: focused backend packages passed with 5,396 tests across seven
  packages; `rtk make lint` reported 0 issues; `rtk git diff --check` passed.
