---
id: "10-quick-chat-canvas-editing"
title: "Quick Chat canvas editing"
status: done
wave: 10
depends_on:
  - "09-promotion-release-management"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-004
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-004.1
  - AC-CANVASES-AGENT-WEB-APPS-004.2
  - AC-CANVASES-AGENT-WEB-APPS-004.3
  - AC-CANVASES-AGENT-WEB-APPS-004.4
  - AC-CANVASES-AGENT-WEB-APPS-004.5
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 10: Quick Chat canvas editing

## Summary

Launch a normal Quick Chat agent with one workspace canvas draft. Restrict the
session to that canvas and preserve published releases after workspace cleanup.

## In scope

- Add a canvas edit-session endpoint and trusted session metadata.
- Create a Quick Chat task with `canvas_edit` origin.
- Materialize the active release source in the ephemeral workspace.
- Tell the agent to read the canvas authoring skill through Canvas MCP.
- Restrict publish and state tools to the trusted target canvas.
- Preserve active and prior releases during Quick Chat cleanup.
- Add a frontend edit launcher with focused tests.

## Out of scope

- A direct source editor, one-shot utility-agent editing, and workspace
  management layout.

## Acceptance

- Edit canvas opens Quick Chat with the current source and trusted target.
- The edit session cannot publish or change state for another canvas.
- Session cleanup cannot remove a retained release.

## Verification

```bash
cd apps/backend && go test ./internal/canvas/... ./internal/task/handlers/... ./internal/mcp/server/...
cd apps && pnpm --filter @kandev/web test -- hooks components/canvas
```

## Files likely touched

- `apps/backend/internal/canvas/edit_session.go`
- `apps/backend/internal/canvas/edit_session_test.go`
- `apps/backend/internal/task/handlers/**`
- `apps/backend/internal/mcp/server/canvas_tools.go`
- Quick Chat workspace materialization and cleanup integration
- `apps/web/hooks/domains/canvas/**`

## Dependencies

- Task 09 provides workspace canvases, permission review, and release history.

## Risks

- Untrusted request fields can select another canvas.
- Quick Chat cleanup can erase source that a retained release still needs.
- An edit can activate new permissions without human review.

## Parallelism

`sequential`

## Inputs

- Quick Chat editing, authoring guidance, authorization, and failure sections.
- Current Quick Chat task and workspace lifecycle patterns.

## Results

Implemented trusted Quick Chat canvas edit-session materialization, target
metadata, source transfer, publish restriction, and retained-release safety.

Verification:

- Backend canvas, backendapp, MCP, and task integration tests — passed.
- Focused frontend canvas host, lifecycle, hook, and API tests — passed.
