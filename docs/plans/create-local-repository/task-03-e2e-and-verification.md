---
id: "03-e2e-and-verification"
title: "Local repository creation E2E verification"
status: done
wave: 3
depends_on: ["01-backend-initialization", "02-task-create-selector"]
plan: "plan.md"
spec: "../../specs/create-local-repository/spec.md"
---

# Task 03: Local Repository Creation E2E Verification

## Acceptance

- Desktop Playwright proves commitless repository initialization, automatic row/branch and direct
  local executor selection, task binding, durable filesystem/DB results, and non-destructive conflict
  handling through the real backend.
- Mobile Playwright proves the same user outcome plus Drawer containment, internal scroll ownership,
  44px actions, safe-area footer visibility, focus/dismiss behavior, and no horizontal overflow.
- Repository-wide format, typecheck, test, and lint checks pass, or exact unrelated blockers are
  recorded.

## Verification

```bash
make fmt
make typecheck
(cd apps/web && pnpm e2e:run tests/task/create-task-new-local-repository.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome \
  tests/task/mobile-create-task-new-local-repository.spec.ts)
make test
make lint
```

## Files Likely Touched

- `apps/web/e2e/tests/task/create-task-new-local-repository.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-new-local-repository.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` only if a typed read/assertion helper is missing.

## Dependencies

Tasks 01 and 02 must be integrated and their targeted tests green.

## Inputs

- All spec Scenarios and Mobile Design Contract.
- `apps/web/e2e/tests/task/create-task.spec.ts` for desktop task creation.
- `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts` for mobile picker geometry.
- `apps/web/e2e/fixtures/test-base.ts` for isolated `HOME`, real Git, and backend paths.

## Output Contract

Report scenario coverage, exact commands and results, screenshots/geometry observations, files
touched, blockers, and residual risks. Mark this file `done` and update the plan only after E2E and
required verification complete.

## Results

- Backend service and handler tests: `720` passed across both packages.
- Focused frontend tests: `82` passed across seven files.
- Desktop Playwright: `2` passed, covering creation/task binding and non-destructive conflict retry.
- Mobile Playwright: `1` passed, covering creation/task binding plus drawer layout, scrolling, focus,
  touch targets, and viewport containment.
- Repository-wide `make fmt`, `make typecheck`, `make test`, and `make lint`: passed.
- Public documentation tests and validation: passed for all `40` published pages.
- Code and security reviews: no actionable blockers remain.
