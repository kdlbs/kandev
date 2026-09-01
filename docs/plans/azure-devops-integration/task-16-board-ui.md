---
id: "16-board-ui"
title: "Editable desktop and mobile board"
status: done
wave: 9
depends_on: ["14-board-discovery", "15-board-mutations"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/azure-devops-integration.md"
---

# Task 16: Editable Desktop And Mobile Board

## Acceptance

- `/azure-devops` opens in Board mode, initializes project → team → board using
  the spec defaults, resets invalid dependent selections, and renders column
  counts/limits plus core card fields from one shared board view model.
- Fine-pointer desktop uses an internally scrolling multi-column board with
  optimistic drag/move and rollback; wider card editing exposes title,
  assignee, tags, split done state, and the Azure link.
- Phone composition shows one focused column with previous/next controls and a
  bottom-drawer hierarchy navigator. A full-height, safe-area-aware editor
  provides the same fields and an explicit move control without document
  horizontal overflow or a touch-drag requirement.

## Verification

- `rtk pnpm --filter @kandev/web test -- --run lib/api/domains/azure-devops-api.test.ts hooks/domains/azure-devops/use-azure-devops-board.test.tsx components/azure-devops/azure-devops-board-model.test.ts` from `apps`.
- `rtk pnpm --filter @kandev/web typecheck` from `apps`.
- `rtk pnpm --filter @kandev/web lint` from `apps`.

## Files Likely Touched

- `apps/web/lib/types/azure-devops.ts`
- `apps/web/lib/api/domains/azure-devops-api.ts`
- `apps/web/lib/api/domains/azure-devops-api.test.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-board.ts`
- `apps/web/hooks/domains/azure-devops/use-azure-devops-board.test.tsx`
- `apps/web/app/azure-devops/azure-devops-page-client.tsx`
- `apps/web/components/azure-devops/azure-devops-board.tsx`
- `apps/web/components/azure-devops/azure-devops-board-model.ts`
- `apps/web/components/azure-devops/azure-devops-board-model.test.ts`
- `apps/web/components/azure-devops/azure-devops-board-navigator.tsx`
- `apps/web/components/azure-devops/azure-devops-board-card.tsx`
- `apps/web/components/azure-devops/azure-devops-board-item-editor.tsx`

## Dependencies

Tasks 14-15.

## Parallelism

Sequential. The page, hooks, components, and API types share the same view
model and mutation state.

## Inputs

- Spec sections: What (Board mode), API Surface, Failure Modes, Scenarios.
- Required workflows: `.agents/skills/tdd/SKILL.md`,
  `.agents/skills/mobile-parity/SKILL.md`, and
  `.agents/skills/e2e/SKILL.md`.
- Mobile exemplar:
  `apps/web/components/kanban/mobile-column-tabs.tsx` for focused-column
  previous/next navigation and the hierarchy drawer.
- Desktop exemplars:
  `apps/web/components/kanban/adaptive-desktop-kanban.tsx` for contained
  horizontal scrolling and
  `apps/web/components/kanban/swimlane-kanban-content.tsx` for optimistic DnD
  rollback.

## Mobile Design Contract

- Desktop outcome: view every Azure column, drag cards between columns, and
  edit core fields without leaving `/azure-devops`.
- Mobile entry point and hierarchy: Board mode opens one focused column; the
  center navigator shows project/team/board/column context, while previous and
  next controls move one column at a time.
- Presentation: an inset bottom drawer owns temporary hierarchy selection; a
  full-height editor owns dense card editing. The board column is the only
  vertical scroll owner, and the editor uses `100dvh`, internal scrolling, and
  bottom safe-area clearance.
- Shared logic: project/team/board selection, grouping, edits, move state,
  conflict refresh, and errors stay in shared hooks/view-model code. Only
  composition and the desktop drag affordance differ by viewport/pointer.
- Touch targets are at least 44px, every required action is visible or in an
  explicit menu/drawer, and mobile selection never overwrites a persisted
  desktop preference.

## Risks

- `azure-devops-page-client.tsx` is already near the frontend file-size limit;
  keep board state/composition in extracted modules rather than growing the
  page.
- A drag result is provisional until Azure returns the updated revision.
  Rollback must not overwrite a newer successful refresh or edit.

## Output Contract

Report responsive compositions, shared-state boundaries, files changed,
RED/GREEN commands, targeted checks, visual verification status, blockers and
risks, then set this task and its plan checkbox to done.
