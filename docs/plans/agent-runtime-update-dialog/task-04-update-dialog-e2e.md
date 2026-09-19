---
id: "04-update-dialog-e2e"
title: "Verify desktop and mobile update dialog flows"
status: done
wave: 3
depends_on: ["01-preview-catalogue", "02-update-preview-api", "03-update-dialog-ui"]
plan: "plan.md"
spec: "../../specs/agents/requirements/runtime-updates.md"
---

# Task 04: Verify desktop and mobile update dialog flows

## Acceptance

- Desktop Playwright proves the icon opens a read-only preview, approval starts
  one job, and stdout/stderr plus terminal state remain inside the dialog.
- Restart coverage proves prior progress/output/result is absent from the card
  and a newly opened dialog.
- Mobile Chrome proves the 44px icon and bottom drawer deliver the same value,
  keep long content internally reachable, clear safe areas, and do not create
  document horizontal overflow.

## Verification

- Desktop:
  `cd apps/web && pnpm e2e:run tests/settings/agent-runtime-update.spec.ts`
- Mobile:
  `cd apps/web && pnpm e2e:run tests/settings/mobile-agent-runtime-update.spec.ts -- --project=mobile-chrome`

## Files likely touched

- `apps/web/e2e/tests/settings/agent-runtime-update-helpers.ts`
- `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`
- `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`
- Runtime-update UI files only for test IDs or regressions exposed by RED

## Dependencies

Tasks 01-03.

## Parallelism

Sequential.

## Inputs

- Every revised user-visible spec scenario
- `/e2e` production-build and mobile input-modality requirements
- Mobile geometry contract in `plan.md`

## Output contract

Report exact RED and GREEN commands/results, rendered artifacts inspected,
desktop/mobile geometry evidence, uncertainties, and update this task plus
`plan.md` status.
