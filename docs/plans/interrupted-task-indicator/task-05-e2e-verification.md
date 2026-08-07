---
id: "05-e2e-verification"
title: "E2E verification and final checks"
status: done
wave: 5
depends_on: ["04-frontend-icon-rendering"]
plan: "plan.md"
spec: "../../specs/tasks/interrupted-task-indicator.md"
---

# Task 05: E2E verification and final checks

## Acceptance

- A Playwright spec seeds one task with `metadata.interrupted_at` and one
  without, and asserts the sidebar task list shows `task-state-interrupted`
  only for the marked task.
- A marked task in `COMPLETED` state shows the done icon, not the red icon.
- The full local verification suite passes: `make fmt`, `make typecheck`,
  `make test`, `make lint`.

## Verification

```bash
cd apps && pnpm --filter @kandev/web e2e -- task-interrupted-icon  # or the repo's e2e invocation for one spec
cd apps && make -C .. fmt 2>/dev/null || make fmt
make typecheck
make test
make lint
```

(Follow the exact e2e invocation documented in `apps/web/e2e/README.md` and the
root `Makefile` targets; run the new spec plus the touched suites.)

## Files likely touched

- `apps/web/e2e/tests/tasks/task-interrupted-icon.spec.ts` (follow sibling
  spec layout under `apps/web/e2e/tests/`)
- No public docs change: the feature is an icon on existing surfaces. Only
  touch docs if a task-status reference page explicitly enumerates status
  icons (check `docs/public/`; if one exists, add a one-line mention).

## Dependencies

Tasks 01–04 (the marker, DTO field, plumbing, and icon must all be live).

## Inputs

- Spec: `Scenarios` 1, 6.
- Plan: `E2E Tests`.
- Existing pattern: `apps/web/e2e/helpers/api-client.ts` `createTask` with
  `metadata`, and sibling specs under `apps/web/e2e/tests/`.

## Risks

- The e2e backend must serve the derived `interrupted` field — run against the
  e2e profile build so the backend code from Tasks 01–02 is present.
- Seeding `interrupted_at` through task metadata is the deterministic
  stand-in for a real crash; do not attempt a real backend kill in the spec.
- A strict locator may match the interrupted icon in hidden/stale sidebar
  instances — scope to the active task list container per the web AGENTS.md
  guidance.

## Output contract

Report the spec file, the e2e command and result, the full
`make fmt/typecheck/test/lint` results, then mark this task `done` and update
`plan.md` to complete.
