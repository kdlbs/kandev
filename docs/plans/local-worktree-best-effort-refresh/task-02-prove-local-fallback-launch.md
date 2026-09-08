---
id: "02-prove-local-fallback-launch"
title: "Prove Desktop and Mobile Launch"
status: completed
wave: 2
depends_on:
  - "01-restore-local-base-fallback"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.7
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 02: Prove Desktop and Mobile Launch

## Summary

Replace E2E expectations that treat every refresh error as fatal. Prove that a
task starts from a valid local base on desktop and mobile when origin is
unavailable.

## In scope

- Convert the desktop required-refresh case into a local-fallback launch case.
- Convert the mobile required-refresh case into the same local-fallback outcome.
- Assert that the session starts and no active launch error remains.
- Restore all modified seed repository state after each case.
- Preserve unrelated missing-base and launch-recovery scenarios.

## Out of scope

- UI layout or component changes.
- New mobile navigation or touch interactions.
- Backend manager implementation.

## Acceptance

- The desktop task reaches a running, idle, or completed session from the local base.
- The mobile task reaches the same state without an active launch error.
- Both specs restore origin and repository settings after completion.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/task/launch-failure-recovery.spec.ts -- --grep 'local base'
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts -- --grep 'local base'
```

## Files likely touched

- `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`
- `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` only if existing seed methods are insufficient.

## Dependencies

- Task 01 must be complete.

## Risks

- The tests can leak a broken origin into later worker-scoped cases.
- A backend-only assertion can miss the user-visible session outcome.

## Parallelism

`sequential`

## Inputs

- `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3`
- `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.7`
- Existing desktop and mobile launch-failure recovery fixtures.

## Results

- The desktop launch-recovery case now proves a valid local base starts a
  session when origin refresh fails and leaves no active launch error.
- The mobile launch-recovery case proves the same outcome in the
  `mobile-chrome` project while retaining the existing native mobile recovery
  coverage for missing bases.
- Both focused E2E commands passed and restore the seed origin and checkout
  state in cleanup.
