---
id: "04-normalize-structured-task-confirmations"
title: "Normalize structured task confirmations"
status: completed
wave: 2
depends_on:
  - "01-standardize-surface-typography-primitives"
plan: "plan.md"
requirements:
  - REQ-UI-SURFACE-TEXT-HIERARCHY-001
acceptance_criteria:
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.2
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.3
  - AC-UI-SURFACE-TEXT-HIERARCHY-001.5
system_design:
  - ../../specs/ui/system-design/surface-text-hierarchy.md
---

# Task 04: Normalize Structured Task Confirmations

## Summary

Normalize the remaining task-domain AlertDialog descriptions with nested
content: session deletion, task detachment, and environment reset.

## In scope

- Preserve one accessible Radix description boundary in each dialog.
- Add semantic paragraph/list grouping, readable spacing, explicit left
  alignment, and min-width containment where needed.
- Preserve all localized copy, warnings, loading/disabled behavior, and
  callbacks.
- Add or extend focused component tests, including a long dynamic task/session
  value.

## Out of scope

- Task archive/delete cleanup surfaces owned by Task 03.
- Copy rewrites, action sizing/variants, shared primitives, and browser E2E.

## Implementation acceptance

1. All three dialogs render structured, left-aligned prose through one
   accessible description node and inherit pretty body wrapping.
2. Long task/session values stay contained without hiding warning content.
3. Confirmation, cancellation, disabled-state, and close behavior remain
   unchanged.

## TDD sequence

1. Extend existing tests and add the missing environment-reset regression for
   rendered description structure, alignment, and a long-value case; observe
   expected failures.
2. Apply the smallest local markup/class changes.
3. Run focused tests GREEN, then typecheck, lint, and i18n checks.

## Verification

```bash
cd apps
pnpm --filter @kandev/web test -- components/task/session-tab-menu.test.tsx components/task/task-detach-confirm-dialog.test.tsx components/task/task-reset-env-confirm-dialog.test.tsx
pnpm --filter @kandev/web run typecheck
cd web
pnpm exec eslint --max-warnings 0 components/task/session-tab-menu.tsx components/task/session-tab-menu.test.tsx components/task/task-detach-confirm-dialog.tsx components/task/task-detach-confirm-dialog.test.tsx components/task/task-reset-env-confirm-dialog.tsx components/task/task-reset-env-confirm-dialog.test.tsx
pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/task/session-tab-menu.tsx`
- `apps/web/components/task/session-tab-menu.test.tsx`
- `apps/web/components/task/task-detach-confirm-dialog.tsx`
- `apps/web/components/task/task-detach-confirm-dialog.test.tsx`
- `apps/web/components/task/task-reset-env-confirm-dialog.tsx`
- `apps/web/components/task/task-reset-env-confirm-dialog.test.tsx` (new)

## Dependencies

- Task 01 must be integrated before final verification.

## Parallelism

`parallel-wave-2`

## Inputs

- `docs/specs/ui/requirements/surface-text-hierarchy.md`
- `docs/specs/ui/system-design/surface-text-hierarchy.md`
- `docs/plans/surface-text-hierarchy/plan.md`
- `apps/web/AGENTS.md`
- `.agents/skills/tdd/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Results

- RED: Focused assertions failed against the pre-change wrapperless fragments
  and spans because the three dialogs lacked the required semantic paragraph
  structure, left alignment, containment, and single description boundary.
- GREEN: Session deletion, task detachment, and environment reset now render
  structured paragraphs inside one `min-w-0 text-left` AlertDialog description;
  compact popover and inline descriptions remain unchanged.
- Added a shared session-delete description renderer plus focused coverage for
  all three surfaces, including a 180-character dynamic task title.
- Focused verification passed with 3 test files and 11 tests. Web typecheck,
  focused ESLint with `--max-warnings 0`, `pnpm run i18n:check`, targeted
  Prettier, and `git diff --check` also passed on the integrated stack.
- Exact-head E2E integration exposed that the archive-only wide popover had
  lost its specified pretty prose wrapping. The wide size contract now applies
  `text-pretty` to its description while the default popover remains unchanged;
  the shared popover and archive confirmation suites pass 20 tests.
