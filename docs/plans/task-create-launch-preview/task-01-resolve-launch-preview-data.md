---
id: "01-resolve-launch-preview-data"
title: "Resolve launch preview data"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001
  - REQ-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.2
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.3
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-001.4
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.3
  - AC-TASKS-TASK-CREATE-LAUNCH-PREVIEW-002.4
system_design:
  - ../../specs/tasks/system-design/task-create-launch-preview.md
---

# Task 01: Resolve Launch Preview Data

## Summary

Create the shared launch-step and prompt-composition projection. Preserve prompt
data when the dialog fetches steps for the effective workflow, including when it
is the visible context workflow.

## In scope

- Add pure action-sensitive launch destination and composed-preview helpers.
- Retain `prompt` in fetched task-create step data.
- Derive one launch-preview model for the effective workflow.
- Add unit tests for routing precedence, stale data, and prompt substitution.

## Out of scope

- Rendered controls or localized copy.
- Backend routing or prompt changes.
- Browser E2E tests.

## Acceptance

- The resolver matches the backend auto-start, configured-start, and positional
  fallback order.
- Fetched steps can contribute only when their workflow ID matches the effective
  workflow. A successful empty fetch is authoritative over the snapshot.
- The effect refreshes the effective workflow even when it matches the visible
  context workflow.
- Composition replaces only the first `{{task_prompt}}` and preserves all
  server-owned placeholders.

## Verification

```bash
cd apps/web
pnpm test -- --run components/task-create-dialog-launch-preview.test.ts components/task-create-dialog-effects.test.ts components/task-create-dialog-prop-builders.test.ts
pnpm run typecheck
```

## Files likely touched

- `apps/web/components/task-create-dialog-launch-preview.ts`
- `apps/web/components/task-create-dialog-launch-preview.test.ts`
- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-effects.ts`
- `apps/web/components/task-create-dialog-effects.test.ts`
- `apps/web/components/task-create-dialog-prop-builders.ts`
- `apps/web/components/task-create-dialog-prop-builders.test.ts`

## Dependencies

None.

## Risks

- A copied routing rule can drift from `ResolveAutoStartStep`.
- Stale fetched steps can display another workflow's prompt without strict
  workflow filtering.

## Parallelism

`sequential`

## Inputs

- Requirement IDs and the launch-step projection section in the system design.
- `workflow.Service.ResolveAutoStartStep` and
  `orchestrator.Service.buildWorkflowPromptWithTrustedContext`.
- Existing task-create workflow default and fetch tests.

## Results

- Added the pure launch-step resolver and prompt-composition helper with the
  backend's auto-start, configured-start, and positional fallback order.
- Preserved `prompt` and workflow identity in fetched task-create steps, then
  derived one effective-workflow launch-preview model in dialog prop assembly.
- Added unit coverage for routing precedence, stale workflow data, and
  server-owned prompt placeholders.
- `cd apps/web && pnpm test -- --run components/task-create-dialog-launch-preview.test.ts components/task-create-dialog-effects.test.ts components/task-create-dialog-prop-builders.test.ts` passed (40 tests).
- `cd apps/web && pnpm run typecheck` passed.
- Review fixup coverage passed: plan-mode routing, authoritative empty fetch,
  and same-workflow refresh. Disabled-tooltip focus coverage passed with the
  task-create component tests.
