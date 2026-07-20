---
id: "03-task-mr-linking-launch"
title: "Task-to-MR linking and quick launch"
status: done
wave: 2
depends_on: ["01-workspace-connection"]
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 03: Task-to-MR Linking And Quick Launch

## Acceptance

- A configured-host MR URL can be idempotently linked to an existing task and
  repository, while wrong-host/cross-workspace URLs are rejected and unlinking
  removes only the selected association/refresh watch.
- MR and issue rows expose action-preset task launch; successful MR launch links
  the created task, while issue launch carries URL context without a durable
  issue association.
- GitLab rows show single/multiple linked-task indicators and update after
  link/unlink on desktop and mobile without affecting GitHub launch behavior.

## Verification

```bash
cd apps/backend && rtk go test -run 'Test.*(TaskMR|MRURL|AssociateExistingMR|UnlinkTaskMR)' ./internal/gitlab/...
cd apps && rtk pnpm --filter @kandev/web test -- --run lib/api/domains/gitlab-api.test.ts lib/state/slices/gitlab/gitlab-slice.test.ts hooks/domains/gitlab/use-task-mr.test.ts components/gitlab/my-gitlab/quick-task-launcher.test.tsx components/gitlab/my-gitlab/mr-row-task-indicator.test.tsx components/gitlab/my-gitlab/mr-list.test.tsx components/gitlab/my-gitlab/issue-list.test.tsx
cd apps/web && rtk pnpm run typecheck
```

## Files Likely Touched

- `apps/backend/internal/gitlab/service_task_mr_link.go` (new)
- `apps/backend/internal/gitlab/service_task_mr_link_test.go` (new)
- `apps/backend/internal/gitlab/controller_task_mrs.go` (new)
- `apps/backend/internal/gitlab/controller_task_mrs_test.go` (new)
- `apps/backend/internal/gitlab/controller.go`
- `apps/backend/internal/gitlab/store.go`
- `apps/backend/internal/gitlab/store_test.go`
- `apps/backend/internal/gitlab/service_sync.go`
- `apps/web/lib/types/gitlab.ts`
- `apps/web/lib/api/domains/gitlab-api.ts`
- `apps/web/lib/api/domains/gitlab-api.test.ts`
- `apps/web/lib/state/slices/gitlab/types.ts`
- `apps/web/lib/state/slices/gitlab/gitlab-slice.ts`
- `apps/web/lib/state/slices/gitlab/gitlab-slice.test.ts`
- `apps/web/hooks/domains/gitlab/use-task-mr.ts`
- `apps/web/hooks/domains/gitlab/use-task-mr.test.ts`
- `apps/web/hooks/domains/gitlab/use-mr-key-to-tasks.ts` (new)
- `apps/web/hooks/domains/gitlab/use-mr-key-to-tasks.test.ts` (new)
- `apps/web/components/gitlab/my-gitlab/quick-task-launcher.tsx` (new)
- `apps/web/components/gitlab/my-gitlab/quick-task-launcher.test.tsx` (new)
- `apps/web/components/gitlab/my-gitlab/mr-row-task-indicator.tsx` (new)
- `apps/web/components/gitlab/my-gitlab/mr-row-task-indicator.test.tsx` (new)
- `apps/web/components/gitlab/my-gitlab/mr-list.tsx`
- `apps/web/components/gitlab/my-gitlab/mr-list.test.tsx` (new)
- `apps/web/components/gitlab/my-gitlab/issue-list.tsx`
- `apps/web/components/gitlab/my-gitlab/issue-list.test.tsx` (new)
- `apps/web/app/gitlab/gitlab-page-client.tsx`

## Dependencies

Task 01 supplies authoritative connection/host resolution. This task owns the
task-MR HTTP handler file; Task 04 must not edit it.

## Inputs

- Spec: browse/launch/link `What`, task association data, Task-to-MR API,
  association failure modes, and link/launch scenarios.
- Pattern: `apps/backend/internal/github/controller.go` `httpCreateTaskPR`,
  GitHub `AssociateExistingPRByURL`, and
  `apps/web/components/github/my-github/quick-task-launcher.tsx`.
- Pattern: GitHub `PRRowTaskIndicator` and `use-pr-key-to-tasks`.
- Constraint: project path matching includes subgroups and configured host;
  never infer self-managed URLs as `gitlab.com`.

## Output Contract

Report URL normalization, workspace/repository validation, association cleanup,
launch defaults, indicator keying, mobile implications, files changed, tests
run, blockers, and residual race behavior. Mark this task `done` and update the
plan after verification.
