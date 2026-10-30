---
status: draft
system: integrations
created: 2026-10-22
owners:
  - kandev
---

# Azure DevOps Microsoft Sign-In Requirements

## Overview

The Azure DevOps connection in
[Azure DevOps Integration](azure-devops-integration.md) authenticates with a
personal access token (PAT). Many Azure DevOps organizations restrict or
disable PAT creation, PATs expire on a fixed date, and users must choose scopes
by hand. Users in those organizations need to connect a workspace by signing in
with their Microsoft work or school account instead, without registering an
application or copying any token.

The integration system owns this capability because it changes the provider
credential of an existing integration connection. Generic Kandev user
authentication remains with the [auth system](../../auth/README.md).

## Terminology

- **Authentication method:** The credential type of one workspace connection:
  personal access token or Microsoft account.
- **Microsoft sign-in:** The browser flow in which the user signs in to
  Microsoft Entra ID and consents to Kandev accessing Azure DevOps on the user's
  behalf.
- **Signed-in identity:** The Azure DevOps identity that the Microsoft sign-in
  produced. It replaces the PAT owner wherever the integration acts as "me".
- **Sign-in availability:** Whether the Kandev installation has a Microsoft
  application identity configured that can start Microsoft sign-in.

## Requirements

### REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001: Microsoft account connection

**Intent:** Connect a workspace to Azure DevOps with a Microsoft account as an
alternative to a PAT, with no application registration or token handling by the
user.

**User story:** As a workspace user whose organization restricts PATs, I want
to sign in with my Microsoft account from Azure DevOps settings, so that Kandev
can use Azure DevOps without a personal access token.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.1:** When a user opens Settings >
  Integrations > Azure DevOps, the system shall show an Authentication method
  choice with Personal access token and Microsoft account. An existing
  connection shall show its stored method; a new connection shall default to
  Personal access token.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.2:** When the user selects Microsoft
  account, the system shall hide the PAT field and its scope help and shall show
  a Sign in with Microsoft action. The action shall be disabled until the
  organization URL is a valid Azure DevOps Services organization URL.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.3:** The Microsoft account block
  shall show an Application (client) ID field, prefilled with the workspace's
  saved value and offering the installation default as placeholder. Sign-in
  shall return to the address of the page the user is on, with or without a
  client ID entered, so no redirect setting needs maintaining when the port or
  host changes. When neither the field nor the installation provides a client ID,
  the system shall disable Sign in with Microsoft and show a localized
  explanation. PAT connections shall remain fully usable.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.4:** When the user activates Sign in
  with Microsoft, the system shall open the Microsoft sign-in page in a popup
  window for the organization's directory, using the entered application or the
  installation default. The user shall never enter a tenant ID, secret, or
  token, and shall not need a client ID when the installation has a default.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.5:** When Microsoft sign-in
  completes successfully, the system shall close the popup, save the
  connection for that workspace and organization with the Microsoft account
  method, and show the signed-in identity's display name without a page reload.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.6:** When the browser cannot return
  to Kandev automatically, the system shall let the user paste the final
  redirect URL into a dialog and shall complete the same sign-in from it.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.7:** When the user cancels sign-in,
  denies consent, closes the popup, or the sign-in attempt is older than ten
  minutes, the system shall leave any existing connection and credential
  unchanged and show a localized failure or cancellation message.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.8:** A sign-in attempt shall
  complete only once, only for the workspace where it started, and only for the
  Kandev user who started it. A reused, foreign, or unknown attempt shall fail
  without changing any connection.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.9:** Desktop and phone layouts shall
  expose the same method choice, sign-in action, paste fallback, signed-in
  identity, and errors. Phone controls shall have touch-sized targets, and the
  paste fallback shall be usable without horizontal scrolling.

### REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002: Equivalent behavior for Microsoft account connections

**Intent:** A Microsoft account connection is a complete alternative to a PAT,
not a partial one.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.1:** When a workspace uses a
  Microsoft account connection, connection test, project and repository
  listing, Boards, work items, pull requests, mentions, task pull-request
  synchronization, watchers, and work-item mutations shall behave as they do
  with a PAT that has the same access.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.2:** When a workspace uses a
  Microsoft account connection, Kandev's server-side clone and pre-worktree
  refresh of that workspace's Azure Repos repositories shall authenticate with
  the signed-in identity.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.3:** When "assigned to me" or
  another current-user filter runs on a Microsoft account connection, the
  system shall resolve it to the signed-in identity.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.4:** While the user does not
  interact, the system shall keep a Microsoft account connection usable for as
  long as Microsoft permits renewal without user interaction. An operation that
  starts after access expiry shall renew it first and shall not fail because
  of the expiry. Renewal shall use the application that signed in, even if the
  installation default changes later.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.5:** When Microsoft requires the
  user to sign in again, the system shall mark the connection unhealthy with a
  localized "sign in again" message in Azure DevOps settings and integration
  health, keep its organization, default project, saved views, watchers, and
  task links, and restore operation after a successful sign-in without other
  reconfiguration.

### REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003: Credential lifecycle across methods

**Intent:** Each workspace has exactly one usable Azure DevOps credential, and
changing it never leaves a stale or leaked credential behind.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.1:** When a workspace switches from
  PAT to Microsoft account, the system shall keep using the PAT until Microsoft
  sign-in succeeds, then remove the PAT.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.2:** When a workspace switches from
  Microsoft account to PAT, the system shall keep using the Microsoft account
  until a PAT is saved successfully, then remove the Microsoft account
  credential.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.3:** When a user changes the
  organization URL of a Microsoft account connection, the system shall require
  a new Microsoft sign-in for that organization before saving the change. Saving
  the default project alone shall not require sign-in.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.4:** When a Microsoft account
  connection is copied to another workspace, the system shall copy the
  organization and default project, mark the target as requiring Microsoft
  sign-in, and shall not copy the source workspace's Microsoft credential.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.5:** When a connection is deleted,
  the system shall remove its credential of either method.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.6:** The system shall never show,
  log, return through an API, persist outside its encrypted secret store,
  include in a URL, or provide to an agent environment the Microsoft access or
  renewal credential.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.7:** Existing PAT connections shall
  continue to work after upgrade without any user action.
- **AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.8:** When a user selects Sign out on
  a signed-in Microsoft account connection, the system shall delete the stored
  Microsoft credential, keep the organization, default project, application,
  watchers, and task links, stop acting as the user until a new sign-in, and
  show the Sign in with Microsoft action with a localized confirmation. Sign-out
  shall not end the user's Microsoft browser session or revoke tokens at
  Microsoft.

## Out of scope

- Microsoft personal accounts (MSA) and organizations backed only by Microsoft
  accounts. Sign-in targets work or school accounts.
- Service principals, managed identities, client credentials, and device-code
  sign-in.
- Azure DevOps Server (on-premises) and non-`dev.azure.com` hosts.
- Credentials for the `az` CLI that agents use to create pull requests; agents
  continue to use ambient `az` authentication.
- Per-Kandev-user Azure identities inside one workspace. One signed-in identity
  serves the whole workspace, as the PAT does.
