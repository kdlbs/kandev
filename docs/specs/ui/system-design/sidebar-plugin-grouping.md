---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-PLUGIN-GROUPING-001
---

# Complete Plugin Grouping for Sidebar Views

## Ownership and evidence

UI owns the personal view and presentation contract. Tasks retains authorization,
filtering, ordering and bounded pagination. Plugins retains classification data
and supervised action dispatch. Tags remains in its dedicated repository,
https://github.com/yattdev/kandev-plugin-tags, not the host monorepo.

At the reconciled main baseline `b4c6a197129b5b4a5e99c3e1dab132ada9fb433f`,
`TasksViewPicker` listed saved views and `GroupPicker` had built-in choices only.
The implemented extension adds backend-only provider metadata, normal generated
saved views, and shared desktop/phone controls. It does not depend on a browser
bundle registration. `groupSidebarTaskPage` still reconstructs the authoritative
bounded server page. The separate `TaskListFacetRegistration` contract remains
synchronous and page-local; `useTaskListFacets` does not discover unloaded tasks.
The originating task's Tags 0.16.0 evidence is historical, not this implementation's
acceptance proof.

The user selected complete saved-sidebar semantics. Reusing page-local facets
unchanged or fetching every task into the browser would violate that decision
and the existing pagination contract.

## Requirement mapping

| Criteria     | Owning design boundary                      |
| ------------ | ------------------------------------------- |
| 001.1, 001.2 | Saved identity and selectors                |
| 001.3        | Verified snapshot and repository projection |
| 001.4, 001.5 | Lifecycle, freshness and failure            |
| 001.6        | Shared desktop/phone composition            |

All criterion suffixes refer to AC-UI-SIDEBAR-PLUGIN-GROUPING-001.

## Verified snapshot contract

Add optional manifest `sidebar_groupings` entries with URL-safe `id`, canonical
`label`, optional localization key, and a declared workspace-scoped `action`
key. Validate duplicate IDs, referenced action existence/scope, field bounds and
cross-entry conflicts at installation. Include eligible descriptors in active
plugin boot/HTTP metadata. Existing manifests default to no entries.

Declarations require the unreleased `min_kandev_version` floor `0.103.0`;
Stable 0.102.0 lacks the contract. Coordinate package publication with the
supporting Host release; matching development builds remain usable. The authenticated
`GET /api/plugins/sidebar-groupings` endpoint returns `{groupings: [...]}`;
`sidebarGroupings` in the boot payload seeds the same independent catalog.
Declaration bounds are 16 providers per plugin, 128-byte plugin/grouping IDs
and 256-byte labels/localization keys. Snapshots accept at most 1,000 values and
10,000 raw membership pairs before deduplication.

Resolve a grouping through the existing supervised plugin action transport,
not a new plugin server or raw HTTP loopback call. Introduce typed SDK snapshot
DTOs and a reusable decoder; the existing action transport remains unchanged.
The read-only action receives verified user/workspace identity and returns:

- `version: 1`, `revision`: nonempty source-content revision;
- `complete: true` explicitly asserting complete workspace classification;
- `groups`: unique opaque value IDs, labels and optional validated hex colors;
- `memberships`: task-ID/value-ID pairs, with duplicate pairs normalized once.

Decode canonical wire fields once in a bounded pass before normalization.
Reject duplicate recognized fields and case-folded aliases at the snapshot,
group and membership levels; a later empty array must never hide earlier raw
memberships or an exceeded bound. Skip unrelated extension metadata without
letting it alter classification.

Task IDs with no membership have no value. Unknown group references, malformed
colors/identities, duplicate conflicting definitions, incomplete results or
oversized responses fail closed. Retain the existing plugin action response
policy of 1 MiB, with at most 1,000 group values and 10,000 membership pairs;
whichever bound is reached first fails explicitly without truncation. These are
provider-input limits, not a license to fetch 10,000 task records. Resolve before beginning the SQLite read
transaction; do not hold a database lock across plugin execution. Use the
request cancellation and a five-second action deadline; late results must not
mutate state or enter a successor query cache.

Only the backend resolves snapshots. Query input names a grouping reference,
not memberships or arbitrary action names. The reference is
`plugin:<pluginId>:<groupingId>`, parsed using the two manifest identities, with
no title/value embedded. Backend dispatch validates active installation,
manifest declaration, action authorization and workspace read access. Plugin
classification is presentation data, never authorization evidence.

## Saved identity and selectors

Extend backend/TS view-group validation and wire types with the canonical
plugin reference, preserving all legacy built-in values. Validate identity
syntax independently of live registry availability so a disabled provider does
not destroy stored settings. Reuse backend-owned workspace view CAS/journals
and boot/HTTP/WS projection; no localStorage or second preference store.

Extend `TasksViewPicker` with a Plugin views section sourced from eligible boot
metadata/registry lifecycle. Selecting Tag sends `select_plugin_grouping` through
the existing workspace journal. Inside the settings CAS, reuse the latest
unchanged generated matching view or create the candidate with a unique name
and the 50-view guard. The browser waits for the authoritative selected ID;
two tabs cannot persist duplicate unchanged generated views. Do not overwrite
renamed/custom views or silently discard a draft. Once created it remains in
the ordinary saved view list and supports normal edit/duplicate/delete.
`GroupPicker` also offers eligible plugin groupings for editing custom views.
Keep a missing saved provider visible as unavailable rather than deleting it.

Ordinary edits carry captured sparse field/disclosure intent into the CAS, not
whole view after-images. `views_update` never recreates absent IDs and applies
disclosure only to its declared grouping owner. `views_create` is insert-only;
a replayed creation cannot replace an intervening rename, and later edits stay
independent. `draft_update` uses its captured base-view/group owner, including
discard, rather than adopting a newer unrelated draft's owner. Explicit full
replacement/upsert remains separate from normal edits.
Failed-response replay preserves touched final assignments rather than cancelling
an inverse edit against an uncertain preimage. Reorder journals the moved ID,
destination and original before/after placement, applying only that move to the
latest admitted collection. Disjoint acknowledged remote moves remain intact;
missing endpoints and collection changes after dispatch report a conflict.

No Tags-specific constant is added to the host. Plugin group captions use their
localized metadata at render time; saved names follow normal user view-name
semantics. Collapsed value identity is `plugin-group:` plus SHA-256 of the
complete provider reference, a NUL delimiter (forbidden in references), and the
opaque value ID. Labels are excluded, so rename/recolor preserves disclosure.
Each complete snapshot has at most 1,000 compact value keys plus Unassigned.
The host owns the missing-value group key and its localized Unassigned label.
Provider labels are data and never act as host sentinels. For disclosure reads,
map the legacy `<reference>:untagged` key to `<reference>:unassigned` so an
existing collapsed preference remains collapsed. Normal reads carry the exact
client disclosure inline. A view with accumulated
retired keys can recover using `disclosure_view_id` instead of transmitting its
oversized array. The authenticated settings reader resolves that personal view
only within the requested workspace, with authorization checked before lookup
and renewed before reporting a missing view. A different-group draft starts
expanded and owns optional `collapsed_groups`; heading toggles update that draft,
not its saved base. Recovery resolves the matching draft's disclosure as well.
The service intersects disclosure with the complete verified provider snapshot,
not the current page, and returns `grouping_keys`.

The client waits for its captured workspace journal before that recovery read.
It admits only the complete keys, not that read's task page: current
store/workspace/provider/view/expected-disclosure fences guard reconciliation
through the ordinary settings journal.
The fence includes saved-versus-different-group-draft ownership. Draft-owned
pruning sets `require_existing` inside CAS and never creates a replacement draft
after another tab has transferred that owner into its saved base.
Query identity includes that concrete disclosure owner, so a remote Save starts
an independent saved-owner read even when the effective group and keys match.
Compaction publishes its bounded keys only after persistence succeeds. A rejected write keeps saved intent and exposes
manual Retry, without rollback-driven automatic writes or probe-page admission.
The bounded successor read again carries exact client keys inline. Valid surviving
keys remain collapsed; retired and duplicate keys are removed. Neither the
256 KiB query limit nor the 2 MiB settings request limit is raised.
Duplicate-only compaction carries an explicit canonicalization intent, so it
deduplicates current committed keys without replacing concurrently added keys.
Save and Save as use a base/group-owned `save_draft` CAS operation to transfer
the latest matching draft, including disclosure accepted from another tab.
Copies retain independent arrays; changed owners conflict rather than replaying
stale view images. Selection and Save expose a workspace-local busy state that
blocks dependent view edits until authoritative settlement without blocking
task actions. Discard leaves its base unchanged. A same-group draft inherits saved disclosure.
Save and Save as are unavailable until preceding workspace preference writes
settle. Rejection of a prerequisite cannot silently acknowledge a transfer of
older fields or disclosure.
An open Save as name editor captures its base/group identity and dismisses on
owner replacement. Store admission checks that identity again; a same-owner
change still transfers the latest committed fields and disclosure.
Selection does not blindly retry a transport-uncertain mutation, because a
previously reused generated view can be renamed or superseded between sends.
Explicit reorders carry their observed order as `view_order_base`. The settings
CAS rejects a changed order without committing the stale permutation; retired
move endpoints cannot abort independent queued edits.
Duplicate clones source disclosure so expanding a copy never expands its source.

## Repository projection and pagination

Inject a narrow grouping resolver into task service orchestration. The
repository accepts an internal verified immutable snapshot separately from the
untrusted `SidebarTaskViewQuery`. Use its existing authorized workspace task
relation, filters, effective-tree ordering and preference projection. Join only
matching accessible workspace tasks; unknown, deleted or foreign membership IDs
never cause reads or rows. Apply grouping before page slicing in the same read
snapshot, using parameterized temporary relations/JSON, not interpolated SQL.

Each root inherits its own declared values; descendants remain with that root
as in existing built-in grouping. A root in multiple groups has a separate
display occurrence per group; never mutate parentage or attach a child to an
unrelated root. Deduplicate repeated values for one task. Group order is stable
Unicode-lowercased label order with value-ID tie-break and Unassigned last; existing
sort/pin/manual-child precedence orders tasks inside each group. Collapse
visibility is applied before the 100-occurrence page window. Group continuation
and off-page parent context retain their existing meanings.

`total_tasks` remains unique matched tasks. `total_visible_tasks` and page bounds
count visible task occurrences, explicitly labeled in pagination copy when
multi-group duplication exists. Page/entry DTOs carry optional group color,
source revision and provider generation across Go/TS/HTTP and boot projection.
React keys and virtual row IDs use provider/group/task occurrence identity;
selection styling and task actions continue to use canonical task IDs. Shift
ranges locate both endpoints by rendered group/task occurrence and deduplicate
canonical IDs only after slicing that span. Tree expansion and manual child
preferences remain task-scoped; group collapse stays group-scoped.

Do not feed plugin queries into the current complete-local-scope optimization
until it can prove the same complete, current snapshot revision and ordering.
Initially use the existing bounded server query for these views; built-in local
optimization is unchanged. No automatic page traversal or browser grouping of
an already-sliced page.

Cold task-detail boot does not resolve plugin groupings synchronously. Keep the
provider metadata in boot; the existing authenticated sidebar read resolves
classification after the task route can paint.

## Lifecycle, freshness and failure

Carry provider source revision and plugin lifecycle generation in query identity
and bounded retained-page eligibility. Subscribe to host plugin-state changes
and installation lifecycle to invalidate the active grouping; requery page one
on classification changes, clearing obsolete pagination bounds. A page request
with a changed source revision restarts at page one rather than continuing a
mixed snapshot. Ordinary task events retain existing shared-record freshness
and membership invalidation. Account/workspace/access changes clear eligible
retained pages. A delayed action or response is fenced by user/workspace/query
and provider generation, not only a component's mount state.

Host workspace state mutations emit identity-only
`plugins.sidebar-grouping.updated` events. The gateway exposes
`plugin.sidebar-grouping.updated` only to authorized workspace subscribers;
global catalog changes contain no workspace classification data. Catalog records
are captured under the persistence guard, then projected after releasing it.
Policy and status updates persist a private record before registry publication;
real dispatch admission cannot observe a provisional status. Generation binds
installation, policy and a per-plugin lifecycle epoch. Committed status changes
invalidate even if cleanup fails; failed commits do not. Catalog refreshes preserve
unchanged provider identities.
Status publication advances its epoch inside the commit critical section, then
releases both service and persistence guards before cleanup or publication.
Per-plugin completion guards preserve cleanup order without blocking another
plugin's metadata writes, and release before synchronous event publication.
Successful uninstall publishes the removed plugin ID immediately;
discovery latency cannot admit an old page after that event.
Only store/auth/reconnect boundaries invalidate every provider; unrelated plugin
configuration or lifecycle changes preserve the selected page.
Boot metadata is owned by its concrete store and scope. Context reconciliation
clears it even while disconnected. Failed catalog transport reads do not publish
an authoritative empty catalog: eligible same-context metadata is retained,
with at most two retries after 1s and 2s admitted within a 5s retry budget.
Server Retry-After is never shortened; delays beyond the remaining budget stop
recovery. Authoritative empty reads clear metadata. Authorization denial also
advances the monotonic plugin fence, so identical rediscovery cannot resurrect
pre-denial cached or in-flight pages. Success, failure and queued retries retain
the request/store/scope fences. Enforced-auth anonymous boot omits the catalog;
signed-in and auth-off boot follow the discovery identity policy.

Loading, timeout, disabled/replaced plugin and incomplete snapshot are distinct
from a successful empty classification. Retain same-context eligible pages
marked stale with Retry; on first failure show unavailable state with access to
other saved views. Never synthesize a successful Unassigned-only result from an
error. Missing provider identity remains saved, and successful reactivation
requeries it. Access denial clears visible/retained data. Error messages are
localized, sanitized and exclude plugin response bodies and credentials.

Saved-disclosure compaction carries the probe's workspace/provider/owner and
expected-key fence through journal dispatch. Invalidating a provider while a
prune waits behind another write retires that prune without saving or adopting
its rows. The metadata read has the ordinary bounded read deadline; an admitted
compaction write is awaited outside automatic read retries. Its terminal
failure preserves preferences and requires manual Retry, including when the
write outlasts the metadata-read deadline.
There is no automatic transport retry for compaction. A network failure requires
manual Retry instead of resending a prune after its provider fence may have
retired. A skipped compaction is not an ACK: failed normal-write intent remains
available to queued successors; a terminal skipped item settles against the
latest authoritative snapshot.

## Tags implementation and compatibility

Extend the real Tags plugin with a declared read-only grouping action over its
existing workspace-shared catalog/application document. Return typed complete
snapshots and a deterministic revision derived from that document. Include
human and agent applications once per tag/task; omit unknown deleted tags.
Do not query host databases or infer completeness from only rendered cards.
The old private-storage compatibility layer remains separate and documented.

The dedicated implementation uses `plugin:kandev-plugin-tags:tags`, package
version `0.19.0`, and the matching standalone Go SDK. Its `sidebar-tags` action
reads only workspace Host state `agent-tags`, without migration writes or private
storage reads. It hashes canonical classification fields, excluding notes,
ownership and timestamps; equal human/agent memberships collapse to one pair.
Short hex colors expand to the host's six-digit format. Bounds fail explicitly
without a private-storage task cap or truncation. These are local integration
artifacts, not a publication or deployment.

Nullable application lists/rows and untyped classification fields are invalid,
not empty classification. Both v1 and v2 count every raw application before
legacy migration, inactive/deleted filtering or duplicate normalization; more
than 10,000 fails explicitly. Typed inactive rows and valid empty arrays remain
valid, and reads never rewrite shared state.

Names legitimately written with controls remain readable, using the writer's
blank-name semantics, including a U+0085-only name. Tag labels escape both
controls and literal backslashes reversibly; Multi-tag quotes each original
name once. Catalog IDs and shared state are unchanged. Apply real encoded label
bounds only to emitted labels; malformed definitions still fail closed.

The manifest exposes the new entry only for this action. Old hosts either
reject the newer minimum host requirement at install or keep using their old
package; do not advertise unsupported complete grouping. Older plugins continue
their page-local /tasks facet without appearing in sidebar Plugin views. Reuse
existing Tags mutation and host state-change notifications, covering remote tag
edits as well as the browser's own mutations. Package against the matching
reviewed host SDK; bump the plugin version according to its repository policy.
Publishing/releasing is not authorized by this plan.

## Desktop and phone composition

Keep `TasksViewPicker` in the desktop Tasks header. The selected Tag view shows
normal sidebar tree rows under colored, collapsible group headings. Reuse the
phone Task views/task-switcher drawer and shared selectors/state. Use one
internal list scroll owner, fixed picker/header, existing safe-area treatment,
44px phone menu targets, Escape/back dismissal and focus return. No desktop
workbench is mounted merely to hide it on phones. See plan UI-01/UI-02.

Nested selectors close through the drawer primitive on selection and
scope/catalog changes before their controls unmount. This restores the parent
task switcher's transform, touch geometry and focus handling. Provider errors
retain the existing bounded sidebar read-recovery clock; this extension does
not introduce a separate retry owner.

## Verification and delivery

Tests must prove 101+ tasks, a tag first appearing beyond the original first
page, multi-group duplicates at a page boundary, root/child context, collapse,
authorization, provider failures and delayed old generations. Mounted selector,
workspace save/reload and real desktop/phone flows own the UI proof. A fixture
can exercise the generic provider but cannot replace the real packaged Tags
smoke. No new telemetry labels include task, workspace or tag identifiers;
existing structured query/action errors suffice.

- [Requirements](../requirements/sidebar-plugin-grouping.md)
- [Plan](../../../plans/sidebar-plugin-grouping/plan.md)
- [Workspace views](workspace-sidebar-task-views.md)
- [Backend-owned preferences ADR](../../../decisions/0041-backend-owned-portable-user-settings.md)
