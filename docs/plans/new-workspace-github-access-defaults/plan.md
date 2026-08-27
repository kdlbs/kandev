---
spec: docs/specs/integrations/requirements/github-authentication.md
created: 2026-08-02
status: completed
---

# Implementation Plan: New Workspace GitHub Access Defaults

## Overview

The confirmed root cause is that `github_workspace_settings` resolves a missing task policy to
`managed`, while a new workspace has no `github_workspace_connections` row. Task launch therefore
requests a broker lease that fails with `ErrGitHubNotConfigured` before the agent starts. The fix
will persist executor task access for each new workspace, optionally snapshot an operator-authorized
active host `gh` account as a named CLI connection, and leave every existing installation unchanged.

The implementation begins with failing backend and browser regressions, then adds the smallest
workspace-initialization seam and GitHub persistence behavior required to make them pass. Public
documentation follows after the behavior is green.

---

## Backend

### GitHub workspace default initializer

- Add a focused workspace-default initialization path under
  `apps/backend/internal/github/` (prefer a new `workspace_defaults.go` with sibling tests rather
  than growing `service_connections.go`).
- Persist `TaskGitCredentialsModeExecutor` for a workspace created after this feature is active.
  Keep `defaultWorkspaceSettings`, `normalizeTaskGitCredentialsMode`, existing schema defaults, and
  upgrade fallbacks on `managed`; those are the backward-compatibility path for existing missing or
  invalid state.
- Discover the currently active account through the service's injectable CLI account source. Only
  an internal caller, synthetic administrator, or real administrator may receive the host account;
  reuse the existing `requireGHCLIOperator` boundary for member denial.
- Validate and store the exact `github.com` host/login as a `gh_cli` connection without persisting a
  bearer token. No active/valid CLI account is a soft condition: executor settings are still saved
  and workspace creation succeeds disconnected.
- Record whether the GitHub schema belongs to a brand-new database before `NewStore` initializes
  its tables. Expose a one-shot startup method that initializes the already-seeded default
  workspace only in that fresh-database case. Existing databases, including workspaces with no
  settings row, are not backfilled or rebound.

### Workspace creation seam

- Add a narrow `WorkspaceDefaultsInitializer` interface and setter in
  `apps/backend/internal/task/service/service.go`.
- Call it in `CreateWorkspace` after the workspace transaction succeeds and before
  `workspace.created` is published, so an immediately created task observes executor mode rather
  than racing an event subscriber. The initializer owns optional-CLI degradation; a returned
  persistence error is logged as a workspace-default initialization failure.
- Wire the GitHub service into this seam in `apps/backend/internal/backendapp/services.go` after
  workspace authorization and connection-secret infrastructure are installed. Invoke the
  fresh-database bootstrap at the same composition point for the repository-seeded initial
  workspace.
- Do not use a `workspace.created` subscription as the correctness boundary: the NATS-backed bus
  need not finish a subscriber before the create request can be followed by task launch.

### Upgrade and security boundaries

- Do not modify existing `github_workspace_connections` or
  `github_workspace_settings.task_git_credentials_mode` rows.
- Do not change the current `managed` normalization for legacy missing/invalid values.
- Do not auto-bind host `gh` for a non-admin member context.
- Do not copy or persist the resolved CLI token, and do not follow later host active-account
  changes for an already-created workspace.

---

## Frontend

No React component, route, API client, state slice, copy, or responsive composition changes are
required. The existing Workspace GitHub access summary and connection dialog already render the
backend's saved `executor` and `gh_cli` values.

### Mobile parity contract

The nearest shipped mobile exemplar remains
`apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`: the existing full-height,
single-scroll-owner connection drawer and 44px task-access controls are unchanged. Update that
scenario to prove the initial summary reads **Inherit executor Git credentials**. Because this is a
persisted backend-default change with no layout, navigation, scroll, touch, or composition change,
the focused existing mobile scenario satisfies mobile parity without a new mobile surface.

---

## Tests

- **What:** a new workspace always receives persisted executor task access; an internal/admin
  creator with a valid active CLI account also receives an exact named CLI connection.
  **File:** `apps/backend/internal/github/workspace_defaults_test.go`.
  **How:** SQLite-backed service tests with injectable CLI account/token validation boundaries.
- **What:** member creation never receives the server operator's CLI identity; unavailable,
  unauthenticated, or invalid CLI discovery degrades to disconnected automation without losing
  executor task access.
  **File:** `apps/backend/internal/github/workspace_defaults_test.go`.
  **How:** table-driven service tests using real store reads and synthetic/admin/member contexts.
- **What:** a database with pre-existing GitHub schema/settings is not treated as fresh and its
  missing or managed behavior is not rewritten, while a genuinely fresh schema initializes the
  seeded workspace once.
  **Files:** `apps/backend/internal/github/workspace_defaults_test.go`,
  `apps/backend/internal/github/workspace_settings_test.go`.
  **How:** fresh and replayed SQLite store setup with row-level assertions.
- **What:** task-service workspace creation runs default initialization before publishing the
  created event.
  **File:** `apps/backend/internal/task/service/service_resources_test.go`.
  **How:** a recording initializer and event bus assert call order and error handling.
- **What:** backend composition wires runtime workspace creation and fresh-install startup to the
  GitHub initializer.
  **File:** `apps/backend/internal/backendapp/services_github_defaults_test.go` (or the nearest
  existing service-composition test if a smaller harness is available during RED).
  **How:** real SQLite repositories with injected GitHub account resolution and stored-state
  assertions.
- **What:** executor policy skips broker lease injection.
  **File:** `apps/backend/internal/orchestrator/executor/executor_credentials_test.go`.
  **How:** retain the existing `executor inheritance` regression as downstream contract evidence;
  add no duplicate test unless the new integration test cannot prove the no-lease outcome.

Targeted backend verification:

```bash
cd apps/backend && go test -tags fts5 ./internal/github ./internal/task/service ./internal/backendapp ./internal/orchestrator/executor -run 'Test.*Workspace.*Defaults|Test.*Workspace.*Initializ|TestTaskGitCredentialModeDefaultsManaged|TestApplyGitCredentialSnapshotUsesExecutorSources' -count=1
```

## E2E Tests

- **Scenario:** GIVEN a new workspace and an active mock CLI account, WHEN an administrator creates
  the workspace and opens Workspace GitHub access, THEN the UI shows that exact CLI automation
  identity and **Inherit executor Git credentials**.
  **File:** `apps/web/e2e/tests/integrations/github-authentication.spec.ts`.
  **What to verify:** automation source/login and task-access summary through the rendered settings
  page, with backend seeding only for the CLI-account precondition.
- **Scenario:** GIVEN the initial fresh E2E workspace, WHEN desktop and mobile users open Workspace
  GitHub access, THEN the existing summary and dialog/drawer show executor inheritance as the
  initial task policy.
  **Files:** `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`,
  `apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`.
  **What to verify:** desktop and mobile report the same saved policy; existing mobile geometry,
  scroll ownership, safe-area containment, and touch targets remain green.

Targeted browser verification after the fresh-worktree install:

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web run e2e:run -- tests/integrations/github-authentication.spec.ts tests/integrations/github-workspace-settings.spec.ts
cd apps && pnpm --filter @kandev/web run e2e:run -- --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts
```

---

## Public Documentation

Update `docs/public/integrations.md` so **Inherit executor Git credentials** is documented as the
new-workspace default, managed credentials are described as an opt-in workspace policy, active host
`gh` auto-binding is limited to operator-authorized creation, remote executors remain responsible
for their own credentials, and upgrades are unchanged.

Validate with:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

---

## Verification Results

- `cd apps/backend && go test -tags fts5 ./internal/github ./internal/task/service ./internal/backendapp ./internal/orchestrator/executor -run 'Test.*Workspace.*Defaults|Test.*Workspace.*Initializ|TestTaskGitCredentialModeDefaultsManaged|TestApplyGitCredentialSnapshotUsesExecutorSources' -count=1` — 12 passed in 4 packages.
- `cd apps/backend && go test -tags fts5 ./internal/github ./internal/task/service ./internal/backendapp ./internal/orchestrator/executor -count=1` — 2402 passed in 4 packages.
- `cd apps && pnpm --filter @kandev/web run e2e:run -- tests/integrations/github-authentication.spec.ts tests/integrations/github-workspace-settings.spec.ts` — 9 passed.
- `cd apps && pnpm --filter @kandev/web run e2e:run -- --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts` — 2 passed.
- `cd apps/backend && make lint` — 0 issues.
- `cd apps && pnpm --filter @kandev/web run typecheck` — passed.
- `cd apps && pnpm --filter @kandev/web run lint` — passed.
- `cd apps && pnpm --filter @kandev/web run i18n:check && pnpm --filter @kandev/web run i18n:ratchet` — passed.
- `node --test scripts/validate-public-docs.test.mjs` — 58 passed.
- `node scripts/validate-public-docs.mjs` — 41 published docs pages validated.
- `cd apps && pnpm run format:check` and `git diff --check` — passed.

---

## Implementation Waves And Parallel Candidates

Execution is sequential in the primary conversation; no task is marked parallel-safe because the
RED tests define the contracts consumed by the backend implementation and documentation must match
the final green behavior.

- [x] [Task 01: Add failing default-behavior regressions](task-01-default-behavior-regressions.md)
- [x] [Task 02: Implement workspace GitHub defaults](task-02-workspace-github-defaults.md)
- [x] [Task 03: Update public integration documentation](task-03-public-integration-documentation.md)
