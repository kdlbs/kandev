---
id: "13-ux-delta-e2e"
title: "UX delta end-to-end coverage"
status: done
wave: 8
depends_on:
  - "11-shared-local-remote-selector"
  - "12-unified-files-workspace-actions"
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 13: UX delta end-to-end coverage

## Acceptance

- Desktop E2E opens the combined menu, exercises both actions, adds sources through the shared
  Local/Remote selector, and proves rows survive mode switching.
- Mobile E2E proves the 44px trigger/menu/mode geometry, inset action menu, full-height Add sources
  drawer, safe-area/footer containment, no document horizontal overflow, submit outcome, and focus
  return.
- Executor E2E enters Add sources through the combined menu and continues proving capability
  filtering without weakening backend source validation. The disposable HTTP Git fixture uses
  test-appropriate provider semantics instead of mislabeling a bridge URL as GitLab.

## Verification

```bash
cd apps/web && pnpm e2e:run e2e/tests/task/add-workspace-sources.spec.ts
cd apps/web && pnpm e2e:run e2e/tests/task/mobile-add-workspace-sources.spec.ts
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e -- \
  e2e/tests/docker/add-workspace-sources.spec.ts \
  e2e/tests/ssh/add-workspace-sources.spec.ts \
  --project=containers
```

## Files likely touched

- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts`
- `apps/web/e2e/tests/docker/add-workspace-sources.spec.ts`
- `apps/web/e2e/tests/ssh/add-workspace-sources.spec.ts`
- `apps/web/e2e/helpers/http-git-server.ts`
- Existing E2E page objects/helpers only when the same interaction is repeated.

## Dependencies

Tasks 11 and 12.

## Inputs

- Spec: combined-menu, selector-parity, mixed-row, active-turn, and phone scenarios.
- Plan: **Tests**, **E2E Tests**, and **Mobile design contract**.
- Mobile project: configured `mobile-chrome` Pixel 5; do not add per-test device overrides.

## Output contract

- Update only this task file to `in_progress` when starting and `done` after acceptance and
  verification pass.
- Return a compact handoff capsule with exact test names, commands/results, screenshot or rendered
  evidence, environment blockers, risk tags, and any uncertainty.
- Do not update `plan.md`; the planner serializes shared-plan status.
