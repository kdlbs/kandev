---
status: active
system: integrations
created: 2026-07-17
updated: 2026-07-31
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

## System design

The migrated technical source is split into [part 1](../system-design/azure-devops-integration-01.md), [part 2](../system-design/azure-devops-integration-02.md).
