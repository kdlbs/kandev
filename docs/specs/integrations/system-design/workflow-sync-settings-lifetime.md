---
status: current
system: integrations
created: 2026-10-06
requirements:
  - REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002
owners:
  - kandev
---

# Workflow Sync Settings Lifetime

## Purpose and ownership

This bounded design extends the existing
[workflow-sync owner](../requirements/gitlab-workflow-sync.md#req-integrations-gitlab-workflow-sync-002-settings-lifetime).
Integrations owns the provider-aware settings outcome for both GitHub and GitLab.
The existing requirement is a 12,841-byte combined migrated contract without a
paired design. Extend its normative requirements and keep this new technical
supplement limited to frontend lifetime; migrating provider/backend detail is
outside this change. The integration README's boundary and migration record
remain accurate. No duplicate incident requirement or reusable UI contract is
introduced. The [authorization specification](../../tasks/requirements/workflow-sync-workspace-authz.md)
continues to own authorization, with no changes here.

The [plan](../../../plans/retire-workflow-sync-navigation/plan.md) records delivery
and private evidence references. This local fix needs no ADR: this design
preserves its rationale without imposing a new framework or repository-wide
ownership convention. [ADR 0021](../../../decisions/0021-go-served-spa-with-boot-state.md)
remains the SPA boundary.

## Source and causal boundary

`WorkspaceWorkflowsRoute` in `apps/web/src/settings-routes.workspace-data.tsx`
sets its loaded state to null on workspace changes. It ordinarily unmounts the
old `WorkspaceWorkflowsClient` before the new client mounts. The client owns
`syncDialogOpen`; `WorkflowSyncSection` owns `useWorkflowSync(workspaceId)` and
keys `WorkflowSyncDialog` by workspace. Ordinary navigation therefore does not
establish same-hook overwrite of B's configuration.

The demonstrated defect is narrower: after actual A-hook unmount and B-hook
mount, old A removal and changed forced-sync completion still call
`router.refresh()`. `useRouter` in `apps/web/lib/routing/client-router.ts`
implements refresh as `window.location.reload()`, affecting the current page.
Toasts can also escape the retired hook through the surviving `ToastProvider`.
Accepted evidence renders the real hook, StateProvider, ToastProvider and API
clients, with only fetchJson transport and the navigation refresh boundary
mocked. It does not render the full route or dialog.

Source audit identifies a separate immediate caller boundary: dialog
`handleSave` awaits the controller's boolean then unconditionally calls
`onOpenChange(false)`. `handleRemove` already captures a layout-effect
generation. Both completion paths must be fenced locally; changing a truthful
admitted save result to false would hide this gap and misrepresent the write.
Independent real section/dialog transport tests establish this caller boundary
and preserve current first-save, provider-switch and removal dismissal.

## Requirement mapping

| Criteria under REQ-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002 | Design section |
| --- | --- |
| .1, .2, .3 | Committed owner and admission |
| .4 | Pending controls |
| .5, .6, .8 | Settlement and compatibility |
| .7 | Immediate dialog completion |

## Committed owner and admission

Keep ownership inside each hook instance. Give each workspace render identity
an immutable local owner identity; returning A after B gets a fresh identity.
Do not mutate the current-owner ref during render. A layout effect activates
the committed render owner and a fresh activation epoch. Its cleanup retires
exactly that activation on replacement, unmount, and StrictMode cleanup.
StrictMode replay must produce a fresh callback-bound activation, so requests
and callbacks from the first setup cannot become current again. A retired
activation is never reactivated. Replay schedules a fresh local render owner;
its callbacks become admissible when that owner commits. Independent instances
share no token.

Every callback captures its render owner. At invocation it must match the
active committed owner before setting state or calling transport. Bind the
callback to its producing activation, rather than reading the current epoch at
invocation. Compare that exact activation at each publication and finalization
boundary. A retained first-setup callback is refused after StrictMode replay;
the live replay callback admits current work. A retained retired callback is refused
even if its workspace ID matches a later A. A speculative, uncommitted render
does not retire committed A or authorize speculative B callbacks.

Apply admission to save, delete, force-sync, initial/background GETs, `update`,
`setUrlInput`, and `setProvider`. Current save callbacks retain their existing
captured form/payload behavior; the token is not a draft-edit revision. Reset
helpers stay private to admitted publication. On committed workspace replacement
in a retained hook, clear old config/form/url and initialize that lifetime's
pending controls before passive effects run. Real route unmounts need no shared
store clearing. Admission and retirement must already work during layout effects,
before old passive cleanup can cancel its GETs or interval.

Keep passive cancellation and interval teardown, with the same
`INTEGRATION_STATUS_REFRESH_MS` cadence. An old timer firing before passive
teardown must fail admission. No transport abortion is needed.

## Pending controls

Keep local loading, saving, and syncing ownership separate. Initial GET owns
loading; save owns saving; force sync owns syncing. Delete currently has no
new pending control and must not acquire an invented UI state. Each admitted
operation receives a control ticket; only the newest ticket for that control
within the active owner may clear it. Replacement initializes new tickets and
state. Old success, rejection, and finally paths cannot settle new controls.

These tickets govern pending controls only. Do not introduce a shared sequence
that rejects current configuration results across GET/save/remove/force sync.
No new global ordering or writer revision is part of this design.

## Settlement and compatibility

| Operation | Current publication | Retired settlement |
| --- | --- | --- |
| Initial GET | Config/form reset; initial error toast; loading finalizer | Suppress publication/finalizer |
| Background GET | Config/status only, silent failure; no form reset | Suppress publication |
| Save | Existing payload trim/provider shape; config/reset, success/error toast | Preserve admitted true/false; suppress presentation/finalizer |
| Remove | Clear config/reset, toast, refresh; true/false | Preserve admitted true/false; suppress presentation/refresh |
| Force sync | Config/reset, outcome toast; refresh for changed result; syncing finalizer | Preserve Promise<void> outcome; suppress presentation/refresh/finalizer |

A refused save/delete callback resolves false; refused force sync is a void
no-op. Already dispatched requests continue to the original workspace through
the unchanged API. A resolved force response containing `error` remains an
accepted HTTP outcome whose current toast reports that sync error. No mutation
is re-targeted to the new workspace, undone, or described as cancelled.

Preserve current same-workspace reset-on-save semantics, including edits made
while saving. The accepted same-hook sentinel motivates lifetime checks; it
does not justify adding draft arbitration. Preserve current happy/error cases,
provider payloads, parsing, background status-only refresh, and existing API
return shapes. Use no global cache, transport abort, backend/DB/schema changes,
global event ordering, feature toggle, new copy, or navigation redesign.

## Immediate dialog completion

Keep dialog dismissal ownership independent from server success. Capture an
open-instance/controller owner and activation epoch before dispatch, refuse a
retained closed/retired caller before invoking the controller, and recheck
after await before invoking `onOpenChange(false)` or publishing retry state.
Committed close/reopen, controller workspace replacement, unmount, and
StrictMode cleanup retire old completion. Apply this to save and removal.

Do not reuse removal's `targetKey` generation blindly for save. A successful
save itself updates config and can change `workflowSyncConfirmationTarget`,
especially the first save or a provider change; that must still close the
current dialog. Successful removal's own config/reset must likewise keep its
current dismissal admitted. Preserve target-sensitive confirmation success and
failure guards separately from the open/controller lifetime. Compare the
confirmation generation captured before dispatch with its value immediately
before successful removal publishes its own config/reset. A genuine target
change during the request suppresses delayed dismissal; the removal's own reset
does not. A failed removal compares the generation before publishing retry state.
The controller may accept a private read-only callback at this pre-reset boundary;
the admitted removal's boolean outcome and server mutation remain truthful.
Bind dialog completion to the hook's local owner identity
using the smallest controller-only addition if necessary; no API/outcome shape
change is required. Updating `onOpenChange` within the same dialog must not
allow an old closure to publish through a superseded callback. The current
completion should use the committed callback. This is consumer glue for this
contract, not a generic dialog refactor or a new draft-edit policy.

## Responsive and documentation audit

The same hook/controller/dialog completion is used on desktop and phone.
`WorkflowSyncRemovalActions` retains InlineConfirmActions on desktop and
MobileActionConfirmation with MobileConfirmationHost on phone. All geometry,
breakpoints, touch targets, scroll owners, existing translations, and navigation
choices remain as shipped. Apply mobile-parity's narrow state/data exception:
component transport-boundary tests exercise both existing branches, including
phone failure/retry; no new mobile Playwright suite or ASCII layout is needed.
This guards retirement-triggered reload without changing intended navigation.

Public-doc audit covers `docs/public/workflow-sync.md`, root README and
`docs/screenshots.md`. The guide's configuration, Save, Sync now, removal,
API, and polling instructions remain accurate. No new copy, screenshots,
operator steps or public-guide edit is needed. Reaudit after implementation.

## Causal verification boundary

Use independently authored regressions at fetchJson with real hook/providers/
API clients and actual immediate dialog integration. Mock only transport and
the necessary actual reload boundary. Assert requests, visible config/form/
feedback, pending controls, true caller outcomes and navigation counts; do not
assert an internal ownership predicate. Test actual unmount/remount and retained
same-hook callbacks separately, naming their different evidence limits.

Cover A-B/A-B-A, same-workspace separate instances, independent stores, committed
replacement/unmount before passive cleanup, speculative renders, StrictMode,
closed/reopened dialog, old success/reject/finalizers, newest-current control
ownership and every current happy/error/refresh control. The
[single work order](../../../plans/retire-workflow-sync-navigation/task-01-retire-settings-completions.md)
owns the exact matrix, commands and receipt discipline.
