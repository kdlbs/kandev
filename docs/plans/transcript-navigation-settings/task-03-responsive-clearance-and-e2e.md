---
id: "03-responsive-clearance-and-e2e"
title: "Responsive clearance and E2E"
status: done
wave: 3
depends_on: ["02-settings-ui-and-transcript-control"]
plan: "plan.md"
spec: "../../specs/ui/requirements/transcript-navigation-settings.md"
---

# Task 03: Responsive Clearance and E2E

## Acceptance

- The settings scroll container reserves enough bottom space for the final control to scroll fully
  above the floating Save action, status bar, and safe-area inset.
- Desktop E2E proves draft-before-Save, persistence after reload, independent visibility, and the
  auto-scroll control hidden in a seeded transcript.
- Mobile E2E proves the final switch and touch-sized Save action do not overlap at 390px and the
  page has no horizontal overflow.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/settings/settings-layout-client.test.tsx
cd apps/web && pnpm e2e:run --no-build -- tests/settings/settings-manual-save.spec.ts --grep "persists transcript navigation choices"
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/settings/mobile-general-settings.spec.ts --grep "keeps the full Transcript Navigation card"
cd apps/web && pnpm e2e:run --no-build -- tests/chat/auto-scroll-toggle.spec.ts --grep "can be hidden without changing"
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- tests/chat/mobile-auto-scroll-toggle.spec.ts --grep "can be hidden without changing"
```

## Files Likely Touched

- `apps/web/components/settings/settings-layout-client.tsx`
- `apps/web/components/settings/settings-layout-client.test.tsx`
- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`
- `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts`
- `apps/web/e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts`
- Existing transcript-navigation E2E cleanup/setup where its baseline changes.

## Dependencies

Task 02.

## Parallelism

Sequential. It verifies the completed settings contract and UI and shares persistent E2E user
settings with existing specs.

## Inputs

- Spec: final-control clearance and persistence scenarios.
- Mobile contract in `plan.md`.
- Existing `mobile-general-settings.spec.ts` geometry assertions and settings fixture cleanup
  conventions.

## Risks

- E2E workers share persisted user settings; every changed field must be restored after each test.
- Production-build E2E must not reuse stale frontend assets.

## Output Contract

Report geometry measurements, desktop/mobile scenarios, rendered verification, files changed,
exact commands/results, blockers and residual risks, then mark this task and `plan.md` complete.

## Completion

- The settings scroll container now reserves `11rem` plus the safe-area and application-status-bar
  height, so the final Transcript Navigation card can clear the floating Save action.
- Focused unit verification passed: 5 settings-layout tests.
- Focused E2E verification passed for manual persistence, mobile final-card clearance, and hidden
  auto-scroll controls on both desktop and mobile.
- No residual blockers.
