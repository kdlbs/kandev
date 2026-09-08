---
id: "03-resolution-errors-and-retry"
title: "Remote resolution errors and retry"
status: done
wave: 2
depends_on: ["01-public-provider-resolution"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/multi-branch.md"
---

# Task 03: Remote resolution errors and retry

## Acceptance

- Branch and GitHub PR/issue loaders retain per-URL errors without losing successful sibling data.
- A committed repository row shows an accessible error and visible Retry action while preserving its URL.
- Retry clears and re-runs both resolution paths; stale callbacks cannot overwrite the newer result.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- hooks/domains/github/use-branches-by-url.test.ts hooks/domains/github/use-pr-info-by-url.test.ts components/task-create-dialog-remote-repo-chip.test.tsx components/task-create-dialog-remote-repo-chips.test.tsx`

## Files likely touched

- `apps/web/hooks/domains/github/use-branches-by-url.ts`
- `apps/web/hooks/domains/github/use-branches-by-url.test.ts`
- `apps/web/hooks/domains/github/use-pr-info-by-url.ts`
- `apps/web/hooks/domains/github/use-pr-info-by-url.test.ts`
- `apps/web/components/task-create-dialog-remote-repo-chips.tsx`
- `apps/web/components/task-create-dialog-remote-repo-chip.tsx`
- Corresponding component tests.

## Dependencies

- Task 01 supplies the public unauthenticated success path.

## Inputs

- `docs/specs/tasks/requirements/multi-branch.md` — failure and retry scenarios.
- Existing per-URL abort and sequence guards in both hooks.

## Output contract

Report the changed state contract, exact tests/results, risk tags, and uncertainties; update this task to `done` only after targeted verification passes.

## Completion evidence

- **Entry points:** per-URL branch and GitHub PR/issue loaders; remote repository chip error and retry UI.
- **Result:** frontend focused verification passed: 67 tests; post-simplification hook suite passed: 33 tests.
- **Risks/uncertainties:** stale callbacks are sequence-guarded; provider failures can still persist until the user retries or corrects access.
