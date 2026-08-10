---
id: "05-wire-gitlab-provider"
title: "Wire the GitLab provider into workflow sync"
status: done
wave: 3
depends_on: ["03-gitlab-workspace-repo-contents", "04-workflowsync-provider-dispatch"]
plan: "plan.md"
spec: "../../specs/gitlab-workflow-sync/spec.md"
---

# Task 05: Wire The GitLab Provider Into Workflow Sync

## Acceptance

1. `initWorkflowSyncService` accepts and forwards `gitlabSvc` alongside
   `githubSvc`, and `gitlab.Service` satisfies `GitLabClientProvider` at
   compile time. Backend boot succeeds with either integration unconfigured.
2. The HTTP config GET/POST payloads carry `provider` and `project_path`; a POST
   omitting `provider` is still accepted as `github`, so existing API clients
   are unaffected. Routes, methods, and status codes are unchanged, and an
   invalid provider returns HTTP 400 via `ErrInvalidConfig`.
3. An end-to-end backend test configures a GitLab sync against a mock GitLab
   client and asserts the fetched definitions are applied — covering spec
   scenarios 1 and 7 (nested subgroup `project_path`).

## Verification

```bash
cd apps/backend && go test ./internal/workflowsync/... ./internal/backendapp/... -race
```

## Files Likely Touched

- `apps/backend/internal/backendapp/services.go` — `initWorkflowSyncService`
  signature and its call site.
- `apps/backend/internal/workflowsync/handlers.go` — only if payload binding
  needs adjustment; the struct-driven bind may already suffice.
- `apps/backend/internal/workflowsync/service_test.go` or a new integration test.

## Inputs

- Spec `## API Surface` → `### HTTP`, and `## Scenarios` 1, 5, 7.
- `services.go:571-578` — current wiring, `githubSvc` only.
- `mock_client.go` from task 01 provides the seedable GitLab fixture.

## Risks

- A hard dependency on a configured GitLab service would break boot for
  GitHub-only installs. Both providers must be optional/nil-tolerant.
- Adding a required `provider` field to the POST contract would break existing
  clients. It must remain optional with a `github` default.

## Output Contract

GitLab workflow sync works end-to-end at the service and HTTP layers, verified
against a mock GitLab client including a nested subgroup path. Frontend is
task 06.
