---
id: "01-workspace-data-scope"
title: "Workspace data scope and legacy upgrade"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CANVASES-WORKSPACE-PREVIEW-001
  - REQ-CANVASES-LOCAL-CREATION-001
  - REQ-CANVASES-AGENT-WEB-APPS-003
acceptance_criteria:
  - AC-CANVASES-WORKSPACE-PREVIEW-001.1
  - AC-CANVASES-WORKSPACE-PREVIEW-001.2
  - AC-CANVASES-WORKSPACE-PREVIEW-001.3
  - AC-CANVASES-WORKSPACE-PREVIEW-001.5
  - AC-CANVASES-WORKSPACE-PREVIEW-001.6
  - AC-CANVASES-WORKSPACE-PREVIEW-001.8
  - AC-CANVASES-LOCAL-CREATION-001.2
  - AC-CANVASES-AGENT-WEB-APPS-003.2
system_design:
  - ../../specs/canvases/system-design/task-canvas-workspace-preview.md
  - ../../specs/canvases/system-design/local-creation-authority.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/plugins/system-design/canvas-workspace-data-scope.md
---

# Task 01: Workspace data scope and legacy upgrade

## Summary

Persist a separate data scope for canvas instances and honor it throughout the
relative web-app protocol. New owner-authorized task canvases get exact
declared workspace grants on first publication; existing task-only canvases
can obtain them only through a conditional owner-reviewed upgrade.

## In scope

- SQLite/PostgreSQL instance mapping and null fallback; version-2 creation
  authority and first-publication transaction.
- Runtime binding, grant checks, task/workflow/write/message/event/dependency
  enforcement, and additive context projection.
- Bounded expvar counters for first publication, reviewed upgrades, and
  promotion data-scope results, without per-canvas metric labels.
- Workspace-data preview and confirmation endpoints for retained task canvases.
- Promotion behavior with an already workspace-data task canvas and tests for
  foreign workspaces, stale reviews, owner drift, and revoked grants.

## Out of scope

- Host action, dialog, translations, browser tests, and public docs.

## Acceptance

1. A new task-placed local canvas can list and operate on all authorized tasks
   in its own workspace through declared capabilities before promotion; no
   foreign workspace or undeclared capability is admitted.
2. Existing task-only instances keep their current behavior until an owner
   confirms an exact, current release review; confirmation changes grants and
   data scope atomically without changing placement or release.
3. Promotion preserves workspace data access when it already exists, while
   stale or invalid runtime bindings and reviews fail closed.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/canvas ./internal/plugins/instances ./internal/plugins ./internal/plugins/webapp ./internal/backendapp)
python3 scripts/list-docs.py validate
```

## Files likely touched

- `apps/backend/internal/plugins/instances/store.go`
- `apps/backend/internal/plugins/webapp/tokens.go`
- `apps/backend/internal/plugins/webapp_protocol.go`
- `apps/backend/internal/plugins/webapp_protocol_data.go`
- `apps/backend/internal/plugins/webapp_events.go`
- `apps/backend/internal/plugins/webapp_protocol_json.go`
- `apps/backend/internal/canvas/authoring.go`
- `apps/backend/internal/canvas/service.go`
- `apps/backend/internal/canvas/types.go`
- `apps/backend/internal/backendapp/` canvas HTTP handlers and tests
- Adjacent `*_test.go` files

## Dependencies

None.

## Risks

Every route that uses placement scope today must be classified by whether it
filters data, determines discovery, or checks grant validity. Missing a route
could make the preview inconsistent or broaden an unintended path.

## Parallelism

`sequential`

## Inputs

- [Workspace preview requirements](../../specs/canvases/requirements/task-canvas-workspace-preview.md)
- [Workspace preview design](../../specs/canvases/system-design/task-canvas-workspace-preview.md)
- [Scope decision](../../decisions/2026-09-23-task-canvas-workspace-data.md)

## Results

Passed: `go test ./internal/canvas ./internal/plugins/instances
./internal/plugins ./internal/plugins/webapp ./internal/backendapp` and
`python3 scripts/list-docs.py validate`. The canvas tests verify fixed-label
counter increments for initial publication, reviewed upgrade, and promotion.
