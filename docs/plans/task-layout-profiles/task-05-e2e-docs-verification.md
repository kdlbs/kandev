---
id: "05-e2e-docs-verification"
title: "E2E, docs, and verification"
status: done
wave: 3
depends_on: ["03-layout-settings-editor", "04-workbench-default-integration"]
plan: "plan.md"
spec: "../../specs/ui/task-layout-profiles.md"
---

# Task 05: E2E, Docs, And Verification

## Acceptance

- Desktop and mobile layout-profile scenarios pass against a production build, including fresh-task terminal suppression, existing-task isolation, and Reset Layout.
- Shared E2E settings reset prevents `saved_layouts` leakage between worker tests.
- Public workbench documentation is current and all repository verification gates are green.

## Files likely touched

- `apps/web/e2e/fixtures/test-base.ts`
- `apps/web/e2e/pages/layout-settings-page.ts`
- `apps/web/e2e/tests/settings/layout-profiles.spec.ts`
- `apps/web/e2e/tests/settings/mobile-layout-profiles.spec.ts`
- `docs/public/sessions-and-review.md`

## Dependencies

- Task 03 provides the complete persisted settings workflow.
- Task 04 provides fresh/reset default behavior and terminal suppression.

## Inputs

- Spec settings, task-isolation, Reset Layout, terminal, persistence, and mobile scenarios.
- Existing E2E `ApiClient.saveUserSettings`, `SessionPage.waitForDockviewReady`, `window.__dockviewApi__`, and `user_shell.list` patterns.
- `/mobile-parity`, `/e2e`, `/docs-maintainer`, `/qa`, `/simplify`, and `/verify` workflows.

## Verification

```bash
make fmt
make typecheck
make test
make lint
make build-web
cd apps/web && pnpm e2e:run tests/settings/layout-profiles.spec.ts tests/task/task-default-layout.spec.ts tests/layout/saved-layout-session-isolation.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome -- tests/settings/mobile-layout-profiles.spec.ts --workers=1
```

## Output contract

Report user-visible behavior, unit/E2E/full verification results, files changed, blockers, and remaining risks; mark this task and its plan item done.
