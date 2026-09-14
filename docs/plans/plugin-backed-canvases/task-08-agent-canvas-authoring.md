---
id: "08-agent-canvas-authoring"
title: "Agent canvas authoring"
status: done
wave: 8
depends_on:
  - "06-canvas-lifecycle"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-002
  - REQ-CANVASES-AGENT-WEB-APPS-008
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-001.1
  - AC-CANVASES-AGENT-WEB-APPS-001.2
  - AC-CANVASES-AGENT-WEB-APPS-001.3
  - AC-CANVASES-AGENT-WEB-APPS-001.4
  - AC-CANVASES-AGENT-WEB-APPS-001.5
  - AC-CANVASES-AGENT-WEB-APPS-001.6
  - AC-CANVASES-AGENT-WEB-APPS-002.2
  - AC-CANVASES-AGENT-WEB-APPS-002.3
  - AC-CANVASES-AGENT-WEB-APPS-008.2
  - AC-CANVASES-AGENT-WEB-APPS-008.3
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 08: Agent canvas authoring

## Summary

Give task agents a scoped create, inspect, publish, skill-read, and state
workflow. Transfer source through one bounded streaming contract for every
executor type.

## In scope

- Add gated canvas create, list, get, publish, skill-read, and state MCP tools.
- Derive canvas identity and scope from trusted task or edit-session context.
- Add a canvas-owned system-skill embed and materialization directory.
- Keep the canvas skill outside Office embeds, rows, and task workspaces.
- Add the allowlisted Canvas MCP skill reader.
- Add a canvas-specific source root and manifest scaffold.
- Add the authenticated agentctl tar stream for the assigned root.
- Enforce publish rate and one in-flight publish per canvas.
- Teach the skill about opaque-origin storage, HTTP routes, limits, and UI
  patterns.
- Add local, Docker, and remote source-transfer tests.

## Out of scope

- Human promotion, release approval, rollback, Quick Chat launch, and host
  management pages.

## Acceptance

- An agent creates and publishes only for its trusted task or edit target.
- Every executor reads the same current skill without workspace injection.
- A failed transfer, validation, rate, or storage check preserves the active
  release.

## Verification

```bash
cd apps/backend && go test ./internal/mcp/server/... ./internal/canvas/... ./internal/agentctl/server/api/... ./internal/agent/runtime/...
```

## Files likely touched

- `apps/backend/internal/mcp/server/canvas_tools.go`
- `apps/backend/internal/mcp/server/canvas_tools_test.go`
- canvas-owned embedded skill source and materializer
- `apps/backend/internal/canvas/authoring.go`
- `apps/backend/internal/canvas/publish.go`
- `apps/backend/internal/agentctl/server/api/**`
- `apps/backend/internal/agent/runtime/**`
- `apps/backend/config/prompts/**`

## Dependencies

- Task 06 provides canvas metadata, limits, and lifecycle services.
- Tasks 02 through 05 provide validation, state, grants, and runtime protocol.

## Risks

- A source stream can escape the assigned root through a link or race.
- A shared skill directory can enter Office discovery.
- A session-supplied canvas ID can bypass the trusted edit target.

## Parallelism

`sequential`

## Inputs

- Agent lifecycle, authoring guidance, tool, source-transfer, limit, and publish
  sections.
- Current MCP task scoping and executor file-transfer patterns.

## Results

Implemented scoped Canvas MCP authoring and state tools, the separate
canvas-authoring skill inventory, bounded authenticated agentctl tar transfer,
trusted executor source resolution, publish admission and validation, and
failure-safe pending/active release ownership.

Verification:

- `go test ./internal/mcp/server/... ./internal/canvas/... ./internal/agentctl/server/api/... ./internal/agent/runtime/... -count=1` — passed.
- `make -C apps/backend lint` — passed.
