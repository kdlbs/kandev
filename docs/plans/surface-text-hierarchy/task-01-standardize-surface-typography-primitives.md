---
id: "01-standardize-surface-typography-primitives"
title: "Standardize surface typography primitives"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SURFACE-TEXT-HIERARCHY-001
acceptance_criteria:
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.1
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.3
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.4
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.5
system_design:
  - ../../specs/ui/system-design/surface-text-hierarchy.md
---

# Task 01: Standardize Surface Typography Primitives

## Summary

Make title and body wrapping semantic and breakpoint-independent in the five
shared surface families. Lock the package contract before consumer cleanup.

## In scope

- Update Alert, AlertDialog, Dialog, Drawer, and Sheet title primitives with
  balanced heading wrapping and long-word containment.
- Update their description primitives with pretty prose wrapping and long-word
  containment.
- Remove `text-balance md:text-pretty` from Alert and AlertDialog descriptions.
- Add one web component contract test that renders all five primitive families,
  including a `className` override case.

## Out of scope

- Consumer markup, copy, action sizing, or dialog geometry.
- New primitive properties, exports, or typography tokens.

## Implementation acceptance

1. Every covered title renders the heading wrapping contract, and every covered
   description renders the prose wrapping contract with no responsive balanced
   body class.
2. Long unbroken values have a containment fallback, while an intentional
   consumer `className` override still wins through existing class merging.
3. Existing accessible primitives, refs, portals, and interactions remain
   unchanged.

## TDD sequence

1. Add the primitive contract test and observe it fail for missing title/body
   classes and the current inverted Alert/AlertDialog breakpoint.
2. Make the smallest shared-class changes in the five primitive files.
3. Run the focused test GREEN, then typecheck and focused lint.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- components/overlay-typography-primitives.test.tsx
pnpm --filter @kandev/web run typecheck
pnpm exec prettier --check packages/ui/src/alert.tsx packages/ui/src/alert-dialog.tsx packages/ui/src/dialog.tsx packages/ui/src/drawer.tsx packages/ui/src/sheet.tsx web/components/overlay-typography-primitives.test.tsx
cd web
pnpm exec eslint --max-warnings 0 components/overlay-typography-primitives.test.tsx
```

## Files likely touched

- `apps/packages/ui/src/alert.tsx`
- `apps/packages/ui/src/alert-dialog.tsx`
- `apps/packages/ui/src/dialog.tsx`
- `apps/packages/ui/src/drawer.tsx`
- `apps/packages/ui/src/sheet.tsx`
- `apps/web/components/overlay-typography-primitives.test.tsx` (new)

## Dependencies

None.

## Parallelism

`sequential-foundation`

## Inputs

- `docs/specs/ui/requirements/surface-text-hierarchy.md`
- `docs/specs/ui/system-design/surface-text-hierarchy.md`
- `docs/plans/surface-text-hierarchy/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Results

- RED: The focused primitive contract test initially failed on the missing
  `text-balance` title default and the inverted Alert/AlertDialog description
  classes.
- Review-remediation RED: The focused contract failed when it required a zero
  minimum width alongside emergency word wrapping for grid and flex items.
- Exact-head media RED: Managed Playwright geometry at 390x844 showed
  `wrap-break-word` allowing a long unbroken task name to render past the
  confirmation's right edge.
- GREEN: Alert, AlertDialog, Dialog, Drawer, and Sheet titles now use
  `min-w-0 text-balance wrap-anywhere`; descriptions use
  `min-w-0 text-pretty wrap-anywhere` at every breakpoint. Alert and
  AlertDialog no longer apply `text-balance md:text-pretty` to descriptions.
- GREEN: `pnpm --filter @kandev/web test --
components/overlay-typography-primitives.test.tsx` (1 file, 2 tests).
- GREEN: `pnpm --filter @kandev/web run typecheck`.
- GREEN: Focused Prettier check for the five primitives and contract test.
- GREEN: Focused ESLint check for the contract test with `--max-warnings 0`.
- The contract test covers all five families, long-value containment classes,
  the absence of responsive balanced body copy, and className wrapping
  overrides. The approved Task 05 work order owns rendered 320px/393px geometry
  checks, preserving this foundation task's component-test boundary.
