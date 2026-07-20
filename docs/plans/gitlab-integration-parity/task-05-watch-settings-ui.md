---
id: "05-watch-settings-ui"
title: "GitLab watch settings UI"
status: done
wave: 3
depends_on: ["01-workspace-connection", "02-watcher-dispatch"]
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 05: GitLab Watch Settings UI

## Acceptance

- The active workspace's GitLab settings page can create and edit review and
  issue watches with workflow step, profiles, optional repository/project
  filters, prompt/query, poll interval, inflight limit, and cleanup policy.
- Tables provide enable/pause, run-now, reset-preview/reset, and delete actions;
  action errors and dependency self-heal errors remain visible.
- All controls fit and remain operable in narrow/mobile layouts, with destructive
  reset requiring confirmation that shows the affected-task count.

## Verification

```bash
cd apps && rtk pnpm --filter @kandev/web test -- --run hooks/domains/gitlab/use-gitlab-review-watches.test.ts hooks/domains/gitlab/use-gitlab-issue-watches.test.ts components/gitlab/review-watch-dialog.test.tsx components/gitlab/review-watch-table.test.tsx components/gitlab/issue-watch-dialog.test.tsx components/gitlab/issue-watch-table.test.tsx components/gitlab/gitlab-settings.test.tsx
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web lint
```

## Files Likely Touched

- `apps/web/lib/types/gitlab.ts`
- `apps/web/lib/api/domains/gitlab-api.ts`
- `apps/web/lib/api/domains/gitlab-api.test.ts`
- `apps/web/hooks/domains/gitlab/use-gitlab-review-watches.ts`
- `apps/web/hooks/domains/gitlab/use-gitlab-review-watches.test.ts` (new)
- `apps/web/hooks/domains/gitlab/use-gitlab-issue-watches.ts`
- `apps/web/hooks/domains/gitlab/use-gitlab-issue-watches.test.ts` (new)
- `apps/web/components/gitlab/review-watch-dialog.tsx` (new)
- `apps/web/components/gitlab/review-watch-dialog.test.tsx` (new)
- `apps/web/components/gitlab/review-watch-table.tsx` (new)
- `apps/web/components/gitlab/review-watch-table.test.tsx` (new)
- `apps/web/components/gitlab/issue-watch-dialog.tsx` (new)
- `apps/web/components/gitlab/issue-watch-dialog.test.tsx` (new)
- `apps/web/components/gitlab/issue-watch-table.tsx` (new)
- `apps/web/components/gitlab/issue-watch-table.test.tsx` (new)
- `apps/web/components/gitlab/review-watch-placeholders.ts` (new)
- `apps/web/components/gitlab/issue-watch-placeholders.ts` (new)
- `apps/web/components/gitlab/gitlab-settings.tsx`
- `apps/web/components/gitlab/gitlab-settings.test.tsx`

## Dependencies

Tasks 01 and 02 must establish workspace config, validated watch contracts,
reset endpoints, and dispatcher/self-heal behavior.

## Inputs

- Spec: watch `What`, Automation watch state machine, watch permissions/failure
  modes, and watch scenarios.
- Patterns: `apps/web/components/github/{review-watch-dialog.tsx,review-watch-table.tsx,issue-watch-dialog.tsx,issue-watch-table.tsx}`.
- Patterns: Jira/Linear issue-watch sections for workspace-scoped repository,
  workflow, and profile defaults.
- Required skills during implementation: `/mobile-parity` and frontend scoped
  guidance in `apps/web/AGENTS.md`.

## Output Contract

Report supported watch fields/actions, error and confirmation states, responsive
behavior, files changed, tests run, blockers, and remaining E2E needs. Mark this
task `done` and update `plan.md` after unit/type/lint verification.
