---
id: "02-contribution-status-and-state"
title: "Contribution status and state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/rich-task-title-previews.md"
---

# Task 02: Contribution status and state

- **Acceptance:** GitHub and GitLab use one generic summary renderer. Co-mounted GitLab consumers share one in-flight request. Later mounts can refresh, and failed refreshes clear stale workspace data.
- **Verification:** `cd apps && pnpm --filter @kandev/web test -- --run hooks/domains/gitlab/use-task-mr.test.ts components/gitlab/mr-task-status-summary.test.ts components/gitlab/mr-task-icon.render.test.tsx components/github/pr-task-icon.test.ts components/github/pr-task-icon.render.test.tsx`.
- **Files likely touched:** change-request summary components, GitLab adapters, hydration hook, and focused tests.
- **Dependencies:** None.
- **Parallelism:** sequential because the renderer and hook share GitLab consumers.

## Results

The final seven-file Vitest command passed 130 tests. It includes 21 GitLab hook tests and the GitHub and GitLab renderer tests. `cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet` passed. External side effects: None.
