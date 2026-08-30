# ADR-2026-07-24-opt-in-authentication: Opt-in Authentication and Per-User Workspace Scoping

**Status:** accepted (amended 2026-08-12)
**Date:** 2026-07-24
**Area:** backend, frontend, protocol, security

## Context

Kandev shipped with no user authentication: one hardcoded `default-user` row,
a WS handshake that read and discarded its token, unfiltered hub broadcasts,
and a server that binds `0.0.0.0` by default. Shared-server deployments need
accounts and privacy between users; local single-user installs must not gain
a login screen or any behavioral change.

## Decision

1. **Opt-in via the `features.auth` runtime feature toggle**, resolved into
   `cfg.Features.Auth` at startup (env `KANDEV_FEATURES_AUTH` > DB override >
   profile default). The effective mode is *derived* from it, not persisted
   separately: `flag off → disabled`, `flag on + no admin → setup`, `flag on
   + admin exists → enabled`. Enablement lives in the existing Feature Toggles
   system — there is no bespoke `auth.mode` setting or Authentication page.
   (An earlier iteration split "reveal UI" and "enforce" into two controls;
   they were collapsed into the one flag.)
2. **Synthetic identity in disabled mode.** The global HTTP middleware (and
   WS gateway) inject `Identity{default-user, admin, Synthetic}` when auth is
   off. Downstream code branches on identity, never on mode — internal
   callers (event bus, pollers, office schedulers) carry no identity and are
   unscoped. This keeps disabled-mode behavior byte-identical and makes every
   consumer identity-aware in one step. In-session agent MCP calls arrive over
   the agent's credential-less WebSocket stream, so they initially fell into
   the same unscoped bucket; `internal/mcp/scope` now resolves the stream's
   task owner and attaches that real identity before dispatch (the owning task
   comes from the `AgentExecution`, never from the agent-supplied payload).
   Because "no identity" means full access, that path never returns an
   identity-free context under enforced auth: an unowned workspace is scoped to
   a sentinel user ID that reaches unowned rows only, and an unresolvable owner
   — deleted task/workspace row, or an account since deleted or disabled —
   denies the dispatch. Disabling a user revokes their sessions and PATs, so
   their still-running agent session must lose this surface too.
3. **Opaque DB-backed credentials, not JWTs.** Sessions (HttpOnly SameSite=Lax
   cookie, base name `kandev_session`, effective name derived from the request
   host — port-scoped on a ported host so instances on one host stay isolated)
   and personal access tokens (`kandev_pat_*`)
   are 256-bit random values stored as SHA-256 digests. Instant revocation
   (user disable, logout-all) outweighs stateless verification at kandev
   scale. The office agent HMAC-JWT remains a separate machine credential.
4. **Setup wizard promotes `default-user`.** The first account reuses the
   pre-auth user row (preserving all settings) and claims unowned workspaces
   and secrets. Rows with empty owners remain visible to everyone until
   claimed — the compatibility contract for pre-auth data.
5. **Scoping at the service layer.** The task service filters and denies by
   the ctx identity with `*NotFound` sentinels (no existence leak); admins do
   NOT bypass (admin is a management role, not a visibility role).
   Session-scoped surfaces funnel through one lifecycle chokepoint
   (`GetOrEnsureExecution`); WS subscriptions, dispatched WS RPC, and
   workspace-carrying broadcasts (`Hub.BroadcastToWorkspace`) apply the same
   rules. New global `hub.Broadcast` call sites require a `//ws:global`
   justification.
6. **Explicit public allowlist** in `auth/httpmw`: readiness probe, SPA
   shell/static, bootstrap reads (`features`, `app-state`), credential
   endpoints, and self-authenticating webhooks (automation, office channels).
   `/mcp` enforces PATs in its own group middleware; office agent JWTs pass
   through the global layer to `AgentAuthMiddleware`. Plugin webhooks
   (`/api/plugins/*/webhooks/*`) are NOT in this allowlist — see the
   2026-08-12 plugin-webhook-auth-gate ADR amendment below.

**Amendment (2026-08-12):** point 6 originally allowlisted plugin webhooks
too, on the premise that "the plugin subprocess owns signature validation."
That premise was unenforced: nothing in the manifest schema or the plugin SDK
required a plugin to authenticate its own webhook. See
`2026-08-12-plugin-webhook-auth-gate.md` for the fix — plugin webhooks now
require a real caller identity by default; a manifest opts a specific webhook
out via `webhooks[].public: true`.

## Consequences

- Multi-user servers get real accounts, invites, per-user workspaces and
  secrets, and scoped live updates; laptop installs are untouched.
- Every future user-facing service entry point must apply identity scoping
  (see `apps/backend/AGENTS.md`); the allowlist is pinned by tests.
- Filesystem isolation is NOT provided — worktrees share one `~/.kandev`
  tree. Documented limitation; per-user executor sandboxing is future work.
- OIDC/SSO can be added by inserting rows with a new provider into
  `auth_identities`; no migration required.
- Office workspace-scoped HTTP routes are ownership-gated via a group
  middleware (agent-JWT callers keep their workspace-claim scoping); office
  run subscriptions (`run.subscribe`) are now ownership-gated too, closed by
  WO-02 — see `apps/backend/AGENTS.md` and the spec.
- The session-access check is enforced at the lifecycle chokepoint AND at
  the reverse-proxy handlers (vscode/port) that resolve executions by bare
  lookup; message read/search and repository-script routes carry explicit
  scoping. These closed IDOR/read gaps found in the pre-merge security audit.
