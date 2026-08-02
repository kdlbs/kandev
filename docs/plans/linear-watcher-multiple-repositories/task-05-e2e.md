---
id: "05-e2e"
title: "E2E: multi-repo watcher flows"
status: pending
wave: 3
depends_on: ["04-frontend-picker-dialog"]
plan: "plan.md"
spec: "../../specs/linear-watcher-multiple-repositories/spec.md"
---

# Task 05: E2E — multi-repo watcher flows

## Acceptance

- Extending `apps/web/e2e/tests/integrations/linear-settings.spec.ts` (mock Linear provider; watch CRUD runs against the real SQLite store):
  - GIVEN a workspace with two seeded repositories (`apiClient.createRepository`), WHEN the user adds both to a new watcher via the repository add control and saves, THEN the stored watch (`apiClient.rawRequest("GET", "/api/v1/linear/watches/issue?workspace_id=…")`) has `repositories` with both entries in the added order.
  - WHEN the dialog is reopened for that watch, THEN both repo rows render with their saved branches.
  - GIVEN a watch created without touching the repository picker, THEN the stored watch carries no `repositories`, and triggering it still produces a task with no repository association (repo-less invariant, asserted via the created task).
- The new copy asserted in the spec (button/option names) matches the `linear` i18n catalog keys, and the spec passes locally.

## Verification

```bash
cd apps/web && pnpm e2e linear-settings.spec.ts
```

(Requires the e2e backend fixture, which already sets `KANDEV_MOCK_LINEAR`.)

## Files likely touched

- `apps/web/e2e/tests/integrations/linear-settings.spec.ts`
- `apps/web/e2e/pages/linear-settings-page.ts` (page-object selectors for the new repository control, if not already generic)

## Dependencies

Tasks 01–04 (full stack). This is the last task.

## Inputs

- Spec: `Scenarios` (all ten).
- Plan: `E2E` section.
- Existing patterns: `apiClient.createRepository(workspaceId, localPath, defaultBranch)` (`apps/web/e2e/helpers/api-client.ts:684-705`), `assertWatcherAgentProfileResetsToStepDefault` / `assertWatcherDispatchOrderPersists` sibling flows in the same spec file, Sentry multi-project e2e (`sentry-settings.spec.ts` from PR #1978) for the "reopen dialog shows saved selection" pattern.

## Risks

- Radix portals listboxes to the document root — scope option queries via `page.getByRole("option", …)` like the existing dialog tests.
- Repo seeding needs real local paths on the e2e host; mirror how the existing task-create e2e helpers seed repositories (`createRepository` + a temp git dir if the watcher flow requires an actual checkout — it should not for create/save/reopen assertions).
- Keep the invariant scenario cheap: assert the created task's repository association via the task API rather than launching a full agent run.

## Output contract

Report the added scenarios, the exact `pnpm e2e linear-settings.spec.ts` result, and any page-object additions; mark this task `done` and update its checkbox in `plan.md`.
