# ADR-2026-07-20-managed-github-app-registration: Manage Self-Hosted GitHub App Registration at Runtime

**Status:** accepted
**Date:** 2026-07-20
**Area:** backend, frontend, security

## Context

[ADR 0047](0047-github-authentication-ownership.md) correctly makes one GitHub App registration a
deployment-owned identity, but it assumes an operator provisions every registration field through
configuration before Kandev starts. That boundary works for hosted deployments and infrastructure
automation, but self-hosters encounter an unexplained disabled workspace option and must manually
translate Kandev permissions, events, and callback URLs into GitHub settings.

GitHub's
[App Manifest flow](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest)
lets Kandev submit those settings, receive a one-hour conversion code, and exchange it for the App
ID, client ID/secret, private key, webhook secret, and slug. Registration remains deployment-owned:
creating the registration is different from installing it for a workspace, and an App installation
remains different from a user's personal OAuth identity.

The current Kandev HTTP runtime has no authenticated system-admin role; system requests effectively
act as `default-user`. That trust boundary must be explicit because registration controls a
deployment-wide private key and every future workspace installation.

## Decision

Kandev supports two sources for its single deployment GitHub App registration:

1. **Environment-managed registration.** Complete `KANDEV_GITHUB_APP_*` configuration remains the
   authoritative source for hosted and externally managed deployments. If any environment App
   configuration is present, it wins and a broken override fails closed rather than falling back to
   persisted credentials. The UI reports it as externally managed and cannot mutate it.
2. **Kandev-managed registration.** When no environment override is present, a deployment operator
   may use **System Settings > GitHub App** to create the App through GitHub's manifest flow. Kandev
   persists non-secret singleton metadata in the GitHub store and stores the private key, client
   secret, and webhook secret as one versioned JSON value in an immutable, generation-addressed
   encrypted-secret entry. Metadata points to the active entry. The registration is hot-loaded
   after conversion and rehydrated on restart.

Manifest registration has its own deployment-scoped, hashed, single-use, one-hour state records. It
does not reuse workspace installation or personal OAuth state. Kandev generates the manifest from a
versioned permission/event policy; operators choose owner type and owner login but do not hand-edit
permissions. The generated App is public (installable outside its owner, but not automatically
listed in Marketplace), requests OAuth on install for verified association, and ignores uninitiated
installations. GitHub.com is the only supported host in this increment.

The public base URL is a canonical HTTPS origin with no credentials, query, or fragment and cannot
be loopback or a private/link-local literal. Every DNS result must be globally routable at setup
time. Kandev derives every registration, installation, personal OAuth, and webhook callback from
that origin. It does not make an arbitrary server-side request to the supplied URL. A completed
public callback proves the browser path; webhook health is reported as verified only after Kandev
receives a correctly signed delivery for the active generation. Local users receive reverse-proxy
or tunnel guidance before registration.

Runtime App dependencies are swapped as one immutable generation. A new encrypted bundle is made
durable before the metadata pointer switches; cancellation or failure before that switch leaves the
prior generation active, while inactive bundles are reconciled later. Old bundles are deleted only
after the pointer commits. Failed conversion, validation, secret persistence, metadata persistence,
or runtime activation leaves the prior generation unchanged. Replacing or deleting a managed
registration is prohibited while any workspace is bound to one of its installations. Environment
credentials are never copied into the encrypted store implicitly.

Until Kandev has authenticated administration, the existing trusted-single-user runtime treats
`default-user` as deployment operator. This is a provisional compatibility boundary, not authority
for a future multi-user system. Deployment App mutation endpoints must require a real system-admin
permission before Kandev is exposed to mutually untrusted users.

This ADR amends only ADR 0047's environment-only deployment registration rule. ADR 0047's separation
of deployment, workspace automation, and personal identities remains in force.

## Consequences

- Self-hosters can create a correctly permissioned App without transcribing secrets or restarting
  Kandev, while hosted deployments retain deterministic environment ownership.
- Deployment status must distinguish `none`, `environment`, and `managed`; workspace status can link
  to setup but cannot configure deployment credentials itself.
- The GitHub service must resolve environment or persisted configuration at boot and atomically
  reload a managed generation after registration.
- A single encrypted bundle per generation prevents mixed key/secret generations. An atomic
  metadata pointer provides crash consistency across stores; inactive bundles are safe to reconcile
  because runtime loading follows only the committed pointer.
- Automatic credential rotation and replacement of an App with active workspace installations are
  deferred. Environment management remains the escape hatch for operator-controlled rotation.
- GitHub Enterprise Server needs a later host/version compatibility design. GitHub documents that
  App manifests are unavailable for enterprise-owned Apps, so it cannot be claimed through a simple
  base-URL substitution.
- The first release inherits Kandev's single-user trust assumption. Shipping it for shared,
  untrusted access without admin authorization would violate this decision.

## Alternatives Considered

### Keep environment-only configuration

Rejected for self-hosted Kandev. It preserves a simple startup boundary but makes the operator
manually reproduce a versioned App policy and creates an unexplained dead end in workspace settings.

### Store generated credentials in a new configuration file

Rejected. It introduces a second plaintext secret format, complicates container persistence and
permissions, and bypasses Kandev's encrypted secret-store backup behavior.

### Let each workspace create its own GitHub App

Rejected. App registration is deployment infrastructure, not workspace state. Per-workspace Apps
multiply private keys, webhook endpoints, OAuth clients, and rotation work while breaking ADR 0047's
single registration boundary.

### Automatically import environment credentials into the database

Rejected. It silently duplicates externally managed secrets and makes source precedence and
rotation ambiguous. Environment state remains externally owned unless a future explicit migration
flow is specified.

### Verify reachability by fetching the supplied public URL from the backend

Rejected. A self-request does not prove GitHub can reach the deployment and creates an SSRF surface.
Callback completion and signed webhook observation are the meaningful external signals.

### Implement an operator-token or full RBAC system in this change

Deferred pending approval. Either would improve shared-deployment security, but both establish a
new authentication product surface beyond GitHub onboarding. The recommended first increment keeps
the explicit trusted-single-user boundary and blocks multi-user claims.
