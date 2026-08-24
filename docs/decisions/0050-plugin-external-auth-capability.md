# 0050 — Plugins provide OIDC/SAML login via a capability-gated, host-minted session

- Status: accepted (amended 2026-08-12)
- Date: 2026-07-26
- Area: backend, security, protocol
- Related: [0043 — Plugin host data API](0043-plugin-host-data-api.md),
  [Opt-in authentication](2026-07-24-opt-in-authentication.md),
  [docs/specs/auth/requirements/auth.md](../specs/auth/requirements/auth.md),
  [docs/specs/plugins/requirements/plugins.md](../specs/plugins/requirements/plugins.md)

## Context

Kandev's opt-in auth (ADR 2026-07-24) ships a provider-based identity schema —
`auth_identities(provider, subject)` with `UNIQUE(provider, subject)` — and says
"OIDC/SSO can be added by inserting rows with a new provider … no migration
required." What it did *not* provide was a way for a **plugin** to do that
insertion and establish a session, so an external IdP (OIDC/SAML) could be
delivered as an out-of-tree plugin rather than baked into the host.

The plugin capability model (ADR 0043) is a set of capability-gated `Host` gRPC
RPCs the plugin *calls*, plus host→plugin events and a proxied inbound webhook
(`/api/plugins/{id}/webhooks/{key}`, reachable by an anonymous visitor when its
manifest entry declares `webhooks[].public: true` — see the 2026-08-12
amendment below). None of these can establish an authenticated browser
session.

The architectural constraint that shaped this decision: the session cookie
(name derived from the request host — base name `kandev_session`) must be set
on the **HTTP response to the browser**. A `Host` gRPC RPC runs on the
go-plugin broker channel, not the browser's HTTP connection, so it *cannot*
set that cookie. The only plugin→browser HTTP path is the webhook response.
Therefore session establishment must happen on the webhook response path,
regardless of whether a new RPC exists.

## Decision

Add an `auth` plugin capability and a **capability-gated login directive on the
plugin webhook response**. An auth-capable plugin, after validating an IdP
token/assertion in its webhook handler (the OIDC callback / SAML ACS), returns
a reserved header `X-Kandev-Auth-Login` whose value is a JSON assertion
`{provider, subject, email, display_name}`. The host webhook relay:

1. **Always consumes and strips** the directive header, so a plugin-controlled
   value can never reach the browser.
2. Interprets it **only when the plugin declares `capabilities.auth`** — a
   plugin that emits the directive without the capability is rejected (403),
   not silently ignored.
3. Calls `auth.Service.AuthenticateExternal`, which maps the assertion to a
   user (existing identity → link-by-email → JIT-provision a **member**),
   mints a session, and the host sets the session cookie itself. The
   email-linking step is trust-sensitive: the plugin **MUST** only assert an
   email the IdP verified as owned by the subject, otherwise a spoofed email
   claim is an account takeover. As defense-in-depth the host **refuses to
   auto-link to an admin account** (`ErrSSOAdminLinkForbidden`) — new users are
   members and an admin can only be reached through a link deliberately
   established while they were a member — but it cannot itself distinguish a
   verified email from an unverified one; that guarantee is the plugin's.
4. **Drops any plugin-supplied `Set-Cookie`** header on the webhook relay, so a
   plugin cannot overwrite the minted session cookie or fixate any other.

The plugin **never receives the raw session token**. Its authority is limited
to *asserting who the user is*; the host owns minting, cookie flags, and the
session lifecycle. `AuthenticateExternal` only operates in `ModeEnabled`, never
creates or elevates an admin, and rejects disabled accounts. Sessions are the
existing DB-backed opaque cookies, so per-user scoping, revocation, and
user-disable apply to SSO sessions unchanged.

We deliberately did **not** add a `Host.AuthenticateExternalIdentity` RPC that
returns the token to the plugin: it cannot set the browser cookie anyway, and
handing the raw session token to the plugin is strictly weaker.

## Consequences

- OIDC/SAML ships as an out-of-tree plugin using only the existing webhook
  transport; no proto/gRPC contract change, no schema migration.
- The `auth` capability is the highest-privilege capability in the system — a
  plugin holding it can log a visitor in as any (or a new) user. It must only
  be granted to operator-trusted plugins; treat it like admin install.
- Dropping plugin `Set-Cookie` on the webhook relay is a small behavior change
  for existing webhook plugins (they had no legitimate reason to set cookies).
- **Login-screen discovery (landed).** A plugin declares login buttons via a
  manifest `auth_providers` block (`{id, display_name, initiate}`, `initiate`
  naming one of its webhook keys). `plugins.Service.SSOProviders()` aggregates
  those from active, auth-capable plugins into initiate URLs, and the anonymous
  `/api/v1/app-state` boot payload carries them as `auth.ssoProviders`; the
  pre-auth login page renders one button per provider. No pre-auth plugin JS
  runs — the button is a plain navigation to the plugin's initiate webhook.
- **Not yet built:** setup-mode bootstrap of the first admin via SSO is out of
  scope (SSO requires `ModeEnabled`); the OIDC/SAML plugin implementations live
  in their own repos (e.g. `kandev-plugin-google-oidc`).

**Amendment (2026-08-12):** `2026-08-12-plugin-webhook-auth-gate.md` closed an
exposure where every plugin webhook, including ones with no external
authentication of their own, was reachable anonymously by default. The
`initiate` webhook this ADR depends on (and the IdP callback key it
redirects to) must now declare `webhooks[].public: true` in the manifest, or
`SSOProviders()` silently omits that provider from the login screen rather
than surface a button that 401s. Existing SSO plugin manifests (e.g.
`kandev-plugin-google-oidc`) need that flag added before their login button
works again on an auth-enabled install.

## Alternatives considered

- **A `Host` RPC that returns the session token to the plugin.** Rejected: the
  RPC channel cannot set the browser cookie, so the plugin would still set it
  via the webhook response — the same path — while additionally holding the raw
  token. More coupling, weaker security, and a proto change for no gain.
- **A plugin-registered HTTP route / middleware seam.** Rejected: a far larger
  surface (arbitrary route registration, request interception) than SSO needs;
  the existing public-allowlisted webhook already receives the callback.
- **First-party OIDC/SAML in the host.** Still possible later, but the point of
  this work is to prove the capability path so IdP integrations live and ship
  independently of the monorepo.
