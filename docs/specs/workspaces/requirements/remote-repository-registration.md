---
status: draft
system: workspaces
created: 2026-09-28
owners:
  - kandev
---

# Remote Repository Registration Requirements

## Overview

Workspace repositories are the durable records that tasks, repository sets,
branch policies, and secrets attach to. Until now the workspace Repositories
settings page could only register a local checkout; a provider-hosted
repository (GitHub, GitLab, Azure DevOps, or a plugin provider such as
Forgejo/Gitea) could be registered only as a side effect of creating a task
that used it. Users who prepare a workspace before any task exists, or who
need a repository for a repository set or a branch policy, need to register
the remote repository directly from the settings page.

The workspace system owns this contract because it owns repository records and
their attachment to a workspace. The task system keeps ownership of task
creation; the plugin and integration systems keep ownership of provider
verification and credentials.

## Terminology

- **Remote repository:** A repository hosted by a built-in provider (GitHub,
  GitLab, Azure DevOps) or by a registered plugin repository provider.
- **Locator:** The credential-free URL or picker selection that identifies a
  remote repository, together with optional provider hints from the picker.
- **Provider identity:** The persisted workspace, provider, host, scope,
  owner or project, and name that make a repository record unique.

## Requirements

### REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001: Register a remote repository from workspace settings

**Intent:** A user registers a provider-hosted repository in a workspace
without creating a task, through the same verified picker that task creation
uses.

**User story:** As a workspace owner, I want to add a remote repository from
Settings, so that repository sets, branch policies, and later tasks can use it
before any task exists.

#### Acceptance criteria

- **AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.1:** When the user opens
  **Settings > Workspace > Repositories** for a writable workspace, the page
  shall expose an **Add repository** control that offers **Local repository**
  and **Remote repository**. The local entry shall open the existing local
  repository discovery dialog unchanged.
- **AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.2:** When the user
  chooses **Remote repository**, the system shall present the same remote
  repository picker as the **Remote** tab in **New Task**: search across every
  connected built-in provider and every registered plugin repository provider,
  or a pasted supported provider URL, followed by a base-branch choice that
  defaults to the repository's default branch.
- **AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.3:** When the user
  confirms a selection, the system shall verify the locator on the server
  before it persists anything: a built-in provider locator through the
  existing URL contract, and a plugin provider locator through the owning
  plugin's repository inspection. Provider hints supplied by the browser shall
  never be persisted without that verification.
- **AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.4:** When the verified
  provider identity matches a repository the workspace already has, the system
  shall return that repository instead of creating a duplicate. When it does
  not match, the system shall create one provider-backed repository record
  with the verified identity, the canonical credential-free clone URL, and the
  chosen base branch as its default branch.
- **AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.5:** When registration
  succeeds, the dialog shall close and the repository shall appear in the
  Repositories list immediately, without an unsaved draft state, and shall
  remain listed after the page is reloaded.
- **AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.6:** When verification
  fails, the system shall persist nothing, the dialog shall stay open with the
  selection intact, and it shall show the failure reason. The API shall answer
  an unsupported or malformed locator, or a provider hint that disagrees with
  the locator, with `400` and `repository_selection_invalid`. For a plugin
  provider, whose inspection is authoritative, the API shall also answer an
  unknown repository with `404` and `repository_selection_not_found` and an
  unreachable provider with `503` and `repository_selection_unavailable`.
  Built-in providers are verified from the locator alone; their default
  branch lookup is best effort and does not block registration.
- **AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.7:** When the workspace
  is the read-only Improve Kandev workspace, the page shall not offer the
  **Add repository** control and the registration API shall refuse with
  `409`.
- **AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.8:** On a phone
  viewport, the **Add repository** menu and the remote repository dialog shall
  stay reachable by touch with the same composition the **New Task** Remote
  tab uses, and the page shall not scroll horizontally.

## Out of scope

- Registering a repository from an issue or pull-request URL; those remain
  task-creation entry points owned by the task system.
- Editing a registered remote repository's provider identity after
  registration.
- Provider credential configuration, which belongs to the integration and
  plugin systems.
- Creating a new repository on the provider.
