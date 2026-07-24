---
spec: docs/specs/workspaces/creation.md
created: 2026-07-23
status: implemented
---

# Implementation Plan: Creation flow reliability

## Overview

Fix the task-create repository tooltip regression independently from the Kanban workspace bootstrap
behavior, then verify both through their existing user-facing creation flows. The backend change
keeps Office onboarding separate while the frontend change preserves the current desktop and mobile
dialog composition.

Related requirement: `docs/specs/tasks/multi-branch/spec.md` task-creation scenarios.

## Backend

### Kanban workspace bootstrap

- Extend `service.CreateWorkspaceRequest` with an internal creation-mode signal used only by the
  standard HTTP and WebSocket Kanban handlers.
- When that signal is set, persist the workspace, a `Kanban` workflow from template `simple`, and
  its template-derived steps in one repository transaction. Return an error without publishing
  either resource if any part of the transaction fails.
- Set the signal in `internal/task/handlers/workspace_handlers.go`; leave
  `internal/backendapp/adapters_office.go` unchanged so Office onboarding does not opt in.
- Preserve the existing workspace response shape and publish the existing workspace/workflow events.

## Frontend

### Repository chip tooltip

- In `components/task-create-dialog-pill.tsx`, replace the fixed post-popover suppression timer with
  suppression that lasts until the pointer leaves the trigger.
- Make repository-path tooltip content wrap unbroken paths and cap its width to the viewport.
- Keep the existing dialog, searchable popover, keyboard focus, and touch behavior unchanged.

### Mobile design contract

- Desktop outcome and mobile entry point remain the existing New Task dialog and repository chips.
- Nearest shipped mobile exemplar:
  `e2e/tests/task/mobile-create-task-repository-selection.spec.ts`; reuse its same dialog/pill flow.
- Presentation remains the existing dialog and touch-usable picker popover because this fix changes
  disclosure timing and path containment, not navigation, hierarchy, scrolling, or primary action.
- Shared `Pill` state owns the behavior on all viewports. No mobile-only state or persisted fallback
  is introduced.
- The dialog remains the scroll owner; tooltip width is viewport-safe and no document horizontal
  overflow is introduced.

## Tests

- **Post-selection suppression:** update `components/task-create-dialog-pill.test.tsx` to prove a
  closed picker ignores tooltip-open requests until pointer leave, then allows a later deliberate
  hover.
- **Long path containment:** assert the repository tooltip receives wrapping and viewport-width
  classes.
- **Kanban workspace bootstrap:** add a focused backend handler/integration regression proving a
  standard workspace creation produces one `Kanban` workflow with template-derived steps.
- **Office exclusion:** add or extend an adapter/onboarding test proving the direct Office creation
  path does not opt into Kanban bootstrap.

## E2E Tests

- Add a desktop fine-pointer regression that renders the real Radix tooltip for a long unbroken
  repository path, proves post-selection suppression through a deliberate leave/re-hover, and checks
  its bounding box against the viewport.
- Extend `apps/web/e2e/tests/task/mobile-create-task-repository-selection.spec.ts` with the
  post-selection no-tooltip and viewport containment assertions.
- Add or extend the workspace settings creation flow to create a workspace through the UI, then
  verify its workflow is visible and the task-create flow is immediately usable.

## Implementation Waves

Wave 1 (parallel):

- [x] [task-01-repository-tooltip](task-01-repository-tooltip.md) — implementer, balanced tier
- [x] [task-02-kanban-workspace-bootstrap](task-02-kanban-workspace-bootstrap.md) — implementer,
  balanced tier

Wave 2:

- Integrate, run focused E2E, simplify, commit, and run delegated change-aware verification.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-pill.test.tsx`
- `cd apps/backend && go test -run 'Test.*CreateWorkspace.*Kanban|Test.*Office.*Workspace' ./internal/task/handlers ./internal/backendapp`
- `make build-backend build-web`
- `cd apps/web && pnpm e2e --project=chromium tests/task/create-task-repository-tooltip.spec.ts`
- `cd apps/web && pnpm e2e -- tests/task/mobile-create-task-repository-selection.spec.ts`
- Focused workspace-creation Playwright spec selected during implementation from the nearest existing
  settings coverage.
