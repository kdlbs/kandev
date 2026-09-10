---
system: canvases
owner: canvases
specification_version: 1
status: draft
migration: complete
last_updated: 2026-09-10
---

# Canvases

Canvases own agent-authored task applications and their promotion to workspace
applications. The system owns canvas scope, source lineage, release selection,
editing sessions, and discovery.

The Plugins system owns the isolated web-application runtime, permissions,
Kandev data access, state, events, and package validation. The task system
remains authoritative for task data and permissions.

## Requirements

- [Agent-authored web-app canvases](requirements/agent-authored-web-apps.md)
- [Owner-authorized creation](requirements/local-creation-authority.md)
- [Deprecated collaborative canvases](requirements/collaborative-canvases.md)

## System design

- [Guided canvas task launch](system-design/guided-canvas-task-launch.md)
- [Agent-authored web-app canvases](system-design/agent-authored-web-apps.md)
- [Owner-authorized creation](system-design/local-creation-authority.md)
- [Superseded collaborative canvases](system-design/collaborative-canvases.md)

## Related context

- [Runtime and permission fix plan](../../plans/canvas-runtime-permission-fixes/plan.md)
- [Direct creation and saved prompt plan](../../plans/canvas-direct-creation/plan.md)
- [Agent MCP discovery guidance](../agents/system-design/mcp-tool-discovery-guidance.md)
- [MCP discovery and canvas prompt plan](../../plans/mcp-discovery-canvas-prompts/plan.md)
- [GitHub Copilot App Canvas reference](../../copilot-canvas-reference.md)
- [Plugin-backed web-app canvases decision](../../decisions/2026-08-26-plugin-backed-web-app-canvases.md)
- [Plugin-backed canvases implementation plan](../../plans/plugin-backed-canvases/plan.md)
- [Canvas UX follow-up implementation plan](../../plans/plugin-backed-canvases-ux-follow-up/plan.md)
- [Historical declarative canvas plan](../../plans/collaborative-canvases/plan.md)
