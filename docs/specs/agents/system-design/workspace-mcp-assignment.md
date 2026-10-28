---
status: current
system: agents
requirements:
  - REQ-AGENTS-MCP-ASSIGNMENT-001
  - REQ-AGENTS-MCP-ASSIGNMENT-002
  - REQ-AGENTS-MCP-ASSIGNMENT-003
  - REQ-AGENTS-MCP-ASSIGNMENT-004
---

# Assignment after workspace MCP registration

## Boundaries and evidence

Implementation completion (2026-10-25): all three work orders are implemented
and verified. The management prerequisite is
published at `45e90466378f982e13cc762c52432f14ab43c32c`, including authorized
workspace-profile destination admission. Earlier design-only blockers are
historical; the route and metadata contracts are integrated.

Profile presentation follows management cleanup `7aca966a6ec262eb1ce839fa883f66e23441d477`:
one inline searchable checkbox section on desktop and phone, with no nested
picker opener or Manage workspace catalog link. The owner-context focus hint
attaches directly to the inline content. It still changes no checkbox/search/save
state; the shared Save changes action retains persistence ownership.

This design extends [workspace MCP configuration](workspace-mcp-configuration.md)
and [setup delivery](workspace-mcp-resolution-delivery.md). Agents retains
definition, selection, and runtime authority. The settings coordinator retains
[manual-save ownership](../../ui/system-design/settings-manual-save.md).

Source inspected at parent PR #3293 commit
`193de56ba2597b335a97b1e5ab01d09340f22647` establishes these integration points:

- `MCPSettings.saveDefinition` closes the custom editor after creation.
  `MCPMarketplaceView` discards the definition returned by `data.install`.
  Both must retain the acknowledged identity for the completion step.
- `useMCPWorkspaceSettings.create/install` already return `MCPServerDefinition`.
  Guard their state updates against a changed workspace and deduplicate by ID.
- `RepositoryMCPSelectionSection` and `ProfileMCPSelectionCard` register shared
  save contributors. `useMCPSelectionEditor` owns the common read/write path.
- `profile-mcp-config-card.tsx` mounts the catalog picker only when `workspaceId`
  is present. Global-profile routing remains legacy in the parent. A latent
  workspace selector in `ProfileMCPSelectionCard` is not a shipped destination.
- `mcp_selection_handlers.go` accepts full selection replacement without a
  precondition. Catalog revision checks and session desired/applied CAS do not
  protect a stale browser selection replacement. The parent design's general
  revision prose must not be treated as proof of that protection.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-AGENTS-MCP-ASSIGNMENT-001 | Registration receipt; Failure and recovery |
| REQ-AGENTS-MCP-ASSIGNMENT-002 | Owner handoff; Authorization and eligibility |
| REQ-AGENTS-MCP-ASSIGNMENT-003 | Conditional replacement; Failure and recovery |
| REQ-AGENTS-MCP-ASSIGNMENT-004 | Responsive composition; Validation |

## Registration receipt

Keep a small workspace-keyed completion record above responsive branches in
`MCPSettings`: definition ID and safe display identity from the successful
create/install response. Do not keep credentials, form configuration, or a
second copy of the catalog. The completed form closes before completion opens.
An edit uses existing feedback and does not trigger the new-registration step.

Custom registration retains its settings Save changes contributor. Marketplace
registration retains its explicit Add MCP server command. After acknowledgment,
insert or reconcile the returned row by ID, select Configured, and display
"Saved to <workspace>" with Done and optional Use in. No owner API is called yet.
The receipt is not a route guard, dirty draft, or unfinished installation.

Done clears transient completion state. Each existing catalog card has a Use in
action that reopens the same chooser after dismissal or reload. This is an
entry point on the current catalog, not a server-detail page or usage inventory.
Disabled definitions retain a readable reason and cannot enter assignment.

The completion callback cannot reject an acknowledged registration merely
because navigation, owner loading, or assignment fails. Creation and selection
have independent error boundaries. Disable duplicate submits while a create
request is in flight. An old workspace completion cannot open a receipt in the
new workspace.

## Owner handoff

The chooser is a read-only navigator. It never performs selection PUTs. Choose
one owner type, search authorized candidates, review the named destination and
scope explanation, then activate Open MCP settings. There is no default owner.
The destination can highlight the saved definition without changing its current
checkbox state. Settings reveal the MCP section; session navigation opens only
the named conversation. The session hint waits until the user opens the existing
plug/Customize. The user makes the actual selection through the existing picker. This avoids replayable
URL-driven writes and preserves all existing drafts and save semantics.

Use existing workspace repository reads (`useRepositories`/`listRepositories`),
metadata-only `listMCPProfileOwners` in the registering workspace, paginated
`listTasksByWorkspace`, and
`listTaskSessions` after one task is selected. Put fetching and request-generation
guards in a domain hook, not in presentational components. Search tasks with
bounded pages (25 items) and explicit pagination. Do not fetch every task's
sessions, read conversations, or use ensure/start APIs to discover candidates.
Do not treat a client-side filter over unauthorized inventory as authorization.
Profile inventory uses management's global-plus-exact-workspace metadata pages,
with stable cursors, cancellation and repeated-cursor failure. It never uses the
global `/agents` inventory as evidence that workspace profiles do not exist.
Only enabled, concrete, MCP-supported profiles are new assignment candidates.
The current owner and definition are read again before returning a destination.

| Owner | Destination and save semantics | Scope explanation |
| --- | --- | --- |
| Repository | Existing workspace Repositories editor, exact repository MCP section expanded; shared Save changes | All future attachments using this repository can inherit the selection. Current delivered turns stay unchanged. |
| Profile | Existing agent-profile editor with explicit registering workspace, MCP section; shared Save changes | Profile defaults apply only within this workspace, including when the profile is global. |
| Task agent session | `linkToTask(taskId, {sessionId})`, exact existing session, composer Customize; existing checkbox autosave | Adds to this session only. Apply state determines idle or next-start timing. |

Navigation identities are typed: workspace ID, definition ID, owner scope and
ID; profile links also carry agent ID, and session links carry task ID. Resolve
names from authorized records, never from query text. Decode and validate each
identity in the destination. Return navigation is a constructed catalog path,
never an arbitrary caller-provided URL. No "assign=true" flag or pending write
is stored in the URL or browser storage.

### Management sibling integration

Task `000d477e-f792-4c9d-8503-50ed73bb9646` owns management clarity, named owner
navigation, and the global-profile workspace picker. Its complete saved scope
was read on 2026-10-22. At that design checkpoint, it recorded design
reconstruction rather than implemented routes. No sibling code was present at
the parent base verified on that date. Management PR #4759 later supplied the
implemented routes and was squash-merged into main.

The sibling's 2026-10-22 saved-plan update proposes
`/settings/workspaces/:workspaceId/mcp-servers?serverId=:definitionId&view=usage`
as a detail view inside the existing catalog. This was a proposal at that
checkpoint and was later implemented by management. Assignment does not require
a usage view for its completion or recovery.
It consumes the sibling's owner destinations when their committed contract is
available, and can link this detail view later without creating another hub.

The sibling's system design section `#shared-owner-navigation-contract` records
`apps/web/lib/mcp/management-links.ts` and typed `MCPOwnerDestination`. Each
variant requires `scope`, `workspaceId`, and `ownerId`. Profile also requires
`agentId`; task_session also requires `taskId`, and its `ownerId` is the session
ID. Reject a conflicting duplicate `sessionId`. Never infer missing identity
from the active browser context. The task variant remains outside assignment.

| Scope | Owner destination |
| --- | --- |
| Repository | `/settings/workspaces/:workspaceId/repositories?repositoryId=:ownerId#mcp` |
| Profile | `/settings/agents/:agentId/profiles/:ownerId?workspaceId=:workspaceId#mcp` |
| Existing session | `linkToTask(taskId, {sessionId: ownerId})`, yielding `/t/:taskId?sessionId=:ownerId` |

Management owns base route construction, authorization and identity validation,
MCP section reveal/focus, and global-profile mounting. Workspace-owned profiles
reject mismatched workspace hints; global profiles accept only an authorized
workspace hint, without changing the active workspace. Repositories must belong
to that workspace. Sessions must belong to the named task, with no fallback to
another session. These behaviors ship in the integrated management prerequisite;
they were absent from the original parent PR.

Assignment owns an optional `definitionId` extension that appends the reserved
`mcpServerId` query key through `URLSearchParams`, preserving query and hash.
The catalog detail keeps its separate `serverId` and `view=usage` keys. Assignment
owns parsing and the following focus-only behavior:

1. Accept one nonempty definition ID. Reject duplicate, malformed, or empty hints
   without changing editor state. Resolve it only against the authorized
   destination workspace's existing picker inventory after that inventory loads.
2. Do not fetch broader inventory or expose a cross-workspace, disabled, removed,
   or unavailable definition. A filtered-out row stays filtered out. No hint can
   change search, checkbox state, an existing draft, or save state.
3. Reveal/highlight an available row once per destination navigation. Consume the
   hint with history replacement after processing, preserving all unrelated query
   keys and the hash. A pending session hint waits for the user to open the
   existing plug/Customize; it never opens that overlay. Context changes cancel
   pending processing. Invalid hints are consumed without selecting a fallback.
4. Do not replay a consumed hint on rerender, reopening Customize, or reload.
   A fresh explicit Use in navigation may provide another focus-only hint.
   Focus and highlight never trigger selection or application requests.

Assignment owns candidate choice, the registration receipt, and this hint
extension. Do not create a Used in page or a second profile editor. Work order
03 depends on the committed management navigation work order. Read that exact
committed package and reconcile its interface before implementation. Do not
import uncommitted sibling files or silently drop global profiles to claim
completion. Work order 03 must record the integrated SHA.

The existing Configured catalog remains the recovery destination; it does not
require the sibling's server-detail destination. Work orders 01 and 02 were
implemented, then reconciled with the integrated management base.

## Authorization and eligibility

Workspace authorization and `mcpSelectionOwnerValidator` remain authoritative.
The registering workspace is fixed for this flow. Global profile identity never
widens definition or credential access. Revalidate the current owner through
the existing authorized reads before navigation and again on selection save.
An invalid target displays a generic unavailable result without fetching its
name from a broader workspace inventory.

Candidate profiles must exist, be enabled, and be global or belong to this
workspace. Repositories must exist in this workspace. Sessions must already
exist under an accessible, unarchived task and support the existing Customize
editor; terminal/deleted or noncustomizable sessions are unavailable. A running
eligible session remains selectable because existing timing handles active
turns. Session choices and their scope review include the exact session ID so
conversations using the same profile remain distinguishable. Destination checks
retain the action button name and announce progress through a translated live
status region. Disabled-profile sessions are not new assignment candidates. Do not
invent a repository enabled flag where none exists.

Mutation validation rejects newly added definitions when the definition or
target becomes ineligible. Previously selected disabled definitions may remain
or be removed; they cannot be newly added. Retain removal and cleanup paths
without automatically enabling a profile or definition. Access loss fails
closed. Resource lifecycle cleanup remains owned by the existing task/settings
services; this flow creates no resource on a stale-owner failure.

## Conditional replacement

For non-session owners, extend the existing typed selection PUT with optional
`expected_definition_ids`, a snapshot of the direct IDs the editor loaded.
An explicit empty array means the known-empty snapshot. Omission preserves
legacy caller behavior; null or malformed values are rejected. Updated web
editors always send the field for profile/workspace, repository and task scopes.
This common-hook change does not add task assignment UI.

Compare normalized sets, not order. Sort copies of the saved and expected IDs
in Go. Database collation must not affect admission or mutate the returned
snapshot's order. The condition concerns direct membership, not inherited
membership, definition revisions, health, or runtime status.
An A-to-B-to-A membership change is equivalent to A for this operation. A
definition revision remains a separate catalog concern. No new revision table
or migration is required for this membership condition.

### Shared session contract with exclusions

Task `15641d1a-d7ba-4216-aefd-5a4c36a88d73` owns the draft exclusion capability.
Its `session-mcp-exclusions.md` design was read on 2026-10-22. Use its proposed
session response `selection_revision` and request `expected_revision` rather
than a second session membership precondition. `selection_revision` represents
the existing desired revision, with zero before the first write; it is not an
independent counter. Read positive IDs, exclusions when supported, and revision
in one snapshot. Check the supplied revision before any write or retry. A
supplied null, negative, fractional, or malformed revision is invalid. Reject
session `expected_definition_ids` and `expected_excluded_definition_ids` rather
than silently ignoring a caller's intended condition. Non-session routes reject
`expected_revision`; they use the membership condition defined above.

The combined compatibility contract is draft and not present in the parent:

- Assignment-origin addition writes send positive IDs and `expected_revision`.
  They omit `excluded_definition_ids`. Omission preserves current exclusions
  inside the transaction; it never means an empty exclusion set.
- Exclusions owns `excluded_definition_ids`, its persistence and validation.
  Explicit `[]` restores exclusions; null is invalid. Its presence requires
  `expected_revision`. Exclusion-only requests may omit positive fields to
  preserve additions. The existing positive-field alias remains compatible.
- Legacy positive-only clients that omit a revision retain replacement behavior,
  acquire the same session transaction lock, and preserve exclusions. Updated
  clients send the accepted `selection_revision`. Runtime desired/applied CAS
  remains separate from request admission.
- Session revision conflicts return 409 before mutation or runtime notification,
  even when the submitted positive set equals the current set. Lost ACK recovery
  reads authoritative state before offering explicit retry; it never blindly
  replays a write. Existing same-set writes may advance desired revision to retry
  application. Do not suppress that behavior as an assignment optimization.

Assignment work order 01 owns generic conditional admission for all scopes,
response/error naming, and the transaction lock and revision handling. The
exclusions sibling's work order 01 depends on that committed result and extends
its session operation with exclusion fields, validation, persistence, and atomic
integration. It does not implement another CAS primitive or revision counter.
Assignment does not depend on exclusions. This explicit order replaces the
earlier whichever-lands-first proposal. Record the committed operation and lock
identity in work order 01's results for the sibling to consume. Exclusions owns
mixed positive/exclusion client race tests after integration. Do not implement
exclusion storage or controls here, or copy uncommitted sibling code. Management
owns neither CAS API. Assignment work order 01 now supplies the committed
primitive; the exclusions extension remains separately owned and unimplemented
in this branch.

An inherited server excluded by that later capability stays excluded when an
addition-only request succeeds. Assignment must not claim runtime inclusion or
automatically restore it. Existing session Customize owns the explicit restore
action and its feedback when the exclusions capability is present.

### Transaction and recovery ownership

The conditional repository operation reads, compares, validates catalog rows,
replaces the set, and (for sessions) advances desired state in one transaction.
Return the exact committed membership snapshot; do not reread a later writer's
set and label that the submitted result. For GET, load membership and session
desired state consistently. Comparison/evidence failures remain separately
unavailable rather than converting a committed selection into a failed save.

SQLite uses its existing immediate writer admission. PostgreSQL uses a
transaction-scoped advisory lock keyed by workspace, scope, and owner before
reading membership, including empty membership. Existing unconditional
replacement, legacy profile import, and session replacement must participate
in the same owner serialization. Lock catalog definition rows in deterministic
ID order while validating enabled state, and coordinate catalog deletion/cleanup
so concurrent removal cannot recreate deleted references. Use a documented
consistent lock order and real multi-connection PostgreSQL tests. Never hold
these locks over runtime/provider I/O.

On mismatch, return 409 with stable `mcp_selection_conflict`, without replacing
rows or advancing desired state or notifying runtime. The hook keeps the draft,
shows Reload choices, and requires explicit review before the next save. An
ordinary 409 is not the shared stale-page interlock. Existing handled interlock
errors continue to use shared reload recovery. The existing-task edit dialog
also exposes this recovery action after its task selection write fails. Reload
reads the accepted selection without discarding task fields or MCP choices;
only a later explicit form submission can retry the write.

For sessions, increment the latest desired revision atomically under the shared
session writer and notify existing runtime application only after commit.
Preserve the accepted baseline and do not overwrite concurrent lifecycle fields.
An exclusion-only change must advance desired revision even when positive IDs
are unchanged. An accepted explicit same-set Retry also advances desired
revision and schedules existing application, without clearing applied revision,
accepted setup, or attachment attempt ID. Preserve the existing behavior rather
than introducing a new retry flag or deduplicating that write. A stale revision
still rejects before retry admission. Internal orchestrator session writes use
the same atomic store operation and workspace/owner/definition validation, but
do not apply the HTTP settings path's session runtime-eligibility gate. This
preserves selections accepted during session creation or launch for later
eligible application. The registration chooser issues no PUT.

The design uses the existing additive and atomic-selection boundary. The
membership precondition is narrower than a new assignment service or persisted
workflow. Its compatibility and race requirements are recorded here rather
than introducing another architecture decision.

## Failure and recovery

| Event | User outcome and recovery |
| --- | --- |
| Creation rejects before acknowledgment | Keep the setup draft and existing save error. Do not offer an invented receipt. |
| Creation result uncertain | Read this workspace's catalog; show potential matching configuration for review. Never automatically recreate or treat a runtime-name collision as proof this request succeeded. |
| Definition saved; target list fails | Keep Saved and Done visible. Retry read-only discovery or return to Configured. |
| Target deleted, disabled, or access revoked | Stop navigation/selection and offer another owner. Do not mutate or disclose the invalid owner. |
| Selection rejected/conflicts | Keep saved definition and owner draft separate. Reload and review membership, then use the existing save control. |
| Selection ACK lost | Read the exact owner membership before retry. If already selected, show saved state; otherwise require renewed explicit selection/save. Never replay create. |
| Selection committed; application failed/deferred | Existing Customize notice and retry own recovery. Do not call creation, restart, or claim connected. |
| Context changes during any request | Mask old state immediately and ignore its completion. Preserve no cross-workspace receipt or target list. |

No rollback deletes a successfully registered definition. The catalog always
offers another Use in attempt. Unknown failure details use sanitized localized
copy. Counts, names, and notices never contain secret bindings or raw config.

## Responsive composition

Use one controller above viewport branches. Desktop completion is an inline
saved card in Configured; Use in opens a bounded dialog. Phone completion is
the same single-column card; Use in opens an inset bottom Drawer with a fixed
title, one `min-h-0` scrolling body and safe-area footer. Owner type, search,
task/session drill-down, and consequence review replace content inside this
one surface. The nearest shipped composition is `MobileMenuSheet` in
`components/kanban/mobile-menu-sheet.tsx`; reuse its Drawer containment and
header/footer pattern, not its domain navigation content.

This is a temporary choice, so a drawer fits; actual owner editing uses direct
navigation to its existing full page or session explorer. Close marketplace
review before showing completion. Never nest assignment over setup or another
chooser. Back moves within task-to-session choice, then dismisses. Escape and
close restore the actual trigger. Settings navigation moves focus to the MCP
heading or available highlighted definition without changing selections. Session
navigation retains conversation focus; its hint runs only after the user opens
Customize, within that existing surface's focus management.

Use `useResponsiveBreakpoint`, `controlSizingClassName`, existing tokens and
one icon family. Touch controls have at least 44px active bounds; desktop
ordinary controls are 28px. Long names wrap with context rather than depending
on a tooltip. Use restrained static states and explicit busy/status text.
Any added press scale uses 0.96 for 150ms ease-out under no-preference motion;
reduced motion retains a static cue. Do not add entrance choreography, global
style changes, or new surface tokens for this flow.

## Validation and documentation

The [plan](../../../plans/mcp-assignment-after-registration/plan.md) maps
criteria to transactional tests, context-race tests, and focused desktop/phone
E2E. Backend tests prove conditional writes, exact response ownership, explicit
retry behavior, authorization and no notification on rejection. UI tests prove no
mutation on handoff and no duplicate registration on assignment recovery.

Update `docs/public/agents-and-profiles.md` (reference) and the workspace MCP
how-to section of `docs/public/automation-and-mcp.md` with the implementation.
Explain saved-only, owner consequences, and partial success. During the
design-only turn, planned behavior was not to be published as shipped. No new telemetry,
provider probe, background job, or persistence workflow is required.

## Related decisions

- [Additive workspace MCP](../../../decisions/2026-09-01-workspace-mcp-configuration.md)
- [Settings save coordinator](../../../decisions/0046-settings-route-save-coordinator.md)
- [Session-owned evidence](../../../decisions/2026-07-30-session-owned-mcp-observability.md)
