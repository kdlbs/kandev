---
spec: docs/specs/ui/mobile-task-navigation.md
created: 2026-07-17
status: complete
---

# Implementation Plan: Mobile Task Navigation Refinement

## Overview

Make workflow and step one visible Kanban hierarchy, remove mobile Pipeline, and replace side sheets with vertically efficient bottom cards. Then simplify Home task interaction and prove Dockview parity, title editing, direct navigation, and active-agent identity.

## Frontend

### Kanban hierarchy and Home menu

- `apps/web/components/kanban/swimlane-container.tsx`, `apps/web/lib/kanban/view-registry.ts`, and `apps/web/components/kanban/swimlane-kanban-content.tsx`: pass workflow identity/options into mobile Kanban, make an explicit workflow choice update the existing active workflow selection, and force the effective mobile view to Kanban without mutating the saved view mode.
- `apps/web/components/kanban/mobile-column-tabs.tsx`: make the central control show workflow plus step and expose both levels in one drawer; keep counts, WIP state, swipe, and previous/next.
- `apps/web/components/kanban/mobile-workflow-picker.tsx`: remove the redundant standalone workflow row or reduce it to reusable drawer rows.
- `apps/web/components/kanban/mobile-menu-sheet.tsx` and `apps/web/components/homepage-commands.tsx`: remove Workflow and Pipeline from secondary mobile choices, exclude the Pipeline command on mobile, and replace the right sheet with an inset bottom Drawer; keep workspace, list/Kanban, repository, preview, search, health, and settings paths.

### Home task interaction

- `apps/web/components/kanban-board.tsx`: send mobile card taps through `useKanbanNavigation` directly and remove intermediate-sheet state/rendering.
- `apps/web/components/kanban/mobile-task-sheet.tsx`: delete after all references and E2E selectors are removed.
- `apps/web/components/task-create-dialog.tsx` and `apps/web/components/task-create-dialog-helpers.ts`: show the title field in edit mode even when `computeIsTaskStarted` locks prompt/repository/session fields.

### Dockview mobile surfaces

- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`: replace left Sheet with an inset bottom Drawer while preserving filters, task actions, create, quick chat, workspace switching, confirmations, and nested menus.
- `apps/web/components/task/mobile/session-mobile-layout.tsx` and `apps/web/components/task/mobile/mobile-sessions-section.tsx`: resolve the effective active session, pass its `agentName` to `MobilePillButton` through `AgentLogo`, and expose a stable test hook.

No backend or API contract changes are required.

## Tests

- `apps/web/components/task-create-dialog-helpers.test.ts`: started edit mode keeps title visible while prompt-lock computation remains true.
- `apps/web/components/task/mobile/mobile-sessions-section.test.tsx`: active-session pill resolves and renders the active profile's agent icon, including session changes and unknown-profile fallback.
- Existing card-click/navigation tests remain green; add focused unit coverage only where new derived helpers are introduced.

## E2E Tests

- `apps/web/e2e/tests/kanban/mobile-kanban.spec.ts` and `apps/web/e2e/pages/mobile-kanban-page.ts`: workflow+step hierarchy is visible, Pipeline is absent with a non-mutating fallback, Home menu is an inset bottom card, task tap navigates directly, old sheet is absent, and a started task title can be edited from More options.
- `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`: Dockview task switcher is an inset bottom card and existing Move to/action parity still works.
- `apps/web/e2e/tests/session/mobile-handoff.spec.ts`: active session pill shows its agent icon and remains correct across session selection/handoff.
- `apps/web/e2e/tests/kanban/pipeline-view.spec.ts`: desktop Pipeline remains available.

## Implementation Waves

Wave 1 (parallel):

- [x] [Task 01 — Mobile Kanban hierarchy](task-01-mobile-kanban-hierarchy.md)
- [x] [Task 03 — Dockview mobile surfaces](task-03-dockview-mobile-surfaces.md)

Wave 2:

- [x] [Task 02 — Mobile Home task interaction](task-02-mobile-home-task-interaction.md) — depends on Task 01 because both update mobile Kanban E2E/page objects.

Wave 3:

- [x] [Task 04 — Integrated mobile verification](task-04-integrated-mobile-verification.md)

## Risks

- Workflow changes remount step content; clamp/reset the active step deterministically and avoid stale Embla indices.
- Workflow selection must update the shared active workflow before task creation or multi-select actions can run against the newly visible board.
- Mobile Pipeline fallback must not persist Kanban and erase a desktop preference.
- Vaul Drawers contain portaled Radix menus/dialogs; verify focus return, nested action selection, safe areas, and internal scroll.
- Direct card navigation must not break drag or multi-select dispatch.

## Verification

```bash
NODENV_VERSION=24.12.0 make fmt
NODENV_VERSION=24.12.0 make typecheck
NODENV_VERSION=24.12.0 make test
NODENV_VERSION=24.12.0 make lint
NODENV_VERSION=24.12.0 make build-web
cd apps/web && NODENV_VERSION=24.12.0 pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/kanban/mobile-kanban.spec.ts e2e/tests/task/mobile-sidebar-task-actions.spec.ts e2e/tests/task/mobile-sidebar-views.spec.ts e2e/tests/session/mobile-handoff.spec.ts --workers=1
cd apps/web && NODENV_VERSION=24.12.0 pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/kanban/pipeline-view.spec.ts --workers=1
```

Completed with the full repository gates green, 35 focused mobile browser tests passing, and 4 desktop Pipeline regression tests passing.
