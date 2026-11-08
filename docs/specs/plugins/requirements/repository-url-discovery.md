---
status: active
system: plugins
created: 2026-10-29
owners:
  - kandev
---

# Repository URL discovery requirements

## Overview

Host callers can receive a repository URL without a provider selection.
An active VCS plugin shall make its supported repository URLs discoverable through the existing server inspection capability.
The plugin system owns candidate eligibility, manifest authority, and inspected descriptors.
The task system owns admission and persistence after discovery.

Installation makes a capability available. Provider connection and authentication requirements still apply in the selected workspace.
The host shall not contain a hostname list for Bitbucket, Forgejo, or future VCS plugins.

## Requirements

### REQ-PLUGINS-REPOSITORY-URL-DISCOVERY-001: Discover repository providers from a bare URL

**Intent:** Host callers resolve supported plugin repository URLs without requiring provider hints or prior repository registration.

#### Acceptance criteria

- **AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.1:** When a host caller supplies a workspace and bare repository URL, discovery shall inspect eligible installed providers. Eligibility requires active manifest ownership and a workspace-scoped repository inspection capability.
- **AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.2:** Each candidate shall receive the caller's verified workspace. A plugin shall verify URL ownership against its own connection before authenticated provider access. An unconfigured, disabled, or unrelated connection shall not claim the URL.
- **AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.3:** Discovery shall return every successful, validated candidate. It shall preserve provider identity, scope, immutable repository ID, exact credential-free clone URL, and default branch.
- **AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.4:** Multiple successful candidates shall remain visible to the host caller. Discovery shall never choose a winner by registration order, hostname resemblance, or response speed.
- **AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.5:** When no candidate succeeds, discovery shall distinguish a normal no-match from invalid output or provider unavailability. Errors shall use bounded existing categories without secrets or raw provider responses.
- **AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.6:** Discovery shall bound concurrent inspections, response sizes, and elapsed time. Caller cancellation shall discard every result and stop outstanding work before returning.
- **AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.7:** A candidate from an inactive, removed, or replaced plugin installation shall not authorize a new selection. Discovery shall retain the existing dispatch-generation checks and revalidate eligible manifest ownership before returning candidates.
- **AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.8:** An explicitly selected provider shall use its owning plugin only. A failed explicit selection shall never fall back to another provider. Existing saved repository IDs and authenticated Host descriptors shall retain their current resolution paths.
- **AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.9:** A compatible new VCS plugin shall participate through its manifest and inspection action without a Kandev provider-specific code change. A plugin without that capability shall retain its existing saved-repository behavior.

## Compatibility and exclusions

The existing [explicit selection contract](repository-provider-task-creation.md) remains authoritative for provider-hinted requests.
The existing HTTPS clone-descriptor rules remain unchanged.
Discovery does not add authentication methods, clone transports, plugin package requirements, or credential access for host callers.
It does not make every installed plugin a repository provider.

## Design and delivery

- [Repository URL discovery design](../system-design/repository-url-discovery.md)
- [Provider-neutral project-source package](../../../plans/office-project-self-managed-gitlab/plan.md)
