---
id: "03-e2e-and-verification"
title: "Issue indicator E2E and verification"
status: done
wave: 3
depends_on: ["02-frontend-link-and-indicator"]
plan: "plan.md"
spec: "../../specs/tasks/link-existing-task-github-issue.md"
---

# Task 03: Issue Indicator E2E And Verification

## Acceptance

- Desktop Playwright coverage proves linked/unlinked/multiple task states and task navigation.
- Mobile Playwright coverage proves the indicator is reachable by touch and navigates without horizontal layout breakage.
- Formatting, relevant tests, typecheck, and lint pass.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/github/issue-list-task-indicator.spec.ts
cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/github/mobile-issue-list-task-indicator.spec.ts
make fmt && make typecheck test lint
```

## Files Likely Touched

- `apps/web/e2e/tests/github/issue-list-task-indicator.spec.ts`
- `apps/web/e2e/tests/github/mobile-issue-list-task-indicator.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`

## Output Contract

Report desktop/mobile results, full verification commands, files changed, blockers, and follow-up risks; mark this task done in this file and `plan.md`.
