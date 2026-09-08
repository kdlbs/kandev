---
id: "06-canvas-lifecycle"
title: "Canvas lifecycle"
status: done
wave: 6
depends_on:
  - "05-live-event-transport"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-002
  - REQ-CANVASES-AGENT-WEB-APPS-005
  - REQ-CANVASES-AGENT-WEB-APPS-007
  - REQ-CANVASES-AGENT-WEB-APPS-008
  - REQ-PLUGINS-ISOLATED-WEB-APPS-008
  - REQ-PLUGINS-ISOLATED-WEB-APPS-010
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-002.1
  - AC-CANVASES-AGENT-WEB-APPS-002.5
  - AC-CANVASES-AGENT-WEB-APPS-002.6
  - AC-CANVASES-AGENT-WEB-APPS-005.1
  - AC-CANVASES-AGENT-WEB-APPS-007.3
  - AC-CANVASES-AGENT-WEB-APPS-008.1
  - AC-CANVASES-AGENT-WEB-APPS-008.3
  - AC-CANVASES-AGENT-WEB-APPS-008.4
  - AC-PLUGINS-ISOLATED-WEB-APPS-008.1
  - AC-PLUGINS-ISOLATED-WEB-APPS-008.2
  - AC-PLUGINS-ISOLATED-WEB-APPS-010.4
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 06: Canvas lifecycle

## Summary

Create the plugin-backed canvas model from current `main`. Add atomic count
admission, task cleanup, workspace cleanup, archive, restore, and removal.

## In scope

- Add the canvas-to-plugin-instance model, repository, and services.
- Add task and workspace canvas list and get operations.
- Add lifecycle WebSocket events without application content.
- Enforce 10 canvases per task and 100 canvases per workspace.
- Count archived canvases and make restore admission atomic.
- Remove task canvases during task cleanup.
- Remove every canvas during workspace deletion.
- Record durable artifact cleanup before release ownership disappears.
- Add restart, concurrency, cleanup, and unavailable-release tests.

## Out of scope

- Declarative canvas cutover, agent publishing, promotion, editing, and host
  surfaces.

## Acceptance

- Canvas scope and active release identity survive backend restart.
- Concurrent create or restore cannot exceed the task or workspace limit.
- Workspace removal leaves no live canvas authority or untracked artifact.

## Verification

```bash
cd apps/backend && go test ./internal/canvas/... ./internal/gateway/websocket/... ./internal/task/service/... ./internal/backendapp/...
```

## Files likely touched

- `apps/backend/internal/canvas/**`
- `apps/backend/internal/gateway/websocket/canvas_*.go`
- `apps/backend/internal/backendapp/**`
- `apps/backend/internal/task/service/**`
- plugin artifact cleanup integration

## Dependencies

- Tasks 02 through 05 provide plugin instances, releases, runtime state, and
  event transport.

## Risks

- Task cleanup can erase a promoted canvas if scope checks use origin data.
- Workspace deletion can lose artifact cleanup inventory before commit.
- Restore admission can race with create without one transaction boundary.

## Parallelism

`sequential`

## Inputs

- Canvas model, state machine, limits, cleanup, API, and failure sections.
- Current task and workspace cleanup transaction patterns.

## Results

Implemented the canvas metadata repository and service, atomic task/workspace
admission, atomic canvas and plugin-instance lifecycle transactions, archive
and restore, task and workspace cleanup, lifecycle event projection, startup
orphan reconciliation, and durable artifact-cleanup jobs.

Verification:

- `go test ./internal/plugins/instances ./internal/canvas ./internal/backendapp ./internal/plugins/webapp ./internal/mcp/server -count=1` — passed.
- The feature-related packages in `go test ./... -count=1` passed. The full
  repository run still reports the known home-config discovery and launcher
  restart-fixture failures caused by this VM's `/root/.kandev/config.yaml`.
