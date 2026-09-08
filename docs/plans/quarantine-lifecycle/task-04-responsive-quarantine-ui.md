---
id: "04-responsive-quarantine-ui"
title: "Responsive quarantine UI"
status: done
wave: 4
depends_on: ["03-frontend-quarantine-domain"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/storage-maintenance.md"
---

# Task 04: Responsive quarantine UI

Make retention eligibility, automatic-cleanup conditions, and both bulk actions understandable and
touch-accessible in the existing Storage page.

## Acceptance

- Every row shows a semantic localized eligibility timestamp/status and disables individual Delete
  while protected; card totals and scheduling copy accurately describe automatic cleanup.
- **Clear eligible** and **Force clear all** use distinct count-aware typed confirmations and shared
  controller actions, with correct empty/active-job disabled states.
- Desktop and phone retain the same capability; phone actions are at least 44 pixels high, paths
  wrap, dialogs stay viewport-contained, and no horizontal document scrolling is introduced.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/settings/system/storage/storage-quarantine-card.test.tsx components/settings/system/storage/storage-maintenance-settings.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/settings/system/storage/storage-quarantine-card.tsx`
- `apps/web/components/settings/system/storage/storage-quarantine-card.test.tsx`
- `apps/web/components/settings/system/storage/storage-confirmation-dialogs.tsx`
- `apps/web/components/settings/system/storage/storage-maintenance-settings.tsx`
- `apps/web/components/settings/system/storage/storage-maintenance-settings.test.tsx`

## Dependencies

Task 03.

## Parallelism

Sequential; Task 06 exercises the rendered contract.

## Inputs

- Spec: quarantine UI and mobile scenarios
- Plan: Quarantine card and Mobile design contract
- Mobile exemplar:
  `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`

## Risks

- Relative status must not obscure the absolute `delete_after` time.
- Client time comparisons need deterministic fake-time component tests.

## Output contract

Report desktop/mobile composition, eligibility and confirmation behavior, rendered checks, files
changed, exact test results, blockers/risks, and update this task plus `plan.md` status.
