---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
---

# Executor profile editor design

## Purpose and boundaries

The existing complete editor at `/settings/executors/:profileId` owns profile
editing. All profile navigation converges on this editor. Executor connection
pages retain their existing routes and behavior.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| AC-EXECUTORS-PROFILE-EDITOR-001.1, .2, .7 | Components and navigation |
| AC-EXECUTORS-PROFILE-EDITOR-001.3, .4 | Bookmark compatibility |
| AC-EXECUTORS-PROFILE-EDITOR-001.5 | State and permissions |
| AC-EXECUTORS-PROFILE-EDITOR-001.6 | Phone composition |
| AC-EXECUTORS-PROFILE-EDITOR-001.8 through .12 | Partial-save script persistence |
| AC-EXECUTORS-PROFILE-EDITOR-001.13 through .15 | Current catalogue publication |
| AC-EXECUTORS-PROFILE-EDITOR-001.16 through .18 | Normal creation catalogue publication |
| AC-EXECUTORS-PROFILE-EDITOR-001.19, .20 | Executor policy acknowledgement publication |

## Components and navigation

`apps/web/app/settings/executors/[profileId]/page.tsx` remains the single
editor implementation. Its `ProfileEditPage` resolves profile ownership from
the hydrated executor store. Its existing section components determine which
controls apply to the executor type.

`executorProfileSettingsPath` in
`apps/web/lib/settings/executor-settings-routes.ts` takes only `profileId` and
returns `/settings/executors/${encodeURIComponent(profileId)}`. Profile
navigation does not depend on executor type. Connection helpers remain separate.

Every production caller uses this helper, including the hub, settings tree,
profile list, task disclosure, settings discovery, creation success navigation,
and task-creation credential links. Discovery appends its existing fragments.

## Bookmark compatibility

`LegacyExecutorSettingsRoute` handles both existing executor-scoped route shapes.
For an explicit profile ID, it first finds the named executor and confirms that
the profile belongs to that executor. A valid pair renders `SettingsRedirect`
to the canonical profile route. It never mounts an editable legacy form.

An invalid pair renders the existing localized unavailable-profile message and
a recovery link. It does not fall back to the first profile or resolve the
profile globally. The reduced page can remain as a small compatibility wrapper
or unavailable-state component, but contains no form state or persistence logic.

`SettingsRedirect` already uses router replacement and preserves query
parameters and fragments through `resolveSettingsRedirect`. Reuse it without
changing generic redirect behavior. Store updates can resolve a temporarily
missing pair. An unavailable state must not cause an eager redirect.

Executor-only URLs retain their current behavior: Kubernetes resolves its first
profile or its connection recovery page. Other executor types retain their
connection editor. An explicit invalid Kubernetes profile must not use the
executor-only fallback.

## State and permissions

The canonical editor retains its current save contributors, serialization,
baseline readiness, deletion handling, and store updates. Navigation unification
requires no migration or backend change. Navigation itself never saves or deletes data.

Kubernetes members retain read-only controls. Docker build controls retain
their administrator checks. The URL ownership check prevents accidental profile
substitution and does not replace backend authorization.

The canonical editor owns the resulting header, recovery, and delete navigation.
The old editor's separate Cancel button and save contributor disappear. The
settings shell continues to own unsaved-change prompts and discard behavior.

## Phone composition

The existing settings index and executor hub provide direct phone navigation.
`mobile-settings-sidebar.spec.ts` confirms that phones do not expose the desktop
settings sidebar. No new menu or drawer is required.

The nearest shipped form is the canonical profile editor, covered by
`mobile-executor-profile-spacing.spec.ts`. The curated direct-navigation
pattern in `components/kanban-with-preview.tsx` supports this choice: the
profile is a primary destination with a long form, not a temporary picker.

The settings content region remains the single page scroll owner. Cards retain
their existing vertical rhythm. The shared floating save control remains the
primary action and retains safe-area clearance. Touch controls retain their
existing phone sizing. Phone tests cover navigation, editing, save, reload,
bookmark recovery, and zero horizontal document overflow.

## Verification boundaries

Route-helper tests cover supported executor callers and encoded profile IDs.
Component tests cover bookmark ownership, missing records, store hydration,
redirect suffixes, and unchanged executor-only routes. Browser tests exercise
the real navigation controls and complete editor together.

Mock Docker build responses follow the existing persistence E2E pattern. This
repair verifies access to the controls, not container runtime execution.

## Partial-save script persistence

Ordinary REST PATCH and WebSocket profile requests in
`internal/task/handlers/executor_profile_handlers.go`, and the `executor_profile`
settings-domain operation in `internal/backendapp/settings_domain_operations.go`,
already carry optional script pointers into `Service.UpdateExecutorProfile` in
`internal/task/service/service_resources.go`. Nil means omission; a non-nil
pointer to an empty string means clear. Existing JSON null decoding remains
nil; this repair adds no null wire contract. The canonical web editor submits
both scripts explicitly, so preserving omissions does not resolve stale full
editor drafts.

The service currently loads a profile snapshot, applies supplied fields, and
calls a full-row repository update. That snapshot must remain available for
existing authorization, Kubernetes validation, Sprites token merging, and other
field behavior. For ordinary built-in saves only, carry the original two script
pointers separately to storage instead of treating snapshot scripts as intent.

Use a small `models.ExecutorProfileScriptIntent` in a focused model file and a
required `ExecutorRepository.UpdateExecutorProfileWithScriptIntent(ctx,
profile, intent) error` method. The method receives the existing prepared model
for all other fields. It refreshes only that model's committed script pair and
timestamp after a successful commit. Required aggregate test doubles implement
the seam explicitly in focused files; unsupported test doubles fail closed.
There is no production fallback to a full-row write and no generic patch API.

### Atomic update and acknowledgement

The SQLite repository package also supports PostgreSQL. The new method uses the
existing writer pool, parameter rebinding, JSON serialization, and UTC timestamp
source. Within a short native transaction, the actual UPDATE writes the existing
non-script assignments and timestamp, and includes a script assignment only
when its pointer is present. There are exactly four script-presence combinations.
Omitted script columns remain untouched by the statement.

Capture `prepare_script`, `cleanup_script`, and `updated_at` with UPDATE
RETURNING into local values. Exhaust and close result rows, check iteration and
close errors, then commit. Publish those values to the caller's model only after
commit succeeds. Defer rollback for unsuccessful paths. No later profile reread
supplies the acknowledgement: another save could have committed by then. This
guarantee covers the script pair and this save's timestamp; unrelated fields
retain existing snapshot behavior and are not promised a global coherent view.

Use the established [SQLite writer transaction boundary](../../../decisions/2026-10-05-sqlite-writer-transaction-admission.md).
Check context before admission and after a returned transaction; drain/join
cancellation and release the connection on every outcome. No instant busy-wait
cancellation guarantee is added. PostgreSQL performs no current-row read before
its atomic UPDATE, so the UPDATE itself acquires its row lock. There is no
read-modify-write critical section requiring an added advisory lock. Actual
physical row-wait tests must prove it retains a holder's committed omitted
script and still applies an explicit script after the wait.

Serialization, query, scan, rows, and commit errors return failure and suppress
the service's success event. Missing rows preserve the ordinary missing-profile
error behavior. This path adds no schema, timestamp algorithm, retry loop, or
new event shape. Commit errors do not authorize a success acknowledgement or an
assumption that an uncertain commit rolled back.

### Compatibility and launch projection

`UpdateExecutorProfileIfUnmodified` remains the full exact timestamp CAS used
when `ExpectedUpdatedAt` is present. Config-mode MCP obtains that version in
`internal/mcp/handlers/config_executor_handlers.go`; it remains guarded for all
updates. Legacy `UpdateExecutorProfile` stays a full replacement. Plugin remote
profiles return through `updatePluginExecutorProfile` before the new seam and
retain their local-script restrictions. Kubernetes and remote Docker admin
checks, Kubernetes configuration validation, global secret-reference admission,
and Sprites token/env merge behavior precede storage as before.

`Executor.applyProfile` in `internal/orchestrator/executor/executor_state.go`
loads stored `PrepareScript` into `SetupScript`, `CleanupScript` into the
cleanup configuration, and nonempty cleanup into lifecycle metadata. Verify
that real projection with the saved row. It does not establish remote execution
or mutate an existing resource. The [public runtime guide](../../../public/executors.md#script-behavior-is-runtime-specific)
continues to own execution timing, including terminal archive/delete cleanup
for SSH and Sprites and no executor cleanup execution for Local or Docker.

### Verification boundary

Permanent tests must reproduce the four disjoint interleavings with independent
real SQLite services, stores, and physical connections, gating only the captured
profile read. Assert stored values, responses, and success-event script pair and
timestamp. Add explicit-clear, both-present, same-script commit order, own-commit
acknowledgement, exact/legacy/plugin/admission, cancellation, rollback, and
missing-row controls. Use fixed persisted timestamps for exact-CAS fixtures.

Registered REST and WS handlers and the actual settings-domain operation need
real database integration evidence. A separate PostgreSQL behavior test must
observe distinct backend PIDs and actual statement row blocking before holder
release; environment skipping is not delivery evidence. Scoped store conformance
and actual native Windows RUN/PASS complete persistence portability coverage.
The repair changes no UI layout, copy, navigation, or touch behavior; backend
integration evidence satisfies this data-only mobile boundary without new
browser tests or frontend builds.

## Current catalogue publication

The normal `ProfileEditForm` branch of `ProfileEditPage` uses
`useProfilePersistence` in `apps/web/app/settings/executors/[profileId]/page.tsx`.
Successful save and remove acknowledgements publish over the current owning
store catalogue. `setExecutors` replaces the whole `executors.items` array,
so publication must retain unrelated changes received during transport.

Obtain the owning store with `useAppStoreApi`. After `updateExecutorProfile`
succeeds, read `appStore.getState().executors.items` immediately before the
synchronous publication. Supply that current array to the existing
`upsertExecutorProfile` in `profile-edit-page-chrome.tsx`. It already replaces
only the acknowledged profile while retaining the matching current executor's
metadata and other profiles. Retain the helper's existing missing-target
fallback; this repair does not define save versus target-deletion ordering.
Do not introduce another await between the current read and publication.
The persistence callbacks no longer need a subscribed catalogue as their
publication input; page ownership resolution remains subscribed as before.

For the same hook's successful remove, read the current catalogue only after
`beforeDelete` and `deleteExecutorProfile` finish. Map that current array and
filter only the target profile from the matching executor. Preserve the existing
navigation-blocker bypass, route, dialog, deleting state and failure handling.
Implement this sibling correction only after independent page-level causal
coverage proves the captured-map loss. Failed transport never reaches either
publication path. Save serialization, payload, contributors, draft baselines,
dirty tracking, success/error notifications and permissions retain their owners.

### Consumers and compatibility

`task-create-dialog-state.ts` subscribes to `executors.items`.
`task-create-dialog-computed.ts` flattens those profiles and fills missing
`executor_type` and `executor_name` from their owning executor, then calls the
actual `useExecutorProfileOptions` in `task-create-dialog-options.tsx`.
`task/new-subtask-dialog.tsx` uses the same catalogue and equivalent fallback
projection before that options hook. Option availability continues to use
current provider/capability gates; preserving a row does not make an ineligible
profile selectable. `task/new-session-dialog.tsx` resolves its executor label
from the catalogue and uses `useTaskExecutorProfile` for profile context; it
does not expose the task-create executor options picker. This repair owns
catalogue publication, not selection defaults or launch logic.

`ProfileEditPage` dispatches `plugin_remote` to `PluginExecutorProfilePage` and
`use-plugin-executor-profile-page.ts`. That path already reads its owning
`appStore.getState()` for load/save/delete. `useKubernetesExecutorResource` in
`hooks/domains/settings/use-kubernetes-settings.ts` also reads the current store
for its connection create/update/remove publications. These are compatible
patterns, not migration targets. Kubernetes profile editing still uses the
normal form's existing combined contributor and administrator gate.

### Verification and phone boundary

Independently authored component integration tests mount the real page,
`StateProvider`/`createAppStore`, `ToastProvider` and `SettingsSaveProvider`.
Use controlled external `fetchJson` transport to hold acknowledgements, real
form editing, the actual save coordinator, and real catalogue subscriptions.
Only the external Monaco renderer may additionally be replaced for its absent
DOM-environment visual capability. Do not replace internal forms, store actions,
contributors, persistence, routing or the actual options hook.

Publish later additions, updates, removals and matching-executor metadata/sibling
changes through the real store while transport is pending; then settle and
assert both the catalogue and options from actual `useExecutorProfileOptions`
with the production fallback projection above. Include unchanged-catalogue
success and rejection/draft/dirty/notification controls. Independently exercise
real delete confirmation, successful navigation and rejection without invoking
private callbacks. Unmount, settle held promises, restore navigation and drain
owned timers on all exits.

This is state/data normalization inside an existing component. Layout, touch,
scroll, navigation structure, copy and viewport-dependent interaction do not
change. The mobile-parity state/data exception permits these targeted component
tests instead of new phone/browser tests or builds. No new persistence, schema,
API, telemetry or arbitration boundary is introduced, so no new ADR is needed.

## Normal creation catalogue publication

`CreateProfilePage` at `/settings/executors/new/:type` dispatches `local`,
`worktree`, `local_docker`, and `sprites` to `CreateProfileForm` and its
`useCreateProfileSave` in `apps/web/app/settings/executors/new/[type]/page.tsx`.
`EXECUTOR_TYPE_MAP` resolves their owners to `exec-local`, `exec-worktree`,
`exec-local-docker`, and `exec-sprites`. Their common contributor uses the
production `SettingsSaveProvider` coordinator. Keep payload construction,
validation, build/secret prerequisites, permissions and contributor identity
with their existing owners.

The creation hook must not map a catalogue captured before
`createExecutorProfile` completes. Because `setExecutors` replaces the array,
that would erase a profile that the registered `executor.profile.created`
handler published for another existing executor while the POST was pending.
This is a client catalogue loss; the service does not delete the independent
profile. Task-create and subtask options consume this same catalogue through
the fallback projection and `useExecutorProfileOptions` described above.

### Acknowledgement publication

Acquire the owning store through `useAppStoreApi`. After the POST succeeds,
read `store.getState().executors.items`, resolve the current executor by the
captured `executorId`, and publish synchronously over that current catalogue.
There must be no intervening await. Reuse the existing
`upsertExecutorProfile` from `profile-edit-page-chrome.tsx`, passing the
current owner rather than a pre-request executor snapshot. It preserves
current metadata and siblings, replaces a matching target ID, and appends
only when that target is absent. Guard an absent current owner: leave the
catalogue unchanged rather than restoring the removed executor.
Do not use the helper's missing-owner synthesis for this route.

Remove the captured catalogue subscription from this hook's publication input
and callback dependencies. Keep all form state and the shared contributor's
revision tracking, `isDirty`, `canSave`, saving/error state, rejection
propagation, navigation blocker bypass, and canonical destination intact.
Creation does not gain the existing editor's draft baseline or discard policy.
On failure, no acknowledgement publication or successful navigation occurs;
live updates received during the pending request remain visible.

### Event-before-response membership

`Service.CreateExecutorProfile` persists the profile and calls
`publishExecutorProfileEvent(..., events.ExecutorProfileCreated, profile)`
before returning to `httpCreateProfile`, which then returns the profile DTO.
WebSocket arrival can therefore precede HTTP arrival. The registered created
handler appends to the current owner. A current-catalogue append in the
creation hook would then introduce a second target membership. The selected
upsert replaces the already published target by ID with the accepted response.
Prove this order with the actual handler and API adapter in the page test,
including an independent live choice in the same pending interval. Assert
target membership and target option counts, not just a name's presence.

This boundary covers a single creation notification delivered before its HTTP
acknowledgement. It introduces no duplicate-event handling, notification-after-
acknowledgement writer redesign, timestamp arbitration, generic cache, global
revision, persistence, schema, API, or framework change. Same-owner sibling
preservation requires independently authored causal page coverage before the
repair can claim it. The earlier normal editor correction remains separate.

### Caller and verification boundaries

SSH and Remote Docker dispatch to their separate create pages, which read
`store.getState()` after their executor/profile requests. Kubernetes dispatches
to `KubernetesCreatePage` and its existing current-store resource hook. Plugin
creation does not enter this normal form. Audit these callers without changing
them. The four normal callers share the repaired hook; Docker build checks and
Sprites secret/network configuration retain their current behavior.

Independently author permanent component integration tests only after the later
implementation release. Mount the real `CreateProfilePage`,
`StateProvider`/`createAppStore`, `ToastProvider`, `SettingsSaveProvider`,
`createExecutorProfile`, registered executor-profile WebSocket handlers, and
actual `useExecutorProfileOptions`. Hold only external `fetchJson` transport;
the external Monaco renderer/loader capability may be stubbed for the test
environment. Do not mock the page, router, store, save contributor, API adapter,
WebSocket handler, options hook, or internal form sections.

First keep strict unchanged-catalogue creation and rejection controls. Then
prove different-owner live creation loss with both current-store and actual-
options soft assertions, zero unhandled errors, and accepted target success.
Independently prove mixed different-owner additions/updates/removals and
same-owner sibling/metadata changes. Include event-before-response uniqueness,
rejection after a live update, and missing-owner non-resurrection controls.
Assert production fallback metadata and eligibility as well as IDs and names.
Settle all held promises and coordinator completions, unmount providers,
restore history/navigation guards and clear owned timers on every exit.

This repair changes only state/data publication inside the existing form.
Composition, copy, touch behavior, scrolling, navigation structure and
viewport-dependent interaction stay unchanged. The mobile-parity state/data
exception permits targeted component integration evidence without new phone
Playwright tests, UI sketches, or browser builds. Reassess that exception if
implementation changes any of those surfaces. Existing store ownership and
upsert semantics supply this correction; no new ADR is required.

## Executor policy acknowledgement publication

`ExecutorEditPage` and its `ExecutorEditForm` in
`apps/web/app/settings/executor/[id]/page.tsx` own executor-wide MCP-policy
editing. The shared `SettingsSaveProvider` snapshots the contributor and
revision, then awaits its `handleSave`. A subscribed `executors.items` captured
by that callback is not a safe publication base after transport: `setExecutors`
replaces the whole array. Registered handlers in
`apps/web/lib/ws/handlers/executor-profiles.ts` already apply created, updated,
and deleted profile events over their current store. Republishing the captured
array loses those independently received choices.

Acquire the form's owning store with `useAppStoreApi`. After the existing
`executor.update` request or `updateExecutorAction` resolves, read that store's
current `executors.items` immediately before synchronous publication. Map only
the matching accepted executor ID with the existing `{ ...item, ...updated }`
merge. Leave all other current entries intact, and do not insert a missing
executor. Remove the form's captured catalogue subscription as publication
input. Keep `ExecutorEditPage`'s subscribed owner resolution and the distinct
`DeleteExecutorSection` intact. Do not add an await between the read and write,
a new store action, global singleton, revision scheme, or cache owner.

### Response and draft boundaries

The REST action calls PATCH `/api/v1/executors/:id` using its existing `fetch`;
the WebSocket branch requests `executor.update`. Both registered backend update
handlers use `dto.FromExecutor`, which includes accepted metadata/config but
does not populate `profiles`. Only the executor-list path attaches profiles.
The current-item merge therefore retains the matching executor's live profile
membership. Keep the existing response contract and spread semantics rather
than synthesizing a replacement profile list or changing an API adapter.
Concurrent server writes to the saved executor's fields remain outside this
client-publication contract.

Keep the existing payload construction: captured executor config plus submitted
`mcp_policy`, with `name` omitted for system executors. The accepted
`updated.config?.mcp_policy ?? ""` remains the saved baseline. The raw current
draft is not rewritten by success. Matching accepted/submitted policy clears
dirty state; a newer draft or a differing normalized response retains the
existing draft-versus-baseline dirty comparison. Discard restores the accepted
baseline through the existing contributor. Transport rejection occurs before
baseline/publication and propagates to the shared coordinator's failure state.
This repair adds no normalization or navigation policy.

### Immediate writers and consumers

`ExecutorProfilesCard.refreshProfiles` awaits `listExecutorProfiles` and maps a
captured catalogue; its create/delete callbacks invoke that separate refresh.
Policy `handleSave` never invokes it, and there is no mount-time refresh.
Neither that writer nor `DeleteExecutorSection.handleDelete` is a causal
dependency of the qualified policy-save loss. They remain outside this package.
An independently proved dependency would require a scope checkpoint before
expanding production edits.

Task-create and subtask subscriptions, fallback `executor_type`/`executor_name`
projection, and actual `useExecutorProfileOptions` are described under
[Current catalogue publication](#current-catalogue-publication). Reuse those
real consumers for evidence; preserve their provider and capability gates.
New-session labels remain a consumer, not a profile-picker repair target.

### Verification and phone boundary

Mount the real `ExecutorEditPage`, `StateProvider`/`createAppStore`, and
`SettingsSaveProvider` with its real coordinator. Control only external REST
`fetch` or the external WebSocket request boundary and the external Monaco
renderer where required; preserve actual loader exports. Keep the page, forms,
store actions, contributor, action adapter, connection selection, registered
profile handlers, router, and actual options hook real. Capture the store from
inside its provider and observe actual task-start options with the production
fallback projection. Verify eligibility and current metadata, not names alone.

Independently author deferred-transport regressions for changed-other-owner
profile creation/update/deletion, whole-owner insertion/removal, and a mixed
catalogue. Also cover current saved-owner profile membership, missing saved
owner non-resurrection, unchanged success, submitted/accepted policy, normalized
response, failure with live updates, in-flight draft edits, and separate live
providers/stores. Controls prove transport is actually held and live changes
are visible before settlement; causal assertions then check both catalogue
and options after success. Settle held promises/coordinator work, unmount,
restore connection/history/guards, and drain owned timers on all exits.

This is state/data publication in an existing component, with unchanged layout,
copy, touch behavior, scrolling, navigation and breakpoint behavior. The
mobile-parity state/data exception permits real rendered component and task-start
view-model evidence without browser/build/E2E runs or UI sketches. No new
architecture or operational boundary warrants an ADR. Public executor and MCP
guides retain the same configuration and user steps; this repair restores their
existing behavior.

## Implementation plans

- [Unified profile editor](../../../plans/executor-profile-editor-unification/plan.md)
- [Preserve scripts during partial saves](../../../plans/executor-profile-script-preservation/plan.md)
- [Preserve the current catalogue during profile mutations](../../../plans/executor-profile-catalogue-preservation/plan.md)
- [Preserve choices during built-in profile creation](../../../plans/executor-profile-create-catalogue-preservation/plan.md)
- [Preserve choices during executor policy saves](../../../plans/executor-policy-catalogue-preservation/plan.md)
