# ADR-2026-10-22-azure-devops-entra-public-client: Azure DevOps sign-in through a Kandev Entra public client

**Status:** proposed
**Date:** 2026-10-22
**Area:** backend, integrations

## Context

The Azure DevOps integration authenticates each workspace with a PAT. Users
asked for a Microsoft sign-in alternative: click a button, sign in to Microsoft
in a popup, return to Kandev connected, with nothing to register or copy.

Azure DevOps OAuth (`app.vssps.visualstudio.com`) no longer accepts new app
registrations and is being retired. Microsoft Entra ID is the supported
delegated-auth path for Azure DevOps Services. Every Entra flow needs an
application (client) ID. Kandev is usually a local installation reached at
`http://localhost:<port>`, but it can also run on a remote host.

Alternatives considered:

1. **Per-workspace bring-your-own Entra app.** Works everywhere but makes every
   user register an application, enter tenant and client IDs, and manage
   redirect URIs. It contradicts the requested experience.
2. **Reuse a Microsoft first-party client ID** (for example the Visual Studio or
   Azure CLI public clients). No registration, but Kandev would impersonate a
   Microsoft application, which Microsoft does not support for third parties
   and which Conditional Access can block without notice.
3. **Delegate to `az login`.** Requires installing the Azure CLI on the backend
   host, opens the browser on that host rather than the user's, and contradicts
   the integration's rule that runtime paths do not require `az`.
4. **Device-code flow.** No redirect URI, but the user must copy a code into a
   second page, which is the friction the request wanted to avoid.
5. **Kandev-published multi-tenant public client with authorization code and
   PKCE.** One registration owned by the Kandev project. Users only sign in and
   consent.

## Decision

Use option 5. Kandev publishes one multi-tenant Entra application registered as
a public client ("Mobile and desktop applications" platform) with the redirect
URI `http://localhost` (the settings page origin), for which Entra ignores the
port. Its client ID ships as the default of
`azureDevOps.oauthClientId`. The backend performs the authorization-code
exchange with PKCE and no client secret, and stores the resulting tokens in the
encrypted secret store per workspace.

Operators can override the client ID and redirect URL in startup configuration
to use their own Entra application, for example when a tenant forbids
third-party applications or Kandev is served from a non-localhost origin. When
the browser cannot reach the configured redirect URL, the user pastes the final
URL into Kandev, which completes the same server-held flow.

Amendment (2026-10-26): a workspace can enter its own Application (client) ID
in the Microsoft account block; the redirect is always the page's own origin,
derived automatically, for both the workspace and the default application. This lets an organization use
an Entra application it already approves (for example, the one an internal tool
uses for Azure DevOps) without operator configuration. The installation default
still applies when the fields are blank, and renewal always uses the client ID
that signed in.

## Consequences

- The Kandev project must register, verify the publisher of, and maintain the
  Entra application. Until a client ID is published, Microsoft sign-in is
  unavailable unless an operator configures one.
- Tenants that block user consent to third-party applications require a one-time
  admin consent or an operator-supplied application.
- Kandev handles token renewal and re-sign-in states that PATs did not have.
- PAT remains supported and is the default method.
