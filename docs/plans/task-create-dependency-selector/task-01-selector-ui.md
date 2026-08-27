---
id: "01-selector-ui"
title: "Implement the selector and layout"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-dependencies-create-dialog-dependency-selector.md"
---

# Task 01: Implement the selector and layout

Implement the searchable multi-select dependency control and pair it with the
workflow selector while preserving the existing `blockedBy` state and payload.

## Acceptance

- The create dialog shows one dependency selector with the `No dependency`
  default, task icons, archived-task filtering, multiple selection, clear-all
  behavior, and an info help control beside the search field.
- The workflow and dependency controls share a desktop row and stack in the
  intended order on mobile. The dependency selector remains present when the
  workflow picker is hidden or locked.
- Component tests cover selection, clearing, task candidates, help copy, and
  layout visibility. New copy is localized in English and pseudo catalogs, and
  the existing `blocked_by` payload tests remain green.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- task-create-dialog-dependencies task-create-dialog-form-body task-create-dialog-helpers
pnpm --filter @kandev/web run typecheck
pnpm --filter @kandev/web lint
pnpm --filter @kandev/web run i18n:check
pnpm --filter @kandev/web run i18n:ratchet
```

## Files likely touched

- `apps/web/components/task-create-dialog-dependencies.tsx`
- `apps/web/components/task-create-dialog.tsx`
- `apps/web/components/task-create-dialog-form-body.tsx`
- `apps/web/components/task-create-dialog-dependencies.test.tsx`
- `apps/web/components/task-create-dialog-form-body.test.tsx`
- `apps/web/components/task-create-dialog-helpers.test.ts`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pseudo/task.json`

## Dependencies

None.

## Parallelism

`sequential`. The layout and selector share the same dialog composition and
must be verified together.

## Inputs

- Spec: `docs/specs/tasks/requirements/task-dependencies-create-dialog-dependency-selector.md`
- Plan: `docs/plans/task-create-dependency-selector/plan.md`, Frontend and
  Mobile parity sections
- Existing candidate derivation and payload behavior in
  `task-create-dialog-dependencies.tsx` and
  `task-create-dialog-helpers.ts`
- Existing selector and mobile picker patterns in `combobox.tsx`,
  `task-create-dialog-selectors.tsx`, and `task-create-dialog-pill.tsx`

## Results

- Implemented the full-width searchable multi-select selector with a `No
  dependency` trigger, dependency icon, task icons, archived-task filtering,
  clear-all entry, selected-count summaries, and an accessible tooltip beside
  the search field.
- Moved workflow and dependency controls into a responsive row. The workflow
  selector renders first on desktop, the dependency selector renders second,
  and dependencies remain visible when a single or locked workflow hides the
  workflow picker.
- Preserved the existing `blockedBy` state and `blocked_by` payload behavior.
- Added `apps/web/components/task-create-dialog-dependencies.test.tsx` and
  paired-row coverage in `task-create-dialog-form-body.test.tsx`.
- Updated English and generated pseudo-locale task copy.

Verification:

```text
pnpm --filter @kandev/web test -- task-create-dialog-dependencies task-create-dialog-form-body task-create-dialog-helpers: pass, 62 tests
pnpm --filter @kandev/web run typecheck: pass
pnpm exec eslint --max-warnings 0 components/task-create-dialog-dependencies.tsx components/task-create-dialog-dependencies.test.tsx components/task-create-dialog-form-body.tsx components/task-create-dialog-form-body.test.tsx components/task-create-dialog.tsx: pass
pnpm run i18n:check: pass; existing advisory real-locale parity warnings remain
pnpm run i18n:ratchet: pass
git diff --check: pass
```

Remaining risk: browser coverage still needs to verify the final selector
against the managed desktop and mobile runners.

## Output contract

Report the implementation summary, files changed, tests run, blockers, and
task/plan status updates in the same conversation.
