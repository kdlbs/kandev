---
id: "01-shared-sizes"
title: "Shared control sizes"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CONTROL-SIZING-001
acceptance_criteria:
  - AC-UI-CONTROL-SIZING-001.1
  - AC-UI-CONTROL-SIZING-001.2
  - AC-UI-CONTROL-SIZING-001.3
  - AC-UI-CONTROL-SIZING-001.4
  - AC-UI-CONTROL-SIZING-001.5
  - AC-UI-CONTROL-SIZING-001.9
system_design:
  - ../../specs/ui/system-design/control-sizing.md
---

# Task 01: Shared control sizes

## Summary

Introduce the shared dimension classes and align the primitive/application-wrapper boundary. Preserve existing primitive size meanings and native input attributes.

## In scope

- Add apps/packages/ui/src/control-sizing.tsx and use it in Button, Input, SelectTrigger, and the single-line InputGroup shell.
- Align settings-control helpers, Combobox triggers, and workspace triggers with the shared dimensions.
- Apply adaptive touch sizing explicitly to ordinary control wrappers. Preserve specialized primitive consumers.
- Establish reusable browser geometry assertions in apps/web/e2e/helpers/control-sizing.ts and the layout spec pair.

## Out of scope

- Feature state, callbacks, permission changes, and persistence.
- Unrelated theme, typography, or navigation redesign.

## Acceptance

- Standard and compact size composition preserves the existing public props and native Input size attribute.
- Real Start Task and Appearance controls meet desktop and touch dimensions, including attached actions.
- The shared code introduces no global selector that resizes every button.

## Regression first

Add a rendered test for the Appearance selector on the existing 900px coarse-pointer tablet fixture.
Confirm that its current desktop height fails the 44px touch minimum.
Also compare ordinary desktop controls with the Start Task action.

Use TDD for new helper logic and browser regressions.
Run the new regression before production edits and record the expected failure.
Rerun the same check after the correction.

## Verification

Run from the repository root. The first package command requires installed workspace dependencies.
Run desktop and mobile checks sequentially.
The mobile command reuses the immediately preceding unchanged production build.

```bash
pnpm --dir apps install --frozen-lockfile
pnpm --dir apps/web exec vitest run lib/ui/control-sizing.test.tsx components/settings/settings-typography.test.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web e2e:run --project chromium tests/layout/control-sizing.spec.ts
pnpm --dir apps/web e2e:run --no-build --project mobile-chrome tests/layout/mobile-control-sizing.spec.ts
```

Before completion, run ESLint on the exact changed TypeScript files from `apps/web`.
Record the expanded file list and command in Results.
Do not substitute class-string checks for browser geometry.

## Files likely touched

- `apps/packages/ui/src/control-sizing.tsx`
- `apps/packages/ui/src/button.tsx`
- `apps/packages/ui/src/input.tsx`
- `apps/packages/ui/src/select.tsx`
- `apps/packages/ui/src/input-group.tsx`
- `apps/web/components/settings/settings-control.ts`
- `apps/web/components/settings/settings-typography.tsx`
- `apps/web/components/combobox.tsx`
- `apps/web/components/workspaces/workspace-picker-content.tsx`
- `apps/web/lib/ui/control-sizing.test.tsx`
- `apps/web/e2e/helpers/control-sizing.ts`
- `apps/web/e2e/tests/layout/control-sizing.spec.ts`
- `apps/web/e2e/tests/layout/mobile-control-sizing.spec.ts`

The [inventory](sweep-inventory.md) section **Shared primitives** defines the remaining file scope.

## Dependencies

None.

## Risks

SelectTrigger data variants and InputGroup accessories need real CSS verification. Existing compact primitives must not receive a global touch expansion.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/control-sizing.md)
- [System design](../../specs/ui/system-design/control-sizing.md)
- [Sweep inventory](sweep-inventory.md)
- Existing Start Task, settings typography, and mobile geometry tests.

## Results

Completed 2026-09-10.

- Red: the shared unit regression first observed the default Button without its
  coarse-pointer classes; the managed desktop regression first measured 28px on
  the coarse tablet instead of the required 44px.
- Green: `lib/ui/control-sizing.test.tsx` and settings typography coverage passed;
  layout control sizing passed with 3 desktop and 2 mobile tests.
- The standard, compact, and icon classes are centralized in
  `apps/packages/ui/src/control-sizing.tsx`; Button `sm` retains its original
  compact semantics.
- Exact changed-file ESLint covered the full 200-file `apps/web` TypeScript set:
  0 errors and 7 non-blocking warnings. The inventory records all candidate and
  additional implementation files.
