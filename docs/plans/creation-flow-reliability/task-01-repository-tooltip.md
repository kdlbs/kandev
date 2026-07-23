---
id: "01-repository-tooltip"
title: "Repository tooltip containment and disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/multi-branch/spec.md"
---

# Task 01: Repository tooltip containment and disclosure

## Acceptance

- Long unbroken repository paths wrap inside a viewport-safe tooltip without covering neighboring
  repository controls.
- Closing a repository picker suppresses its tooltip until the pointer leaves; a later deliberate
  hover can open it.
- Existing keyboard-focus, popover selection, and mobile repository-selection behavior remain
  usable.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-pill.test.tsx`
- `make build-web`
- `cd apps/web && pnpm e2e -- tests/task/mobile-create-task-repository-selection.spec.ts`

## Files likely touched

- `apps/web/components/task-create-dialog-pill.tsx`
- `apps/web/components/task-create-dialog-pill.test.tsx`
- `apps/web/components/task-create-dialog-workspace-repo-chips.tsx`
- `apps/web/e2e/tests/task/mobile-create-task-repository-selection.spec.ts`

## Dependencies

None.

## Inputs

- `docs/specs/tasks/multi-branch/spec.md` task-creation scenarios.
- `docs/plans/creation-flow-reliability/plan.md` frontend and mobile design sections.
- Existing `useTooltipMountGate` behavior and Radix tooltip guidance in `apps/web/AGENTS.md`.

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed files and entry
points, risk tags, exact commands/results, uncertainties, and this task status set to `done`.

## Completion

- Tooltip content wraps long unbroken paths inside a viewport-width cap.
- Post-picker suppression is armed after the close settles, so transient focus/pointer events cannot
  reveal the tooltip; a later pointer leave or keyboard blur releases it.
- Unit, mobile touch, and real desktop Radix coverage exercise the disclosure and containment paths.
