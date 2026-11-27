---
id: "01-phone-availability"
title: "Keep the first-run dialog off phones"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-FIRST-RUN-DIALOG-001
acceptance_criteria:
  - AC-UI-FIRST-RUN-DIALOG-001.1
  - AC-UI-FIRST-RUN-DIALOG-001.2
  - AC-UI-FIRST-RUN-DIALOG-001.3
  - AC-UI-FIRST-RUN-DIALOG-001.4
  - AC-UI-FIRST-RUN-DIALOG-001.5
system_design:
  - ../../specs/ui/system-design/first-run-dialog-availability.md
---

# Task 01: Keep the first-run dialog off phones

## Summary

Hide the entire first-run dialog below the canonical 768 CSS-pixel phone breakpoint. Preserve the unfinished tour for a later larger-screen visit and leave the normal phone page usable.

## In scope

- Gate dialog presentation in `PageClient` with the existing responsive breakpoint hook.
- Preserve the current completion marker and agent-profile save behavior on larger viewports.
- Replace both mobile browser tests that currently expect the tour to open.
- Update the public first-run guide to explain phone availability.

## Out of scope

- Executor step content, other tour-step content, a new completion backend, or contextual feature guides.

## Acceptance

1. No first-run dialog appears below 768 CSS pixels, including initial render and resize from a larger viewport.
2. Phone visits and desktop-to-phone resizes leave the completion marker and dirty profile state untouched. An unfinished tour opens on resize back to a larger viewport.
3. A completed tour remains hidden on larger screens. Back, Next, Skip, Get Started, and dirty profile saves continue to work there.

## ASCII UI preview

UI-02 in the [plan](plan.md#ui-02-phone-visit) shows the normal phone page with no tour overlay. UI-01 remains the desktop tour.

## Verification

Write a focused `PageClient` regression test and update the mobile browser expectations before changing presentation. Then run:

```bash
cd apps/web
pnpm exec vitest run app/page-client.test.tsx
pnpm run typecheck
pnpm exec eslint app/page-client.tsx app/page-client.test.tsx e2e/tests/office/mobile-onboarding-dialog.spec.ts e2e/tests/office/mobile-onboarding-dialog-rich.spec.ts
pnpm e2e:run --project mobile-chrome tests/office/mobile-onboarding-dialog.spec.ts tests/office/mobile-onboarding-dialog-rich.spec.ts
```

For public documentation and design records, run from the repository root:

```bash
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/app/page-client.tsx`
- `apps/web/app/page-client.test.tsx`
- `apps/web/e2e/tests/office/mobile-onboarding-dialog.spec.ts`
- `apps/web/e2e/tests/office/mobile-onboarding-dialog-rich.spec.ts`
- `docs/public/use-kandev.md`

## Dependencies

None. Read the UI requirement and design first.

## Risks

- A delayed breakpoint check can flash the dialog on a phone. The initial-render test must catch it.
- A phone visit must not call the existing completion handler. The test must inspect the marker rather than infer state from the absence of a dialog.

## Parallelism

`sequential`

## Inputs

- [First-run availability requirement](../../specs/ui/requirements/first-run-dialog-availability.md)
- [First-run availability design](../../specs/ui/system-design/first-run-dialog-availability.md)
- [Executor onboarding plan](plan.md)

## Results

Implemented the synchronous `isMobile` gate in `PageClient`. While the tour is unfinished, its component stays mounted and receives `open=false` on phones. This hides the dialog without resetting its step or dirty agent-profile edits. Reopening refreshes agent data and preserves a dirty edit only when the refreshed profile ID still matches; if a profile was replaced or removed while hidden, the fresh settings win and no save targets the stale ID. The browser-local completion marker remains unchanged. Updated both mobile Playwright cases and the first-run paragraph in the public Get Started guide.

Verification passed:

- `pnpm exec vitest run components/onboarding-dialog.test.tsx app/page-client.test.tsx` (32 tests, including dirty-profile resize, stale-profile replacement, and save-on-proceed coverage)
- `pnpm run typecheck`
- `pnpm e2e:run --no-build --project mobile-chrome tests/office/mobile-onboarding-dialog.spec.ts tests/office/mobile-onboarding-dialog-rich.spec.ts` (2 tests)
