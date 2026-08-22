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

- Desktop E2E proves queued semantics on the task indicator, task hover
  summary, compact PR popover, and PR detail notice using one seeded linked PR
  with position and estimated merge duration.
- Mobile E2E proves the existing PR status drawer and full-height Review surface
  expose the same queue state and metadata without hover.
- The mobile scenario retains touch reachability and asserts no document-level
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

- The test must seed queue membership, position, and estimate as authoritative
  TaskPR state rather than infer them from the merge-button success toast.
- Desktop and mobile projects must run separately; repeating `--project` in one
  runner invocation would silently select only the final project.

## Output contract

Report the summary, exact files changed, discovered test counts, exact E2E
commands and results, failure artifact paths if any, cleanup/teardown evidence,
blockers, risks, and synchronized task/plan status in this conversation.

## Results

Passed:

- `cd apps/web && pnpm e2e:run tests/pr/pr-merge-queue.spec.ts` built the backend, Vite production bundle, and plugin. The first assertion run exposed a strict locator ambiguity in the shared tooltip portal; the test now selects the first matching summary, and the final `pnpm e2e:run --no-build tests/pr/pr-merge-queue.spec.ts` passed 1 test in 6.2 seconds.
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/pr/mobile-pr-merge-queue.spec.ts` passed 1 test in 6.4 seconds. The scenario verified the touch drawer, full-height Review surface, queue metadata, and no document-level horizontal overflow.
- `CAPTURE_PR_ASSETS=true pnpm e2e:run --no-build tests/pr/pr-merge-queue.spec.ts` and the matching mobile command each passed 1 test. The desktop and mobile PNGs were inspected and compressed with `pnpm dlx pngquant-bin@9.0.0 --quality 65-90 --ext .png --force`; the compressed assets were preserved outside the worktree for PR media publication.
- `cd apps/web && pnpm e2e:clean` removed generated E2E results, reports, PR assets, and shard logs. Tests used mock GitHub state and temporary repositories only; no external GitHub writes occurred.
