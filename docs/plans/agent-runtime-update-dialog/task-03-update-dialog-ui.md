---
id: "03-update-dialog-ui"
title: "Move updates into an ephemeral responsive dialog"
status: done
wave: 2
depends_on: ["02-update-preview-api"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 03: Move updates into an ephemeral responsive dialog

## Acceptance

- Managed cards show only a compact, accessible update icon; versions, command,
  output, progress, and results appear only in the opened surface.
- Opening is read-only and approval is unavailable until current version,
  target version, explanation, and exact command are visible.
- The approved job streams stdout/stderr and terminal state in the current
  surface, while closing or restarting the page discards its presentation.
- Desktop uses a centered dialog; phone uses an inset bottom drawer with a
  44px trigger, fixed header/footer, one safe-area-aware scrolling body, and no
  document horizontal overflow.
- Public Agents documentation describes the preview/approval dialog and notes
  that its visible progress and results reset on page restart.

## Verification

- Targeted state/WS tests:
  `cd apps && pnpm --filter @kandev/web test -- --run lib/state/slices/settings/settings-slice.test.ts lib/ws/handlers/agents.test.ts`
- Typecheck:
  `cd apps/web && pnpm run typecheck`
- Lint:
  `cd apps && pnpm --filter @kandev/web lint`
- Public docs:
  `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs`

## Files likely touched

- `apps/web/lib/api/domains/agent-update-api.ts`
- `apps/web/lib/api/index.ts`
- `apps/web/hooks/domains/settings/use-agent-runtime-updates.ts`
- `apps/web/app/settings/agents/page.tsx`
- `apps/web/components/settings/installed-agent-card.tsx`
- `apps/web/components/settings/agent-runtime-update-control.tsx`
- `apps/web/components/settings/use-agent-update-dialog-state.ts`
- `apps/web/lib/state/slices/settings/settings-slice.test.ts`
- `docs/public/agents-and-profiles.md`

## Dependencies

Task 02 defines the read-only preview contract.

## Parallelism

Sequential.

## Inputs

- Spec approval, non-persistence, failure, and mobile scenarios
- `components/kanban/mobile-menu-sheet.tsx` mobile surface exemplar
- Existing Settings WS job merging and install/update conflict handling
- Mobile contract in `plan.md`
- `/docs-maintainer` task-oriented documentation requirements

## Output contract

Report RED/GREEN evidence where non-visual logic changes, rendered desktop and
phone evidence, changed files, accessibility/mobile/state risks, and update
this task plus `plan.md` status.
