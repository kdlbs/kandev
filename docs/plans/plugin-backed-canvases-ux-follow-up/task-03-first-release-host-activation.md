---
id: "03-first-release-host-activation"
title: "Open the first task release"
status: done
wave: 3
depends_on:
  - "02-guided-canvas-task-launch"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-005
  - REQ-CANVASES-AGENT-WEB-APPS-006
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-001.2
  - AC-CANVASES-AGENT-WEB-APPS-005.4
  - AC-CANVASES-AGENT-WEB-APPS-006.1
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 03: Open the first task release

## Summary

Connect committed canvas lifecycle notifications to the active task host.
Open one desktop panel or one focused phone route without a reload.

## In scope

- Retain bounded lifecycle identity hints while HTTP remains authoritative.
- Match first-release events to the active task and workspace.
- Add and activate a deduplicated Dockview canvas panel.
- Open pending-permission host state for the first review.
- Use the focused canvas route on a phone.
- Ignore another task's event and repeated delivery.

## Out of scope

- Sidebar expansion, permission approval logic, and release persistence.

## Acceptance

- The active task opens its first canvas after publish without manual action.
- Repeated events do not create or focus duplicate panels.
- A different task's event does not change the current task surface.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/ws/handlers/canvases.test.ts lib/canvas-lifecycle.test.ts components/task/dockview-canvas-activation.test.tsx components/settings/canvas-host-route.test.tsx
cd apps/web && pnpm run typecheck && pnpm run lint
cd apps/web && pnpm e2e:run tests/canvas/plugin-canvas.spec.ts -- --retries=0
cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --retries=0
```

## Files likely touched

- `apps/web/lib/ws/handlers/canvases.ts`
- `apps/web/lib/canvas-lifecycle.ts`
- `apps/web/components/task/dockview-desktop-layout.tsx`
- `apps/web/components/task/dockview-canvas-activation.ts`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`

## Dependencies

- Task 02 provides the user flow that creates and opens the authoring task.

## Risks

- HTTP refresh can complete after a newer lifecycle event.
- Phone navigation can interrupt an unrelated user action.

## Parallelism

`sequential`

## Inputs

- Agent authoring lifecycle and desktop information architecture sections.
- `CANVAS-UX-04` in the investigation.

## Results

Implemented bounded lifecycle identity hints and task-scoped activation. A
matching first-release event refetches authoritative canvas metadata, opens a
deduplicated `canvas:<id>` Dockview panel on desktop, and routes to the focused
canvas on mobile. Pending permission events use the same path and unrelated or
repeated events are ignored.

Verification passed: lifecycle and Dockview activation tests, desktop and
phone Playwright flows with retries disabled, web typecheck, lint, and i18n
checks.
