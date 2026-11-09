---
status: current
system: agents
requirements:
  - REQ-AGENTS-MCP-MANAGEMENT-001
  - REQ-AGENTS-MCP-MANAGEMENT-002
  - REQ-AGENTS-MCP-MANAGEMENT-003
  - REQ-AGENTS-MCP-MANAGEMENT-004
---

# Workspace MCP management design

## Boundary and baseline evidence

The baseline is PR #3293 at `193de56ba2597b335a97b1e5ab01d09340f22647`.
The parent [testing guide](../../../plans/workspace-mcp-configuration/testing-guide.md)
explicitly defers this management experience. No archived design or Platform
patch was recovered. The design was rebuilt from that tip. Implementation was
subsequently authorized and completed on 2026-10-25.

For draft publication on 2026-10-24, the branch was reconciled onto main
`cc687bdce96bc22fb8a9ca87abe571b8309d3125`, containing parent squash
`0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`, without replaying parent commits.
The relevant MCP selection, profile picker, and evidence contracts remain the
baseline for the extension. The design-only publication did not authorize
implementation. A later user request authorized all three work orders; existing
settings navigation and repository discard support are retained.

Agents owns the catalog and direct associations. Task services supply owner
identity and access. Platform owns attachment evidence. Existing
[additive selection](../../../decisions/2026-09-01-workspace-mcp-configuration.md)
and [session-owned evidence](../../../decisions/2026-07-30-session-owned-mcp-observability.md)
remain authoritative. This extension needs no new persistence model or ADR.

| Requirement | Design section |
| --- | --- |
| REQ-AGENTS-MCP-MANAGEMENT-001 | Usage projection and destinations |
| REQ-AGENTS-MCP-MANAGEMENT-002 | Profile context and save ownership |
| REQ-AGENTS-MCP-MANAGEMENT-003 | Evidence presentation |
| REQ-AGENTS-MCP-MANAGEMENT-004 | Responsive composition |

Verified source gaps at the design baseline (resolved by this implementation):

- `SelectionImpact` in `internal/agent/mcpconfig/selections.go` counts direct
  associations. Catalog handlers return these counts, without named owners.
- `ProfileMcpConfigCard` routes only saved workspace-owned profiles to
  `ProfileMCPSelectionCard`. The latter already accepts a global profile but
  clears local draft and baseline immediately when workspace changes.
- `buildMcpExplorerServers` promotes configured-only third-party names to
  `delivered` when any observed row exists. This is not server-specific proof.
- `MCP_STATUS_DOT_CLASS` colors `delivered` amber. The Platform design explicitly
  specifies that color, so implementation must reconcile its wording by intent.

## Usage projection and destinations

Add a read-only `GET /api/v1/workspaces/:id/mcp-servers/:serverID/usage` beside
the existing catalog routes. Keep list responses compact. Fetch usage on detail
entry and explicit refresh, not once for every card or on a new background timer.

The settings store enumerates direct association references from the existing
four tables, constrained through `mcp_server_definitions.workspace_id` and,
for profile rows, the association's workspace. It does not join task tables
across storage owners. A typed projection service in `mcpconfig` accepts an
owner-reader callback wired in `internal/backendapp`, beside
`mcpSelectionOwnerValidator`. That callback reads authoritative profile,
repository, task, and session records and checks the current caller's access.
Reuse `AuthorizeWorkspaceAccess` and the resource services used by the existing
owner validators. Profile projection reads only name, agent identity, enabled
state, and workspace membership; it does not load legacy configuration or run
profile backfill. Do not infer access from a browser inventory or an owner ID.

Proposed response:

```text
workspace_id, definition_id
items: [{ scope, owner_id, display_name?, workspace_id,
          agent_id?, task_id?, session_id?, owner_state,
          destination: { kind, ...authorized identity fields } | null }]
next_cursor?: opaque string
```

`scope` uses the existing `profile`, `repository`, `task`, `task_session` enum.
`owner_state` distinguishes ordinary, disabled, and archived owners when the
owning service reports those states. A disabled profile or archived task can
retain saved usage. Offer its existing view destination when authorized. Do
not enable an owner or start a session to make it editable.

Authorize workspace and definition before enumerating. Omit deleted,
cross-workspace, or inaccessible owners and all of their identity fields.
An existing authorized owner lacking a display name uses a localized scope
label. Missing route identity produces a noninteractive row, not a guessed URL.
Authorization is checked again by the destination's normal loader and writes.
An access loss after navigation shows existing unavailable recovery and never
falls back to another owner or the first session.

Use stable `(scope, owner_id)` ordering and keyset pagination. Default page
size is 50 and maximum is 100. Process at most 200 candidate associations per
request. An opaque cursor is bound to workspace, definition, and ordering.
Protect the cursor with authenticated encryption so skipped owner IDs are not
exposed; a plain base64 or signed plaintext cursor is insufficient. A page
can be empty with a continuation cursor. Continue until no cursor remains.
Never display an exact total based on a partial page or raw association counts.
Only returned authorized rows contribute to visible counts. Read failure
returns an error, not an empty success. Owner-service failure must not be
silently treated as permission denial. A failed refresh keeps the last snapshot
with a stale indication. Access denial clears it immediately.

The projection is a current read, not an atomic snapshot across task/settings
stores. Cursor encryption keys live for the backend process; after a restart,
an invalid cursor requires a fresh usage read. No persistent key or owner
snapshot is added. Dedupe paged results by scope and owner. An explicit refresh restarts
pagination so renames, deletions, and new associations converge. No database
transaction remains open during cross-service reads. Existing guarded delete
counts remain deletion semantics, not an authorization oracle for the new view.

Create a shared frontend destination builder in `lib/mcp/management-links.ts`
(new). It accepts typed identities and emits only internal known routes.
Encode path/query segments with existing helpers. Never accept a backend URL
or arbitrary `returnTo` value. The shared catalog destination is:

```text
/settings/workspaces/:workspaceId/mcp-servers?serverId=:definitionId&view=usage
```

This extends the existing catalog page, not a new management hub. Unknown or
removed definitions show unavailable recovery with Back to configured servers.
Profile destinations use the existing agent/profile route, with validated
`workspaceId` query context and an MCP section anchor. Repository destinations
use the existing workspace repositories route, with a validated `repositoryId`
query that opens that repository's existing editor and MCP section. Task and
session destinations use `linkToTask`, including its existing `sessionId`
option for sessions. A task row opens task detail. A session row opens the
specific existing conversation, whose composer plug owns customization.

### Shared owner-navigation contract

Task 02 owns the following route spellings and their validated entry behavior.
These are planned extensions, not a claim that query-driven entry already
ships in the parent. The assignment sibling consumes the shared destination
builder after task 02 is integrated.

The shared builder takes a discriminated `MCPOwnerDestination` input:

```ts
type MCPOwnerDestination =
  | { scope: "repository"; workspaceId: string; ownerId: string }
  | { scope: "profile"; workspaceId: string; ownerId: string; agentId: string }
  | { scope: "task"; workspaceId: string; ownerId: string }
  | { scope: "task_session"; workspaceId: string; ownerId: string; taskId: string };
```

`ownerId` is respectively the repository, profile, task, or session ID. Keep
workspace context in every variant for validation, even when the final task
URL omits it. Normalize a caller's separate `sessionId` to `ownerId`; reject
disagreement when both fields are supplied. Do not invent missing agent/task
identities from the active browser context. The exported type and helper live
in `apps/web/lib/mcp/management-links.ts`.

- Catalog usage:
  `/settings/workspaces/:workspaceId/mcp-servers?serverId=:definitionId&view=usage`.
  Open the saved definition's Used in view in the existing catalog.
- Profile:
  `/settings/agents/:agentId/profiles/:profileId?workspaceId=:workspaceId#mcp`.
  Resolve the profile under its stored agent ID (legacy agent-name routes remain
  compatible). Validate the workspace hint.
  Reveal the shared MCP card after its context loads. A workspace-owned profile
  rejects a hint for another workspace. A global profile uses the validated
  hint without changing the application's active workspace preference.
- Repository:
  `/settings/workspaces/:workspaceId/repositories?repositoryId=:repositoryId#mcp`.
  Resolve the repository inside that workspace. Open its existing editor and
  expand the existing MCP section once. Preserve the shared Save changes owner.
- Task: `/t/:taskId`, emitted only through `linkToTask(taskId)`.
- Existing task session: `/t/:taskId?sessionId=:sessionId`, emitted through
  `linkToTask(taskId, { sessionId })`. Verify the session belongs to the task
  and the caller can access it before navigation. An invalid explicit session
  must not fall back to the primary session for this handoff. The user opens
  the existing composer plug and Customize view to make a selection.

All identifiers are raw typed inputs to the builder, which encodes them once.
The `#mcp` anchor only reveals/focuses the existing settings section. It carries
no selection intent. Navigation never preselects a definition, submits a save,
starts an agent, or invokes provider application. This contract adds no
definition-selection query and no arbitrary return URL.

Definition highlighting is assignment-owned. Its optional `definitionId`
input can append a focus-only `mcpServerId` query to the profile, repository,
or session destination through URLSearchParams, preserving existing query
keys and `#mcp`. This is separate from catalog detail's `serverId` query.
Management navigation works without the hint. Assignment owns parsing,
workspace validation, and one-time reveal/highlight after the existing picker
loads. Unknown, removed, cross-workspace, or filtered-out definitions must not
become selected or cause a broader inventory read. The hint never changes
checkboxes, search, or save state. For sessions, retain the hint until the user
opens the existing plug/Customize; it does not automatically open an overlay.
Assignment must cover this extension in its own requirements and tests.

The assignment sibling owns the saved-definition receipt, repeatable Use in
entry on catalog cards, owner choice, and partial-success recovery. It retains
the successfully saved definition ID and presents its receipt/context without
re-registering the definition when owner assignment fails. Repository/profile
edits use shared manual Save changes. Existing session choices autosave only
after an explicit user selection in Customize. This management package owns
Used in, route context, and authorized navigation, not assignment behavior.

The registration-assignment sibling can later link to this exact catalog
destination after a successful definition save. This package provides no
assignment controls or post-registration flow. Until this destination ships,
that sibling must not claim it exists or create a competing hub. Its draft
assignment design explicitly depends on this package's completed navigation
work. Its later conditional-selection write extension belongs to that sibling.
Reconcile that extension before implementation without assuming it is present.

## Profile context and save ownership

Route every saved MCP-capable profile to the shared selection card. Unsaved
profiles retain their existing creation behavior. Workspace-owned profiles use
their stored workspace. Global profiles accept a validated route workspace or
require an explicit choice from authorized workspaces. Do not silently select
the first workspace or modify the application's active workspace preference.

Use the shared `MCPSelectionContent` directly in the profile card. Keep one
scope description and inline search/checkbox list on desktop and phone, with
the settings page as scroll owner. Reuse the repository settings list treatment
and touch-sized rows. Do not add a nested picker opener/drawer or catalog
shortcut. Reset only the search query on workspace-context change; selection
drafts and their navigation guard keep their existing owner.

Treat `(profile_id, workspace_id)` as the editor identity. Reuse
`useMCPWorkspaceDefinitions` and `useMCPSelectionEditor`. Mask old choices
immediately when identity changes and reject stale reads and save completions.
The workspace control calls `requestNavigation` with a context-change callback.
The existing `SettingsSaveProvider` then owns Save changes, discard, and stay.
Disable duplicate switches during save/exclusive operations. A rejected save
must reject the contributor promise and retain its original baseline. The
editor can return null for an obsolete context. Treat that as cancellation,
not successful saving, using the existing `SettingsSaveCancelledError` path.

Capture selected IDs and identity before awaiting save. Accept the server's
canonical selection as the baseline only for that identity. Preserve any newer
draft instead of clearing it. Include context in contributor identity/revision
and unregister only after the navigation guard permits the context change.
Existing profile fields and repository selectors retain their contributors.
Shared save can partially succeed across contributors. Successful contributors
remain saved, failures retain drafts, and navigation stays blocked until all
required writes succeed or the user discards them.

The current profile selection PUT has no expected-revision field. Do not invent
CAS support or broaden it here. Retain its server write contract, the shared
settings interlock, and context-generation protection. A separate assignment
flow must inspect that actual contract before promising optimistic concurrency.

Expose safe import readiness on profile selection GET as optional
`legacy_import_status: complete | pending | unknown`, derived from existing
`GetMCPImportState`. Missing/failed reads mean unknown, not complete. Do not
return raw failure text or legacy configuration through the selection response.
No import is triggered by a read or workspace change. Unknown/pending shows a
localized fallback explanation and retains the existing explicit legacy editor
for callers already authorized by `org.config.manage`. Label it as global
profile legacy configuration, distinct from workspace choices. Avoid mounting
two active save contributors for the same legacy draft. An ordinary workspace
member can still use their authorized catalog selection API without gaining
global profile configuration rights. Never change complete markers, resolver
fallback, secret bindings, or import retry rules in this package.

## Evidence presentation

Keep `MCPAttachmentHistory` and Platform reducers unchanged. Change
`buildMcpExplorerServers` so configured-only rows are `unknown`, including when
other observed rows exist. Current server-specific history still wins.

Use one shared status presentation for list, detail, and tooltip. Delivered
and unknown use neutral muted tokens with explanatory text. Preserve Active
green, explicit failure red, connected evidence, and filtered/unavailable
reasons. No new status enum, tool count, runtime probe, timeout, or reconnection
is introduced. Pending markers continue to derive exclusively from
`MCPSessionSelectionState` through `mcpSelectionMarker`.

Amend Platform's existing observability requirement and design during task 03
to record neutral delivered/unobserved styling and remove the old amber rule.
Keep connected-without-tools distinct from unobserved delivery. Preserve the
existing AC identities and evidence authority. This is a planned amendment,
not a claim that the missing archived Platform patch was recovered.

## Responsive composition

The catalog card has competing edit/disable/delete actions. Add an explicit
Used in action rather than making the whole card a conflicting link. Desktop
usage appears in a bounded dialog. Phone usage uses a dedicated full-height
detail surface with a fixed title/back control and one internally scrolling
owner list. It contains no additional settings editor.

Use `useResponsiveBreakpoint`, shared Dialog/Drawer primitives, and the shipped
MCP explorer's fixed-header/one-scroller layout. The curated
`components/kanban/mobile-menu-sheet.tsx` provides safe-area, inset, and
contained-scrolling patterns. For the short workspace choice, use the existing
touch picker treatment. Owner rows have one navigation action and are tappable
across the row. Read-only rows have no misleading chevron.

Use `100dvh`, `min-h-0`, and `overscroll-contain` for bounded phone bodies.
Clear safe areas and keep 44px phone/coarse-pointer targets, including at 767px
with a fine pointer. Preserve 28px ordinary desktop controls. Verify 320px,
393px, 767px, 768px, and a 900px coarse-pointer tablet. Keep Connections'
centered touch title, contained close action, and nonoverlapping long rows.

Preserve tokens, restrained surfaces, currentColor icons, static text cues,
and existing primitive motion. Do not animate routine refresh or row ordering.
Any new motion respects reduced motion and disabled controls. Keep one editor
above responsive composition so resizing does not register duplicate savers.

## Read failures and recovery

Keep confirmed missing sessions separate from unavailable session-list reads.
An explicit session link must validate its exact task/session pair before
admission. A failed list read renders task-scoped retry copy and refreshes the
same requested session on retry. Ordinary route errors retain the existing
client recovery shell. No error path substitutes another conversation.

Synchronize usage and settings-draft ownership refs after React commits. A
suspended or abandoned render must not change which draft is saved or suppress
a committed context response. Cursor transport must not require optional
`URLSearchParams.size` support.

The usage reader lives above Dialog/Drawer composition so accepted pages and
cursor survive breakpoint changes. A failed refresh marks retained rows stale;
links and continuation stay disabled through retries until a successful fresh
page replaces them. Next-page errors retain valid earlier rows and their links,
with a continuation retry. Access loss clears all rows. Row identity includes
scope and owner ID; actionable owner links use the shared pointer styling.

Profile legacy recovery is shown only after an accepted selection read confirms
incomplete/unknown import, or after explicit recovery has been opened. No
workspace/loading/error state invents legacy-import evidence. Owner response
identity mismatches and denied/missing owners do not offer a futile retry;
recoverable metadata read failures do.

## Failure, privacy, and documentation

No usage response contains endpoints, arguments, headers, environment,
secret references, secret values, or legacy JSON. Do not log response bodies,
names, or cursor payloads. Use existing bounded error codes and request logging.
No new telemetry producer is needed.

The existing how-to sections in `docs/public/agents-and-profiles.md` and
`docs/public/automation-and-mcp.md` describe direct saved usage, workspace scope,
legacy recovery, and delivery limits. Parent testing notes now cover the
implemented global-profile picker; historical verification ledgers are preserved.

## Delivery

[The plan](../../../plans/mcp-management-clarity/plan.md) sequences usage
projection, management navigation, and neutral evidence. All three work orders
are implemented and record their validation. The original design turn ran no
runtime validation; implementation checks ran on 2026-10-25. The PR remains
draft, and mock browser fixtures do not establish live provider connectivity.


### Workspace-profile destination admission

The global `GET /agents` inventory intentionally excludes workspace-owned
profiles. It is not a destination-admission source for those profiles, and
Office AgentInstance normalization is not a substitute for settings identity.
The management follow-up exposes metadata-only reads:

- `GET /api/v1/workspaces/:id/mcp-profile-owners?cursor=:lastId&limit=:n`
  returns `{workspace_id, items, next_cursor?}`. The default page is 50 rows,
  maximum 100. Stable ID ordering uses the last returned visible profile ID
  as the continuation cursor. An empty page is a successful authorized read.
- `GET /api/v1/workspaces/:id/mcp-profile-owners/:profileId` returns one owner.
  Both operations authorize workspace access before reading. Missing, deleted,
  and foreign-workspace profiles are indistinguishable on the detail route.
  Missing dependencies fail closed; failed reads never return empty success.

Each item contains only `id`, `agent_id`, `agent_name`, `name`, `workspace_id`,
`kind`, `enabled`, and `supports_mcp`. Empty `workspace_id` means global.
Inventory includes global plus exact-workspace settings profiles, including
ineligible or disabled rows so consumers can distinguish eligibility from
access. The query reads no configuration, secrets, or legacy backfill and
performs no writes. Clients must still use authorized selection APIs to save.

`listMCPProfileOwners` and `getMCPProfileOwner` in `lib/api/domains/mcp-api.ts`
perform uncached reads. `useMCPProfileOwner` masks previous context immediately,
rejects mismatched response identities, and ignores obsolete completions.
The existing profile route validates its workspace, agent and profile through
this detail read whenever `workspaceId` is explicit. Global profiles retain
the existing full settings editor. Workspace-owned concrete MCP profiles open
the existing `ProfileMCPSelectionCard` with the real profile ID and fixed
workspace, using shared manual Save changes. This is the same owner route,
not a new management destination or a fabricated full profile configuration.
Workspace-owned metadata never enters the global agent store or task picker.
Assignment consumes this metadata contract for its own chooser/revalidation;
its `mcpServerId` hint and conditional writes remain independently owned.
