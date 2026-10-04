---
status: draft
system: plugins
requirements:
  - REQ-PLUGINS-MANAGED-COORDINATION-001
  - REQ-PLUGINS-MANAGED-COORDINATION-002
  - REQ-PLUGINS-MANAGED-COORDINATION-003
  - REQ-PLUGINS-MANAGED-COORDINATION-004
  - REQ-PLUGINS-MANAGED-COORDINATION-005
  - REQ-PLUGINS-MANAGED-COORDINATION-006
  - REQ-PLUGINS-MANAGED-COORDINATION-007
  - REQ-PLUGINS-MANAGED-COORDINATION-008
  - REQ-PLUGINS-MANAGED-COORDINATION-009
  - REQ-PLUGINS-MANAGED-COORDINATION-010
  - REQ-PLUGINS-MANAGED-COORDINATION-011
  - REQ-PLUGINS-MANAGED-COORDINATION-012
created: 2026-09-25
owners:
  - kandev
---

# Plugin managed coordination system design

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLUGINS-MANAGED-COORDINATION-001` | [Authority and exact commands](#authority-and-exact-commands) |
| `REQ-PLUGINS-MANAGED-COORDINATION-002` | [Conversation lifecycle](#conversation-lifecycle) |
| `REQ-PLUGINS-MANAGED-COORDINATION-003` | [Ordered input and recovery](#ordered-input-and-recovery) |
| `REQ-PLUGINS-MANAGED-COORDINATION-004` | [Workspace observations](#workspace-observations) |
| `REQ-PLUGINS-MANAGED-COORDINATION-005` | [Task commands and execution](#task-commands-and-execution) |
| `REQ-PLUGINS-MANAGED-COORDINATION-006` | [Task commands and execution](#task-commands-and-execution) |
| `REQ-PLUGINS-MANAGED-COORDINATION-007` | [Workspace administration and source writeback](#workspace-administration-and-source-writeback) |
| `REQ-PLUGINS-MANAGED-COORDINATION-008` | [Workspace administration and source writeback](#workspace-administration-and-source-writeback) |
| `REQ-PLUGINS-MANAGED-COORDINATION-009` | [Host UI](#host-ui) |
| `REQ-PLUGINS-MANAGED-COORDINATION-010` | [Reference plugin](#reference-plugin) |
| `REQ-PLUGINS-MANAGED-COORDINATION-011` | [Policy and reconciliation](#policy-and-reconciliation) |
| `REQ-PLUGINS-MANAGED-COORDINATION-012` | [Independent consumers and compatibility](#independent-consumers-and-compatibility) |

## Purpose and boundaries

Kandev supplies generic execution, queries, commands, and reusable UI. Plugins
supply roles, policy, memory, proposals, schedules, reports, and product layout.
This design extends the existing public surface; it does not move the fork's
`internal/orchestration` product into core.

Task claims and completion gates belong to the [tasks design](../../tasks/system-design/coordination-controls.md).
Tool restrictions belong to the [agents design](../../agents/system-design/managed-tool-policy.md).
Automation destination admission belongs to the [Office design](../../office/system-design/plugin-conversation-targets.md).
Integration services retain credential and provider ownership. Workspace services
retain workflow and repository invariants. Their adapters do not copy domain rules.

The approval ledger is already implemented. Reuse
[capability approval](capability-approval.md) and its completed
[plan](../../../plans/plugins-capability-approval/plan.md).
The older [generic Host ADR](../../../decisions/2026-08-31-generic-plugin-host-boundary.md)
provides exact API names. Most higher-level operations in that ADR are proposals,
not implemented features. This design narrows its deliverable scope and explicitly
extends its busy-dispatch behavior with a separate queued-input contract.

## Authority and exact commands

Keep v1 RPCs and their permissions unchanged. Add opt-in `*Exact` RPCs with typed
Go and frontend SDK contracts. `GetCapabilityContext` returns supported operations,
provider restrictions, quantitative limits, effective capability revision, and
manifest digest. Unsupported operations must not appear usable in the UI.

Every exact request includes `request_id` and `workspace_id`. The transport derives
installation identity. Writes require an idempotency key, canonical payload digest,
resource versions, and the relevant pending-transition predicate. Agent-originated
calls also require transport-derived managed execution provenance. Never accept
installation or actor identity from arbitrary tool arguments.

Authorize at admission and immediately before effect using the existing
`Service.AuthorizeCapability` substrate. Bind approval revocation and command
admission through one installation/workspace authorization guard. Persist the
checked revision in the receipt. Revocation blocks later admissions; it cannot
undo an external request already sent. In-flight external effects remain visible
and reconciled. A host restart reloads approvals before admitting queued effects.

Use the existing `host.v2.read:<resource>` and `host.v2.write:<resource>` grammar.
Maintain one registry mapping exact methods to resources, human restrictions,
provider support, and display descriptions. Resource families are
`managed_agent_conversations`, `tasks`, `task_relations`, `task_directives`,
`sessions`, `interactions`, `workspace_configuration`, `source_issues`,
`change_request_evidence`, `automations`, and `usage`. These names are proposed; the wire/manifest
commit freezes them together and must reject undeclared aliases. Read and write
approvals remain separate. Native human decisions cannot be delegated by granting
a broad write capability.

Reuse the ADR's `HostCommandResult` states: `APPLIED`, `ALREADY_APPLIED`,
`NO_CHANGE`, `CONFLICT`, `DENIED`, `NOT_FOUND`, `INVALID`, `UNSUPPORTED`,
`RATE_LIMITED`, `UNAVAILABLE`, and explicitly defined `PARTIAL`. Asynchronous
operations return a durable receipt with their own lifecycle state. An uncertain
external result uses that receipt's `uncertain` state; it is never misreported as
an effect-free `UNAVAILABLE` or automatically retried under a new key.

New SQLite tables, owned by `internal/plugins`, store command intents, canonical
digests, principal/target/version, authorization receipt, and result references.
Shared domain commands consume the same operation identity inside their own
transaction. An outbox hands off lifecycle and external effects after commit.
A plugin receipt alone is insufficient deduplication if a crash can occur after
a domain mutation but before receipt completion. Add recoverable domain operation
records where the current service lacks them. Never promise exactly-once model
execution or arbitrary provider writes.

Retain compact idempotency tombstones for the installation lifetime, including
archived tasks; purge only under explicit host data deletion. Bound request sizes,
page sizes, queue depth, and concurrent executions. Admission rejects saturation
with retry metadata rather than dropping work. API limits are discoverable.

Capability settings use the existing plugin-detail route. Group requested reads
and writes, show workspace and installation, and require an explicit human grant.
Revocation disables affected controls immediately. An upgrade that expands a
manifest needs review; a reinstall has a new identity. No plugin may grant itself
capabilities. The approval audit is append-only.

## Conversation lifecycle

Add `EnsureManagedAgentConversationExact`,
`GetManagedAgentConversationStatusExact`, `ListManagedAgentConversationsExact`,
and `DeleteManagedAgentConversationExact`. The SDK groups them under
`Host.ManagedAgentConversations`. The key is
`(installation_id, workspace_id, instance_key)`, where the instance key is opaque
to core. Reuse task/session runtime storage behind a hidden workflowless task.
Do not expose that internal task as an ordinary board item.

Persist conversation revision, internal task/session IDs, requested profile and
executor, validated tool-policy revision, desired pause state, and retention mode.
`SetManagedAgentConversationPausedExact` changes desired pause state with the
current revision. Pause blocks subsequent starts; stopping a running input is a
separate exact cancellation. Mutable launch settings take effect between turns. A concurrent revision conflicts;
a running turn is never silently reconfigured. No repository means an explicit
scratch execution, not an implicit first repository.

Pause blocks new starts while retaining input. Disable revokes admission, cancels
active execution through the runtime, and preserves unstarted inputs and history.
An interrupted input becomes failed or uncertain based on observed effects.
Re-enable reconciles state and requests human recovery for uncertain inputs.
Upgrade keeps identity and transcripts but rechecks manifest approval and policy.
Uninstall tombstones authority and retains a read-only host transcript; a human
may explicitly purge it. Plugin-owned memory follows plugin data retention and
requires export before uninstall when preservation is desired. Reinstall cannot
silently adopt retained resources from an old installation.

Keep the existing v1 `AgentConversations` implementation and uninstall tests intact.
Its delete-on-disable behavior does not become a hidden migration. The reference
plugin uses only the new lifetime. An optional explicit migration maps an owned
v1 conversation into the new lifetime while idle, with human confirmation.

### Atomic managed settings admission

The [correction plan](../../../plans/managed-conversation-admission/plan.md) covers
AC-PLUGINS-MANAGED-COORDINATION-002.1 and .4-.8. Keep draft status and delivery history. This released correction implements the
host contract without schema, wire/SDK, or execution redesign.

Discovery reads are advisory. Native admission authorizes from current rows,
replacing service-local locks and separate full task/session configuration writes.

Add mandatory typed `EnsureManagedConversation` and
`ChangeManagedConversationState` methods to the task repository/adapter slice.
Typed domain requests/results and errors belong in
`internal/task/repository/managedconversation`: server-owned coordinates, prepared
settings/policy, expected revision, operation/digest; state intent is pause, detach
or invalidate. Results contain fresh task/session projections and created/changed/
replayed flags. No callback, arbitrary patch, exposed tx or generic version engine.
Backendapp/provider wiring requires these methods at compile time; no optional
unsafe fallback. Legacy-only test adapters may fail closed as unavailable. Profile
resolution, tool sorting, validation ordering and configuration-required stay intact.

#### Native transaction and current predicates

Use the writer pool. SQLite reserves its writer BEFORE gated reads through the
existing db.LockTaskHierarchy no-op UPDATE, including independent pools.
PostgreSQL uses READ COMMITTED: workspace hierarchy boundary, physical task row,
primary session rows in ID order, then existing primary executor row. Read current
task after locking; re-read session/executor after locking. Separate lock queries
precede joined session projection: FOR UPDATE on a nullable LEFT JOIN is invalid.
Preserve task -> session order used by creation, promotion, purge and registration.
Reuse tx-level scan/insert helpers; never acquire another repository connection.

Physical session UPDATEs fence runtime state/attempt/snapshot CAS and metadata
writers without new advisory locks. UpsertExecutorRunning already locks task then
session. Lock an existing executor row too: direct executor update/deletion need
not take task/session locks. Absent executor registration is fenced by canonical
task/session locks, not a lock on an absent row; raw unguarded imports are excluded.
Workspace locking covers missing task creation/deletion; task locks cover primary
creation/promotion. Honor active cleanup barriers before repair/config. No runtime,
event, wake or lifecycle callback under locks. Rollback every own failure and
cancellation before commit; no automatic stale-command retry as a new operation.

Validate retained/ephemeral identity, physical/metadata workspace, installation,
instance and detached state from current task. Check operation+digest replay BEFORE
revision. Replay may succeed while busy without config writes; missing-primary
repair uses CURRENT stored config and retains revision. Different digest follows
existing revision handling and host ledger conflicts. Non-replay stale revision
maps to Aborted before busy checks. Changed config with current STARTING/RUNNING or
nonempty joined execution ID maps to FailedPrecondition, even a leftover execution
ID on an otherwise idle row. CURRENT primary identity/incarnation, attempt metadata,
state and execution govern admission; never write a replaced predecessor.
Unchanged config at current revision returns exists while busy without writes or
revision increment. Missing/detached remains NotFound; service maps typed native
errors to existing codes.

#### Commit, writers, and exclusions

Update only primary agent_profile_id, executor_id, executor_profile_id, updated_at.
Preserve runtime state/execution, route, snapshots, metadata, environment/workspace,
errors/completion. Overlay only managed config, invalidated reset, revision and
operation/digest into CURRENT task metadata using raw JSON members plus timestamp; preserve other scalars
and keys, pause/detach/retention, empty-field semantics, effective profile, instruction
and policy comparison. One change or non-replay primary repair increments once.
Create deterministic task+primary together at revision 1; reconcile competing
creation against committed ownership. Failure rolls both back. Historical partial
rows/replay repair retain identity/transcript. Replace the fake expectation of a
new failed creation leaving a task with atomic rollback plus successful retry.

Canonical config/revision writers in managed_conversation_settings.go are create, reconcile/
persist, exact pause, installation pause, invalidation and detach. Route ALL through
the seam. Exact pause checks current replay/revision, increments only on change,
retaining operation stamps. A missing non-replay primary fails before pause commit; accepted replay repairs it. Lifecycle inventory selects candidates; each transaction
revalidates owner and overlays its flags, incrementing on first transition.
Invalidation clears operation/digest; detach also pauses. Neither rejects busy
runtime nor overwrites a config winner. Stop/notifier run AFTER commit; failure
reports must acknowledge committed flags. Keep host approvalEffectMu, disable,
queued-input and immediate busy-dispatch contracts; no second authority/dispatcher.

DeleteManaged is lifecycle removal, not a config/revision writer. Its explicit-delete
revision preflight and cleanup protocol stay unchanged. Admission observes task
absence/cleanup barriers; never repair disposal or wrap DeleteTask in this tx or
replace lifecycle deletion with bare SQL. Legacy v1, ordinary full-snapshot/bulk
imports and generic task writes retain existing contracts. Ordinary metadata merges
serialize physically and survive this admission; later explicit legacy replacement
is excluded. Runtime preparation predating config commit gets no new launch snapshot
contract. Runtime state/execution accepted BEFORE admission is preserved; writers
blocked behind it resume under their existing rules.

#### Results and evidence

Descriptors use the committed projection. Rejection has no applied descriptor,
creation event, wake or stop. pluginHostManagedConversationManager retains actual
CommandStore.Admit/Complete; Aborted/FailedPrecondition remain CONFLICT. Operation
stamps support replay after lost receipt acknowledgement; preserve
command_receipt_unavailable with committed descriptor, not an effect-free claim.
Publish best-effort task.created only after new-task commit, not repair/replay/
nochange; resume notification follows committed pause admission.

The work order defines permanent existing-API RED before correction without ROOT
proof replay. Independent real SQLite and registered Host v2 prove status/receipts;
only profiles/external transport may be mocked. PG16 requires physical task/session/
executor waits, cancellation and READ COMMITTED current-result proof. Inject failure
between writes for update/create/repair rollback; check identity, metadata, runtime,
events, replay/nochange. SQLguard/store conformance apply without exemptions.

No rendering, navigation, touch/scroll, viewport, copy or browser/runtime instance
changes: mobile parity uses pure state/data exception with integration proof.
Public plugins-authoring already promises revision/active-turn rejection; restoring
it needs no guide edit. Preserve the broad coordinator package's completed history.

## Ordered input and recovery

Keep `DispatchManagedAgentConversationExact` as immediate dispatch with a typed
busy result. Add `EnqueueManagedAgentInputExact`, `GetManagedAgentInputExact`,
`ListManagedAgentInputsExact`, and `CancelManagedAgentInputExact` for durable input.
The reference plugin and automation destination use enqueue exclusively.

Store input ID, occurrence key, canonical payload, origin, enqueue sequence,
conversation revision, timestamps, execution reference, and state in host storage.
States are `accepted`, `running`, `completed`, `failed`, `cancelled`, and `uncertain`.
An execution-admission record and sequence lock prevent two workers from starting
the same accepted input. Enqueue is transactional and FIFO per conversation.
Return receipt identity before displaying a prompt as delivered. The component
keeps its retry key until it receives authoritative acknowledgement.

Only pending periodic inputs with an explicit coalescing key may be replaced.
Replacement records supersession; human inputs and answers are never coalesced.
Cancellation of an accepted input is atomic. Cancellation after start requires
an exact execution generation and reports whether a stop was confirmed.

Reuse `internal/orchestrator/messagequeue` and shared runtime dispatch where their
semantics match. A dedicated managed-input repository may wrap those queues;
there must be one durable admission authority, not competing drains.
On startup, join receipts to execution state before scheduling. Retry automatic
launch only when it is known not to have started. A lost completion acknowledgement
or crash after a tool effect needs reconciliation or explicit retry consent.
Queue acceptance is not a completed agent response.

## Workspace observations

Implement the ADR's exact workspace/workflow/step/task/session/interaction queries,
`ListSanitizedMessagesExact`, task inbox/directive queries, relations, and pending
transitions. Lists use opaque cursors bound to workspace, installation, filters,
approval revision, and snapshot. Return resource versions and observed timestamps;
reject stale cursors instead of producing a mixed snapshot.

`ExactSessionObservation` also returns the opaque current execution ID with its
session resource version and state. A consumer may offer guarded recovery only
from this exact observation; both values are command preconditions, and the Host
rechecks them before launch. This field is available through the declared and
approved `host.v2.read:sessions` operation, not the browser task projection.

Project task state from `internal/task/statussummary`, rather than asking plugins
to derive it from task and session enums. Include step group, assignee, management
claim, blockers, current execution, last meaningful activity, source links, and
pending moves. Reads stay bounded; message content is sanitized and paginated.

`GetChangeRequestEvidenceExact` and `ListChangeRequestEvidenceExact` return linked
provider/repository identity, PR head revision, check/review/thread evidence,
fetch time, and provider error or rate-limit state. They do not grant merge,
comment-writing, or CI-rerun authority. Evidence becomes stale when its subject
head changes. Unsupported providers return unsupported, not an empty success.

Add a bounded `ListTaskUsageExact` projection over the existing
[task cost ledger](../../task-cost-ledger/spec.md). Reuse
`task/service.GetTaskUsageTotals` and `GetTaskSessionUsageTotals`, including retained
rows after session deletion. Preserve integer `CostSubcents`, event counts,
estimated/unpriced counts, and output-token completeness. A scope with no recorded
events retains its native zero aggregates plus an explicit no-observation state;
it must not be described as a measured free run.
Report measured tokens, provider currency/cost when known, elapsed durations,
and estimated values separately. Unknown is nullable with a reason, not zero.
Expose aggregate task/run counters and snapshot time; plugins choose reports.
Do not add a billing ledger merely to support dashboard estimates.

Plugin events are wake hints. The current delivery queue can drop events and is
not a durable log. Plugins persist their own reconciliation cursor and periodically
reread authoritative snapshots, including after reconnect. Never trust an event's
old task state as the authorization precondition for a write.

## Task commands and execution

Implement `CreateTaskExact`, `UpdateTaskExact`, `MoveTaskExact`, `SetTaskLabelsExact`,
`AddTaskRelationExact`, `RemoveTaskRelationExact`, `SendMessageExact`,
`IssueTaskDirectiveExact`, and `ResolveTaskDirectiveExact`. Extend with typed
`AssignTaskExact`, `ArchiveTaskExact`, and human-confirmed deletion commands.
Creation accepts explicit repositories, workflow, step, profile, executor, parent,
and source identity. Do not overload a generic unvalidated JSON metadata writer.

Enforce idempotency and source identity deduplication under the task transaction.
An archived match is returned for explicit reuse/unarchive handling. Do not silently
create new work or launch it when a duplicate source task exists. Relation writes
validate workspace scope and cycles. Parent assignment does not grant deletion
or management authority. Messages/directives return accepted receipt IDs, not
claims that an agent has acted on them. SDK batching is bounded fan-out of these
single-target commands with a distinct idempotency key and receipt per item. It
returns explicit per-item outcomes; it does not imply atomic multi-task success.

`EnsureTaskRunExact`, `StopTaskRunExact`, `RecoverSessionExact`, and
`CancelPendingTaskTransitionExact` call shared orchestrator/runtime services.
Fence every action to the observed execution and pending-transition generation.
Preserve normal workflow admission, WIP, completion, and pending-move TTL rules.
Recovery can settle a genuinely execution-less interrupted turn and resume only
through normal launch. It cannot clear provider login locks or modify credentials.

`RespondPermissionExact` and `AnswerClarificationExact` use the host interaction
service and a host-issued, single-use human response receipt. Plugins may relay
a request to shared UI; agent text is not evidence of consent. First valid response
wins. `SetSessionModeExact` supports only modes the adapter advertises and the
workspace permits. It cannot enable bypass permissions. The shared UI owns these
high-trust decisions even when embedded in a plugin page.

The legacy v1 `RespondToPermission`, `AnswerClarification`, and
`CancelClarification` methods remain in the wire schema for source compatibility,
but return `PermissionDenied` because they cannot bind a human decision to the
observed interaction revision and response payload.

Management claims and completion evidence use the tasks-owned commands defined
in the adjacent design. A task management write from any plugin checks an existing
claim; ordinary human commands remain possible and audit supersession. A watched
task need not be claimed until an explicit adoption action.

## Workspace administration and source writeback

Expose bounded typed commands for workspace name/description/default updates,
workflow/step create/update/reorder, and local or remote repository registration,
attachment, and base-branch settings. Defaults can reference only available profiles
and executors. Step fields include prompts, stage, WIP, transition conditions, and
supported session policies through typed native DTOs. Host adapters call existing
workspace/task services. No arbitrary workspace DB writer, hidden-workflow editor,
or creation of foreign workspaces. Repository paths, URLs, and provider identities
use existing native validation. Script content and secret-binding changes remain
human-only native configuration actions; the plugin may direct the user to them,
not obtain execution or secret-scope expansion through an administrative API.
Synchronized workflow edits, active-step deletion, repository detachment, and
worktree state retain their domain restrictions. Preview/confirm destructive
operations with target versions and expiration. Host UI records consent; a plugin
cannot mint confirmation. Unsupported destructive operations remain unavailable.

`GetSourceIssueCapabilitiesExact`, `CommentSourceIssueExact`, and
`TransitionSourceIssueExact` resolve an existing task source link in host services.
Initial implementations cover Jira and Linear through their native clients.
Map provider transitions from current issue state; do not accept arbitrary URLs
or credentials. A link change invalidates a prepared write.

Persist the external intent before sending, and store the provider receipt after.
On timeout, reconcile through provider identity or idempotency support when
available. Otherwise report `uncertain` and require explicit human resolution.
Do not automatically repeat comments, assume rollback, or report an optimistic
transition as verified. Apply provider rate limits and bounded backoff before
known-safe retries. PR discussions through other integrations remain optional
plugin integrations, outside this host writeback contract.

## Host UI

Export `host.ui.WorkspaceAgentChat` with workspace/conversation IDs, resource
version, read-only state, and status callback. Add typed facade methods for
managed-input receipts, interactions, and supported recovery. Keep v1 task-panel
conversation consumers compatible. The component owns transport subscription,
reconnect, ordering, composer retries, accessibility, and shared native dialogs.
It must not embed a coordinator role name, plugin route, or task-selection policy.
Recovery is shown only when the controller reports support and the observed
session is not starting or running and has both resource-version and
execution-generation fences. A native confirmation precedes the exact recovery
request. Desktop uses a split chat/task view; phone uses full-height Chat, Tasks,
and Outcomes tabs and a bottom-drawer instance selector.

Provide composable canonical task-status and usage display primitives plus
query hooks. The plugin supplies selected tasks, grouping, actions, and labels.
Plugin routes, nav contributions, configuration, and storage remain the existing
extension mechanisms. A host-owned transcript route supports retained uninstalled
conversations with no plugin JavaScript required.

Desktop may place chat beside a task list. Phone uses a full-height route with
Chat/Tasks tabs, a pinned header, one scrolling body, and a safe-area composer.
Instance selection and filters use a bottom drawer with a fixed title/action row.
Touch targets are at least 44px; desktop controls retain normal density. Support
keyboard focus, screen readers, reduced motion, and no horizontal page scrolling.
Localize host copy in all supported catalogs. Plugin examples demonstrate the
existing localization interface. See the plan's stable UI previews.

## Reference plugin

Build in a dedicated repository based on `kdlbs/kandev-plugin-template`, never
under Kandev's production packages. The implementation work creates a local sibling
checkout; repository publication and marketplace release are separate actions.
Reuse only public Go/frontend SDKs. Do not assume an existing third-party plugin
repository is owned by Kandev or silently update it.

The plugin owns SQLite tables for role templates, workspace instances, memory,
task watches, proposals, trigger cursors, intent outbox, and outcome projections.
Role templates contain display name, icon, instructions, default tools, and limits.
Instances select role, profile, executor, task scope, overrides, and pause state.
Core stores only opaque instance keys. Every plugin key includes workspace and
instance; storage does not grant cross-workspace authority.

Each agent tool validates structured arguments, then invokes typed Host APIs.
The only production tool chain is managed agent -> namespaced plugin tool ->
plugin backend -> Host command -> shared domain service. Do not add a second
plugin MCP server or route commands through global ambient Kandev MCP.
The plugin defines which tools are appropriate for each role; host execution
policy limits that choice to installed, approved tools.

## Policy and reconciliation

A proposal stores immutable input, revision, approval state, source identity, and
stable command key. Approval is a compare-and-set transition that queues one
outbox intent; duplicate approvals read the same receipt. Editing approved work
creates a new proposal revision and invalidates its old approval.

Persist outbox intent before calling Host. Reconcile receipt identity after a
crash instead of issuing a new key. Apply event filters, debounce, and periodic
full reconciliation to prevent callback loops and recover dropped events.
Deduplicate callbacks by input event identity plus observed task revision.
Bound follow-up depth and concurrent runs. Pause suppresses new policy actions
and preserves work; it does not automatically stop independent worker tasks.

Recurring routines may use the native automation destination or a plugin-owned
scheduler with durable occurrence IDs. The reference uses native automation where
available. Host dispatch owns admission reliability, not the policy that chooses
when to act. Plugin budgets stop new discretionary work at configured measured
or estimated thresholds; they are not a promise of an exact financial hard cap.
Report pending usage and potential overshoot from concurrent runs.

Claims require explicit adoption. The plugin may propose completion criteria,
collect evidence, and submit verification with subject versions. Tasks own the
gate. Plugin report text cannot mark a task complete. Reports show completed,
failed, blocked, and uncertain outcomes with provenance, duration, and available
costs. Linked-issue writes use their own optional capabilities.

## Independent consumers and compatibility

Package a second identity with a proposal-first observer policy: no automatic
adoption or worker start; it watches selected tasks and proposes follow-ups for
human confirmation. It must define different tools and instructions while using
the same generic APIs. A test fixture is suitable for host regression tests; the
second example itself lives outside the monorepo with the reference consumers.

Version the Go SDK, protocol, manifest rules, Host UI declarations, frontend SDK,
and authoring guide together. Feature detection controls optional functions.
Set `min_kandev_version` to the actual first shipped release when known, never
a guessed future version. Verify old plugins still install and run through v1.
New providers require tool-policy contract tests before advertising support.

## Persistence, failure, and security

Host SQL migrations are additive and tested on existing databases. Core retains
transcripts, admissions, command receipts, claims, and criteria. Plugin data stores
policy state only. Model/provider secrets remain in native integrations. A plugin
crash does not erase host audit or release task ownership. A host restart does not
reset idempotency identities. Downgrade below the required schema or API version
must be rejected or use the documented host backup/restore process.

Native plugins and their JavaScript are trusted installed code, not an OS or browser
sandbox. These contracts constrain supported Host access and managed agents; they
do not claim to contain a malicious installed process. Restricted execution still
requires provider enforcement, current approval, and runtime fencing.

## Observability

Persist operation receipts and expose their IDs in plugin diagnostics and shared
UI. Record lifecycle, queue admission, deduplication, denial, conflict, reconciliation,
and uncertain outcome events. Keep metric labels to operation family, state,
provider kind, and reason. Put workspace/task/conversation IDs in structured logs,
not metric labels. Never log prompt bodies, approval tokens, or secrets.

## Related documents

- [Requirements](../requirements/managed-coordination.md)
- [Implementation plan](../../../plans/plugin-coordinator-platform/plan.md)
- [Coordination platform decision](../../../decisions/2026-09-25-plugin-coordination-platform.md)
