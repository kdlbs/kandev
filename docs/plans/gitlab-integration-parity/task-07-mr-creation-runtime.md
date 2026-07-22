---
id: "07-mr-creation-runtime"
title: "GitLab merge request creation runtime"
status: done
wave: 4
depends_on: ["01-workspace-connection", "03-task-mr-linking-launch"]
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 07: GitLab Merge Request Creation Runtime

## Acceptance

- `worktree.create_pr` detects GitLab HTTPS, SSH, and self-managed remotes,
  pushes the current branch, and creates a ready/draft MR through `glab` or the
  token-authenticated REST fallback using the explicit or project-default base.
- A successful response identifies `provider: "gitlab"`, uses MR terminology in
  desktop/mobile UI, and asynchronously links the returned URL to the originating
  task repository; retries are idempotent after partial push/MR failure.
- Workspace host/token injection and clone URL construction are isolated by
  workspace, redacted in errors/logs, and never fall back to `gitlab.com` for a
  self-managed connection.

## Verification

```bash
cd apps/backend && rtk go test -run 'Test.*(GitLab|CreatePR|PRProvider|PRCreated)' ./internal/agentctl/server/process/... ./internal/agent/handlers/... ./internal/backendapp/... ./internal/repoclone/... ./internal/orchestrator/executor/...
cd apps/backend && rtk go test ./internal/agentctl/server/process/... ./internal/agent/handlers/... ./internal/backendapp/... ./internal/repoclone/... ./internal/orchestrator/executor/...
cd apps && rtk pnpm --filter @kandev/web test -- --run hooks/use-git-operations.test.ts components/vcs/vcs-dialogs.test.tsx components/task/mobile/session-mobile-top-bar-git-controls.test.tsx components/task/changes-panel-data.test.tsx
```

## Files Likely Touched

- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_test.go`
- `apps/backend/internal/agentctl/server/process/git_pr_providers.go`
- `apps/backend/internal/agentctl/server/process/git_pr_providers_test.go`
- `apps/backend/internal/agent/runtime/agentctl/git.go`
- `apps/backend/internal/agent/handlers/git_handlers.go`
- `apps/backend/internal/agent/handlers/git_handlers_test.go`
- `apps/backend/internal/backendapp/gateway.go`
- `apps/backend/internal/backendapp/gateway_test.go` (new)
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_test.go`
- `apps/backend/internal/repoclone/protocol.go`
- `apps/backend/internal/repoclone/protocol_test.go`
- `apps/backend/internal/agent/credentials/env_provider.go`
- `apps/backend/internal/agentctl/AGENTS.md`
- `apps/web/hooks/use-git-operations.ts`
- `apps/web/hooks/use-git-operations.test.ts` (new)
- `apps/web/components/vcs/vcs-dialogs.tsx`
- `apps/web/components/vcs/vcs-dialogs.test.tsx` (new)
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.test.tsx` (new)
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel-data.test.tsx` (new)
- `apps/web/components/task/changes-panel-helpers.ts`
- `apps/web/lib/state/slices/gitlab/gitlab-slice.ts`

## Dependencies

Task 01 supplies workspace connection resolution/token ownership. Task 03
supplies the explicit association service used by the create callback.

## Inputs

- Spec: first-class MR creation `What`, creation protocol response, action
  failure modes, persistence, and draft-MR scenario.
- Scoped guidance: `apps/backend/internal/agentctl/AGENTS.md`.
- Patterns: GitHub and Azure creators in
  `apps/backend/internal/agentctl/server/process/git_pr_providers.go` and current
  GitHub callback in `apps/backend/internal/backendapp/gateway.go`.
- Constraint: JSON bodies must use structured encoding; command/log sanitization
  must cover title, description, token, and credential-bearing remotes.

## Output Contract

Report provider detection/remote parsing, glab/REST selection, target-branch
resolution, callback association, environment/clone resolution, UI terminology,
files changed, tests run, blockers, and security risks. Mark this task `done` and
update `plan.md` after verification.
