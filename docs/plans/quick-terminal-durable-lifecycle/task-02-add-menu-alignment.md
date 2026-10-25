---
id: "02-add-menu-alignment"
title: "Fix add-menu alignment"
status: draft
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-terminal.md"
---

# Task 02: Fix add-menu alignment

## Acceptance

- The Quick Chat plus-button creation menu opens toward the trailing edge of the tab strip (aligned
  to the button's start), not toward the leading edge, on pointer viewports.
- The existing menu content is unchanged: an **Agents** section with **New Agent**, a separator, and
  a **Terminals** section with **New Terminal**.
- The existing mobile/coarse-pointer bottom-sheet treatment for the dropdown remains intact.

## Verification

```bash
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm vitest run components/quick-chat)
```

## Files likely touched

- `apps/web/components/quick-chat/quick-tab-add-menu.tsx`
- An existing `apps/web/components/quick-chat/*` test that covers the add menu (extend it; add a new
  test file only if none covers this component).

## Dependencies

None.

## Parallelism

Fully independent of the backend tasks; can land in parallel with Task 01 and Task 03.

## Inputs

- Spec section: What (plus-button menu opens toward the trailing edge).
- `quick-tab-add-menu.tsx`: `DropdownMenuContent align="end" className="w-64"` is the current source
  of the leftward opening.
- `apps/web/app/globals.css` inset bottom-sheet treatment for Radix DropdownMenu below 640px.

## Output contract

Report the alignment change, the test assertion added/updated, and exact command results. Update this
task and the parent plan only after typecheck, lint, and the focused test pass.

## Results

_To be filled in during implementation._
