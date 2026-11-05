---
status: active
system: integrations
created: 2026-07-17
updated: 2026-10-20
owners:
  - tbd
---
# Azure DevOps Integration Requirements

## Overview

Teams whose source code and planning work live in Azure DevOps cannot use Kandev's GitHub or GitLab browsing surfaces to find work items, inspect pull requests, or associate a pull request with a task. Azure users must be able to connect their workspace, work from the same team board they use in Azure DevOps, and inspect Azure Repos data without installing or authenticating the GitHub CLI.

## Requirements

### REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001: Azure DevOps Integration

**Intent:** Teams whose source code and planning work live in Azure DevOps cannot use Kandev's GitHub or GitLab browsing surfaces to find work items, inspect pull requests, or associate a pull request with a task. Azure users must be able to connect their workspace, work from the same team board they use in Azure DevOps, and inspect Azure Repos data without installing or authenticating the GitHub CLI.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.1:** An Azure DevOps connection is configured independently for each Kandev workspace.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.2:** The first release supports Azure DevOps Services organizations hosted at `https://dev.azure.com/<organization>` and authenticates with a personal access token stored in Kandev's encrypted secret store.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.3:** Azure DevOps reads use the Azure DevOps REST API directly. Neither `gh` nor `az` is required for connection checks, work-item reads, pull-request reads, or pull-request synchronization.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.4:** Users can test, replace, copy to another workspace, and delete an Azure DevOps connection from Settings > Integrations > Azure DevOps.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.5:** Users can browse work items returned by WIQL, inspect their core fields, and launch the existing task-creation flow with the work-item title, description, URL, project, type, state, and identifier available to the launcher.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.6:** The Azure DevOps browser includes a Board mode alongside Work items and Pull requests. Board mode is the default connected view and selects context in Azure's hierarchy: project, then team, then board/backlog level.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.7:** Board mode initially selects the configured default project when available, the first accessible team, and the first visible requirement board (falling back to the first visible board). Users can change every level explicitly. Each user's last valid mode, preset, project, team, board, focused column, work-item filters, and pull-request filters are restored independently for each workspace on the next load.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.8:** The selected board shows Azure's columns, column item counts and limits, and work-item cards with ID, title, type, assignee, and tags.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.9:** When a user opens the task-creation Remote repository picker, Kandev shall request and show repositories only from source-control integrations that report an available connection for that workspace. An unconfigured integration shall not produce a repository-list error or a provider tab.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.10:** When repository listing fails for a source-control integration that reported an available connection, Kandev shall show that provider failure without removing repositories loaded from other providers, and the user shall be able to retry the listing.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.11:** Desktop and phone task-creation flows shall apply the same provider-availability and partial-failure behavior while preserving manual remote URL entry.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.12:** When a user opens an Azure work item, its detail surface shall list every linked Kandev task in the current workspace whose association identifies that item and its project by either the selected project ID or the current provider work item's project name. Existing project-ID associations and normal task-creation associations stored with a project name shall both remain visible on desktop and phone.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.13:** When the current work item's project name is missing, empty, or whitespace-only, detail shall match only the nonempty selected project ID. Empty association project values shall never match. When the selected project and work-item project values coincide, each matching association shall appear once; several distinct linked tasks shall all remain visible.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.14:** Detail shall exclude associations for another workspace, another work item, or a project value other than those two exact current-project identities, including when the work-item number matches. Matching shall not infer project aliases by changing case, trimming identifiers, or dropping the project constraint.

- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.15:** When saved-view saves or confirmed deletes overlap within one mounted Azure page, the system shall preserve action order in both the visible list and persisted list. An older result shall not undo newer pending intent.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.16:** When a saved-view action fails, the system shall remove that action's optimistic effect and preserve successful actions. Later pending actions shall use the last confirmed server list, including server normalization, rather than assuming failed actions succeeded. The failed action shall remain retryable.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.17:** When the initial saved-view read is pending across an admitted action, the system shall preserve the loaded views that action does not change, and shall not publish the initial read over the pending action. If the read fails, the system shall send no saved-view write, reject actions admitted against that read, and reload the server list when the user retries an action.
- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.18:** Desktop and phone shall apply the same saved-view ordering and recovery behavior. Save failures shall retain the existing dialog feedback, and delete failures shall show localized feedback without an unhandled rejection.

- **AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.19:** Assignment and Move completion shall not reopen closed detail or replace a later work-item selection. Closing, reopening, changing scope, or unmounting shall invalidate the old opening's local completion, error, and busy state. Confirmed board data shall still update while its originating workspace, project, team, and board remain current. Desktop and phone shall preserve the same ownership rules.

## Exclusions

Linked-task display does not change association validation or stored identities,
introduce a migration, or infer historical project names after an Azure project
rename. It adds no provider mutation or new detail interaction.

Saved-view ordering is scoped to one mounted workspace page. Multi-tab consistency,
new backend concurrency contracts, and ordering across page remounts are excluded.

## System design

The migrated technical source is split into [part 1](../system-design/azure-devops-integration-01.md), [part 2](../system-design/azure-devops-integration-02.md).
