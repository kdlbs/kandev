---
id: "01-submit-feedback"
title: "Add clarification submit spinner"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/clarification-submit-feedback.md"
---

# Task 01: Add clarification submit spinner

## Acceptance

- The shared multi-question clarification Submit button shows the existing
  translated `Submitting...` label and an animated `Spinner` while its response
  request is pending; it remains disabled and does not show the idle check
  icon.
- Idle and failed submissions retain the current retryable behavior, and the
  shared change works in task chat and Quick Chat without changing the mobile
  composition.
- Desktop and mobile Playwright coverage proves the pending loading state.

## Verification

Run from the repository root in one sequential block:

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run --project chromium tests/chat/clarification.spec.ts -- --grep "question shortcuts stay disabled while answers are submitting"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "loading spinner"
cd ../.. && git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/clarification-overlay-header.tsx`
- `apps/web/e2e/tests/chat/clarification.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`

## Dependencies

None.

## Parallelism

Sequential. The shared component and its desktop/mobile proof must stay in one
vertical slice.

## Inputs

- `docs/specs/ui/requirements/clarification-submit-feedback.md`
- `docs/plans/clarification-submit-feedback/plan.md`
- `apps/web/hooks/domains/session/use-clarification-group.ts`
- `apps/web/components/task/chat/clarification-overlay-header.tsx`
- Existing `Spinner` usage in `apps/web/components/settings/system/bundle-customizer.tsx`

## Risks

- The design-system spinner exposes an accessible status label; tests should
  scope that status to the submit button so unrelated loading indicators cannot
  satisfy the assertion.
- The mobile test must use the configured `mobile-chrome` project and the
  existing causal response-hold pattern, not a fixed sleep.

## Results

- `cd apps && pnpm install --frozen-lockfile` passed.
- `cd apps/web && pnpm run typecheck` passed.
- `cd apps/web && pnpm run lint` passed with no errors or warnings.
- `cd apps/web && pnpm exec prettier --check components/task/chat/clarification-overlay-header.tsx e2e/tests/chat/clarification.spec.ts e2e/tests/chat/mobile-clarification.spec.ts` passed.
- `cd apps/web && pnpm e2e:run --project chromium tests/chat/clarification.spec.ts -- --grep "question shortcuts stay disabled while answers are submitting"` passed (1 test).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-clarification.spec.ts -- --grep "loading spinner"` passed (1 test).
- `cd ../.. && git diff --check` passed.
- Final files: `apps/web/components/task/chat/clarification-overlay-header.tsx`, `apps/web/e2e/tests/chat/clarification.spec.ts`, `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`, `docs/specs/ui/requirements/clarification-submit-feedback.md`, `docs/plans/clarification-submit-feedback/plan.md`, and this task file.
- The managed E2E runs rebuilt the backend and pseudo-locale Vite assets and cleaned their isolated test artifacts. No E2E blockers or failure artifacts remain.
- Fresh phone and desktop pending-state screenshots were captured with synthetic E2E data, visually inspected, compressed, and validated through `apps/web/.pr-assets/manifest.json`; the disposable capture specs were removed afterward.
- Fixup: addressed the P2 accessibility review by adding `aria-hidden="true"` to the decorative spinner and asserting that attribute in both desktop and mobile E2E coverage.
- Fixup verification passed: `cd apps/web && pnpm run typecheck`, `cd apps/web && pnpm run lint`, and the focused desktop/mobile E2E commands above.
