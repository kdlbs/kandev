---
status: draft
system: agents
requirements:
  - REQ-AGENTS-PROFILE-LIST-ORDERING-001
  - REQ-AGENTS-PROFILE-LIST-ORDERING-003
---

# Agent Profile List Ordering System Design

## Purpose and boundaries

The agent settings store owns one saved profile order per agent and a
monotonic revision of it. The settings controller validates and applies a
reorder, the HTTP handler exposes it behind the existing agent-configuration
permission and broadcasts a WebSocket event, and clients accept an order only
when its revision is newer than the one they already hold. The Settings > Agents
page and its navigation tree present the saved order; profile selectors outside
that surface retain their existing order, recency, and default-selection
behavior. The page exposes no automatic profile-sorting action.

Agent-card display order stays with `sortAgentsByDisplayOrder` in the settings
controller and `orderAgentsForDisplay` in the web app. The Dynamic agent
(`agents.DynamicAgentID`, name `dynamic`) is excluded.

## Requirement mapping

| Requirement                            | Design section                                                                                                                                                  |
| -------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-AGENTS-PROFILE-LIST-ORDERING-001` | [Frontend](#frontend), [Save coordination](#save-coordination)                                                                                                  |
| `REQ-AGENTS-PROFILE-LIST-ORDERING-003` | [HTTP](#http), [Persistence](#persistence), [Order revisions](#order-revisions), [Store updates](#store-updates), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `store.Repository` gains `ReorderAgentProfiles(ctx, agentID, orderedIDs)`,
  returning the resulting revision and whether the order changed, plus
  `GetAgentProfileOrderSnapshots(ctx, agentIDs)` for snapshot reads. It returns
  each requested agent's ordered profile rows and revision together. The method
  lives in a new `store/sqlite_profile_order.go` (`sqlite.go` is far over the
  file-length limit). It reads requested profile rows and the revision map in
  one read-only transaction. PostgreSQL uses `sql.LevelRepeatableRead`; SQLite
  correctness relies on the read transaction retaining its WAL snapshot from
  the first query, not on `TxOptions` (the SQLite driver ignores its isolation
  setting). `GetAgent` and `ListAgents` use this result rather than separate
  profile and revision reads. Thus a concurrent reorder yields the pre-commit
  rows with the pre-commit revision or the committed rows with the committed
  revision, never a mixed pair. The reorder runs in one transaction, takes a
  PostgreSQL advisory lock via
  `pg_advisory_xact_lock(hashtextextended('agent-profile-order:' || agentID, 0))`
  before reading profile membership, then reads the agent's non-deleted global
  profile IDs (`workspace_id = ''`) in list order, requires set equality with
  `orderedIDs`, and, only when the sequence differs, writes
  `sort_order = index + 1` and increments the agent's revision. On SQLite, it
  reserves the single writer before reading membership with a no-op update of
  the owning `agents` row. It never writes profile `updated_at`.
- Global-membership writes acquire the same per-agent serialization lock in
  their transaction: `CreateAgentProfile`, `DuplicateAgentProfile`,
  `DeleteAgentProfile`, and `DeleteAgent` (which cascades profiles).
  `UpdateAgentProfile` and `UpdateAgentProfileWithDynamic` are full-row writes
  of `agent_id` and `workspace_id`; each acquires a stable per-profile advisory
  lock, plus per-agent locks for any known global source/target, even when the
  preliminary read suggests no membership change. Acquire all per-agent keys in
  sorted order before the per-profile key, then reread the source row under the
  transaction. If its current global owner is not covered by a held per-agent
  lock, roll back and retry with the newly observed owner; never write under a
  stale lock set. The per-profile lock also serializes competing promotions
  from a workspace-scoped row, which have no shared old-agent key. Global
  creates and duplicates know the target agent ID; deletion resolves the
  candidate owner, acquires the same lock order, and rechecks owner and scope
  before its soft-delete. A changed owner invalidates the preliminary lookup
  and retries from a fresh transaction. PostgreSQL uses
  `pg_advisory_xact_lock(hashtextextended('agent-profile-membership:' || profileID, 0))`
  for the per-profile lock. SQLite reserves its single writer before reading
  membership with a no-op update of the owning `agents` row inside the
  transaction; membership changes use the same write-reservation step.
- `ListAgentProfiles` orders rows by `sort_order ASC, created_at DESC, id ASC`.
  `filterGlobalProfiles` in `controller/agent_crud.go` still filters
  workspace-scoped rows and preserves input order. `GetAgent` and `ListAgents`
  expose the persisted profile order and `profile_order_revision` from the same
  `GetAgentProfileOrderSnapshots` result.
- Selector-facing projections restore the pre-feature per-agent baseline
  (`created_at DESC, id ASC`) from profile timestamps before constructing
  `AgentProfileOption` values. Direct selector paths that flatten `Agent.profiles`
  use the same projection; context-specific recency and default-selection
  behavior remains as it was. Settings page and navigation projections continue
  to use the persisted order.
- `handlers.httpReorderAgentProfiles` binds the body, maps errors, and, when the
  order changed, broadcasts the event with `h.hub.Broadcast`, next to
  `broadcastProfileEvent`, with a `//ws:global` comment: only global profiles are
  ever reordered. The route uses the same `cfg` permission and `h.interlock`
  middleware as `POST /agents/:id/profiles`.
- The web client adds `reorderAgentProfilesAction(agentId, profileIds)` beside
  the other `*Action` exports in `app/actions/agents.ts`, returning the committed IDs and revision, or rejecting with an API error.
  A 15 s abort timeout enters the queue failure path; HTTP 409 enters its
  membership-refetch path.
- `lib/settings/agent-profile-order.ts` holds the pure helpers:
  `reorderIds(ids, activeId, overId)` and
  `insertFirstInAgentGroup(options, agentId, option)`.
- The settings slice owns the order state and the save queue, see
  [Store updates](#store-updates) and [Save coordination](#save-coordination).
  `lib/settings/profile-order-queue.ts` holds the queue functions, which take
  the store, so the WebSocket handler, the page, and every list use one queue.
- `hooks/domains/settings/use-profile-order.ts` is a thin accessor over the
  queue, mounted once on the Settings > Agents page and passed down.
- `AgentProfilesSubList` in `components/settings/agents/agent-profiles-section.tsx`
  wraps rows in a dnd-kit `DndContext` and `SortableContext`, following
  `components/task/sidebar-filter/automatic-color-rule-list.tsx`. The sortable
  row is a separate component so `ProfileRowCard` does not grow.
- `InstalledAgentsHeader` in `app/settings/agents/page.tsx` keeps the existing
  Terminal, Rescan, and agent-creation actions. It has no profile-sorting action.
- `lib/ws/handlers/agents.ts` handles the new event.

## Data and contracts

### Persistence column and table

```sql
ALTER TABLE agent_profiles ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS agent_profile_orders (
    agent_id TEXT PRIMARY KEY,
    revision INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (agent_id) REFERENCES agents(id) ON DELETE CASCADE
);
```

`sort_order INTEGER NOT NULL DEFAULT 0` is added in four places in
`store/sqlite.go`, like `command_prefix` and the columns after it:

1. The `agent_profiles` `CREATE TABLE`.
2. The `CREATE TABLE agent_profiles_new` statement in
   `recreateAgentProfilesWithoutModelCheck`.
3. A `srcHasSortOrder := columnExists(...)` guard that appends the column to
   `srcCols` and `dstCols` of its copy statement.
4. An `r.migrate.Apply("agent_profiles.sort_order", ...)` after the
   `migrateDropModelCheckConstraint` block, which upgrades existing databases
   and is a tolerated duplicate on fresh ones.

`agent_profile_orders` is created with the other `CREATE TABLE IF NOT EXISTS`
statements. A missing row means revision `0`. The reorder transaction upserts
the row with `revision = revision + 1`; the primary key makes the increment
atomic per agent. The table is listed in the `agent-settings` required-store
descriptor and verified by its fixed conformance adapter. The explicit-tag
`v0.93.0` upgrade manifest asserts that existing profiles receive
`sort_order = 0` on SQLite and Postgres; the historical SQL fixture itself
remains unchanged.

All existing profile rows keep `sort_order = 0`, so the first read orders by
`created_at DESC` exactly as today. A reorder writes positions `1..n` to global
rows only. A profile created or duplicated later has `0` and sorts first, which
matches newest-first. Workspace-scoped rows keep `0`. Creating or deleting a
profile does not change the revision. Moving a profile to another owner or
scope resets its old group rank to `0` inside the locked transaction, so a
profile entering a global group cannot inherit a saved position from its prior
group. Surviving profiles keep their saved relative order.

### HTTP

`PUT /api/v1/agents/:id/profiles/order`, body `{ "profile_ids": ["..."] }`.

| Status | Condition                                                                                                           |
| ------ | ------------------------------------------------------------------------------------------------------------------- |
| `200`  | Saved or already equal. Body `{ "agent_id": "...", "profile_ids": [...], "revision": 7 }`                           |
| `400`  | Malformed body, empty list, an ID longer than 255 bytes, or the Dynamic agent (`code: "profile_order_unsupported"`) |
| `404`  | Agent not found                                                                                                     |
| `409`  | `{ "code": "profile_order_stale" }`: list is not exactly the agent's current global profiles                        |
| `403`  | Caller lacks agent-configuration permission                                                                         |

A request equal to the stored order returns `200` with the current revision and
without a write or event. `GET /agents` and `GET /agents/:id` carry
`profile_order_revision` on each agent. The route is added to
`mutatingSettingsRoutes` in `handlers/agent_settings_org_scope_test.go` and to
the interlock table in `handlers/interim_settings_interlock_test.go`.

### WebSocket

`agent.profiles.reordered` with
`{ "agent_id": "...", "profile_ids": [...], "revision": 7 }`. The action
constant lives beside `ActionAgentProfileUpdated` in `pkg/websocket/actions.go`,
and the payload type is added to `BackendMessageMap` in
`apps/web/lib/types/backend.ts`. The IDs describe membership at the reorder
transaction's commit and are an order patch, not an authoritative profile set:
clients reorder IDs present in the current agent group, ignore IDs no longer
present, and preserve current profiles omitted from the event. Delivery order is
not relied on: broadcasts can interleave with create/delete events and reorder
events can arrive out of commit order; revision comparison applies to reorder
events.

## Control flow

### Order revisions

The slice holds, per agent, `ProfileOrderSync`:
`{ revision, order, inFlight, queued }`: the newest server order and revision
this client knows (`order` is `null` until one is known) and the pending
optimistic orders. Fresh creation/deletion events and duplicate publications
atomically update rollback membership and retain pending overlays, including
when a legacy owning agent omits its order-revision field.

`acceptServerOrder(agentId, ids, revision)` stores the order and revision only
when `revision` is greater than the stored one, or when no order is stored yet,
and returns whether it did. Every source of a server order goes through it:
the `200` body, the event, and the `profile_order_revision` on each agent of a
fetched or hydrated snapshot. A snapshot with a revision not newer than the
stored one does not replace the known relative order. Fresh membership
snapshots separately prepend previously unknown IDs and remove absent IDs,
even when the order revision is unchanged. A later order acknowledgement
preserves known profiles created after that reorder began. The client epoch
fence prevents pre-event snapshots from changing membership.

The repository snapshot method reads ordered profile rows and their revision
map in one read-only transaction. PostgreSQL uses `sql.LevelRepeatableRead`.
SQLite's driver ignores the requested isolation enum; correctness comes from a
file-backed WAL read transaction retaining its snapshot from the first query
while the independent writer commits. A deterministic two-connection test
uses a package-private after-profile-query barrier in the same private
implementation called by the production method. It commits a reorder on the
writer while the read transaction is paused, then proves the in-flight result
is O/r and the next snapshot is O2/r+1 on SQLite and PostgreSQL. The test must
exercise the repository implementation rather than repeat its SQL reads.

`reconcileAgentOrders(agents, sync)` is the pure function for the Settings
projection. It applies overlays to `settingsAgents` only. The selector-facing
`agentProfiles` projection keeps the pre-feature per-agent baseline
(`createdAt DESC, id ASC`) and does not consume persisted order IDs; existing
selector-specific recency and default-selection logic remains unchanged.

### Store updates

- `setSettingsAgents` reconciles order overlays; `setAgentProfiles` preserves
  selector ordering and does not apply the Settings order. `profile_order_revision`
  protects order only and is unchanged by create/delete.
- `agentProfiles.version` is the client-local profile snapshot epoch, distinct
  from the backend order revision. The profile created, updated, and deleted
  WebSocket handlers and accepted local membership mutations advance it. Every browser `GET /agents` result written
  directly to the live store captures the epoch before the request and applies
  both `settingsAgents` and `agentProfiles` atomically through
  `applyAgentListSnapshot(agents, epoch)`. The action rejects the result if the
  current epoch differs; otherwise it reconciles `ProfileOrderSync` for the
  Settings projection while rebuilding selector options in their baseline order.
  A rejected result is discarded and the caller uses a fresh resource read
  rather than writing either list. On acceptance, fresh profiles replace
  flat-list groups for agents in the snapshot, while options for agent IDs
  absent from it, and workspace-scoped options owned by a refreshed agent, are
  retained. Workspace-scoped Office agents are excluded from
  `GET /agents`, so their options remain available to Office pickers across
  unrelated list refreshes. An existing-agent save captures the epoch before
  sending its requests. Its response still applies agent-level fields after the
  epoch changes, but keeps the current stored profile list instead of applying a
  profile snapshot that could undo create, update, or delete events received
  during the save.
- `AgentListResourceScope` captures the epoch at request start, rejects and
  retries a response if profile events advanced it while the request was in
  flight, and keys its cached response by that epoch. Direct browser list
  requests, including `handleCreateCustomTUI` and the missing-profile fallback
  in `use-agent-profile-settings.ts`, use the same guarded action rather than
  writing raw response arrays. `loadSettingsInitialState` repeats its complete
  read until the epoch is stable; `hydrateSettings` rejects an older incoming
  epoch, preserves live membership in both projections, and leaves
  `settingsData.agentsLoaded` false so the list is retried.
- The order revision acceptance rule cannot prevent a stale snapshot from
  deleting or resurrecting membership. `reconcileAgentOrders` only reconciles
  Settings ordering; the epoch fence above handles membership freshness.
- `setAgentProfileOrder(agentId, ids)` reorders that agent's entries in
  `settingsAgents` only. It does not reorder the flat `agentProfiles` selector
  list, rebuild it from `settingsAgents`, or discard orphan options.
- The `agent.profile.created` handler, `applyProfileDuplicated` in
  `hooks/domains/settings/use-profile-duplicate.ts`, and
  `hooks/domains/settings/use-agent-creation-store-sync.ts` place newly created
  profiles first in their own Settings group. The creation publisher retains
  current-main revision checks, accepted-creation metadata, and missing-owner
  protection. Non-creation saves use `agent-save-store-sync.ts`, preserve current
  membership when the request epoch is stale, and update only the saved group's
  known global selector options. Workspace-scoped and unrepresented Office
  options survive. Selector projections restore the creation baseline and do
  not consume Settings ranks. The Office setup writer updates the live selector
  list without replacing it with its potentially smaller wizard snapshot and
  mirrors a created global profile into its existing Settings owner.
  `components/agent/cli-profile-editor.tsx` returns the profile to its caller
  and writes no store.

### Save coordination

All per-agent state lives in the slice, so the page, every
`AgentProfilesSubList`, and the WebSocket handler share it.

1. `requestProfileOrder(agentId, ids)` sets the optimistic overlay and applies
   `setAgentProfileOrder`.
2. If nothing is in flight for the agent, it moves `ids` to `inFlight` and sends
   the PUT. Otherwise it replaces `queued`, so two reorder requests for one
   agent never overlap and the latest request wins.
3. Events and snapshots are never suppressed: they update the known server order
   through `acceptServerOrder`, and the overlay keeps masking it while an intent
   is pending. Own echoes carry the revision the `200` returns and are therefore
   no-ops.
4. On `200`, `acceptServerOrder(agentId, ids, revision)` runs, `inFlight` is
   cleared, and the next PUT starts from `queued` if set. When both are clear the
   overlay is gone and the known order shows. A foreign order with a higher
   revision that arrived meanwhile is already the known order, so it wins over
   the own `200`.
5. On network, `5xx`, or timeout failure, if a newer `queued` intent exists,
   promote it to `inFlight` and submit it without clearing the optimistic
   overlay. Do not roll back the newer intent or show a terminal-failure toast
   while it is still being submitted. If there is no queued intent, clear the overlay and show the known
   server order with an error message.
6. On `409`, capture the current
   `agentProfiles.version` before refetching `listAgents({ cache: "no-store" })`.
   Retain a newer queued intent and its overlay while refetching. Apply the
   response only through `applyAgentListSnapshot`; if a create/delete event
   advanced the epoch during the request, discard the response without changing
   either list and obtain a fresh result through `AgentListResourceScope`. Only
   after an accepted snapshot, reconcile against the latest membership in the
   store: place newly present IDs first in their refreshed server order, then
   retain queued IDs that still exist in their queued relative order. This
   keeps a profile created after the queued drag first and excludes deleted
   IDs. Submit the reconciled intent after the refetch, using the latest queued
   value if a further drag arrived meanwhile.
   If no newer intent exists, clear the overlay and show the
   accepted server order. A refetch failure clears the overlay and shows the
   known server order with an error. Other agents' pending overlays and queues
   remain independent.

Other clients, and this client while idle, apply the event through
`acceptServerOrder` and `setAgentProfileOrder`. `setAgentProfileOrder` treats
event IDs as an order patch over current membership, preserving profiles added
after the reorder commit and ignoring IDs already deleted. The Settings
navigation tree (`use-settings-menu-branches.ts`) reads `settingsAgents`, so it
follows without extra code.

### Frontend

- Each `ProfileRowCard` gets a handle button (`IconGripVertical`) with
  `data-testid="agent-profile-drag-handle"`, `touch-none`, the shared 28 px
  desktop control size and 44 px phone/coarse-pointer hitbox, an `aria-label` from `agents:dragProfile` with `{{name}}`, and the
  dnd-kit `aria-roledescription` overridden with `agents:profileSortable`
  (the repo pattern in `automatic-color-rule-card.tsx`). The handle is the only
  drag activator (`setActivatorNodeRef`) and sits in the `z-10` action layer
  above the row's overlay link, so row links and action buttons stay clickable.
  Instructions and start, move, drop, and cancel announcements use translated
  profile names and positions through `profile-drag-accessibility.ts`.
  Handles render only when `canManage` is true and the agent has two or more
  profiles.
- Sensors: `PointerSensor` with `distance: 8` and `KeyboardSensor` with
  `sortableKeyboardCoordinates`. The handle uses `touch-none`, so a touch drag on
  the handle arrives as pointer events and starts after 8 px of movement; page
  scroll outside the handle is unaffected. No `TouchSensor` is registered.
- Each `AgentProfilesSubList` has its own `DndContext`. Pointer and touch collision
  detection requires containment in one of that group's rows; a drop over another
  agent or empty space has no `over` target and changes nothing. Keyboard
  navigation resolves the nearest sortable row within the same group.
- The Installed agents toolbar has no automatic profile-sorting control. Its
  actions remain Terminal, Rescan, and agent creation; Rescan stays immediately
  before creation and creation stays rightmost
  (`AC-AGENTS-SETTINGS-PROFILE-LAYOUT-001.4`). The desktop and mobile layout
  specs assert the toolbar test IDs `["open-host-shell", "rescan-agents-button",
"new-agent-button"]`.
- Copy lives in the `agents` namespace in all locales: `dragProfile`,
  `profileSortable`, `profileOrderSaveFailed`, and the five drag instruction
  and announcement keys. There is no sort-action label.
  Traditional Chinese comes from `pnpm run i18n:zh-hant` and the pseudo catalog
  from `pnpm run i18n:pseudo`.

## Failure and recovery

- Network or `5xx` failure, or timeout: step 5 of Save coordination. An aborted
  request may still have committed; its event or the next snapshot carries a
  higher revision and converges the page.
- `409`: step 6 of Save coordination. The refetch captures the client membership
  epoch and applies only through `applyAgentListSnapshot`; if the epoch changed,
  it discards the response and obtains a fresh resource result before reconciling
  or replaying queued IDs.
- Concurrent reorder and global-membership mutations serialize on the same
  PostgreSQL per-agent advisory lock. If a create/delete commits first, reorder
  reads the changed set and rejects stale IDs; if reorder holds the lock first,
  it commits against the exact set validated, then the mutation follows.
  `DeleteAgent` takes the agent lock before its cascading delete. Ownership
  updates acquire candidate agent locks in sorted order, then the profile-ID
  lock, reread the source row, and retry from a fresh transaction if the current
  global owner is not locked. This serializes competing moves such as A→B and
  A→C and promotions from a workspace-scoped row. SQLite reserves its single
  writer before membership reads, serializing the same paths. Multi-connection
  tests on SQLite and Postgres cover create/delete, competing ownership/scope
  moves, and reorder in both lock acquisition orders.

## Persistence

`agent_profiles.sort_order` and `agent_profile_orders.revision` are install-wide,
not user-scoped. The reorder is a single transaction per agent, so no reader
sees a half-written order or an order without its revision. Membership-changing
transactions use the same lock as reorder so their commit cannot straddle the
set-validation-and-write interval. Soft-deleted rows keep their `sort_order`
and are excluded by `deleted_at IS NULL`. No index is added: reads already
filter by `idx_agent_profiles_agent_id` and sort a small set. Postgres and
SQLite share statements except for the per-engine lock acquisition.

## Security

Only callers with the agent-configuration permission reach the route; the
interim settings interlock applies as for profile writes. The handler rejects
IDs that do not belong to the addressed agent, so a caller cannot reorder or
probe another agent's profiles. Workspace-scoped profiles and the Dynamic agent
are never written by this route.

## Observability

The existing handler error path reports unexpected repository failures. No
reorder-specific logs or metrics are added; expected stale membership errors
are returned as HTTP 409.

## Consumers of profile order

Only Settings > Agents and its navigation tree consume the persisted order.
Selector projections restore `createdAt DESC, id ASC` within each agent, with
submillisecond timestamp precision and ID tie-breaking. Boot options recover
creation timestamps from their matching normalized profile snapshot. Unstamped
legacy options and workspace-scoped Office options retain their slots; only
stamped global profiles are reordered. Options for agent IDs absent from a
Settings snapshot are retained.

Consumers that read the first profile as a default, including Office chat and
Office setup, therefore retain their pre-feature default. Existing contextual
recent-use rules remain independent of Settings order. The Office routing
provider sorts its execution-profile catalog by name and ID and is unaffected.

## Related decisions

- [ADR 0005](../../../decisions/0005-agent-model-unification.md) introduced the
  `workspace_id`-scoped profile rows this design excludes. No new ADR is needed.


### New-owner creation compatibility

A successful new-agent POST identifies an accepted new owner independently of
unrelated catalogue epoch changes. Both successful and partial MCP continuations
publish through the creation boundary with that explicit new-owner intent; they
retain current independent options and current target capability metadata.
Existing-owner creation never recreates a missing owner. While the app store
lives, its subscription records identities actually removed from the current
catalogue, including after an editor unmounts while its save is pending. The
observation belongs to that store instance and does not cross providers. A
later new-owner callback cannot reinsert one of those observed removals. An
identity never present in that catalogue is not
classified as deleted merely because another profile advanced the epoch.
