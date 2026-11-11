---
status: draft
system: integrations
created: 2026-11-02
owners:
  - kandev
---

# Azure DevOps Connector Plugin Requirements

## Overview

Azure DevOps Services users need Boards, work items, Azure Repos pull requests,
watches, and task workflows in Kandev. Today a first-party integration provides
them. This capability moves the whole connector into a runtime plugin, modeled
on the [Bitbucket connector plugin](bitbucket-plugin.md), using only the host
contracts that exist today. Kandev removes the first-party integration in one
change and lists the plugin in its registry in another; see
[Azure DevOps leaves core](../../plugins/requirements/azure-devops-extraction.md).
The plugin also gains Microsoft account sign-in as an alternative to a personal
access token (PAT).

The integrations system owns this contract because it covers provider
credentials, external work-item and pull-request synchronization, and the
provider-specific screens.

## Terminology

- **Connector plugin:** the runtime plugin `kandev-plugin-azure-devops`, built
  in the dedicated repository `JnManso/kandev-plugin-azure-devops`. The
  repository can later be transferred to the `kdlbs` organization.
- **Connection:** one workspace's Azure DevOps Services organization, its
  default project, and exactly one usable credential.
- **Microsoft account connection:** a connection whose credential comes from a
  Microsoft Entra ID sign-in with a work or school account.
- **Application (client) ID:** the Microsoft Entra public client used for
  sign-in. A workspace can enter one; otherwise the plugin uses the
  installation default from the plugin's operator configuration.
- **Built-in integration:** the first-party Azure DevOps integration that this
  plugin replaces.
- **Shared executor profile:** an executor profile that is not owned by a
  single user. With authentication disabled, every profile is shared.

## Requirements

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001: Connector plugin packaging and ownership

**Intent:** Azure DevOps ships and evolves independently of Kandev, like the
other code-host connector plugins, without new host contracts.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.1:** When the plugin is published,
  Kandev's plugin registry shall list `kandev-plugin-azure-devops` with its
  dedicated repository. Its manifest shall pin `min_kandev_version` to the
  first released Kandev version in which the built-in integration is removed,
  and shall never name an unreleased version.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.2:** When the plugin is active on
  such a host, it shall own the repository provider identity `azure_devops` and
  the integration settings identity `azure-devops`. Existing repositories with
  provider `azure_devops` shall resolve to the plugin without any row being
  rewritten.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.3:** The plugin shall own Azure
  DevOps REST payloads, authentication, token renewal, pagination, rate-limit
  handling, health polling, watches, task links, and its screens. It shall use
  only the host's existing action, repository-provider, credential-broker,
  review-provider, task-action, reference-source, agent-tool, state, secret,
  and user-storage contracts.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.4:** The plugin shall follow
  Kandev's current unsigned marketplace trust contract, as the Bitbucket
  plugin does.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001.5:** The plugin UI shall localize
  every user-facing string for every Kandev locale through the plugin
  translation contract.

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002: Workspace connection with PAT or Microsoft account

**Intent:** A workspace connects to Azure DevOps with either a PAT or a
Microsoft account. Neither method requires an application registration by the
user when the installation provides a default application.

**User story:** As a workspace user whose organization restricts PATs, I want
to sign in with my Microsoft account from Azure DevOps settings, so that Kandev
can use Azure DevOps without a personal access token.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.1:** When a user opens
  Settings > Integrations > Azure DevOps, the system shall show the
  organization URL, an Authentication method choice with Personal access token
  and Microsoft account, and the default project. An existing connection shall
  show its stored method. A new connection shall default to Personal access
  token.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.2:** When the user selects
  Microsoft account, the system shall hide the PAT field and show an
  Application (client) ID field followed by a Sign in with Microsoft action.
  The field shall show the application that signed in, and otherwise offer the
  installation default as its placeholder. The action shall be disabled while
  the organization URL is invalid, or while the field, the application that
  signed in, and the installation all provide no client ID. A localized explanation shall
  accompany the disabled state.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.3:** When the user activates Sign
  in with Microsoft, the system shall open Microsoft sign-in in a popup for the
  organization's directory. Sign-in shall return to the origin of the page the
  user is on, so no redirect setting needs maintaining when the port or host
  changes. The user shall never enter a tenant ID, secret, or token.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.4:** When sign-in completes, the
  system shall close the popup, save the connection with the Microsoft account
  method, and show the signed-in display name and the application in use
  without a page reload. When the popup cannot return to Kandev, the user shall
  be able to paste the final redirect address and complete the same sign-in.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.5:** When the user cancels, denies
  consent, closes the popup, or the attempt is older than ten minutes, the
  system shall leave any existing connection and credential unchanged and show
  a localized message. An attempt shall complete only once, only for its
  workspace, and only for the Kandev user who started it.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.6:** When a workspace switches
  method, the system shall keep the previous credential until the new one is
  saved successfully, then delete it. Changing the organization of a Microsoft
  account connection shall require a new sign-in for the new organization.
  Saving only the default project shall not.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.7:** When the user selects Sign out,
  the system shall delete the Microsoft credential, keep the organization,
  default project, application, watches, and task links, and show Sign in with
  Microsoft again. Deleting a connection shall delete its credential of either
  method.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.8:** When a connection is copied to
  another workspace, the system shall copy the organization, default project,
  and application, and shall not copy any credential. A copied Microsoft
  account connection shall require sign-in in the target workspace.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.9:** The system shall never show,
  log, return through an API, or put in a URL a PAT or a Microsoft access or
  renewal credential, and shall store them only in the plugin's encrypted
  secret storage. Executors shall receive only short-lived Git credential
  leases, never the stored credential.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.10:** Desktop and phone layouts
  shall expose the same method choice, sign-in, paste fallback, signed-in
  identity, sign-out, and errors. Phone controls shall be touch-sized, and the
  paste fallback shall be usable without horizontal scrolling.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.11:** The integrations card and the
  settings header shall offer an enable switch for each workspace. It shall
  default to on, save through the native Save changes bar, and keep its value
  after a reload. While the value cannot be read, the switch shall stay hidden
  rather than show a guess. The switch shall drive only Kandev's settings menu
  badge and "Hide disabled integrations" row; connections, watches, and
  credentials shall keep working while it is off.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.12:** The connection form shall
  save through the native Save changes bar, like the other settings sections,
  and shall have no separate save button. The bar shall appear only when the
  form differs from the saved connection, and Kandev shall warn before leaving
  with unsaved changes. When the form cannot be saved, the bar shall say why:
  a token is required, the organization of a Microsoft account changed and
  needs a new sign-in, or the Microsoft sign-in expired. The form shall show
  the organization message next to the sign-in buttons. Microsoft sign-in and
  the application ID shall stay on the sign-in buttons, not on the bar.

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003: Connection health and renewal

**Intent:** A connection stays usable without user interaction for as long as
its credential allows, and tells the user when it needs attention.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003.1:** When an operation starts after
  a Microsoft access token expired, the system shall renew it first with the
  application that signed in, and shall not fail because of the expiry.
  Concurrent operations shall share one renewal. A sign-in that completes
  during a renewal shall win.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003.2:** When Microsoft requires a new
  sign-in, or a PAT is rejected, the system shall mark the connection unhealthy
  in settings and integration health with a localized message. It shall keep
  every setting, watch, and task link, and shall resume after a new sign-in or
  PAT without other reconfiguration.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003.3:** The system shall check each
  connection's health at least every 90 seconds while the plugin runs.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003.4:** When a current-user filter
  such as "assigned to me" runs, the system shall resolve it to the identity of
  the connection's credential.

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004: Browse Boards, work items, and pull requests

**Intent:** Every Azure DevOps screen of the built-in integration keeps working
with the same capabilities after the move.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.1:** When the plugin is active and
  a workspace is connected, the system shall show an Azure DevOps navigation
  entry that opens the plugin's Azure DevOps page. The plugin's in-app icons
  shall use the surrounding text color, like Kandev's other navigation icons.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.2:** The page shall offer work-item
  search, Boards, and pull-request modes. It shall offer project, team, and
  board scope selectors and query filters, and shall load further pages of
  projects, teams, and results on demand.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.3:** When the user opens a work
  item, the system shall show its fields, description, comments, and linked
  Kandev tasks. The user shall be able to change its state and assignee from
  Kandev.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.4:** When the user opens a pull
  request, the system shall show its status, reviewers, and review feedback.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.5:** The user shall be able to
  save, reorder, rename, and delete views. Workspace query presets and quick
  actions shall be configurable in settings. A user's browse preferences shall
  persist per user.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.6:** Desktop and phone layouts
  shall expose the same browse capabilities with phone-native composition.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.7:** Work-item mode shall offer a
  visible search box that finds a work item by number or by words in its
  title, with the WIQL query kept as an advanced option. Project, team, board,
  and repository selectors shall filter their options by the typed text and
  list every option Azure DevOps returns, beyond its 100-record pages. The
  repository selector shall offer only the selected project's repositories,
  and the **All** pull-request status shall return pull requests of every
  status.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.8:** Picking the selected option of
  a project, team, board, or repository selector again shall keep it selected.
  When no project is selected, the Board view shall still offer the project
  selector. A title search shall find work items whose title contains every
  typed word, in any order. A search, from the button or the Enter key, shall
  run only when its required filters are set, and a failed search shall not
  keep the previous results or their count.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.9:** When the Board opens on a
  project, it shall show the team and board last opened in that project and
  load that board without waiting for the project's team list, then keep them
  only if the list still contains them. While teams, boards, or the board
  itself load, the Board view shall show a loading indicator.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.10:** A Board card shall offer the
  same task actions as a work-item result, without opening or moving the card.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.11:** The board selector shall
  offer only backlog levels that have a Kanban board, so the task level is
  left to the sprint taskboard. A failed board load shall show its error
  without the previous board's cards.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.12:** The scope bar shall show
  the current mode's shortcuts and mark the one the filters match. Closed,
  **Advanced** shall show how many of its filters are set.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004.13:** Pull-request creator and
  reviewer filters shall pick by name: anyone, me, or a creator or reviewer
  of the repository's recent pull requests. No identity IDs.

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005: Tasks, links, and pull-request status

**Intent:** Azure DevOps work items and pull requests connect to Kandev tasks
through the same native surfaces as other code-host plugins.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.1:** When the user starts a task
  from a work item or pull request, the system shall open Kandev's task
  creation dialog prefilled from it, and shall link the created task to it.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.2:** In a task's **Link** submenu,
  the system shall offer **Azure DevOps Pull Request** and **Azure DevOps Work
  Item** entries that open Kandev's task link dialog. Linked items shall be
  unlinkable.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.3:** When a task has a linked pull
  request, the system shall show its state, checks, and review status in the
  native task indicators and review detail surface, and shall refresh them at
  least every 90 seconds while visible.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.4:** Native **Create PR** shall
  create an Azure Repos pull request for a task checkout branch that has
  commits and no linked pull request, and shall link it to the task.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.5:** The composer shall offer `#`
  references to Azure DevOps work items and pull requests of the connected
  organization.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005.6:** An agent working in a task
  shall be able to create and link an Azure Repos pull request through a
  Kandev agent tool, without the `az` CLI or a credential in its environment.

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006: Repositories and Git credentials

**Intent:** Azure Repos repositories behave like other plugin-provided
repositories in task creation and on executors.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006.1:** When the user creates a task,
  the system shall list the connection's Azure Repos repositories and their
  branches in the native repository and branch pickers, and shall accept an
  Azure Repos URL.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006.2:** When a task uses a private
  Azure Repos repository, Kandev's clone and refresh and the agent's fetch and
  push shall authenticate with the workspace connection through the host's Git
  credential broker. Local and worktree executors shall need no configuration.
  Docker, remote Docker, SSH, Kubernetes, and Sprites executors shall need the
  host's public broker address, as for the Bitbucket plugin, and the plugin
  documentation shall say so.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006.3:** The plugin shall issue a Git
  credential only for a repository of the task's workspace connection and only
  for the exact task, session, repository, host, and path that Kandev verified.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006.4:** When the task's repository has
  no recorded clone URL, the plugin shall match the requested path against the
  repository's provider host (`https://dev.azure.com`), project, and name in
  the connected organization. A repository with neither a clone URL nor that
  identity shall receive no credential.

### REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007: Watches

**Intent:** Work-item and pull-request watches keep creating tasks
automatically.

#### Acceptance criteria

- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.1:** The user shall be able to
  create, edit, enable, disable, delete, trigger, preview a reset of, and reset
  work-item watches and pull-request watches in settings. A reset shall make
  every current match eligible to create a task again; tasks already created
  shall be kept.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.2:** When a watch matches a new
  work item or pull request, the system shall create at most one task for it,
  with the watch's workflow, step, agent profile, executor profile, prompt,
  repository, and base branch, and shall start it when the watch requests that.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.3:** The watch editor shall offer
  only agent and executor profiles that the plugin can launch with, which are
  the shared ones. When a saved watch's profile stops being available, the
  system shall disable the watch and show a localized reason in settings.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.4:** Tasks created by a watch shall
  carry the watch identity and the item identity in task metadata, so the
  plugin can show them on the task and avoid duplicates after a restart.
- **AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.5:** A task created by a watch
  shall have a title that fits Kandev's task title limit, shortened with an
  ellipsis when the item title is long, and a prompt with the full item title.

## Out of scope

- Azure DevOps Server (on-premises) and hosts other than `dev.azure.com`.
- Microsoft personal accounts, service principals, managed identities,
  client-credential and device-code sign-in.
- Per-user Azure identities within one workspace. One credential serves the
  workspace.
- A Kandev-published Entra application. Users or operators supply the client
  ID.
- Migration of built-in integration data. Users reconnect, re-enter the
  credential, and recreate watches and saved views after installing the plugin.
- Known differences from the built-in integration, accepted until the host
  offers generic contracts for plugin providers: linked pull requests do not
  feed the task delivery ledger; watch-created tasks are not classified as
  review-watch or issue-watch tasks in the sidebar; watches cannot launch on
  executor profiles owned by a single user; watches cannot delete the tasks
  they created, so there is no cleanup policy and resetting or deleting a
  watch keeps its tasks; the enable switch cannot hide the plugin's main
  sidebar entry, because plugin navigation items have no availability gate.
- New Azure DevOps capabilities beyond the built-in integration, except
  Microsoft account sign-in, the agent pull-request tool, the teams and
  sprints capability in [its own requirements](azure-devops-plugin-sprints.md),
  and Board card children in
  [their own requirements](azure-devops-plugin-board-children.md).
