---
status: current
system: agents
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-004
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007
---

# Workspace MCP Configuration System Design

## Purpose and boundaries

The agent system owns workspace MCP definitions, scope selections, effective
runtime composition, and delivery to agent runtimes. The task system supplies
repository, task, and session identities. The workspace secret service owns
secret values. Session-owned MCP observability remains governed by
[ADR-2026-07-30](../../../decisions/2026-07-30-session-owned-mcp-observability.md).

This design extends `apps/backend/internal/agent/mcpconfig`. It replaces the
raw profile-only configuration path with workspace definitions and explicit
associations. It does not create a second MCP configuration subsystem.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001` | [Catalog domain](#catalog-domain), [Persistence](#persistence) |
| `REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002` | [Marketplace aggregation](#marketplace-aggregation), [Marketplace installation](#marketplace-installation) |
| `REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003` | [Selection model](#selection-model), [Effective-set resolution](#effective-set-resolution) |
| `REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-004` | [Runtime delivery](#runtime-delivery), [Observability](#observability) |
| `REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005` | [Idle-session reconfiguration](#idle-session-reconfiguration), [Failure and recovery](#failure-and-recovery) |
| `REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006` | [Frontend surfaces](#frontend-surfaces), [Responsive behavior](#responsive-behavior) |
| `REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-007` | [Legacy migration](#legacy-migration) |

## Existing boundaries

The current implementation has these relevant paths:

- `internal/agent/mcpconfig` resolves one `AgentProfileMcpConfig` and adapts it
  to ACP or provider-specific passthrough configuration.
- `internal/agent/settings/models/mcp.go` and the
  `agent_profile_mcp_configs` table store raw server maps by profile.
- `internal/agent/runtime/lifecycle/manager_profile.go` resolves MCP servers
  from the effective profile before an agent starts.
- `internal/agentctl/server/adapter/transport/acp/adapter_session.go` already
  accepts an MCP server list on both `NewSession` and `LoadSession`, filters it
  through `filterMcpServersWithDecisions`, and emits attachment evidence for
  every included and filtered server. It has no `session/resume` request.
- `internal/agentctl/server/instance` resolves stdio MCP commands with
  `exec.LookPath` in `buildMcpServerConfigs` and drops unresolvable entries with
  a warn log.
- `internal/agent/mcpconfig/passthrough.go` holds the per-CLI strategies. Claude,
  Cursor, Pi, and OpenCode write MCP configuration files; Codex passes each
  server as `-c mcp_servers.*` process arguments.
- `types/streams/mcp_attachment.go` projects session-owned delivery and
  attachment evidence.
- `profile-mcp-config-card.tsx` is the raw JSON profile editor that this design
  retires after migration. `profile-edit/mcp-policy-card.tsx` (transport policy),
  `mcp-task-agent-profile-default-settings.tsx`, and `custom-tui-mcp-card.tsx`
  are separate profile surfaces that this design keeps.
- `app/settings/external-mcp` exposes Kandev's own MCP server to outside clients.
  It is the opposite direction from this design and is not replaced.

## Components and responsibilities

### Catalog domain

`internal/agent/mcpconfig` gains a workspace catalog service and repository
interfaces. The service owns:

- definition validation and normalized runtime names.
- transport and package or endpoint configuration.
- secret-reference validation without reading secret values into responses.
- catalog CRUD, impact counts, disable, and guarded delete.
- installed source metadata and immutable installation provenance.
- definition revisions used by attachment snapshots.

An `MCPServerDefinition` has these stable fields:

| Field | Purpose |
| --- | --- |
| `id` | Stable Kandev identifier used for deduplication |
| `workspace_id` | Authorization and lifecycle boundary |
| `runtime_name` | Name delivered to the agent. It is unique per workspace. |
| `display_name`, `description` | User-facing identity |
| `enabled` | Inclusion gate for new effective sets |
| `execution_mode` | `remote`, `managed_package`, or `existing_executable` |
| `transport` | Normalized stdio, HTTP, or SSE transport |
| `configuration` | Non-secret executable, package, arguments, URL, and option template |
| `secret_bindings` | Named references into workspace secrets |
| `source` | `curated`, `registry`, `custom`, or `legacy_import` |
| `source_identity` | Registry name/version/package or curated template version |
| `revision` | Monotonic revision for attachment evidence |

The catalog reserves `kandev` and any future Kandev-owned runtime names. The
service rejects a second definition with the same normalized runtime name.

### Selection model

Selections store definition identifiers, not copied server configuration.
Separate typed associations preserve foreign keys and ownership checks:

- `workspace_agent_profile_mcp_selections(workspace_id, profile_id,
  mcp_server_id)`.
- `repository_mcp_selections(repository_id, mcp_server_id)`.
- `task_mcp_selections(task_id, mcp_server_id)`.
- `task_session_mcp_selections(task_session_id, mcp_server_id)`.

The workspace column on profile selections is required. Global agent profiles
remain install-wide, but their MCP choices are workspace contextual. The agent
profile settings page supplies a workspace selector for a global profile. A
workspace-scoped profile uses its owning workspace directly.

Repository, task, and session services expose typed selection operations. They
delegate definition and workspace validation to `mcpconfig.Service`. No
generic polymorphic scope endpoint or unvalidated owner ID is introduced.

### Registry aggregator

`mcpconfig/registry` implements an aggregator client for the public MCP
Registry. It reads the versioned Registry API and persists normalized discovery
records separately from installed definitions.

The sync worker performs a full cursor-paginated sync when no successful
cursor exists. Later syncs use `updated_since` from the last successful sync.
It records status changes, including deprecated and deleted entries. A
scheduled sync runs at most hourly. An authorized manual refresh uses the same
single-flight worker and rate limit.

Registry responses are untrusted discovery data. The aggregator applies
response-size, page-count, request-time, and string-length limits before
storage. It never follows arbitrary package or endpoint URLs during sync.

Kandev-curated entries are versioned embedded templates. The marketplace query
merges curated templates with the last successful public registry cache and
labels the source. Curated status means Kandev provides a setup template. It is
not a security guarantee.

### API handlers

Workspace-authorized HTTP handlers expose these resources under the existing
API version:

- `GET|POST /api/v1/workspaces/{workspaceID}/mcp-servers`.
- `GET|PATCH|DELETE /api/v1/workspaces/{workspaceID}/mcp-servers/{serverID}`.
- `GET /api/v1/workspaces/{workspaceID}/mcp-marketplace`.
- `POST /api/v1/workspaces/{workspaceID}/mcp-marketplace/refresh`.
- `POST /api/v1/workspaces/{workspaceID}/mcp-marketplace/install`.
- `GET|PUT .../mcp-selections` on the existing agent-profile, repository, task,
  and task-session resources.

Mutation requests contain definition IDs and expected revisions. Responses
return sanitized definition summaries, selection revisions, inherited origins,
and effective application state. They never return resolved secret values.

Task creation and task-session creation requests gain optional
`mcp_server_ids`. An omitted field and an empty list both add no scope-specific
servers. Neither value disables inherited servers.

### Frontend surfaces

The workspace settings tab manifest (`lib/settings/workspace-settings-tabs.ts`,
read by the tab strip, each tab page heading, the workspace list links, and the
settings menu's Workspaces branch) gains an `MCP servers` destination. Because
Kandev already has an External MCP settings page for the opposite direction,
this destination's label and description name the direction explicitly rather
than reusing a bare `MCP` title. Its desktop layout provides configured and
marketplace views. Configured cards show
state, transport, source, and selection impact. Marketplace results support
search, source/status labels, review, and setup.

A shared `MCPSelectionPicker` serves:

- the workspace-contextual card in agent profile settings.
- repository settings.
- task creation.
- new task agent session creation.
- idle task-session settings.

Repository settings renders the picker inside a collapsed section. The closed
section shows its selected count. It does not reserve the height of the picker.

Task creation renders the picker inside collapsed Advanced settings. The main
task form does not show an MCP summary before the user opens Advanced settings.
The expanded section shows inherited origins and task-level additions.

The picker separates inherited rows from additions at the current scope. Each
inherited row lists its origins. It does not show a misleading checkbox that
implies subtraction.

Only the raw JSON editor is retired. The profile transport policy card remains,
because it configures the executor and provider filtering that runs after scope
composition. The task-agent default settings card and the custom TUI MCP card
also remain and are not merged into the picker.

Frontend state uses typed API clients and query or store ownership consistent
with the surrounding settings surface. Raw transport configuration stays in
the catalog editor. Selection surfaces exchange only definition IDs and
sanitized summaries.

## Marketplace installation

Marketplace workspace installation is an explicit copy operation:

1. The browser requests one cached marketplace entry and its available
   packages or remotes.
2. The user chooses a supported transport and reviews its executable, package,
   or remote origin.
3. The user supplies required non-secret values and chooses existing workspace
   secrets for secret inputs.
4. The backend validates the cached source identity, request revision,
   transport, runtime name, and secret references.
5. The catalog service creates a workspace definition with pinned source
   metadata. It does not download, connect to, or start the server.

The setup request cannot replace executable metadata with an arbitrary hidden
value. A user who needs different executable metadata uses the custom-definition
flow, which records `source = custom`.

Registry entries can provide `packages`, `remotes`, or both. The setup UI asks
the user to choose one compatible option. The workspace definition stores that
choice and does not switch to another option automatically.

The first release supports these Registry choices:

- remote Streamable HTTP and SSE endpoints when the selected agent transport
  supports them.
- exact-version npm packages with stdio transport through Kandev's managed Node
  runtime in the task executor.

The marketplace shows other package types as unavailable. It names the missing
materializer or executor capability. PyPI, Cargo, NuGet, OCI, and MCPB do not
become executable through a generic command fallback.

### Custom definition setup

The custom-definition flow starts with an explicit execution mode:

1. `remote` asks for an HTTP or SSE endpoint and optional header bindings.
2. `managed_package` asks for a supported package type, exact identity,
   version, arguments, and environment bindings.
3. `existing_executable` asks for a command, arguments, and environment
   bindings. Kandev does not install this executable.

Secret inputs select or create workspace secrets. The definition stores only
secret identifiers. Saving any custom mode performs validation but does not
connect to or execute the server.

## Effective-set resolution

`mcpconfig.Resolver` accepts a typed `ResolutionContext` containing:

- workspace ID.
- all task repository IDs.
- effective agent profile ID.
- task ID.
- task session ID.
- executor and provider capabilities.

Resolution follows this order:

1. Load repository selections for every task repository.
2. Load the profile selection for the execution workspace.
3. Load task and task-session selections.
4. Union rows by stable definition ID and accumulate origin descriptors.
5. Load enabled definitions and sort them by normalized runtime name.
6. Resolve secret references on the server at the last responsible moment.
7. Apply executor and provider transport policy.
8. Add Kandev's internal task-aware MCP endpoint through its existing reserved
   path.

The order does not create precedence. It only guarantees deterministic reads,
origin reporting, and delivery. A missing or disabled definition cannot create
a server entry. A name collision fails resolution rather than selecting one
arbitrarily.

The result contains a sanitized projection for UI and observability plus an
ephemeral delivery value that can include resolved secrets. The delivery value
does not enter durable storage or WebSocket payloads.

## Runtime delivery

`manager_profile.go` stops treating `AgentProfileMcpConfig` as the runtime
source of truth. Agent start, restart, resume, context reset, and workspace
rebind call the same resolver with the current typed context.

ACP delivery continues to use `McpServers` on session setup requests.
Passthrough delivery continues to use the strategies established by ADR 0014
and ADR 0020. Each strategy receives the already composed effective set. It
cannot independently read profile JSON or add user-selected servers.

Definition changes affect the next attachment attempt. They do not mutate an
already running child process or ACP turn.

### Runtime materialization

The resolver produces one of three delivery preparations for each definition:

- A remote definition becomes an ACP HTTP or SSE server entry. No local
  package is installed. The agent connects when it accepts the session setup.
- A managed npm definition invokes the exact package version through the
  Kandev-managed Node runtime inside the task executor. The package manager can
  download it on first use and reuse its executor-local cache later.
- An existing-executable definition resolves the command inside the task
  executor. Preparation fails if the executable does not exist.

The materializer runs after effective-set composition and before MCP delivery.
It writes only to a Kandev-owned executor cache. It does not write to the task
repository, change a package manifest, or change a lockfile.

`instance.Manager.buildMcpServerConfigs` currently resolves stdio commands with
`exec.LookPath` and drops an unresolvable entry with a warn log. That drop
becomes a typed filtered-attachment decision carrying the definition ID, so an
existing-executable definition that is absent from the executor stays visible in
session-owned evidence instead of disappearing.

The cache key includes package type, registry base, identity, exact version,
integrity metadata when present, platform, and architecture. A different key
creates a different materialization. Kandev never substitutes `latest`.

Package preparation executes publisher-supplied code inside the task executor.
The setup review states this consequence before the user adds the definition.
Materialization failure prevents delivery of that server and records a
sanitized failure reason in its attachment attempt.

## Idle-session reconfiguration

ACP v1 includes the full MCP list on `session/new`, `session/load`, and
`session/resume`. It does not define an in-place server-list replacement for an
active turn. Kandev therefore treats a saved session selection and its runtime
application as separate states.

`SessionMCPSelectionState` contains:

- `desired_revision`.
- `applied_revision`.
- `apply_state`: `applied`, `pending_idle`, `deferred_restart`, or `failed`.
- a sanitized failure code and summary.
- the attachment attempt that applied the revision.

The idle apply flow is:

1. The handler validates and saves a new desired session selection revision.
2. If the session has an active turn, it records `pending_idle` and performs no
   provider operation.
3. When the session becomes idle, the lifecycle manager resolves the complete
   effective set again under the existing per-session lifecycle lock.
4. The ACP adapter prefers `session/resume` when the agent advertised resume.
   It otherwise uses `session/load` when advertised.
5. The request uses the same provider session identity, working directory, and
   complete MCP list.
6. A successful response records the new applied revision and attachment
   attempt before publishing the applied state.

For passthrough agents and ACP agents without either capability, the desired
revision becomes `deferred_restart`. Kandev does not silently restart the
process or create a replacement conversation.

Capability advertisement permits an attempt but does not prove that a provider
can reconnect with changed servers. Provider request success and the normal MCP
attachment evidence remain the authoritative observations.

### Capability source

Both capabilities come from the agent's ACP `initialize` response, read on the
adapter: `sessionCapabilities.resume` for `session/resume` and the existing
`capabilities.LoadSession` for `session/load`. `agents.RuntimeConfig`
`SupportsSessionResume` is not either of them. It is a static per-agent
declaration set to `true` for nearly every registered ACP agent and it describes
Kandev's own conversation-resume behavior, so gating a `session/resume` request
on it would send that request to agents that do not implement the method.

### Session/load fallback cost

`session/load` is not a neutral way to replace an MCP list. The current adapter
implementation also clears pending wakeups, cancels the armed wakeup timer,
cancels async turn completions, clears Codex subagent correlations, Cursor task
metadata, and prompt-handoff tool tracking, increments the config generation,
clears context-window samples, and receives a full suppressed history replay.

An idle session with an armed `ScheduleWakeup` or `Monitor` therefore loses it.
The fallback consequently states what will be reset before the user confirms it,
and `session/resume` is preferred not only because it is cheaper but because it
does not replay history. A resume-capable agent never takes this path.

## Failure and recovery

Catalog mutations use optimistic revision checks. A conflict returns the
current sanitized definition so the user can reload instead of overwriting a
concurrent change.

A registry sync failure preserves the last complete cache. The sync state
records the last success, last attempt, and sanitized error. The marketplace
serves cached results with a stale or degraded marker.

Selection mutation is atomic per scope. Effective resolution never sees a
partially replaced selection set.

An idle reconnect failure does not advance `applied_revision` and does not
supersede the prior successful attachment attempt. The desired selection stays
saved. A user can retry after the session is idle or rely on the next normal
agent start. The existing session transcript and profile remain unchanged.

A secret-resolution error filters only the affected server from delivery and
records a typed failed-resolution event. The lifecycle follows the existing
MCP policy for whether other servers can proceed. It never substitutes an empty
secret value.

## Persistence

SQLite and Postgres migrations add:

- `mcp_server_definitions`.
- `mcp_registry_entries`.
- `mcp_registry_sync_state`.
- `workspace_agent_profile_mcp_selections`.
- `repository_mcp_selections`.
- `task_mcp_selections`.
- `task_session_mcp_selections`.
- desired/applied MCP selection state for task sessions.

Foreign keys bind every selection to its owner and definition. Service-level
validation also verifies that both resolve to the same workspace. Workspace,
repository, task, and session deletion uses the existing lifecycle cleanup
transactions.

Installed definitions are independent of the registry cache. Cache refresh or
entry deletion cannot mutate a definition. A definition edit increments its
revision.

### Legacy migration

Legacy `agent_profile_mcp_configs` rows remain readable during a bounded
compatibility period. An idempotent application migration runs after schema
creation:

1. Find each workspace that currently binds or uses the profile.
2. Create deterministic `legacy_import` definitions for the profile's server
   map in that workspace.
3. Create workspace-contextual profile selections for those definitions.
4. Mark the profile-workspace pair imported only after both writes commit.

A deterministic import key derived from the legacy profile ID and server name
prevents duplicates. Global profiles create separate definitions per workspace
because secret and authorization ownership is workspace local.

If import fails for one pair, runtime resolution reads its legacy profile row
as a compatibility fallback. The raw editor is retired only after the new
catalog surfaces ship and migration coverage proves fallback behavior. Legacy
tables are not dropped in the first delivery.

## Security

Every catalog and selection operation passes through existing workspace
authorization. Services derive the owning workspace from stored resources, not
only from URL parameters supplied by the browser.

Secret bindings store secret identifiers and input names. Secret values are
resolved only during runtime delivery. List responses, registry records,
selection state, logs, metrics, attachment evidence, and errors contain no
secret values.

That boundary covers Kandev-owned storage and every Kandev-owned read surface.
It does not, and cannot, cover the task executor itself. Delivery necessarily
hands the value to the agent process: the Claude, Cursor, Pi, and OpenCode
passthrough strategies write it into an MCP configuration file, and the Codex
strategy JSON-encodes it into `-c mcp_servers.*` process arguments that are
readable through the executor's process list. ACP stdio servers receive it as
child-process environment. A workspace secret bound to an MCP definition is
therefore readable by anything with access to that executor, including the agent
itself and any other MCP tool it runs. The setup review states this before a
member binds a secret, and this design does not claim to narrow it.

Public registry metadata is untrusted publisher input. Search results are
escaped as text, sanitized to bounded fields, and never rendered as arbitrary
HTML. Registry sync does not execute package managers, fetch package contents,
or connect to advertised MCP endpoints.

Custom executable configuration is an explicit privileged workspace action.
The UI shows the package, executable, or remote origin before save. It warns
that package mode executes publisher code and that MCP tools receive agent
data.

## Responsive behavior

The existing workspace settings horizontal tab strip remains the mobile entry
point. The MCP destination must not add a second page-level scroll owner.

On desktop, configured and marketplace views can use a two-pane composition.
On a phone, results use a single-column list. Marketplace setup opens a direct
full-height route or full-height surface with safe-area padding because it is a
deep form.

The shared selection picker uses the established mobile bottom-sheet pattern
for short multi-select flows. Rows have 44-pixel touch targets, visible selected
state, and accessible names. The task-create mobile dialog retains its current
full-height composition and contains no nested horizontal scroll.

The repository MCP section is collapsed by default on desktop and mobile. Its
header remains a 44-pixel disclosure target. Task creation keeps the whole MCP
selector inside the existing Advanced settings disclosure on both viewports.

Inherited origins wrap as text or chips. They never force the card width.
Pending, deferred, and failed runtime application states remain next to the
session-level control on desktop and mobile.

## Observability

The existing `MCPAttachmentReport` gains definition ID, definition revision,
and a bounded list of origin descriptors for each server. Each reconnect
creates a new `attachment_attempt_id` before delivery.

Structured events record catalog mutation outcomes, registry sync totals,
selection changes, effective-set size, filtering reasons, apply-state
transitions, and reconnect method. Labels use bounded source and reason codes.
They exclude runtime arguments, full URLs, headers, environment values, and
secret identifiers.

The UI distinguishes configured, delivered, and provider-observed evidence. A
successful `session/resume` or `session/load` proves request acceptance, not
that every third-party MCP server connected.

## Test strategy

- Repository tests cover workspace isolation, revision conflicts, scope
  replacement, cascade cleanup, and SQLite/Postgres migration replay.
- Registry client tests use a bounded fake server for cursor pagination,
  incremental sync, status changes, limits, and stale-cache fallback.
- Resolver tests cover multi-repository union, workspace-contextual global
  profiles, duplicate collapse, deterministic order, disabled definitions,
  secret failures, and provider filtering.
- Lifecycle tests cover resume preference, load fallback, active-turn deferral,
  failed reconnect rollback, next-start deferral, and attachment-attempt
  evidence.
- Frontend unit tests cover typed clients, inherited-origin projection,
  selection state, and responsive component behavior.
- Playwright covers catalog setup, scoped union at task creation, idle-session
  changes, stale registry state, keyboard operation, and phone layouts.

## Related decisions

- [ADR-0014: Per-CLI MCP server injection for passthrough mode](../../../decisions/0014-passthrough-mcp-injection-strategies.md)
- [ADR-0020: Pi project MCP config injection](../../../decisions/0020-pi-project-mcp-config-injection.md)
- [ADR-2026-07-30: Keep MCP attachment evidence session owned](../../../decisions/2026-07-30-session-owned-mcp-observability.md)
- [ADR-2026-08-18: Preserve ACP runtime configuration across context reset](../../../decisions/2026-08-18-context-reset-preserves-runtime-configuration.md)
- [ADR-2026-09-01: Compose workspace MCP definitions additively](../../../decisions/2026-09-01-workspace-mcp-configuration.md)

## External protocol references

- [ACP session setup](https://agentclientprotocol.com/protocol/v1/session-setup)
- [MCP Registry aggregator guide](https://github.com/modelcontextprotocol/registry/blob/main/docs/modelcontextprotocol-io/registry-aggregators.mdx)
- [MCP Registry preview](https://blog.modelcontextprotocol.io/posts/2025-09-08-mcp-registry-preview/)
