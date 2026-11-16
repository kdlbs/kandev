---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002
  - REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003
---

# Sentry Watch Duplicate and Instance Rebinding System Design

## Purpose and boundaries

Integrations owns the Sentry issue watch row, its import history
(`sentry_issue_watch_tasks`), the PATCH contract, and the settings dialog.
Duplicate is a frontend-only create flow: it reuses the existing create API
and adds no clone endpoint. Instance rebinding changes the PATCH contract,
the store's update transaction, and all three orchestrator history mutations.
[ADR-2026-10-24-sentry-watch-instance-rebinding](../../../decisions/2026-10-24-sentry-watch-instance-rebinding.md)
records why the binding becomes mutable and why history is cleared rather than
namespaced per instance. It amends ADR 0030's immutability rule.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-001` | [Duplicate flow](#duplicate-flow), [Desktop and phone](#desktop-and-phone) |
| `REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-002` | [Instance-scoped field reconciliation](#instance-scoped-field-reconciliation) |
| `REQ-INTEGRATIONS-SENTRY-WATCH-DUPLICATE-REBIND-003` | [Rebinding contract](#rebinding-contract), [Persistence](#persistence), [Stale poll guard](#stale-poll-guard), [Edit dialog](#edit-dialog) |

## Why the binding was immutable

`sentry_issue_watch_tasks` is unique on `(issue_watch_id, issue_short_id)`.
Short IDs such as `WEB-1A` are allocated per Sentry organization and project,
so two regional instances with the same project slugs reuse the same short IDs
for unrelated issues. `CheckIssueWatch` skips every short ID in that set.
Keeping the set across an instance change would silently suppress new-instance
issues. `ResetIssueWatch` also enumerates tasks from this set. The other watch
state is instance-independent: `last_polled_at` only gates cadence
(`isIssueWatchDue`), and the in-flight cap counts tasks by the
`sentry_issue_watch_id` task metadata key, not by history rows.

## Duplicate flow

- `sentry-issue-watch-table.tsx`: `WatchActions` gains an `onDuplicate(watch)`
  callback and an icon button (`IconCopy`) placed after Reset and before
  `WatcherDeleteAction`, with `data-testid="sentry-watch-duplicate-<id>"`,
  `aria-label` and tooltip from the `sentry` catalog. The button stops event
  propagation like its siblings, so it never opens the edit dialog.
- `sentry-issue-watchers-section.tsx`: holds a `duplicating` watch snapshot
  next to `editing`. `openDuplicate` snapshots the row passed from
  `enabledDrafts.items`, including the displayed unsaved Enabled value; it does
  not look up the persisted row or save the source draft. It clears `editing`,
  sets `duplicating`, and opens the dialog; `openCreate` and `openEdit` clear
  `duplicating`. The dialog receives `duplicateOf={duplicating}`.
- `sentry-issue-watch-dialog.tsx`: a new optional `duplicateOf` prop. With
  `watch` null and `duplicateOf` set, the opening is create mode initialized
  from the source without default substitution, including `enabled` and an
  empty stored prompt. Add an explicit duplicate initializer that uses the
  stored `prompt` verbatim; leave `formStateFromWatch`'s existing edit fallback
  unchanged. Only fresh create uses `makeEmptyForm` defaults. Existing readiness
  may disable Create until an empty copied prompt is filled.
  The opening key adds a scalar mode (`create`, `edit`, `duplicate`) plus the
  source watch ID, so duplicate never reuses the edit owner. The title uses
  `sentry:duplicateSentryWatcher`; the footer keeps Create.
- Save calls the existing `onCreate` with `workspaceId` and
  `sentryInstanceId` from the form, so the backend assigns a fresh ID, no
  history, and a null `last_polled_at`. The source row is never written.
- A legacy unbound source yields an empty `sentryInstanceId`. Create mode
  requires an instance. Sole-instance auto-selection must use the same scope-
  change transition as manual selection, including empty->sole-instance for a
  legacy duplicate with carried org/projects. No direct instance-only setForm
  bypass: Save and scoped selectors stay blocked until reconciliation settles.
- Legacy filters remain verbatim. A rejected create uses the existing error
  toast and keeps the draft open; the user corrects it before retrying.

## Instance-scoped field reconciliation

Today the picker's `onChange` clears `orgSlug` and `projectSlugs`, and
`useSentryProjects` lives inside `FilterFields`. The design:

- Lift project loading next to `useWatchOrgs` in the dialog. Both hooks return
  `{ items, status }` with `status` in `idle | loading | ready | error`, keyed
  to `(workspaceId, instanceId)` so a late response for a previous instance is
  ignored. Projects are fetched per instance, not per org. Narrow the returned
  projects by the kept org on the client without refetching on org changes.
  `FilterFields` receives the narrowed project list as a prop.
- Every effective instance change, automatic or manual, keeps carried org and
  projects pending verification. Use one transition for picker selection and
  sole-instance defaults; do not limit pending capture to the picker handler.
- A pure helper `reconcileInstanceScope(form, orgs, projects)` in
  `sentry-issue-watch-form.ts` applies the rule once the needed lists settle:
  - Org list `ready` without the org: clear org and projects; in create or
    duplicate mode, select the sole offered org if there is one, with no
    carried projects. Edit mode leaves the org empty. This transition owns the
    sole-org default; it must not depend on the old auto-select effect firing
    again after reconciliation.
  - Org list `error`: clear org and projects; resolved.
  - Org present, project list `ready`: keep only project slugs offered under
    that org; resolved.
  - Org present, project list `error`: clear projects; resolved.
- Disable both org and project selectors during pending instance-scope
  verification. Carried values may remain as read-only display, never as
  interactive synthetic options. Only reconciliation against ready new-key
  lists, or clearing on their failure, resolves pending. Do not resolve it
  merely because a change handler fires.
- `isWatchFormReady` gains `scopePending`; Save is disabled while pending.
  Disabled selectors cannot change the draft through keyboard or pointer
  input. On completion, render only verified options for the changed scope.
- `orgSelectItems` and `projectMultiSelectItems` keep injecting saved values
  only on unchanged-instance openings. Do not use their synthetic carried
  options as evidence of membership during reconciliation.
- Every other form field is untouched by an instance change.

## Rebinding contract

- `UpdateIssueWatchRequest` gains `SentryInstanceID *string`
  (`json:"sentryInstanceId,omitempty"`). Absent means unchanged.
- `UpdateIssueWatch` resolves the request before any write:
  - Present and blank after trimming: `ErrInstanceRequired` (400,
    `SENTRY_INSTANCE_REQUIRED`).
  - Present: `requireInstance(ctx, w.WorkspaceID, id)`; a foreign or missing
    instance returns `ErrInstanceNotFound` (404,
    `SENTRY_INSTANCE_NOT_FOUND`). The handler already maps both.
  - Equal to the stored binding: no rebind.
  - Stored binding empty (legacy unbound): always rebind. Current resolution
    cannot establish which instances contributed the existing history.
  - Otherwise: rebind.
- The frontend type `UpdateSentryIssueWatchRequest` gains optional
  `sentryInstanceId`. The dialog sends it only when the form instance is
  non-empty and differs from the opening's saved instance. Updates that keep
  the instance continue to omit it, matching the lifetime tests.
- Add optional `UpdateIssueWatchRequest.ExpectedScopeRevision *int64`
  (`json:"expectedScopeRevision,omitempty"`) and the corresponding frontend
  request field. It is an equality precondition, never an assignment to
  `scope_revision`. After authorized resource lookup, compare it with the row
  before applying the patch, then retain it for the store's write-boundary CAS.
  Mismatch returns `ErrIssueWatchConflict` with no effects, even if the change
  preceded this request's server-side read or the binding went A->B->A.

## Persistence

Add persisted `scope_revision` (non-null integer, default 0) to each watch.
Expose `IssueWatch.ScopeRevision` as read-only `scopeRevision` in watch
responses, never a writable request field. Include it in row materialization and
`NewSentryIssueEvent`. A successful rebind or Reset finalization increments it;
normal poll/error stamps and non-rebind edits do not. The additive migration
runs after the legacy instance-table rebuild in `Store.initSchema`; fresh
schema and replay include the same column. Existing rows start at 0.

`Service.UpdateIssueWatch` uses a supplied `expectedScopeRevision` for its CAS;
when omitted by an older caller, it captures the authoritative pre-read's
revision instead. Pass that expected revision plus the rebind decision to
`Store.UpdateIssueWatch`; never replace a supplied precondition with the
current pre-read value to make a stale opening pass.
Every update uses `WHERE id = ? AND scope_revision = ?`; rebind atomically
increments the revision, changes the binding, clears history and poll/error
state. A zero-row CAS returns new `ErrIssueWatchConflict`, mapped to HTTP 409
`SENTRY_WATCH_CONFLICT`, without deleting anything. A->B->A therefore cannot
make an old pre-read current again. No automatic replay of a stale patch.
The revision is immutable state evidence; `expectedScopeRevision` is a client
comparison against that evidence, not a writable watch revision.

Mutation-response publication in `useSentryIssueWatches` must not regress a
row to an older scope: replace a matching row only when the response's
`scopeRevision` is at least the displayed row's revision. Apply that rule to
list refresh merges as well; workspace/opening ownership guards still apply.
Do not infer freshness from response arrival time or caller request order.
Return the committed revision with each response, not a stale pre-update
snapshot. Reset refresh must publish its new revision before older PATCH
responses can overwrite that scope. Missing evidence must trigger an
authoritative refresh rather than an invented revision. Response guards protect
future openings, not an already-open draft: the separate immutable opening
precondition below protects that path. Equal-revision non-scope edit ordering
remains existing behavior. A new edit/duplicate snapshots the guarded row.

`httpDeleteIssueWatch` must route admission errors through
`writeIssueWatchError`, replacing its hardcoded uncoded 500 path. PATCH,
Reset, and Delete all map `ErrIssueWatchConflict` to coded 409. Exercise DELETE
and Reset while real Dispatch or Reset admission is held, before any effects.

All transactions that mutate history acquire the parent watch row before any
child row, including reserve, assign, release, rebind, `ResetIssueWatchState`,
and `DeleteIssueWatch`. Use the cross-dialect parent write-lock statement
`UPDATE sentry_issue_watches SET updated_at = updated_at WHERE id = ?`.
SQLite acquires writer admission; Postgres locks the parent row. Never retain
a database transaction across task creation, task deletion, or agent startup.
Always clear history on explicit legacy binding. Existing tasks and their
`sentry_issue_instance_id` metadata remain unchanged.

## Operation admission and recovery

The runtime supports one backend replica, including with Postgres (see
[Operations](../../../public/operations.md#postgresql)). Use one coordinator
owned by the existing Sentry `Service`, shared by dispatch, rebind, Reset, and
watch deletion. It tracks per-watch active dispatch count and exclusive
operation ownership under a short mutex; it does not hold `Service.mu`, a
database connection, or a mutex across I/O. Release handles are idempotent,
deferred on all paths, and idle entries are removed.

- Dispatch uses shared admission: reject if exclusive ownership exists,
  otherwise increment the active count. After admission, read the current
  row from the writer handle and validate the event's instance and revision.
  On mismatch or read failure release and skip. Multiple admitted dispatches
  can run concurrently, preserving the existing in-flight cap.
- Rebind, Reset, and deletion use exclusive try-admission. If any dispatch or
  exclusive operation is live, return `ErrIssueWatchConflict` before writes
  or task deletion. A failed dispatch admission skips the event; the next
  ordinary poll can rediscover it. Do not wait while holding DB locks.
- Acquire the dispatch handle at the beginning of
  `WatcherDispatchCoordinator.Dispatch`, before preflight/self-heal or Reserve,
  and defer release until the entire synchronous Dispatch call exits
  (including attach/release and auto-start). `dispatchWatcherEvent` spawns a
  goroutine, so a handle scoped only to the bus subscriber is insufficient.
  Add an optional `WatcherDispatchGuard.BeginDispatch` source interface;
  `SentryWatcherSource` implements it through `SentryService` and its adapter.
  Other sources keep their existing pipeline. Sentry with no wired service
  fails closed rather than bypassing ownership.
- A rebind takes exclusive admission after request validation, before its
  store CAS. An intervening completed rebind/Reset is detected by the captured
  revision. Non-rebind edits use the same store CAS but do not need exclusive
  admission, since they do not clear history.
- Reset takes exclusive admission before calling `watchreset.Run` and retains
  it through the complete task-deletion loop and finalization, using the same
  cancellation-detachment semantics as that helper. Its Sentry resetter
  captures the current revision and task-ID snapshot under admission. Clear
  compares the captured revision before clearing and incrementing it. A CAS
  failure changes no history and is reported, never retried over another scope.
  No new dispatch or rebind can interleave while the task deleter is paused.
  Reset task-deletion failure semantics remain those of `watchreset.Run`.
  Pass a Sentry-only `watchreset.TaskDeleter` adapter to that helper. The
  adapter always forwards `DeleteTaskTree(ctx, recordedID, false)` to the
  configured handoff service, ignoring the helper's cascade=true argument.
  Each enumerated history ID is an independent deletion target. Replace
  Handoff's separate normalize-then-bulk-reparent sequence with an observed-
  child transaction, proposed repository operation
  `DetachObservedChildrenForDelete(ctx, rootID, workspaceID, observedChildren)`.
  Under the existing repository hierarchy admission (`hierarchyTxOptions` and
  `lockTaskHierarchy`), re-read the authorized root and complete structural
  child set from the transaction's writer view, including archived, ephemeral,
  and automation-origin rows. Match the exact observed ID set and workspace/
  parent/mode/orphan guards before changing anything. Added, removed, reparented,
  or scope-drifted children return the existing hierarchy/metadata conflict;
  they are never silently included in a bulk detach or ignored.
  Lock rows in deterministic order and normalize/detach only validated IDs in
  that same transaction. For `inherit_parent` children, set shared-group mode
  and clear obsolete orphan markers as the existing normalization does, while
  preserving archive state and unrelated current metadata. Do not apply the
  active-only orphan-stamping requirement to deletion normalization. Any
  failed comparison/update rolls back all normalization and detachment. Reuse
  the existing cross-dialect workspace CAS rules through a transaction-bound
  helper, not a service callback that opens another writer connection.
  Handoff retains the preimages for conditional compensation and publishes
  child updates only after the transaction commits. Cut over the non-cascade
  handoff path with no fallback to the unsafe bulk method; remove its obsolete
  split normalization/reparent helpers. Other independent reparent callers
  retain their existing APIs.
  No hierarchy lock/DB transaction spans lifecycle I/O. Keep the final
  root-delete child check: a child attached after detach commits must stop
  root deletion, not be bulk-detached later or strand an invalid workspace.
  Conflict/compensation follows existing Reset best-effort failure semantics;
  a racing child may safely leave the recorded root undeleted, but no surviving
  child may be detached with `workspace.mode=inherit_parent`.
  Non-recorded children otherwise survive with valid normalized ancestry;
  recorded children are independently deleted in either enumeration order,
  including when those children are archived.
  Leave `watchreset.Run` and all other integrations' cascade behavior unchanged.
  Preview count and TasksDeleted now consistently count recorded tasks, not
  arbitrary descendant trees. Update Sentry preview/reset comments accordingly.
- Deletion takes exclusive admission before its parent-first store transaction.
  It removes watch/history, never tasks. Missing watch remains idempotent.
- The active-count handle, not an empty SQL reservation or its age, proves
  live dispatch ownership. After a failed attach or process exit, abandoned
  empty rows may remain but no live Dispatch owns them. Once exclusive
  admission succeeds, Reset can clear them and rebind can discard them safely.
  Pending-but-not-admitted events must obtain admission and revalidate their
  revision before side effects; Reset finalization invalidates those events.
  Restart does not resume old in-process watcher dispatch goroutines.

All production destructive Sentry callers must use the service's admission
path, including HTTP and backend settings operations. Store helpers are the
transaction layer, not an alternative live-operation admission API. Tests
exercise the real service plus coordinator seam, not only raw store calls.

## Stale poll guard

Reserve, assign, and release carry both `instanceID` and `scopeRevision` from
the event through `SentryWatcherSource`, `SentryService`, and
`sentryServiceAdapter`. Each parent-first transaction requires a current watch
with matching revision and either NULL binding or matching instance. Reserve
returns false on mismatch; release is a no-op; assign returns the existing
missing-reservation error. Matching instance alone is insufficient after
A->B->A or Reset. The coordinator's dispatch handle prevents a rebind/Reset
between a successful reserve and task creation; SQL predicates protect delayed
callbacks and raw-store misuse from changing a new scope's rows.

GitLab provides the guarded INSERT/UPDATE/DELETE pattern. Sentry adds a
revision for scope identity and process-owned admission for effects that cannot
be rolled back with the history transaction. Do not add per-instance history
or hold a DB lock across those effects.

## Edit dialog

- `InstancePicker` is enabled in edit mode. `sentry:sentryInstanceHelp`
  becomes "Which Sentry instance this watcher polls." in all seven locales.
- When `watch` is set and the form instance differs from
  `watch.sentryInstanceId`, a warning `Alert` from `@kandev/ui` renders
  directly under the picker with `sentry:instanceChangeWarning`. Returning to
  the saved instance hides it.
  Explicit legacy binding shows the same warning because it always clears.
- Save captures the original watch, workspace, and the outgoing instance with
  the rest of the payload, preserving the lifetime contract of
  `REQ-INTEGRATIONS-SENTRY-WATCH-DIALOG-001`.
- Capture `watch.scopeRevision` once as an immutable opening precondition.
  Every edit PATCH, including unchanged-instance and legacy edits, sends it as
  `expectedScopeRevision`; revision 0 is valid and must not be dropped by a
  truthiness check. A missing response revision blocks edit submission until
  authoritative data is available, never substitutes 0. Do not reset the
  opening or refresh its expected revision from passive list hydration.
  After a late accepted rebind updates the list, an already-open old draft
  retains its text but its Save conflicts rather than writing into the new
  instance. The existing error toast reports 409 and the draft stays open;
  close/reopen to intentionally take the new baseline. Create/Duplicate do not
  send an update precondition. The lifetime action still completes normally;
  response publication remains governed by the revision guard above.

## Desktop and phone

The Duplicate button lives in `WatchActions`, shared by the desktop table and
phone card. Use 28 px targets on fine-pointer desktop and at least 44 px on
phone or coarse pointers. The dialog and warning share a scrollable composition.
Phone E2E must exercise changed-instance edit, warning, Update, and Reset,
not only unchanged-edit lifetime behavior or Duplicate/Create.

## Failure and recovery

- Instance list or browse failures during reconciliation clear the
  unverifiable selection instead of saving stale slugs.
- A failed PATCH leaves the row, history, and tasks untouched, and the dialog
  keeps its draft. A live dispatch/exclusive operation or stale scope revision
  returns 409. Retry after the live operation exits; reopen an edit to refresh
  a stale opening. There is no time-based ownership expiry.
- Reset admission fails before any deletion if dispatch or rebind owns the
  watcher. A Reset that owns the operation blocks rebind and new dispatch
  until finalization, including when the caller disconnects. An interrupted
  backend cannot leave a live process-local ownership token; remaining history
  follows the existing partial-Reset retry semantics.
- A stale in-flight poll may still run its deferred `last_polled_at` stamp or
  error stamp after the rebind. That delays the first new-instance poll by at
  most one interval, or shows the old error until the next poll clears it.
  It cannot reserve, create, or modify history after a successful rebind.
  "Check now" polls immediately.

## Compatibility

- Older PATCH callers that omit `expectedScopeRevision` retain server-pre-read
  CAS semantics; they do not get opening-age protection. The updated edit UI
  always sends the opening precondition, even when it omits `sentryInstanceId`.
  Other integrations and the agent settings catalog stay unchanged; settings
  rebinding and a generic cross-integration ETag protocol are out of scope.
- Reset after a rebind deletes only tasks recorded after the rebind; earlier
  tasks keep their watch metadata and remain in the in-flight count.
  `ResetWatchDialog` gains an optional localized `scopeDescription` rendered
  independently of preview status and an optional `previewFailureDescription`
  replacing the generic "delete every task" fallback. Sentry supplies both
  from new `sentry` catalog keys; other integrations keep their current copy.
  The scope note states that only current history is reset and tasks from
  before an instance change remain. Correct the backend Reset comment too.
- Agent/MCP settings rebinding is out of scope. The settings catalog's
  `issueWatchFields` deliberately stays unchanged, so `sentryInstanceId`
  remains an unknown setting there. `backendapp.updateSentryIssueWatch` still
  decodes the shared request type for allowed fields; it is not a second
  rebinding entry point. HTTP PATCH and the settings UI expose rebinding.
- Switching back to a previous instance re-imports its current matches; the
  ADR records this accepted cost.

## Observability

No new metrics. Rebinds log at info level with watch ID and old/new instance
IDs; rejected stale reservations log at debug level in the store caller.

## Test strategy

Backend: migration/replay tests and service/store tests cover atomic rebind,
revision CAS including A->B->A, all history guards, live versus abandoned
dispatch, and Reset/deletion lock ordering. Hold CreateIssueTask after Reserve:
Reset, deletion, and rebind must return 409 without effects. Hold the real
Sentry resetter's task deleter: rebind and dispatch must not enter; after
Reset completes, rebind succeeds. Reverse the order and prove a Reset after
rebind deletes only current-scope tasks. Cover attach failure followed by safe
abandoned-row cleanup and backend-restart recovery.

Use the real Sentry Reset plus handoff service for a hierarchy regression:
a current-history root has a prior-instance imported child, an unrelated
archived child, and a current-history child. Only enumerated IDs disappear;
all others survive with valid top-level ancestry, regardless of iteration
order. A fake recording cascade=false is insufficient proof.

Also pause real handoff/Reset after structural enumeration but before guarded
detachment, commit a new non-recorded inheriting child through the real task
repository, then resume. Require conflict before any observed-child mutation,
or an equivalent safe outcome with every detached child normalized. In
particular, never permit `parent_id=''` with `workspace.mode=inherit_parent`.
Cover metadata/mode drift and a child arriving after detach but before final
root deletion; retain the root check and conditional compensation. Use
independent database connections and barriers, including PostgreSQL, not
only an in-memory fake or timing sleeps.

Stale-edit tests cover a rebind accepted before the old opening's request is
even read by the server, with unchanged-instance payload, A->B->A, and Reset.
In a real hook/section/dialog regression, hold the accepted A->B response,
cancel/reopen the still-displayed A row and edit prompt, then publish B and Save
the old form. Its captured expected revision remains A's; 409 retains the
draft and cannot copy A's filter into B. Same-scope hydration stays compatible.

Use file-backed shared SQLite and an isolated Postgres schema with multiple
physical connections for lock/interleaving proof. The existing single-
connection fixtures cannot prove lock ordering. Require a non-empty
`KANDEV_TEST_POSTGRES_DSN` and successful Postgres execution, never a skipped
test. Work orders name exact new tests and commands.

Frontend behavior tests cover blocked synthetic choices while deferred/failing
lookups run, lossless empty-prompt duplication, sole-org handoff, failed
create/PATCH draft retention, and truthful Reset scope. Desktop and phone
Playwright scenarios complete duplicate, changed-instance edit, and Reset.
The per-criterion mapping lives in the plan.

## Related decisions

- [ADR 0030](../../../decisions/0030-workspace-scoped-integration-settings.md)
- [ADR-2026-10-24-sentry-watch-instance-rebinding](../../../decisions/2026-10-24-sentry-watch-instance-rebinding.md)
