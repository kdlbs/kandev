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

The original workspace-authentication buildout (Tasks 01-10) is complete. This extension adds
self-hosted deployment App registration through GitHub's App Manifest flow (Tasks 11-16) without
changing the shipped workspace/personal routing model.

## Architecture

- Product contract: `docs/specs/integrations/github-authentication.md`.
- Ownership decision: `docs/decisions/0047-github-authentication-ownership.md`.
- Managed registration amendment:
  `docs/decisions/2026-07-20-managed-github-app-registration.md`.
- Existing integration ownership: `docs/decisions/0030-workspace-scoped-integration-settings.md`.
- `github.CredentialResolver.Resolve(ctx, Request)` is the only source of operational clients. A
  request carries `WorkspaceID`, optional `UserID`, purpose, and optional repository target; a
  resolution returns `Client`, principal metadata, capabilities, credential generation, and expiry.
- Automation purpose never resolves a personal credential. Personal purpose resolves the current
  user's App token, then a human PAT/CLI automation connection. Manual mutation may finally fall
  back to an App installation with explicit App attribution.
- One deployment App registration resolves from complete environment configuration first, then an
  encrypted persisted registration. Workspace and personal APIs receive only availability and
  non-secret metadata; System Settings owns registration creation and removal.
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

### Managed Deployment App Registration

Extend `apps/backend/internal/github/models.go` and `store.go`, with focused implementation in new
`deployment_app_store.go` and `deployment_app_config.go`. Add singleton
`github_app_registration` metadata and deployment-scoped `github_app_registration_flows`. Store the
private key, client secret, and webhook secret in one versioned encrypted bundle at
`github:deployment-app:credentials`. Source resolution is `environment > managed > none`; any
partial environment override is authoritative but invalid, so it never falls through to persisted
credentials. Metadata/secret writes use rollback compensation and never replace the active runtime
until both are durable.

Add `deployment_app_manifest.go` and `deployment_app_registration_service.go`. The manifest policy
contains the existing minimum permissions/events and derives registration, installation, personal
OAuth, and webhook URLs from one canonical GitHub.com HTTPS origin. The flow hashes one-hour state,
POSTs to the personal or organization GitHub registration endpoint, exchanges the returned code at
`POST /app-manifests/{code}/conversions`, verifies the returned App identity/config, and hot-swaps an
immutable runtime generation. Public-origin validation rejects credentials, query/fragment,
loopback, and private/link-local literals; it does not fetch an arbitrary operator URL.

Update `apps/backend/internal/backendapp/services.go` so boot resolves the same runtime bundle used
by registration callbacks. Refactor `ConfigureGitHubAppAuth` in `service_app_auth.go` into a
generation-safe replace operation that updates App client, installation auth, personal auth,
webhook verifier, and credential-resolver user provider together. Environment-managed state is
reported read-only. Managed deletion is blocked while any `github_workspace_connections` row uses
`github_app_installation`. Invalid webhook signatures never mutate health; only post-signature
processing failures for the active generation or App-JWT-authenticated GitHub delivery status may
mark webhook health failing.

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

Add deployment registration routes to `controller.go` with handlers in
`controller_app_registration.go`: status, start, public callback, and managed deletion. The callback
redirects to `/settings/system/github-app` with a non-secret result code. Extend `mock_controller.go`
and E2E reset behavior with deterministic unconfigured, environment-managed, registering, ready,
invalid, and webhook-health states. Do not put registration secrets or conversion codes in status,
logs, redirects, mock snapshots, or errors.

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

Add `/settings/system/github-app` to `apps/web/src/settings-routes.tsx`, the System settings sidebar,
and the corresponding app route. `components/settings/system/github-app-settings.tsx` renders one
deployment-level surface: identity model comparison, environment-managed status, or a guided managed
setup. The setup asks where the App should be owned (personal account or organization), requires an
organization login when selected, validates/normalizes the public URL, previews permissions behind a
button/dialog, and uses a deliberate external-navigation confirmation before form-POSTing the
manifest to GitHub. Completed callbacks show registration and webhook health plus **Install in a
workspace** guidance; no secret value is rendered.

Update `components/github/github-connection-dialog.tsx` so an unavailable App is an actionable
method instead of a disabled unexplained option. It explains that PAT/CLI are human workspace
identities while an App is deployment-owned automation, and links unconfigured operators to System
Settings. A ready deployment retains the existing installation action. Personal identity remains
visible only when workspace automation uses the App.

### Mobile Design Contract

- **Entry/outcome:** desktop and mobile enter through **System > GitHub App**; both can complete the
  same registration and return to the same health state.
- **Nearest exemplar:** use the existing single-column System settings pages and the GitHub mobile
  connection dialog; reuse their route navigation, inset spacing, and 44px controls.
- **Hierarchy:** deployment status first, App-versus-personal explanation second, owner/public URL
  form third, permission disclosure and primary create action last.
- **Presentation:** direct System route, not a nested desktop dialog. GitHub's owner confirmation is
  external navigation; the workspace connection surface remains a dialog.
- **Scrolling:** the settings content region is the only scroll owner. The page has no fixed footer,
  respects safe-area padding supplied by the settings shell, and has no horizontal overflow.
- **Shared logic:** API state, validation messages, manifest submission, and callback result parsing
  are shared; only responsive layout classes change.
- **Proof:** a `mobile-chrome` Playwright scenario starts setup, validates the form, observes the
  mocked callback result, and follows the workspace handoff with all targets at least 44px.

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
- **Managed registration persistence:** `deployment_app_store_test.go` covers singleton replay,
  encrypted bundle compensation, generation replacement, source precedence, invalid partial env,
  restart rehydration, and deletion blocked by App-backed workspaces.
- **Manifest protocol:** `deployment_app_manifest_test.go` covers exact permission/event manifest,
  personal/organization URLs, canonical public URL rules, state expiry/replay, conversion response
  bounds, and no secrets in errors.
- **Runtime/API integration:** `deployment_app_registration_service_test.go` and
  `controller_app_registration_test.go` use real test stores from start through callback and runtime
  availability, including conversion rollback and environment read-only behavior.
- **Frontend unit tests:** `github-app-settings.test.tsx`, `github-connection-dialog.test.tsx`, and
  GitHub API tests cover setup explanations, source states, owner validation, public URL errors,
  permission disclosure, callback outcomes, and the System Settings handoff.

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

Add `apps/web/e2e/tests/settings/github-app-registration.spec.ts` and
`mobile-github-app-registration.spec.ts`:

- An unconfigured deployment explains App automation versus personal identity, validates owner and
  public origin, and submits the generated manifest to the correct mocked GitHub owner endpoint.
- A successful callback hot-enables **Install GitHub App** in workspace settings without a backend
  restart; invalid/replayed callbacks do not change the active registration.
- Environment-managed registration is labeled read-only; managed deletion is blocked while a
  workspace installation binding exists.
- Desktop and Pixel 5 screenshots verify the direct settings page, permission dialog, callback
  result, 44px controls, internal scrolling, and zero document horizontal overflow.

## Public Documentation

Update `docs/public/integrations.md` with local PAT/CLI selection, hosted App installation, actor
semantics, permissions, migration, and disconnect behavior. Update `docs/public/configuration.md`
with `KANDEV_GITHUB_APP_*` variables, secret-file guidance, callback/webhook URLs, HTTPS rules, and
the minimum GitHub App permissions/events. Reconcile relevant scoped `AGENTS.md` guidance if the
credential-provider pattern changes.

Extend those pages with the System Settings manifest workflow, owner choice, public HTTPS and local
tunnel/reverse-proxy guidance, environment precedence/read-only behavior, webhook verification,
safe removal rules, GitHub.com-only scope, and the distinction between deployment App, workspace
installation, and personal identity. Link the official GitHub manifest and setup-URL documentation.

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

### Wave 8: Managed registration foundations (parallel)

- [x] [Task 11: Deployment App persistence and source resolution](task-11-deployment-app-persistence.md)
- [x] [Task 12: GitHub App manifest protocol](task-12-app-manifest-protocol.md)

Task 11 owns store/config source files. Task 12 owns new pure manifest, URL-validation, and
conversion-client files; it does not edit shared store/config files.

### Wave 9: Runtime and HTTP integration

- [x] [Task 13: Deployment App runtime and API](task-13-deployment-app-runtime-api.md)

### Wave 10: System and workspace UX

- [x] [Task 14: GitHub App onboarding UI](task-14-github-app-onboarding-ui.md)

### Wave 11: Product verification

- [x] [Task 15: End-to-end coverage and public docs](task-15-onboarding-e2e-docs.md)
- [x] [Task 16: Integrated security and QA](task-16-onboarding-security-qa.md)

Task 16 starts only after Task 15 and owns no production implementation files.

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
cd apps/web && rtk pnpm e2e:run tests/settings/github-app-registration.spec.ts -- --project=chromium
cd apps/web && rtk pnpm e2e:run --no-build tests/settings/mobile-github-app-registration.spec.ts -- --project=mobile-chrome
```

The final security pass additionally verifies that logs, API responses, executor environments,
process arguments, persisted metadata, and E2E snapshots contain no PAT, App private key, App client
secret, webhook secret, personal access token, refresh token, or live installation token.

The original Tasks 01-10 verification passed on 2026-07-19. Tasks 11-16 passed final verification on
2026-07-20: format, generated metadata, typecheck, the complete backend suite, 719 web test files
(5,614 passed and four skipped), 30 CLI test files (280 passed), script tests, full lint, and
`git diff --check`. The new onboarding suites passed four desktop and three mobile scenarios, and
their captured layouts were inspected for overflow and overlap. An independent security re-review
confirmed all four initial findings were resolved with regression coverage and found no remaining
blocker. A real GitHub organization registration/installation, OAuth exchange, and webhook delivery
remain external staging validation because those flows require deployed callback URLs and
GitHub-owned credentials; Task 16 records the checklist.

## Risks

- The current GitHub package has many direct singleton-client call sites; a compile-only migration
  can still route the wrong workspace. Every service family needs behavioral isolation tests.
- Credential refresh across local, Docker, SSH, and remote executors depends on a reachable broker
  path. Task 06 must prove each supported executor path before static App token injection is removed.
- OAuth callback correctness depends on the deployed public URL and GitHub App registration. Config
  validation and actionable diagnostics are required before enabling install buttons.
- App permission subsets vary by installation. Capability checks must be operation-specific rather
  than a single broad connected flag.
- GitHub's manifest conversion returns the only copy of the generated private key. A persistence
  failure after conversion can leave an orphan App on GitHub, so the callback must clearly report
  recovery steps without claiming Kandev can delete an unpersisted App.
- The current runtime has no authenticated system-admin role. Treating `default-user` as operator is
  acceptable only under the explicitly approved trusted-single-user deployment boundary.
- GitHub Enterprise Server and enterprise-owned Apps are not covered by GitHub.com's manifest
  contract and must not be inferred from a configurable host field.

## Approval

Tasks 01-10 were approved and completed on 2026-07-19. On 2026-07-20, the user approved the amended
spec, accepted ADR, wave graph, verification commands, trusted-single-user `default-user` operator
boundary, and GitHub.com-only scope for Tasks 11-16.
