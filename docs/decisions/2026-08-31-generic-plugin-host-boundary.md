# ADR-2026-08-31-generic-plugin-host-boundary: Generic plugin Host boundary and capability approvals

**Status:** accepted
**Date:** 2026-08-31
**Area:** backend, frontend, protocol, security, workflow

## Context

The Coordinator product is moving into a separately released plugin. Its policy,
scheduling, durable state, reconciliation, prompts, reports, tools, and product UI
belong to that plugin, but it still needs a supported way to observe and change
Kandev state. The current Host contract is too small for that use case, and its
manifest declarations are not an operator approval ledger.

The preserved migration baselines prove useful shapes but also show the boundary
failure. Host PR [#2793](https://github.com/kdlbs/kandev/pull/2793) at
`afd2b699bfe9b6af9353ea01728582f61a7be2be` combines generic conversation and
relation work with Coordinator-specific principals, grants, storage, and UI. Plugin
PR [#1](https://github.com/yattdev/kandev-plugin-coordinator/pull/1) at
`5bfdbcf7d9608d1210453f95ebfc8f66c3179225` keeps product state in the plugin but
uses dispatch bookkeeping rather than a durable intent/outbox protocol. Both heads
are read-only migration evidence, not branches to extend or close here.

This decision freezes generic public names and invariants so later implementation
units do not invent Coordinator concepts in core. It defines contracts; it does not
implement them.

## Decision

### Ownership and sanctioned call chain

Coordinator identity, policy, scheduling, durable SQLite state, reconciliation,
prompts, reports, namespaced agent tools, and Overview/Chat/Reports/Policy/Audit UI
remain plugin-owned. Kandev owns only generic Host contracts, shared domain
commands and queries, capability approval, audit/event infrastructure, and generic
UI/config primitives.

The only production call chain is:

```text
plugin-managed agent
  -> namespaced plugin-owned agent tool
  -> plugin backend
  -> capability-gated generic Host RPC
  -> shared Kandev domain command or query
  -> repository and event infrastructure
```

An ordinary task agent may reach the same named domain command or query through a
global MCP adapter. Host RPC and MCP request DTOs may differ, but neither adapter
owns workflow, WIP, authorization, ownership, idempotency, lifecycle, relation,
task-run, pending-transition, or audit rules.

A plugin must never integrate through ambient/global Kandev MCP, private or
undocumented REST, direct database access, shelling into Kandev, or prompt/comment
text treated as authority. Core must not gain a Coordinator table, field, profile,
role, principal, grant, setting, tool, or audit vocabulary.

### Contract-wide DTOs and result vocabulary

Every Host request carries `request_id`, `workspace_id`, and
`capability_revision`. Every writer also carries `idempotency_key`, exact target
resource versions, and a `PendingTransitionGuard` that is either
`expect_none` or `{ pending_transition_id, resource_version }`. An
agent-originated writer additionally carries `ManagedExecutionProvenance` with
`conversation_id`, `session_id`, and `execution_generation_token`. The Host
derives installation identity; callers cannot supply or override it.

Every list request carries `Page { limit, cursor, snapshot_version }`; responses
carry `PageInfo { next_cursor, has_more, snapshot_version }`. Cursors are opaque
and bound to workspace, filters, snapshot version, capability revision, and
installation. Every mutable/readback resource carries `resource_version`.

Writers return a typed `HostCommandResult` with one of these stable states:

| State | Meaning |
| --- | --- |
| `APPLIED` | The command committed and the response contains authoritative readback. |
| `ALREADY_APPLIED` | The same idempotency identity already committed with the same canonical digest. |
| `NO_CHANGE` | Preconditions held, but authoritative state already matched the requested outcome. |
| `CONFLICT` | A target version, pending-transition predicate, directive state, or generation changed. |
| `DENIED` | Current approval, provenance, or immutable Human policy rejects the operation. |
| `NOT_FOUND` | The exact workspace-scoped target does not exist or is not visible to the principal. |
| `INVALID` | The request is structurally or semantically invalid. |
| `UNSUPPORTED` | The Host version or provider cannot satisfy the named contract. |
| `RATE_LIMITED` | Provider rate budget prevents the operation; reset metadata is present when known. |
| `UNAVAILABLE` | A required service is temporarily unavailable and no side effect is claimed. |
| `PARTIAL` | Only a contract that explicitly defines durable progress may use this; completed receipts are included. |

`HostCommandResult` also carries `audit_id`, `result_resource_version`, and an
authoritative readback or readback reference. Stable conflict reasons are
`STALE_CAPABILITY_REVISION`, `STALE_RESOURCE_VERSION`,
`PENDING_TRANSITION_CONFLICT`, `EXECUTION_GENERATION_FENCED`,
`DIRECTIVE_REVOKED`, `IDEMPOTENCY_MISMATCH`, and `HUMAN_RESERVED`.
Transport status codes may classify the same outcome, but clients branch on the
typed result and tolerate unknown enum values as `UNSUPPORTED`.

### Public Host surface inventory

The wire service remains `kandev.plugin.v1.Host`. The namespace column is the
permanent SDK shape; the RPC names in the method column are permanent wire names.
`HostReadReceipt` identifies the installation, workspace, capability revision,
request, and snapshot read. `HostAuditReceipt` additionally records before/after
versions, idempotency identity, provenance when present, and the command result.

| Unit | Permanent service / namespace | Permanent method names | Request DTO | Response and result states | Page, version, idempotency, and preconditions | Declared capability | Audit / event identity | Shared domain owner |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| H1 managed conversations | `Host.ManagedAgentConversations` | `EnsureManagedAgentConversation` / `Ensure`; `DispatchManagedAgentConversation` / `Dispatch`; `GetManagedAgentConversationStatus` / `Status`; `ListManagedAgentConversations` / `List`; `DeleteManagedAgentConversation` / `Delete` | `EnsureManagedAgentConversationRequest`, `DispatchManagedAgentConversationRequest`, `ManagedAgentConversationStatusRequest`, `ListManagedAgentConversationsRequest`, `DeleteManagedAgentConversationRequest` | `ManagedAgentConversationResult`: common states plus `CONFIGURATION_REQUIRED`, `BUSY`, `FENCED`, and descriptor/status readback | List uses `Page`; every descriptor has `resource_version`; writers carry idempotency, capability revision, exact descriptor version, transition guard, and generation token where continuing a run | `api_read:managed_agent_conversations`; `api_write:managed_agent_conversations` | `HostReadReceipt`; `HostAuditReceipt`; `managed_agent_conversation.*` with `audit_id` | `ManagedAgentConversationQuery` and `ManagedAgentConversationCommand`; H1 |
| H1 generic chat UI | `host.ui.WorkspaceAgentChat` | `WorkspaceAgentChat(props)` | `WorkspaceAgentChatProps { workspaceId, conversationId, resourceVersion, readOnly?, onStatus? }` | Host-owned loading, ready, configuration-required, fenced, error, and read-only states | The component consumes H1 versions and never receives authority through props | No new capability; backing H1 calls remain gated | UI lifecycle generation and H1 receipts | Host UI primitive; H1 |
| H2a topology and runtime projections | `Host.Workspaces`, `Host.Workflows`, `Host.Tasks`, `Host.Sessions`, `Host.Interactions` | `ListWorkspaces`; `ListWorkflows`; `ListWorkflowSteps`; `ListTasks`; `GetTask`; `ListSessions`; `ListPendingInteractions`; `GetInteraction` | Existing filters become workspace-explicit and add `Page`; exact getters use workspace plus id | Bounded projection DTOs with per-item `resource_version`; pending interactions preserve their typed terminal states | Every list uses page/snapshot binding; no read filter grants authority | Existing `api_read:workspaces`, `api_read:workflows`, `api_read:tasks`, `api_read:sessions`, `api_read:interactions` | `HostReadReceipt`; no mutation event | `WorkspaceTopologyQuery`, `TaskProjectionQuery`, `SessionProjectionQuery`, `InteractionQuery`; H2a |
| H2b communications | `Host.Messages`, `Host.TaskInbox`, `Host.TaskDirectives` | `ListSanitizedMessages`; `ListTaskInbox`; `GetTaskDirective`; `ListTaskDirectives` | Workspace/task/session filters, sanitization level, directive id, `Page` | Sanitized message, durable inbox item, and directive status/readback; terminal directive states remain queryable | Snapshot-bound pages; items carry resource versions and canonical digests | `api_read:messages`, `api_read:task_inbox`, `api_read:task_directives` | `HostReadReceipt`; no mutation event | `TaskCommunicationQuery`, `TaskInboxQuery`, `TaskDirectiveQuery`; H2b |
| H2c relations and pending transitions | `Host.TaskRelations`, `Host.TaskTransitions` | `GetTaskRelations`; `ListTaskRelations`; `ListPendingTaskTransitions` | Exact task/workspace filters plus `Page` | Relation graph with edge versions and child/dependency closure; pending transition with predicate and resource version | Snapshot-bound pages; versions are consumable by H3b/H3c/H5 | `api_read:task_relations`, `api_read:task_transitions` | `HostReadReceipt`; no mutation event | `TaskRelationQuery`, `TaskTransitionQuery`; H2c |
| H2d exact change evidence | `Host.ChangeRequests` | `GetChangeRequestEvidence`; `ListChangeRequestEvidence` | Canonical provider, workspace, repository, base, head, immutable head/merge-ref identity, and `Page` for the result or a named nested collection | Exact checks/jobs, review, thread, mergeability, divergence, rate budget, and receipt projections described below | Snapshot/resource version binds provider, repository, base, immutable head, merge ref, provider connection generation, and nested cursors | `api_read:change_requests` | `HostReadReceipt` plus provider read receipt | `ChangeEvidenceQuery`; H2d |
| H2e terminal provenance | `Host.TerminalResources` | `GetTerminalResourceProvenance` | Exact workspace/task, optional closure depth, and `Page` plus section key for every nested collection | Canonical change disposition, resources, consumers, closure, interactions, cleanup ownership, and prior receipts described below | One exact provenance `resource_version`; every nested page binds to it; no cleanup authority is implied | `api_read:terminal_resources` | `HostReadReceipt`; no mutation event | `TerminalResourceQuery`; H2e |
| H3a messaging and directives | `Host.Messages`, `Host.TaskDirectives` | `SendMessageExact`; `IssueTaskDirective`; `ResolveTaskDirectiveExact` | `SendMessageExactRequest`, `IssueTaskDirectiveRequest`, `ResolveTaskDirectiveExactRequest` | Common results; directive readback includes pending, resolved, revoked, expired, and generation-fenced states | Capability revision, idempotency, target/session versions, transition guard; agent origin adds managed execution provenance; directive semantics are frozen below | `api_write:messages`, `api_write:task_directives` | `HostAuditReceipt`; `message.dispatched`; `task_directive.issued|resolved|revoked` | `MessageCommand`, `TaskDirectiveCommand`; H3a |
| H3b task writers | `Host.Tasks` | `CreateTaskExact`; `MoveTaskExact`; `UpdateTaskExact`; `SetTaskLabelsExact`; `SetTaskFlagsExact` | Exact create/placement, move, field-mask, label, and flag requests | Common results with authoritative task readback | Capability revision, idempotency, exact task/workflow/step versions as applicable, transition guard, optional managed execution provenance | `api_write:tasks` | `HostAuditReceipt`; existing `task.*` events gain `audit_id` | `TaskCommand`; H3b |
| H3c relation writers | `Host.TaskRelations` | `AddTaskRelationExact`; `RemoveTaskRelationExact` | Exact workspace, source, target, relation kind, edge/endpoint versions | Common results with authoritative relation-graph readback | Capability revision, idempotency, endpoint and edge versions, transition guard, optional managed execution provenance | `api_write:task_relations` | `HostAuditReceipt`; `task_relation.added|removed` | `TaskRelationCommand`; H3c |
| H3d provider actions | No Host writer is approved in H0 | No RPC or SDK method is reserved | None | `UNSUPPORTED` if a plugin assumes one | None | None | None | Deferred independently as recorded below; H3d |
| H4 exact execution lifecycle | `Host.TaskRuns`, `Host.Sessions` | `EnsureTaskRunExact`; `RecoverSessionExact` | Exact task/workstep/session, desired execution identity, prompt/action digest, dispatch idempotency, expected versions | Common results plus one admissible `execution_generation_token` and authoritative run/session readback | Capability revision, idempotency, exact task/workstep/session/run versions, transition guard; replacement must fence the prior generation first | `api_write:task_runs`, `api_write:sessions` | `HostAuditReceipt`; `task_run.ensured`; `session.recovered`; generation-fenced lifecycle events | `TaskRunCommand`, `SessionRecoveryCommand`; H4 |
| H5 pending transitions | `Host.TaskTransitions` | `CancelPendingTaskTransitionExact` | Exact transition id/version and cancellation reason | Common results plus optional single-use continuation token and authoritative readback | Cancel requires capability revision, idempotency, exact transition version, and current target version | `api_write:task_transitions` | `HostAuditReceipt`; `task_transition.cancelled` | `TaskTransitionCommand`; H5 |
| H7 generic config and UI | manifest config schema, `Host.GetConfig`, `host.ui`, plugin registry | formats `agent-profile` and `textarea`; numeric `minimum`/`maximum`; `WorkspaceAgentChat`; `registerIntegrationSettings`; Integrations navigation | JSON Schema config and existing registry DTOs | Typed validation/configuration-required states; revocable registration handles | Config revision and UI lifecycle generation; backend access still uses its own capability revision | No new capability merely for rendering/config; each backing API remains gated | Config revision and owner lifecycle generation; no domain event | Plugin config/registry owners; H7 |

No public Host name in this inventory depends on any product plugin being
installed.

### H2d exact-head evidence

`ChangeRequestEvidence` contains canonical provider ID and connection generation;
workspace; immutable repository ID plus canonical owner/name; base branch and base
commit; head branch and immutable head commit; immutable merge-ref identity when the
provider exposes one; draft, mergeability, conflict, and base-divergence states;
every exact check-run or job identity, status, conclusion, URL, and timestamps; causal
relationships from a check/job to its aggregate; review decision; requested and
completed reviewers; unresolved actionable thread count; provider rate budget and
reset; notification receipts; provider-action receipts; snapshot/resource version;
and provider read receipt. Aggregate green is never evidence that each causal job
passed, and an updated head invalidates evidence and action receipts from the old
head.

### H2e terminal resource provenance

`TerminalResourceProvenance` contains canonical change and commit disposition;
pushed, contained, unpushed, and orphan history; owned worktree; local and remote
branches; active sessions, turns, processes, runtimes, data, and artifact consumers;
child and dependency closure; pending interactions and transitions; cleanup owner;
prior preview/action receipts; and its resource version. Done placement or merged
status never grants cleanup authority.

Any later cleanup writer is a separate capability and command. It must be exact,
provenance-bound, policy-gated, dry-run capable, idempotent, auditable, and followed
by authoritative readback. Until that writer is separately approved, a plugin may
only report a Human cleanup action.

### C1: installation authority and execution provenance

The Host principal is `PluginInstallationPrincipal { installation_id,
workspace_id, capability_revision }`. It authorizes deterministic scheduler,
startup repair, reconciliation, event recovery, and outbox retry without requiring a
live managed conversation.

An agent-originated write additionally binds the exact managed conversation,
session, and execution-generation fencing token. This provenance proves which
execution requested an operation; it never creates, delegates, or widens installation
authority. A missing, revoked, or stale approval revision is `DENIED`; a stale
execution token is `CONFLICT/EXECUTION_GENERATION_FENCED`.

### C2: atomic pending-transition safety

Every writer that can wake, redirect, message, create work for, relate, or transition
a task accepts an exact `PendingTransitionGuard`. Its shared domain command
atomically revalidates the guard in the same transaction or serialized critical
section as the side effect. A separate list followed by send, move, ensure, or update
is not a safety proof.

`CancelPendingTaskTransitionExact` may return a single-use continuation token bound
to the cancelled transition, target versions, installation, capability revision,
idempotency identity, and expiry. A following writer either consumes that token
atomically or revalidates the now-current pending-transition predicate itself.

### C3: non-amplifying TaskDirectives

A `TaskDirective` is bound to installation, capability revision, workspace, exact
task, exact capability class, canonical instruction/action digest, issue time,
expiry, revocation state, single-resolution state, optional task/workflow/session-
generation preconditions, stable idempotency identity, and Host audit ID.

Worker admission intersects all of those fields with the current installation
approval and immutable Human-reserved policy. A directive cannot delegate itself,
widen through prose, authorize another operation, survive capability revocation,
resolve twice, or replay into a replacement execution generation. Failed admission
returns a typed denial or conflict and never partially dispatches.

### C4: execution-generation fencing and durable dispatch

`EnsureTaskRunExact` and `RecoverSessionExact` establish exactly one admissible
task/workstep execution generation and return its fencing token. Messages,
directives, prompt admission, completion, and transition signals for that run require
the token. Replacement terminally fences the previous generation before dispatch.

The plugin persists an action intent/outbox record before any Host dispatch. It then
records the Host acknowledgment, `audit_id`, result, and authoritative readback.
Recovery retries the same idempotency identity. Failed-session unread queues are
diagnostic evidence, not durable work state.

### C5: terminal provenance before cleanup

H2e is the sole generic summary of terminal resources and consumers. Cleanup is never
inferred from a workflow column or change-request state. A missing provenance field,
active consumer, stale resource version, or unknown ownership causes a Human cleanup
action, not optimistic deletion.

### C6: exact-head provider evidence

H2d binds every provider observation and future action receipt to canonical
provider/repository/base/head and immutable head/merge-ref identity. Any future H3d
command is a separate capability with exact-head, current-state, rate-budget, and
idempotency preconditions. Merge is not a Host writer in this decision.

### C7: permanent implementation decomposition

H2 and H3 are documentation umbrellas only. Permanent implementation/task/PR units
are H2a through H2e and H3a through H3d. Each has one owner/worktree/PR unless source
evidence proves two shapes inseparable. No implementation unit may recreate a
Coordinator-specific core concept to avoid one of these generic boundaries.

### Capability model and H6 decision

Decision: **`H6_REQUIRED`**.

Current source can enforce only a manifest declaration copied into a global installed
record. It cannot represent the principal or approval revision required above.

| Source evidence | What exists | Why it is insufficient |
| --- | --- | --- |
| `apps/backend/internal/plugins/manifest/manifest.go` (`Capabilities`) | Manifest-owned `events`, `api_read`, `api_write`, and boolean declarations | A package declares its own ceiling; there is no Human-approved workspace set, revision, or revocation record. |
| `apps/backend/internal/plugins/store/store.go` (`Record`) | One filesystem installation record embeds the manifest and runtime status | The record is instance-global and has no workspace approval, approval history, actor, or immutable audit identity. |
| `apps/backend/internal/plugins/service_install.go` (`Service.Install`) | Admin-only install validates and saves the package manifest | Install does not persist a reviewed capability set distinct from the package declaration; an upgrade replaces the embedded manifest. |
| `apps/backend/internal/plugins/service.go` (`hostForPlugin`) | A Host instance copies `rec.Capabilities` at spawn/restart | Authorization knows plugin ID and declared capabilities only, not workspace, current approval revision, revocation, or execution provenance. |
| `apps/backend/internal/plugins/host.go` and `host_data.go` | Per-method declaration checks return `PermissionDenied` | The checks do not intersect an operator approval or produce a generic Host audit receipt. |
| `apps/backend/internal/task/models/models.go` (`PermissionResolutionAudit`) | Narrow audit for agent permission resolution | It is not a generic plugin command/audit ledger and cannot identify installation capability revisions. |

The smallest extension is owned by the plugin system:

| Contract | Required shape |
| --- | --- |
| `PluginCapabilityApproval` | One current row per `(installation_id, workspace_id)` with monotonic `revision`, manifest capability digest, approved capability IDs/classes, status `active|revoked`, approving Human actor, issued/updated timestamps, and immutable Human-policy version. |
| `PluginCapabilityApprovalEvent` | Append-only grant, narrow, revoke, and upgrade-review events with `audit_id`, before/after revisions and digests, actor, reason, and timestamp. |
| Authorization | The effective set is `manifest declaration ∩ current workspace approval ∩ immutable Human policy`. The request revision must exactly equal the active revision. |
| Revocation | Revocation increments the revision, denies new calls immediately, fences outstanding directives/tokens, and makes retry/readback return typed stale-revision results. |
| Upgrade | Equal or narrower declarations retain the approved set at a new recorded manifest digest. Any widening remains unavailable until a Human approves a new revision; activation fails closed for required unavailable capabilities. |
| Compatibility | Existing installed plugins retain their currently shipped v1 capabilities through a synthetic legacy revision only for already-shipped RPCs. Every H1-H5 capability named here requires an explicit workspace approval revision; no legacy declaration implicitly gains it. |
| Audit | Every H1-H5 read receipt and write receipt records installation, workspace, exact approval revision, method/capability, request/idempotency digest, result, resource versions, and managed execution provenance when present. |

No Coordinator principal, role, profile, grant, or audit type is created.

### Human-reserved and non-delegable matrix

“Agent provenance” below means it is required when the request originated in a
managed agent execution. Installation approval is always required even when that
column says no.

| Capability class | Installation approval may authorize | Agent provenance additionally required | Exact preconditions | Human-reserved |
| --- | --- | --- | --- | --- |
| Read-only projections | Yes | No | Workspace, capability revision, snapshot/resource version | No, except secret values and cross-workspace data |
| Managed conversation ensure/status/list/dispatch | Yes | Only for an agent-originated continuation | Conversation/config versions; idempotency; transition guard; generation token for continuation | Delete is reserved when unique/still-needed content is not proven disposable |
| Routine messages and task writes | Yes | Yes for agent-originated writes; no for deterministic backend reconciliation | Target/workflow/step versions, transition guard, idempotency, optional continuation token | No, unless the requested field/action enters another reserved row |
| Trusted directive issue/resolve | Yes, as a separately approved class | Yes for agent-originated issue/resolve | Directive digest/state, target versions, generation, expiry, transition guard | Cannot authorize any Human-reserved class |
| Relations and pending-transition cancellation | Yes | Yes for agent-originated writes | Endpoint/edge or transition versions, transition guard, idempotency | Destructive relation effects remain subject to dependency policy; no ToDeploy override |
| Task-run ensure and session recovery | Yes | Yes for agent-originated recovery; no for deterministic repair | Exact task/workstep/session/run versions, generation fencing, transition guard, outbox idempotency | No, but replacement must fence rather than rewrite history |
| Provider retry/rerun, draft-to-ready, reviewer notification | Not in H0 | N/A | Would require exact head, current state, idempotency, rate budget, audit, and readback | Deferred; no authority exists |
| Removal of unique or still-needed state | No | N/A | Exact provenance, dry-run receipt, Human confirmation, current versions | Yes |
| Security/trust-boundary changes | No | N/A | Human-authenticated exact change and policy version | Yes |
| Secret/credential disclosure or scope expansion | No | N/A | Human-authenticated secret scope and audit | Yes |
| Cross-workspace access | No | N/A | Separate Human approval per workspace; no wildcard delegation | Yes |
| Force-push, rebase, squash, amend, or history rewrite | No | N/A | No Host writer defined | Yes |
| Merge | No | N/A | No Host writer defined | Yes |
| Release | No | N/A | No Host writer defined | Yes |
| Deploy | No | N/A | No Host writer defined | Yes |

### H3d provider-action decision

H3d is explicitly deferred. Existing provider-neutral UI callbacks do not satisfy a
durable backend command boundary, and the built-in GitHub writer does not provide the
exact-head/idempotency/audit contract required here.

| Candidate action | Decision | Source evidence and missing invariant |
| --- | --- | --- |
| Retry or rerun | Deferred | `apps/backend/internal/github/client.go` exposes check reads but no provider-neutral check/job rerun command. Providers differ on workflow, pipeline, job, and check-suite identity, so “rerun failed” has no stable target or causal readback. |
| Draft to ready | Deferred | `apps/packages/plugin-sdk/src/index.ts` exposes `supportsDraft` and draft creation only. There is no shared backend transition command, exact-head/current-draft precondition, or provider receipt. |
| Reviewer notification or re-request | Deferred | `apps/backend/internal/github/service_pr.go` has GitHub-specific `RequestReviewersForWorkspace`, but it accepts repository/number/reviewer inputs without immutable-head binding, once-per-head idempotency, generic provider semantics, Host audit receipt, or authoritative provider readback. |

No capability string, RPC, or provider action is reserved for these candidates. A
later H3d ADR may include an action only after defining provider-neutral target
identity, immutable head/current state, idempotency, notification-once-per-head when
applicable, rate-budget behavior, audit receipt, and authoritative readback. Merge is
excluded.

### Versioning and compatibility

- `kandev.plugin.v1` and public SDK DTOs evolve additively. Removing/renaming fields,
  changing field meaning, or reusing enum numbers requires a new protocol version.
- Unknown fields are ignored and preserved where the runtime permits. Unknown enum or
  result values map to typed `UNSUPPORTED`; they never fall through to success.
- Capability IDs and result strings are permanent once shipped. A retired capability
  identity is never reused for another meaning.
- The plugin manifest's existing `min_kandev_version` is the declaration consumed by
  the Host. A plugin that needs this surface sets it to the first stable Kandev release
  that implements every required capability. H0 does not guess that future version.
- Plugin upgrade follows the H6 revision rules above. A missing required capability,
  stale approval revision, or older Host yields `UNSUPPORTED`/`DENIED`; the plugin
  parks the action and reports a configuration/upgrade requirement. It must not fall
  back to MCP, private REST, SQL, or shell.
- Host RPC and global MCP parity tests instantiate both adapters over the same named
  domain query/command, assert the same authorization and conflict verdicts, and
  compare authoritative readback. Consumer-specific DTO differences are allowed only
  outside those invariants.
- Events are at-least-once hints with `event_schema_version`, `resource_version`,
  `audit_id`, installation/workspace/capability revision, and immutable resource
  identity. Unknown events may be ignored. Dropped, duplicated, delayed, or reordered
  events are repaired by authoritative Host readback; event text is never authority.
- Rollout order is H6 approval/audit foundation, then required read contracts, then
  exact writers, then the plugin adapters/UI. The plugin stays degraded and performs
  no unsafe write when any required Host capability is absent.
- Exact-version integration I1 records accepted Host and plugin commit IDs, their
  declared minimum/actual versions, capability revision, and compatibility-test
  receipts. A moving branch or aggregate “latest” version is not evidence.

### Migration and disposition ledger

H0 does not close, message, move, rewrite, repurpose, or mutate any listed task or PR.

| Evidence identity | Disposition and reusable evidence | Replacement owner / sequencing |
| --- | --- | --- |
| Task `9e67c426-1300-46ef-a00f-e5603791212d`; host PR #2793 at `afd2b699bfe9b6af9353ea01728582f61a7be2be` | Keep as monolithic read-only migration baseline. Extract conversation proto/SDK/service/test patterns from commits `e7ea57a1`, `e13ca6cd`, `76e44a74`, `9d2e0733`, `ad843976`, and `4baa37d8`; supersede Coordinator authority/storage/UI (`internal/coordinator`, workspace-agent principal/grant shapes). | H1 owns conversations; H2c owns relations; H6 owns generic approval/audit. Extract only after H0. |
| Plugin PR #1 at `5bfdbcf7d9608d1210453f95ebfc8f66c3179225`; tasks `52892e8e-dc44-4d38-80ab-14bb75f7b6bf`, `3ec598a8-b49d-4d5f-9bf3-52b0d611cd32` | Keep plugin/recovery baseline. Preserve `server/coordinator/{config,prompt,reports,state,tools}.go` and `ui/src/**` as plugin-owned evidence. Supersede `scheduler.go` dispatch bookkeeping with C4 intent/outbox and Host readback. | P1/P2 consume stable H1/H2 contracts; plugin recovery follows exact accepted Host head. |
| PR #2756 | Keep scheduler history (`internal/scheduler/cron`, session-wake repository/service/tests). Do not close until a linked plugin scheduler replacement exists. | Plugin scheduler/P1 after H0 and stable Host reads. |
| Task `c642d57a-5a24-48ca-8f85-57d31115eeb5`; PR #2909 | Retain generic automatic exact-turn repair and durable delivery evidence in lifecycle, orchestrator message queue, completion intent, and stale-session tests. | H4 extracts explicit `TaskRuns.EnsureExact` / `Sessions.RecoverExact`; it does not replace generic repair. |
| Task `9349b6e5-a167-4d88-af14-cb355015e3dd`; PR #2841 | Extract relation projection/service/wire tests (`host_task_relations.go`, `handoff_access.go`, related-task authorization tests). Supersede Coordinator-specific MCP authorization. | H2c, then H3c. |
| Task `ec384aac-cd4f-469c-8893-3aa8383da9d6`; PR #2974 | Retain task-inbox queue identity, bounded polling, transition, and trusted-principal evidence (`list_task_inbox`, messagequeue repository/service/tests). Do not reuse task-specific mutation authority. | H2b read projection only; H3a later owns exact writers. |
| Task `fa3fba49-2018-460b-a600-adae23b24cc8`; PR #3048 | Security-decision input only. Reuse adversarial provenance/audit test ideas; do not continue `internal/coordinator`, coordinator grants, UI, routes, tables, or MCP authority. | H6 generic plugin/workspace approval and Host audit extension. |
| Task `b2da5061-07a3-46e6-ab48-3881929ac9a5`; merged PR #3147 | Keep platform TTL/orphan invariant and row-local/race-safe pending-move sweep tests. Never recreate it in the plugin. | H2c observes; H5 composes with the retained platform invariant. |
| Task `7056a702-a3c3-4fe8-8535-c6b8d340ef6a`; PR #3155 | Retain exact compare-and-delete transaction history in `pending_move_exact_cancel.go` and its repository/service tests. No H0 mutation or disposition action. | H5 after H2c and this capability identity; C2 applies to every following writer. |
| Task `531a41cd-57ef-495a-8dfa-614d2a4d0d52` | Preserve the supplied Human-owned ToDeploy boundary as generic H3c design input only. Do not inspect or mutate the protected task. | H3c after H2c; no task-specific authority. |
| Task `afdb2ef3-06ca-4cd5-a074-c4e691679da9` | Preserve isolated QA baseline. H0 does not reuse moving version inputs. | Exact-version I1 replaces version inputs on accepted Host/plugin heads. |

### Dependency and delivery graph

1. H0 completes first.
2. H1, H2a through H2e, and H7 may then proceed independently where file
   ownership permits.
3. H3a follows H2a/H2b; H3b follows H2a/H2c; H3c follows H2c; H3d follows H2d
   only after a later action decision.
4. H4 follows H1, H2a, H2b, and H3a directive semantics.
5. H5 follows H2c and H0 capability identity. C2 must reach every affected
   writer before plugin messaging/recovery is safe.
6. P1 follows H0 and stable H1/H2 state shapes. Each P2 adapter follows its Host
   dependency; P3 follows H7 and stable P1 view models.
7. I1 runs only on exact accepted Host/plugin heads and the declared minimum
   version.
8. Every node independently traverses Work, Review, QA, PR/CI, and semantic
   Human-QA. None of those phases authorizes merge, deploy, release, or ToDeploy.

## Consequences

- The Coordinator remains independently releasable and core gains only reusable
  plugin contracts.
- H6 is mandatory before any H1-H5 capability named here can authorize a write.
- The public contract is larger and intentionally exact: resource versions,
  transition predicates, generation tokens, audit receipts, and readback add work to
  every adapter but prevent split-brain automation.
- H3d has no implementation unit until provider-neutral semantics exist.
- Existing Host v1 behavior remains compatible for existing plugins, but it does not
  implicitly grant the new surface.

## Alternatives Considered

### Continue the monolithic host PR

Rejected because it makes product identity, grants, scheduling, and UI core concepts
and couples Coordinator delivery to a Kandev release.

### Let the plugin call global MCP or private REST

Rejected because ambient authority, consumer-specific authorization, and network
topology bypass installation capability revisions and cannot guarantee command
parity.

### Treat the manifest as Human approval

Rejected because a package authors its own manifest, upgrades can widen it, and the
current record has no workspace revision, revocation history, or generic audit.

### Use list-then-write concurrency checks

Rejected because another writer can change the pending transition, run generation,
or target resource between calls.

### Approve provider actions now

Rejected because current source does not prove stable provider-neutral identities or
exact-head/idempotency/audit semantics for any of the three candidates.
