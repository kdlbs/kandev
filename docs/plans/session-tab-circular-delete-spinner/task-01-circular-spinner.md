---
id: "01-session-tab-feedback"
title: "Refine session-tab feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/session-tab-delete-feedback.md"
---

# Task 01: Refine session-tab feedback

## Intent

Replace the pending desktop agent-session tab delete indicator with the circular terminal-style
spinner and suppress routine progress/success toasts when promoting a session to primary. Preserve
error reporting, session lifecycle behavior, and mobile composition.

## Acceptance

- Confirmed X-initiated session deletion renders a disabled, busy compact circular spinner matching
  terminal-tab closing; the grid spinner and its cube children are absent.
- Successful primary-session promotion emits no progress or success toast; a failed promotion keeps
  the existing one-error-toast behavior.
- Idle X behavior, pointer suppression, accessible status semantics, and duplicate-activation guard
  remain unchanged.
- Mobile session deletion and primary-session actions remain reachable through the native Sessions
  picker without markup or interaction changes.

## Files likely touched

- `apps/web/components/task/session-tab-close-action.tsx`
- `apps/web/components/task/session-tab-close-action.test.tsx`
- `apps/web/hooks/domains/session/use-session-actions.ts`
- `apps/web/hooks/domains/session/use-session-actions.test.ts`
- `apps/web/e2e/tests/session/session-tab-management.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The component and its regression test share the same visual contract.

## Inputs

- Spec: `docs/specs/ui/requirements/session-tab-delete-feedback.md`, pending close-state and primary-promotion
  feedback requirements and scenarios.
- Plan: `plan.md`, `Root cause`, `Session-tab close action`, and `Tests`.
- Exemplar: `TerminalTabClosingSpinner` in
  `apps/web/components/task/terminal-tab.tsx`.
- Mobile precedent: `apps/web/components/task/mobile/mobile-sessions-section.tsx`; no mobile code
  change is expected.

## Verification

Bootstrap once if this worktree does not already have dependencies:

```bash
cd apps && pnpm install --frozen-lockfile
```

Run the focused component/hook regressions and frontend checks:

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/session-tab-close-action.test.tsx
cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/session/use-session-actions.test.ts
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm run i18n:ratchet
cd apps && pnpm --filter @kandev/web exec eslint components/task/session-tab-close-action.tsx components/task/session-tab-close-action.test.tsx hooks/domains/session/use-session-actions.ts hooks/domains/session/use-session-actions.test.ts
cd apps && pnpm exec prettier --check web/components/task/session-tab-close-action.tsx web/components/task/session-tab-close-action.test.tsx web/hooks/domains/session/use-session-actions.ts web/hooks/domains/session/use-session-actions.test.ts
```

Run the existing rendered lifecycle checks after the frontend production build:

```bash
cd apps/web && pnpm e2e:run tests/session/session-tab-management.spec.ts -- --grep "tab close button shows delete confirmation and removes session on confirm"
cd apps/web && pnpm e2e:run tests/session/session-tab-management.spec.ts -- --grep "primary star survives a kanban.update broadcast"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-deletion.spec.ts
```

The managed E2E runner performs the required production build and isolated cleanup. The transient
spinner shape is asserted by the component test because a real deletion can settle before a stable
Playwright frame.

## Output contract

Report the exact changed files, circular-spinner and silent-primary assertions, all focused command
results, whether existing desktop/mobile E2E coverage passed, any generated artifacts or cleanup
evidence, and synchronized task/plan status. Do not alter session deletion semantics, error toasts,
or mobile markup.

## Results

Implemented and verified:

- The close-action component now renders the terminal-style circular spinner with no grid-spinner
  cubes while deletion is pending.
- `setPrimary` now uses inline feedback: successful promotion is silent, while a failed request
  produces one error toast.
- Focused Vitest coverage passes 17/17; scoped ESLint, Prettier, typecheck, i18n checks, and
  `git diff --check` pass.
- Managed production-build E2E coverage passes for desktop deletion (1/1), desktop primary
  promotion and no-toast behavior (1/1), and mobile session-picker deletion (1/1).
