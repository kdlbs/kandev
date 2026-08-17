---
id: "03-desktop-mobile-e2e"
title: "Desktop and mobile merge E2E"
status: done
wave: 3
depends_on: ["01-backend-queue-aware-merge", "02-frontend-merge-outcomes"]
plan: "plan.md"
spec: "../../specs/github-pr-merge-queue/spec.md"
---

# Task 03: Desktop And Mobile Merge E2E

## Inputs

- `docs/specs/github-pr-merge-queue/spec.md`
- `docs/plans/github-pr-merge-queue/plan.md`
- `.agents/skills/e2e/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Likely Files

- `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` or GitHub mock fixtures if response
  configuration needs a new seed helper

## Acceptance

- Desktop E2E proves merge-queue acceptance from the existing PR UI.
- Component coverage proves immediate merge feedback and duplicate-submission
  suppression without duplicating those deterministic cases in Playwright.
- Mobile E2E reaches the same queued outcome through Review using touch and
  verifies action size and absence of horizontal document overflow.
- The managed runner builds fresh backend/frontend artifacts and both focused
  projects pass without retries masking failures.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run tests/pr/pr-merge-queue.spec.ts && pnpm e2e:run --project mobile-chrome tests/pr/mobile-pr-merge-queue.spec.ts
```

## Dependencies And Risks

- Depends on Tasks 01 and 02.
- The mock GitHub server must represent queue-required and accepted-queue
  states without relying on real GitHub credentials.
- The mobile project only discovers `mobile-*.spec.ts` files; verify the
  reported test count before recording success.

## Results

- Desktop managed E2E passed with one discovered test.
- Mobile Chrome managed E2E passed with one discovered test, including the
  touch action, minimum target height, and horizontal-overflow assertions.
- `pnpm run typecheck` passed after the final E2E helper and spec changes.
- Isolated desktop and mobile screenshots were captured under
  `/tmp/kandev-merge-queue-screenshots-20260817/`; capture-only test hooks were
  removed and both specs passed again afterward.
