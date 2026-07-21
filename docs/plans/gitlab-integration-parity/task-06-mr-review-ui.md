---
id: "06-mr-review-ui"
title: "Linked merge request review UI"
status: done
wave: 4
depends_on: ["03-task-mr-linking-launch", "04-reviewers-subscriptions"]
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 06: Linked Merge Request Review UI

## Acceptance

- A linked GitLab MR opens in the task review/dockview surface and shows live
  overview, branches, mergeability, files, commits, approvals, pipeline,
  reviewers, and threaded discussions.
- Users can reply/resolve, approve/unapprove, merge, set labels/assignees/
  reviewers, toggle GitLab notifications, link/unlink, refresh, and add selected
  feedback to task prompt context with success/error state.
- GitHub PR panels remain unchanged and all GitLab review actions are reachable
  with correct merge-request terminology on desktop and mobile.

## Verification

```bash
cd apps && rtk pnpm --filter @kandev/web test -- --run lib/api/domains/gitlab-api.test.ts components/gitlab/mr-detail-panel.test.tsx components/gitlab/mr-discussions-section.test.tsx components/gitlab/mr-reviewer-control.test.tsx components/gitlab/subscription-toggle.test.tsx components/task/dockview-session-tabs.test.ts components/task/task-center-panel.test.tsx
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web lint
```

## Files Likely Touched

- `apps/web/lib/types/gitlab.ts`
- `apps/web/lib/api/domains/gitlab-api.ts`
- `apps/web/lib/api/domains/gitlab-api.test.ts`
- `apps/web/hooks/domains/gitlab/use-mr-feedback.ts` (new)
- `apps/web/hooks/domains/gitlab/use-mr-actions.ts` (new)
- `apps/web/components/gitlab/mr-detail-panel.tsx` (new)
- `apps/web/components/gitlab/mr-detail-panel.test.tsx` (new)
- `apps/web/components/gitlab/mr-overview-section.tsx` (new)
- `apps/web/components/gitlab/mr-files-section.tsx` (new)
- `apps/web/components/gitlab/mr-commits-section.tsx` (new)
- `apps/web/components/gitlab/mr-discussions-section.tsx` (new)
- `apps/web/components/gitlab/mr-discussions-section.test.tsx` (new)
- `apps/web/components/gitlab/mr-reviewer-control.tsx` (new)
- `apps/web/components/gitlab/mr-reviewer-control.test.tsx` (new)
- `apps/web/components/gitlab/subscription-toggle.tsx` (new)
- `apps/web/components/gitlab/subscription-toggle.test.tsx` (new)
- `apps/web/components/gitlab/mr-topbar-button.tsx`
- `apps/web/components/task/task-center-panel.tsx`
- `apps/web/components/task/task-center-panel.test.tsx` (new)
- `apps/web/components/task/dockview-shared.tsx`
- `apps/web/components/task/dockview-panel-content.tsx`
- `apps/web/components/task/dockview-session-tabs.ts`
- `apps/web/components/task/dockview-session-tabs.test.ts`
- `apps/web/components/task/dockview-add-panel-items.tsx`
- `apps/web/components/task/task-pr-picker-dialog.tsx`
- `apps/web/components/task/chat-context-items.ts`
- `apps/web/lib/state/slices/comments/types.ts`
- `apps/web/lib/state/slices/comments/index.ts`

## Dependencies

Task 03 supplies durable links and Task 04 supplies reviewer/subscription
contracts. This task owns task-panel provider routing; Task 07 must reuse that
routing rather than edit the same dockview files.

## Inputs

- Spec: MR review `What`, reviewer/notification API, permissions/failures, and
  linked-MR/reviewer/subscription scenarios.
- Existing GitLab endpoints: feedback, discussions, files, commits, approvals,
  merge, labels, and assignees.
- Patterns: `apps/web/components/github/pr-detail-panel.tsx` and its section
  components, but retain GitLab discussion semantics instead of flattening them.
- Required skills during implementation: `/mobile-parity` and `/e2e` follow-up.

## Output Contract

Report provider routing, displayed live state, supported write actions, prompt
context shape, GitHub regression coverage, responsive behavior, files changed,
tests run, blockers, and tier/version risks. Mark this task `done` and update
`plan.md` after verification.
