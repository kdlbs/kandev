---
id: "01-stabilize-refresh"
title: "Stabilize GitHub status refresh"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-authentication.md"
---

# Task 01: Stabilize GitHub Status Refresh

## Acceptance

- A manual same-workspace refresh preserves the loaded GitHub status until the newest request
  resolves; initial loads and workspace switches still show no stale status.
- The automation and personal identity settings remain mounted during refresh, while the existing
  refresh button is disabled, busy, and visibly spinning.
- Desktop and mobile E2E coverage proves the loaded content does not flash away while a refresh
  response is held pending; the mobile target remains at least 44px and the page has no horizontal
  overflow.

## Verification

```bash
cd apps/web && pnpm test -- hooks/domains/github/use-github-status.test.tsx components/github/github-status.test.tsx
cd apps/web && pnpm e2e:run tests/integrations/github-authentication.spec.ts -- --grep "keeps loaded settings visible during refresh"
cd apps/web && pnpm e2e:run --no-build tests/integrations/mobile-github-auth-settings.spec.ts -- --project=mobile-chrome --grep "keeps loaded settings visible during refresh"
```

## Files Likely Touched

- `apps/web/hooks/domains/github/use-github-status.ts`
- `apps/web/hooks/domains/github/use-github-status.test.tsx`
- `apps/web/components/github/github-status.tsx`
- `apps/web/components/github/github-status.test.tsx`
- `apps/web/e2e/tests/integrations/github-authentication.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-github-auth-settings.spec.ts`

## Dependencies

None.

## Parallelism

`sequential` — the hook behavior, loading presentation, and E2E assertions share one state contract.

## Inputs

- GitHub authentication spec: `UX And Mobile Contract` and the stable-refresh scenarios.
- Plan sections: `Workspace status state`, `GitHub settings status`, `Tests`, and `E2E Tests`.
- Existing same-identity stale-while-refresh pattern in
  `apps/web/hooks/domains/github/use-pr-feedback.ts`.
- Existing mobile refresh-control treatment in
  `apps/web/components/branch-refresh-button.tsx`.

## Risks

- Do not weaken request-version ordering for overlapping refreshes.
- Do not carry status across workspace IDs.
- Do not let connection replacement or disconnect keep stale content after the response settles.

## Output Contract

Report the root cause, files changed, RED and GREEN test evidence, exact commands and results,
remaining blockers or risks, and update this task to `done` plus the linked plan checkbox after all
targeted checks pass.

## Verification Results

- RED: the new hook and component regressions failed before the production change because refresh
  cleared the cached status and the personal identity returned `null` while loading.
- GREEN: `cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/github/use-github-status.test.tsx components/github/github-status.test.tsx` — 2 files, 12 tests passed.
- Typecheck: `cd apps/web && pnpm run typecheck` — passed.
- Lint: targeted ESLint over the six changed TypeScript/TSX files — passed with no warnings.
- E2E desktop: production-build managed runner — 1 Chromium test passed.
- E2E mobile: production-build managed runner with `mobile-chrome` — 1 test passed.
