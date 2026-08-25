---
id: "03-desktop-mobile-e2e"
title: "Desktop and mobile merge E2E"
status: done
wave: 3
depends_on: ["01-backend-queue-aware-merge", "02-frontend-merge-outcomes"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/github-pr-merge-queue.md"
---

# Task 03: Desktop And Mobile Merge E2E

## Inputs

- `docs/specs/integrations/requirements/github-pr-merge-queue.md`
- `docs/plans/github-pr-merge-queue/plan.md`
- `.agents/skills/e2e/SKILL.md`
- `.agents/skills/mobile-parity/SKILL.md`

## Likely Files

- `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` or GitHub mock fixtures if response
  configuration needs a new seed helper

## Acceptance

- Desktop E2E has separate action and display scenarios. The action scenario
  proves the existing PR UI exposes `Merge PR`, returns the queued API outcome,
  shows the success notification, and suppresses the accepted action. The
  display scenario proves task hover, compact popover, and detail notice expose
  queued state, position, and estimated duration.
- Mobile E2E has separate action and display scenarios. The action scenario
  reaches the queued outcome through Review using touch, verifies a minimum
  44px action target, and checks the success notification. The display scenario
  verifies the existing status drawer and Review surface expose queue metadata
  without hover.
- Both display scenarios retain the absence-of-horizontal-document-overflow
  assertion, and the managed runner builds fresh backend/frontend artifacts
  before the focused projects pass without retries masking failures.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run tests/pr/pr-merge-queue.spec.ts && pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-pr-merge-queue.spec.ts
```

## Dependencies And Risks

- Depends on Tasks 01 and 02.
- The mock GitHub server must represent queue-required and accepted-queue
  states without relying on real GitHub credentials.
- The mobile project only discovers `mobile-*.spec.ts` files; verify the
  reported test count before recording success.

## Results

Recorded after the remediation run:

- Desktop managed E2E passed with two discovered tests in 12.2 seconds: the
  merge action and the queued-status display scenario.
- Mobile Chrome managed E2E passed with two discovered tests in 9.6 seconds:
  the Review merge action with the minimum target-height assertion and the
  drawer/Review queue display scenario. Both retained the horizontal-overflow
  assertion.
- The fresh-build desktop command was followed by the mobile `--no-build`
  command, so both projects exercised the same newly built artifacts.
- Existing queue-status screenshots remain valid for the display scenario;
  action coverage is behavioral and does not add capture assets.
