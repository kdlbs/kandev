---
id: "07-canvas-host-task-surfaces"
title: "Task canvas host surfaces"
status: done
wave: 7
depends_on:
  - "03-isolated-browser-runtime"
  - "04-browser-data-state"
  - "05-live-event-transport"
  - "06-canvas-lifecycle"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-005
  - REQ-CANVASES-AGENT-WEB-APPS-007
  - REQ-PLUGINS-ISOLATED-WEB-APPS-007
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-001.2
  - AC-CANVASES-AGENT-WEB-APPS-001.5
  - AC-CANVASES-AGENT-WEB-APPS-005.1
  - AC-CANVASES-AGENT-WEB-APPS-005.4
  - AC-CANVASES-AGENT-WEB-APPS-005.5
  - AC-CANVASES-AGENT-WEB-APPS-005.6
  - AC-CANVASES-AGENT-WEB-APPS-007.1
  - AC-CANVASES-AGENT-WEB-APPS-007.2
  - AC-CANVASES-AGENT-WEB-APPS-007.3
  - AC-PLUGINS-ISOLATED-WEB-APPS-007.6
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
  - ../../specs/plugins/system-design/isolated-web-app-contributions.md
---

# Task 07: Task canvas host surfaces

## Summary

Add the shared web-application host, direct route, and desktop task panel. Use
one domain model for runtime state and host actions.

## In scope

- Add canvas API types, stores, hooks, and lifecycle subscriptions.
- Add the direct canvas route and desktop task panel.
- Add one canvas picker for task and applicable workspace canvases.
- Keep status and host action slots outside the iframe.
- Show loading, ready, offline, permission, invalid, and unavailable states.
- Add a sample task-board application for desktop acceptance.
- Cover task grouping, Continue, workflow movement, reconnect, and recovery with
  seeded plugin-release fixtures.
- Add localized copy in all required catalogs.

## Out of scope

- Workspace sidebar, workspace settings composition, and phone navigation.

## Acceptance

- Task and direct entries render the same active immutable release.
- The task board uses live task data and normal task and message services.
- Host recovery controls remain usable when application content is unavailable.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/canvas components/plugins hooks/domains/canvas lib/api/domains/canvas-api
cd apps/web && pnpm run typecheck && pnpm run lint
```

## Files likely touched

- `apps/web/components/canvas/**`
- `apps/web/components/plugins/web-app-frame.tsx`
- `apps/web/components/task/dockview-*.tsx`
- `apps/web/hooks/domains/canvas/**`
- `apps/web/lib/api/domains/canvas-api.ts`
- `apps/web/lib/types/canvas.ts`
- `apps/web/lib/ws/canvas-subscriptions.ts`
- `apps/web/src/spa-routes.tsx`
- `apps/web/src/locales/**/canvases.json`

## Dependencies

- Tasks 03 through 06 freeze runtime, data, event, lifecycle, and host-state
  contracts.

## Risks

- A delayed HTTP response can overwrite a newer lifecycle event.
- A hidden iframe can keep a stale event stream alive.
- Host controls can become inaccessible behind application content.

## Parallelism

`sequential`

## Inputs

- Runtime data flow, direct route, desktop information, host state, and
  authorization sections.
- Current Dockview, direct route, and task panel patterns.

## Results

Implemented the shared canvas host, direct route, task Dockview panel, host
recovery states, lifecycle action slots, and localized task surfaces.

Verification:

- Focused Vitest: canvas host, lifecycle, workspace, and Dockview tests passed.
- `pnpm run lint`, `pnpm run typecheck`, and the i18n checks: passed.
- Desktop Playwright canvas flow: first-release permission review, task data,
  Continue, workflow movement, state recovery, SSE reconnect, and resync passed.
