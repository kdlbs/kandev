---
id: 06-build-desktop-workflow-inspector
title: Build the desktop workflow inspector
status: done
wave: 5
depends_on:
  - 05-build-workflow-editor-view-model
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-008
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.1
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.6
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-008.9
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 06: Build the desktop workflow inspector

## Summary

Move editing from expanded workflow cards to a dedicated route with a compact
constrained pipeline and persistent Agent/Automation/Policies inspector.

## In scope

- Lightweight workflow list links, persisted workflow route, and client-only
  new-workflow route shell with post-save URL replacement.
- Compact nodes, derived connectors, selection, issue summary, and bounded
  internal pipeline scrolling.
- Agent and Policies tab migration with focused controls and read-only viewing.
- Route-level draft contributor, dirty navigation, Save changes, and destructive
  confirmations.

## Out of scope

- Action recipe editors, mobile composition, and backend persistence changes.

## Acceptance

1. Opening a workflow shows its ordered pipeline and selected inspector without
   rendering every step form or introducing freeform canvas state.
2. Agent and Policies controls retain their current values, mutations,
   validation, effective-profile explanation, and read-only restrictions.
3. Step/tab navigation retains unsaved changes; Save changes and dirty-route
   confirmation follow the settings manual-save contract.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/settings/workflow-editor app/settings/workspace
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check
git diff --check
```

## Files likely touched

- `apps/web/app/settings/workspace/[id]/workflows/[workflowId]/page.tsx`
- `apps/web/app/settings/workspace/[id]/workflows/new/page.tsx`
- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`
- `apps/web/components/settings/workflow-editor/`
- `apps/web/components/settings/workflow-card.tsx`
- `apps/web/components/settings/workflow-pipeline-editor.tsx`
- Workflow locale catalogs and route/component tests.

## Dependencies

- Task 05 supplies the shared view model and mutations.

## Risks

- Scoping the save contributor below a selected step would lose drafts during
  navigation.
- Existing new-workflow client identities need a defined path into the route.
- Pipeline overflow must remain internal rather than widen the settings page.

## Parallelism

`sequential`. It establishes the route and inspector shell for later editors.

## Inputs

- Existing workflow card draft contributor and settings save coordinator.
- Existing constrained pipeline and workflow configuration diagnostics.

## Results

Implemented the dedicated persisted and client-only workflow routes, compact
ordered pipeline, desktop Agent/Automation/Policies inspector, route-local
manual-save draft, read-only state, and first-save identity replacement.
Desktop editor E2E coverage passes.
