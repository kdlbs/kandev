---
id: "03-frontend-dialog-and-mobile"
title: "Dialog persistence, workflow selection, and mobile entry"
status: done
wave: 2
depends_on: ["02-backend-issue-workflow"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/improve-kandev.md"
---

# Task 03: Dialog persistence, workflow selection, and mobile entry

## Acceptance

- Selecting **Do not show this again** safely persists the browser-local
  preference and future opens go directly to task creation unless GitHub-auth
  recovery must be shown.
- **Open issue** selects `issue_workflow_id`, renders report-only copy and the
  one-step preview, while fork-blocked users remain blocked only on Bug
  fix/Feature request.
- Phone users reach the shared flow from a 44px-or-larger Utilities row in the
  mobile home menu, without stacked overlays; touched files pass frontend
  complexity/length/nested-ternary rules.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/improve-kandev-dialog-model.test.ts components/improve-kandev-dialog-helpers.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/improve-kandev-dialog.tsx components/improve-kandev-dialog-create.tsx components/improve-kandev-dialog-model.ts components/kanban/mobile-menu-sheet.tsx
```

## Files likely touched

- `apps/web/lib/api/domains/improve-kandev-api.ts`
- `apps/web/components/improve-kandev-dialog.tsx`
- `apps/web/components/improve-kandev-dialog-create.tsx`
- `apps/web/components/improve-kandev-dialog-model.ts`
- `apps/web/components/improve-kandev-dialog-model.test.ts`
- `apps/web/components/improve-kandev-dialog-helpers.test.ts`
- `apps/web/components/kanban/mobile-menu-sheet.tsx`

## Dependencies

Task 02.

## Parallelism

Sequential. This task consumes the bootstrap contract and owns the UI used by
Task 04.

## Inputs

- Spec: **What**, **Persistence guarantees**, **Failure modes**, and mobile
  scenario.
- Plan: **Frontend**, **Mobile design contract**, and **Continuation Snapshot**.
- Mobile exemplar:
  `apps/web/components/kanban/mobile-menu-sheet.tsx`.

## Completed implementation notes

- The response type, dual step loading, intro checkbox/local-storage behavior,
  issue tab/notice, workflow switching, EMU scoping, and mobile utility entry
  are already present.
- Frontend typecheck and existing description-helper tests passed before the
  final mobile-menu edit.
- Pure model tests were added before extraction, and the known ESLint
  complexity, nested-ternary, and line-length warnings were resolved without
  suppressions.

## Recorded implementation and verification

- Added `improve-kandev-dialog-model.ts` with safe local-storage persistence,
  intro-mode selection, workflow/start-step selection, and EMU fork-block
  decisions.
- Added five table-driven model tests before extraction; the initial run failed
  because the model module was absent, then passed after implementation.
- Refactored the dialog to consume the pure model and extracted the report-only
  bottom slot/contributor message decisions.
- Added the mobile utility entry and extracted its render surface so the
  touched component passes repository line/complexity/nested-ternary rules.
- `cd apps && pnpm --filter @kandev/web test -- --run components/improve-kandev-dialog-model.test.ts components/improve-kandev-dialog-helpers.test.ts` — passed (10 tests).
- `cd apps/web && pnpm run typecheck` — passed.
- `cd apps/web && pnpm exec eslint components/improve-kandev-dialog.tsx components/improve-kandev-dialog-create.tsx components/improve-kandev-dialog-model.ts components/kanban/mobile-menu-sheet.tsx` — passed with no warnings.

Task 03 is complete. Continue with Task 04 for rebuilt desktop and mobile
browser coverage.

## Risks

- `TaskCreateDialog` resets form state when its locked `workflowId` prop
  changes. Confirm report-kind switching before data entry or preserve entered
  title/description deliberately; do not silently lose user text.
- Closing the mobile drawer before opening the dialog must preserve predictable
  focus/dismiss behavior.
- Local-storage access can throw in restricted browser contexts.

## Output contract

The shared dialog persists the intro preference safely, switches between the
implementation and report-only workflows, and is reachable through the native
mobile menu without stacked overlays.

Update this task and `plan.md`, list the helper behavior proven test-first,
record exact unit/typecheck/lint results, and note any remaining rendered
behavior for Task 04.
