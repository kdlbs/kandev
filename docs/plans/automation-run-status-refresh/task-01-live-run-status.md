---
id: "01-live-run-status"
title: "Keep visible run status live"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/automation-runs.md"
---

# Task 01: Keep visible run status live

Update the shared automation detail refresh gate so a visible Running row is
not stranded when the server summary temporarily reports zero open runs.

## Acceptance

- A visible `triggered` or `task_created` run keeps the detail rail/drawer
  refreshing until the API returns a terminal status.
- A run outside the capped history window still keeps polling through the
  server's `openRuns` count.
- When neither signal is active, the detail page makes no repeat requests.
- The desktop rail and mobile drawer use the same behavior because both consume
  the shared detail page data.

## Verification

From `apps/`:

```bash
rtk pnpm --filter @kandev/web test -- --run components/runs/automation-detail-page.test.tsx components/runs/use-live-refresh.test.ts components/runs/use-automation-activity.test.ts
```

Also run `rtk git diff --check` and the changed-file Prettier check before
commit. Fresh worktrees must first run `rtk pnpm install --frozen-lockfile`
from `apps/` when `apps/node_modules/` is absent.

## Files likely touched

- `apps/web/components/runs/automation-detail-page.tsx`
- `apps/web/components/runs/automation-detail-page.test.tsx`

## Dependencies

None.

## Parallelism

Sequential. The production refresh gate and its rendered regression test share
the same behavior and should be developed in one TDD cycle.

## Inputs

- `docs/specs/office/requirements/automation-runs.md`, especially the live-status scenarios
  and the visible-row/zero-summary regression.
- `apps/web/components/runs/use-live-refresh.ts` for interval ownership.
- `apps/web/components/runs/use-automation-activity.ts` for the capped-window
  server count.
- `apps/web/components/runs/run-status.ts` for the definition of an open row.

## Output contract

Report the root cause, files changed, the failing RED assertion, the exact
focused command and result, any blockers or risks, and synchronized task/plan
status in the same conversation. Do not add a new backend contract or polling
surface.

## Results

- RED: `rtk pnpm --filter @kandev/web test -- --run
  components/runs/automation-detail-page.test.tsx` — 1 expected regression
  failure and 22 passing tests; `run-group-completed` was absent after the
  polling interval.
- GREEN: `rtk pnpm --filter @kandev/web test -- --run
  components/runs/automation-detail-page.test.tsx components/runs/use-live-refresh.test.ts
  components/runs/use-automation-activity.test.ts` — 3 files passed, 37 tests
  passed.
- `rtk git diff --check` — passed.
- `rtk pnpm exec prettier --check components/runs/automation-detail-page.tsx
  components/runs/automation-detail-page.test.tsx` — passed.
- No backend contract or persistent state changed. No temporary files or
  diagnostic sessions were created.
