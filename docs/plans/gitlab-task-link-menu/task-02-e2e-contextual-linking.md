---
id: "02-e2e-contextual-linking"
title: "GitLab contextual linking E2E"
status: in_progress
wave: 2
depends_on: ["01-frontend-link-menu"]
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 02: GitLab Contextual Linking E2E

## Acceptance

- Desktop E2E proves an unlinked task has no generic top-bar link action and
  can link an MR through right-click `Link` > `GitLab Merge Request`.
- Mobile E2E proves the visible `Task actions` ellipsis exposes the same nested
  action, opens the link dialog, and does not rely on right-click or long press.
- The desktop flow proves the linked-MR status control appears after linking
  and survives reload; mobile menu surfaces remain viewport-contained with
  touch-sized rows and no document horizontal overflow.

## Files Likely Touched

- `apps/web/e2e/tests/gitlab/gitlab-parity.spec.ts`
- `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts`
- Optional focused page-object helper under `apps/web/e2e/pages/` only when it
  removes repeated stable selectors.

## Dependencies

- `01-frontend-link-menu`

## Inputs

- Spec contextual-link/top-bar scenarios.
- Plan `E2E Tests` and `Mobile design contract` sections.
- Existing manual-link section in `gitlab-parity.spec.ts`.
- Existing mobile task-actions geometry coverage in
  `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts` and nested-link
  coverage in `apps/web/e2e/tests/task/mobile-external-link-menu.spec.ts`.

## Implementation Notes

- Follow `/e2e`, `/tdd`, and `/mobile-parity`; do not spawn subagents.
- Confirm the updated test fails for the expected missing menu action before
  relying on the implementation.
- Use the managed `pnpm e2e:run` runner so production frontend assets are
  rebuilt before the tests execute.
- Update only this task file's status. Do not edit `plan.md`.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/gitlab/gitlab-parity.spec.ts tests/gitlab/mobile-gitlab-parity.spec.ts
```

## Verification Status

The desktop and mobile coverage is implemented and passes Prettier, ESLint,
and `git diff --check`. The managed runner rebuilt the backend and frontend,
but Playwright could not start its worker backend because the sandbox denied
loopback listeners. An elevated retry was requested and declined, so the task
remains `in_progress` until the runtime scenarios can execute.

## Output Contract

Report changed E2E scenarios, the exact command and result, rendered desktop
and phone observations, failure artifact paths if any, blockers, and residual
risks. Set this task to `in_progress` before changes and `done` only after both
desktop and mobile acceptance criteria pass.
