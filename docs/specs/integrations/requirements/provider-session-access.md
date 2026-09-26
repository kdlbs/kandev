---
status: draft
system: integrations
created: 2026-09-26
owners:
  - kandev
---

# Provider access for managed plugin sessions

## Overview

An approved managed plugin session may need to call its provider directly,
including recovering a CI run for an unchanged linked PR head. Integrations
owns the credential grant and provider identity boundary. The plugin owns the
provider action and its outcome. This requirement supersedes the action-specific
[scoped Coordinator CI run](scoped-coordinator-ci-runs.md) contract.

## Terminology

- **Provider grant:** administrator approval for one plugin installation,
  workspace, managed conversation, target task/repository, provider, and
  permission purpose. It has a generation and can expire or be revoked.
- **Access lease:** opaque, short-lived, non-secret authority to redeem one
  credential for an exact active session and target identity.
- **Provider receipt:** a plugin-owned record of a provider operation. A Host
  credential receipt proves only admission or redemption.

## Requirements

### REQ-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001: Grant direct provider access to an exact managed session

**Intent:** A workspace administrator can authorize a plugin's managed agent
session to obtain only the temporary provider credential needed for one
workspace/repository target. Kandev never performs the requested provider
operation or gives the credential to the agent.

**User story:** As a workspace administrator, I want to grant and revoke
provider access for a specific managed plugin conversation so its agent can
recover a linked PR's CI without granting all tasks or exposing my long-lived
provider credential.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.1:** Only an authenticated
  workspace administrator may create, narrow, list, or revoke a grant. A grant
  names one installed plugin, workspace, managed conversation, target task,
  repository, provider, permission purpose, and expiry. Replacement advances
  its generation atomically; revocation denies later issuance and redemption.
- **AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.2:** Issuance and redemption
  shall require the current manifest declaration and workspace approval, the
  same active grant generation, an active session whose backing task is owned
  by that plugin's exact managed conversation, a live target task and
  repository attachment in the same workspace, and the current provider
  connection. Missing, stale, cross-workspace, or caller-asserted identity
  shall fail closed.
- **AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.3:** For a PR/MR-bound
  request, the Host shall bind its lease to the current linked change request,
  expected head, base and head repository identity, requested operation class,
  and target run/check identity where applicable. A stale link, head, session,
  generation, or provider connection shall deny redemption. A lease alone
  shall never report provider action success.
- **AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.4:** A lease response shall
  contain only a bounded opaque identifier, expiry, exact non-secret scope,
  and audit correlation. Redemption shall deliver credential material only
  over the installed plugin's Host connection, transiently and without
  persistence or logs. Task MCP, agent tool results, UI, and audit receipts
  shall not contain a bearer token, App private key, authorization header,
  or secret-store pointer.
- **AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.5:** GitHub redemption shall
  require the verified workspace App installation and mint an uncached token
  for exactly one canonical repository with the minimum permission profile.
  PAT, named `gh` account, executor or ambient token, and legacy shared
  credentials shall not be fallback sources. Unsupported or insufficient
  permissions shall return a typed denial without a token.
- **AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.6:** On grant revocation,
  session termination, provider disconnect or rotation, plugin disable or
  uninstall, and lease expiry, Kandev shall reject future redemption. It
  shall attempt provider revocation of an issued token and report whether
  provider invalidation was confirmed; a failure shall retain a bounded,
  non-secret pending-revocation receipt until expiry or reconciliation.
- **AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.7:** The Host shall record
  non-secret grant, issuance, redemption, denial, expiry, and revocation
  receipts bound to plugin installation, workspace, task/session, target,
  provider principal, grant generation, and request identity. Repeated
  requests with the same exact idempotency identity shall not mint additional
  tokens; a changed scope with that key shall conflict. Host receipts shall
  never claim that the plugin's provider operation succeeded.
- **AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.8:** Kandev shall expose no
  action-specific rerun or workflow-dispatch MCP/Host proxy as part of this
  capability. A plugin shall make the provider API call and own its current
  PR/fork/run checks, idempotent action record, ambiguous-send reconciliation,
  and provider receipt.
- **AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.9:** A provider without a
  short-lived scoped credential and revocation contract, including the
  current GitLab workspace PAT path, shall be reported as unsupported for
  this grant. Kandev shall not expose the workspace token as a substitute.

## Exclusions

- General provider administration, arbitrary repositories, merge, deployment,
  release, branch or history rewrite, synthetic check status, and credential
  delivery to ordinary task agents.
- Treating plugin-supplied PR/run identity as authoritative without current
  Host linkage and plugin-side provider revalidation.
- Treating a GitHub repository-scoped installation token as cryptographically
  restricted to one PR, run, or workflow.

## Related contracts

- [System design](../system-design/provider-session-access.md)
- [Plugin Host boundary](../../plugins/requirements/plugins.md)
- [Direct provider access decision](../../../decisions/2026-09-26-plugin-direct-provider-access.md)
