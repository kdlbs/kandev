---
status: active
system: plugins
created: 2026-08-26
owners:
  - kandev
---

# Plugin Repository Task Creation Requirements

## Overview

The native task dialog can list repositories from an installed code-host
plugin before Kandev has stored those repositories. Today, the browser sends a
complete repository description during task creation. The task service does
not trust that description, so it tries to parse the URL as a built-in
provider. A Bitbucket URL then fails after the task row exists and leaves an
empty task.

Kandev must resolve a first-use plugin repository through the active plugin on
the server. The browser can select a repository, but it cannot define the
repository identity or the host that can receive credentials.

## Terminology

- **Repository selection:** Untrusted request data that identifies what the
  user selected. It is a lookup hint, not authority.
- **Authoritative descriptor:** A complete, credential-free repository
  identity returned by the active manifest owner through a verified workspace
  action.
- **First-use repository:** A provider repository that is visible in the
  picker but does not yet have a Kandev repository row.

## Requirements

### REQ-PLUGINS-REPOSITORY-TASK-CREATION-001: Server-Resolved Plugin Repository Selection

**Intent:** Let users create tasks from first-use plugin repositories without
trusting repository identity supplied by the browser or leaving partial tasks
when resolution fails.

**User story:** As a workspace user, I want to select a repository from a
connected code-host plugin and create a task immediately, so that I do not have
to register the repository in a separate step.

#### Acceptance criteria

- **AC-PLUGINS-REPOSITORY-TASK-CREATION-001.1:** When a user selects a
  first-use repository from an active plugin provider in the native task
  dialog, Kandev shall resolve, register, and attach that repository during the
  same task-create operation.
- **AC-PLUGINS-REPOSITORY-TASK-CREATION-001.2:** Kandev shall resolve an
  unregistered plugin repository by invoking the active manifest owner's
  workspace-scoped `repositories.inspect` action from the backend.
- **AC-PLUGINS-REPOSITORY-TASK-CREATION-001.3:** For a first-use plugin
  selection, Kandev shall treat every provider host, scope, repository ID,
  owner, name, clone URL, and default branch received from REST, WebSocket, or
  MCP task-create input as untrusted. It shall persist only the descriptor
  returned by the verified plugin action. Existing repository IDs, valid
  built-in provider URLs, and authorized plugin Host `Tasks.Create` calls shall
  continue through their existing server-owned paths.
- **AC-PLUGINS-REPOSITORY-TASK-CREATION-001.4:** Before persistence, Kandev
  shall verify manifest ownership, workspace scope, provider identity,
  immutable repository identity, a credential-free HTTPS clone URL, and clone
  origin agreement with the returned provider host. If request pinning hints
  are present, the returned immutable values shall match them.
- **AC-PLUGINS-REPOSITORY-TASK-CREATION-001.5:** When the provider is unknown,
  inactive, disconnected, or missing the inspect action, or when inspection
  times out, finds no match, or returns an invalid descriptor, Kandev shall
  return a safe, actionable error before it creates a task row, repository row,
  or task-repository association.
- **AC-PLUGINS-REPOSITORY-TASK-CREATION-001.6:** For a multi-repository task,
  Kandev shall resolve and validate every first-use plugin selection before it
  creates the task or any repository. One failed selection shall not leave a
  partial task or a partially registered repository set.
- **AC-PLUGINS-REPOSITORY-TASK-CREATION-001.7:** Existing workspace repository
  IDs, built-in provider URLs, and authorized plugin Host `Tasks.Create` calls
  shall keep their current resolution paths. A retry with an already-settled
  external task ID shall return the existing task without invoking a plugin.
- **AC-PLUGINS-REPOSITORY-TASK-CREATION-001.8:** The native task dialog shall
  keep the user's input open and show the bounded provider-resolution error.
  It shall not show plugin secrets, raw authenticated URLs, or unrestricted
  upstream error text.
- **AC-PLUGINS-REPOSITORY-TASK-CREATION-001.9:** Desktop and phone users shall
  have the same repository selection, branch selection, submit, success, and
  failure capabilities through the shared native task dialog.

## Out of scope

- Making every task-create write fully transactional after provider resolution.
- Adding a separate repository-registration wizard.
- Trusting a browser descriptor when the inspect action is unavailable.
- Changing built-in GitHub, GitLab, or Azure DevOps URL parsing.
- Changing how a plugin authenticates to its external code host.

## System design

- [Plugin Repository Task Creation System Design](../system-design/repository-provider-task-creation.md)
