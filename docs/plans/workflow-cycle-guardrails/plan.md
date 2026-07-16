---
spec: docs/specs/workflow-cycle-guardrails/spec.md
created: 2026-07-15
status: complete
---

# Implementation Plan: Workflow Cycle Guardrails

## Overview

Add a deterministic frontend graph analyzer first, then place a shared mutation
guard in the workflow card so both draft creation and existing immediate step
mutations use the same proposed-shape decision. Build the trace UI on that
contract and finish with desktop and mobile Playwright coverage. The existing
workflow HTTP contracts and runtime engine remain unchanged.

---

## Backend

No backend changes are planned. The feature is an authoring-time guard over the
complete client-visible `WorkflowStep[]`; it intentionally does not make replay
cycles a persistence invariant or alter `POST /api/v1/workflow/steps`,
`PUT /api/v1/workflow/steps/:id`, step deletion/reorder, or workflow import.

Relevant contracts verified during planning:

- `apps/backend/internal/workflow/models/models.go` defines `on_enter`,
  `on_turn_start`, and `on_turn_complete` action semantics.
- `apps/backend/internal/workflow/engine/types.go` resolves the same three move
  action kinds and applies first-eligible-transition behavior.
- `apps/backend/internal/orchestrator/event_handlers_workflow.go` confirms that
  `on_turn_start` is user-message driven and an entered `auto_start_agent` step
  starts the workflow prompt.
- `apps/backend/internal/orchestrator/task_operations.go` defines the three
  prompt-source cases that the diagnostic describes.

---

## Frontend

### Replay-cycle analyzer

Files:

- `apps/web/lib/workflows/replay-cycle-analysis.ts` (new)
- `apps/web/lib/workflows/replay-cycle-analysis.test.ts` (new)

Changes:

- Add `analyzeWorkflowReplayCycles(steps: WorkflowStep[])` and exported
  diagnostic/trace types matching the spec's Analysis Contract.
- Normalize order by `position`, resolve `move_to_next`,
  `move_to_previous`, and valid `move_to_step` targets for both kanban
  triggers, and find paths that return to each `on_enter.auto_start_agent`
  step through an `on_turn_complete` replay edge. Treat `on_turn_start` entry
  into an auto-start step as safe because the runtime skips destination
  `on_enter` while dispatching the user's message.
- Classify a hop as automatic when its source auto-starts on entry and the
  transition does not require approval. This includes `on_turn_start`
  transitions fired by an auto-started prompt. A cycle is blocking only when
  all of its hops are automatic.
- Produce stable diagnostic identities, affected step IDs, a deterministic
  highest-severity/shortest trace per auto-start step, and the prompt-source
  category.
- Ignore unresolved targets and cycles without an auto-start re-entry.

### Proposed-shape mutation guard

Files:

- `apps/web/components/settings/workflow-card-actions.ts`
- `apps/web/components/settings/workflow-card-actions.test.ts`
- `apps/web/components/settings/workflow-card.tsx`

Changes:

- Introduce a workflow-card guard state that compares analyzer diagnostics for
  the current server-backed shape and a proposed shape.
- Route existing step update, add, remove, and reorder handlers through a
  single guard before their current API call. Build proposed arrays with the
  same step update/start-step demotion, add, delete/reindex, and reorder
  semantics already used by the handlers.
- For a new draft, run the guard in `handleSaveWorkflow` before
  `createWorkflowAction` so cancellation sends neither the workflow create nor
  any follow-up step mutations.
- Hold one pending user-mediated operation until confirmation; execute it once
  on **Apply anyway** or **Create anyway**, and discard it on cancel.
- Reject fully automatic proposals without exposing an override. Do not gate
  changes whose diagnostic identities already exist in the current shape, and
  always allow changes that remove diagnostics.
- Preserve current request errors, refresh behavior, temp-step remapping, and
  workflow creation sequencing.

### Diagnostic presentation

Files:

- `apps/web/components/settings/workflow-cycle-diagnostic.tsx` (new)
- `apps/web/components/settings/workflow-pipeline-editor.tsx`
- `apps/web/components/settings/workflow-card.tsx`

Changes:

- Add a reusable diagnostic view for the inline alert and the confirmation or
  blocking dialog. Render severity, ordered trace, user-mediated hops, and the
  prompt-source explanation without rendering prompt contents.
- Place the inline alert above the pipeline's horizontal scroll area and pass
  affected step IDs into pipeline nodes for icon/text plus border treatment,
  so affected steps are not communicated by color alone.
- Use an `AlertDialog` for a pending proposal. Blocking diagnostics provide
  only a return action; warning diagnostics provide Cancel plus the context
  appropriate **Apply anyway** or **Create anyway** action.
- Keep dialog actions and the full trace keyboard accessible. At narrow widths,
  stack trace hops and actions, wrap long names, maintain touch-sized controls,
  and prevent horizontal page overflow while leaving the pipeline's contained
  horizontal scrolling intact.

No API-client or Zustand state changes are required; workflow cards already own
their loaded step arrays and mutation callbacks.

---

## Tests

- **What:** transition resolution for next, previous, and explicit targets on
  both triggers, including reordered positions and dangling targets.
  **File:** `apps/web/lib/workflows/replay-cycle-analysis.test.ts`
  **How:** table-driven pure unit tests.

- **What:** fully automatic cycles are blocking only when every source step
  auto-starts and every hop is unapproved `on_turn_complete`.
  **File:** `apps/web/lib/workflows/replay-cycle-analysis.test.ts`
  **How:** graph fixtures that vary auto-start, trigger, and approval config.

- **What:** user-mediated cycles, generic cycles without auto-start, stable
  diagnostic identities, deterministic trace selection, and all three prompt
  sources.
  **File:** `apps/web/lib/workflows/replay-cycle-analysis.test.ts`
  **How:** focused unit fixtures.

- **What:** the built-in Review `on_turn_start` feedback edge into In Progress
  does not warn or replay auto-start behavior.
  **File:** `apps/web/lib/workflows/replay-cycle-analysis.test.ts`
  **How:** regression fixture matching the built-in Kanban transition shape.

- **What:** an existing step edit that introduces a blocking cycle does not call
  `updateWorkflowStepAction`; a warning holds the call until confirmation and
  calls it exactly once; cancel does not call it.
  **File:** `apps/web/components/settings/workflow-card-actions.test.ts`
  **How:** mocked action tests around the guard/controller helper extracted from
  `useWorkflowStepActions`.

- **What:** add, remove, and reorder use proposed full shapes and existing
  diagnostics are not repeatedly confirmed.
  **File:** `apps/web/components/settings/workflow-card-actions.test.ts`
  **How:** mocked action tests with baseline/proposed diagnostic identities.

- **What:** draft creation is blocked before `createWorkflowAction`, warning
  cancellation sends no requests, and confirmation runs the existing creation
  sequence once.
  **File:** `apps/web/components/settings/workflow-card-actions.test.ts`
  **How:** extend the current new-workflow save tests.

- **What:** the inline and dialog variants render the exact trace, prompt source,
  affected-step accessibility text, and correct action labels.
  **File:** `apps/web/components/settings/workflow-cycle-diagnostic.test.tsx`
  **How:** Vitest/Testing Library component tests.

---

## E2E Tests

- **Scenario:** a persisted existing-workflow edit introduces a user-mediated
  return path to an auto-start step; no PUT is observable until **Apply
  anyway**, and the inline alert remains after persistence.
  **File:** `apps/web/e2e/tests/workflow/workflow-cycle-guardrails.spec.ts`
  **What to verify:** exact step/trigger trace, task-description prompt source,
  cancellation behavior, confirmation, and persisted events through the API
  helper.

- **Scenario:** a draft workflow contains a fully automatic cycle; saving is
  blocked and no workflow is created.
  **File:** `apps/web/e2e/tests/workflow/workflow-cycle-guardrails.spec.ts`
  **What to verify:** blocking severity, no override action, and workflow absent
  after reload/API list.

- **Scenario:** a cycle without auto-start remains allowed.
  **File:** `apps/web/e2e/tests/workflow/workflow-cycle-guardrails.spec.ts`
  **What to verify:** no diagnostic and normal persistence.

- **Scenario:** at a mobile viewport, a user-mediated draft warning exposes the
  complete trace and prompt source and can be cancelled/confirmed by touch
  without horizontal page overflow.
  **File:** `apps/web/e2e/tests/workflow/mobile-workflow-cycle-guardrails.spec.ts`
  **What to verify:** touch-reachable dialog actions, wrapped/visible trace,
  successful confirmation, and document width not exceeding viewport width.

---

## Implementation Waves

Wave 1:

- [x] [task-01-cycle-analyzer](task-01-cycle-analyzer.md) (done)

Wave 2:

- [x] [task-02-mutation-guard](task-02-mutation-guard.md) (done)

Wave 3:

- [x] [task-03-diagnostic-ui](task-03-diagnostic-ui.md) (done)

Wave 4:

- [x] [task-04-e2e-verification](task-04-e2e-verification.md) (done)

---

## Verification Commands

Format first:

```bash
make fmt
```

Focused frontend unit tests:

```bash
(cd apps && pnpm --filter @kandev/web test -- lib/workflows/replay-cycle-analysis.test.ts components/settings/workflow-card-actions.test.ts components/settings/workflow-cycle-diagnostic.test.tsx)
```

Frontend typecheck and lint:

```bash
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
```

Focused desktop and mobile E2E (rebuilds the production web/backend artifacts
through the repo E2E harness):

```bash
(cd apps/web && pnpm e2e:run --host tests/workflow/workflow-cycle-guardrails.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/workflow/mobile-workflow-cycle-guardrails.spec.ts)
```

Full repository verification:

```bash
make typecheck test lint
```

## Risks

- `move_to_next` and `move_to_previous` change targets when steps are reordered;
  every topology-changing handler must analyze its proposed array, not only
  event updates.
- Existing step controls issue remote mutations immediately and some maintain
  local debounced input. The pending-operation guard must avoid optimistic UI
  that disagrees with the server after cancel or failure.
- The backend engine supports additional generic triggers and configurations.
  The diagnostic must state its two-trigger scope and tests must prevent the UI
  from implying that every possible workflow event has been analyzed.
- New workflow persistence is a multi-request sequence. Preflighting before the
  first request is required so a cancelled warning cannot leave a partial
  workflow.

## Open Questions

None for this implementation. The first iteration deliberately guards the
kanban-era `on_turn_start` and `on_turn_complete` paths agreed in the feature
spec and leaves direct API/import enforcement for separate work.
