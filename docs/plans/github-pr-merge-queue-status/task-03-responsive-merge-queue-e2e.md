---
id: "03-responsive-merge-queue-e2e"
title: "Responsive merge-queue E2E"
status: done
wave: 3
depends_on: ["01-backend-queue-state", "02-frontend-queued-status"]
plan: "plan.md"
spec: "../../specs/github-pr-merge-queue/spec.md"
---

# Task 03: Responsive merge-queue E2E

## Acceptance

- Desktop E2E has separate action and display scenarios. The action scenario
  proves the existing PR UI exposes `Merge PR`, returns the queued API outcome,
  shows the success notification, and suppresses the accepted action. The
  display scenario proves the task indicator, task hover summary, compact PR
  popover, and PR detail notice expose queue state, position, and estimated
  merge duration.
- Mobile E2E has separate action and display scenarios. The action scenario
  reaches the queued outcome through Review using touch, verifies a minimum
  44px action target, and checks the success notification. The display scenario
  proves the existing PR status drawer and full-height Review surface expose the
  same queue state and metadata without hover.
- Both display scenarios retain touch reachability and assert no document-level
  horizontal overflow against a freshly built production bundle.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && cd web && pnpm e2e:run tests/pr/pr-merge-queue.spec.ts && pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-pr-merge-queue.spec.ts
```

## Files likely touched

- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/pr/pr-merge-queue.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-merge-queue.spec.ts`

## Dependencies

Tasks 01 and 02 must provide the seed contract and rendered queue surfaces.

## Parallelism

Sequential. These tests validate the integrated backend/frontend behavior and
must follow both implementation tasks.

## Inputs

- Spec scenarios for active queue entry, mobile reachability, and terminal or
  authoritative queue exit behavior.
- Plan **E2E Tests** and **Mobile design contract** sections.
- Existing patterns in the two merge-queue specs, `SessionPage`,
  `mockGitHubAssociateTaskPR`, and layout assertions.

## Risks

- The display tests must seed queue membership, position, and estimate as
  authoritative TaskPR state rather than infer them from the merge-button
  success toast. The action tests must seed an eligible, not-yet-queued PR so
  the merge button and accepted response remain covered independently.
- Desktop and mobile projects must run separately; repeating `--project` in one
  runner invocation would silently select only the final project.

## Output contract

Report the summary, exact files changed, discovered test counts, exact E2E
commands and results, failure artifact paths if any, cleanup/teardown evidence,
blockers, risks, and synchronized task/plan status in this conversation.

## Results

Passed:

- `cd apps/web && pnpm e2e:run tests/pr/pr-merge-queue.spec.ts` built the backend, Vite production bundle, and plugin, then passed 2 tests: the desktop merge action and the queued-status display. The display test retains the strict-tooltip locator correction.
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-pr-merge-queue.spec.ts` passed 2 tests: the mobile Review merge action with the minimum target-height assertion and the drawer/Review queue display with no document-level horizontal overflow.
- `CAPTURE_PR_ASSETS=true pnpm e2e:run --no-build tests/pr/pr-merge-queue.spec.ts` and the matching mobile command continue to capture the display tests; the desktop and mobile PNGs were inspected and compressed with `pnpm dlx pngquant-bin@9.0.0 --quality 65-90 --ext .png --force`. Action coverage is behavioral and adds no capture asset.
- `cd apps/web && pnpm e2e:clean` removed generated E2E results, reports, PR assets, and shard logs. Tests used mock GitHub state and temporary repositories only; no external GitHub writes occurred.
