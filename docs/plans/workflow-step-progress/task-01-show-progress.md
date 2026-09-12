---
id: "01-show-progress"
title: "Show workflow step progress"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-PROGRESS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.1
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.2
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.3
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.4
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.5
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.6
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.7
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.8
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.9
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.10
  - AC-TASKS-WORKFLOW-STEP-PROGRESS-001.11
system_design:
  - ../../specs/tasks/system-design/workflow-step-progress.md
---

# Task 01: Show workflow step progress

## Summary

Implement the shared progress derivation and display it through the existing marker and disclosures.
Verify desktop, preview, tablet, and phone outcomes with controlled lifecycle evidence.

## In scope

All criteria in [requirements](../../specs/tasks/requirements/workflow-step-progress.md), following the paired design.
Add regression tests before production changes with TDD.
Use existing translations and add new keys in all five supported language catalogs.
Generate Traditional Chinese with `pnpm run i18n:zh-hant` from `apps/web`.

## Out of scope

Backend changes, new lifecycle fields, startup optimization, new overlays, inline status rows, and retry actions.

## Acceptance

1. Correct task/step progress survives request settlement and follows current lifecycle evidence, including all failure and stale-state cases.
2. Spinner geometry preserves the existing layout; status and agent details appear only in existing disclosures, with keyboard and touch parity.
3. Targeted tests pass for every mapped criterion; translations and public documentation reflect the final behavior.

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

See the [complete plan](plan.md#ascii-ui-preview) for shared context and test mapping.

## Verification

Run from the repository root. Install dependencies once in a fresh worktree.
The managed E2E runner rebuilds the app; do not use stale build artifacts.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-workflow-step-progress.test.ts hooks/domains/kanban/use-workflow-step-move.test.ts components/task/workflow-stepper.test.tsx components/task/workflow-stepper-keyboard.test.tsx components/task-preview-panel-step-indicator.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run build:e2e)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium tests/layout/task-topbar-workflow-stepper.spec.ts tests/kanban/preview-workflow-step-navigation.spec.ts tests/workflow/workflow-step-progress.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/workflow/mobile-workflow-step-progress.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If public docs change, also run:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/hooks/domains/kanban/use-workflow-step-progress.ts` and `.test.ts` (new).
- `apps/web/hooks/domains/kanban/use-workflow-step-move.ts` and `.test.ts`.
- `apps/web/hooks/domains/kanban/use-preview-workflow-step-move.ts`.
- `apps/web/components/task/workflow-stepper.tsx` and `.test.tsx`.
- `apps/web/components/task/workflow-step-disclosure.tsx`.
- `apps/web/components/task-preview-panel.tsx` and `task-preview-panel-step-indicator.test.tsx`.
- `apps/web/components/task/task-management-drawer.tsx`.
- `apps/web/src/locales/` task catalogs and generated Traditional Chinese values.
- The four E2E files named in the verification block; the two progress specs are new.
- Existing public workflow navigation documentation if its explanation needs an update.

## Dependencies

None. Reuse existing lifecycle projections and task/session merge ordering.

## Risks

See [plan risks](plan.md#risks). Do not work around missing state with timers or guessed agent phases.
If existing projections cannot satisfy a criterion, record the exact gap before widening the backend contract.

## Parallelism

Sequential. No delegation is authorized by this package.

## Inputs

- [Requirements](../../specs/tasks/requirements/workflow-step-progress.md).
- [System design](../../specs/tasks/system-design/workflow-step-progress.md).
- Existing stepper/preview tests and compact disclosure controls.
- `apps/web/AGENTS.md`, mobile-parity, and E2E skills.

## Results

Implemented the shared lifecycle progress presentation across the desktop
stepper, compact disclosure, preview, tablet drawer, and phone Move to drawer.
The move hook now clears a settled target when the authoritative task advances
past it or becomes terminal, while late responses remain scoped to their
request and presentation. The progress resolver carries the existing primary
session `cancellation_pending` evidence and reports an explicit stopping state
before terminal cancellation. Non-current spinners keep the 8px painted and
in-flow bounds; the current marker keeps its existing 14px decoration. The
full stepper trigger is keyboard-focusable and opens the existing HoverCard.

Focused coverage includes request settlement through SCHEDULING, STARTING,
and RUNNING; no-auto-start; failure and cancellation; direct supersession;
terminal cleanup; stale response ownership; marker, label, and connector
geometry; reduced motion; and the full hover card after the destination becomes
current. The desktop browser suite holds the move request, verifies the 8px
SVG bounds and stable layout, then completes the move and checks the current
destination disclosure. The mobile browser test verifies the existing phone
Move to drawer and touch dimensions.

Final verification passed:

- Focused Vitest: 5 files, 62 tests.
- `pnpm run typecheck`, full `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`.
- `pnpm run build:e2e`.
- Desktop Chromium E2E: 8 tests across top-bar, preview, and workflow-progress suites.
- Mobile Chromium E2E: 1 workflow-progress test.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, public-doc validators, and `git diff --check`.
