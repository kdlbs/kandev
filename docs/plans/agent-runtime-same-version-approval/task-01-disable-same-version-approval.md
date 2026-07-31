---
id: "01-disable-same-version-approval"
title: "Disable same-version approval"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/runtime-updates.md"
---

# Task 01: Disable same-version approval

## Acceptance

- A resolved preview with identical non-empty current and target versions shows
  the version once with **Up to date**, omits the arrow transition, keeps
  **Approve update** disabled, and sends no update POST.
- Differing-version approval and failed-job retry remain enabled.
- Desktop and mobile use the same predicate, and the public Agents guide
  explains the unavailable approval state.

## TDD sequence

1. Add the equal-version desktop and mobile Playwright scenarios and run each
   focused test to confirm it fails because the UI shows an arrow transition
   and the confirmation button is enabled.
2. Render one version plus **Up to date**, then tighten `canApproveUpdate` with
   the minimal target-presence and inequality checks.
3. Run both runtime-update spec files to prove the new regression and existing
   enabled paths.
4. Update and validate the public documentation.

## Verification

- Desktop E2E:
  `cd apps/web && pnpm e2e:run tests/settings/agent-runtime-update.spec.ts`
- Mobile E2E:
  `cd apps/web && pnpm e2e:run tests/settings/mobile-agent-runtime-update.spec.ts -- --project=mobile-chrome`
- Frontend typecheck:
  `cd apps/web && pnpm run typecheck`
- Public docs:
  `node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs`

## Files likely touched

- `apps/web/components/settings/agent-runtime-update-control.tsx`
- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- `docs/public/agents-and-profiles.md`

## Dependencies

None.

## Parallelism

Sequential. The E2E scenarios depend on the same shared component change and
the task is one focused behavior slice.

## Inputs

- Equal-version behavior in `docs/specs/agents/runtime-updates.md`
- Shared body presentation and footer state in
  `apps/web/components/settings/agent-runtime-update-control.tsx`
- Existing desktop and mobile runtime-update fixtures and scenarios
- Mobile parity contract in `plan.md`

## Risks

- Preserve failed-job retries when preview versions differ.
- Do not disable the card's update icon before a fresh preview has resolved.

## Output contract

Report RED/GREEN evidence, changed files, desktop/mobile results, docs
validation, any blockers or risks, and update this task plus `plan.md` status in
the same conversation.
