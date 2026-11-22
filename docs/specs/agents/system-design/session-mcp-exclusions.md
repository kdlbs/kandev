---
status: current
system: agents
requirements:
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-001
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-002
  - REQ-AGENTS-SESSION-MCP-EXCLUSIONS-003
created: 2026-10-22
owners:
  - kandev
---

# Session MCP Exclusions System Design

## Boundary and grounding

Agents owns catalog composition and desired/applied state. Tasks supplies
authorized workspace, repository, profile, and session identities. Platform
continues to own session attachment evidence. This design extends
[configuration](workspace-mcp-configuration.md),
[pending comparison](workspace-mcp-pending-changes.md), and
[operation-owned delivery](workspace-mcp-resolution-delivery.md).

Grounding is PR #3293 at `193de56ba2597b335a97b1e5ab01d09340f22647`.
Its source has positive selections only. `SelectionService.Replace` does not
accept a client revision, and `ResolutionInputs` does not contain exclusions.
`MCPSetupSnapshot` version 1 stores selected descriptors, including disabled
definitions. It does not prove delivery or connectivity.

This extension introduces a session exception, not precedence among selecting
owners. The [composition ADR](../../../decisions/2026-09-01-workspace-mcp-configuration.md)
records the narrow exception.
The feature-local alternatives and lifetime decision below preserve sufficient
rationale without a second independent ADR.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-SESSION-MCP-EXCLUSIONS-001 | Composition, persistence, API, authority |
| REQ-AGENTS-SESSION-MCP-EXCLUSIONS-002 | Snapshot, lifecycle, recovery, evidence |
| REQ-AGENTS-SESSION-MCP-EXCLUSIONS-003 | Editor and responsive contract |

## Composition and lifetime

Let U be the existing positive union by definition ID. Let E be the exclusions
saved for this workspace and session. The desired catalog membership is U minus E.
Enabled definitions then pass through existing secret, executor, and provider
policy. An exclusion covers every origin, including a positive session selection.
Neither the exclusion nor restoration rewrites those positive selections.

Store exclusions by definition ID, never runtime name. They survive same-session
restart, reset, profile changes, repository changes, and archive/reopen. A
profile change may make an exclusion dormant. A new session identity starts
with no exclusions, even when created by duplication. Cross-workspace resolution
must not reuse a prior workspace's exclusions. Existing ownership checks reject
foreign definitions. Do not broaden workspace-transfer behavior in this feature.

An exclusion remains until explicit restoration, definition deletion, or session
deletion. If U no longer contains its definition, show it under Saved exclusions
with No longer inherited. Restoring a dormant exclusion removes only E and does
not add a positive selection. If U later contains that same ID, E suppresses it.
Renaming retains the ID. Delete/recreate does not inherit suppression.

Disabled inherited definitions can be excluded or restored. Restore means
follow inheritance, not enable the definition. An excluded-and-disabled row
shows both facts. Session-only additions keep their existing removal control.
The exclusion action is limited to inherited rows and previously saved exclusions.

## Persistence and atomic writes

Add `task_session_mcp_exclusions` in the existing agent settings store, with
`workspace_id`, `task_session_id`, and `mcp_server_id`. Use a unique key over
all three and a definition foreign key with deletion cleanup. Task/session rows
live in a different store, so use the established owner validator and lifecycle
cleanup callbacks rather than inventing a cross-database foreign key.

Fresh creation and migration replay leave E empty. Extend settings-store schema
initialization and conformance fixtures for SQLite and PostgreSQL. No accepted
snapshot backfill or automatic revision increment is needed for existing rows.

Extend the conditional session repository operation owned by registration
assignment work order 01. Atomically replace positive session selections when
supplied, replace exclusions when supplied, and advance the desired revision.
Preserve accepted setup, applied revision, and attempt ID. Reuse that operation's
workspace/scope/owner lock identity, deterministic catalog lock order, and exact
committed response. PostgreSQL acquires its advisory transaction lock before
reading; SQLite uses immediate writer admission. Lifecycle CAS and this writer
must not overwrite each other's baseline. Do not add a competing operation or
rely on `SelectionService.stateMu` across instances.

Validate additions and exclusions under the write transaction against the
authorized workspace. New exclusions require a current inherited contribution.
Retained exclusions may be dormant. Enabled-state validation remains required
for newly added positive selections. Retaining or removing a previously selected
disabled definition must not block an unrelated exclusion edit.

Extend `DeleteMCPTaskData`, `DeleteMCPWorkspaceData`, definition deletion, and
individual-session deletion wiring to remove exclusions. The workspace cleanup
must handle sessions with only exclusions and no positive session selection.
Exclusions do not count as positive Selected/Used in associations. Definition
deletion clears them without changing that sibling-owned usage contract.

## API and compatibility

Extend only the existing task-session `GET/PUT /api/v1/task-sessions/:id/mcp-selections`.
The response adds `excluded_definition_ids` alongside the assignment package's
`selection_revision`. That value is the existing session `desired_revision`,
with zero before the first write, not an independent counter. Read both saved
sets and that revision in one consistent snapshot. Existing desired-state
changes can invalidate the precondition even if positive membership is unchanged;
applied-status changes alone do not. Runtime acceptance still uses its own CAS.

The PUT adds optional `excluded_definition_ids` and reuses `expected_revision`.
Presence of the exclusion field requires the revision. `[]` explicitly restores
all exclusions. Omission preserves E in the same transaction, including requests
from the registration-assignment sibling. `null` is invalid, not a clearing value.
The existing positive `definition_ids` and `mcp_server_ids` alias remain compatible.
An exclusion-only request can omit both positive fields and preserve additions.
An old request without an exclusion field retains its positive replacement behavior.

The updated session editor supplies the latest revision for every write.
Exclusion actions send both sets; addition-only saves and Retry omit exclusions
to preserve them in the shared transaction. A stale expected revision returns 409 with a stable
`mcp_selection_conflict` error code, including when submitted sets match current
sets. It performs no mutation or provider notification. The shared controller retains addition drafts separately from committed state
and offers Reload choices. Exclusion actions show only committed state and must
be repeated explicitly after reload. No failed action is replayed automatically.
Old addition-only clients remain supported but must acquire the same transaction
lock, preserve E, and increment the latest desired revision.

Reject exclusion fields on repository/profile/task routes. Authorize workspace
and session using stored ownership before reading definitions. Unknown and foreign
IDs fail without disclosing foreign metadata. Reserved built-in names and synthetic
legacy IDs are not valid definition IDs. A rejected combined write changes neither
set. Return the exact committed sets and revision, not a later writer's state.
Only a successful commit schedules the existing apply notifier.

If a response is lost after commit, reload before retrying. Do not assume failure
means rollback. Same-set writes retain the existing retry behavior: a new desired
revision may request application, but cannot erase an accepted snapshot. Changing
only E also advances desired revision. The assignment package owns this shared
session write/retry rule; this package adds mixed addition/exclusion race tests.
Non-session `expected_definition_ids` membership preconditions are owned by the
assignment package and are not duplicated on the session route.

This contract supersedes the crossed proposal to compare session positive
and excluded ID sets. Reject session `expected_definition_ids` and
`expected_excluded_definition_ids` instead of ignoring them. A supplied null,
negative, fractional, or malformed `expected_revision` is invalid. Non-session
routes reject `expected_revision`. Assignment owns these shared validation rules.
The assignment-first dependency below supersedes whichever-lands-first ordering.

## Resolver and snapshot

Extend `ReadMCPResolutionInputs` and `ResolutionInputs` with exclusion IDs read
inside the same catalog/selection snapshot. PostgreSQL keeps REPEATABLE READ.
Maintain the unfiltered origins map for presentation and legacy-name suppression.
Subtract E before `deliveryServer`, secret lookup, managed package preparation,
and provider filtering. Emit safe `MCPResolutionDecision` reason
`session_excluded` with definition identity, revision, and every origin.

Do not remove excluded IDs from `addLegacyFallback`'s selected-name guard.
Otherwise the legacy fallback could reinject the same server by name. Unmigrated
synthetic legacy identities remain outside E. Provider-owned native configuration
can still supply a similarly named server. No native file or independent tool
permission changes follow from a catalog exclusion.

Build desired `MCPSetupSnapshot` from definitions in U minus E using a shared
pure helper. Use that helper for both runtime preparation and comparison.
The snapshot stays version 1: omitted IDs mean absent selection intent, while
disabled descriptors retain `enabled=false`. Existing snapshots already have
those semantics and require no conversion.

Extend `MCPSetupComparison` with the exclusion IDs corresponding to its saved
revision. The frontend verifies positive IDs, exclusion IDs, and revision before
using a comparison. Missing exclusion identity is valid only for an old response
with an empty E. Preserve known-empty versus unavailable, descriptor bounds
(1,024 entries and 256 KiB), and sanitization. Include the added IDs in the
existing bound check. Oversize explanation becomes unavailable, not truncated
exact changes. Exclusion persistence does not inherit explanation limits.

Exclusion from an accepted set yields remove. Restoration yields add if U still
contains the definition. Reverting before acceptance can yield no changes while
application acknowledgment remains pending. Disabled descriptors compare as
setup intent, so a removal must never be described as a disconnected server.

## Lifecycle, recovery, and evidence

Reuse `RequestSessionMCPReconfiguration`, `applyPendingSessionMCP`,
`prepareExecutionMCP`, and `SaveAcceptedMCPSetup`. Do not add a reconnect worker.
The prepared operation freezes E's result with its exact setup and desired
revision. All actual start/reset/rebind paths use that same resolver.

Retain startup lease then prompt-lock order, client ownership, operation deadline,
and accepted-attempt fencing. A busy turn saves pending state only. Resume/load
eligibility and strict runtime-setting restoration remain as shipped. Project
MCP strategies, passthrough, absent execution, and unsupported ACP defer to an
actual process start. Live-process adoption is not a start or acceptance.

Provider failure, timeout, or post-RPC restoration failure leaves the last
accepted snapshot authoritative as a historical baseline. Do not claim that
the runtime rolled back: a provider may have partly accepted the operation.
Keep the failed choice and retry. Newer desired revisions survive older success
or failure, and stale results cannot affect a replacement execution.

On backend restart, restored saved state and accepted evidence remain separate.
A retained active execution keeps its delivered set. The existing next eligible
idle/start path applies the saved exclusion. Reopening the editor never starts
an agent. Connection health continues to come from the attachment evidence path.

Pass `session_excluded` through the existing safe resolution metadata pipeline,
including bounded reason-code consumers and localization. Never send an executable
placeholder for an excluded server. Do not synthesize disconnection or tool-count
events. Exclusion decisions remain distinct from `definition_disabled` and
provider transport filtering. No new metrics or endpoint probes are required.

## Built-in and sibling authority

Kandev's reserved endpoint remains separately injected. This control cannot remove
its lifecycle, question, completion, or other built-in capabilities. Do not
introduce a built-in catalog record, optional-group policy, tool pruning, or a
mandatory-tool list. The built-in-management sibling owns those decisions.
Customize includes a short explanation that required Kandev tools are managed
separately. Connections retains actual built-in evidence without a new toggle.

The management-clarity sibling owns Used in navigation and neutral connection
presentation. Registration assignment task
`5a8cb5cd-22ee-42e5-bd6d-e2179e899c8c` owns optional assignment after creation,
non-session membership preconditions, the session revision precondition, shared
transaction serialization, consistent reads/exact committed responses, and
conflict/retry semantics. This package owns E persistence, presence-aware
combined writes, inherited-target validation, composition, and exclusion UI.

Before local work order 01, integrate the assignment package's committed
`docs/plans/mcp-assignment-after-registration/task-01-guard-selection-replacement.md`
implementation. Reuse its primitive and reconcile conflicts by intent. Do not
copy sibling uncommitted code or implement a second CAS API. Assignment work
orders 02/03, management, and built-in management are not prerequisites. The prerequisite was integrated at `f9137074309bebe99e2c7576baefe936bc20358f`
after direct user implementation authorization on 2026-10-23. The exclusions
branch stacks on `feature/clarify-mcp-assignme-mhh`.

## Editor and responsive contract

Keep one `useTaskSessionMCPSelection` controller above responsive composition.
Extend `useMCPSelectionEditor` and typed `mcp-api.ts` requests with the session
options. Preserve committed state, context generation, save/read guards, and
the existing application poller. Do not construct an independent exclusions poller.

Extend `MCPSessionSelectionBody` and `McpOriginSections` through an explicit
existing-session exclusions capability. `sessionPresentation` alone is not
authority: the same body appears in new-session creation. Shared repository,
profile, task, and creation pickers remain additive and read-only for inheritance.

Inherited rows retain task/repository/profile grouping and all origin names.
Add a labelled Exclude for this session button, replaced by Restore inheritance
after an accepted save. Keep the row in place. Show Saved: excluded from this
session and its pending comparison annotation. Include dormant rows in Saved
exclusions with their restore action. Search continues to filter additions only.
The pending section stays before origins and remains independent of search.

On rejection, retain committed choices and a local error. On conflict, retain
addition drafts separately, keep exclusion rows at their committed state,
announce that saved choices changed, and offer Reload choices followed by
explicit review and a repeated exclusion action. Reuse the assignment controller's
conflict handling. Application Retry writes the committed positive IDs directly,
without setting or acknowledging the addition draft. Do not offer Retry while a
save is running or a rejected save awaits Reload choices. Keep shared stale-page recovery ownership. Pending/failed/deferred timing stays in `McpSelectionNotice`, not
duplicated as connection badges. Use a translated live status region. Do not
display internal revisions in primary copy.

Desktop retains the existing `McpServerExplorer` dialog. Phone uses its full-height
drawer, and coarse-pointer tablets retain its 80dvh drawer. The nearest shipped
exemplars are that explorer and `MobilePickerSheet`/`mobile-menu-sheet.tsx` for
fixed headings and safe-area scrolling. This occasional multi-server edit needs
one focused sheet, not a nested confirmation or a new page.

The header and Connections/Customize selector stay fixed. One body scroller owns
status, origins, exclusions, and additions. Use `min-h-0`, dynamic viewport bounds,
safe-area padding, wrapping metadata, and contained overscroll. On phone, the
action follows the row's descriptive text at full available width. Desktop can
align the action beside the name. Ordinary desktop buttons remain 28px. Phone
and coarse-pointer hit areas are at least 44px in both dimensions.

Keep existing neutral surfaces, border/radius tokens, Tabler icons, and visible
focus. No new animation is needed for frequent exclude/restore actions. Existing
motion respects reduced-motion settings. All copy uses i18next in English,
pt-pt, zh-cn, zh-hk, zh-tw, ja, and ko, with generated Traditional Chinese and
pseudo output. No health colors or hover-only explanation convey saved state.

## Alternatives and verification

Editing upstream selections would affect sibling sessions. Browser-local state
would lose exclusions on restart. A one-turn expiry would unexpectedly restore
capabilities. Name-based suppression could capture a different definition.
The selected design preserves stable ownership and makes reversal explicit.

The [plan](../../../plans/session-mcp-exclusions/plan.md) maps acceptance to
transaction, resolver, lifecycle, hook, and rendered tests. Mock acceptance proves
Kandev's delivery contract only, not real-provider connection or tool revocation.
