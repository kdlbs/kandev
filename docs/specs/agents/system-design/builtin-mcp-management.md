---
status: draft
system: agents
created: 2026-10-22
requirements:
  - REQ-AGENTS-BUILTIN-MCP-001
  - REQ-AGENTS-BUILTIN-MCP-002
  - REQ-AGENTS-BUILTIN-MCP-003
  - REQ-AGENTS-BUILTIN-MCP-004
owners:
  - kandev
---

# Built-in MCP packages, authority, and execution binding

## Ownership and grounding

Agents owns package membership, selection, and delivery. Platform's
[session observability](../../platform/system-design/mcp-session-observability.md)
continues to own connection/tool observations. The
[proposed decision](../../../decisions/2026-10-22-builtin-mcp-tool-policy.md)
records alternatives. This revised draft replaces workspace-only group pruning
with the selectable native packages requested on 2026-10-24.

Source baseline is actual parent #3293 merge
`0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`. Verified source facts:

- `internal/mcp/server/server.go:profileToolGroups` composes named groups from
  backend surfaces/capabilities. Configuration and task registrations share
  canonical discovery/task names. Automation composes and prunes a fixed set.
- `internal/mcp/profile.Context` carries surface, capabilities, providers, and
  managed/coordinator policies. It has no operator package selection today.
  `Server.SetMode`, `SetProfile`, and plugin rebuilds can replace registration.
- `Executor.resolveTaskSessionMCPProfile` selects the actual surface, title
  ownership, question mode, and canvas capability. `McpProfile` crosses launch,
  executor, agentctl create/configure, and persisted instance boundaries.
- `internal/mcp/scope/principal.go:ScopePrincipal` derives task/session identity
  and surface from stored records. `configurationPrincipalSurface` uses the
  session marker. Ordinary tasks must not gain that marker for package selection.
- `guardedMCPDispatcher` in `handlers/automation_authorization.go` applies
  automation/coordinator checks. The new package boundary needs a separate
  execution-bound check; hiding tools alone does not guard raw backend actions.
- `config.injectKandevMcpServerAtHost`, API route wiring, ACP transport, passthrough
  config generation, and `process.manager_permission_policy.go` assume one
  injected `kandev` endpoint. Permission parsing accepts letters, digits, `_`
  and `-`, but not dots. Use exact hyphenated package identities.
- `sysprompt.FormatKandevContextWithOptions` and Kandev/Office templates currently
  require rich output unconditionally. Walkthrough has both template references
  and a UI request hook. All must consume bound package availability.
- Catalog services live in `internal/agent/mcpconfig`; settings owns persistence
  and HTTP handlers. Web `mcp-settings.tsx` and `McpServerExplorer` provide the
  existing settings and desktop/touch navigation patterns.

All backend paths above are relative to `apps/backend`; web paths below are
relative to `apps/web`. Proposed types/routes/test names are explicitly new.

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-BUILTIN-MCP-001 | Packages; Authority; Management |
| REQ-AGENTS-BUILTIN-MCP-002 | Membership; Selection; Dispatch; Guidance |
| REQ-AGENTS-BUILTIN-MCP-003 | Persistence; Binding; Compatibility |
| REQ-AGENTS-BUILTIN-MCP-004 | Evidence; Desktop and phone |

## Packages and defaults

These are logical MCP servers in the existing agentctl process, each with its
own registration and transport session. No extra daemon, installation, endpoint
editor, credentials, or workspace catalog row is introduced.

| Package ID | Display name | Reserved MCP name | Regular Kanban task | Configuration chat | Office task |
| --- | --- | --- | --- | --- | --- |
| `core` | Kandev Core | `kandev` | Required | Required subset | Required subset |
| `configuration` | Kandev Configuration | `kandev-configuration` | Off; explicit human opt-in | Required | Ineligible |
| `rich-output` | Kandev Rich output | `kandev-rich-output` | On by default | Ineligible | On by default |
| `walkthroughs` | Kandev Walkthroughs | `kandev-walkthroughs` | On by default | Ineligible | Ineligible |
| `diagnostics` | Kandev Diagnostics | `kandev-diagnostics` | On by default | Ineligible | Ineligible |

The configuration-chat default is a product rule, independent of the workspace
option to block Configuration in regular tasks. That option must be labelled
"Allow Configuration for regular tasks", not "Disable Configuration". Existing
authorization still determines who can start and use a configuration chat.
Office can change Rich output using the same workspace/task policy. No other
Office grants expand. External, Automation, Coordinator, and managed-plugin
surfaces retain their current exact catalogs and legacy host routing. They do
not become user-selectable packages or accept task package preferences.

Keep existing optional task capabilities enabled by default for both existing
and new workspaces. Configuration is never workspace-default-on for a regular
task. A human may enable it per task where workspace policy permits it. Explicit
Off on the task wins over an optional workspace default. A workspace block wins
over task On. Core is always required where the host already injects it.

### Why these borders

Configuration changes Kandev resources; its mutation authority warrants a
separate opt-in. Rich output, Walkthroughs, and Diagnostics each support an
independent task and can be omitted without removing task control. Their schema
costs differ; bundling them as Presentation would defeat selective pruning.

Planning and delegation remain Core because injected workflow/recovery rules
use them. Provider review, branch/source, and canvas stay Core under existing
capability limits for this release; removing them needs a separate workflow
contract review. No hypothetical package is exposed as an empty switch.

## Authoritative tool membership

Add bounded package metadata beside the server registry, with typed package IDs
in `internal/mcp/profile`. Descriptors include display/description locale keys,
supported surfaces, required reasons, default policy, canonical names, and
backend action membership. Registration, validation, prompts, UI descriptors,
and permission provenance derive from this catalog. No duplicate frontend map.
New tools require an explicit package and surface predicate, proven by tests.

| Package | Membership at the verified baseline |
| --- | --- |
| Core | Existing `kanban-task`, `task-dependencies`, `related-tasks`, `plan`, `step-completion`, question/title groups, `task-pr-links`, `github-pr`, `review`, `task-branch-sources`, eligible `canvas-authoring`, `office-documents`, and `task-transfer`. Configuration-context lifecycle tools and common discovery below also live here. |
| Rich output | `show_rich_output_kandev` |
| Walkthroughs | `show_walkthrough_kandev`, `get_walkthrough_kandev`, `delete_walkthrough_kandev` |
| Diagnostics | `get_diagnostic_bundle_kandev` |
| Configuration | Configuration operations listed below, excluding common discovery and task lifecycle assigned to Core |

Configuration's exact initial set, with the `_kandev` suffix on every name:

- Workflows: `create_workflow`, `update_workflow`, `delete_workflow`,
  `import_workflow`, `export_workflow`; steps: `create_workflow_step`,
  `update_workflow_step`, `delete_workflow_step`, `reorder_workflow_steps`.
- Agents/profiles: `update_agent`, `create_agent_profile`, `delete_agent_profile`,
  `list_agent_profiles`, `update_agent_profile`.
- MCP: `get_mcp_config`, `update_mcp_config`.
- Shared prompts: `list_shared_prompts`, `get_shared_prompt`,
  `create_shared_prompt`, `update_shared_prompt`.
- Executors: `list_executors`, `create_executor_profile`,
  `update_executor_profile`, `delete_executor_profile`.
- Settings: `search_settings`, `describe_setting`, `get_settings`,
  `update_settings`, `list_settings_resources`.
- Automation setup: `create_automation`.

Common discovery (`list_workspaces`, `list_workflows`, `list_workflow_steps`,
`list_repositories`, `list_agents`, `list_executor_profiles`, all suffixed
`_kandev`) belongs to Core with its existing surface-specific schemas and scope.
Configuration task management (`list_tasks`, `move_task`, `delete_task`,
`archive_task`, `update_task_state`, `get_task_conversation`, `list_task_sessions`) stays Core under its
existing surface predicates. Do not register duplicate versions through both
packages. Consolidate overlapping registration definitions without widening a
surface's schema or handler rights. Catalog tests compare full schemas as well
as names against the prior context-specific catalog.

Core's `transfer_task_kandev` remains configuration/external/Office-only under
existing actor checks. A regular task selecting Configuration still cannot
transfer tasks. External question answering and agent-permission resolution
remain outside the selectable Configuration package. Automation setup configures
future work; it does not grant the caller the Automation or Coordinator surface.

For a configuration chat, Core union Configuration must equal today's eligible
configuration catalog. For a default Kanban task, Core union the three default
optional packages must equal today's eligible task catalog. Selecting
Configuration adds only the explicit configuration set above. Plugin tools stay
on their existing Core/legacy host path and grant contract; do not duplicate them
across package servers. Managed conversations retain their exact plugin policy.

## Authority and selection algorithm

Package control is a human management operation. Reuse current workspace/task
access checks and authenticated identity; add no role system. Auth-disabled mode
retains the synthetic single-user identity. Resolve the task's workspace from
stored data before authorization, not a caller-supplied workspace value.
Workspace policy writes require `workspace.manage`; task preference writes require
`task.write`; reads require `workspace.read`. These are existing resource scopes.
Workspace viewers cannot grant packages. Collaborators can change task preferences
within the workspace policy but cannot change that policy.

Workspace policy limits optional task packages. Per-package values are:
`blocked`, `available`, or `default`. Defaults are Configuration=`available` and
Rich output/Walkthroughs/Diagnostics=`default`. Configuration=`default` is invalid.
The Office resolver only considers Rich output. Ineligible packages are never
selected because a broad workspace default happens to include them.

Task preferences map eligible optional package IDs to `inherit`, `on`, or `off`.
Absent preference means `inherit`; persist canonical explicit overrides only.
The same task preferences apply to its normal sessions, but each execution binds
independently. Configuration chats instead use the fixed Core+Configuration
rule; do not consume ordinary-task overrides from the container task.

Resolve in this order:

1. Derive actual task/session surface and eligible capabilities from trusted data.
2. For fixed/restricted surfaces, keep the exact existing policy and stop.
3. Read workspace policy and task preferences with their revisions in one database
   snapshot. Validate all data, including surface eligibility.
4. Include Core. For each eligible optional package, apply workspace block first,
   then explicit task On/Off, then workspace default. Configuration needs explicit
   task On. A blocked stored On remains visible as suppressed, never effective.
5. Create the trusted execution manifest. Profile permission/provider limits and
   each handler's resource authorization still apply inside selected packages.

A workspace block saved after task opt-in suppresses that opt-in at future starts.
Keep the task preference so the UI can explain it. Removing a block can make an
existing explicit On effective again; workspace save previews this effect.
A save attempting On while blocked is rejected with the reason. Task Reset clears
all overrides, including Configuration; workspace Reset restores the stated
limits/defaults. Neither Reset changes third-party server selections.

Agent profiles have no package preference in this release. Reusing one profile
across workspaces cannot copy a package grant. Session overrides and workflow
presets are deferred. Task duplication, agent-created children, automation-created
tasks, and profile changes do not copy explicit Configuration grants. A human
creating a task can explicitly choose Configuration in the same authorized create
flow; a default duplicate uses inherited optional defaults. Record actor and
revision for explicit task choices. Existing sessions are not retroactively
changed when the task preference changes.

Every package-control writer rejects MCP agent principals, including configuration
agents and external MCP callers. Generic `update_settings`, `update_agent_profile`,
task/session metadata mutation (including mode markers), legacy MCP config tools,
and launch inputs cannot carry
or write package policy. Use a dedicated human API/service, protect its fields in
shared mutation paths, and strip untrusted package/execution metadata before
building launch metadata. Configuration can still change ordinary authorized
settings. It cannot modify the grant used to expose Configuration itself.

Configuration operates with the real authenticated owner's existing resource
rights. Some profiles/executors/shared prompts are not task-local; describe that
scope in the selector. Workspace package permission is not a new resource sandbox.
Owner/resource checks, secret redaction, provider permission prompts, and explicit
user instructions remain in force. Do not broaden an existing domain check to
make a failing configuration call succeed.

## Persistence and HTTP contract

Use the existing agent-settings schema owner for a new
`workspace_builtin_mcp_policies` table (workspace key, schema version 1, revision,
canonical per-package limits/defaults, actor/timestamps). Store task preferences
in a new task-owned `task_builtin_mcp_preferences` table (task key, version,
revision, canonical overrides, actor/timestamps), with cascade/cleanup conventions
from the task store. Do not overload third-party definition/selection tables or
arbitrary task metadata. A narrow resolver reads both stores in the owning
persistence transaction; no executor I/O occurs inside that transaction.

Management endpoints:

- `GET /api/v1/workspaces/{id}/mcp-built-in`: catalog descriptors, policy,
  revision, fixed context defaults, and editability. `can_edit` governs workspace
  policy; `can_choose_for_task` independently reflects human task-write authority
  for initial task choices. No global health field.
- `PUT /api/v1/workspaces/{id}/mcp-built-in/policy`: complete optional-package
  policy with `expected_revision`.
- `GET /api/v1/tasks/{id}/mcp-built-in`: task overrides, workspace limits,
  revisions, effective desired packages, and per-choice explanations.
- `PUT /api/v1/tasks/{id}/mcp-built-in/preferences`: complete task overrides,
  `expected_revision`, and `expected_workspace_policy_revision`. Recheck current
  workspace policy and both revisions atomically before accepting an opt-in.
- `GET /api/v1/task-sessions/{id}/mcp-built-in`: authorized desired manifest,
  bound manifest or legacy/unknown state, and package evidence identities.

The existing HTTP and WebSocket task-create requests accept optional
`builtin_mcp_preferences`: `version: 1`,
`expected_workspace_policy_revision`, and `packages` with Inherit/On/Off values.
Only the authenticated human task writer can supply this consent. Validate its
workspace and policy revision before insertion. Persist it before the task-created
launch event; a failed preference write removes the partial task and emits no
launch event. Missing policy wiring rejects an explicit choice. Replayed creates
do not revise an existing task's preferences. An omitted field keeps default
inheritance; it does not grant Configuration.

Missing rows are successful revision-0 defaults; errors are not missing rows.
Versioned payloads reject unknown fields, duplicate IDs, invalid values, raw tool
names, Core changes, and unsupported contexts. CAS revision0 inserts only if
absent; stale writes return 409 and sanitized current state. Ordinary access denial
uses current 403/not-found conventions, not a stale-page-token error. No-op saves
return the same revision; failed writes publish no success event. Record no secrets.

Use SQLite/PostgreSQL migration, rollback/upgrade fixtures, store conformance,
and workspace/task deletion coverage. No default rows need to be materialized
on upgrade. This is product configuration, not an install-wide environment setting.
No release toggle is proposed by this package.

## Transport, registration, and dispatch

Add a typed package manifest to the trusted `mcpprofile.Context` carrier. It
contains schema/catalog version, workspace/task IDs, both revisions, resolved
package IDs and selection provenance. A canonical binding envelope contains the
normalized profile and its digest. Persist that envelope with the execution
generation through the trusted launch metadata builder. A digest is
an integrity comparison, not proof of caller authority. The backend must resolve
it from authenticated execution state rather than trust an agent-supplied digest.

Build package-specific MCP server views from the catalog. Core retains `/mcp`,
`/sse`, and `/message`. Optional packages use `/mcp/packages/{id}`,
`/sse/packages/{id}`, and `/message/packages/{id}`. SSE message routing/session IDs
must stay within the same package; a Core transport session cannot call an
optional route. Disabled/unknown package routes have no handler and return a
neutral unavailable result. Preserve the existing external endpoint unchanged.
Do not leave an aggregate in-session route that exposes disabled tools.

All server views share the execution backend client and lifecycle, but retain
separate transport sessions and tool registries. A small package host owns atomic
profile updates, package lifecycle, and shutdown; do not copy the entire Server
or one goroutine/process tree per package. Mode/title/provider/plugin updates
recompute within the frozen package manifest. They cannot add a package. Reject
unsupported live surface transitions; use a normal process start instead.

Injection builds entries from the trusted manifest: HTTP first and SSE fallback
for each exact package name, deduplicating by package identity after adapter
capability filtering. Selection of an optional package injects only that package's
endpoint and registered tools. Preserve existing transport choice and host routing
for local/worktree, container, SSH, Kubernetes, Sprites, and remote executors.

Add execution-bound package authorization to `guardedMCPDispatcher` and the
runtime-to-backend MCP stream boundary. Resolve manifest/owner from the stream's
execution identity. Reject actions outside the permitted registered set before
handler effects, including raw requests that bypass MCP discovery. Keep existing
automation/coordinator checks. Do not change `Principal.Surface` to Configuration
when a Kanban task selects the package. Carry a separate trusted package grant.
Never infer that grant from a canonical name, route string, argument, session
metadata supplied by an agent, or `config_mode` changes.

Some actions are shared by multiple tools or parameterized settings. Catalog
metadata must identify the allowed action plus any operation discriminator;
handler authorization still validates target and fields. Package-control keys
are forbidden through every settings/profile/task MCP writer even if the generic
settings action is permitted. Tests enumerate registered tools and dispatch
coverage so a new action cannot bypass the boundary.

Reserve the exact new hyphenated names across catalog create/update/import,
legacy composition, Cursor/plugin extraction, and provider config conversion.
Keep existing `kandev` and `kandev.*` reservations. Do not trust all `kandev-` names
by prefix. Existing user definitions with a new reserved name are not overwritten,
deleted, or silently renamed. Report a collision and require the operator to
rename that definition before a new package-mode launch in its affected context.
Retained processes remain intact. Never substitute a foreign endpoint.

Permission auto-approval must match exact package name, canonical bound tool,
trusted injection provenance, current host/port, and that package's route. An
unknown name, user-defined lookalike, wrong endpoint, unbound tool, or extra
command/env/headers cannot inherit built-in approval. Preserve exact Coordinator
allowlists and managed policies. Canonical names remain `*_kandev`; client-qualified
names change for moved tools. Discovery instructions must use the client's actual
qualified name, not assume every tool belongs to `mcp__kandev__...`.

## Execution binding and compatibility

Prepare one manifest before new prompt admission and runtime launch. Send the
same prepared value to injection, all package registries, backend dispatch, and
prompt generation. `Normalize`, clone/equality, `SetMode`, `SetProfile`, JSON
round trips, every executor DTO, and helper recovery must preserve and validate
it. Publish bound state only after successful process-start acceptance for that
execution generation; a launch attempt is not accepted evidence.

An unused workspace helper can precede the agent process. Before configuring its
agent command, the backend binds its prepared profile through authenticated
`PUT /api/v1/mcp/builtin-binding`. The helper serializes this operation with
process start. It accepts one binding and an identical retry; it rejects a
changed binding, an already configured command, or prior process use. Optional
routes become available only after binding. The response must confirm the exact
canonical profile. This preparation does not claim accepted process evidence.
Persist an unavailable marker before a remote binding or capability update.
Persist the confirmed value only after helper success. Use an execution-ID CAS
that changes only the binding metadata key. Status snapshots preserve that key
for an existing execution, so delayed status writes cannot restore an old grant.

If policy R+1 is saved while a start uses R, the accepted manifest remains R and
R+1 is pending. Failed or superseded starts cannot replace accepted state. Compare
resolved package content and provenance; a revision-only no-op does not require
a false pending badge. Profile capability changes may narrow eligible tools
inside a selected package under existing contracts but cannot resurrect an
excluded package. Required title and question transitions retain their guards.

Saving changes only desired state. Same-process reconnect, idle, continuation,
context reset, and backend adoption retain the bound manifest. A fresh process
resolves current policy. Workspace/task rebind cannot reuse another owner's
manifest; reject or require the normal restart boundary. Multi-session tasks
can therefore show different bound packages, correctly labelled per execution.

Use an agentctl capability `builtin-mcp-packages-v1`, with backend catalog-version
agreement. A fresh package-mode launch on an old helper or unsupported injection
adapter fails before start with upgrade guidance. Even default selection needs
support because an old Core server would still contain moved optional tools.
Do not silently collapse packages or ignore exclusions to maintain compatibility.

Existing retained monolithic executions remain in explicit legacy mode until a
normal process start. Recovery never retrofits package routes or rewrites their
catalog. Missing historical manifest is Unknown, not today's default. Explicit
corrupt/unsupported manifests use the existing recovery-required path; Stop stays
available. No fallback to a broader new execution is allowed. Default capability
parity is preserved on upgrade; transport/qualified names change only at a new
package-capable start.

| Path | Intended support and evidence | Unsupported behavior |
| --- | --- | --- |
| Structured ACP/native adapter | Multiple exact injected entries, transport negotiation, per-package observations; wire/adapter tests | Refuse package-mode start if entries cannot be represented |
| CLI passthrough | Extend each supported config builder, quoting and reserved-name handling; config-generation tests | No synthetic connected badge; missing injection strategy is unsupported |
| Local/worktree and remote executors | Shared manifest plus each launch carrier; round-trip and helper negotiation tests | Old helper fails before fresh start |
| Retained monolithic agent | Existing routes/catalog and honest legacy evidence | No hot migration or automatic restart |
| External/Automation/Coordinator/managed conversation | Existing exact catalogs, gates, and routes | No user package grants |

Mock wire tests do not certify installed provider connectivity. Record provider
and transport coverage precisely; do not claim all providers from a shared code path.

## Guidance and dependent actions

Extend Kandev/Office/configuration context formatters with a resolved execution
manifest. Generate selected-tool sections, server discovery hints, and mandatory
rules from that same view. Core markers, task/session identity, question barriers,
title ownership, delegation limits, plan editing, and completion gates remain.
A regular task with Configuration receives configuration operation guidance in
addition to its task rules; it does not receive a configuration-chat identity.
Configuration guidance names shared-resource effects and package-control denial.

Parameterize rich-output Essential workflow text and tool entry, walkthrough
entries, and Office's static rich-output section. When excluded, permit normal
text/Markdown and do not demand rediscovery of a known-disabled tool. Keep custom
operator/repository prompts and transcript history unchanged. New generated
blocks explicitly supersede stale optional-tool guidance. Legacy unknown state
uses actual discovery and makes no package availability claim.

Guard `hooks/domains/session/use-request-changes-walkthrough.ts` and all desktop,
mobile, review, and changes callers with authoritative bound availability for a
running agent, or desired availability for a future start. Unknown blocks a new
authoring request with a refresh/details action. A desired Off must not disable
a still-bound capable execution. Record typed queued built-in intent and recheck
at dispatch against the target execution; never parse arbitrary message text.
Existing saved walkthrough/output rendering remains available.

Before shipping, reconcile the conditional availability and discovery wording in
`agent-rich-output.md`, `mcp-tool-discovery-guidance.md`, and workspace MCP
requirements/design, plus configuration tool contracts where they state a single
server. This draft declares the proposed change; active documents continue to
describe shipped behavior until the implementation work order updates them.

## Management projection and evidence

Use `kind: builtin` and stable package IDs in a separate typed projection.
Configured shows a Built-in section before workspace definitions; definition
counts and empty-catalog help remain accurate. A package is not a catalog
selection ID, assignment candidate, or inherited-server exclusion. General
navigation redesign remains owned by the sibling task.

Workspace detail exposes package limits/defaults and fixed configuration-chat
behavior. Task Customize exposes separate Built-in choices and their origins.
Configuration explicitly states that it can change authorized workflows, agents,
profiles, MCP settings, prompts, executors, and automations, including shared
resources. The existing Save action records consent to that package choice;
do not add a second approval dialog. It does not authorize every later action.

Session detail joins expected package descriptors to `MCPAttachmentReport` by
session/execution, exact server identity, and host provenance. Distinguish desired
selection, accepted manifest, expected catalog, actual observed `tools/list`, and
connection status/time. Missing counts are Unknown rather than zero. Never copy
Core's health to other packages or treat registration as a successful connection.
A legacy monolithic report remains one legacy report, not five fabricated ones.

Carry safe manifest/package identity in attachment DTOs and hydration/reducers,
using existing observation ordering and sanitization. No new poller, health enum,
probe, or global health aggregate. Domain hooks invalidate on known writes and
reopen, key by workspace/task/session/execution, and reject stale responses.
Third-party pending-change acknowledgments cannot clear package pending state.

## Desktop and phone composition

UI-01 workspace: Configured > Built-in package row > detail. Desktop uses the
existing settings editor replacement and shared Save/Discard contributor. Detail
order is purpose/ownership, tools and eligible contexts, task policy, fixed
required context, then application timing. Core shows reasons rather than a
misleading disabled toggle. Package defaults use explicit labelled choices.

UI-02 task: MCP plug > Customize > Built-in packages. Each optional row shows
Inherit/On/Off, effective value, and workspace limit. Core shows Required.
Configuration starts Inherit (Off); details explain authority before save.
Existing authorized task creation can save these choices before the first start.
Session Connections lists actual package entries; selecting one shows desired,
bound, and observed state with an authorized Manage action.

Phone uses one full-height detail in the existing settings shell or explorer
drawer, with list > package detail > tool detail in the same surface. Back returns
one level and Close returns focus to the invoker. Fixed navigation/save controls
surround one `min-h-0` scrolling body with safe-area clearance; no nested drawer.
Use the shipped touch-tablet drawer. Maintain one domain controller and draft
across responsive branches and viewport changes.

Use `controlSizingClassName`: at least 44px phone/coarse targets, nominal 48px
header close, 28px ordinary desktop controls. Keep reasons/errors visible without
hover. Conflict preserves the draft and offers explicit reload; denial remains
unsaved. Shared stale-page handling stays in the existing API client. All copy
uses seven locales and generated Traditional Chinese. Reuse neutral surfaces,
Tabler icons, headings, spacing, and focus rings. Toggles need no new animation;
any press feedback follows better-ui 0.96/150ms and reduced-motion preferences.

## Observability and validation

Log policy/preference changes, accepted package digest, and bounded failure
reasons with normal workspace/task/session/execution context. No secrets, full
endpoints, arguments, or prompt text. Avoid per-entity metric labels.

Catalog tests prove exhaustive ownership, unique names, schema/default union
parity, every eligible selection combination, and all fixed/restricted profiles.
Authorization tests cover direct routes, raw backend actions, forged manifests,
foreign ownership, self-grants through generic writers, and transfer denial.
Runtime tests cover atomic snapshots, old helpers, collisions, recovery, stale
starts, workspace switches, qualified names, transport and permission provenance.
UI/E2E tests prove regular-task Configuration opt-in, configuration-chat defaults,
workspace blocking, optional pruning, truthful evidence, and desktop/phone parity.
The [plan](../../../plans/builtin-mcp-management/plan.md) maps criteria to tests
and the three sequential work orders. No production or permanent tests change
at this design checkpoint.
