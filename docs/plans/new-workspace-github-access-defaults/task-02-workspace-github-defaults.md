---
id: "02-workspace-github-defaults"
title: "Workspace GitHub defaults"
status: completed
wave: 2
depends_on: ["01-default-behavior-regressions"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 02: Workspace GitHub defaults

- **Acceptance:** Every newly created workspace and the initial workspace on a genuinely fresh
  database persist executor task access before they can launch a task; existing installations are
  not backfilled or rewritten.
- **Acceptance:** Internal/synthetic-admin/real-admin creation stores a validated active CLI
  host/login without a token, while member creation and absent/invalid CLI state remain disconnected
  and still succeed.
- **Acceptance:** All Task 01 backend, desktop, and mobile regressions pass, and executor mode issues
  no managed credential lease.
- **Verification:** Run the targeted backend and managed desktop/mobile E2E commands exactly as
  listed below after the final production edit.

```bash
cd apps/backend && go test -tags fts5 ./internal/github ./internal/task/service ./internal/backendapp ./internal/orchestrator/executor -run 'Test.*Workspace.*Defaults|Test.*Workspace.*Initializ|TestTaskGitCredentialModeDefaultsManaged|TestApplyGitCredentialSnapshotUsesExecutorSources' -count=1
cd apps && pnpm --filter @kandev/web run e2e:run -- tests/integrations/github-authentication.spec.ts tests/integrations/github-workspace-settings.spec.ts
cd apps && pnpm --filter @kandev/web run e2e:run -- --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts
```

- **Files likely touched:**
  - `apps/backend/internal/github/store.go`
  - `apps/backend/internal/github/workspace_defaults.go`
  - `apps/backend/internal/github/workspace_defaults_test.go`
  - `apps/backend/internal/github/workspace_settings_test.go`
  - `apps/backend/internal/task/service/service.go`
  - `apps/backend/internal/task/service/service_resources.go`
  - `apps/backend/internal/task/service/service_resources_test.go`
  - `apps/backend/internal/backendapp/services.go`
  - `apps/backend/internal/backendapp/services_github_defaults_test.go`
  - `apps/web/e2e/tests/integrations/github-authentication.spec.ts`
  - `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`
  - `apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`
- **Dependencies:** Task 01.
- **Parallelism:** sequential; it changes the shared service-composition and GitHub workspace
  persistence path.
- **Inputs:** Task 01 RED evidence; ADR-2026-08-02-new-workspace-github-access-defaults; existing
  GitHub connection validation/commit functions, mock account injection, workspace authorization,
  and task-service workspace creation ordering.
- **Risks:** Do not use ambient host `gh` in E2E, do not auto-bind members, do not reinterpret legacy
  missing settings as executor, and do not make CLI unavailability fail workspace creation.
- **Output contract:** Summarize implementation, actual files changed, exact GREEN command results
  and Playwright artifact paths, security/external-side-effect boundaries, and remaining risks;
  update this task and `plan.md` statuses/results.

## Results

- Added `Store.EnsureWorkspaceExecutorDefaults` and fresh-schema detection; existing missing or
  invalid rows still resolve through the managed compatibility fallback.
- Added the GitHub workspace-default initializer, operator/admin boundary, injectable CLI account
  discovery, token-free named connection commit, soft CLI degradation, and one-shot seeded-workspace
  bootstrap.
- Added the task-service initializer seam and wired it before `workspace.created`; backend composition
  wires the GitHub service and invokes fresh-install bootstrap after authorization/secret setup.
- Focused Go suite: `12 passed in 4 packages`.
- Scoped Go suite: `2402 passed in 4 packages`.
- Desktop GitHub authentication/settings E2E: `9 passed` in the combined run after the fixture
  correction; focused settings rerun `4 passed`.
- Mobile GitHub settings E2E: `2 passed`.
- No CLI bearer token is persisted; member-created workspaces never list or bind operator accounts.
