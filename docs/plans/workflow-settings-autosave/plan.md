---
spec: docs/specs/workflow-settings-autosave/spec.md
created: 2026-07-14
status: superseded
superseded_by: docs/plans/settings-manual-save/plan.md
---

# Implementation Plan: Workflow Settings Autosave

> Superseded by [Settings Manual Save](../settings-manual-save/plan.md). This completed plan remains as the implementation record for the former autosave behavior.

## Overview

First move workflow creation into the dialog confirmation path and introduce a shared card persistence status with retry. Then make the workflow and step editors responsive without changing their API contracts. Finally update desktop and mobile E2E coverage to assert persistence, status, and reachability.

## Backend

No backend changes are required. Existing workflow and workflow-step create/update endpoints remain the persistence boundary.

## Frontend

### Creation and autosave state

- `apps/web/app/settings/workspace/use-workflow-creation.ts`: create workflows and initial steps from dialog confirmation, protect against websocket bootstrap races, and clean up partial creation failures.
- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`: integrate immediate creation and debounce persisted workflow name changes.
- `apps/web/components/settings/workflow-card-actions.ts`: expose step mutation status and retry while preserving current refresh/error behavior.
- `apps/web/components/settings/workflow-card.tsx`: combine metadata and step statuses, render a card-level autosave indicator, and remove the manual Save control.
- `apps/web/app/settings/workspace/workspace-workflows-dialogs.tsx`: keep the dialog open while creating, disable duplicate submission, and report creation progress.

### Responsive layout

- `apps/web/components/settings/settings-section.tsx`: stack section heading/actions at narrow widths.
- `apps/web/components/settings/workflow-card.tsx`: stack workflow detail fields and wrap card actions.
- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`: stack fixed-width step header controls and keep touch targets reachable.

## Tests

- `apps/web/components/settings/workflow-card-actions.test.ts`: step mutation status and retry behavior.
- `apps/web/hooks/domains/settings/use-workflow-settings.test.ts` or a focused new hook test: metadata autosave debounce/retry logic if extracted into a hook.

## E2E Tests

- `apps/web/e2e/tests/workflow/workflow-settings.spec.ts`: creation persists directly; workflow metadata and step settings persist without Save; no Save control remains.
- `apps/web/e2e/tests/workflow/mobile-workflow-settings.spec.ts`: all required actions fit a 390px viewport, the document has no horizontal overflow, and autosave persists a user-visible edit.
- `apps/web/e2e/pages/workflow-settings-page.ts`: replace manual-save helpers with autosave status helpers.

## Implementation Waves

Wave 1:
- [x] [Task 01: Autosave state](task-01-autosave-state.md) (done)

Wave 2:
- [x] [Task 02: Responsive layout](task-02-responsive-layout.md) (done)

Wave 3:
- [x] [Task 03: E2E coverage](task-03-e2e-coverage.md) (done)

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run app/settings/workspace/use-workflow-creation.test.ts components/settings/workflow-card-actions.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/workflow/workflow-settings.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome --no-build tests/workflow/mobile-workflow-settings.spec.ts
GOCACHE=/tmp/kandev-go-cache make fmt
GOCACHE=/tmp/kandev-go-cache make typecheck
GOCACHE=/tmp/kandev-go-cache make test
GOCACHE=/tmp/kandev-go-cache GOLANGCI_LINT_CACHE=/tmp/kandev-golangci-cache make lint
```

## Completion Report

- Verification: focused creation/autosave tests, web typecheck, desktop workflow E2E, mobile Chrome E2E, and the full repository format/typecheck/test/lint pipeline passed locally.
- Behavior: workflow creation is immediate; metadata and step writes use separate serialized queues whose statuses are combined on the card; failures pause and retry the exact failed operation within the affected queue.
- Responsive result: page actions, card fields, destructive actions, and the open step editor remain within a 390px viewport without document overflow.
- Blockers: no implementation blocker. PR CI later exposed a shared `runtime-latest` Playwright browser-revision mismatch before any E2E spec started.
- Residual races: cross-tab edits remain last-write-wins and are explicitly out of scope; within one card, stale completions cannot overwrite newer local metadata.
