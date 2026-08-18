# ADR-2026-08-12-plugin-webhook-auth-gate: Require Auth for Plugin Webhooks Unless Declared Public

**Status:** accepted (amended 2026-08-18)
**Date:** 2026-08-12
**Area:** backend, frontend, security

## Context

The global authentication middleware let all plugin webhook requests pass.
It assumed that each plugin authenticated its own caller. The manifest did not
state this requirement, and the host did not enforce it.

An anonymous caller could therefore invoke a plugin webhook with a costly Host
capability. Public provider callbacks and SSO callbacks must still work without
a Kandev session. The platform needs a per-webhook access rule.

Existing API v1 plugins use omitted webhook access as public behavior. Published
plugins and the plugin template depend on this behavior. A hard change inside
API v1 would silently break those plugins.

## Decision

`manifest.Webhook` has an optional `access` field. It accepts `public` or
`authenticated`. The default depends on the manifest API version:

- API v1 keeps the public default for compatibility. Kandev logs a warning that
  names each v1 plugin and each webhook with omitted access.
- API v2 uses the authenticated default. New examples and fixtures use API v2.
- An explicit value has the same meaning in both versions.

`internal/auth/httpmw` structurally defers GET and POST plugin webhook routes.
The middleware cannot read the plugin registry. `internal/plugins.Controller`
reads the declaration and applies the access rule.

The controller applies the rule before it returns lookup details. An anonymous
caller gets the same 401 response for an unknown plugin, an unknown key, or an
authenticated webhook. This prevents anonymous enumeration.

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

## Consequences

- New plugins get a secure default without breaking published API v1 plugins.
- API v1 remains usable, but logs identify the declarations that need migration.
- Public callbacks must validate the provider method, signature, token, replay
  identifier, or equivalent credential inside the plugin.
- Authenticated plugin UI work must use authenticated webhooks or plugin actions.
  Ambient browser and proxy credentials never reach authenticated webhooks.
- A lifecycle change can wait for an in-flight webhook and its response side
  effects. This gives one generation ownership of the complete request.
- The true access policy stays in the plugin controller. The global middleware
  only recognizes the route shape.

This decision amends `2026-07-24-opt-in-authentication.md`,
`0050-plugin-external-auth-capability.md`,
`2026-08-01-per-user-plugin-storage.md`, and
`2026-08-11-composer-access-authenticated-webhooks.md`.

## Alternatives Considered

1. **Use registry-aware authentication middleware.** Rejected because it couples
   the global middleware to the plugin service and duplicates the manifest lookup.
2. **Require a PAT for every webhook.** Rejected because third-party providers and
   first-session SSO callbacks cannot present a Kandev PAT.
3. **Use one host-managed secret for each plugin.** Rejected because providers use
   different authentication protocols and SSO has no Kandev secret before login.
4. **Change all API v1 omissions to authenticated.** Rejected because it silently
   changes an existing public contract and breaks published plugins.
5. **Keep every API version public by default.** Rejected because new UI and costly
   webhook operations would remain exposed unless every author found the opt-in.
