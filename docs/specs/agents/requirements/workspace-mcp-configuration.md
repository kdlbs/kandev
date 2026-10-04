---
status: active
system: agents
created: 2026-09-01
owners:
  - kandev
---

# Workspace MCP Configuration Requirements

## Overview

Workspace MCP configuration lets users install reusable MCP server definitions
once, then enable them for repositories, agent profiles, tasks, and task agent
sessions. The agent system owns this contract because it owns agent profiles,
provider capabilities, and the runtime configuration sent to an agent.

## Terminology

- **MCP definition:** A workspace-owned, reusable server configuration. It
  contains non-secret settings and references to workspace secrets.
- **Marketplace entry:** Discovery metadata from Kandev's curated catalog or
  the public MCP Registry. An entry is not executable until a user reviews and
  adds it as an MCP definition.
- **Workspace installation:** The act of saving a reviewed MCP definition in a
  workspace. This action does not download, connect to, or start a server.
- **Runtime materialization:** The preparation of a managed package inside a
  task executor when an effective MCP set first uses that package.
- **Scope selection:** MCP definition identifiers for a repository, a profile
  in one workspace, a task, or one task agent session.
- **Effective MCP set:** The additive union of applicable scope selections for
  one attachment attempt, after duplicate definitions are collapsed.
- **Desired revision:** The latest saved selection state for a task agent
  session.
- **Applied revision:** The selection state used by the current successful MCP
  attachment attempt.

## Requirements

### REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001: Workspace MCP catalog

**Intent:** Let workspace members configure reusable MCP servers without
editing agent-specific JSON.

**User story:** As a workspace member, I want to configure an MCP server once,
so that I can reuse it across agent contexts.

#### Acceptance criteria

- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.1:** An authorized workspace
  member can list, create, edit, disable, and delete workspace MCP definitions.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.2:** A custom MCP definition can
  describe a remote endpoint, a managed package, or an existing executable.
  It can include non-secret settings and workspace-secret bindings.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.3:** The system rejects duplicate
  runtime names within one workspace and reserves Kandev-owned runtime names.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.4:** Disabling a definition keeps
  its saved scope selections, but excludes it from new effective MCP sets.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.5:** Before deleting a selected
  definition, the system shows the affected scopes and requires confirmation.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.6:** A workspace member cannot
  view or mutate definitions that belong to another workspace.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.7:** Saving a custom definition
  does not download, connect to, or start the server.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.8:** A custom managed package uses
  an exact package identity and version. An existing-executable definition
  states that the executable must already exist in the task executor.

### REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002: Marketplace discovery and setup

**Intent:** Provide useful curated starting points and searchable public
discovery without treating registry metadata as trusted code.

**User story:** As a workspace member, I want to find and review published MCP
servers, so that I can configure one without reconstructing its metadata.

#### Acceptance criteria

- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.1:** The marketplace shows a
  small Kandev-curated catalog and searchable entries from the public MCP
  Registry.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.2:** Registry search uses the most
  recent successful local catalog when the public service is unavailable. The
  UI identifies stale or degraded results.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.3:** Adding an entry requires
  a review of its publisher metadata, selected package or remote endpoint,
  transport, requested configuration, and secret bindings.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.4:** Marketplace discovery and
  review do not download, launch, or connect to the listed server.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.5:** An installed definition
  records its registry identity and selected version, and a later registry
  change does not silently mutate the installed definition.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.6:** Deprecated or deleted
  registry entries are not offered for a new installation. Existing workspace
  definitions remain visible and usable until a member changes them.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.7:** The marketplace states that
  registry metadata is publisher supplied and is not a Kandev security review.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.8:** A remote entry creates a
  definition that connects to its endpoint when an agent receives it. Kandev
  does not install a local package for that entry.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.9:** A compatible package entry
  pins its package identity and version. Kandev materializes it in the selected
  task executor when an agent first uses it.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.10:** Runtime materialization uses
  a Kandev-managed cache and does not modify repository files or lockfiles.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.11:** The marketplace disables an
  unsupported package choice and explains which runtime or executor capability
  is missing. Another compatible package or remote remains selectable.

### REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003: Additive scope selection

**Intent:** Let each execution context contribute capabilities without
silently removing capabilities selected by another context.

**User story:** As a workspace member, I want scope-specific MCP selections, so
that agents receive the required capabilities.

#### Acceptance criteria

- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.1:** A member can select enabled
  workspace MCP definitions for repositories, profiles, tasks, and task agent
  sessions.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.2:** Agent-profile selections are
  workspace contextual. A selection for a global profile in one workspace does
  not affect the same profile in another workspace.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.3:** The effective MCP set is the
  union of repository, workspace-profile, task, and task-session selections. It
  includes every repository in the task.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.4:** Selecting the same MCP
  definition in multiple scopes creates one effective server. The effective
  result reports every contributing scope.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.5:** Scope selections cannot
  reference a definition from another workspace or a disabled definition.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.6:** A task keeps its saved task
  selection when agents are added, profiles change, or the task is resumed.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.7:** The selection UI distinguishes
  inherited MCP servers from additions at the current scope without providing
  subtraction or per-scope overrides.

### REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-004: Effective MCP attachment

**Intent:** Resolve one deterministic, auditable MCP configuration for every
agent attachment attempt.

#### Acceptance criteria

- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.1:** Before each agent attachment
  attempt, the backend resolves the effective MCP set from current definitions
  and applicable saved selections.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.2:** The backend sends the full
  effective set through the agent's supported ACP or passthrough mechanism in
  deterministic runtime-name order.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.3:** Executor or provider
  transport filtering happens after scope composition and remains visible in
  session-owned MCP attachment evidence.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.4:** Each attachment attempt
  records the definition revision and contributing scopes for every effective
  server without recording secret values.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.5:** Editing a definition or
  selection does not mutate an MCP configuration already delivered to a
  running turn.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.6:** If a secret binding cannot be
  resolved, the affected server is not delivered and the user receives a
  sanitized, actionable error.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.7:** Resolved secret values stay out
  of Kandev-owned storage, API responses, WebSocket payloads, logs, metrics, and
  attachment evidence. Delivery to the agent still places them in the task
  executor as process arguments or agent configuration files, and setup states
  that consequence before a member binds a secret.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.8:** A stdio server whose command
  cannot be resolved in the task executor is reported as filtered attachment
  evidence with a typed reason. It is not dropped silently.

### REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005: Idle-session reconfiguration

**Intent:** Apply MCP selection changes to an existing task agent session when
the provider can safely reconnect that session.

**User story:** As a task participant, I want to change an idle agent's MCP
servers, so that the same conversation can use new tools.

#### Acceptance criteria

- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.1:** A user can save session-level
  MCP additions while the task agent session is idle.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.2:** Kandev never replaces an MCP
  list during an active turn. The UI explains that changes can apply after the
  turn becomes idle.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.3:** When the adapter supports
  safe reconnection, Kandev reconnects the same provider session. The request
  includes its working directory and complete effective MCP set.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.4:** Kandev prefers ACP
  `session/resume` when the agent advertised it during `initialize` and falls
  back to the advertised `session/load` capability. Static Kandev agent-registry
  metadata is not a substitute for either advertisement, and Kandev does not
  claim live reconfiguration from advertisement alone.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.9:** A `session/load` fallback
  preserves scheduled agent wakeups and session runtime telemetry, or the user
  is told before the fallback runs that they will be lost. Suppressed history
  replay does not duplicate transcript content.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.5:** A successful reconnect makes
  the desired revision the applied revision and creates a new MCP attachment
  attempt.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.6:** If reconnection is
  unsupported, the desired selection remains saved and the UI states that it
  will apply on the next agent start.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.7:** If reconnection fails, the
  prior applied revision remains authoritative for the current attachment.
  The UI shows the failure and offers retry or next-start application.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.8:** Session reconfiguration does
  not create a new task session, discard the transcript, or change the selected
  agent profile.

### REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006: Responsive and accessible controls

**Intent:** Make catalog management and scope selection complete on desktop
and mobile.

#### Acceptance criteria

- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.1:** Workspace settings provides
  a dedicated MCP servers destination with configured and marketplace views. Its
  name and description distinguish it from the existing External MCP settings
  page, which exposes Kandev's own MCP server to outside clients.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.2:** Repository settings, agent
  profile settings, task creation, and task agent creation provide an MCP
  selection control with inherited-origin summaries.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.3:** On a phone, catalog setup uses
  a dedicated full-height surface. Short selection lists use a bottom sheet or
  direct route with one scroll owner.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4:** Interactive rows and actions
  provide keyboard access, visible focus, and a touch target of at least 44 by
  44 CSS pixels.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5:** Loading, empty, stale,
  unsupported, pending, success, and failure states remain readable without
  horizontal page overflow.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.6:** All user-facing text uses the
  active locale and every shipped Kandev locale contains the new keys.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.7:** Repository settings places
  its MCP picker in a collapsed section. The section summary shows the selected
  count without consuming the expanded form space.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.8:** Task creation places its MCP
  selector inside collapsed Advanced settings. The summary appears only after
  the user expands that section.

### REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007: Legacy profile compatibility

**Intent:** Preserve existing raw profile MCP behavior while moving ownership
to workspace definitions and selections.

#### Acceptance criteria

- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.1:** An upgrade imports each
  existing profile MCP server into every workspace that binds the profile. It
  selects the imported definition for that workspace-profile pair.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.2:** Legacy import is idempotent
  and does not duplicate definitions or selections after a restart or retry.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.3:** Existing legacy configuration
  remains the fallback for a profile-workspace pair until that pair imports
  successfully.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.4:** The raw JSON editor is removed
  only after the workspace catalog and fallback path preserve existing launch
  behavior.
- **AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-007.5:** The upgrade does not expose a
  stored header, environment value, or other secret in list responses, logs,
  browser state, or attachment evidence.

## Out of scope

- Ratings, reviews, download counts, automatic popularity ranking, and
  marketplace monetization.
- Automated security scanning, trust badges, or claims that a registry entry is
  safe.
- Private registry and subregistry configuration in the first release.
- Automatic package download, server launch, or endpoint connection during
  marketplace discovery.
- Subtractive selections, precedence rules, and per-scope configuration
  overrides for one definition.
- Changing MCP servers inside an active agent turn.
- Using marketplace entries without first installing a workspace definition.
