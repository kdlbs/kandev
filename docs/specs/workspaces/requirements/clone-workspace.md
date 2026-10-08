---
status: active
system: workspaces
created: 2026-10-07
owners:
  - kandev
---

# Workspace cloning requirements

## Overview

Users can create an independent workspace from an existing workspace's setup,
avoiding repeated GitHub connection and query configuration. The user selected
full workspace setup: general settings, repositories, workflow definitions,
and GitHub connection/query settings, starting with no tasks or sessions.

The workspace system owns creation of the new configuration graph. Integration
authentication and query interpretation retain their existing owners. This is
configuration cloning within one Kandev installation, not copying Git checkouts.

## Requirements

### REQ-WORKSPACES-CLONE-001: Independent workspace setup

**Intent:** Reuse a configured workspace without sharing mutable configuration
or duplicating its work history.

#### Acceptance criteria

- **AC-WORKSPACES-CLONE-001.1:** Cloning a supported workspace with a nonblank
  new name shall create a separate workspace with that trimmed name, the source
  description, four default selections, task prefix, and idle-suspension settings.
  Defaults shall reference existing selections available to the creating user;
  unavailable selections shall reject the clone with an actionable error.
- **AC-WORKSPACES-CLONE-001.2:** The clone shall retain each live repository's
  source identity, branch/worktree settings, inline and named scripts, and copy-file settings,
  together with branch policies and ordered repository sets. Provider-managed
  checkout paths shall be prepared independently when needed. User-managed
  local repositories shall continue to refer to the same local source without
  copying, changing, or deleting its files during cloning.
- **AC-WORKSPACES-CLONE-001.3:** User-managed visible workflows and all their
  steps shall retain ordering, prompts, profiles, behavior, and internal step
  references in the destination. Copied references shall resolve within the new
  graph. Synced definitions shall become independent editable definitions. A
  source with no eligible workflow shall produce one ordinary Kanban workflow.
- **AC-WORKSPACES-CLONE-001.4:** The clone shall retain the workspace GitHub
  automation connection and host, repository scope, task Git credential mode,
  saved queries and their default markers, configured PR/issue default queries,
  and quick-action presets. An absent source connection shall remain absent.
  Personal GitHub OAuth authentication shall require its existing sign-in flow.
- **AC-WORKSPACES-CLONE-001.5:** Copied workspace-owned records shall have new
  identities and creation timestamps; edits and deletion in either workspace
  shall leave the other's configuration intact. Shared install-wide profiles,
  executors, environment definitions, GitHub App registrations/installations,
  and user-managed local sources retain their existing independent contracts.
- **AC-WORKSPACES-CLONE-001.6:** The clone shall start with no tasks, sessions,
  worktrees, task counters, watcher subscriptions, queued work, personal view
  state, or copied memberships. Cloning shall not start an agent, run a script,
  contact GitHub to create resources, or activate a watcher.
- **AC-WORKSPACES-CLONE-001.7:** Copied configuration shall survive reload and
  backend restart. Entering the clone's GitHub dashboard shall apply its copied
  PR and issue query defaults through the existing query-selection behavior.

### REQ-WORKSPACES-CLONE-002: Authorized and complete creation

**Intent:** A clone either becomes a usable configuration or leaves no new
workspace or copied credentials behind.

#### Acceptance criteria

- **AC-WORKSPACES-CLONE-002.1:** The caller shall require source workspace
  management permission, integration configuration permission for copied
  credentials, and ordinary workspace creation/destination placement admission.
  Destination ownership and reach shall derive from the creating identity and
  normal creation policy, not copied source memberships or client-supplied ownership.
- **AC-WORKSPACES-CLONE-002.2:** Validation, missing source, inaccessible
  references, credential-read/encryption failure, insert failure, or cancellation
  before commit shall leave no destination workspace, configuration, PAT secret,
  or successful creation notification. Source data shall remain intact.
- **AC-WORKSPACES-CLONE-002.3:** GitHub PAT credentials shall be stored separately
  for the destination; disconnecting either workspace shall not disconnect the
  other. Cloning an App connection shall preserve both registration and
  installation identity without duplicating registration credentials. Tokens
  shall never appear in clone responses, logs, or notifications.
- **AC-WORKSPACES-CLONE-002.4:** Office workspaces and the managed Improve Kandev
  workspace shall be unavailable for this ordinary setup clone. Hidden/system
  workflows and explicit references to source tasks, sessions, or excluded
  workflows shall not be retained in a purported independent clone. An invalid
  included reference shall reject creation instead of silently redirecting it.
- **AC-WORKSPACES-CLONE-002.5:** An unsupported GitHub connection source shall
  reject cloning with a clear error rather than fall back to another identity.
  An inactive connection may retain its inactive state; cloning shall not claim
  to repair expired or revoked external access.

### REQ-WORKSPACES-CLONE-003: Desktop and phone clone flow

**Intent:** The same clone is available from workspace settings on desktop and phone.

#### Acceptance criteria

- **AC-WORKSPACES-CLONE-003.1:** Each manageable supported workspace in the
  workspace settings list shall expose a compact actions menu beside its name,
  separate from its overview link, with an accessible Clone action. The viewed
  workspace's settings header shall offer the same action on every tab.
  Read-only and unsupported workspaces shall not offer an executable clone action.
- **AC-WORKSPACES-CLONE-003.2:** The clone form shall identify its source, offer
  an editable localized copy-name suggestion, and explain included setup and
  exclusions, including separate personal sign-in and uncopied repository secrets.
  A blank name shall disable submission. Cancel shall create nothing and return
  focus to the initiating action.
- **AC-WORKSPACES-CLONE-003.3:** One pending submission shall disable repeat
  submission and keep the form visible. A failed request shall retain the name
  and present an error with a retry path. An uncertain response shall prompt
  checking the workspace list before retrying; the client shall not automatically
  repeat a creation request.
- **AC-WORKSPACES-CLONE-003.4:** Success shall add the new workspace once to the
  list and navigate to its settings overview, without changing the source or
  automatically replacing the globally active workspace. Reloaded settings
  shall expose the copied repositories, workflows, and GitHub configuration.
- **AC-WORKSPACES-CLONE-003.5:** Desktop shall use a compact dialog; phone shall
  use an inset bottom drawer with one scroll owner, safe-area clearance, labeled
  touch controls of at least 44px, and no horizontal overflow. Resizing shall
  preserve the draft. All new copy shall use the complete locale catalogs.

## Exclusions

- Task history, runtime resources, Office agents/projects/routines, hidden
  managed workflows, workspace coordinators, general automations, and workflow
  synchronization subscriptions.
- General workspace/repository secret values and their bindings, other provider
  integration connections, plugin grants, personal GitHub OAuth tokens, and
  browser-local or personal task views.
- Cross-install export/import, physical repository duplication, connection
  repair, a globally synchronized snapshot during concurrent source edits, or
  automatic idempotent replay after an ambiguous network result.

## References

- [System design](../system-design/clone-workspace.md).
- [Creation](creation.md).
- [GitHub authentication](../../integrations/requirements/github-authentication.md).
- [Query default selection](../../ui/requirements/github-saved-query-defaults.md).
- [Implementation plan](../../../plans/clone-workspace/plan.md).
