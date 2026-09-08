---
id: "03-gitlab-workspace-repo-contents"
title: "GitLab workspace-routed repository content reads"
status: done
wave: 2
depends_on: ["01-gitlab-repo-content-client"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/gitlab-workflow-sync.md"
---

# Task 03: GitLab Workspace-Routed Repository Content Reads

## Acceptance

1. `gitlab.Service` exposes
   `ListRepoTreeForWorkspace(ctx, workspaceID, projectPath, path, ref string)`
   and `GetRepoFileContentForWorkspace(ctx, workspaceID, projectPath, path, ref string)`,
   each resolving the client through the existing `ClientForWorkspace` and
   delegating to the task-01 client methods.
2. A workspace with no GitLab connection produces an actionable error naming
   GitLab (not a nil-pointer panic and not a generic failure), propagated from
   the existing `ClientForWorkspace` error path.
3. No new credential, host, or token handling is introduced — host and auth
   resolution stay entirely inside `ClientForWorkspace`, so self-managed GitLab
   works without additional configuration.

## Verification

```bash
cd apps/backend && go test ./internal/gitlab/... -race
```

## Files Likely Touched

- `apps/backend/internal/gitlab/service_repo_contents.go` *(new)*.
- `apps/backend/internal/gitlab/service_repo_contents_test.go` *(new)*.

## Inputs

- Spec `## API Surface` → `### gitlab.Client`, and `## Permissions`.
- `apps/backend/internal/github/service_repo_contents.go:5-29` is the shape to
  mirror, minus `ensureRepositoryInWorkspaceScope` and
  `resolveAutomationClient` — GitLab has no App-installation or repo-scope-mode
  concept, so the workspace token is itself the scope boundary (spec
  `## Permissions`).
- `service_config.go:432-458` — `ClientForWorkspace` / `ClientForWorkspaceHost`.

## Risks

- Reaching for `ClientForWorkspaceHost` with an explicit host would reintroduce
  a host field the spec deliberately excludes. Use the plain
  `ClientForWorkspace`.

## Output Contract

Two workspace-routed methods on `gitlab.Service` that structurally satisfy the
`GitLabClientProvider` interface defined in task 04, with a test covering the
missing-connection error. Nothing imports them yet.
