---
status: draft
system: plugins
created: 2026-11-02
owners:
  - kandev
---

# Azure DevOps Leaves Core Requirements

## Overview

The [Azure DevOps connector plugin](../../integrations/requirements/azure-devops-plugin.md)
replaces the first-party Azure DevOps integration. Kandev removes the
integration in one change and lists the plugin in its registry in another, as
agreed with the maintainer. Like the [Voice](voice-extraction.md) and
[Prompt History](prompt-history-extraction.md) extractions, no data moves: the
only known user reconnects after installing the plugin.

The plugins system owns this contract because it defines what core removes and
releases when a capability becomes a plugin.

## Terminology

- **Built-in integration:** the first-party Azure DevOps integration under
  `apps/backend/internal/azuredevops`, its web screens, and every Azure DevOps
  branch in shared code.
- **Reserved identity:** the repository provider identity `azure_devops` and
  the integration settings identity `azure-devops`, which the web plugin
  registry rejects while core owns them.

## Requirements

### REQ-PLUGINS-AZURE-DEVOPS-EXTRACTION-001: Built-in integration removal

**Intent:** Core no longer contains Azure DevOps knowledge, and the plugin can
take over the identities and the existing repositories.

#### Acceptance criteria

- **AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.1:** When the removal ships, Kandev
  shall contain no Azure DevOps REST client, routes, settings or browse
  screens, store slice, locale namespace, mention providers, watch dispatch,
  executor clone path, delivery-ledger provider branch, sidebar metadata keys,
  user preference field, startup configuration keys, settings-catalog domains,
  E2E mocks, or `az` pull-request path.
- **AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.2:** When the removal ships, the
  web plugin registry shall accept a plugin that claims `azure_devops` and
  `azure-devops`, and existing repositories with provider `azure_devops` shall
  take the plugin repository-provider and Git credential paths. The only row
  change is the provider host: a repository whose clone URL is on
  `https://dev.azure.com`, the only origin the built-in integration accepted,
  shall get that provider host, because managed Git credentials refuse a
  repository without one.
- **AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.3:** When an upgraded installation
  starts, Kandev shall take its usual pre-migration database backup, then drop
  the built-in integration's tables and delete its stored credentials in a
  replay-safe migration. Unrelated user settings columns can remain unused.
- **AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.4:** When the removal ships, public
  documentation shall describe Azure DevOps as a plugin, the built-in
  specifications shall be marked deprecated or superseded, and the release
  notes shall tell users to install the plugin and reconnect.
- **AC-PLUGINS-AZURE-DEVOPS-EXTRACTION-001.5:** After the plugin's first
  release, Kandev's plugin registry shall list `kandev-plugin-azure-devops`
  through a separate change.

## Out of scope

- The plugin's behavior, which the integrations system owns.
- Migrating built-in integration data or credentials.
- A coexistence period or a feature toggle for the built-in integration.
- A notice page at the removed routes.
- Generic host contracts for plugin providers (delivery-ledger links, sidebar
  watch classification, launch delegations, broker reachability on Docker
  and SSH). They remain possible follow-ups.
