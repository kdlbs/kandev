---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-OWNERSHIP-001
  - REQ-EXECUTORS-OWNERSHIP-002
  - REQ-EXECUTORS-OWNERSHIP-003
  - REQ-EXECUTORS-OWNERSHIP-004
  - REQ-EXECUTORS-OWNERSHIP-005
---

# Per-user executor ownership system design

## Purpose and boundaries

This design extends executor admission and profile configuration. It consumes
`authn.Identity`, `authz.ScopeOrgConfigManage`, and existing workspace/tenant
checks. It does not grant workspace visibility or change unrelated shared
configuration. The implementation and its task-level evidence are recorded in
[the delivery plan](../../../plans/executor-ownership/plan.md).

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-OWNERSHIP-001 | Persistence; Authorization policy |
| REQ-EXECUTORS-OWNERSHIP-002 | Configuration mutations; Responsive surfaces |
| REQ-EXECUTORS-OWNERSHIP-003 | Catalogue and events; Responsive surfaces |
| REQ-EXECUTORS-OWNERSHIP-004 | Admission and execution; Failure behavior |
| REQ-EXECUTORS-OWNERSHIP-005 | Autonomous attribution; Failure behavior |

## Current implementation evidence

- `internal/task/models/models.go` defines `Executor` and `ExecutorProfile`.
  Neither currently records a user owner.
- `internal/task/service/service_resources.go` implements executor/profile
  CRUD. `GetExecutorProfile`, `ListExecutorProfiles`, and
  `ListAllExecutorProfiles` currently delegate to repository reads.
- `internal/task/repository/sqlite/executor_profile.go` persists profiles,
  including optimistic updates. `executor_profile_script_update.go` owns
  script-intent partial updates; ownership must survive every writer.
- `internal/task/handlers/executor_profile_handlers.go` exposes HTTP and WS;
  `internal/mcp/handlers/config_executor_handlers.go` exposes configuration tools.
  `internal/backendapp/settings_*` adds another settings-operation surface.
- `publishExecutorProfileEvent` in `internal/task/service/service_events.go`
  sends configuration to the event bus. `internal/gateway/websocket/task_notifications.go`
  subscribes to executor/profile changes. Catalogue privacy needs recipient
  filtering, not just filtered HTTP lists.
- `authz.Subject.Unscoped` currently grants all org scopes. Absence of a user on
  a background context is therefore not evidence of private-compute authority.
- `internal/automation/models.go` has `ExecutorProfileID` but no Kandev creator
  field in its automation model. Provider fields such as Linear's `CreatorID`
  are issue filters, not a platform identity.

All paths above are relative to `apps/backend/`.

## Persistence

Add nullable `owner_user_id` to `executors` and `executor_profiles`, represented
in Go as optional owner data and in JSON as `owner_user_id`. SQL NULL means
shared; do not create ownership for legacy rows. Empty strings must normalize
to NULL. Update all inserts, selects, ordinary updates, optimistic updates,
script-intent writers, clones, DTOs, and shared SQLite/PostgreSQL conformance
coverage. Use the existing versioned migration infrastructure, with a distinct
ownership migration rather than editing historical migration bodies.

Deletion of a user must not erase resource ownership or implicitly share it.
Retain the owner reference as a tombstone if physical deletion is possible;
do not use `ON DELETE SET NULL`. Deleted/disabled identities fail admission.
Owner reassignment validates an active real account within the applicable
resource/tenant boundary. Synthetic identities cannot be stored as owners.

A parent executor owned by A may contain shared profiles or profiles owned by A.
It cannot contain a profile owned by B. Claim/reassign of a parent with
incompatible child owners is rejected atomically. A shared parent can contain
profiles belonging to different users. Ownership is conjunctive, so a profile
without its own owner never bypasses a private parent. Built-in system executor
rows remain shared; use profile ownership for private presets under them.

Persist `created_by_user_id` on autonomous sources where no equivalent verified
Kandev identity exists: automations, workflows, and each supported provider-watch
family. Persist an immutable server-derived execution actor on pending launches
and sessions needed by retries or new prompt admission. Reuse an existing
verified durable field if implementation investigation establishes equivalence;
do not introduce a second source of truth. Legacy unknown creators remain NULL;
never backfill them from an arbitrary workspace owner. No credentials or role
snapshots are stored in actor records.

## Authorization policy

Implement one executor-resource policy consumed by task services and admission,
using the existing auth identity and scope resolver. Parent/profile reads remain
internal repository operations; user-facing reads and execution are authorized
service operations. Do not put user filtering in a repository method also used
for maintenance, backup, recovery inventory, and cleanup.

Evaluate existing tenant/resource reach first. For a real active user, a resource
is usable when shared, owned by that user, or accessible through
`org.config.manage`. Both parent and profile checks apply. A manager bypass does
not confer workspace permissions. Resolve current account status and role at
background dispatch and admission, rather than trusting creation-time roles.

Only a positively established authentication-disabled synthetic identity gets
the legacy single-user bypass. `Subject.Unscoped`, a missing identity, an event
bus context, or `context.Background()` never bypass ownership. Actorless legacy
background work can use shared resources through its current shared-resource
contract; it cannot use owned resources. Keep runtime maintenance capability
separate from authority to launch or prompt.

## Configuration mutations

Extend create/update request types, HTTP/WS payloads, MCP schemas, and settings
operations. PATCH distinguishes absent (preserve), null (clear), and a user ID
(assign). A plain `*string` decoder that conflates absent and null is insufficient;
use the repository's optional-field pattern or a presence-aware DTO.

Existing permission checks for shared configuration remain in force. A permitted
creator can explicitly create personal resources; a non-manager cannot claim an
existing shared resource or nominate another owner. Current owners can clear
ownership. Managers can assign/reassign/clear. All protected configuration reads,
updates, deletes, connection tests, clones, preparation/build actions, and secret
reference resolution enforce effective parent/profile eligibility first.

Ownership-only edits do not authorize protected provider configuration changes.
Retain Kubernetes, Remote Docker, and plugin-specific administrative restrictions
for their existing protected fields. Validate the ownership-only payload
separately so that an owner can clear ownership without acquiring configuration
management rights. Do not infer that changing an owner copies credentials into
another profile.

Use transactional or version-checked writes against the observed ownership and
`updated_at`, including ordinary partial saves, so stale edits cannot reverse a
transfer. Deletes also compare the parent/profile ownership snapshot under the
same parent lock as transfers, including an expected missing parent for orphan
cleanup. Parent and child consistency checks share the mutation transaction.
Successful notifications use committed ownership; rejected writes publish none.

## Catalogue and events

Apply the same effective policy to service lists and GETs, enriched nested
executor profiles, boot payloads, settings snapshots, HTTP, WS, MCP, discovery,
connection status, workflow/automation defaults, and fallback resolution. Empty
parents may remain as public provider/type catalogue entries only if their
projection contains no protected instance configuration. Hidden resources do
not contribute configuration or secret references to a public projection.

Retain internal events for infrastructure. For user WS clients, filter profile
and executor payloads per recipient against committed current state. Ownership
transitions also need eviction: use a payload-free catalogue invalidation sent
to authenticated clients, followed by an authorized refetch that replaces the
catalogue. An invalidation must contain no private profile configuration or owner
identity. This also handles a formerly shared resource becoming private. Fence
in-flight catalogue requests by generation so old responses cannot restore
revoked entries. Clear profile caches on logout/user changes. During an
authorized catalogue refetch, retain the mounted editor draft behind a hidden,
inert surface scoped to the same identity and profile. Restore it only when
the refreshed catalogue still grants access; confirmed removal or an identity
change discards it. Unrelated configuration events must not erase unsaved edits.

Foreign direct reads return 404/WS-not-found and the corresponding MCP error;
an otherwise visible resource with a forbidden management operation returns 403.
Do not render details copied from private session snapshots as selectable
configuration. An inaccessible saved default becomes unavailable and blocks
start until an eligible choice is explicitly made; do not silently change the
workspace's saved default or pick another machine.

## Admission and execution

Legacy Office `local_pc` selections resolve to the built-in local executor for
ownership admission only when no actual executor row has that ID. Existing
rows (including retired rows), inaccessible profiles, unknown IDs, and repository failures never fall
back through this compatibility alias. Session and inherited-environment checks
apply the same resolution.

The request identity is the actor for interactive task creation/start, new
session, resume/new prompt, retry, runner switch, workflow movement that starts
work, and delegated MCP actions. Derive MCP identity from trusted task/session
bindings, not tool arguments. Workspace/session scopes are still required.

Resolve the selected profile and its actual parent after all defaults,
inheritance, and continuation selection. Validate ownership before resolving
secrets, calling prepare/build/connection actions, launching compute, or handing
a prompt to an existing agent. Enforce again at the common orchestrator/runtime
admission boundary so repository-direct lookups cannot bypass the policy.
Carry only the trusted actor ID across queue persistence; reload the account
and ownership when the request is admitted. Client-supplied owner/actor fields
cannot establish authority.

Apply the resource eligibility check to existing environment reuse and to
session execution surfaces, including terminal, command/file-write/editor
access, and credential-bearing control operations. A user who reads a shared
task cannot gain shell or agent execution access to private compute. Board and
conversation reads, and already-authorized read-only previews, retain their
existing contracts. Do not introduce a broader preview-sharing mechanism.

Admission is the cutover point: a successful guarded admission may proceed;
a later ownership change does not kill that admitted operation. Pending work
must be revalidated before admission and must not inherit a successful old UI
check. Serialize ownership mutation against the guarded admission decision using
stored ownership/revision evidence; do not hold a database transaction over
remote network operations. A retry/new prompt is a new admission. Passive
reattachment or cleanup can inspect existing runtime inventory without gaining
permission to send prompts, seed credentials, or provision replacement compute.

## Autonomous attribution

At create/import, derive `created_by_user_id` from the authenticated Kandev
identity. Do not accept it as caller-controlled configuration. Updates preserve
it; importing a portable definition assigns the importer. Setup-created legacy
sources remain unknown unless historical creator evidence is unambiguous.

At dispatch, load that account, resolve its current scopes and workspace reach,
and pass a trusted launch actor through queue and runtime admission. Cover
GitHub/GitLab PR and issue watches, Jira/Linear/Sentry issue watches, Azure DevOps
watches, cron/webhook/manual automation triggers, workflow continuations, and
restart/retry paths. A webhook signature authenticates the trigger, not a
private executor owner. Provider issue-author `CreatorID` is never used here.

An interactive trigger or workflow move requires both the requesting user's
eligibility and the creator eligibility if executing configured autonomous work.
A scheduled/poller trigger uses the creator. Any editor changing prompt,
repository, triggers, workflow steps, or executor selection on a source targeting
private compute must be eligible for that resource; otherwise shared-workspace
edit access could turn the creator into a deputy. Validate the resulting source,
not only a changed executor field. A later editor never becomes creator
implicitly. Persist a separate trusted executor actor on tasks created before a
session exists, and on admitted sessions. WIP queues and recovery restore that
actor without reading caller-writable metadata. Preserve it in both the task
repository and Office legacy table rebuilds. Unknown task actors cannot borrow
a workspace or workflow owner. Check run authorization before starting a task/session runtime;
existing run records may retain a denied outcome for diagnosis.

Office, coordinator, plugin, and other indirect launches use the same common
boundary. If a source cannot supply a verified actor, it can continue using
shared resources but must fail closed on an owned resource. This package does
not invent personal ownership rules for those other source types.

## Responsive surfaces

Reuse the executor hub and complete profile editor under
`apps/web/app/settings/executors/`; add ownership near the profile identity,
with shared status, current owner, and allowed choices. Ordinary owners see
self/shared choices; only configuration managers get a user-assignment picker.
Existing provider-specific fields retain their permission treatment. Reuse
shared domain state and handlers for desktop and phone.

Desktop uses the existing Settings form and compact select/combobox. Phone
uses direct Settings navigation to the complete editor, one primary form scroll
owner, and an inset bottom Drawer for the temporary owner choice. The nearest
shipped geometry exemplar is `mobile-executor-profile-spacing.spec.ts`; the
curated mobile choice pattern is Drawer with a fixed title, scrollable options,
and safe-area clearance. This is an occasional short choice within a deeper
editor, so it does not need another destination route. Use 28px desktop controls
and at least 44px actual phone/coarse-pointer hit targets, dynamic viewport
bounds for an overflowing drawer, focus return, and no horizontal overflow.

Task-create pickers reuse the filtered catalogue on both layouts. Show a
localized unavailable-selection state and disable Start when a stored choice
is revoked; retain the draft so an eligible choice can repair it. Loading is
not an empty eligible list. Save errors preserve edits and refetch authorized
state. All new status/help/error text goes through i18next and seven locale
catalogues; generate Traditional Chinese using the existing script.

## Failure behavior and observability

Use existing typed not-found, forbidden, validation, and run-error channels.
Ownership/account lookup errors fail closed for owned resources; never fall
back to admin, synthetic identity, shared default, or workspace owner. Denied
background runs report an authorization reason and require creator/admin action
rather than retrying credentials or provisioning. Preserve protected details
from unauthorized observers even when a run is visible on a shared board.

Record authorization denials with structured operation, source kind, and bounded
reason values; user/resource identifiers belong only in appropriately protected
logs. Do not log secret values, environment contents, or configuration blobs.
Use existing diagnostics; no new metrics framework is needed.

## Related contracts and decisions

- [Roles and scopes](../../auth/requirements/roles-and-scopes.md).
- [Authentication](../../auth/requirements/auth.md): shared-by-default configuration.
- [Profile editor](../requirements/profile-editor.md).
- [Ownership ADR](../../../decisions/2026-10-15-private-executor-admission.md).
- [Implementation plan](../../../plans/executor-ownership/plan.md).

The implementation must qualify the authentication specification's shared
surface statement with the optional executor ownership exception, linking this
pair rather than duplicating it. Public documentation changes ship with behavior.
