---
status: building
created: 2026-07-24
owner: tbd
---

# Opt-in Authentication & Multi-User Segregation

## Why

Kandev ships as a single-user local tool with zero authentication: the backend
binds `0.0.0.0` by default and anyone who can reach the port owns the instance
and every agent credential on it. Teams who share a Kandev server (VPS, homelab,
office box) need user accounts and privacy between users — without burdening
the local single-user install with a login screen it never asked for.

## What

- **Opt-in.** Authentication is OFF by default. A disabled instance behaves
  byte-identically to pre-auth Kandev — no login, no visible auth UI beyond
  the settings toggle (gated by the `auth` feature flag).
- **Enablement.** An admin enables auth from `Settings > System >
  Authentication`, or a deployment sets `KANDEV_AUTH_REQUIRED=true`. Either
  path enters **setup mode**: the first visitor completes a wizard
  (email + password) and becomes the admin. The wizard promotes the existing
  single-user profile — settings, workspaces, and secrets carry over to the
  admin account.
- **Accounts.** Local email + password (argon2id). The identity schema is
  provider-based so OIDC/SSO can be added later without migration. Roles:
  `admin` (user management, auth settings, destructive system operations,
  feature toggles) and `member`. Users can be disabled (revokes all sessions
  and tokens); the last active admin cannot be demoted or disabled.
- **Sessions & tokens.** Browser sessions are opaque cookies
  (`kandev_session`, HttpOnly, SameSite=Lax, sliding 30-day expiry,
  DB-backed and revocable from `Settings > Account`). Programmatic clients
  (external MCP, scripts, CI) use personal access tokens
  (`kandev_pat_…`, shown once, revocable).
- **Invites.** Admins mint tokenized invite URLs (`/invite?token=…`,
  optional pinned email, member/admin role, 7-day default expiry, single
  use). No email server required. Admins can also create accounts directly.
- **Segregation: per-user workspaces.** Every workspace has one owner. Users
  see and touch only their own workspaces — tasks, sessions, repositories,
  workflows, terminals, previews, and WebSocket events included. Admins do
  NOT see other users' workspaces (hard privacy; admin is a management role,
  not a visibility role). Secrets are likewise per-user. Pre-auth data is
  claimed by the admin at setup.
- **Shared surfaces.** Executors, agent profiles, environments, integrations
  configuration, and system pages remain instance-global; mutation of system
  settings and feature toggles is admin-only when auth is enabled.
- **Public endpoints.** `/health`, the SPA shell/static assets, the boot
  payload (`/api/v1/app-state` — returns only `{features, auth}` for
  anonymous visitors), `/api/v1/features`, credential endpoints
  (login/setup/invite-accept), and self-authenticating webhooks (automation
  `X-Webhook-Secret`, office channel HMAC, plugin webhooks).
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

## Known v1 limits

- Filesystem isolation is NOT enforced: worktrees and repos live under one
  `~/.kandev` tree readable by the OS user running the backend. DB-level
  isolation is the boundary; per-user executor sandboxing is future work.
- No workspace sharing/membership — one owner per workspace.
- No OIDC/SSO yet (schema is ready).
- Office workspace-scoped HTTP routes (those carrying a `:wsId`) are
  ownership-checked when auth is enabled; office run *subscriptions* are not
  yet ownership-checked (run events carry no workspace context at the
  subscription layer).
