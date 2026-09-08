---
id: "01-remove-mobile-repository-switcher"
title: "Remove mobile repository switcher"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/mobile-task-navigation.md"
---

# Task 01: Remove Mobile Repository Switcher

## Acceptance

- A multi-repository task never renders the repository pill or repository picker in the phone task workbench.
- The existing mobile session pill remains visible and continues to own active-session and repository-context changes.
- When loaded sessions span repositories, the session rows and active-session pill identify the bound repository so same-label sessions are distinguishable; selecting a row updates the active repository context without depending on optional workflow hydration.
- Optional repository/session hydration failures leave the phone task view non-empty, free of unexpected browser errors, and without document horizontal overflow; desktop, tablet, and other repository flows remain unchanged.

## TDD Sequence

1. **RED:** update the focused mobile SPA-resilience scenario to expect no `mobile-repo-pill` and a visible `mobile-sessions-pill`; run it and confirm it fails because the repository pill still renders.
2. **GREEN:** remove the top-bar `MobileRepoPill` render and delete its now-unreferenced picker components and component test.
3. **REFACTOR:** remove dangling imports/references only, then rerun the focused browser scenario and typecheck.
4. **REVIEW RED:** add component and mobile E2E coverage for two repository-bound sessions and confirm both fail because the retained picker omits repository identity.
5. **REVIEW GREEN:** add repository slugs to the retained picker when sessions span repositories and extend the E2E seed route to persist a session repository binding.
6. **REVIEW REFACTOR:** rerun focused component, backend harness, typecheck, and production mobile E2E checks.
7. **REVIEW 2 RED:** remove the workflow-task repository snapshot from the focused component fixture and confirm repository labels disappear.
8. **REVIEW 2 GREEN:** derive repository labels from loaded session bindings and the repository store, then rerun focused component, typecheck, lint, and production mobile E2E checks.
9. **REVIEW 3 RED:** introduce a partial session event into an already-loaded task session list and confirm the list incorrectly remains authoritative.
10. **REVIEW 3 GREEN:** invalidate the loaded list only for unknown live sessions so `useTaskSessions` hydrates their full rows, then verify known-session events do not trigger reloads.
11. **REVIEW 4 RED:** fail the API hydration after a live partial upsert and confirm the non-forced error path clears all session rows.
12. **REVIEW 4 GREEN:** preserve the current store snapshot on non-forced hydration failure so live rows remain visible and later refresh triggers can recover.
13. **REVIEW 5 RED:** resolve an older in-flight list request after a live session arrives and confirm its response drops the new row without another fetch.
14. **REVIEW 5 GREEN:** preserve sessions added during a list request and queue one forced follow-up load for authoritative hydration.

## Verification

```bash
cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/layout/mobile-spa-resilience.spec.ts --grep "keeps a multi-repository task usable without a mobile repository switcher when optional hydration fails" --workers=1
cd apps/web && pnpm e2e:run tests/session/mobile-multi-repository-session-picker.spec.ts -- --project=mobile-chrome
cd apps && pnpm --filter @kandev/web test -- --run components/task/mobile/mobile-sessions-section.test.tsx
cd apps/backend && go test ./internal/office/testharness
cd apps/web && pnpm run typecheck
cd apps/web && pnpm lint
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

- `docs/specs/ui/requirements/mobile-task-navigation.md` — mobile Dockview behavior and resilience scenario.
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
- **Review remediation:** repository-aware picker tests failed first on the missing label. The retained picker now shows canonical repository slugs when loaded sessions span repositories, and selecting the secondary-repository row updates the active pill. The focused component suite passed (`8 passed`), the E2E seed-route repository test passed, and the production cross-repository mobile scenario passed (`1 passed`).
- **Review remediation 2:** removing the optional workflow-task repository snapshot reproduced the missing labels. Label derivation now uses required loaded session bindings plus the repository store; the focused component suite (`8 passed`), web typecheck, web lint, and production cross-repository mobile scenario (`1 passed`) passed.
- **Recorded commands:** `cd apps/backend && go test ./internal/office/testharness` passed; `cd apps/web && pnpm lint` passed.
- **Review remediation 3:** a partial live-session upsert now marks an already-loaded per-task list stale so the existing hook fetches authoritative repository fields; events for known sessions keep the list loaded. Focused slice/hook/picker tests (`39 passed`), typecheck, full lint, production build, and a page-live secondary-session mobile E2E (`1 passed`) passed.
- **Review remediation 4:** failed stale-list hydration now re-commits the current live store rows instead of replacing them with `[]`; the list remains available and avoids a retry loop until reconnect or foreground refresh. Focused slice/hook/picker tests (`40 passed`), typecheck, full lint, production build, and the live-seeded mobile E2E (`1 passed`) passed.
- **Review remediation 5:** an older successful response now merges live sessions introduced after its request began, then schedules one forced list reload so their complete repository data replaces the partial row. Request reconciliation was extracted to keep the hook within lint limits; focused slice/hook/picker tests (`41 passed`), typecheck, full lint, production build, and the live-seeded mobile E2E (`1 passed`) passed.
- **Remaining risks:** none known within scoped phone task workbench; repository selection elsewhere is unchanged.
