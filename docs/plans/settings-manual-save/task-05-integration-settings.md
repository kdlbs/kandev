---
id: "05-integration-settings"
title: "Integration settings migration"
status: done
wave: 2
depends_on: ["01-save-coordinator"]
plan: "plan.md"
spec: "../../specs/ui/settings-manual-save.md"
---

# Task 05: Integration Settings Migration

## Acceptance

- Page-level integration forms and GitHub scope/preset/query editors use the shared floating action without embedded Save buttons.
- GitHub/Jira/Linear/Sentry watcher enabled toggles are drafts; watcher dialogs, manual runs/resets, and confirmed deletes remain immediate.
- Resetting default queries/action presets stays local until Save.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/github components/gitlab components/jira components/linear components/sentry components/slack
cd apps/web && pnpm run typecheck
```

## Files Likely Touched

- `apps/web/components/github/github-settings.tsx`
- `apps/web/components/github/github-repo-scope-section.tsx`
- `apps/web/components/github/action-presets-section.tsx`
- `apps/web/components/github/default-queries-section.tsx`
- `apps/web/components/github/review-watch-table.tsx`
- `apps/web/components/github/issue-watch-table.tsx`
- `apps/web/components/gitlab/gitlab-settings.tsx`
- `apps/web/components/jira/jira-settings.tsx`
- `apps/web/components/jira/task-presets-section.tsx`
- `apps/web/components/jira/jira-issue-watchers-section.tsx`
- `apps/web/components/linear/linear-settings.tsx`
- `apps/web/components/linear/linear-issue-watchers-section.tsx`
- `apps/web/components/sentry/sentry-issue-watchers-section.tsx`
- `apps/web/components/slack/slack-settings.tsx`

## Dependencies

Task 01.

## Inputs

- Spec: Settings-wide coverage, Immediate actions, reset scenarios.
- Existing integration form save handlers and watcher dialog boundaries.

## Output Contract

Report each migrated form/toggle, retained immediate commands, tests run, files touched, integration-specific risks, and update task/plan status.

## Result

- Migrated GitHub, GitLab, Jira, Linear, Sentry, and Slack route-level forms to the shared
  floating action while retaining overlay-local connection and destructive commands.
- Drafted GitHub action presets/default queries/repository scope, Jira task presets, and
  GitHub/Jira/Linear/Sentry watcher enabled state until Save.
- Added existing Sentry instance editing to the route coordinator; new-instance creation keeps
  its explicit form action.
- Verified 363 focused integration tests, web typecheck, scoped ESLint, and affected Playwright
  scenarios.
