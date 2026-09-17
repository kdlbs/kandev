---
status: draft
system: orchestration
requirements:
  - REQ-ORCHESTRATION-ASSISTANT-001
  - REQ-ORCHESTRATION-ASSISTANT-002
  - REQ-ORCHESTRATION-ASSISTANT-003
  - REQ-ORCHESTRATION-ASSISTANT-004
  - REQ-ORCHESTRATION-ASSISTANT-005
  - REQ-ORCHESTRATION-ASSISTANT-006
  - REQ-ORCHESTRATION-ASSISTANT-007
  - REQ-ORCHESTRATION-ASSISTANT-008
  - REQ-ORCHESTRATION-ASSISTANT-009
  - REQ-ORCHESTRATION-ASSISTANT-010
---

# Personal assistant completion design

## Boundary and current implementation

This design completes the existing `internal/orchestration` assistant. Ownership,
durable intake and objectives already exist; context/credential work is partial.
Capability discovery, enforced inspect mode, attention, native resolution, the
app-level shell, maintenance and workspace grants remain pending. The
[workspace Coordinator view](coordinator-view.md) is a separate observation UI
that may ship first. Its grouped task list is not an assistant attention ledger.

Use core tasks/sessions/comments, `internal/runs`, `internal/clarification`,
runtime authentication and current MCP/plugin dispatch. `backendapp` composes
ports; Orchestration does not import Office. Existing workflow gates, provider
accounts and ownership remain authoritative.

| Requirement | Design section |
| --- | --- |
| REQ-ORCHESTRATION-ASSISTANT-001 | Existing invariants; Public contracts |
| REQ-ORCHESTRATION-ASSISTANT-002 | Context completion |
| REQ-ORCHESTRATION-ASSISTANT-003 | Capability directory |
| REQ-ORCHESTRATION-ASSISTANT-004 | Execution authority |
| REQ-ORCHESTRATION-ASSISTANT-005 | Attention projection |
| REQ-ORCHESTRATION-ASSISTANT-006 | Native resolution and controls |
| REQ-ORCHESTRATION-ASSISTANT-007 | Frontend |
| REQ-ORCHESTRATION-ASSISTANT-008 | Maintenance |
| REQ-ORCHESTRATION-ASSISTANT-009 | Workspace grants |
| REQ-ORCHESTRATION-ASSISTANT-010 | Rollout isolation |

## Existing invariants

Retain the owner on `orchestration_conversations`, independently of the mutable
default binding. Binding CAS and ownership claim remain one transaction. Core
task/session/configuration authorization continues even with every feature off.
An inactive private conversation is readable by its owner but accepts no turns.

Intake keys bind owner/conversation/client message identity and payload digest;
comment plus receipt/outbox persist atomically. The dispatcher attaches exact
source comment, intent revision, owner/binding version and context references.
Duplicate requests return the receipt, not another action. Agent run/session
credentials remain fresh, scoped and revocable after finish or binding changes.

Objectives retain acceptance revisions, mode, authorized target scope and typed
task/evidence links. Completion requires settled relevant sessions and current
evidence, not a model claim or board status alone. Operation state remains
prepared → dispatched → acknowledged/failed/unknown. Restart reconciles native
receipts and does not replay uncertain writes.

## Rollout isolation

Add `features.personalAssistant` / `KANDEV_FEATURES_PERSONAL_ASSISTANT` using the
typed runtime flag registry and existing override precedence. The candidate key
is unused in the active/retired registry at the planning baseline. Verify it again
before implementation. Default prod/dev/e2e false; document restart requirement
according to composition lifecycle. Overall enabled state is Orchestration AND
Personal assistant. Existing Office remains independent.

Gate human assistant selection/mutations, runtime assistant endpoints/CLI/tool
exposure, assistant intake dispatch, callbacks/attention wakes and launch. Do not
gate required schema construction, retained-owner checks or core privacy guards.
Do not fall back to a legacy coordinator launch for an owned assistant conversation.
Keep owner-authorized historical reads available through existing protected
conversation paths. Flag-off selection/configuration requests return the existing
unavailable style rather than secretly creating a binding.

The coordinator-only live pilot uses Orchestration on and Personal assistant off.
Assistant adoption is a later milestone after context, authority, attention,
resolution and UI checks pass. Records already created by the prototype remain
preserved even when the assistant is disabled.

## Context completion

Extend the existing typed descriptor response with response-only, server-owned validation
metadata: `validation_status`, optional `validated_at`, and a bounded
`availability_reason`/`unblock_action`. Unknown or never-checked metadata is not
“ready.” The health adapter remains metadata-only and has no reveal or vault-list
method. Resolver availability must come from actual attached capability evidence;
do not claim an external vault integration exists because its name is recognized.

Validation results bind descriptor ID/revision/profile/resolver reference and
the configuration generation checked by the health reader. Derive them on read;
do not persist a historical ready flag or add a credential-secret cache. A
timestamp is present only when an actual metadata validation occurred during that
read. Existing descriptors with no available validator remain unknown or
unavailable. A transient health failure is not a permanent descriptor deletion;
user payloads cannot forge validation fields. This completion needs no new SQL
column. Any later persisted validation cache requires a separate invalidation
design and SQLite/PostgreSQL migration evidence.

Complete owner-list pagination and explicit workspace/project/task/environment
scope checks. Cursor binds owner, filter and stable ordering; do not encode raw
memory content. Missing/foreign scope IDs return 404/422 consistently before a
write. CAS conflicts leave the original content intact. Test confirmation,
expiry, correction and forgetting across pages and owners.

Keep the current 12-KiB packet and 1-KiB excerpt contracts. Mandatory account/
objective/user constraints may reject overflow; optional context reports omissions
with continuation references. Queue messages retain their own context reference;
merging/send-now cannot borrow newer task metadata to authorize stale input.
Preserve launch/resume/steer/PTY guards already present in native execution.

## Capability directory

Introduce a read-only `CapabilityReader` port assembled in `backendapp` from
native operations, integration health/configuration metadata, installed plugin
agent tools, attached MCP metadata and enabled execution profiles. Entries are
typed descriptors: stable ID, origin, scope, schema reference, applicability,
effect class, health/reason and source generation. Bound a page to 100 entries
and reject cursors from another owner/workspace/generation.

Use an allowlisted DTO. Never serialize integration secrets, raw environment
variables, MCP command arguments containing credentials or complete plugin config.
Unknown effect/health remains unknown. Treat descriptions and schemas as untrusted
data; they cannot override runtime policy. Lookup pages are observations, not grants.

The MCP profile reuses `SurfaceConversation`. Plugin manifests with API version
2 may explicitly add the conversation surface through native validation and
registration. Task-only declarations retain their scope; compatibility tests
cover both the old tools and the new opt-in. Keep individual tool schemas and
existing dispatch names; do not introduce an unrestricted generic invoke-plugin
endpoint. Discovery invalidation cannot replace invocation-time revalidation.

## Execution authority

Resolve effective authority server-side as the intersection of owner/workspace
access, binding/grant, current intent, selected profile/executor, mode, capability
effect and native approval gates. Put the result in immutable run context with
revision identities, not in caller-provided tool names or prompt text. Every
native/MCP/plugin invocation checks the current authoritative state again.

Maintain a support matrix per provider/executor path: which native tools can be
disabled or constrained, which remote MCP endpoints enforce effects, and what
evidence proves it. Inspect admission succeeds only for a proven path. If an
agent can still reach unrestricted shell, network-backed tools or an ungoverned
MCP attachment, inspect launch returns an unsupported-policy error before any
process is started. Do not fall back to the same unrestricted agent with a
read-only instruction. Supporting a new provider requires current adapter/docs
verification and negative execution tests; this plan does not guess CLI flags.

Internal receipt/audit writes are an explicit narrow class. Repository, task,
external-service, plugin-configuration and credential mutations are denied in
inspect. Tests use native/plugin/MCP/provider stubs with mutation counters and
untrusted text attempting to change mode or grant scope. Unknown tool effects
are denied. Execute/design mode still follows native gates and operation receipts.

## Attention projection

Use a new Orchestration-owned attention record keyed by binding, source identity
and kind; store source revision, task/session/workspace IDs, bounded redacted
summary, state, last-notified revision and resolution evidence. Supported kinds
are question, permission, authentication, failure, review and result.

Read authoritative pending requests and all relevant task sessions through
backendapp adapters, using task status summaries as hints rather than substitutes
for request identity. Managed-work scope comes from objective/task links and
explicit adoption. Do not scan unrelated workspace or installation work.

Project source snapshots deterministically. Source revision advances supersede
old records; expiry/cancellation does not mean answered. Subscribe to task,
session, clarification, permission, turn/result and integration health events.
Batch bursts by identity and use injected clocks. Persist attention updates and
a wake outbox in one transaction; the existing run queue deduplicates the
binding/source-revision key. Queue consumption rechecks flag/pause/intent/grants.

Reconcile managed sources at startup and every 60 seconds in bounded batches.
Unchanged reads produce no write/wake/model turn. Live events target five-second
visibility under healthy local conditions. Backoff/deduplicate repeated errors;
report partial/inaccessible inputs instead of inventing blocked reasons. Provide
counts/timing for reconciliation and deduped wakes without logging prompt bodies.

## Native resolution and controls

Reuse the native clarification resolver and permission-response execution
boundary. An attention card contains a reference to a current native request,
not a copied approval capability. Load the canonical request immediately before
resolution and verify task/session/turn/request/revision and current human actor.

Separate human resolve and runtime answer endpoints. Human choices preserve
native option schemas. Runtime answers require a delegable question, confirmed
scoped provenance and a stable operation ID; permission kinds always reject runtime
callers. Never send a generic worker message to simulate approval. Native source
events drive both UIs after success. Stale/already-resolved/expired/unknown delivery
return distinct receipts without double submission or resurrected handles.

`pause` affects new assistant dispatch only. `stop_managed_work` is a human action
that enumerates currently managed worker targets, checks access and invokes native
stop once per target with individual receipts; it does not stop unrelated tasks.

## Public contracts

Existing paths stay under `/api/v1/orchestration`; new contracts must preserve
existing shapes unless explicitly extended. Proposed routes:

| Consumer | Route | Contract |
| --- | --- | --- |
| Human | GET/PUT `/assistant` | Owner-selected binding; CAS `expected_version`; existing conversation identity. |
| Human | GET `/assistant/objectives`, `/assistant/attention` | Bounded cursor summaries and current revisions. |
| Human | POST `/assistant/attention/:id/resolve` | Operation ID, expected revision and typed native answer/permission option. |
| Human | POST `/assistant/control` | Pause/resume/stop-managed-work; binding version and per-target receipts. |
| Human | GET/PUT/DELETE `/assistant/memory/:id`, `/assistant/credentials/:id` | Existing owner/CAS contract; list routes are paginated and scope validated. |
| Human | GET/PUT/DELETE `/assistant/workspaces/:workspaceId` | Explicit grant/revoke, allowed operations, receiving profile and expected version. |
| Runtime | GET `/runtime/capabilities` | Entries, source generation/revision and scope-bound next cursor. |
| Runtime | GET `/runtime/attention`, `/runtime/objectives`, `/runtime/context/:objectiveId` | Claimed-run scoped observations/context. |
| Runtime | POST `/runtime/attention/:id/answer` | Delegable non-permission question only; operation and revision identities. |
| Runtime | Existing objective/task mutation routes | Recheck exact intent, mode, operation and context references. |

Existing comment intake adds `client_message_id`; first acceptance is 201,
identical retry 200 and changed payload under the same ID 409. Legacy requests
may get generated identities but cannot claim client retry deduplication.
Use 401 invalid credentials, 404 inaccessible resources, 403 forbidden authority,
409 stale/conflicting identities, 422 unsupported policy, 503 unavailable inputs.

## Frontend

Add `/assistant` with owner binding setup, shared coordinator conversation,
objectives/evidence, attention, capabilities and scoped memory panels. Keep
workspace Coordinator as the workspace task-observation destination. Extract
native question/permission presentations behind explicit transport interfaces;
do not copy resolution logic into a second component tree.

Client state keys include owner/binding/version/workspace. Accepted messages keep
stable IDs through retry, streams and cursor-based reconnect. Render delivery,
run activity and objective completion as separate states. Abort stale loads on
identity changes and clear prior content synchronously. Keep desktop/mobile
controls equivalent, with deliberate disclosure and one scroll owner per pane.
Every new user-facing string uses the existing locale pipeline.

## Maintenance

Normalize incidents into an Orchestration-owned redacted record with origin,
operation/capability, execution identity, policy/classifier version if known,
request identity, reason, resolution and result. Fingerprint by workspace,
account, policy version and normalized cause; do not group raw prompt text.

A unique candidate appears after three matching incidents on two tasks within
seven days. A candidate is evidence for investigation, never a permission grant.
The human maintenance grant specifies repository/workspace/profile and permitted
local actions. Reuse objective/routing/receipts to create or adopt one isolated
normal task. Record positive and negative regressions and a local commit. No
automatic push, PR, merge, production restart, deployment or policy broadening.

## Workspace grants

Persist binding/workspace/owner, allowed operation set, receiving profile,
revision and revoked timestamp in Orchestration storage. Grant/revoke is human
only and checks current native workspace access. Home scope is explicit; linked
scope never overrides native membership. A profile change invalidates incompatible
export grants rather than silently sending context to another account.

Use a separate assistant broker credential binding owner, binding, run/session
and target grant revision. Do not broaden the meaning of `workspace_coordinator`
JWTs. Recheck target grant before every read, wake, operation and context export;
revoke invalidates queued work. Export bounded task/evidence references and scoped
context, not wholesale foreign transcripts. UI names the receiving profile and
the limits of forgetting already-delivered content.

## Storage, verification and release

New attention/outbox, friction/candidate and workspace-grant tables belong to the
existing Orchestration store, not a plugin SQLite file. Use additive, replay-safe
migrations; keep required-store startup independent of feature flags. Extend the
fixed conformance adapter with fresh/replay/upgrade assertions on SQLite and
PostgreSQL for every new durable boundary. No destructive downgrade is assumed.

Each work order has named unit/integration tests and exact commands. Combined
browser fixtures include multiple sessions/owners, lost acknowledgements,
duplicate/reordered events, restart, revoke and zero unauthorized mutation counters.
Synthetic provider runs are distinct from optional constrained real-provider
trials. Never use a real vault or private prompt history as a test fixture.

The [delivery plan](../../../plans/orchestration-delivery/plan.md) orders the
coordinator pilot before assistant expansion and defines candidate qualification,
private migration rehearsal, rollback and contribution export.

## Related decisions

- [Runtime ownership](../../../decisions/2026-09-07-workspace-orchestration.md).
- [Retained private ownership](../../../decisions/2026-09-16-private-conversation-ownership.md).

### Implemented memory continuation and validation details

Owner memory pages use stable ID order, a default of 50 and a cap of 100, with
continuations bound to owner, binding version and scope filters. Expired and
forgotten rows are excluded before paging. Native adapters validate repository,
task and environment references; environment-only memories resolve the native
environment's task before checking its workspace. Rejected scope edits precede
the CAS write. Legacy unowned workspace memories remain unconfirmed.

A packet's full-memory continuation is `/runtime/context/:objectiveId/memory`,
with the same profile/task/project/environment query and a context-digest-bound
cursor. Metadata-only credential checks return typed validation observations;
the descriptor record accepts no observation fields or secret values. Each
lookup uses the recorded assistant owner. The context digest includes status and
configuration generation but excludes observation time, so rechecking unchanged
metadata cannot make every queued packet stale. Actual descriptor/profile/scope
or resolver configuration changes still invalidate dispatch.


## Implemented capability directory

The bounded `CapabilityReader` port composes native adapters in backendapp.
Pages contain at most 100 entries (default 50), sorted by stable identity, and
cursors bind owner, workspace, binding version, session, filter and catalog
hash. Each page reauthorizes the workspace and reloads native sources; a changed
generation returns 409. Profile MCP configuration and live session attachment
are separate rows. Attachment requires current session/executor/profile identity
and connection evidence; a stopped, replaced, disconnected or disabled session
cannot retain attached status.

Directory DTOs exclude configuration, environments, credentials, provider error
text and transcripts. Schemas are bounded structural projections (8 KiB, six
nested levels and 64 properties); defaults, descriptions, examples, references
and extensions are omitted, with `schema_partial` indicating incomplete data.
Invocation uses the native full schema. Plugin hints and external MCP tools have
unknown effect and no inspect grant. Stored integration health is read without
provider probes; it is a historical observation, not a credential-use grant.
