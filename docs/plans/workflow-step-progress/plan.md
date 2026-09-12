---
created: 2026-09-12
status: complete
requirements:
  - REQ-TASKS-WORKFLOW-STEP-PROGRESS-001
system_design:
  - ../../specs/tasks/system-design/workflow-step-progress.md
legacy_specs: []
---

# Implementation plan: Workflow step progress

## Overview

Replace the workflow dot with a same-size spinner during move submission and agent startup.
Place lifecycle details in the existing disclosure, with no additional closed-stepper text.
One sequential work order implements and verifies this cohesive UI outcome.

## Evidence and scope

The investigated move began immediately. Prompt dispatch occurred about 16 seconds later; reply streaming began about 32 seconds after the move.
Runtime logs confirmed `gpt-5.6-luna`. Preparation, prior-runtime shutdown, and ACP initialization accounted for the startup interval.
This package addresses visibility. It does not change startup performance or dispatch behavior.

### In scope

- [Requirements](../../specs/tasks/requirements/workflow-step-progress.md), all eleven acceptance criteria.
- Existing full, compact, preview, and touch workflow surfaces.
- Lifecycle-derived status, fixed marker geometry, accessibility, translations, and focused regression coverage.

### Out of scope

- Backend contracts, lifecycle policies, startup optimizations, new polling, and retry controls.
- Extra inline text, independent tooltips, or a new phone workflow toolbar.

## Technical approach

Follow the [system design](../../specs/tasks/system-design/workflow-step-progress.md).
Add a shared progress derivation hook beside `useWorkflowStepMove`.
Keep request ownership separate from primary-session lifecycle derivation.
Thread progress into `StepCircleIndicator`, `StepHoverContent`, compact disclosure rows, preview navigation, and the existing phone drawer.
Preserve the marker's 8px layout footprint and current-step 14px absolute decoration.
Clear an accepted move target when authoritative task state shows a direct
supersession or a terminal outcome, while retaining late-response ownership
guards. Carry existing `cancellation_pending` evidence through lifecycle
derivation so it overrides obsolete startup progress.
Use actual runtime model metadata only when available; otherwise show the known profile name.

## ASCII UI preview

### UI-01: Desktop stepper, preparation

```text
Before:  o Analysis ---- O Implement ---- o Review
After:   o Analysis ---- @ Implement ---- o Review
                         +-----------------------+
Existing hover card:     | Current step          |
                         | Preparing agent       |
                         | <agent/profile name>  |
                         | [capability icons]    |
                         +-----------------------+
```

`@` means the spinner, not a literal glyph. `O` means the current dot and ring.
The marker, label, and connectors retain their existing positions and dimensions.
Only the existing hover card grows to contain details. Spacing in this sketch is illustrative.

### UI-02: Existing hover card, move request

```text
+-----------------------+
| [-> Moving...]        |
| [Options]             |
| Moving to this step   |
| [capability icons]    |
+-----------------------+
```

Keep the existing button/option eligibility and pending labels.
When the step becomes current, its existing controls change as today; the status block remains.
After startup, restore the dot and show `Agent running` only inside the card.
For failure, show `Agent failed` there and stop the spinner. Existing move errors remain in their current surface.

### UI-03: Compact and phone disclosure

```text
Tablet closed:  @ Implement  3/6  v
Tablet open:    [existing workflow Drawer]
                 @ Implement       Current step
                   Starting agent
                   <agent/profile name>

Phone closed:  [existing task chrome, unchanged]
Phone open:    [Task actions > Move to]
                 Implement         Current
                 Starting agent
                 <agent/profile name>
                 Review            [existing action]
```

Details appear only after opening the existing surface. The phone does not gain a stepper.
The drawer owns scrolling and retains its safe-area clearance and touch hit areas.
These structures are required by AC .2-.4 and .10; example names and text wrapping are illustrative.

## Tests

| Criteria | Evidence |
| --- | --- |
| .1, .5-.9 | New `use-workflow-step-progress.test.ts`: lifecycle table, request-settlement transition through SCHEDULING/STARTING/RUNNING, current primary ownership, unloaded sessions, no-auto-start, failure/cancellation, terminal precedence, reload snapshots |
| .7, .8 | `use-workflow-step-move.test.ts`: late rejection, overlapping request, task switch, close/reopen identity, direct supersession, terminal cleanup, and late-response ownership |
| .2-.4, .10, .11 | `workflow-stepper.test.tsx` and `workflow-stepper-keyboard.test.tsx`: marker and painted-size state, current-card details after move, controls preserved, reduced motion, full-layout keyboard focus, cancellation/failure disclosure |
| .1-.4, .8 | `task-preview-panel-step-indicator.test.tsx`: shared preview progress and stale presentation protection |

All numbers refer to `AC-TASKS-WORKFLOW-STEP-PROGRESS-001`. Add any new component test files to the work order's command before completion.

## E2E tests

- `workflow-step-progress.spec.ts`: desktop request delay, full-layout disclosure continuity after the destination becomes current, no-auto-start, and unchanged marker/paint/label/connector geometry. Failure and cancellation derivation are covered by the focused unit/component suites.
- `task-topbar-workflow-stepper.spec.ts`: existing full and compact behavior plus tablet drawer details and unchanged collapse behavior.
- `preview-workflow-step-navigation.spec.ts`: preview marker and existing disclosure continuity.
- `mobile-workflow-step-progress.spec.ts`: Pixel 5 Task actions > Move to, current status, no new top-bar text, 44px controls, viewport containment, and no horizontal overflow.

Use the default desktop and configured mobile-chrome projects. Tablet coverage uses the existing tablet fixture.
Use controlled gates and causal waits for lifecycle events. Do not reproduce the incident with a fixed 16-second sleep.

## Work orders

- [x] [Task 01: Show workflow step progress](task-01-show-progress.md)

## Verification results

Design validation on 2026-09-12:

- `python3 scripts/list-docs.py validate`: passed, 264 decisions and 820 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/workflow-step-progress`: passed.
- `git status --short -- docs/specs docs/plans/workflow-step-progress`: confirmed the new requirement, design, and plan directory.

Implementation is complete and the package remains uncommitted. Final evidence:

- `pnpm exec vitest run hooks/domains/kanban/use-workflow-step-progress.test.ts hooks/domains/kanban/use-workflow-step-move.test.ts components/task/workflow-stepper.test.tsx components/task/workflow-stepper-keyboard.test.tsx components/task-preview-panel-step-indicator.test.tsx`: 5 files, 62 tests passed.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`: passed.
- `pnpm run build:e2e`: passed.
- Desktop Chromium E2E for task top-bar, preview navigation, and workflow progress: 8 tests passed.
- Mobile Chromium E2E for the phone Move to drawer: 1 test passed.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, `node --test scripts/validate-public-docs.test.mjs`, `node scripts/validate-public-docs.mjs`, and `git diff --check`: passed.

## Risks

- Old session state must not label a newly selected destination as running.
- SCHEDULING and STARTING arrive independently of the move response; request settlement cannot own the whole startup indication.
- The absolute current-step ring must not become an in-flow 14px spinner.
- A CREATED session can be idle; treating every CREATED session as starting produces endless spinners.
- Configured profile names are not proof of the applied runtime model.

## Documentation impact

This turn changes internal design documents only. During implementation, inspect existing public workflow navigation documentation and add the smallest useful explanation.
