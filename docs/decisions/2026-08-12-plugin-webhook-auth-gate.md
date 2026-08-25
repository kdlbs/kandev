# ADR-2026-08-12: Require Auth for Plugin Webhooks Unless the Manifest Declares Them Public

**Status:** accepted
**Date:** 2026-08-12
**Area:** backend, frontend, security

## Context

`isPublicPath` in `internal/auth/httpmw/middleware.go` allowlisted every
`/api/plugins/*/webhooks/*` path as unauthenticated, on the stated premise
that "the plugin subprocess owns signature validation." That premise was
enforced nowhere: `manifest.Webhook` had exactly three fields (`key`,
`description`, `method`), `validateWebhooks` checked only duplicate keys, and
nothing in `pkg/pluginsdk` required a plugin's `HandleWebhook` to verify its
caller. QA confirmed the consequence on an auth-enabled instance: an
anonymous `POST /api/plugins/kandev-plugin-notes/webhooks/enhance` passed the
middleware, reached the plugin subprocess, and got as far as
`host.InvokeUtilityAgent`, stopping only because no agent profile was bound.
With a profile bound, the same anonymous request returned a real, billed LLM
completion. Any plugin webhook reaching `agent_invoke` or another
spend-worthy capability was an open proxy on any reachable instance.

This was pre-existing — neither the route nor the allowlist entry came from
any single feature branch. What changed reachability was a later branch
whose guided-setup buttons walk users to a bound utility-agent profile,
moving the exposure from latent to live on installs that follow the new
setup flow.

Two anonymous callers of the webhook proxy are load-bearing and documented
elsewhere, ruling out simply dropping plugin webhooks from the allowlist:

- **SSO.** `0050-plugin-external-auth-capability.md` — the webhook response
  is the only plugin→browser HTTP path, so it is how a first session is
  minted for a visitor with no credentials yet.
- **Third-party callbacks.** `docs/specs/plugins/requirements/plugins.md` pins Jira POSTing
  to `/api/plugins/kandev-plugin-jira/webhooks/jira-webhooks` as an
  acceptance criterion; the plugin registry lists `kandev-plugin-slack` and
  `kandev-plugin-github-status`, which take the same shape of callback.

## Decision

`manifest.Webhook` gains a `public bool` field (`webhooks[].public`, default
`false`). `internal/auth/httpmw` no longer allowlists plugin webhooks
unconditionally: it structurally defers GET/POST `/api/plugins/<id>/webhooks/<key>`
(`isPluginWebhookPath`, replacing an over-matching `strings.Contains` check)
because the middleware has no access to the plugin registry to decide policy
per-webhook. `internal/plugins.Controller.webhook` is the one place holding
the manifest, so it enforces the actual gate
(`webhookCallerAuthorized`): a webhook whose manifest entry declares
`public: true` is relayed to anyone; anything else requires a real request
identity — a resolved session/PAT, or the synthetic identity
`httpmw.Middleware` injects while authentication is disabled. Identity
*presence* is a complete proxy for "authenticated or auth is off," so the
plugins package needs no dependency on `internal/auth` to decide.

The gate runs before any lookup-error or 404 is written. For a caller with no
identity, an unknown plugin ID, an undeclared webhook key, and a
declared-non-public key all return the same `401 {"error":"authentication
required"}` — never a 404 — so an anonymous caller cannot use response-code
differences to enumerate installed plugins.

`SSOProviders()` gains a matching skip: a provider whose `initiate` webhook
is declared but not `public` is omitted from the login screen rather than
surfaced as a button that immediately 401s (a visitor clicking a login
button has no session or PAT to present). This is a clean skip, not a
manifest validation error — the same tolerance `SSOProviders()` already had
for an `initiate` key naming no declared webhook — so an unpatched SSO
plugin degrades to "no login button" rather than an error state on upgrade.

The relay's header forwarding also changes: `flattenHeaders` now strips
Kandev's own session cookie and any `kandev_pat_*` Authorization bearer
before building the `WebhookRequest`. This matters because a non-public
webhook is now routinely reached by an authenticated browser
(`host.api.fetch` sends `credentials: "include"`) or PAT caller — the plugin
subprocess should see only what it needs to verify itself (a provider's own
bearer, a signature header), never the caller's Kandev credential. A
non-Kandev cookie or non-PAT bearer still relays unchanged, so a `public`
webhook can still verify a provider's own token.

A cookie-authenticated webhook request must include an accepted `Origin`.
Kandev uses the shared `internal/common/httpmw.AllowedOrigin` policy. PAT calls
and the synthetic identity used when authentication is off do not need an
Origin header.

Header forwarding depends on access:

- An authenticated webhook receives no `Authorization` or `Cookie` header.
  This prevents ambient reverse-proxy and browser credentials from reaching the
  plugin process.
- A public webhook can receive provider credentials. Kandev still removes its
  own session cookie and every `kandev_pat_*` credential.

Webhook dispatch uses the existing plugin generation read lease. The lease
covers the plugin RPC and response handling. Disable, uninstall, configuration
restart, and upgrade wait for the response to finish. Thus, an access decision
and an SSO login directive cannot cross into a replacement plugin generation.

`SSOProviders()` only returns a provider when its initiate webhook is effectively
public. API v1 omission remains public. API v2 requires `access: public` for the
initiate webhook and its callback.
The host logs one load-time warning when a declared initiate webhook is not
public. This gives the operator a diagnostic without logging on each boot read.

## Consequences

- The allowlist's stated premise ("the plugin subprocess owns signature
  validation") is now something the manifest actually asserts per webhook,
  instead of a blanket, unverifiable claim covering every plugin webhook.
- Production plugins taking third-party callbacks (`kandev-plugin-slack`,
  `kandev-plugin-github-status`, `kandev-plugin-google-oidc`) need a manifest
  update adding `public: true` to their callback/initiate webhook keys before
  they work again on an auth-enabled install; `kandev-plugin-notes`' `enhance`
  webhook is deliberately left unflagged, since it is only ever called from
  the logged-in Kandev UI.
- An auth-enabled install running an unpatched SSO plugin loses its login
  button (a clean `SSOProviders()` skip) until that plugin's manifest ships
  the flag; mitigated by the setup-wizard local admin always being available.
- `internal/plugins` now imports `internal/auth` for `auth.PATPrefix` — a
  same-direction import verified acyclic (nothing under `internal/auth`
  imports `internal/plugins`) and preferred over duplicating a
  security-relevant token format.
- The `auth/httpmw` pinning test for the plugin-webhook path is weaker by
  construction: under deferral, the middleware passes the path through either
  way, so it can no longer prove the auth policy on its own. The real pin is
  `internal/plugins/handlers_webhook_auth_test.go`.
- A `public: true` webhook is exactly as exposed as every plugin webhook was
  before this change — the flag makes that exposure explicit and reviewable,
  it does not remove it. Kandev can only enforce that *a* caller identity is
  present; it cannot verify a `public: true` webhook actually authenticates
  its own callers. No host-side rate limiting is added by this change.
- This amends `2026-07-24-opt-in-authentication.md` (point 6, the public
  allowlist), `0050-plugin-external-auth-capability.md` (the "already on the
  auth public allowlist" claim), and `2026-08-01-per-user-plugin-storage.md`
  (the "public, unauthenticated" webhook proxy claim).

## Alternatives Considered

1. **Registry-aware middleware.** Inject a plugin-webhook policy lookup into
   `httpmw.Middleware` so `isPublicPath` could consult the manifest directly.
   Rejected: couples the auth middleware to the plugin service, duplicates
   the manifest lookup the handler already performs, and breaks
   `isPublicPath`'s pure-function `(method, path)` testability.
2. **Drop plugin webhooks from the allowlist entirely, require a PAT for
   every caller.** Rejected: breaks the documented SSO first-session path (no
   PAT can exist before a session does) and every third-party callback
   (Slack, Jira, GitHub cannot present a Kandev PAT).
3. **Per-plugin shared secret verified by the host.** Rejected: leaves the
   platform default insecure (every plugin would need to opt into the
   secret), repeats the check outside any single enforcement point, and
   cannot express the SSO first-session case (no secret exists yet either).
4. **`api_version`-gated grandfathering** (unflagged webhooks on manifests
   still declaring `api_version: 1` stay public). Rejected: keeps the
   exposure open for exactly the plugins that have it today, defeating the
   purpose of the change; the flag addition is additive YAML the parser
   already tolerates, so there was no compatibility reason to gate it.
