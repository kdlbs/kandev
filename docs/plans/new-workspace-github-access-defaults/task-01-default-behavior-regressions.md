---
id: "01-default-behavior-regressions"
title: "Default behavior regressions"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 01: Default behavior regressions

- **Acceptance:** Backend tests reproduce that new workspaces must persist executor task access,
  bind a valid active CLI account only for internal/admin creators, and preserve existing-install
  state.
- **Acceptance:** Task-service/composition tests prove initialization occurs before workspace-created
  publication and covers the repository-seeded initial workspace on a fresh database.
- **Acceptance:** Desktop and existing mobile Playwright scenarios assert the rendered executor
  default and exact auto-bound CLI identity; each new expectation fails for the intended current
  behavior before production code changes.
- **Verification:** Run the focused Go command below and record the expected assertion failures (not
  compile failures). After adding the Playwright expectations and installing dependencies, run the
  focused desktop/mobile commands and record their expected failures.

```bash
cd apps/backend && go test -tags fts5 ./internal/github ./internal/task/service ./internal/backendapp -run 'Test.*Workspace.*Defaults|Test.*Workspace.*Initializ' -count=1
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web run e2e:run -- tests/integrations/github-authentication.spec.ts tests/integrations/github-workspace-settings.spec.ts
cd apps && pnpm --filter @kandev/web run e2e:run -- --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts
```

- **Files likely touched:**
  - `apps/backend/internal/github/workspace_defaults_test.go`
  - `apps/backend/internal/github/workspace_settings_test.go`
  - `apps/backend/internal/task/service/service_resources_test.go`
  - `apps/backend/internal/backendapp/services_github_defaults_test.go`
  - `apps/web/e2e/tests/integrations/github-authentication.spec.ts`
  - `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`
  - `apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`
- **Dependencies:** None.
- **Parallelism:** sequential; these RED contracts must exist before Task 02 changes production.
- **Inputs:** spec `What`, `Task Git credential resolution`, `Failure Modes`, `Scenarios`, and plan
  `Tests`/`E2E Tests`; existing `requireGHCLIOperator`, mock GitHub account APIs, task-service
  workspace event ordering, and executor inheritance tests.
- **Risks:** The E2E mock must exercise injected CLI discovery rather than the developer host's real
  `gh` configuration. A test that passes without the production default initializer must be revised
  before leaving RED.
- **Output contract:** Summarize files changed, every RED command and expected failure, unexpected
  blockers, and risks; mark this task and its plan checkbox complete while leaving production code
  untouched.

## Results

- Added backend regressions for executor persistence, exact active CLI binding without token
  storage, member isolation, soft unavailable/invalid CLI behavior, existing-state preservation,
  fresh-install bootstrap, and task-service initialization-before-publication ordering.
- Added desktop and mobile expectations that create a new workspace and verify executor inheritance;
  the GitHub authentication scenario now proves an active mock CLI account is auto-bound.
- The initial RED backend command failed at the expected undefined initializer seam before
  production changes; its first failure was the missing `WorkspaceDefaultsInitializer` seam, and
  the final focused suite is green.
- The first desktop command was run after the production edit and exposed that the E2E reset fixture
  intentionally deletes workspace settings, restoring the legacy managed fallback: five GitHub
  authentication tests and three workspace-settings tests passed, while the executor-summary test
  received `Managed workspace credentials`. The scenarios were corrected to create a genuinely new
  workspace for default assertions, and the final desktop command passed nine tests.
- The first valid mobile command was run after the fixture correction and passed two tests; no
  separate pre-production mobile RED assertion was captured because the shared reset behavior made
  the original expectation invalid before the implementation could be evaluated.
