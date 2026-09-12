---
status: active
system: integrations
created: 2026-07-19
owners:
  - Kandev
---
# Workspace GitHub Authentication Requirements

## Overview

GitHub credentials must not silently cross workspace boundaries. A local workspace may only need a human PAT or a named `gh` CLI account, while unattended company automation benefits from a GitHub App's short-lived, repository-scoped installation tokens. Users also need to keep work and personal automation under different GitHub Apps without operating separate Kandev deployments.

## Requirements

### REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001: Workspace GitHub Authentication

**Intent:** GitHub credentials must not silently cross workspace boundaries. A local workspace may only need a human PAT or a named `gh` CLI account, while unattended company automation benefits from a GitHub App's short-lived, repository-scoped installation tokens. Users also need to keep work and personal automation under different GitHub Apps without operating separate Kandev deployments.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.1:** Every workspace chooses exactly one automation source: PAT, a named `gh` CLI account, a verified GitHub App installation, or the migration-only `legacy_shared` source.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.2:** GitHub App registration is configured from the workspace GitHub settings flow. There is no singleton GitHub App settings page and no automatically active deployment App.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.3:** A workspace may select a GitHub App registration already known to the Kandev deployment, import an existing GitHub App that the user owns, or create a new GitHub App through GitHub's App Manifest flow. Import and creation guide the user through ownership, callback, webhook, permission, visibility, and installation requirements.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.4:** The deployment stores a catalog of GitHub App registrations because a user may intentionally reuse one App across workspaces. Each workspace still selects and installs an App independently. Selecting an existing registration never binds another workspace automatically.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.5:** Users who require independent root credentials, bot identity, revocation, or ownership create a separate registration for each trust boundary. Work and personal workspaces can therefore use different Apps.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.6:** Reusing one registration shares its App private key, client secret, webhook secret, permission policy, and bot identity. Installation tokens, workspace repository scope, connection generation, broker leases, health, and personal OAuth tokens remain workspace isolated.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.7:** A newly created App defaults to private, meaning GitHub permits installation only on the account that owns it. The user may explicitly choose public when the same App must be installable on other GitHub accounts or organizations. Public does not list the App in Marketplace, reveal secrets, or grant repository access without installation approval.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.8:** PAT and named CLI automation act as the verified human account. A separate `My GitHub` connection is only offered when workspace automation uses a GitHub App, because App installations are not people and cannot provide authenticated-viewer semantics.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.9:** When a Local or Worktree task inherits executor Git credentials, every Kandev-managed GitHub checkout shall use the current host `gh` clone protocol for `github.com`. Host-specific configuration takes precedence over global configuration, and SSH is used when neither scope provides a supported protocol.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.10:** Every Local or Worktree launch and resume shall re-evaluate the inherited clone protocol. The system shall reconcile the Kandev-managed checkout before task preparation continues. This behavior shall not require a backend restart. Kandev shall not rewrite the origin of a user-managed local checkout.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.11:** When a Local or Worktree task launches or resumes, the system shall inspect each attached Kandev-managed GitHub checkout before the agent starts. This behavior includes a prepared workspace that the task reuses.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.12:** The system shall reconcile each managed checkout to the canonical transport for the current task policy. It shall not rewrite an already-canonical origin or a user-managed local checkout.


### REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-002: Executor HTTPS access through host GitHub CLI

**Intent:** Local and Worktree tasks can use an available host GitHub CLI credential for HTTPS Git operations.
This extends executor inheritance without changing workspace automation identity or managed task access.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.1:** In executor mode, Local and Worktree tasks shall use an available host CLI credential for each eligible attached GitHub host. HTTPS credential lookup shall work without a separate global credential-helper setup.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.2:** The bridge shall preserve every valid inherited Git configuration entry, its value, and its order. This includes multiple entries for hooks and notes, and meaningful repeated helper entries.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.3:** Explicit profile or executor tokens shall retain precedence over the host login. Managed task authentication shall never fall back to the host bridge after a broker failure.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.4:** The bridge shall apply only to eligible attached GitHub hosts on Local and Worktree executors. Remote executors, unrelated hosts, and other providers shall receive no host credential bridge.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.5:** When the host CLI is absent, unauthenticated, or exceeds its lookup deadline, task preparation shall continue without the optional bridge. Existing Git helpers and SSH behavior shall remain available. Caller cancellation shall remain cancellation.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.6:** Initial launch, resume, and prepared-workspace agent start shall apply the current eligibility rules before task commands run. Repeated preparation shall not accumulate bridge entries.
- **AC-INTEGRATIONS-GITHUB-AUTHENTICATION-002.7:** The bridge shall change only the task execution environment. It shall not write Git configuration files, persist tokens, change workspace automation identity, or rewrite repository remotes.

#### Exclusions

This requirement does not provide repository permissions, create an SSH key, or validate token permissions through a network request.
It does not change managed repository materialization, credential snapshots, UI controls, or credential routing for remote executors.

## System design

The migrated technical source is split into [part 1](../system-design/github-authentication-01.md), [part 2](../system-design/github-authentication-02.md), [part 3](../system-design/github-authentication-03.md).
