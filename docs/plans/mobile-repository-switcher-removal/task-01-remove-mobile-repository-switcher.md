---
id: "01-remove-mobile-repository-switcher"
title: "Remove mobile repository switcher"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/mobile-task-navigation.md"
---

# Task 01: Remove Mobile Repository Switcher

## Acceptance

- A multi-repository task never renders the repository pill or repository picker in the phone task workbench.
- The existing mobile session pill remains visible and continues to own active-session and repository-context changes.
- Multi-repository session rows and the active-session pill identify the bound repository so same-label sessions are distinguishable; selecting a row updates the active repository context.
- Optional repository/session hydration failures leave the phone task view non-empty, free of unexpected browser errors, and without document horizontal overflow; desktop, tablet, and other repository flows remain unchanged.

## TDD Sequence

1. **RED:** update the focused mobile SPA-resilience scenario to expect no `mobile-repo-pill` and a visible `mobile-sessions-pill`; run it and confirm it fails because the repository pill still renders.
2. **GREEN:** remove the top-bar `MobileRepoPill` render and delete its now-unreferenced picker components and component test.
3. **REFACTOR:** remove dangling imports/references only, then rerun the focused browser scenario and typecheck.
4. **REVIEW RED:** add component and mobile E2E coverage for two repository-bound sessions and confirm both fail because the retained picker omits repository identity.
5. **REVIEW GREEN:** add repository slugs to the retained picker only for multi-repository tasks and extend the E2E seed route to persist a session repository binding.
6. **REVIEW REFACTOR:** rerun focused component, backend harness, typecheck, and production mobile E2E checks.

## Verification

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/layout/mobile-spa-resilience.spec.ts --grep "keeps a multi-repository task usable without a mobile repository switcher when optional hydration fails" --workers=1
cd apps/web && pnpm e2e:run tests/session/mobile-multi-repository-session-picker.spec.ts -- --project=mobile-chrome
cd apps && pnpm --filter @kandev/web test -- --run components/task/mobile/mobile-sessions-section.test.tsx
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/task/mobile/session-mobile-top-bar.tsx`
- `apps/web/components/task/mobile/mobile-repo-pill.tsx` (delete)
- `apps/web/components/task/mobile/mobile-repos-section.tsx` (delete)
- `apps/web/components/task/mobile/mobile-repos-section.test.tsx` (delete)
- `apps/web/e2e/tests/layout/mobile-spa-resilience.spec.ts`
- `apps/web/components/task/mobile/mobile-sessions-section.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.test.tsx`
- `apps/web/e2e/tests/session/mobile-multi-repository-session-picker.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/backend/internal/office/testharness/routes.go`
- `apps/backend/internal/office/testharness/routes_test.go`

## Dependencies

None.

## Parallelism

Sequential. Production removal and its browser regression own the same interaction contract.

## Inputs

- `docs/specs/ui/mobile-task-navigation.md` — mobile Dockview behavior and resilience scenario.
- `plan.md` — scoped frontend removal and mobile design contract.
- `apps/web/components/task/mobile/mobile-sessions-section.tsx` — retained session-picker behavior.
- `apps/web/e2e/tests/layout/mobile-spa-resilience.spec.ts` — existing multi-repository hydration-failure setup.

## Risks

- Do not remove repository selection from task creation, source attachment, Files, desktop, or tablet surfaces.
- Preserve all failure interception and browser-issue assertions in the existing resilience scenario.

## Output Contract

Report RED and GREEN evidence, files deleted/changed, exact command results, remaining risks, and update this task plus `plan.md` status in the same conversation.

## Results

- **RED:** baseline production build, after the retained session pill became visible, failed with `mobile-repo-pill` expected count `0`, received `1`.
- **GREEN:** removing `MobileRepoPill` from `session-mobile-top-bar.tsx` made the focused phone scenario pass.
- **REFACTOR:** deleted `mobile-repo-pill.tsx`, `mobile-repos-section.tsx`, and `mobile-repos-section.test.tsx`; no remaining source or mobile E2E interaction references exist.
- **Verification:** `cd apps/web && pnpm run typecheck` passed. Final change-aware `pnpm e2e:run --project mobile-chrome -- tests/layout/mobile-spa-resilience.spec.ts --grep "keeps a multi-repository task usable without a mobile repository switcher when optional hydration fails" --workers=1` rebuilt production assets and passed (`1 passed`).
- **Review remediation:** repository-aware picker tests failed first on the missing label. The retained picker now shows canonical repository slugs only for multi-repository tasks, and selecting the secondary-repository row updates the active pill. The focused component suite passed (`8 passed`), the E2E seed-route repository test passed, and the production cross-repository mobile scenario passed (`1 passed`).
- **Remaining risks:** none known within scoped phone task workbench; repository selection elsewhere is unchanged.
