---
title: "Authentication & Users"
description: "Enable opt-in authentication, manage users and invites, and use personal access tokens on a shared Kandev server."
---

# Authentication & Users

Kandev ships as a single-user local tool with authentication **disabled** — nothing changes for laptop installs. When several people share one Kandev server, enable authentication to give each person their own account and their own private workspaces.

> Feature flag: the authentication settings surfaces are gated by the `auth` feature toggle (`KANDEV_FEATURES_AUTH`) while the feature rolls out. Enforcement itself is controlled by the steps below, not by the flag.

## What changes when authentication is on

- Everyone signs in with email + password. Browser sessions last 30 days (sliding) and can be revoked from `Settings > Account`.
- **Workspaces become per-user.** You only see workspaces you own — including their tasks, sessions, repositories, terminals, previews, and live updates. Existing data is assigned to the admin created during setup.
- Secrets are per-user. Executors, agent profiles, environments, and integration configuration remain shared across the instance.
- Admins manage users and instance settings, but do **not** see other users' workspaces.
- Programmatic clients (external MCP, scripts) authenticate with personal access tokens.

## Enabling authentication

**From the UI:** open `Settings > System > Authentication` and turn it on. You are taken to the setup wizard — the account you create becomes the admin, and all existing workspaces, settings, and secrets carry over to it.

**From the environment** (fresh servers, Docker, Kubernetes): set

```bash
KANDEV_AUTH_REQUIRED=true
```

The instance boots into setup mode: the **first visitor** creates the admin account. Complete the wizard immediately after deploying. With `KANDEV_AUTH_REQUIRED=true`, authentication cannot be disabled from the UI.

A server that listens on non-loopback interfaces with authentication disabled logs a startup warning.

## Users and invites

`Settings > System > Users` (admin only):

- **Invite links** — mint a tokenized URL (`/invite?token=…`) and share it out of band. Optional pinned email, member or admin role, single use, 7-day default expiry. No email server needed.
- **Direct creation** — create an account with a password yourself.
- **Disable / role changes** — disabling a user immediately revokes their sessions and tokens. The last active admin cannot be demoted or disabled.

Roles: `admin` (user management, authentication settings, destructive system operations, feature toggles) and `member` (everything else, scoped to their own workspaces).

## Personal access tokens

`Settings > Account > API Tokens`. Tokens look like `kandev_pat_…`, are shown **once** at creation, and are sent as a bearer header:

```bash
curl -H "Authorization: Bearer kandev_pat_..." https://kandev.example.com/api/v1/workspaces
```

External MCP clients (Claude Code, Cursor connecting to `/mcp`) must be configured with a PAT once authentication is enabled. WebSocket clients that cannot send headers can pass `?token=<PAT>` on the connection URL.

## Endpoints that stay public

`/health` (readiness probes), the login/setup/invite pages, `GET /api/v1/features`, and self-authenticating webhook receivers (automation webhooks with `X-Webhook-Secret`, office channel HMAC webhooks, plugin webhooks). Everything else requires a session or token.

## Limitations

- **Filesystem isolation is not enforced.** Worktrees and repositories live under one `~/.kandev` tree owned by the OS user running the backend. Authentication isolates users at the application layer; anyone with shell access to the server can read all files. Combine with OS-level access control for hard isolation.
- One owner per workspace — no sharing or team workspaces yet.
- Local accounts only for now; the account model is ready for OIDC/SSO later.
- Authentication does not replace TLS. Terminate HTTPS in front of Kandev (the session cookie is marked `Secure` when the request arrives over TLS or `X-Forwarded-Proto: https`).

## Disabling again

An admin can turn authentication off in `Settings > System > Authentication` (unless `KANDEV_AUTH_REQUIRED` forces it). Data and ownership are retained; everyone who can reach the instance has full access again.
