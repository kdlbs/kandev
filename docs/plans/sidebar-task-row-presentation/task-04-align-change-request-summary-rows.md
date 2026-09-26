---
id: "04-align-change-request-summary-rows"
title: "Align change-request summary rows"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-task-status-summary.md"
---

# Task 04: Align Change-Request Summary Rows

## Acceptance

- Review, CI, merge, and terminal status values share one horizontal start in each summary entry.
- Secondary detail starts under the status text and wraps inside the value column.
- GitHub, GitLab, and registered-provider summaries keep their current state, icon, color, copy,
  focus, hover, and viewport behavior.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run components/integrations/change-request-task-status-summary.test.tsx components/integrations/registered-change-request-task-icon.test.tsx components/github/pr-task-status-summary.test.ts components/gitlab/mr-task-status-summary.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint -- components/integrations/change-request-task-status-summary.tsx
```

## Files Likely Touched

- `apps/web/components/integrations/change-request-task-status-summary.tsx`
- `apps/web/components/integrations/change-request-task-status-summary.test.tsx`
- Existing GitHub, GitLab, and registered-provider summary tests
- `docs/specs/ui/requirements/pr-task-status-summary.md`

## Dependencies

None.

## Parallelism

`parallel-safe` with Task 01. This task owns only the shared summary layout and focused tests.

## Inputs

- The alignment requirements and scenarios in the PR task status summary spec.
- The existing `SummaryRow`, provider presentation data, and merge-queue detail output.
- Existing GitHub, GitLab, and registered-provider render coverage.

## TDD Sequence

1. Add a focused shared-component test with short and long row labels and queue detail. Record the
   expected layout-contract failure.
2. Add provider regression assertions for unchanged labels, tones, icons, and test IDs.
3. Replace per-row independent grids with one shared label, icon, and flexible value grid.
4. Place row detail in the value column under the status text.
5. Verify wrapped text, multiple entries, and absent rows.
6. Run focused tests, typecheck, and changed-file lint. Record exact results.

## Risks

- CSS `subgrid` support or test environments can differ. Prefer a direct shared parent grid if it
  provides the same alignment with simpler compatibility.
- A fixed label width can clip translated copy. Use a content-sized track.
- Flattening row wrappers can remove test IDs or accessible grouping. Preserve both.

## Output Contract

Report the RED layout assertion, final grid contract, provider regressions checked, files changed,
exact test, typecheck, and lint results, blockers, risks, and synchronized task and plan status.

## Results

RED shared-component coverage asserted the common label, icon, and value grid contract. The summary
now aligns all status values and places secondary detail beneath the value column while preserving
provider copy, tones, icons, test IDs, wrapping, and viewport behavior. GitHub, GitLab, and
registered-provider coverage passed with 31 tests across 4 files; typecheck and changed-file ESLint
passed.
