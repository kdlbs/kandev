---
id: "04-production-operation-audit"
title: "Localize production operation copy"
status: done
wave: 4
depends_on: ["03-review-plan-operations"]
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n-second-audit-gaps.md"
---

# Task 04: Production Operation Audit

## Acceptance

- File operations, task creation, session/task actions, task movement, GitHub/GitLab feedback, and
  sidebar synchronization expose no owned English toast/error copy found by the source audit.
- Unknown owned fallbacks are localized while raw server/domain details and console diagnostics remain unchanged.
- Changed helper logic receives failing tests first and all nearest focused tests pass.

## Likely Files

`apps/web/hooks/use-file-operations.ts`, `use-sidebar-views-sync.ts`, task/session/movement/file hooks,
GitHub/GitLab feedback hooks, `apps/web/components/task-create-dialog-submit.tsx`, task-create helpers,
new-session/file-browser/quick-launch components, tests, catalogs, and guard configuration.

## Verification

```bash
cd apps/web && pnpm test -- --run hooks/use-file-operations.test.ts hooks/use-sidebar-views-sync.test.ts components/task-create-dialog-submit.test.tsx hooks/domains/session/use-session-actions.test.ts hooks/use-task-workflow-move.test.ts && pnpm run lint:i18n -- hooks components/task-create-dialog-submit.tsx components/task-create-dialog-effects.ts components/task-create-dialog-fresh-branch-consent.ts
```

## Risks

The sweep includes tests, fixtures, and domain data; translate only copy with a confirmed user-visible path.

## Results

Localized task/session/file operations, task creation and movement, GitLab association recovery,
repository discovery, editor fallbacks, plan actions, nesting, and sidebar synchronization. Focused
tests, typecheck, source audit, and guard lint passed.
