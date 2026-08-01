---
title: "Authentication & Users"
description: "Enable opt-in authentication, manage users and invites, and use personal access tokens on a shared Kandev server."
---

# Authentication & Users

Kandev ships as a single-user local tool with authentication **disabled** — nothing changes for laptop installs. When several people share one Kandev server, enable authentication to give each person their own account and their own private workspaces.

Authentication is a **runtime feature toggle** — the same system as the other feature flags — so there is no separate "Authentication" configuration page.

## Quick checklist

1. Keep authentication off only for a single-user laptop bound to loopback or another private listener.
2. For any server reachable by other users or networks, enable **Authentication & users** and restart.
3. Create the first admin immediately after setup mode appears.
4. Use personal access tokens for scripts and external MCP clients.
5. Keep TLS, filesystem permissions, and agent credentials as separate security boundaries.

## What changes when authentication is on

- Everyone signs in with email + password. Browser sessions last 30 days (sliding) and can be revoked from `Settings > Account`. The signed-in user is shown in the bottom-left of the sidebar, with a log-out menu.
- **Workspaces become per-user.** You only see workspaces you own — including their tasks, sessions, repositories, terminals, previews, and live updates. Existing data is assigned to the admin created during setup.
- Secrets are per-user. Executors, agent profiles, environments, and integration configuration remain shared across the instance.
- Admins manage users and instance settings, but do **not** see other users' workspaces.
- Programmatic clients (external MCP, scripts) authenticate with personal access tokens.

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

## What is isolated

When authentication is on, everything in a workspace is private to its owner and returns "not found" to anyone else — even if they know the ID: workspaces, tasks, workflows, sessions, plans, walkthroughs, terminals, VS Code, port previews, git snapshots, secrets, **and the workspace's third-party integration settings (GitHub/GitLab/Jira/Linear/Sentry/Slack/Azure) and automations**. Admins manage users but do not see other users' workspaces.

Shared across the instance (by design): executors, agent profiles, environments, editors, prompts, and system pages.

## Limitations

- **Filesystem and agent credentials are not isolated.** Worktrees and repositories live under one `~/.kandev` tree owned by the OS user running the backend, and agent CLI logins (`gh auth`, `claude login`, provider API keys) authenticate as that OS user — so all app-users share the same on-disk agent credentials, and anyone with shell access to the server can read all files. Authentication isolates users' kandev *data* at the application layer, not the filesystem or per-user agent auth. For hard isolation of agent credentials, run a separate kandev instance per user (or use OS-level access control / sandboxed executors).
- One owner per workspace — no sharing or team workspaces yet.
- Local accounts only for now; the account model is ready for OIDC/SSO later.
- Authentication does not replace TLS. Terminate HTTPS in front of Kandev (the session cookie is marked `Secure` when the request arrives over TLS or `X-Forwarded-Proto: https`).
