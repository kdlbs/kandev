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
| AC-EXECUTORS-PROFILE-EDITOR-001.21 through .24 | Connection save catalogue publication |
| AC-EXECUTORS-PROFILE-EDITOR-001.25 through .27 | Profile-card refresh publication |

## Components and navigation

`apps/web/app/settings/executors/[profileId]/page.tsx` remains the single
editor implementation. Its `ProfileEditPage` resolves profile ownership from
the hydrated executor store. Its existing section components determine which
controls apply to the executor type.

`executorProfileSettingsPath` in
`apps/web/lib/settings/executor-settings-routes.ts` takes only `profileId` and
returns `/settings/executors/${encodeURIComponent(profileId)}`. Profile
navigation does not depend on executor type. Connection helpers remain separate.

Hub, settings tree, profile list, task disclosure, discovery, creation and
credential links use this helper; discovery retains its fragments.

## Bookmark compatibility

`LegacyExecutorSettingsRoute` handles both existing executor-scoped route shapes.
For an explicit profile ID, it first finds the named executor and confirms that
the profile belongs to that executor. A valid pair renders `SettingsRedirect`
to the canonical profile route. It never mounts an editable legacy form.

An invalid pair shows the localized unavailable state and recovery link, without
first-profile fallback or global resolution. The wrapper has no editable state
or persistence logic.

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

The canonical editor (`mobile-executor-profile-spacing.spec.ts`) reuses the
curated `components/kanban-with-preview.tsx` direct-navigation pattern for a
primary destination with a long form.

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

`CreateProfilePage` at `/settings/executors/new/:type` dispatches Local,
Worktree, Docker and Sprites to `CreateProfileForm` and `useCreateProfileSave`
in `apps/web/app/settings/executors/new/[type]/page.tsx`. `EXECUTOR_TYPE_MAP`
resolves `exec-local`, `exec-worktree`, `exec-local-docker`, and `exec-sprites`.
The shared `SettingsSaveProvider` owns contributor identity, revisions,
validation, build/secret prerequisites, permissions, dirty/canSave state,
rejection propagation and navigation-blocker bypass.

After `createExecutorProfile` succeeds, acquire the owning store through
`useAppStoreApi`, read its current `executors.items`, and publish synchronously
without an intervening await. Resolve the current owner by the captured ID;
leave an absent owner absent. Reuse `upsertExecutorProfile` in
`profile-edit-page-chrome.tsx` with that current owner, preserving metadata and
siblings, replacing a matching profile ID or appending when absent. Do not use
its missing-owner synthesis. Remove the hook's captured catalogue subscription
as publication input; retain draft behavior and canonical destination.
Creation does not gain the existing editor's baseline/discard policy.
Failure publishes no acknowledgement or successful navigation and retains live
updates, recoverable draft and failed contributor state.

`Service.CreateExecutorProfile` persists and publishes
`events.ExecutorProfileCreated` through `publishExecutorProfileEvent` before
`httpCreateProfile` returns the DTO. The real registered handler can therefore
append the accepted target before HTTP delivery. Upsert replaces that membership
with the accepted response; a second append would duplicate the target and
picker choice. This contract covers a single notification before acknowledgement,
not duplicate events, notification-after-response writer redesign, timestamp
arbitration, generic cache/revision policy, schema or framework changes.

SSH and Remote Docker use separate creation pages with current-store reads;
Kubernetes uses `KubernetesCreatePage` and its current-store resource hook.
Plugin creation does not enter this form. Audit without changing them. Docker
build checks and Sprites secret/network configuration retain their behavior.

Independently authored integration tests mount real page/forms, store/provider,
toast/save coordinator, API adapter, registered profile handlers, router and
`useExecutorProfileOptions` with the production fallback projection from
[Current catalogue publication](#current-catalogue-publication). Hold external
`fetchJson` only; external Monaco renderer/loader capabilities may be stubbed.
Keep unchanged success and rejection controls. Prove different-owner creation
loss, mixed additions/updates/removals, same-owner sibling/metadata preservation,
event-before-response uniqueness, rejection with live updates, and absent-owner
non-resurrection. Assert catalogue plus rendered options, current owner metadata,
eligibility and membership counts, not names alone; zero unhandled errors.
Settle promises/coordinator work, unmount, restore guards/history and drain owned
timers on every exit. Same-owner preservation needs independent causal coverage.

This state-only publication keeps composition, copy, touch, scrolling, routes
and breakpoint behavior. Mobile-parity permits targeted component evidence
without new phone Playwright tests, UI sketches or browser builds. Reassess if
those assumptions change. Existing store ownership/upsert semantics need no ADR.

## Executor policy acknowledgement publication

`ExecutorEditPage` and `ExecutorEditForm` in
`apps/web/app/settings/executor/[id]/page.tsx` own MCP-policy editing.
`SettingsSaveProvider` snapshots the contributor/revision and awaits
`handleSave`. After `executor.update` or `updateExecutorAction` resolves,
`useAppStoreApi` supplies the current catalogue immediately before synchronous
`setExecutors`. Map only the accepted executor ID with the existing
`{ ...item, ...updated }` merge. Keep all other current entries and do not insert
an absent owner. Remove the form's captured-catalogue subscription as publication
input; keep the page's subscribed resolution and `DeleteExecutorSection`.
No intervening await, global singleton, new action, revision or cache is needed.

The REST action PATCHes `/api/v1/executors/:id`; WS requests `executor.update`.
Both backend handlers use `dto.FromExecutor` without attached profiles, unlike
the list path. The current-item merge therefore retains target profiles.
Keep API/spread semantics; same-executor server-write arbitration is excluded.
Payload still uses captured config plus submitted `mcp_policy`, omitting name
for system executors. `updated.config?.mcp_policy ?? ""` is the saved baseline;
the current raw draft is never rewritten. Matching draft/baseline becomes clean;
a newer draft or differing normalized acknowledgement stays dirty. Discard
restores the baseline. Rejection precedes baseline/publication and reaches the
coordinator failure state. No normalization or navigation redesign is added.

Registered executor/profile handlers already write over their current store.
Policy save does not call `ExecutorProfilesCard.refreshProfiles`, which has no
mount-time refresh. Card refresh is a separate writer covered by
[Profile-card refresh publication](#profile-card-refresh-publication).
`DeleteExecutorSection` remains outside policy-save repair. Task-create/subtask
options, fallback metadata and new-session label boundaries remain as described
in [Current catalogue publication](#current-catalogue-publication).

Mount real page/forms, `StateProvider`/`createAppStore`, save coordinator,
action adapter, connection selection, registered handlers, router and options
hook. Control external REST fetch or WS request only, plus external Monaco
renderer when needed while preserving loader exports. Observe catalogue and
rendered task-start options with production fallback metadata/eligibility.
Independently prove different-owner profile create/update/delete, whole-owner
insert/remove, mixed inventory, current saved-owner profiles and absent-owner
non-resurrection. Cover unchanged success, submitted/accepted policy, normalized
response, failure with live updates, in-flight draft edits and separate stores.
Assert held transport and visible live changes before settlement, then catalogue
and options after success. Settle coordinator/promises, unmount, restore
connection/history/guards and drain owned timers on every exit.

State-only publication keeps layout, copy, touch, scroll, routes and breakpoints.
Mobile-parity permits real component/options evidence without browser/build/E2E
replay or sketches. Public executor/MCP instructions remain accurate. This local
correction creates no architecture or operational boundary requiring an ADR.

## Connection save catalogue publication

`useSaveExecutorConnection` serves the SSH connection page at
`app/settings/executors/ssh/[executorId]/page.tsx` and
`RemoteDockerConnectionSection`. `updateExecutor` in `settings-api.ts` returns
`Promise<void>` even though PATCH returns an executor DTO. Keep that adapter
contract. The subsequent `listExecutors` attaches profiles; its successful
result must never replace the shared catalogue from this save hook.

### Membership and field ownership

| Publication input | Owned values |
| --- | --- |
| Current owning AppStore, read at publication | All executor membership/order, every profile and sibling executor, target type/status/provider/system/timestamps |
| Matching refreshed target | Target name and config keys without observed changes during this invocation; server-read normalization includes fingerprint |
| Submitted form/builder, only if refresh fails or target is omitted | Same eligible name/config fields; no normalized-value claim |
| Current target fields observed changing | Current name and individual config-key values/presence, including removals and later restoration |

The backend `Service.UpdateExecutor` persists full submitted config and emits
`executor.updated` before returning. `registerExecutorsHandlers` merges updates
over current items; `registerExecutorProfileHandlers` separately changes nested
profiles. Event delivery need not precede HTTP completion. This hook preserves
observed transitions, without correlating its own save echo or arbitrating
unobserved server writes. Remote Docker's existing builder retains non-SSH
config; submission semantics remain with that builder.

### Invocation-local observation and publication

Before awaiting PATCH, snapshot the current target and subscribe to this
invocation's store. Track name changes and a set of changed config keys by
comparing successive target values, including own-property presence across the
union of keys. Keep touched keys even if a value later returns to its original
value. Profile/metadata-only updates must not mark unchanged connection fields.
Remember target absence: an initially missing, removed, or removed-and-readded
target is ineligible for this invocation's publication. Membership always comes
from the current catalogue. No global revision, handler or store action changes.

After PATCH succeeds, retain the list request. Select only its target ID's
name/config. Refresh rejection or a missing target selects submitted name/config
instead. In one synchronous turn, read current items, map only an eligible
existing target, and publish through existing `setExecutors`. No await separates
read and write. Start config from the current target: for every untouched key in
the union of current and selected config, apply selected presence/value (absence
deletes); retain every touched key from current. Adopt selected name only when
untouched. Preserve all other target fields and profiles verbatim. Config absence
and empty config mean no selected keys; do not infer missing executor membership.

Dispose the observer in `finally` around transport/publication, before awaiting
`onSaved`. Dispose on PATCH failure too; no failure publication or callback.
After successful persistence/publication, await `onSaved` exactly once. Keep its
rejection outside the refresh catch. In-flight work retains its captured store;
unmount does not redirect it into another provider or prevent a successful save.
No retries, timers or subscriptions beyond a save invocation are introduced.

### Callers, consequences and verification

The SSH page's callback still reloads its separate route resource and remounts
the connection card by fingerprint. Remote Docker receives the updated executor
from its subscribed owner. Keep trust gating, save contributor and callback
behavior. Real component regressions must prove normalized pinned values and
save failure through these callers, alongside hook/store/registered-event tests.
Task-create/subtask fallback projection and actual `useExecutorProfileOptions`
remain the consumers described in Current catalogue publication; render their
labels and assert eligibility, membership and current owner metadata.

Desktop and phone share these catalogue outcomes. Unchanged composition, copy,
touch, scrolling and breakpoints permit the mobile-parity state/data exception:
rendered caller/options tests without new Playwright/build work. Reassess if a
viewport interaction changes. Public instructions and screenshots remain accurate.
Existing store ownership and this local rationale need no new ADR.

## Profile-card refresh publication

`ExecutorProfilesCard` in `apps/web/components/settings/executor-profiles-card.tsx`
is mounted by the executor listing (`app/settings/executors/page.tsx`) and the
legacy connection page (`app/settings/executor/[id]/page.tsx`). After DELETE
acknowledgement or built-in/plugin creation callback, `refreshProfiles` reads
GET `/api/v1/executors/:id/profiles` through real `listExecutorProfiles` with
`cache: "no-store"`. Publication belongs only to this card. Keep its subscribed
owner lookup, provider gate, dialogs, interactions and creation navigation.

### Read eligibility and ownership

Acquire the captured owning store through `useAppStoreApi`. At each GET start,
advance a component-local read sequence and capture its value, store and target
ID. An older invocation cannot publish after a newer invocation starts, even
if the newer read fails. Another card's publication is visible through the
store observation below; no global read registry or handler changes are needed.

Before awaiting GET, resolve the current target and subscribe to this store.
Keep invocation-local sticky flags for target absence and target profile
transitions. Compare successive ordered profile IDs and object references;
undefined profiles and an empty array represent the same empty list. Length,
order, membership or a profile object replacement marks the list changed, even
if restored later. Existing immutable store/WS writers make object replacement
observable. Compare entries rather than array or executor identity: the real
`executor.profile.deleted` handler clones every executor/profile array even
when deleting from another executor. Such unrelated clones must not invalidate
this target. Target metadata-only updates also remain eligible. Absence at start
or any later disappearance invalidates the read, including remove/reintroduce.
Observation retains only the preceding target list and two flags, not a change
journal, timestamp arbitration or per-ID tombstone history.

After a successful GET, publish only if its sequence is still current and
neither flag was set. Read `store.getState().executors.items` and synchronously
map only the existing captured target to `{ ...item, profiles: resp.profiles }`.
No await separates read and existing `setExecutors` write. Membership, ordering,
all unrelated entries and target non-profile fields come from current state.
An uncontested response can replace the target list, including with an empty
list. A contested response publishes nothing: keep the entire current target
profile list, rather than partially merging a stale list. This conservative
local fence may defer unseen response-only profiles until a later ordinary
refresh; it adds no retry or server ordering claim.

Dispose observation in `finally` for success, rejection and skipped publication.
Failed GET retains the existing swallowed failure and no publication. Failed
DELETE must not start GET. Keep successful creation navigation after awaiting
refresh, even for rejection or a skipped result; do not change the dialogs'
unawaited `onSaved` contract. In-flight work stays bound to its captured store;
no unmount cancellation, navigation lifetime or cross-provider transfer is added.
No dependencies, API/persistence changes, telemetry or new ADR are needed.

### Consumers and verification

Use independently authored `executor-profiles-card-refresh.test.tsx` with the
real rendered card, creation dialog, `StateProvider`/`createAppStore`/`useStore`,
registered executor/profile WS handlers and actual DELETE/POST/GET adapters.
Hold only external fetch. Render subscribed profile-card rows and downstream
`useExecutorProfileOptions` labels with the real task-create/subtask fallback
projection described above; preserve eligibility gates. Keep router real for
creation navigation. Prove unrelated update/create/delete and mixed inventory,
target metadata, ordinary and failed refresh, failed delete, target absence,
changed/restored target profiles, unrelated clone eligibility, reversed read
completion and newer-read failure. Assert live state and displayed inventory
while GET is pending and after settlement. Clean up held promises, providers,
history/guards and subscriptions on every exit; require no unhandled errors.

Desktop/phone share this state-only outcome. Mobile-parity's explicit exception
permits rendered component/transport/store/options tests without new mobile E2E,
whole-app builds or UI sketches because markup, copy, layout, touch, scrolling,
routes and breakpoint behavior are unchanged. Reassess if implementation changes
those surfaces. Public executor instructions and screenshots remain accurate.

## Implementation plans

- [Unified profile editor](../../../plans/executor-profile-editor-unification/plan.md)
- [Preserve scripts during partial saves](../../../plans/executor-profile-script-preservation/plan.md)
- [Preserve the current catalogue during profile mutations](../../../plans/executor-profile-catalogue-preservation/plan.md)
- [Preserve choices during built-in profile creation](../../../plans/executor-profile-create-catalogue-preservation/plan.md)
- [Preserve choices during executor policy saves](../../../plans/executor-policy-catalogue-preservation/plan.md)
- [Preserve choices during connection refresh](../../../plans/executor-connection-catalogue-preservation/plan.md)
- [Preserve live executors during profile-card refresh](../../../plans/executor-profile-card-refresh-preservation/plan.md)
