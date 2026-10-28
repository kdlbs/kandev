---
status: draft
system: integrations
created: 2026-10-09
requirements:
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-002
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-003
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-004
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-005
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-006
  - REQ-INTEGRATIONS-CHANGE-REGISTRATION-007
owners:
  - kandev
---

# Plugin change request registration design

## Boundary and mapping

This is the plugin transport/storage boundary of [task change registration](task-change-registration.md), owned by Integrations.
It extends the same vertical feature. It does not create a separate association system or transfer plugin persistence to the host.
The parent design owns admission, composer behavior, retries, observation-only policy, and shared MCP guidance.

| Requirement | Sections |
| --- | --- |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-002 | Identity and resolution |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-003 | Shared tools and guidance |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-004 | Persistence and replay |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-005 | Authority and side effects |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-006 | UI projections |
| REQ-INTEGRATIONS-CHANGE-REGISTRATION-007 | All sections |

## Verified existing seams

- `apps/backend/internal/plugins/mentions.go`: `MentionSourceBridge` loads active manifest `reference_sources`. Search and submission authorization use generation-bound plugin calls.
- `apps/backend/internal/plugins/manifest/manifest.go`: `ReferenceSource`, `repository_providers`, and authenticated action declarations already exist.
- `apps/backend/internal/plugins/repository_provider_inspect.go`: `InspectRepositoryProvider` invokes the owner's `repositories.inspect` action and validates the returned descriptor.
- `apps/backend/internal/plugins/repository_provider.go`: `repositoryProviderAction` resolves active ownership; `InvokeAction` is the backend dispatch seam.
- `apps/backend/internal/plugins/action_handlers.go`: the HTTP relay validates task/workspace/repository/session selectors and constructs `VerifiedActionContext`.
- `apps/backend/pkg/pluginsdk/types.go` and `proto/kandev/plugin/v1/plugin.proto` below `apps/backend`: `PluginActionRequest` carries verified context and bounded JSON bodies over existing `HandleAction`.
- `apps/web/lib/plugins/types.ts` and `registry.ts`: registered review providers supply task snapshots, workspace association snapshots, subscriptions, refresh, and optional unlink.
- [Plugin API](../../../plans/plugins/PLUGIN-API.md), [mutation ADR](../../../decisions/2026-08-10-plugin-change-request-mutations.md), and [repository task creation](../../plugins/system-design/repository-provider-task-creation.md) define the established ownership split.

The original entity-reference design still describes its plugin bridge as future work. Current code and public manifest contracts already implement that bridge.
Update that stale scope wording during implementation. Do not implement another search registry.
Frontend callbacks alone cannot service a submitted message or MCP call without an open browser. This package adds the missing backend contract.

## Capability declaration and compatibility

Propose additive manifest `change_request_providers` entries with:

- `provider_id`: an existing repository-provider identity owned by this plugin.
- `contract_version: 1`: the complete registration contract below.
- `reference_sources`: exact declared source IDs that return PR/MR references for this provider.
- `review_provider_id`: the plugin's existing browser review-provider ID for projection/invalidation.

All entries require the three standardized authenticated task-scoped actions:
`change_requests.resolve`, `change_requests.list`, and `change_requests.manage`.
Version 1 requires exact listing, observation-only link, unlink, operation receipts, revisions, tombstones, and explicit restoration.
Replacement is composed by the host from these operations with existing compensation rules; no separate plugin replace callback is required.
This is a proposed addition, not an existing manifest field. Validate the declaration in manifest/store/registry and export SDK JSON DTOs.
Use existing `HandleAction`; no new RPC transport or MCP tool is necessary.

Reject duplicate active ownership, sources owned by another plugin, invalid kinds, unknown versions, and incomplete action declarations.
Derive capability from active backend ownership and contract compatibility, not from a UI bundle, source label, or repository provider alone.
A search-only plugin retains its autocomplete results with Context only and “Task linking is unavailable for this provider”.
A capable plugin can support agent URL linking without declaring search. In that case it supplies no autocomplete results.
Browser review registration improves presentation but is not needed for headless server registration.

Older hosts/plugins keep established manual actions. New registration fields require the minimum host version documented with the SDK release.
Do not silently advertise new behavior for an installed plugin that did not adopt version 1.
The host package ships generic conformance fixtures. It does not ship Bitbucket or Forgejo production code in the monorepo.
Provider authors adopt these typed actions over their existing provider-specific association services; existing records and manual flows stay authoritative.

## Identity and resolution

Canonical plugin identity is:
`(task_id, provider_id, host, connection_scope, provider_repository_id, change_request_id)`.
Retain Kandev `repository_id` as the authorized task attachment and `review_key` as an opaque UI selector.
Neither a review key, display title, URL basename, nor numeric PR value alone establishes identity.
`change_request_id` is the provider's canonical bounded nonempty string. Numeric providers normalize their positive integer to decimal text.
Connection scope preserves separate installations, tenants, and Data Center context roots on the same host.
The immutable provider repository ID prevents rename/path changes from moving an association to another repository.

`resolve` input contains either a URL or a validated selected reference, plus host-derived task repository descriptors.
`VerifiedActionContext` supplies the actual workspace/task and authorized optional session/repository selectors.
The response is either `{matched:false}` or an exact canonical identity, canonical URL, title snapshot, and association revision/state.
Resolve performs provider authorization and read-only validation. It cannot create a repository, association, checkout, watch, or task.

For selected references, map the manifest source to its declared repository provider and active plugin owner.
Do not infer ownership by stripping a `plugin:` prefix or equating mention-provider and repository-provider strings.
For URL-only calls, first use trusted attached repository/provider descriptors and configured origins to select eligible resolvers.
Invoke eligible resolvers with bounded concurrency/timeouts. Plugins parse their own URL grammar; the host validates returned host/scope/repository against attachments.
Zero matches returns unsupported/unresolved. Multiple matches return `ambiguous_provider`; registration order never breaks ties.
An explicit provider is an assertion that narrows resolution, not authority. Explicit repository/number/ID conflicts fail before mutation.
Retain exact Data Center context roots and Forgejo host identities. Never apply GitHub URL patterns to plugin URLs.

Repository-only plugins and unknown URLs remain context or receive a structured error. No auto-import or first-repository fallback is permitted.
A fork's change-request identity uses the target repository that owns the request. A head-fork attachment alone does not satisfy target identity.

## Action contracts and authority

`list` takes the verified task plus opaque cursor and bounded page size. It returns exact associations, revisions, origins, policies, and a next cursor.
Each association includes active/detached state when requested for reconciliation. Normal UI lists return active associations.
Unresolved historical entries retain unresolved status; do not guess identity to satisfy a tool DTO.
Bound aggregate reads and report incomplete pages/errors. A plugin failure never appears as an empty complete set.

`manage` accepts host-prepared operation `link | unlink`, exact resolved identity, stable operation ID, expected revision,
source/origin, and `association_mode: observation` for registration. Explicit restoration carries its separate intent.
The response includes operation receipt, current revision, known state, canonical association, and structured error code.
Provider API payloads and credentials are never returned as error text.

Reuse authenticated action dispatch, access checks, body/response bounds, cancellation, and `pluginDispatchGeneration`.
The host resolves caller identity before building verified context. Agent arguments and browser bodies cannot supply source, workspace authority, or mutation revisions.
Reserve the standardized registration action keys for coordinator dispatch. Direct generic action relay calls cannot bypass registration ordering.
Expose `POST /api/v1/tasks/:task_id/change-requests` as the shared browser management entry to the same coordinator.
Add a typed browser SDK helper `host.changeRequests.manage` around that endpoint; the helper is proposed and must be documented with the SDK.
Compatible plugins route their existing manual link/unlink callbacks through that helper and reuse their association services behind the backend actions.
Legacy plugin callbacks retain their old path until capability adoption; they are not advertised for automatic registration.

A capability declaration grants no broader repository or execution permissions. Read and mutation calls use the user's authorized task/workspace.
Retain action access requirements when invoked from MCP; a backend bridge must not bypass an admin-only declaration.
Plugins do not read host tables or query private host APIs. Provider credentials remain in their established plugin connection scope.

## Shared tools and guidance

Extend the existing neutral coordinator's provider dispatch with a plugin adapter. Keep built-in adapters and legacy fields compatible.
`provider` remains a registry-validated string. Add optional `change_request_id` and `connection_scope` for explicit plugin identities.
Keep existing integer `number` for built-ins and numeric plugins. If both number and string ID are present, require the same canonical identifier.
URL-only remains the routine shape; agents do not need repository IDs or plugin action names.
For replacement, add matching optional `old_change_request_id` and `old_connection_scope`; require complete same-provider old identity.
Conflicting scopes fail closed. Host composition preserves new-link-first behavior and never compensates away a link that predated the request.

Management/read responses add these plugin fields without changing numeric built-in fields.
`get_task_change_requests_kandev` joins built-in and plugin projections, reports per-provider completeness, and includes provenance/capabilities.
`list_tasks_kandev` and `list_related_tasks_kandev` keep their legacy GitHub `prs` projection; additive plugin entries join `change_requests` where that projection exists.
Use bounded task-scoped provider reads, not a full remote PR fetch per card. UI uses the existing workspace association snapshot path.

Tool discovery includes active capable plugin-only tasks in both supported MCP protocol eras and preserves mode restrictions.
Shared guidance says PR/MR work includes compatible plugin providers, with the same URL, stack, background, final-check and failure rules.
No new per-plugin registration tool or duplicate instruction block is needed.
Existing plugin-specific tools can coexist, but do not substitute for the shared linked-set check.
Do not expose built-in auto-fix, merge, or outcome-reporting support merely because a plugin can register a link.

## Persistence and replay

Provider associations remain in the plugin's existing durable store. The host retains only admission intent, operation receipts, and bounded result snapshots.
There is no atomic transaction across host SQL and plugin storage. Do not apply the parent design's built-in transaction guarantee across this boundary.

1. The host commits the authorized reference snapshot and intent with the message/queue transaction. Resolved identity and expected plugin revision are nullable.
2. The host invokes `manage` using a stable operation ID derived from that intent. Automatic retries never generate a fresh relink operation.
3. The plugin atomically writes its association/tombstone, monotonic revision, provenance, and operation receipt in its own store.
4. The host records the confirmed result, then publishes status/invalidation. A lost response leaves the host intent uncertain, not failed-empty or linked.
5. Recovery reuses the operation ID and performs exact readback. The plugin checks its receipt before reapplying work and reports current state alongside original outcome.

### Resolution and admission fence

Reference authorization remains mandatory before admission. Association resolution is separate and cannot reject an otherwise authorized message.
A bounded read-only resolution attempt can run before admission, outside the SQL transaction. It cannot create an association or operation receipt.
If that attempt succeeds, admission records its identity, installation, connection scope, revision, and detached state as an immutable fence.
If it fails, admission records null identity/revision and `failed` with `resolution_unavailable`. The message remains accepted and Retry stays available.
No provider call runs inside the acceptance transaction. Queue receipt replay returns the recorded fence before any new resolver call.

After acceptance, processing reauthorizes the caller and attachment, then resolves the exact provider identity.
If a saved identity conflicts with the result, processing fails without mutation. Neither Retry nor a newer URL result can rebind an intent.
An unresolved intent remains bound to its admitted installation, connection, and authorized reference. Ownership changes require a fresh action.
Missing or changed repository identity fails closed instead of trusting a reused path.

| Stored fence | Permitted mutation | Recovery outcome |
| --- | --- | --- |
| Known revision | Compare-and-set against that revision | A stale detached revision is suppressed; Retry never advances the fence |
| Null revision | `create_if_absent`, with restoration forbidden | Create only when no row/tombstone exists; an active row is idempotent success; any tombstone is suppressed |
| Uncertain mutation | Reconcile the original operation ID and fence | Show `uncertain` until the plugin confirms current state; never generate a new mutation identity |

Add `create_if_absent` to the typed `manage` precondition. The plugin enforces it under the same lock as unlink and receipt persistence.
Provider resolution can race with unlink before host admission. The captured fence stays unchanged, even when that unlink precedes acceptance.
This conservative result shows Unlinked and offers Link again. It never converts a stale revision into permission to restore.
If Link again cannot obtain an exact identity and revision, it returns a recoverable resolution error without restoration authority.
The next deliberate Link again uses a fresh operation ID and a new fence. A successful reply replay retains its original fence.

Required schedules include unlink before resolution, between resolution and acceptance, and after acceptance before mutation.
Also cover unavailable resolution through acceptance, restart, tombstone creation, and recovery with `create_if_absent`.

Use the plugin's existing transactional store. Host `state` key/value methods provide no cross-key transaction or compare-and-swap guarantee.
A provider using those methods must supply equivalent durable serialization before it advertises version 1; declaration alone is not conformance.

Unlink shares the same plugin identity lock/revision discipline as link and automatic detection.
It creates a tombstone even when no active association exists. A stale in-flight link cannot overwrite that tombstone.
Manual unlink, backend unlink, plugin branch detection, and plugin status sync must use the same storage rules.
A replayed successful link receipt cannot resurrect state or cause the host to show success after a newer unlink.
Fresh selected-reference admission or explicit Link again can restore only with a captured exact revision. Ordinary Retry cannot acquire restoration authority.
All relevant stack layers and other task links remain untouched.

If the plugin commits before a network loss, exact reconciliation can recover success without another upstream action.
If a readback cannot establish state, return `state_known:false` and keep Retry available. Never retry PR creation to repair an association.
Retain first-origin metadata and nullable message/session pointers; message deletion cannot remove the plugin's association.
Task deletion invalidates host intents and follows existing plugin task-cleanup events/read guards; retain current exact task identity checks on every callback.

## Disable, upgrade, and uninstall

Capture plugin installation identity and dispatch generation before each request. Validate current ownership and compatibility again before consuming its result.
Never dispatch a stale request to a replacement owner just because it uses the same provider name.
Disable or upgrade cancels pending host dispatch. Cancellation does not prove that a plugin transaction rolled back.
A late result from an obsolete generation cannot update the UI. Mark it uncertain and reconcile with the same installation after compatible restart and renewed authorization.
Changed provider ownership, connection identity, or incompatible schema requires explicit recovery; it cannot trigger a fresh automatic link.

Uninstall removes plugin state under the established lifecycle. Preserve the submitted reference and host origin receipt as unavailable history.
Do not report the previous links as a verified empty set. Never replay old intents automatically into a new installation after reinstall.
The new installation can receive a fresh intentional link after authorization. This does not promise recovery of plugin data deleted by uninstall.
If a disabled plugin cannot commit unlink, show failure/unavailable and preserve last-known state; never claim removal from an inaccessible store.

## Authority and side effects

Version 1 registration is observation-only. The plugin must neither change checkout/comparison nor create a wake watcher, fix, merge, or workflow transition.
New plugin links have disabled execution authority. Registering an existing link preserves previously granted authority without granting more.
Existing polling/status observation can refresh the linked PR through provider-owned lifecycle code; this feature adds no polling daemon.
Plugin automatic branch detection must preserve registration policy and unlink suppression.
A provider that cannot separate its link operation from execution effects is not compatible and must remain context-only until updated.

## UI projections

Use the parent's chip dropdown, phone drawer, submitted states, and retry/relink controls without provider-specific layouts.
Resolution failure shows Not linked with Retry. A lost mutation reply shows Status unknown with Retry for receipt reconciliation.
Neither state resends the message. A suppressed result offers Link again, which must obtain a fresh fence before restoration.
Display provider name plus repository and identifier; same-number entries from different connections remain distinguishable in details and accessible names.
Both built-in and plugin results use `task.change_request_registration.updated` for reference feedback.

After confirmed mutation, invalidate the matching registered review provider's task and workspace association resources.
Use its `refresh`, `refreshAssociations`, subscription and snapshot methods. Extend the shared provider invalidation contract in the SDK if necessary.
Lists/counts update from committed association snapshots, not a second optimistic plugin array or synthetic GitHub event.
Carry revision/provenance in additive `ReviewTaskAssociation`/`ReviewItemSummary` fields and preserve lifecycle cancellation on unload.
After reconnect or lost invalidation, refresh the provider's existing resources. Read failures retain an unavailable/last-known indication, never a misleading zero count.
Origin details use the shared UI-05 behavior. Existing plugin unlink controls target the same exact identity through the coordinator helper.

## Delivery and validation

[Task 04](../../../plans/pr-mention-linking/task-04-plugin-registration.md) delivers the generic backend/UI capability and conformance fixture.
It follows the three built-in work orders and completes this package's plugin scope.
Update public manifest/authoring docs, Go SDK DTOs, browser SDK contracts and `PLUGIN-API.md` together during implementation.
No specific installed Bitbucket or Forgejo version is claimed compatible without its manifest and conformance evidence.
The [matrix](../../../plans/pr-mention-linking/validation-matrix.md) includes Bitbucket Cloud/Data Center and Forgejo-shaped fixtures, protocol errors, disable/restart, and lost replies.
Production plugin adoption remains in each plugin's dedicated repository and release process; no extra task or repository attachment is authorized by this plan.
