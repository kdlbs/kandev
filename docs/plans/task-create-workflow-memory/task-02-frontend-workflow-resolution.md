---
id: "02-frontend-workflow-resolution"
title: "Restore the remembered workflow"
status: done
wave: 2
depends_on: ["01-backend-workflow-memory"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-create-workflow-memory.md"
---

# Task 02: Restore the Remembered Workflow

## Intent

Map the per-workspace backend preference into shared frontend state and resolve
standard Create Task workflows with last-used precedence without overriding a
manual choice or an explicitly locked flow.

## Acceptance

- Wire, store, boot/HTTP/WS, and queued-overlay paths preserve multiple
  `workflowIdsByWorkspace` entries and converge after backend publication.
- Workflow resolution uses locked workflow → current-dialog manual selection → valid
  per-workspace last-used workflow → valid unlocked context → sole visible
  workflow → none.
- Restored workflows drive step loading and workflow-agent overrides, while the
  one-visible-workflow selector suppression and locked edit/Improve Kandev flows
  remain unchanged.

## TDD sequence

1. Add mapper and overlay tests for absent, populated, merged, stale, and synced
   workflow maps; confirm RED before adding the field.
2. Add table-driven workflow resolver tests, including conflicting filter,
   manual choice, locked choice, deleted/hidden history, cross-workspace
   history, and sole-visible fallback; confirm RED.
3. Thread the normalized preference through dialog setup/computed state,
   separate unlocked context from manual/locked form selection, and update
   dependent workflow-step/agent effects.
4. Remove the component-local workflow recency sorter and run the focused
   frontend tests plus typecheck.

## Files likely touched

- `apps/web/lib/types/http-user-settings.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/ssr/user-settings.ts`
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/hooks/use-ensure-user-settings.ts`
- `apps/web/hooks/use-ensure-user-settings.test.ts`
- `apps/web/components/state-provider.tsx`
- `apps/web/components/state-provider.test.tsx`
- `apps/web/components/task-create-dialog-handlers.ts`
- `apps/web/components/task-create-dialog-handlers.test.ts`
- `apps/web/components/task-create-dialog-defaults.ts`
- `apps/web/components/task-create-dialog-defaults.test.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-state.ts`
- `apps/web/components/task-create-dialog-computed.ts`
- `apps/web/components/task-create-dialog-setup.ts`
- `apps/web/components/task-create-dialog-locked-fields.ts`
- `apps/web/components/task-create-dialog-effects.ts`
- `apps/web/components/task-create-dialog-autopick.ts`
- Focused existing tests for state, setup, locked fields, effects, and form body
- `apps/web/components/task-create-dialog-form-body.tsx`
- `apps/web/components/workflow-selector-row.tsx`

## Dependencies

- Task 01 supplies the authoritative wire field and backend merge semantics.

## Parallelism

`sequential` — shared state, dialog form state, and resolver wiring overlap
throughout this task.

## Inputs

- Spec `What`, `Failure modes`, and workflow-resolution scenarios.
- Existing `createDefaultUserSettings`, `parseTaskCreateLastUsed`,
  `mergeTaskCreateLastUsedOverlay`, `computeSingleWorkflowFallbackId`,
  `useLockedFieldSync`, and `useWorkflowAgentProfileEffect` patterns.
- Task 01's final backend JSON and boot-state shapes.

## Verification

- `cd apps/web && pnpm test -- --run lib/ssr/user-settings.test.ts hooks/use-ensure-user-settings.test.ts components/state-provider.test.tsx components/task-create-dialog-handlers.test.ts components/task-create-dialog-defaults.test.ts components/task-create-dialog-state.test.ts components/task-create-dialog-effects.test.ts components/task-create-dialog-form-body.test.tsx`
- `cd apps/web && pnpm run typecheck`

## Mobile parity

The change is shared state normalization inside the existing responsive dialog.
It does not change composition, navigation, overlay geometry, scrolling,
safe-area behavior, focus, or touch targets. The current mobile Kanban FAB flow
remains the nearest exemplar and existing mobile E2E coverage remains valid; no
new mobile-only test is required.

## Risks

- Settings can settle after open; late restoration must not replace a workflow
  the user manually chose in that open cycle.
- Unlocked context and locked workflow currently share `selectedWorkflowId`;
  separating them must not weaken locked feature/edit behavior.
- Map comparisons must be content-based so overlay cleanup is not blocked by
  new object identities.

## Output contract

Report RED evidence, changed files, focused Vitest counts, typecheck result,
mobile-parity confirmation, blockers, risks, and synchronize this task plus
`plan.md` status/results in the primary conversation.

## Results

Mapped the backend workflow history into SSR, HTTP, WebSocket, boot, and
queued-overlay state. The dialog now resolves locked, manual, remembered,
contextual, and sole-visible workflows in the approved order; workflow steps
and agent overrides consume the same effective ID. The component-local option
sorter was removed, and locked feature flows retain their explicit selection.

TDD RED evidence included seven failing resolver cases before the resolver was
implemented; overlay and settings tests then covered missing, merged, stale,
and converged maps. Final verification:

`cd apps/web && pnpm test -- --run lib/ssr/user-settings.test.ts lib/ws/handlers/users.test.ts hooks/use-ensure-user-settings.test.ts components/state-provider.test.tsx components/task-create-dialog-handlers.test.ts components/task-create-dialog-defaults.test.ts components/task-create-dialog-state.test.ts components/task-create-dialog-effects.test.ts components/task-create-dialog-form-body.test.tsx`

9 files and 158 tests passed. `cd apps/web && pnpm run typecheck` also passed.
The mobile-parity contract remains unchanged: shared resolution is used by the
existing responsive dialog and no new mobile-only composition was introduced.
