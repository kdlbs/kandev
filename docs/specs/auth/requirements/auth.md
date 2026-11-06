---
status: active
system: auth
created: 2026-07-24
owners:
  - tbd
---
# Opt-in Authentication & Multi-User Segregation Requirements

## Overview

Kandev ships as a single-user local tool with zero authentication: the backend binds `0.0.0.0` by default and anyone who can reach the port owns the instance and every agent credential on it. Teams who share a Kandev server (VPS, homelab, office box) need user accounts and privacy between users — without burdening the local single-user install with a login screen it never asked for.

## Requirements

### REQ-AUTH-AUTH-001: Opt-in Authentication & Multi-User Segregation

**Intent:** Kandev ships as a single-user local tool with zero authentication: the backend binds `0.0.0.0` by default and anyone who can reach the port owns the instance and every agent credential on it. Teams who share a Kandev server (VPS, homelab, office box) need user accounts and privacy between users — without burdening the local single-user install with a login screen it never asked for.

#### Acceptance criteria

- **AC-AUTH-AUTH-001.1:** **Opt-in.** Authentication is OFF by default. A disabled instance behaves byte-identically to pre-auth Kandev — no login, no auth UI.
- **AC-AUTH-AUTH-001.2:** **Enablement is a runtime feature toggle.** Authentication is turned on/off through the `features.auth` runtime flag ("Authentication & users" in `Settings > System > Feature Toggles`, or `KANDEV_FEATURES_AUTH=true`) — a restart-required flag like the other feature toggles, with no separate Authentication settings page. Turning it on (and restarting) enters **setup mode**: the first visitor completes a wizard (email + password) and becomes the admin. The wizard promotes the existing single-user profile — settings, workspaces, and secrets carry over to the admin account. The effective mode is derived: `flag off → disabled`, `flag on + no admin → setup`, `flag on + admin exists → enabled`.
- **AC-AUTH-AUTH-001.3:** **Accounts.** Local email + password (argon2id). The identity schema is provider-based so OIDC/SSO can be added later without migration. Roles: `admin` (user management, auth settings, destructive system operations, feature toggles) and `member`. Users can be disabled (revokes all sessions and tokens); the last active admin cannot be demoted or disabled.
- **AC-AUTH-AUTH-001.4:** **Sessions & tokens.** Browser sessions are opaque cookies with the base session-cookie name `kandev_session` (effective name is request-host derived — see the port-scoping bullet below), HttpOnly, SameSite=Lax, sliding 30-day expiry, DB-backed and revocable from `Settings > Account`. Programmatic clients (external MCP, scripts, CI) use personal access tokens (`kandev_pat_…`, shown once, revocable).
- **AC-AUTH-AUTH-001.5:** **Cookie name is port-scoped when the instance is served on a non-default port.** Cookies ignore port when deciding scope, so two auth-enabled instances on one host (same IP, different ports) would otherwise overwrite each other's `kandev_session` and yank every instance to `/login` on the next request. The backend derives the session cookie name from the request host: `kandev_session_<port>` on a ported host, plain `kandev_session` on a default-port host (see `fix-multi-instance-cookie-isolation`). An explicit `auth.cookieName` config value is used verbatim. The legacy unprefixed cookie is deliberately not read (one re-login after upgrade keeps the cross-instance conflict dead).
- **AC-AUTH-AUTH-001.6:** **Invites.** Admins mint tokenized invite URLs (`/invite?token=…`, optional pinned email, member/admin role, 7-day default expiry, single use). No email server required. Admins can also create accounts directly.
- **AC-AUTH-AUTH-001.7:** **Segregation: per-user workspaces.** Every workspace has one owner. Users see and touch only their own workspaces — tasks, sessions, repositories, workflows, plans, walkthroughs, terminals, VS Code, port previews, git snapshots, session turns/context, secrets, **third-party integration settings (GitHub/GitLab/Jira/Linear/Sentry/Slack/Azure), automations**, and WebSocket events included. Cross-user access returns 404 — knowing an ID from another user's workspace does not grant access. Admins do NOT see other users' workspaces (hard privacy; admin is a management role, not a visibility role). Pre-auth data is claimed by the admin at setup.
- **AC-AUTH-AUTH-001.8:** **Task lifecycle mutations are ownership-checked at the service layer.** Changing a task's state, moving it between workflows/steps, updating a task repository's base branch, and archiving or deleting a task (including the cascade that walks its subtree) all refuse a foreign task before any read or write reaches the repository. This holds on every transport — HTTP, WebSocket and MCP — because the check lives below them, and it holds for long-running deletes, which detach from request cancellation but keep the caller identity.
- **AC-AUTH-AUTH-001.9:** When a completed personal access token creation result is closed and the user opens New token again, the system shall show a fresh name form without redisplaying the previous raw token, and allow another token to be created. This outcome applies on desktop and phone.
- **AC-AUTH-AUTH-001.10:** When the token creation dialog is dismissed from its form or result, reopening it shall start with an empty name and no previous error or copied indication. Done, Cancel, the close control, Escape, and outside dismissal retain their existing closing behavior.
- **AC-AUTH-AUTH-001.11:** When a session or PAT cannot be verified because a required credential, account, or organization read fails, authentication shall fail closed with HTTP 503. The response shall contain `code: persistence_unavailable`, omit `WWW-Authenticate`, and neither clear the cookie nor revoke the credential. This applies before HTTP handlers and WebSocket upgrades, including query-token authentication. A later request with the same credential shall authenticate after storage recovers.
- **AC-AUTH-AUTH-001.12:** Missing, malformed, expired, revoked, or unknown credentials shall remain invalid. Missing or disabled accounts and missing or suspended organizations shall remain invalid. When a protected request has no valid credential and no unresolved lookup error, it shall retain its existing 401 response and challenge behavior.
- **AC-AUTH-AUTH-001.13:** Session-cookie authentication shall retain precedence over bearer authentication. A successfully verified fallback bearer shall authenticate after an invalid or unreadable cookie. If no candidate succeeds and any required lookup fails, the result shall be unavailable rather than invalid. An invalid fallback shall not erase an earlier lookup error.
- **AC-AUTH-AUTH-001.14:** Failures in optional activity updates or expired-session cleanup shall not change a completed authentication decision. Such failures shall neither admit invalid credentials nor reject credentials whose required checks succeeded.
- **AC-AUTH-AUTH-001.15:** A required authentication lookup failure shall produce an operator diagnostic that identifies the operation and error category. Diagnostics and responses shall exclude raw credentials, credential hashes, cookies, authorization headers, database connection strings, and unsanitized database errors.
- **AC-AUTH-AUTH-001.16:** Disabled authentication, credentialless public endpoints, and independently authenticated requests without a failed session/PAT lookup shall retain their existing behavior. Non-PAT bearers shall retain downstream validation. Requests with an unresolved session/PAT lookup error shall receive 503 before public-path or deferred-handler processing can misclassify them as anonymous or invalid.
- **AC-AUTH-AUTH-001.17:** On desktop and phone, an in-use browser session shall retain its identity, cookie, and route after an authentication-storage 503. A subsequent successful request shall use the same session without another login. A genuine Kandev session challenge shall retain the existing sign-in transition.

## Migrated source detail

## Why

Kandev ships as a single-user local tool with zero authentication: the backend
binds `0.0.0.0` by default and anyone who can reach the port owns the instance
and every agent credential on it. Teams who share a Kandev server (VPS, homelab,
office box) need user accounts and privacy between users — without burdening
the local single-user install with a login screen it never asked for.

## What

- **Opt-in.** Authentication is OFF by default. A disabled instance behaves
  byte-identically to pre-auth Kandev — no login, no auth UI.
- **Enablement is a runtime feature toggle.** Authentication is turned on/off
  through the `features.auth` runtime flag ("Authentication & users" in
  `Settings > System > Feature Toggles`, or `KANDEV_FEATURES_AUTH=true`) — a
  restart-required flag like the other feature toggles, with no separate
  Authentication settings page. Turning it on (and restarting) enters **setup
  mode**: the first visitor completes a wizard (email + password) and becomes
  the admin. The wizard promotes the existing single-user profile — settings,
  workspaces, and secrets carry over to the admin account. The effective mode
  is derived: `flag off → disabled`, `flag on + no admin → setup`,
  `flag on + admin exists → enabled`.
- **Accounts.** Local email + password (argon2id). The identity schema is
  provider-based so OIDC/SSO can be added later without migration. Roles:
  `admin` (user management, auth settings, destructive system operations,
  feature toggles) and `member`. Users can be disabled (revokes all sessions
  and tokens); the last active admin cannot be demoted or disabled.
- **Sessions & tokens.** Browser sessions are opaque cookies with the base
  session-cookie name `kandev_session` (effective name is request-host
  derived — see the port-scoping bullet below), HttpOnly, SameSite=Lax,
  sliding 30-day expiry, DB-backed and revocable from `Settings > Account`.
  Programmatic clients (external MCP, scripts, CI) use personal access
  tokens (`kandev_pat_…`, shown once, revocable).
- **Cookie name is port-scoped when the instance is served on a non-default
  port.** Cookies ignore port when deciding scope, so two auth-enabled
  instances on one host (same IP, different ports) would otherwise overwrite
  each other's `kandev_session` and yank every instance to `/login` on the
  next request. The backend derives the session cookie name from the request
  host: `kandev_session_<port>` on a ported host, plain `kandev_session` on a
  default-port host (see `fix-multi-instance-cookie-isolation`). An explicit
  `auth.cookieName` config value is used verbatim. The legacy unprefixed
  cookie is deliberately not read (one re-login after upgrade keeps the
  cross-instance conflict dead).
- **Invites.** Admins mint tokenized invite URLs (`/invite?token=…`,
  optional pinned email, member/admin role, 7-day default expiry, single
  use). No email server required. Admins can also create accounts directly.
- **Segregation: per-user workspaces.** Every workspace has one owner. Users
  see and touch only their own workspaces — tasks, sessions, repositories,
  workflows, plans, walkthroughs, terminals, VS Code, port previews, git
  snapshots, session turns/context, secrets, **third-party integration
  settings (GitHub/GitLab/Jira/Linear/Sentry/Slack/Azure), automations**, and
  WebSocket events included. Cross-user access returns 404 — knowing an ID
  from another user's workspace does not grant access. Admins do NOT see other
  users' workspaces (hard privacy; admin is a management role, not a
  visibility role). Pre-auth data is claimed by the admin at setup.
- **Task lifecycle mutations are ownership-checked at the service layer.**
  Changing a task's state, moving it between workflows/steps, updating a task
  repository's base branch, and archiving or deleting a task (including the
  cascade that walks its subtree) all refuse a foreign task before any read or
  write reaches the repository. This holds on every transport — HTTP, WebSocket
  and MCP — because the check lives below them, and it holds for long-running
  deletes, which detach from request cancellation but keep the caller identity.
- **The WebSocket backstop reads `id` on top-level `task.*` actions.** In
  addition to `task_id` / `session_id` / `task_environment_id`, an action named
  `task.<verb>` (exactly two segments — `task.state`, `task.move`, `task.get`, …)
  has its payload `id` treated as a task ID. Deeper namespaces (`task.plan.*`,
  `task.review.*`) are untouched: their `id` names a plan, revision or finding,
  not a task.
- **Shared surfaces (instance-global by design).** Executors, executor
  profiles, environment definitions, agent profiles/agents, editors, prompts,
  sprites, voice, notification providers, and system pages remain shared
  across the instance; mutation of system settings and feature toggles is
  admin-only when auth is enabled. **Note:** a shared agent profile means all
  users share the agent's provider credentials — see the OS-level limit below.
- **Public endpoints.** `/health`, the SPA shell/static assets, the boot
  payload (`/api/v1/app-state` — returns only `{features, auth}` for
  anonymous visitors), `/api/v1/features`, credential endpoints
  (login/setup/invite-accept), and self-authenticating webhooks (automation
  `X-Webhook-Secret`, office channel HMAC). The GitHub
  credential broker (`/api/v1/github/credentials/resolve`, GET readiness +
  POST resolve) is likewise public: containers and remote executors hold no
  session cookie or PAT by design, and the opaque, task-scoped lease in the
  request body — hashed at rest, TTL'd, scope-matched on redeem — is the
  self-authenticating credential. The GitHub App webhook
  (`/api/v1/github/app/registrations/{id}/webhook`) is public for the same
  reason as the other webhooks: its own HMAC (`X-Hub-Signature-256`) is
  verified by the handler.
- **Plugin webhooks (`/api/plugins/{id}/webhooks/{key}`) are NOT
  self-authenticating by default.** The global middleware cannot read the
  plugin registry, so it structurally defers GET/POST requests for the path; `plugins.Controller`
  enforces the actual gate: a webhook requires a real caller identity
  (session/PAT, or the synthetic identity while auth is disabled) unless its
  manifest declares `webhooks[].public: true`. An anonymous caller sees 401
  for an unknown plugin ID, an undeclared key, and a declared-non-public key
  alike, so a caller with no identity cannot enumerate installed plugins.
- **Session challenges are distinct from provider authentication failures.**
  The browser clears its Kandev identity and opens `/login` only when a 401 is
  a Kandev session challenge. A third-party integration may also return 401
  when GitHub, GitLab, Jira, or Linear rejects its own credential; that failure
  remains on the current page and is rendered by the integration surface.
- **Disable.** An admin can turn auth off again (unless env-forced).
  Ownership data is retained; everyone reaching the instance has full access
  again.

## API surface

`/api/v1/auth/*`: `setup`, `login`, `logout`, `me`, `password`, `sessions`,
`tokens`, `invites` (+ `preview`, `accept`), `settings`. `/api/v1/users`
(admin CRUD: list, create, role/status). WS: cookie on same-origin upgrade or
`?token=<PAT>`; subscriptions and RPC actions are scoped to the caller.

## Failure modes

- Wrong credentials and unknown emails return the same generic 401; login is
  rate-limited (10 attempts / 5 min per IP+email).
- Foreign workspaces/tasks read as 404 — existence is not leaked.
- A server bound to non-loopback interfaces with auth disabled logs a
  prominent startup warning.
- Sessions/PATs of disabled users fail closed immediately.
- Third-party provider authentication failures do not clear the authenticated
  Kandev user, replace the current route, or trigger a login redirect.

## Scenarios

- **GIVEN** two users A and B and a task owned by B, **WHEN** A sends
  `task.state` or `task.move` over the WebSocket naming that task by `id`,
  **THEN** the call is refused, B's task DTO is not serialized into the
  response, and no state, position or workflow write reaches the repository.
- **GIVEN** the same pair, **WHEN** A sends `DELETE /api/v1/tasks/{B's task}`
  or `POST /api/v1/tasks/{B's task}/archive`, **THEN** the response is 404 and
  no task row, subtree row, or workspace-group membership is touched — while
  the owner's own delete still runs to completion if the owner's browser
  disconnects mid-request.
- **GIVEN** an authenticated Kandev browser session, **WHEN** a protected API
  request receives a Kandev session challenge, **THEN** the browser clears the
  stale Kandev identity and navigates to `/login`.
- **GIVEN** two auth-enabled Kandev instances A (port 8080) and B (port
  8081) on the same host, **WHEN** the user logs into B, **THEN** A's session
  keeps working and A's next page load authenticates with A's own
  `kandev_session_8080` cookie rather than showing the Sign in page.
- **GIVEN** an authenticated Kandev browser session and an expired or invalid
  third-party integration credential, **WHEN** GitHub, GitLab, Jira, or Linear
  data loading returns 401 without a Kandev session challenge, **THEN** the
  browser stays on the integration route and displays the provider loading
  error.

## Known v1 limits

- **Filesystem / agent-credential isolation is NOT enforced.** Worktrees and
  repos live under one `~/.kandev` tree readable by the OS user running the
  backend, and agent CLI logins (`gh auth`, `claude login`, provider API keys)
  authenticate as that OS user — so all app-users share the same on-disk agent
  credentials. Application-layer isolation (DB + the checks above) is the
  boundary between users' *kandev data*; it does NOT sandbox the filesystem or
  per-user agent auth. For hard isolation of agent credentials, run separate
  kandev instances per user or use per-user OS accounts / sandboxed executors
  (future work).
- No workspace sharing/membership — one owner per workspace.
- No first-party OIDC/SSO. The host supports **plugin-provided** external login
  (an `auth`-capable plugin asserts a validated OIDC/SAML identity and the host
  mints the session — see ADR 0050); the pre-auth login-page SSO buttons and
  anonymous provider discovery (`auth.ssoProviders` in the boot payload) ship
  here too. What remains is a published IdP plugin (e.g. Google OIDC, developed
  in its own repo) and setup-mode first-admin bootstrap via SSO (SSO requires
  enforced auth today).
- Office workspace-scoped HTTP routes (those carrying a `:wsId`) are
  ownership-checked when auth is enabled; office run *subscriptions* are not
  yet ownership-checked (run events carry no workspace context at the
  subscription layer).
