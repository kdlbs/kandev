---
spec: docs/specs/tasks/requirements/workflow-duplication.md
created: 2026-08-11
status: complete
---

# Implementation Plan: Workflow Duplication

## Overview

Workflow duplication will reuse the current draft and save coordinator. It will not add a backend duplication endpoint.

The frontend will load the saved source steps and build an independent client-only graph. It will insert the graph after the source. The current workflow contributor will persist it on **Save changes**.

This order preserves the no-write-before-save behavior. It also reuses the current partial-save retry contract.

## Backend

No new duplication endpoint, schema, service, or route is required. The existing workflow-step controller and frontend adapter must carry the persisted `stage_type` field through create, update, and list operations so the current save endpoints preserve semantic step configuration.

The draft-save pipeline creates only a workflow and its steps. As a result, it excludes tasks and history.

## Frontend

### Draft construction

- Add `apps/web/app/settings/workspace/workflow-duplication.ts` with pure helpers that:
  - derive the lowest available `<base> (copy N)` name from persisted workflows and route-local drafts.
  - allocate a temporary workflow ID and one temporary ID per source step with `generateUUID()`.
  - copy only editable workflow metadata: `description`, `prompt`, and `agent_profile_id`.
  - omit `workflow_template_id`, `source`, `source_path`, timestamps, and server identity.
  - clone all persisted editable step fields, including `stage_type`.
  - remap nested `step_id` values in events and remap `pull_from_step_id`.
  - clear timestamps and return an independent object graph.
- Extend `useWorkflowCreation` in `apps/web/app/settings/workspace/use-workflow-creation.ts` with `handleDuplicateWorkflow(source, sourceSteps)`. It stores the copied steps in `initialStepsByWorkflowId` and inserts the draft immediately after the source in `workflowItems`.
- Wire the handler through `useWorkflowActions`, `WorkflowList`, and `WorkspaceWorkflowsClient` in `apps/web/app/settings/workspace/workspace-workflows-client.tsx`. Existing ID remapping and workflow-order save behavior will persist the copy's final position.

### Duplicate action

- Extend `WorkflowCard` in `apps/web/components/settings/workflow-card.tsx` with an async duplication callback. On activation, fetch the authoritative saved source steps with `listWorkflowStepsAction`, then hand them to the parent draft constructor.
- Keep request state and disabled/error behavior in `apps/web/app/settings/workspace/use-workflow-duplication.ts`; keep dialog composition separate from the workflow card so the card stays within the frontend size limit.
- Disable the action for temporary workflows, dirty workflow/step drafts, pending workflow mutations, and while duplication is loading. Hide or disable it in the Improve Kandev workspace. Do not disable it solely because `workflow.source` is sync-managed.
- Surface step-load failures through the existing toast provider and create no draft on failure.
- Add a labeled **Duplicate** button with an `IconCopy`, stable `data-testid="duplicate-workflow-button"`, loading state, and save-first tooltip to `apps/web/components/settings/workflow-card-header-actions.tsx`. Place it between Export and Delete.
- Add all new copy to `apps/web/src/locales/en/workflows.json`. Do not hardcode user-facing strings.

### Mobile design contract

- **Desktop outcome:** the workflow-card footer offers Export, Duplicate, and Delete. Duplicate opens the full copied card after its source.
- **Mobile entry point:** the same visible labeled Duplicate action remains in the existing wrapping card-action row. It is not hidden behind hover or a desktop-only menu.
- **Nearest shipped exemplar:** use `WorkflowCardHeaderActions` and `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`. Reuse their single-column composition and document scroll owner.
- **Hierarchy and primary action:** the copied card is the next vertical item, while the existing safe-area-aware floating **Save changes** control remains the persistence action.
- **Presentation rationale:** duplication is a single frequent command with no choices, so a direct button is clearer than a drawer or dialog. The resulting editor is substantive content and remains inline rather than being placed in a temporary overlay.
- **Geometry:** give the new touch action at least a 44px active height on phone/coarse-pointer layouts, keep the existing document as the only page scroll owner, and add no fixed controls or new safe-area requirements.
- **Shared logic:** all viewports share naming, graph cloning, request state, and persistence. Only button size and wrapping are responsive.

### Public documentation

- Update the workflow authoring how-to in `docs/public/workflow-tips.md` with the Duplicate flow, save requirement, generated names, and the explicit exclusion of tasks and history.

## Tests

- **What:** copied names choose `(copy)`, then the lowest free numeric suffix, including when duplicating a duplicate.
  **File:** `apps/web/app/settings/workspace/workflow-duplication.test.ts`.
  **How:** table-driven pure-function tests including persisted and unsaved name collisions.
- **What:** workflow and step configuration is deeply copied, source metadata is omitted, and all internal step references target copied IDs.
  **File:** `apps/web/app/settings/workspace/workflow-duplication.test.ts`.
  **How:** construct a multi-step graph with nested transition actions and pull-from relationships. Mutate the result and prove the source is unchanged.
- **What:** the creation hook inserts the duplicate after its source and registers copied initial steps without persistence.
  **File:** `apps/web/app/settings/workspace/use-workflow-creation.test.ts`.
  **How:** hook test with mocked state setter and deterministic UUIDs.
- **What:** the action invokes duplication, exposes loading/error-safe disabled states, prevents same-render duplicate requests, and explains why an unsaved source cannot be copied.
  **File:** `apps/web/components/settings/workflow-card-header-actions.test.tsx` and, if needed for card request wiring, a focused `workflow-card` test.
  **How:** Testing Library interaction and accessibility assertions.
- **What:** the existing workflow-step transport preserves a non-custom stage type through list, create, update, and draft save.
  **Files:** `apps/web/app/actions/workspaces.test.ts`, `apps/backend/internal/workflow/handlers/step_events_test.go`, and `apps/web/e2e/tests/workflow/workflow-duplication.spec.ts`.
  **How:** assert frontend DTO and request bodies, assert the backend update event payload, and assert a persisted `review` step remains `review` after duplication.

## E2E Tests

- **Scenario:** GIVEN a configured workflow with a task, WHEN the user duplicates it, THEN a configured draft appears without a write. After Save and reload, it has remapped steps and no copied task.
  **File:** `apps/web/e2e/tests/workflow/workflow-duplication.spec.ts`.
  **What to verify:** generated name, pre-save workflow count, copied metadata and step relationships, post-save persistence, source preservation, and absence of tasks in copied steps.
- **Scenario:** GIVEN a 390px touch viewport, WHEN the user duplicates and saves a workflow, THEN the action is touch-reachable and the copied editor stays within the viewport.
  **File:** `apps/web/e2e/tests/workflow/mobile-workflow-duplication.spec.ts`.
  **What to verify:** `.tap()` activation, at least 44px Duplicate hitbox, visible duplicate card and Save action, persistence after reload, and no document horizontal overflow.
- Extend `apps/web/e2e/pages/workflow-settings-page.ts` with a duplicate-action locator/helper shared by both specs.

## Verification Results

Passed:

- `cd apps && pnpm install --frozen-lockfile` completed successfully.
- `cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-card-header-actions.test.tsx app/settings/workspace/workflow-duplication.test.ts app/settings/workspace/use-workflow-creation.test.ts app/settings/workspace/use-workflow-duplication.test.ts` passed 4 files and 20 tests.
- `cd apps/web && pnpm --filter @kandev/web test -- --run app/actions/workspaces.test.ts app/settings/workspace/use-workflow-duplication.test.ts app/settings/workspace/workflow-duplication.test.ts components/settings/workflow-card-actions.test.ts` passed 4 files and 41 tests after the review fixes.
- `cd apps/backend && go test ./internal/workflow/handlers -run 'TestHTTP(UpdateStepPublishesUpdatedEvent|CreateStepPublishesCreatedEvent)' -count=1` passed.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps && pnpm --filter @kandev/web lint` passed for all changed frontend, E2E, and test files.
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet` passed. Existing real-locale parity warnings remain advisory; the pseudo catalog and new-code ratchet are clean.
- `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs` passed 59 tests and validated 41 published docs pages.
- `git diff --check` passed.
- `cd apps/web && pnpm e2e:run tests/workflow/workflow-duplication.spec.ts` passed 1 desktop test.
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/workflow/mobile-workflow-duplication.spec.ts` passed 1 mobile test after the shared card finder was made timing-safe.
- The managed E2E runner cleaned `e2e/test-results`, `e2e/blob-report`, `.pr-assets`, and temporary shard logs before each run. No generated E2E artifacts remain in the worktree.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Build duplication drafts](task-01-build-duplication-drafts.md)

Wave 2:

- [x] [Task 02: Expose duplicate action](task-02-expose-duplicate-action.md)

Wave 3:

- [x] [Task 03: Prove duplication flows](task-03-prove-duplication-flows.md)

The tasks are sequential. The action depends on the draft constructor, and E2E depends on the completed UI and persistence wiring.
