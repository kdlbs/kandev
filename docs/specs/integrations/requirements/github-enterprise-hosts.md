---
status: active
system: integrations
created: 2026-10-14
owners:
  - Kandev
---

# GitHub Enterprise host requirements

## Overview

Users shall connect workspaces to GitHub.com, GitHub Enterprise Server (GHES),
and GitHub Enterprise Cloud with data residency on `*.ghe.com`. Integrations
owns this capability because it owns provider credentials, external identities,
and the operations performed against those identities.

The original failure occurred in the existing interaction: the GitHub CLI
account dropdown listed Enterprise accounts, but saving one failed with
`unsupported GitHub host`. Discovery and selection already existed. Success
requires the saved account's API requests and subsequent workflows to use its
host, not merely removing the save-time rejection.

The user confirmed both Enterprise host families, all existing authentication
methods, and the documented server-side OAuth flow for GHES versions without
PKCE. The existing one-automation-connection-per-workspace model remains the
scope boundary. Different workspaces may connect to different hosts concurrently.

This feature extends [GitHub authentication](github-authentication.md). Its host
rules supersede that feature's GitHub.com-only restriction;
credential ownership, authorization, and task-access policy remain governed by
the existing authentication requirements.

## Requirements

### REQ-INTEGRATIONS-GITHUB-ENTERPRISE-001: Host-aware connections

**Intent:** A selected GitHub host determines where credentials and requests go.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.1:** When a user selects an authenticated
  Enterprise CLI account from the existing dropdown and saves it, Kandev shall
  verify that exact host/login, save the connection, and successfully read a
  repository from that host without changing the CLI's active account.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.2:** PAT and App connection setup shall
  accept a GitHub web hostname or HTTPS origin and use the matching web, REST,
  and GraphQL destinations for GitHub.com, GHES, and `*.ghe.com`.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.3:** Invalid hosts shall fail before
  credential transmission. Credentials shall not follow an API redirect or
  pagination link to another origin, a downgrade to HTTP, or an unexpected API
  path. Failure shall preserve the prior working connection.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.4:** Connections to private-network GHES
  shall work with valid TLS and operator-installed trust roots. An untrusted
  certificate or unreachable host shall produce a recoverable connection error;
  Kandev shall not disable TLS verification.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.5:** Existing GitHub.com connections and
  omitted-host legacy requests shall keep their GitHub.com meaning. Changing
  ambient CLI accounts or environment variables shall not redirect an explicitly
  selected connection.

### REQ-INTEGRATIONS-GITHUB-ENTERPRISE-002: Authentication parity

**Intent:** Enterprise users can use the same authentication choices available
for GitHub.com within the provider's supported capabilities.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.1:** PAT and named CLI connections shall
  validate credentials on their selected host, keep secrets in their existing
  ownership boundaries, and distinguish authentication failure from host or
  network failure without exposing credentials.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.2:** Users shall select, import, create
  through a manifest, and install GitHub Apps on the selected host. Identical
  App or installation IDs on different hosts shall remain distinct. Existing
  operator permissions and user/organization App ownership rules shall apply.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.3:** Personal App OAuth shall authorize,
  exchange, refresh, and revoke credentials on the registration's host. It shall
  use PKCE where supported and the documented confidential-client flow on GHES
  versions without PKCE. A failed exchange shall never trigger a weaker retry.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.4:** OAuth and installation callbacks
  shall reject replay, expired state, wrong host/registration/workspace, and a
  connection replaced during authorization. Signed webhooks shall affect only
  the verified registration's host and matching installation.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-002.5:** A provider version, permission, or
  policy that cannot satisfy an operation shall produce a specific unsupported
  or denied outcome. An unavailable optional capability shall not break basic
  repository, issue, and PR reads or cause a fallback to GitHub.com.

### REQ-INTEGRATIONS-GITHUB-ENTERPRISE-003: External identity isolation

**Intent:** Equal owner/repository names and resource numbers on different hosts
must never refer to the same external work.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.1:** Repository browsing/import, issue
  linking, PR creation/review/merge, fork workflows, checks, and watches shall
  preserve the resource's host in requests, task associations, and external links.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.2:** Two workspaces using different
  hosts with equal owner/repository names, PR/issue numbers, and provider IDs
  shall not share cached results, credentials, rate-limit state, associations,
  webhook effects, or automation outcomes.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3:** Replacing a connection with a
  different host shall invalidate old credentials and pending authorization
  flows. Existing repositories, linked tasks, watches, and saved filters shall
  retain their original host and shall not execute against the replacement host.
  Users shall see which items require reconnection or new configuration.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.4:** After upgrade and restart,
  previously valid GitHub.com state shall continue working. Ambiguous or
  contradictory historical host identity shall fail closed and require explicit
  repair; it shall not be inferred from a newly selected connection.

### REQ-INTEGRATIONS-GITHUB-ENTERPRISE-004: Task Git and CLI access

**Intent:** Enterprise repositories remain usable throughout task execution.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.1:** Managed tasks shall clone, fetch,
  push, and use `gh` for attached Enterprise repositories with renewable,
  repository-bound credentials under the existing task-access policy. Root App
  secrets and personal OAuth tokens shall never enter the executor.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.2:** A managed credential request for a
  different host, repository, workspace, or expired generation shall fail.
  Managed `gh` shall select the proper host/token family and shall not inherit
  unrelated ambient tokens or redirect its credential to another requested host.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.3:** Local/Worktree executor inheritance
  shall use host-specific CLI protocol and credential settings for eligible
  attached repositories. Explicit token precedence shall distinguish GitHub.com
  and `*.ghe.com` from GHES. User-managed checkout origins shall not be rewritten.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-004.4:** Launch, resume, prepared-workspace
  reuse, and credential renewal shall preserve host identity. Remote executors
  shall report missing connectivity or trust without silently using local host
  credentials or GitHub.com.

### REQ-INTEGRATIONS-GITHUB-ENTERPRISE-005: Discoverable setup and recovery

**Intent:** Users can connect and diagnose Enterprise hosts on desktop and phone.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.1:** CLI setup shall keep the existing
  account dropdown, display each account's host, and derive the host from the
  selection without requiring a second hostname entry. PAT/App setup shall expose
  a host field defaulting to GitHub.com and generate host-correct setup links.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.2:** Settings shall display the selected
  host and identity, retain a non-secret draft after validation failure, prevent
  stale responses from a previous host/workspace from replacing the current
  state, and explain host replacement effects before saving.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.3:** The phone connection surface shall
  provide the same setup and recovery actions with one scrolling body, reachable
  actions, at least 44px touch targets, safe-area clearance, keyboard access, and
  no document horizontal overflow. Resizing shall preserve the unsaved draft.
- **AC-INTEGRATIONS-GITHUB-ENTERPRISE-005.4:** New copy shall be localized in all
  supported catalogs. Public documentation shall explain host setup, App
  callbacks/webhook reachability, trust roots, CLI token precedence, compatibility
  limits, and recovery from host changes.

## Out of scope

- Multiple simultaneous GitHub automation hosts within one workspace, cross-host
  forks, and automatic migration of repositories between GitHub installations.
- Enterprise-account-owned or enterprise-level App installations, which introduce
  different ownership/permission contracts from the current user/organization Apps.
- Custom API base overrides, path-prefixed reverse proxies, HTTP hosts, TLS bypass,
  and automatic distribution of corporate CA certificates to executors.
- Changing Kandev's own release/download URLs, GitHub Actions, or unrelated
  plugins merely because they contain GitHub.com URLs.

## Design and delivery

- [System design](../system-design/github-enterprise-hosts.md)
- [Implementation plan](../../../plans/github-enterprise-hosts/plan.md)
