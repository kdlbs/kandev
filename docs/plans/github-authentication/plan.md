---
spec: docs/specs/integrations/github-authentication.md
created: 2026-07-19
status: done
---

# Implementation Plan: Workspace GitHub Authentication

## Overview

Replace the singleton GitHub client with a purpose-aware credential resolver keyed by workspace and
user. Preserve existing installations through an explicit legacy source, add PAT and named `gh`
account isolation, implement deployment GitHub App installation and user OAuth lifecycles, then
route backend, background, and executor operations through the new boundary before changing the UI.

## Architecture

- Product contract: `docs/specs/integrations/github-authentication.md`.
- Ownership decision: `docs/decisions/0047-github-authentication-ownership.md`.
- Existing integration ownership: `docs/decisions/0030-workspace-scoped-integration-settings.md`.
- `github.CredentialResolver.Resolve(ctx, Request)` is the only source of operational clients. A
  request carries `WorkspaceID`, optional `UserID`, purpose, and optional repository target; a
  resolution returns `Client`, principal metadata, capabilities, credential generation, and expiry.
- Automation purpose never resolves a personal credential. Personal purpose resolves the current
  user's App token, then a human PAT/CLI automation connection. Manual mutation may finally fall
  back to an App installation with explicit App attribution.
- One deployment App registration is loaded from config/env. Workspace and personal APIs receive
  only availability and non-secret metadata.
- Agent git transport uses a task-scoped broker lease and credential helper, not static App tokens
  or App private-key injection.

## Backend

### Persistence And Migration

Extend `apps/backend/internal/github/models.go` and split connection persistence into
`apps/backend/internal/github/store_connections.go`. `Store.initSchema` in `store.go` creates the
four spec tables, adds `workspace_id` to PR watches/task PRs, and runs replayable, dialect-safe
backfills. The migration inserts `legacy_shared` only for workspaces that exist at migration time;
normal workspace creation does not create a GitHub connection.

Use deterministic secret keys under a GitHub namespace:

- `github:workspace:<workspace_id>:pat`
- `github:user:<workspace_id>:<user_id>:access`
- `github:user:<workspace_id>:<user_id>:refresh`

Connection replacement uses validate-first semantics and a transaction/compensation path so a
failed secret write does not leave metadata pointing at absent credentials. Workspace deletion and
the E2E reset path remove connection rows and owned secrets. Workspace copy continues copying only
repository settings and presets.

### Deployment App Configuration

Add `GitHubAppConfig` to `apps/backend/internal/common/config/config.go` with App ID, client ID,
private key or private-key file, client secret, webhook secret, App slug, and public base URL.
Explicit `KANDEV_GITHUB_APP_*` bindings support multiline/file secrets and validate all-or-none
requirements. Callback URLs require HTTPS outside loopback development.

Create token-source-neutral REST/GraphQL construction in `apps/backend/internal/github/token_client.go`
from the current `pat_client.go`. Add App JWT signing, installation lookup, permission mapping, and
singleflight installation-token caching in `app_client.go` and `app_token_cache.go`. Refresh begins
before expiry; expired tokens are never returned.

### Credential Resolver

Add `auth_resolver.go`, `auth_principal.go`, `gh_accounts.go`, and
`service_connections.go` under `apps/backend/internal/github/`. Refactor `factory.go` so its current
discovery order is available only to `legacy_shared`. Named CLI resolution uses `gh auth status`
structured output and `gh auth token --hostname --user`; subprocesses strip ambient GitHub token
variables. PAT/CLI configuration validates identity before atomically switching the workspace.

The resolver owns client/rate-limit instances by `(workspace, generation, principal)`. Service TTL
cache keys add that identity. Disconnect, replacement, webhook revocation, and token refresh
invalidate the applicable entries without flushing other workspaces.

### App Installation, OAuth, And Webhooks

Add `app_installation_service.go`, `personal_auth_service.go`, `oauth_flow.go`, and
`webhook_service.go`. Setup and OAuth states are random, expiring, persisted, and single-use. User
OAuth uses PKCE. An installation callback verifies that the authorizing GitHub user can access the
returned installation before binding it. Personal access/refresh token replacement is atomic and
verified against GitHub's user endpoint.

Webhook processing validates `X-Hub-Signature-256`, validates the configured App and known
installation/user identity, claims `X-GitHub-Delivery` once, and handles installation create/delete,
suspend/unsuspend, repository access changes, and user authorization revocation. Unknown valid
installation events do not create bindings.

Review-watch creation snapshots the verified effective human login into `target_login`. App-backed
polling renders an explicit review qualifier; migrated watches without a derivable target are
disabled instead of treating the App as `@me`.

### Service Routing

Remove operational use of `Service.client` and `Service.authMethod` across:

- `service_pr.go`, `service_pr_status.go`, `service_reviews.go`, `service_issues.go`,
  `service_accessible_repos.go`, `workspace_settings_service.go`, `repo_contents.go`,
  `branch_protection.go`, `gists.go`, and `service_task_issue.go`;
- `poller.go`, `service_pr_watch.go`, `service_pr_watch_batched.go`, `service_cleanup.go`,
  `service_ci_automation.go`, and `service_task_events.go`;
- GitHub call sites in `internal/orchestrator/` and `internal/automation/` that currently obtain a
  raw global client.

HTTP entry points pass the selected workspace and `default-user` through a replaceable current-user
resolver. Task/PR/watch paths derive and verify workspace ownership from persisted task relations.
Missing or contradictory ownership returns a typed error and never invokes legacy/global fallback.
Personal query/mutation paths enforce the effective repository intersection before provider calls.

### Executor Credential Broker

Replace global token injection in
`apps/backend/internal/orchestrator/executor/executor_credentials.go` with a task-scoped broker lease.
Add `apps/backend/internal/github/credential_broker.go` and a narrow authenticated broker handler.
The request includes the lease, task/repository identity, and GitHub host; the server rechecks task
workspace ownership and workspace repository scope before resolving git-transport credentials.

Add an `agentctl git-credential` helper in `apps/backend/cmd/agentctl/` that implements Git's
credential-helper protocol and obtains credentials through the task-scoped broker. Executor launch
configures `credential.helper` and the broker endpoint/lease for local, Docker, SSH, and remote
executors. Leases are stored hashed, expire with the task session, and become unusable when the
workspace credential generation changes. Remove automatic global `GITHUB_TOKEN`/`GH_TOKEN`
fallbacks. Explicit profile environment variables retain their documented higher priority and are
reported as unmanaged overrides rather than silently mixed into workspace status.

Add a PATH-prepended agentctl-backed `gh` shim that obtains a fresh automation token for each real
`gh` child invocation, sets `GH_TOKEN` only on that child, and uses an isolated `GH_CONFIG_DIR`.
Remote executor launch requires an HTTPS-reachable broker base URL; host-local loopback is allowed
only for local/worktree execution. Clone preparation fails closed when the broker is unavailable.

### HTTP, Health, And Mocks

Update `controller.go` and `handlers.go`, and add focused controllers for connections, App flow,
personal flow, webhook, and broker contracts. Keep `/token` as a one-release deprecated alias with
mandatory `workspace_id`. Add stable error codes for missing workspace, capability denial,
reconnect, invalid callback, and unavailable App configuration.

Change `internal/health` to report workspace GitHub status rather than a singleton provider. Extend
`mock_client.go` and `mock_controller.go` with isolated workspace principals, App permissions,
expiring tokens, callback state, and webhook transitions. Update `backendapp/e2e_reset.go` to clear
all new workspace/user rows and secrets deterministically.

## Frontend

### API, Types, And State

Update `apps/web/lib/api/domains/github-api.ts`, `lib/types/github.ts`, the GitHub Zustand slice, and
`hooks/domains/github/use-github-status.ts` so status is keyed by workspace. The active-workspace
change invalidates and refetches status; no previous workspace actor or rate limit remains visible.
Add API methods for CLI account selection, workspace connection replacement/disconnect, App install,
and personal connect/disconnect.

### Settings And Product Surfaces

Refactor `components/github/github-status.tsx` and `github-settings.tsx` into two un-nested sections:
**Workspace automation** and **My GitHub identity**. Automation offers PAT, named CLI account, and
App installation when deployment config is available. It labels legacy shared auth, actor
attribution, permissions, suspension/revocation, and diagnostics. Personal auth shows the verified
login, expiry/reconnect state, and the fact that agents never receive it.

Update `app/github/github-page-client.tsx`, integration menu status, PR review/merge affordances, and
repository selection to consume effective capabilities. App-only workspaces keep automation
surfaces but show a connect-personal state for `My GitHub`; manual mutations disclose the effective
actor. Use the existing icon system and responsive settings conventions. Mobile has the same
states/actions in a single-column layout with no desktop-only auth capability.

## Tests

- **Persistence/migration:** `store_connections_test.go` covers replay, legacy rows only for existing
  workspaces, new workspace disconnect, deterministic secret ownership, PR/watch workspace backfill,
  workspace deletion, and copy exclusion.
- **Resolver isolation:** `auth_resolver_test.go`, `gh_accounts_test.go`, and `factory_test.go` cover
  per-workspace PATs, exact named CLI account commands, stripped ambient env, purpose routing,
  generation invalidation, no cross-workspace fallback, and personal repository intersection.
- **App primitives:** `app_client_test.go` and `app_token_cache_test.go` cover JWT claims,
  installation verification, permission mapping, concurrent refresh coalescing, early refresh, and
  refusal to return expired tokens.
- **OAuth/webhooks:** `oauth_flow_test.go`, `personal_auth_service_test.go`, and
  `webhook_service_test.go` cover PKCE/state expiry/replay/wrong workspace, verified user and
  installation association, refresh compensation, HMAC verification, delivery dedupe, suspension,
  deletion, repository changes, and personal revocation.
- **Service integration:** existing GitHub service tests are converted to workspace-aware fixtures.
  New table-driven tests cover personal/App actor routing, background workspace derivation, missing
  ownership fail-closed behavior, capability errors, rate/cache isolation, and App-only `My GitHub`.
- **Credential broker:** executor, broker, and agentctl tests cover git protocol output, lease hashing
  and expiry, repository/workspace mismatch, generation revocation, >1-hour token refresh, the `gh`
  shim, remote broker reachability, and no App/private/personal secret in launch environments.
- **HTTP integration:** `controller_test.go` uses a real test store from handler through resolver,
  including status, connection replacement, deprecated token alias, callbacks, webhook, and stable
  error codes.
- **Frontend unit tests:** status slice/hook tests cover workspace switching; settings and integration
  menu tests cover every source/state, effective actor, capability, reconnect, and App unavailable
  state.

## E2E Tests

Extend `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts` and add
`github-authentication.spec.ts`:

- Two workspaces select different PAT/CLI mock identities and never show each other's actor, repos,
  diagnostics, or rate limits.
- Legacy auth remains visible for a seeded upgraded workspace; a new workspace is disconnected; the
  legacy option disappears after reconfiguration.
- App install callback success and invalid/replayed callbacks update only the intended workspace.
- App-only automation leaves watches available and `My GitHub` gated; connecting a personal identity
  enables personal results and human-attributed review UI.
- Suspension, revocation, missing permissions, expired user token, reconnect, and disconnect states
  render actionable errors.
- Desktop and mobile screenshots verify the two identity sections and all controls fit without
  overlap.

## Public Documentation

Update `docs/public/integrations.md` with local PAT/CLI selection, hosted App installation, actor
semantics, permissions, migration, and disconnect behavior. Update `docs/public/configuration.md`
with `KANDEV_GITHUB_APP_*` variables, secret-file guidance, callback/webhook URLs, HTTPS rules, and
the minimum GitHub App permissions/events. Reconcile relevant scoped `AGENTS.md` guidance if the
credential-provider pattern changes.

## Implementation Waves

### Wave 1: Durable ownership

- [x] [Task 01: Persistence and legacy migration](task-01-persistence-migration.md)

### Wave 2: Credential foundations (parallel)

- [x] [Task 02: GitHub App token primitives](task-02-app-token-primitives.md)
- [x] [Task 03: Workspace PAT and CLI resolver](task-03-workspace-credential-resolver.md)

Task 02 owns App/config/new token-source files. Task 03 owns legacy/PAT/CLI resolver files.

### Wave 3: App lifecycle

- [x] [Task 04: App installation and personal OAuth lifecycle](task-04-app-oauth-webhooks.md)

### Wave 4: Runtime routing (parallel)

- [x] [Task 05: Workspace-aware GitHub service routing](task-05-service-routing.md)
- [x] [Task 06: Renewable executor GitHub credentials](task-06-executor-credentials.md)

Task 05 owns GitHub domain operations and pollers. Task 06 owns the broker, executor, and agentctl
credential-helper boundary.

### Wave 5: HTTP boundary

- [x] [Task 07: Connection API, health, and mocks](task-07-http-health-mocks.md)

### Wave 6: User interface

- [x] [Task 08: Workspace and personal GitHub settings](task-08-frontend-settings.md)

### Wave 7: Product verification

- [x] [Task 09: End-to-end coverage and public docs](task-09-e2e-docs.md)
- [x] [Task 10: Integrated QA and security verification](task-10-qa-security.md)

Task 10 begins only after Task 09; it is listed in the same product-verification phase, not executed
in parallel.

## Final Verification

Formatting runs before lint and complexity checks:

```bash
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test
cd apps && rtk pnpm --filter @kandev/web lint
cd apps/web && rtk pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/integrations/github-authentication.spec.ts --project=chromium
cd apps/web && rtk pnpm exec playwright test --config e2e/playwright.config.ts e2e/tests/integrations/mobile-github-auth-settings.spec.ts --project=mobile-chrome
```

The final security pass additionally verifies that logs, API responses, executor environments,
process arguments, persisted metadata, and E2E snapshots contain no PAT, App private key, App client
secret, webhook secret, personal access token, refresh token, or live installation token.

All listed verification passed on 2026-07-19. The desktop suite passed four scenarios and the
mobile suite passed two scenarios; both captured layouts were inspected for overflow and overlap.
The full frontend suite passed 5,380 tests with four skipped, the complete backend and aggregate
repository test/lint/typecheck/build checks passed, and the security review found no blocking
issues. A real GitHub organization installation, OAuth exchange, and webhook delivery remain an
external staging validation because those flows require deployed callback URLs and GitHub-owned
credentials.

## Risks

- The current GitHub package has many direct singleton-client call sites; a compile-only migration
  can still route the wrong workspace. Every service family needs behavioral isolation tests.
- Credential refresh across local, Docker, SSH, and remote executors depends on a reachable broker
  path. Task 06 must prove each supported executor path before static App token injection is removed.
- OAuth callback correctness depends on the deployed public URL and GitHub App registration. Config
  validation and actionable diagnostics are required before enabling install buttons.
- App permission subsets vary by installation. Capability checks must be operation-specific rather
  than a single broad connected flag.

## Approval

Implementation was approved on 2026-07-19. Scope changes to identity ownership, App attribution,
legacy migration, or executor secret boundaries require updating the spec and ADR before coding.
