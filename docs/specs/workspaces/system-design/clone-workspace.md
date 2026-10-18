---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-CLONE-001
  - REQ-WORKSPACES-CLONE-002
  - REQ-WORKSPACES-CLONE-003
---

# Workspace cloning system design

## Ownership and existing seams

The workspace creation owner coordinates one new configuration graph. The
integration and secret owners continue to interpret and store their own data.
`task/service.Service.CreateWorkspace` already resolves identity, placement,
owner membership, defaults, and publication; its immediate event publication
and best-effort default initialization make it unsuitable as the first step of
a multi-write clone.

`github.Service.CopyWorkspaceSettingsToWorkspace` copies scope, query blobs,
and action presets. `CopyWorkspaceConnectionToWorkspace` copies workspace
automation authentication, including a destination PAT secret and both App
identifiers. Neither helper is currently atomic across its writes. The latter
is an internal bootstrap helper without public clone authorization. Reuse their
field mapping and tests, not a best-effort sequence of calls after creation.

## Requirement mapping

| Requirement | Design boundary |
| --- | --- |
| REQ-WORKSPACES-CLONE-001 | Configuration mapping and transaction participants |
| REQ-WORKSPACES-CLONE-002 | Admission, credential handling, and atomic persistence |
| REQ-WORKSPACES-CLONE-003 | HTTP contract and responsive form |

## HTTP and application contract

Add `POST /api/v1/workspaces/:id/clone` to
`task/handlers/workspace_handlers.go`. Request: `{ "name": "New workspace" }`.
The route returns the standard access-aware `WorkspaceDTO` with HTTP 201 after
commit. No owner, organization, connection tokens, or selected-row IDs are
accepted from the client. Name suggestions are UI data; the server trims and
validates the supplied name with ordinary workspace naming/reserved-name rules.
Workspace names need not become a new global uniqueness constraint.

Use ordinary workspace not-found/permission handling without revealing source
settings. Invalid name is 400, unsupported source or unusable configuration is
409, and storage failure is a sanitized 500. Configuration errors identify a
safe setting or reference label the user can fix. A disconnected connection
remains disconnected, rather than becoming a different authentication source.

The proposed `Service.CloneWorkspace` performs access/admission checks, builds
the target workspace with ordinary creation policy, and delegates persistence
to a narrow clone participant interface wired in `backendapp/services.go`.
`backendapp` composes the existing domain stores and shared writer pool; no
generic unit-of-work framework, alternate database, MCP tool, or new WS command
is needed. Standard workspace/workflow creation events provide cross-tab updates.

## Configuration mapping

Allocate all target IDs before rewriting any references. Copy only live source
repositories and user-managed visible workflows. Reset destination workspace
identity/timestamps, owner and placement, task sequence, and Office workflow ID.
Retain description, four available defaults, idle settings, and task prefix.
Resolve executor/environment/profile eligibility using existing catalogs; reject
inaccessible/deleted selections instead of inventing replacements.

For repositories, retain source/provider identity, default branch, branch
prefix/template, pull behavior, setup/cleanup/dev script bodies, and copy-file
patterns. Preserve user-managed local source paths. Clear `LocalPath` for
provider-managed repositories so preparation uses the destination workspace's
managed path. Copy branch policies and repository sets with remapped repository,
set, and membership IDs. Copy ordered `RepositoryScript` records with fresh
IDs and remapped repository IDs. Do not copy deleted rows or secret bindings.

For workflows, preserve name, description, prompt, profile/template reference,
sort order, presentation style, and every step behavior field. Make copied
synced workflows manual (`Source`/`SourcePath` reset using ordinary manual-write
conventions); do not copy workflowsync configuration. Do not use presentation
`Style` as an Office eligibility test. Workspace `OfficeWorkflowID` and
`IsImproveKandev()` identify unsupported managed workspaces.

Use `workflow/models.RemapStepEvents`, `RemapWorkflowSessionTarget`, and
`RemapStepID` for their typed references. Validate references before and after
mapping: `RemapStepID` preserves unknown values, so it alone cannot prove clone
independence. Reject references to excluded/nonexistent steps or concrete source
tasks/sessions. Preserve the `this` task sentinel and profile references only
when their existing runtime meaning remains valid. Do not substitute portable
YAML import/export: it resolves profiles and omits some live definition fields.
Validate profiles on workflows, steps, and native review actions against the
same shared-profile eligibility rule.
If no eligible workflow exists, use the built-in Kanban bootstrap mapping once.

GitHub query blobs are workspace-contained values, so saved query IDs and
default markers remain intact inside the copied blobs; they are not database
row identities. Preserve nil versus explicit empty blobs and default fallback.
`ActionPreset` contains no workflow/step IDs. Copy action presets under the new
workspace ID, retaining built-in fallback when the source has no stored row.

## Atomic persistence and domain participants

All these stores use the installation's shared database writer. Begin one
native `sqlx.Tx` on that writer and pass that handle explicitly to each narrow
clone read/write participant. Follow existing SQLite writer admission and
PostgreSQL READ COMMITTED conventions; use the shared SQL binding/dialect helpers.
Do not perform a nested pool checkout while holding the writer transaction.

Extend existing domain storage helpers only as needed to execute on the
supplied transaction. Task storage owns workspace/repository/set/policy rows,
workflow storage owns step serialization, GitHub storage owns connection,
settings and preset rows, and secret storage owns encryption and credential
rows. The coordinator owns begin/commit/rollback, not foreign table SQL or
cryptography. No schema change or persistent clone job is required.

Authorize source management and credential access, resolve caller placement,
and validate default selection eligibility before checking out the writer.
Inside the transaction, lock and read the source workspace; reject changes to
its version, ownership, tenant or placement since admission and recheck managed
workspace markers. Each domain validates its included references before its
copy completes; any rejection rolls back all participants. Shared profile
eligibility uses the separate profile reader, avoiding another writer checkout.
Reads capture persisted configuration observed during the request; this is not
a promise of one global snapshot under unrelated concurrent edits.
Reject an inconsistent included graph. PostgreSQL source row locking must
follow existing lock order and prevent source deletion invalidating a copy
already admitted; do not add a process-local lock as a substitute.

Insert workspace, creator membership, repositories, policies, sets, workflows,
steps, GitHub data, and encrypted destination PAT in the same transaction.
Fresh-workspace default initialization must not overwrite copied query values
or silently supply a connection when the source has none. Optional absent rows
retain their existing read-time defaults. Abort and rollback on any failure.
Publish parent workspace then workflow events only after successful commit;
event send failure does not roll back committed configuration. Return the
access-aware persisted workspace projection.

## Credential boundary and compatibility

Require both workspace management and existing integration credential-copy
authority before reading secrets. PAT material passes only within the secret
owner and is encrypted under a new target key, with fresh row identity. A
transaction-aware secret adapter avoids independently committing `Set` while
holding the writer. Never expose plaintext to the task DTO or logs.

| Connection source | Copy behavior | Proof |
| --- | --- | --- |
| None | No target connection | Missing-row integration test |
| `pat` | Independent encrypted `WorkspacePATSecretKey(target)` and connection | Reveal only in tests; disconnect source and inspect target |
| `gh_cli` | Same explicit host/account selector, fresh connection generation | Routing test, no workspace PAT |
| `github_app_installation` | Preserve registration + installation pair, fresh generation | Two registrations/installation IDs tested without conflation |
| `legacy_shared` | Preserve explicit legacy source under existing resolver rules | Existing legacy routing fixture |
| `github_app_user` / unknown | Reject; personal authentication is excluded | No writes on unsupported source |

Workspace personal user connections, OAuth flows, broker leases/token caches,
webhook deliveries, watch rows, and install-wide App root credentials are not
copied. Shared registration revocation can affect both App-connected workspaces
as before; per-workspace disconnect remains isolated. Other provider repository
definitions can be retained, but their integration connections need normal
setup in the destination. Clone does not claim provider readiness.

## Responsive form and shared state

Use a shared workspace actions menu beside the name and Active badge in each
eligible list card and in the workspace settings page header on every tab.
The ellipsis stays attached to the workspace identity, including wide desktop
cards; it does not occupy the empty space after the resource tiles. The menu fits its translated label within the viewport and
contains a labelled Clone workspace action and has a workspace-specific
accessible name. Use the shared square trigger sizing (28px desktop; at least
44px on phones/coarse pointers). The list trigger sits above the existing
whole-card link. Clone targets the viewed workspace, preserving globally active
selection. Derive eligibility from projected scopes, `isOfficeWorkspace` and
the managed-workspace predicate; the backend remains authoritative.

Reuse the shared Radix menu's inset phone treatment and 44px menu rows. Close
the menu before opening the clone form, recording its persistent ellipsis trigger
as the focus-return target. Keep the page switcher flexible on phones so the
name, Active badge and actions fit together without horizontal overflow.

The clone state/submit hook lives in `hooks/domains/workspace/` and the shared
menu and `workspace-clone-dialog.tsx` in `components/settings/workspaces/`.
List cards share one form owner; the settings shell owns one form across its
responsive presentation. Both entry points clone persisted setup, not unsaved
settings drafts. The existing settings navigation guard still owns leaving a
dirty page after clone success.
Desktop uses the existing Dialog primitive. Phone uses the inset Drawer
geometry exemplified by `components/task/mobile/mobile-picker-sheet.tsx`.
This occasional single-field operation needs a short drawer rather than a
full-screen settings page. Source label and title precede name, setup summary,
exclusions, error, and Cancel/Clone actions. Long translations use one internal
scroll region bounded by dynamic viewport height. Footer clears safe areas;
touch controls meet 44px while fine-pointer desktop retains standard 28px controls.

Keep draft state above the responsive presentation boundary, preserve it on
resize, and return focus on dismissal. Synchronously guard in-flight submission
with a ref; a render-time disabled flag alone permits rapid double submission.
No automatic POST retry. On uncertain response, show localized recovery copy
and refresh the list before an explicit retry. No idempotency claim is made.

Add `cloneWorkspaceAction` in `app/actions/workspaces.ts`, encode source ID, and
use the existing API error handling. Merge the created DTO through
`mapWorkspaceItem` using current store state, deduplicating HTTP and WS arrival.
Navigate with the SPA router to the target overview. Keep `activeId` unchanged.
All new name suggestions, summaries, validation, and feedback use locale keys;
add English and the six complete real locales, deriving Traditional Chinese
through the existing generator. Never translate identifiers or confirmation tokens.

## Failure and verification evidence

Inject a failure at each persistence participant and cancellation before commit;
assert zero target rows/credentials/events and untouched source. Unit tests prove
reference mapping and external-reference rejection. Shared-database integration
tests prove commit/reload independence, authorization and App/PAT routing; exercise
SQLite and available PostgreSQL fixtures. Browser tests prove actual workspace
cloning, copied GitHub query entry/kind switch, error recovery, and phone drawer
geometry. Public docs are updated with implemented behavior in the final work order.

## Related sources

- [Requirements](../requirements/clone-workspace.md).
- [SQLite writer admission](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).
- [GitHub authentication design](../../integrations/system-design/github-authentication-01.md).
- [Repository sets](repository-sets.md).
- [Implementation plan](../../../plans/clone-workspace/plan.md).
