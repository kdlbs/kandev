---
title: "Authentication & Users"
description: "Enable opt-in authentication, manage users and invites, and use personal access tokens on a shared Kandev server."
---

# Authentication & Users

Kandev ships as a single-user local tool with authentication **disabled**: nothing changes for laptop installs. When several people share one Kandev server, enable authentication to give each person their own account and their own private workspaces.

Authentication is a **runtime feature toggle**: the same system as the other feature flags, so there is no separate "Authentication" configuration page.

## Quick checklist

1. Keep authentication off only for a single-user laptop bound to loopback or another private listener.
2. For any server reachable by other users or networks, enable **Authentication & users** and restart.
3. Create the first admin immediately after setup mode appears.
4. Use personal access tokens for scripts and external MCP clients.
5. Keep TLS, filesystem permissions, and agent credentials as separate security boundaries.

## What changes when authentication is on

- Everyone signs in with email + password. Browser sessions last 30 days (sliding) and can be revoked from `Settings > Account`. The signed-in user is shown in the bottom-left of the sidebar, with a log-out menu.
- **Workspaces become per-user by default.** You only see workspaces you own, including their tasks, sessions, repositories, terminals, previews, and live updates. Existing data is assigned to the admin created during setup. A workspace can then be shared with colleagues on purpose: see [Team Access](team-access.md).
- Secrets are per-user. A **Global** secret is user-global across that user's workspaces; a **Workspace** secret belongs to one of their workspaces. With authentication disabled, Global is install-global. Executors and agent profiles remain shared across the instance, so they can reference Global secrets only; repositories may bind Global or same-workspace secrets.
- Admins manage users and instance settings, but do **not** see other users' private workspaces. Admin is a management role, not a visibility role.
- Programmatic clients (external MCP, scripts) authenticate with personal access tokens.

## Sharing work with a team

Authentication makes each person's work private. To give a team a shared board without a shared login, see [Team Access](team-access.md): workspace visibility, member roles, and scoped permissions.

## Enabling authentication

**From the UI:** open `Settings > System > Feature Toggles`, turn on **Authentication & users**, and restart Kandev when prompted (it is a restart-required flag, like the other feature toggles). After the restart the instance is in setup mode: the setup wizard appears and the account you create becomes the admin. All existing workspaces, settings, and secrets carry over to that admin.

**From the environment** (fresh servers, Docker, Kubernetes): set

```bash
KANDEV_FEATURES_AUTH=true
```

The instance boots into setup mode and the **first visitor** creates the admin account. Complete the wizard immediately after deploying. When the flag is forced on by the environment variable, it cannot be turned off from the UI.

To turn authentication **off** again, an admin flips the same toggle off (and restarts), or the environment variable is unset. Ownership data is retained.

A server that listens on non-loopback interfaces with authentication disabled logs a startup warning.

## Users and invites

`Settings > System > Users` (admin only):

- **Invite links**: mint a tokenized URL (`/invite?token=…`) and share it out of band. Optional pinned email, member or admin role, single use, 7-day default expiry. No email server needed.
- **Direct creation**: create an account with a password yourself.
- **Disable / role changes**: disabling a user immediately revokes their sessions and tokens. The last active admin cannot be demoted or disabled.

Roles: `admin` (user management, authentication settings, destructive system operations, feature toggles) and `member` (everything else, scoped to their own workspaces).

## Personal access tokens

`Settings > Account > API Tokens`. Tokens look like `kandev_pat_…`, are shown **once** at creation, and are sent as a bearer header:

```bash
curl -H "Authorization: Bearer kandev_pat_..." https://kandev.example.com/api/v1/workspaces
```

External MCP clients (Claude Code, Cursor connecting to `/mcp`) must be configured with a PAT once authentication is enabled. WebSocket clients that cannot send headers can pass `?token=<PAT>` on the connection URL.

## Endpoints that stay public

`/health` (readiness probes), the login/setup/invite pages, `GET /api/v1/features`, and self-authenticating webhook receivers (automation webhooks with `X-Webhook-Secret`, office channel HMAC webhooks, plugin webhooks). Everything else requires a session or token.

## Multiple instances on one host

Browsers match cookies by host and ignore the port, so several auth-enabled kandev instances on the same host (same IP, different ports) would otherwise share one cookie jar. Kandev isolates them by port-scoping its instance-identity cookie **names**: `kandev_session_<port>`, `kandev-active-workspace_<port>`, and `office-active-workspace_<port>` on a ported host, plain names on a default-port host. Default-port normalization is scheme-aware and mirrors the SPA's `URL.port` behavior: `:80` on HTTP and `:443` on HTTPS yield no suffix (browsers omit them from `Host`), while the scheme-mismatched combinations (HTTP `:443`, HTTPS `:80`) keep their port suffix on both sides, so `example.com:80` and `example.com` resolve to the same plain names and two HTTP instances on ports 80 and 443 stay isolated. Logging into one instance no longer logs the others out, and selecting a workspace in one no longer changes what the others boot into.

**Reverse proxies must preserve the browser hostname.** The CORS/WS origin gate compares the browser `Origin` hostname with the request `Host` and ignores `X-Forwarded-Host` and ports. The proxy may either preserve the full `Host` (`public.example:8443`) or rewrite only the port and forward a correct `X-Forwarded-Host` (the cookie resolver takes the public port from it; the header is honored only from peers listed in `KANDEV_TRUSTED_PROXIES`, the same list that gates `X-Forwarded-For`). Rewriting a **non-loopback hostname** is rejected with 403 before authentication. Loopback-alias rewrites (e.g. `localhost` → `127.0.0.1`) pass the gate by design.

**Session-cookie migration.** Old auth-enabled builds conflict with each other via the shared unprefixed `kandev_session`; an upgraded instance ignores the legacy session token and requires one re-login (the new build never reads the unprefixed session cookie, so this holds on rollback and re-upgrade too). Workspace selections keep their validated legacy read fallback, so a pre-upgrade selection survives.

A custom `auth.cookieName` (see [configuration](configuration.md#authentication-office-plugins-and-feature-flags)) disables automatic port isolation and must be unique per cookie host. In particular, an explicit `auth.cookieName: kandev_session` (copied from the pre-isolation default) silently keeps the old shared-name behavior after upgrade: remove the setting (or any other value equal to the base name) to inherit port isolation. A custom name does not change the origin gate, which compares hostnames independently of cookie names. Instances served on the same host at default ports over different schemes (HTTP `:80` + HTTPS `:443`) carry no port in their Host and keep the plain names; they are not isolated by this mechanism.

## What is isolated

When authentication is on, everything in a workspace is private to its owner and returns "not found" to anyone else, even if they know the ID: workspaces, tasks, workflows, sessions, plans, walkthroughs, terminals, VS Code, port previews, git snapshots, Workspace secrets, repository bindings, **and the workspace's third-party integration settings (GitHub/GitLab/Jira/Linear/Sentry/Azure) and automations**. A user's Global secrets are also private to that user. Admins manage users but do not see other users' workspaces or secrets.

Shared across the instance (by design): executors, agent profiles, environments, editors, prompts, and system pages.

## Limitations

- **Filesystem and agent credentials are not isolated.** Worktrees and repositories live under one `~/.kandev` tree owned by the OS user running the backend, and agent CLI logins (`gh auth`, `claude login`, provider API keys) authenticate as that OS user, so all app-users share the same on-disk agent credentials, and anyone with shell access to the server can read all files. Authentication isolates users' kandev *data* at the application layer, not the filesystem or per-user agent auth. For hard isolation of agent credentials, run a separate kandev instance per user (or use OS-level access control / sandboxed executors).
- One owner per workspace, no sharing or team workspaces yet.
- Local accounts only for now; the account model is ready for OIDC/SSO later.
- Authentication does not replace TLS. Terminate HTTPS in front of Kandev (the session cookie is marked `Secure` when the request arrives over TLS or `X-Forwarded-Proto: https`).
