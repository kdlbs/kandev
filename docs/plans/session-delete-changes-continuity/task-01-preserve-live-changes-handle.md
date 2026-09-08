---
id: "01-preserve-live-changes-handle"
title: "Preserve the live Changes lookup handle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9
system_design:
  - ../../specs/tasks/system-design/session-delete-resource-cleanup.md
---

# Task 01: Preserve the Live Changes Lookup Handle

## Summary

Make the environment-stable session selector retain only a live lookup handle.
Use unit and browser regressions to prove the correction. Deleting an active
session must not empty workspace or pull-request data from a shared task
environment.

## In scope

- Add the environment-session selector unit regression before production code.
- Correct cached-handle validation without changing ordinary live
  same-environment switch behavior.
- Add a desktop session-deletion E2E regression that verifies workspace and PR
  files remain in Changes.

## Out of scope

- Backend lifecycle or API changes.
- Changes panel presentation or mobile-specific composition changes.
- Broad refactors of environment-keyed frontend state.

## Acceptance

- A cached session remains stable while it is active or remains mapped to the
  active environment.
- Removing the cached session's mapping promotes the current active session,
  including when that session resolves to the same environment.
- Deleting the active session through the UI leaves workspace and matching PR
  files visible in Changes through the surviving session.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web test -- hooks/use-environment-session-id.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run tests/session/session-tab-management.spec.ts -- --grep "deleting the active shared-environment session keeps Changes data visible")
```

## Files likely touched

- `apps/web/hooks/use-environment-session-id.ts`
- `apps/web/hooks/use-environment-session-id.test.ts`
- `apps/web/e2e/tests/session/session-tab-management.spec.ts`

## Dependencies

None.

## Risks

- The selector must not discard a valid cached handle during a normal
  same-environment tab switch.
- The pre-hydration active-session fallback must remain valid.
- The PR fixture must be branch-scoped to the environment checkout.

## Parallelism

`sequential`

## Inputs

- `AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9`.
- The frontend environment-handle continuity section in the paired system
  design.
- `useSessionActions.remove` ordering and session-runtime purge behavior.
- Existing active-session deletion and PR Changes Playwright scenarios.

## Results

- Added a failing selector regression for a purged same-environment cache
  handle, then corrected the selector to promote the surviving active session.
- Preserved live same-environment handle stability, environment-switch
  replacement, and the pre-hydration active-session fallback in unit coverage.
- Added a Chromium regression that deletes the active session through the UI
  and verifies that a workspace file and branch-matching PR file remain visible
  in Changes.
- Verification passed: 4 focused Vitest cases, web TypeScript, and 1 focused
  production-build Playwright case.
