# ADR-2026-07-21-workspace-selectable-github-app-registrations: Select GitHub App Registrations Per Workspace

**Status:** accepted
**Date:** 2026-07-21
**Area:** backend, frontend, security

## Context

[ADR 0047](0047-github-authentication-ownership.md) separated deployment App credentials,
workspace automation, and personal identity. The first implementation then made one GitHub App a
singleton configured under System Settings. That prevents one Kandev deployment from giving work
and personal workspaces independent App ownership, bot identity, root credentials, and revocation.
It also makes a deployment setting appear active even though installation and repository grants are
workspace decisions.

Users need three App onboarding paths from workspace GitHub settings: select an App already known
to Kandev, import an existing App they own with exact configuration guidance, or create an App with
GitHub's manifest flow. The singleton schema and configuration-backed App source have not shipped,
so they can be removed rather than migrated.

## Decision

Kandev stores a deployment catalog of zero or more GitHub App registrations. A registration holds
the App's root identity and credential generation, but it is never globally active. Every workspace
chooses one automation source and, for App automation, explicitly selects one catalog registration
and completes a verified installation. The workspace connection persists both registration ID and
installation ID. Runtime resolution, callback state, webhook delivery, caches, and broker leases
carry registration identity as well as workspace identity.

Registration management is entered from workspace GitHub settings. A user can:

1. select a known registration and install it for the current workspace;
2. import an existing App by applying Kandev's documented callback, setup URL, webhook, permission,
   and event policy, then submitting its App ID, OAuth client material, private key, and webhook
   secret for verification and encrypted storage; or
3. create a registration through a workspace-bound App Manifest flow.

Catalog scope permits deliberate reuse without duplicating root secrets. Reuse shares the App
private key, OAuth client, webhook secret, permission policy, bot identity, and their operational
revocation boundary. Installation tokens, repository grants, workspace credential generations,
personal tokens, and leases remain isolated. Users who need a work/personal or organizational trust
boundary create separate registrations and select them in the corresponding workspaces.

Kandev-created Apps default to private because a workspace-specific App normally installs only on
its GitHub owner. The creation flow allows an explicit public choice when one registration must be
installed on other accounts or organizations. Public means installable outside the owner; it does
not publish the App to Marketplace, expose its credentials, or grant repository access.

The unpublished `KANDEV_GITHUB_APP_*` fields, environment bindings, source resolver, and
configuration documentation introduced on this branch are removed. They do not create a catalog
entry or configure runtime App auth. An operator with an existing App uses the guided import flow,
which verifies and stores it with the same lifecycle as every other imported registration.

Public routes identify the candidate registration before secret-dependent processing. Manifest,
installation, and personal callbacks use a registration ID in the route and must also match hashed
single-use state. Webhooks use
`/api/v1/github/app/registrations/:registrationId/webhook`; Kandev loads only that registration's
secret and verifies HMAC before parsing the payload, claiming a delivery, or changing health. A
delivery is unique per `(registration_id, delivery_id)`.

Managed/imported credentials remain versioned immutable encrypted bundles. Registration metadata
atomically points to an active generation only after its bundle is durable. A registration cannot
be deleted while any workspace or personal connection references it. Changing workspace App
automation atomically revokes old leases and removes personal OAuth issued by the prior App.

Until Kandev has authenticated roles, the trusted `default-user` acts as both registration manager
and workspace operator. This provisional authority must be replaced before exposing the deployment
to mutually untrusted users.

This decision supersedes
[ADR-2026-07-20-managed-github-app-registration](2026-07-20-managed-github-app-registration.md) and
amends ADR 0047's deployment registration layer. ADR 0047's one-automation-connection-per-workspace
and optional workspace personal identity rules remain in force.

## Consequences

- Work and personal workspaces can use different Apps without separate Kandev deployments.
- A workspace can reuse a known managed/imported App without copying its root secret, while the UI
  must disclose the shared revocation and identity boundary.
- App client construction changes from one atomically swapped runtime to a registry keyed by
  registration ID and credential generation.
- Webhook and OAuth paths become registration-aware. Route IDs are selectors, not authorization;
  signatures, state, workspace access, and verified GitHub association remain mandatory.
- System Settings no longer owns a GitHub App page. Workspace settings must explain method choice,
  registration reuse, import, creation, installation, personal identity, and private/public meaning.
- Hosted and self-hosted operators use the same managed/imported catalog contract; there is no
  configuration-only registration path or precedence rule.
- The unpublished singleton tables, migrations, services, and frontend are rewritten directly.
  Released legacy-shared workspace migration remains unchanged.
- More registrations increase key rotation, webhook health, and operational surface. The catalog
  and explicit reuse option prevent forcing one App per workspace when a shared organizational App
  is the intended trust model.

## Alternatives Considered

### One deployment App with many installations

Rejected. Installation tokens isolate repositories, but the root key, bot identity, permission
policy, ownership, and revocation remain shared across unrelated work and personal contexts.

### Always create one App per workspace

Rejected. It provides strong isolation but needlessly multiplies credentials and GitHub settings for
organizations that intentionally want one automation identity across related workspaces.

### Duplicate an imported App's credentials into every workspace

Rejected. It creates ambiguous rotation and deletion semantics and stores multiple copies of the
same root secret. A catalog registration plus explicit workspace bindings represents reuse directly.

### Keep registration management in System Settings

Rejected. Registration is only useful after a workspace selects and installs it. Workspace-context
onboarding makes the consequence, method alternatives, and eventual binding visible while the
catalog still permits deliberate cross-workspace reuse.

### Keep a configuration-backed App registration

Rejected. It creates a second secret lifecycle and precedence model outside workspace onboarding,
and makes an App appear configured without an explicit catalog/import action. Operators who already
own an App use the same verified import flow as every other deployment.

### Choose a webhook secret by trying every registered App

Rejected. It has linear cost, obscures routing, and expands the set of keys touched by an
untrusted request. Registration-specific routes select one candidate before constant-time HMAC
verification.
