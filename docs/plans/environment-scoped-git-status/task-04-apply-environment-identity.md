---
id: "04-apply-environment-identity"
title: "Apply delivered environment identity"
status: completed
wave: 4
depends_on:
  - "03-deliver-environment-status"
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001
  - REQ-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.5
  - AC-TASKS-SESSION-DELETE-RESOURCE-CLEANUP-001.9
system_design:
  - ../../specs/tasks/system-design/environment-owned-git-status.md
---

# Task 04: Apply Delivered Environment Identity

## Summary

Use the delivered task environment ID as the only current-status store key.
Prove that an old sibling snapshot cannot restore removed files after reload.

## In scope

- Require `task_environment_id` on frontend status-update payloads.
- Ignore session-only status updates.
- Pass environment identity directly to the Git-status store action.
- Keep repository identity as the second store key.
- Preserve timestamp response ordering.
- Normalize missing file-detail maps to empty objects.
- Add focused handler and store regressions.
- Add the two-session Changes browser regression.

## Out of scope

- Changes panel layout or interaction changes.
- New copy or translation keys.
- Separate mobile state or presentation logic.
- Identity changes for non-status Git events.

## Acceptance

- The handler never derives current-status identity from `session_id`.
- A payload without `task_environment_id` does not change Git status.
- A newer sparse observation clears older file details for its repository.
- Reload and sibling hydration do not restore removed files in Changes.

## Verification

```bash
(cd apps/web && pnpm test -- lib/ws/handlers/git-status.test.ts lib/state/slices/session-runtime/set-git-status-return.test.ts lib/state/slices/session-runtime/git-status-normalizer.test.ts)
(cd apps/web && pnpm run build:e2e)
(cd apps/web && pnpm e2e:run tests/session/session-tab-management.spec.ts --project=chromium -g "does not restore removed Changes files after sibling hydration")
```

## Files likely touched

- `apps/web/lib/types/git-events.ts`
- `apps/web/lib/ws/handlers/git-status.ts`
- `apps/web/lib/ws/handlers/git-status.test.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/state/slices/session-runtime/session-runtime-slice.ts`
- `apps/web/lib/state/slices/session-runtime/git-status-state.ts`
- `apps/web/lib/state/slices/session-runtime/git-status-normalizer.ts`
- `apps/web/lib/state/slices/session-runtime/git-status-normalizer.test.ts`
- `apps/web/lib/state/slices/session-runtime/set-git-status-return.test.ts`
- `apps/web/e2e/tests/session/session-tab-management.spec.ts`

## Dependencies

Task 03 supplies the required payload identity.

## Risks

- Existing tests and fixtures construct session-only status payloads.
- Sparse status payloads can contain undefined `files` at runtime.
- The E2E setup must wait for both sibling hydration messages by cause.

## Parallelism

`sequential`

## Mobile parity

Desktop and mobile Changes use the same store and hooks. This task changes no
composition, control, gesture, scroll owner, or breakpoint behavior.

Focused state tests cover the shared contract. No separate mobile browser case
is needed because this task changes no mobile presentation or interaction.

## Inputs

- System-design sections "Frontend storage" and "Workspace views"
- Existing `session.git.event` handler and timestamp-order tests
- Existing two-session Changes Chromium scenario

## Results

- Status-update handling now requires and directly uses `task_environment_id`.
- Sparse status payloads now normalize missing file maps to empty objects.
- Added focused handler and store regressions and the two-session Changes
  browser regression.
- Verified 26 focused frontend tests, typecheck, the E2E build, and the
  Chromium regression. The browser test passed.
- The browser regression helper now reads the stable single-repository
  environment status shape. The final targeted Chromium run passed.
