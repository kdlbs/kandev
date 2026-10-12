# kandev.plugin.v1 — gRPC plugin contract (FROZEN)

Supersedes the HTTP+HMAC transport. Every task builds against this file; do not
diverge without updating it. The frontend contract (PLUGIN-API.md) is unchanged
except where noted in §7.

## 1. Architecture

- Plugin **backends are Go binaries** distributed in a release tarball and
  **spawned by kandev as subprocesses** via `hashicorp/go-plugin`.
- Transport: gRPC over a unix domain socket (macOS/Linux) or loopback TCP +
  AutoMTLS (Windows) — negotiated by go-plugin, invisible to authors.
- Auth: the spawn relationship + go-plugin handshake + AutoMTLS. **No api_key,
  no webhook_secret, no HMAC** — all credential machinery is removed for managed
  plugins. The remote/self-hosted tier (`base_url` registration) is REMOVED
  (future work if ever needed).
- The `LISTENING <addr>` stdout handshake is replaced by go-plugin's handshake.

## 2. go-plugin handshake

```go
var Handshake = plugin.HandshakeConfig{
    ProtocolVersion:  1,
    MagicCookieKey:   "KANDEV_PLUGIN",
    MagicCookieValue: "kandev-plugin-v1",
}
// plugin map key: "plugin"; AutoMTLS: enabled on the client (kandev) side.
```

Env kandev injects into the subprocess:

- `KANDEV_PLUGIN_DATA_DIR` — per-plugin writable dir (`~/.kandev/plugins/<id>/data`).

## 3. Proto (`apps/backend/proto/kandev/plugin/v1/plugin.proto`)

```proto
syntax = "proto3";
package kandev.plugin.v1;
import "google/protobuf/struct.proto";

// Implemented by the PLUGIN. kandev is the client.
service Plugin {
  rpc DeliverEvent(Event) returns (EventAck);
  rpc HandleWebhook(WebhookRequest) returns (WebhookResponse);
  // Browser calls pass through host-authenticated declared actions only.
  rpc HandleAction(PluginActionRequest) returns (PluginActionResponse);
  // Manifest-owned dynamic composer reference source operations.
  rpc SearchEntityReferences(SearchEntityReferencesRequest) returns (SearchEntityReferencesResponse);
  rpc AuthorizeEntityReference(AuthorizeEntityReferenceRequest) returns (AuthorizeEntityReferenceResponse);
  // Optional provider-neutral credential resolver for declared repository providers.
  rpc ResolveGitCredential(ResolveGitCredentialRequest) returns (ResolveGitCredentialResponse);
  rpc GetGitCredentialBinding(GitCredentialBindingRequest) returns (GitCredentialBindingResponse);
  // Optional, all-or-nothing remote executor provider extension.
  rpc ValidateExecutorProfile(ValidateExecutorProfileRequest) returns (ValidateExecutorProfileResponse);
  rpc ProvisionExecutorEnvironment(ProvisionExecutorEnvironmentRequest) returns (ProvisionExecutorEnvironmentResponse);
  rpc RecoverExecutorOperation(RecoverExecutorOperationRequest) returns (RecoverExecutorOperationResponse);
  rpc AttachExecutorEnvironment(AttachExecutorEnvironmentRequest) returns (AttachExecutorEnvironmentResponse);
  rpc InspectExecutorEnvironment(InspectExecutorEnvironmentRequest) returns (InspectExecutorEnvironmentResponse);
  rpc ResolveExecutorConnection(ResolveExecutorConnectionRequest) returns (ResolveExecutorConnectionResponse);
  rpc DestroyExecutorEnvironment(DestroyExecutorEnvironmentRequest) returns (DestroyExecutorEnvironmentResponse);
}

// Implemented by KANDEV (served back over the go-plugin broker).
// Ordinary Host APIs are capability-gated. Executor callbacks are bound to an
// admitted provider operation and its current dispatch generation.
service Host {
  rpc GetState(GetStateRequest) returns (GetStateResponse);
  rpc SetState(SetStateRequest) returns (SetStateResponse);
  rpc DeleteState(DeleteStateRequest) returns (DeleteStateResponse);
  rpc ListState(ListStateRequest) returns (ListStateResponse);
  rpc RevealSecret(RevealSecretRequest) returns (RevealSecretResponse);
  rpc EmitEvent(EmitEventRequest) returns (EmitEventResponse);

  // Empty profile_id delegates to the platform default. A non-empty value
  // selects that exact eligible profile. This separate method prevents an
  // older host from silently ignoring an explicit selection.
  rpc InvokeUtilityAgentWithOptions(InvokeUtilityAgentWithOptionsRequest) returns (InvokeUtilityAgentResponse);

  // The plugin's own operator-editable config (Settings > Plugins > <plugin>,
  // driven by the manifest's config_schema). Ungated; secret values arrive
  // in cleartext — this RPC is how an operator-configured credential (e.g. a
  // PAT) reaches the plugin. At rest, secret config fields live in kandev's
  // encrypted vault (the config file holds only a vault reference); the Host
  // resolves them before responding. kandev restarts a running plugin on
  // config change, so reading config at startup is sufficient.
  rpc GetConfig(GetConfigRequest) returns (GetConfigResponse);

  // Plugin-scoped secret primitives — capability `secrets`. Keys are
  // namespaced server-side to the calling plugin (vault id
  // "plugin:<id>:secret:<key>", key must match
  // [a-zA-Z0-9][a-zA-Z0-9._-]{0,127}), so a plugin can only ever touch its
  // OWN entries; RevealSecret remains the way to resolve an
  // operator-provided reference to a shared/global secret. Values live in
  // kandev's encrypted vault (AES-256-GCM at rest) and the whole
  // "plugin:<id>:" namespace is deleted on uninstall.
  rpc GetSecret(GetSecretRequest) returns (GetSecretResponse);
  rpc SetSecret(SetSecretRequest) returns (SetSecretResponse);
  rpc DeleteSecret(DeleteSecretRequest) returns (DeleteSecretResponse);

  // Host data API (ADR 0043, §3a below) — reads, capability api_read:<resource>.
  rpc ListTasks(ListTasksRequest) returns (ListTasksResponse);
  rpc GetTask(GetTaskRequest) returns (Task);
  rpc ListWorkspaces(ListWorkspacesRequest) returns (ListWorkspacesResponse);
  rpc ListWorkflows(ListWorkflowsRequest) returns (ListWorkflowsResponse);
  rpc ListWorkflowSteps(ListWorkflowStepsRequest) returns (ListWorkflowStepsResponse);
  rpc ListAgentProfiles(ListAgentProfilesRequest) returns (ListAgentProfilesResponse);
  rpc ListExecutorProfiles(ListExecutorProfilesRequest) returns (ListExecutorProfilesResponse);
  rpc CheckpointExecutorResource(CheckpointExecutorResourceRequest) returns (CheckpointExecutorResourceResponse);
  rpc ReportExecutorProgress(ReportExecutorProgressRequest) returns (ReportExecutorProgressResponse);
  rpc ReadExecutorRuntimeArtifact(ReadExecutorRuntimeArtifactRequest) returns (stream ExecutorRuntimeArtifactChunk);
  rpc ListRepositories(ListRepositoriesRequest) returns (ListRepositoriesResponse);
  rpc ListSessions(ListSessionsRequest) returns (ListSessionsResponse);
  rpc ListSessionCodeStats(ListSessionCodeStatsRequest) returns (ListSessionCodeStatsResponse);

  // Host data API — writes, capability api_write:<resource>. Route through the
  // first-party service layer so events fire (§3a "Writes"). api_write:tasks
  // gates CreateTask/UpdateTask; api_write:messages gates SendMessage (delivers
  // a prompt to a task session through the orchestrator). Undeclared → gRPC
  // PermissionDenied.
  rpc CreateTask(CreateTaskRequest) returns (Task);
  rpc UpdateTask(UpdateTaskRequest) returns (Task);
  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);
  // Additive Host v2 exact operations. Existing v1 wire methods remain; their
  // current compatibility behavior, including denied unsafe writes, is documented below.
  rpc GetCapabilityContext(GetCapabilityContextRequest) returns (GetCapabilityContextResponse);
  rpc ApplyWorkspaceAdministrationExact(ApplyWorkspaceAdministrationExactRequest) returns (WorkspaceAdministrationExactResponse);
  rpc GetSourceIssueCapabilitiesExact(GetSourceIssueCapabilitiesExactRequest) returns (GetSourceIssueCapabilitiesExactResponse);
  rpc CommentSourceIssueExact(CommentSourceIssueExactRequest) returns (SourceIssueWritebackExactResponse);
  rpc TransitionSourceIssueExact(TransitionSourceIssueExactRequest) returns (SourceIssueWritebackExactResponse);
  rpc UpdateTaskExact(UpdateTaskExactRequest) returns (UpdateTaskExactResponse);
  rpc AcquireTaskManagementClaimExact(AcquireTaskManagementClaimExactRequest) returns (TaskManagementClaimExactResponse);
  rpc ReleaseTaskManagementClaimExact(ReleaseTaskManagementClaimExactRequest) returns (TaskManagementClaimExactResponse);
  rpc TransferTaskManagementClaimExact(TransferTaskManagementClaimExactRequest) returns (TaskManagementClaimExactResponse);
  rpc SetTaskCompletionCriteriaExact(SetTaskCompletionCriteriaExactRequest) returns (TaskCompletionGateExactResponse);
  rpc VerifyTaskCompletionCriterionExact(VerifyTaskCompletionCriterionExactRequest) returns (TaskCompletionGateExactResponse);
  rpc EnsureManagedAgentConversationExact(EnsureManagedAgentConversationExactRequest) returns (EnsureManagedAgentConversationExactResponse);
  rpc GetManagedAgentConversationStatusExact(GetManagedAgentConversationStatusExactRequest) returns (GetManagedAgentConversationStatusExactResponse);
  rpc ListManagedAgentConversationsExact(ListManagedAgentConversationsExactRequest) returns (ListManagedAgentConversationsExactResponse);
  rpc SetManagedAgentConversationPausedExact(SetManagedAgentConversationPausedExactRequest) returns (SetManagedAgentConversationPausedExactResponse);
  rpc DeleteManagedAgentConversationExact(DeleteManagedAgentConversationExactRequest) returns (DeleteManagedAgentConversationExactResponse);
  rpc EnqueueManagedAgentInputExact(EnqueueManagedAgentInputExactRequest) returns (EnqueueManagedAgentInputExactResponse);
  rpc GetManagedAgentInputExact(GetManagedAgentInputExactRequest) returns (GetManagedAgentInputExactResponse);
  rpc ListManagedAgentInputsExact(ListManagedAgentInputsExactRequest) returns (ListManagedAgentInputsExactResponse);
  rpc CancelManagedAgentInputExact(CancelManagedAgentInputExactRequest) returns (CancelManagedAgentInputExactResponse);
  rpc DispatchManagedAgentConversationExact(DispatchManagedAgentConversationExactRequest) returns (DispatchManagedAgentConversationExactResponse);
  rpc ListManagedConversationSchedulesExact(ListManagedConversationSchedulesExactRequest) returns (ListManagedConversationSchedulesExactResponse);
  rpc CreateManagedConversationScheduleExact(CreateManagedConversationScheduleExactRequest) returns (ManagedConversationScheduleExactResponse);
  rpc UpdateManagedConversationScheduleExact(UpdateManagedConversationScheduleExactRequest) returns (ManagedConversationScheduleExactResponse);
  rpc SetManagedConversationScheduleEnabledExact(SetManagedConversationScheduleEnabledExactRequest) returns (ManagedConversationScheduleExactResponse);
  rpc DeleteManagedConversationScheduleExact(DeleteManagedConversationScheduleExactRequest) returns (HostCommandResult);
  rpc EnsureTaskRunExact(EnsureTaskRunExactRequest) returns (EnsureTaskRunExactResponse);
  rpc StopTaskRunExact(StopTaskRunExactRequest) returns (StopTaskRunExactResponse);
  rpc RecoverSessionExact(RecoverSessionExactRequest) returns (RecoverSessionExactResponse);
  rpc CancelPendingTaskTransitionExact(CancelPendingTaskTransitionExactRequest) returns (CancelPendingTaskTransitionExactResponse);
  rpc GetSessionModeContextExact(GetSessionModeContextExactRequest) returns (GetSessionModeContextExactResponse);
  rpc SetSessionModeExact(SetSessionModeExactRequest) returns (SetSessionModeExactResponse);
  rpc RespondPermissionExact(RespondPermissionExactRequest) returns (RespondPermissionExactResponse);
  rpc AnswerClarificationExact(AnswerClarificationExactRequest) returns (AnswerClarificationExactResponse);
  rpc PreviewPluginOwnedTaskTree(PreviewPluginOwnedTaskTreeRequest) returns (PreviewPluginOwnedTaskTreeResponse);
  rpc DeletePluginOwnedTaskTree(DeletePluginOwnedTaskTreeRequest) returns (DeletePluginOwnedTaskTreeResponse);
}

message Event {
  string event_id = 1;                     // fresh uuid per delivery
  string event_type = 2;                   // bus subject, e.g. "task.created"
  string occurred_at = 3;                  // RFC3339 UTC
  string workspace_id = 4;                 // empty if not derivable
  google.protobuf.Struct payload = 5;      // marshaled bus event.Data
}
message EventAck {}

message WebhookRequest {
  string webhook_key = 1;
  string method = 2;
  string path = 3;                         // remainder after the key
  string query = 4;
  map<string, string> headers = 5;         // single-valued; multi joined by ", "
  bytes body = 6;
}
message WebhookResponse { int32 status = 1; map<string, string> headers = 2; bytes body = 3; }

// The host derives resources and actor after normal HTTP auth/authorization. Body is
// untrusted JSON bounded by the manifest declaration; plugins must not infer authority
// from it. Response headers are filtered by the host allowlist.
message PluginActionRequest {
  string action_key = 1;
  VerifiedActionContext context = 2;
  bytes body = 3;
}
message VerifiedActionContext {
  string actor_id = 1;
  string workspace_id = 2;
  string task_id = 3;
  string repository_id = 4;
  string session_id = 5;
  string head_branch = 6;
}
// status=0 preserves legacy 200. Otherwise the host accepts 200..599 and
// projects the status after filtering headers and enforcing the body limit.
message PluginActionResponse { bytes body = 1; map<string, string> headers = 2; int32 status = 3; }

message SearchEntityReferencesRequest { string source = 1; string workspace_id = 2; string query = 3; int32 limit = 4; }
message SearchEntityReferencesResponse { repeated EntityReferenceCandidate candidates = 1; }
message EntityReferenceCandidate { string provider_local_id = 1; string title = 2; string url = 3; google.protobuf.Struct attributes = 4; }
message AuthorizeEntityReferenceRequest { string source = 1; string workspace_id = 2; string purpose = 3; google.protobuf.Struct reference = 4; }
message AuthorizeEntityReferenceResponse { bool allowed = 1; string reason = 2; }

// Scope is host-verified. The credential value is transient; it must never be written
// into a host URL, task state, command argument, log, or executor environment.
message ResolveGitCredentialRequest { string provider_id = 1; string workspace_id = 2; string task_id = 3; string session_id = 4; string repository_id = 5; string host = 6; string path = 7; }
message ResolveGitCredentialResponse { string username = 1; string secret = 2; string expires_at = 3; }
message GitCredentialBindingRequest { string provider_id = 1; string workspace_id = 2; string task_id = 3; string session_id = 4; string repository_id = 5; string host = 6; string path = 7; }
message GitCredentialBindingResponse { string binding = 1; }

message GetStateRequest { string scope = 1; string scope_id = 2; string key = 3; }
message GetStateResponse { bool found = 1; google.protobuf.Struct value = 2; }
message SetStateRequest { string scope = 1; string scope_id = 2; string key = 3; google.protobuf.Struct value = 4; }
message SetStateResponse {}
message DeleteStateRequest { string scope = 1; string scope_id = 2; string key = 3; }
message DeleteStateResponse {}
message ListStateRequest { string scope = 1; string scope_id = 2; }
message ListStateResponse { repeated StateEntry entries = 1; }
message StateEntry { string key = 1; google.protobuf.Struct value = 2; string updated_at = 3; }

message GetConfigRequest {}
message GetConfigResponse { google.protobuf.Struct config = 1; }

message GetSecretRequest { string key = 1; }
message GetSecretResponse { bool found = 1; string value = 2; }
message SetSecretRequest { string key = 1; string value = 2; }
message SetSecretResponse {}
message DeleteSecretRequest { string key = 1; }
message DeleteSecretResponse {}

message RevealSecretRequest { string ref = 1; }
message RevealSecretResponse { string value = 1; }

message EmitEventRequest { string event_name = 1; google.protobuf.Struct payload = 2; }
message EmitEventResponse {}
message InvokeUtilityAgentRequest { string prompt = 1; }
message InvokeUtilityAgentWithOptionsRequest {
  string prompt = 1;
  string profile_id = 2;
}
message InvokeUtilityAgentResponse { string text = 1; }
```

Notes: scope ∈ instance|workspace|task|agent (empty scope_id for instance —
matches the state store). The plugin never passes its own id; the Host service
instance is bound to the plugin's record at spawn time.

`InvokeUtilityAgent` remains the prompt-only compatibility method. On a revised
host it uses the platform default profile from Settings > Utility Agents.
`InvokeUtilityAgentWithOptions` accepts the same prompt plus an optional
`profile_id`; a non-empty ID selects that exact eligible profile and an invalid
explicit ID returns `FailedPrecondition` without fallback. The revised SDK
uses the options RPC for every call, including calls without an override. An
older host returns `Unimplemented` for that method, and the SDK does not retry
through the prompt-only RPC. Plugins own saved preferences and pass them in
the request. The host does not read plugin configuration, utility-agent
records, or transition metadata for invocation selection.

`PreviewPluginOwnedTaskTree` remains available for source-scoped inspection.
`DeletePluginOwnedTaskTree` remains in the v1 wire contract for compatibility,
but the current Host denies it with `PermissionDenied` and reason
`native_human_confirmation_required`. The RPC cannot carry the current
authenticated Human's task-deletion confirmation, so plugins must direct the
user to Kandev's native task UI. Native confirmation is bound to the Human,
selected task roots, cascade choice, worktree-discard choice, and task-tree
snapshot; it expires after five minutes and cannot be replayed. A changed tree
requires a new preview and confirmation. Do not treat plugin chat, a previous
preview, or task provenance as deletion consent.

### 3a. Host data API (ADR 0043)

Read/write RPCs let plugins read and write kandev's own domain data —
tasks, sessions, workspaces, workflows, agent profiles, repositories, messages —
over the same Host gRPC channel used for state/secrets, instead of opening the
kandev database file directly. Full message definitions (`Page`, `PageInfo`,
`Task`, `TaskFilter`, `Workspace`, `Workflow`, `WorkflowStep`, `AgentProfile`,
`Repository`, `Session`, `SessionFilter`, `SessionCodeStats`, and the write
`CreateTaskRequest`/`UpdateTaskRequest`/`SendMessageRequest`/`SendMessageResponse`) live in
the real proto — `apps/backend/proto/kandev/plugin/v1/plugin.proto` — and are not
duplicated here; this section covers the RPC list (added to `service Host` above),
capability gating, and cross-cutting conventions. See ADR 0043
(`docs/decisions/0043-plugin-host-data-api.md`) for the design rationale.

API v1 DTO fields are additive-only. `Task.labels` field 23, shipped in v0.93.0,
remains generated and readable as a deprecated compatibility field. New plugins
store provider-specific annotations in plugin-owned task state and render them
through plugin UI slots; CreateTask and UpdateTask do not expose label writes.

**Readable resources.** Each read RPC requires `api_read:<resource>` in the
plugin's manifest:

| RPC                                          | Capability                   | Resource          |
| -------------------------------------------- | ---------------------------- | ----------------- |
| `ListTasks` / `GetTask`                      | `api_read:tasks`             | tasks             |
| `ListWorkspaces`                             | `api_read:workspaces`        | workspaces        |
| `ListWorkflows`                              | `api_read:workflows`         | workflows         |
| `ListWorkflowSteps`                          | `api_read:workflows`         | workflows         |
| `ListAgentProfiles`                          | `api_read:agent_profiles`    | agent_profiles    |
| `ListExecutorProfiles`                       | `api_read:executor_profiles` | executor_profiles |
| `ListRepositories`                           | `api_read:repositories`      | repositories      |
| `ListSessions`                               | `api_read:sessions`          | sessions          |
| `ListSessionCodeStats`                       | `api_read:sessions`          | sessions          |
| `ListMessages`                               | `api_read:messages`          | messages          |
| `ListPendingInteractions` / `GetInteraction` | `api_read:interactions`      | interactions      |

An undeclared capability returns gRPC `PermissionDenied` with message
`capability 'api_read:tasks' not declared` (substituting the actual resource) —
identical in shape to the existing state/secrets gating. Declaring the resource
grants every RPC listed against it; there is no finer-grained gate within a
resource (e.g. `api_read:workflows` covers both `ListWorkflows` and
`ListWorkflowSteps`).

### Host v2 linked issue writeback

`pluginsdk.HostSourceIssueWriteback(host)` exposes exact capabilities, comment,
and transition operations for a task's existing Jira or Linear issue. Reads
require `api_read:source_issues` and `host.v2.read:source_issues`. Writes
require `api_write:source_issues` and `host.v2.write:source_issues`, plus the
current approval revision and manifest digest.

The Host resolves the source identity from the persisted task-to-issue link and
checks that the task metadata still names that link. Plugins cannot submit an
arbitrary issue URL, provider credential, or external issue identifier. The
capability response includes the task and source resource versions, current
status, and only the transitions observed for that issue. A write must include
both observed versions, a stable idempotency key, and exactly one comment or
transition operation.

The Host persists the command and source receipt before it asks the provider to
write. It rechecks the source link and provider state immediately before the
request. Completed retries return the original result. A provider timeout or
other ambiguous outcome returns `UNCERTAIN`; retries with the same key return
that receipt and never send the comment or transition again. A provider 429 is
`RATE_LIMITED`, absent workspace credentials are `UNAVAILABLE` with reason
`missing_credentials`, changed source links or versions are `CONFLICT`, and
unsupported operations are `UNSUPPORTED`. Receipts identify the plugin
installation, workspace, task, provider, issue identity, operation, and
provider receipt when one is available. The Host stores only the payload
digest, not the comment body or credentials.

```go
manager, ok := pluginsdk.HostSourceIssueWriteback(host)
if !ok {
    return errors.New("source issue operations are unavailable")
}
result, source, _, err := manager.GetCapabilities(ctx, pluginsdk.SourceIssueCapabilitiesQuery{
    RequestID: "read-source", WorkspaceID: workspaceID, TaskID: task.ID,
})
if err != nil {
    return err
}
if result == nil || result.Status != pluginsdk.CommandApplied || source == nil {
    return errors.New("linked source issue is unavailable")
}
result, receipt, err := manager.Comment(ctx, pluginsdk.SourceIssueWritebackCommand{
    RequestID: "comment-source", WorkspaceID: workspaceID, TaskID: task.ID,
    IdempotencyKey: "release-note-42", ExpectedTaskResourceVersion: source.TaskResourceVersion,
    ExpectedSourceResourceVersion: source.ResourceVersion,
    ApprovalRevision: capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
    Body: "The release is ready for review.",
})
if err != nil {
    return err
}
_ = result.Status // inspect APPLIED, RATE_LIMITED, CONFLICT, or UNCERTAIN
_ = receipt       // retain the source identity and operation result for reconciliation
```

**Writes.** `CreateTask`, `UpdateTask`, and `SendMessage` are implemented.
`CreateTask`/`UpdateTask` are gated by `api_write:tasks` and route through
`internal/task/service` (never a repository), so `task.*` events fire and
WS-driven UI stays in sync. The server stamps `source = "plugin:<id>"` on the
created task's metadata — a plugin cannot set it itself. `CreateTask` resolves
sane placement defaults when the plugin omits them: an empty `workspace_id`
resolves to the single workspace (ambiguous otherwise → `InvalidArgument`), an
empty `workflow_id` to that workspace's first workflow. `UpdateTask` accepts a
conservative field mask — `title`, `description`, `state`, and `priority` (each
optional/leave-unset). `workflow_step_id` remains present for
wire compatibility but is rejected; plugins use `MoveTask` for transitions.
`start_agent` best-effort auto-launches an agent
through the orchestrator; a launch failure does not fail the create.

Write validation/error contract (so plugin authors can predict outcomes):
`CreateTask` requires a non-empty `title` (`InvalidArgument` otherwise);
`UpdateTask` requires `id` (`InvalidArgument`) and returns `NotFound` for a
task that doesn't exist; an `UpdateTask` `state` outside the known task-state
enum (or the orchestrator-owned `SCHEDULING`) is rejected with `InvalidArgument`
before it reaches the service.

`SendMessage` is gated by `api_write:messages` and delivers a prompt to a task
session through the orchestrator's real delivery path (the same one
`message_task` uses), so the message reaches the agent and drives a turn — not
an office comment. It resolves the target session (explicit `session_id`,
verified to belong to the task, or the task's primary session), records a user
message stamped `source = "plugin:<id>"`, and dispatches by session state: a
running session queues the prompt (`status: "queued"`); an idle/completed one is
prompted, resuming the agent if its process is gone (`"sent"`); a never-started
one is launched with the prompt as its first turn (`"started"`). A failed
dispatch deletes the recorded message so no orphan prompt is left.

**Pending interactions** (`api_read:interactions` / `api_write:interactions`,
ADR 0052 — `docs/decisions/0052-plugin-host-interaction-api.md`) are the durable
record of every agent request still owed a human answer: a tool permission
request, or a whole clarification bundle collapsed into one `Interaction` with
its `questions`. Session state is deliberately NOT that record —
`WAITING_FOR_INPUT` also describes an ordinarily completed turn — so a plugin
that branches on state alone reports attention nobody owes.

`ListPendingInteractions` applies the same turn/session authority Kandev's own
list surfaces use: only the session's current durable turn counts, terminal
sessions quarantine pending history, and only the newest permission row of that
turn is answerable. `GetInteraction` resolves ANY interaction by pending id,
terminal ones included, so an event-driven cache that started late, restarted,
or dropped an event converges on the current result instead of `NotFound`.

The original v1 implementation routed the three writes through the first-party
services the native UI drives:
`RespondToPermission` through the orchestrator, `AnswerClarification` and
`CancelClarification` through the clarification handler (including its durable
exclusive claim and its detached-resume fallback). A permission response must
name one of the interaction's declared options — Kandev derives the
approve/deny outcome from that option's recorded ACP kind, so a plugin cannot
report an outcome the agent never offered — and the target session comes from
the durable record, never from the request.

Writes are terminal-once: the first response wins, an already-resolved
interaction answers `FailedPrecondition`, and an unknown id answers `NotFound`.
Those two codes are the distinction a reconciling cache needs between "someone
else answered first" and "my id is stale". `CancelClarification` is delivered as
a decline rather than an in-memory cancellation, so it also settles a bundle
whose original waiter went away in a restart.

**Current compatibility rule:** this historical section describes the original
v1 behavior. The current Host retains these RPCs for wire and SDK source
compatibility, but returns `PermissionDenied` for plugin-originated responses.
Use the exact response methods documented below, which require a Host-issued
receipt from the authenticated native UI and the observed interaction revision.

**Reads and writes go through the service layer, never a repository.** Each read
handler calls the relevant internal service (task service, workflow service, the
analytics service for `ListSessionCodeStats`); each write handler calls the
event-publishing service method — so derived fields, events, and future access
rules stay centralized in one place.

**DTOs are a hand-mapped, versioned contract — never internal structs.** The
backend maps internal models to the proto messages above with explicit
conversion code; it never marshals domain structs through
`google.protobuf.Struct` and never generates the messages from models. Fields
are additive-only after merge — removing or renaming one is a breaking change
requiring a new `api_version`. `SessionCodeStats` in particular is a
deliberately **computed** shape (`lines_added_committed` /
`lines_deleted_committed` from commit sums, `lines_added_peak_pending` /
`lines_deleted_peak_pending` from the peak uncommitted diff across snapshots),
computed on demand per request — plugins never see the raw
`task_session_commits` / `task_session_git_snapshots` rows those numbers are
derived from.

**Conventions.**

- **Pagination:** opaque-cursor. A request carries `Page{limit, cursor}` (0 limit
  → server default, currently 50, capped at 200); a list response carries
  `PageInfo{next_cursor, has_more}`. An empty cursor is the first page; echo
  `next_cursor` to continue. Plugins MUST NOT interpret cursor contents — the
  current server encodes it as a decimal offset, but that is an implementation
  detail, not part of the contract.
- **Timestamps:** RFC3339 strings (`created_at`, `updated_at`, `started_at`,
  `occurred_at`, ...), matching the `Event` envelope and the JSON API — one time
  representation across the whole plugin contract, never protobuf `Timestamp`.
- **Nullables:** optional string fields use proto3 `optional` (e.g.
  `Task.started_at`, `Task.completed_at`, `Task.parent_id`,
  `Session.ended_at`), so an absent (NULL) value is distinguishable from an
  empty string.
- **Scoping (v1):** reads are global to the kandev instance — plugins are
  installed instance-wide, not per-workspace. Filters (`workspace_ids`,
  `task_ids`, `states` on `TaskFilter`/`SessionFilter`) narrow results but do
  not themselves confer or restrict visibility; a server-side scoping hook is
  reserved for a future per-plugin/per-user restriction without a contract
  change (ADR 0043, open decision (a)).
- **Ephemeral tasks** (quick-chat) are excluded from `ListTasks` unless the
  request sets `TaskFilter.include_ephemeral`.

## 4. SDK (`apps/backend/pkg/pluginsdk`)

Public Go module surface (authors import only this):

```go
type Plugin interface {
    OnEvent(ctx context.Context, e *Event) error            // return err → kandev retries
    HandleWebhook(ctx context.Context, req *WebhookRequest) (*WebhookResponse, error)
}
type Host interface {                                        // injected before Serve returns
    GetState/SetState/DeleteState/ListState(...)
    GetConfig(ctx) (map[string]any, error)                   // own operator config, cleartext
    RevealSecret(ctx, ref string) (string, error)            // operator-provided shared-secret ref
    GetSecret(ctx, key) (value string, found bool, err error) // plugin-owned, vault-backed
    SetSecret(ctx, key, value string) error
    DeleteSecret(ctx, key string) error
    EmitEvent(ctx, name string, payload map[string]any) error

    // Host data API (ADR 0043, §3a) — each accessor is capability-gated by
    // the corresponding api_read:<resource>; see "Host data API accessors"
    // below for the reader interfaces and Go-native DTOs.
    Tasks() TaskReader
    Sessions() SessionReader
    Workspaces() WorkspaceReader
    Workflows() WorkflowReader
    AgentProfiles() AgentProfileReader
    Repositories() RepositoryReader
}
// Optional Host extension, discovered without breaking existing Host implementations.
type ExecutorProfileHost interface {
    ExecutorProfiles() ExecutorProfileReader
}
func ExecutorProfiles(host Host) (ExecutorProfileReader, bool)
func Serve(p Plugin, opts ...Option)     // blocks; wires go-plugin server + broker
// Optional embeddable no-op base: sdk.UnimplementedPlugin
// Optional embeddable no-op base for Host data accessors (every method
// PermissionDenied/Unimplemented): sdk.UnimplementedHostData — used on the
// kandev side, not by plugin authors.
```

### Provider extensions (additive)

`Plugin` remains source-compatible. `Serve` detects optional handler interfaces and
returns `Unimplemented` only when a plugin has not opted into the corresponding
manifest declaration:

```go
type ActionHandler interface {
    HandleAction(context.Context, *PluginActionRequest) (*PluginActionResponse, error)
}
type EntityReferenceHandler interface {
    SearchEntityReferences(context.Context, *SearchEntityReferencesRequest) (*SearchEntityReferencesResponse, error)
    AuthorizeEntityReference(context.Context, *AuthorizeEntityReferenceRequest) (*AuthorizeEntityReferenceResponse, error)
}
type GitCredentialResolver interface {
    ResolveGitCredential(context.Context, *ResolveGitCredentialRequest) (*ResolveGitCredentialResponse, error)
}
type GitCredentialBinder interface {
    GetGitCredentialBinding(context.Context, *GitCredentialBindingRequest) (*GitCredentialBindingResponse, error)
}
```

`HandleAction` receives host-verified actor/resource context and bounded untrusted body
separately. For task actions, an optional session selector is verified against the task;
when paired with a verified repository selector, `head_branch` is resolved from that
session's exact repository worktree. Browser body JSON cannot select or override it.
`SearchEntityReferences` candidates are untrusted: the host injects
descriptor identity and constructs canonical reference fields. `AuthorizeEntityReference`
runs for search and submission. `ResolveGitCredential` receives an exact host-verified
scope for both initial host materialization and helper-lease redemption, and returns
only a transient credential consumed by the host Git process. Initial materialization
and strict pre-worktree refresh carry the same task/session/repository scope; after a
successful refresh the worktree layer uses local refs and performs no second network operation.
Credential requests must include workspace, task, active session, repository, exact host, and exact path;
an incomplete plugin-provider scope fails closed.
`GetGitCredentialBinding` receives the same scope and returns an opaque, non-secret
generation checked before and after redemption; missing or changed bindings fail closed.
Disabling, failing, or uninstalling a plugin immediately revokes leases for all
manifest-declared provider IDs. Repository host and path matching are exact and
case-sensitive. The broker does not add, remove, or equate a trailing `.git`.

### Remote executor providers

The optional `ExecutorProviderPlugin` extension is all-or-nothing. Its seven RPCs
validate profiles, provision environments, recover uncertain operations, attach to
existing resources, inspect state, resolve connection leases, and destroy resources.
An older plugin that implements none of these methods remains compatible. A plugin
that declares a provider but omits any required method is unavailable for that provider.

```go
type ExecutorProviderPlugin interface {
    ValidateExecutorProfile(context.Context, *ValidateExecutorProfileRequest) (*ValidateExecutorProfileResponse, error)
    ProvisionExecutorEnvironment(context.Context, *ProvisionExecutorEnvironmentRequest) (*ProvisionExecutorEnvironmentResponse, error)
    RecoverExecutorOperation(context.Context, *RecoverExecutorOperationRequest) (*RecoverExecutorOperationResponse, error)
    AttachExecutorEnvironment(context.Context, *AttachExecutorEnvironmentRequest) (*AttachExecutorEnvironmentResponse, error)
    InspectExecutorEnvironment(context.Context, *InspectExecutorEnvironmentRequest) (*InspectExecutorEnvironmentResponse, error)
    ResolveExecutorConnection(context.Context, *ResolveExecutorConnectionRequest) (*ResolveExecutorConnectionResponse, error)
    DestroyExecutorEnvironment(context.Context, *DestroyExecutorEnvironmentRequest) (*DestroyExecutorEnvironmentResponse, error)
}

type ExecutorProviderHost interface {
    ExecutorProvider() ExecutorProviderHostAPI
}

type ExecutorProviderHostAPI interface {
    CheckpointExecutorResource(context.Context, *CheckpointExecutorResourceRequest) (*CheckpointExecutorResourceResponse, error)
    ReportExecutorProgress(context.Context, *ReportExecutorProgressRequest) (*ReportExecutorProgressResponse, error)
    ReadExecutorRuntimeArtifact(context.Context, *ReadExecutorRuntimeArtifactRequest, func(*ExecutorRuntimeArtifactChunk) error) error
}
```

The manifest owns provider identity and declares contract and resource-state
versions. The host forms the identity as `plugin:<plugin-id>:<provider-key>`.
Provider context includes an operation ID and input digest. Retries of one launch
reuse both values; the provider must make provisioning idempotent and recover the
same resource after a lost response. Recovery distinguishes a found resource, a
confirmed absent operation, and an unknown outcome. Unknown outcomes stay in host
cleanup inventory until resolved.

Profile secret values are transient inputs. Do not write them, bootstrap nonces,
or connection credentials to resource state, logs, command arguments, task data,
or endpoint URLs. Persist only bounded, non-secret state accepted by the manifest's
`resource_state_schema`. The host supplies generation-fenced callbacks to checkpoint
resource state, report progress, and stream the agentctl artifact selected for the
resource platform.

The provider resolves short-lived HTTPS leases for agentctl. Keep credentials in
HTTP or WebSocket headers. Do not put credentials in URL queries or user info.
The host validates endpoint addresses and TLS and decorates agentctl, editor, and
preview proxy traffic. Remote environments must reach the configured Kandev API
URL for normal task and session operations.

Kandev owns agentctl, ACP, workspace materialization, authorization, task lifecycle,
and durable resource inventory. The provider owns remote compute and provider
credentials. Disable, uninstall, or upgrade cannot discard retained-resource
inventory. Cleanup remains retryable until the provider confirms absence.

SDK types mirror proto but use `map[string]any` for Struct fields. The SDK owns
all go-plugin/grpc plumbing (handshake, broker for Host, conversions).

### Host v2 exact task update

The Host service remains in the `kandev.plugin.v1` package for wire
compatibility. Exact operations are an optional Go SDK extension: `HostV2(host)`
returns an `ExactHost` only when the connected host implements the v2 methods.
Legacy Host methods, including `Tasks().Update`, keep their existing semantics.

`GetCapabilityContext` reports the exact operations supported by the host,
workspace approval revision, installation manifest digest, and bounded limits.
It does not grant permission. `UpdateTaskExact` requires an active workspace
approval for `host.v2.write:tasks`, a matching approval revision and manifest
digest, a stable idempotency key, and the task `resource_version` returned by
Host task reads. It can change title, description, state, or priority.
`SetTaskLabelsExact` replaces the label list at the same exact resource version.
`AssignTaskExact` sets or clears the human assignee at that version without
changing the task's worker-agent assignment.
`MoveTaskExact` uses the observed task version and shared workflow validation,
WIP admission, and pending-transition rules. Its task-row change and operation
identity commit together, so a retry can recover a move after a lost receipt.
`ArchiveTaskExact` applies the task's normal archive lifecycle only when its
observed version still matches; the archive marker, queue purge, and operation
identity commit together, while normal cleanup continues through the task
service.
`CreateTaskExact` accepts a workspace-scoped external source identity; the task
service returns an existing active or archived task for that identity instead
of creating another one.

`HostTaskManagementClaims(host)` exposes the optional `Acquire`, `Release`, and
`Transfer` commands. Each command compares the observed task and claim resource
versions and records an idempotent receipt plus audit history. The Host binds
the owner to its authenticated installation and the plugin supplies only its
opaque instance key. Claims are independent of worker assignment and never
expire into a new owner. A claim change advances its generation; exact task
updates, labels, assignment, moves, archive, relations, and message admission
must carry the current `ManagementInstanceKey` and
`ExpectedClaimGeneration`. A transfer or human takeover makes the prior
generation stale. Native task detail can inspect and transfer or release a
claim even when its plugin is unavailable.

#### Host v2 task completion gates

`HostTaskCompletionGates(host)` exposes `SetCriteria` and `Verify`. Both
commands require the task's observed resource version, completion-set revision,
stable idempotency key, approved `host.v2.write:tasks` grant, and the current
management instance key and claim generation. The Host derives the actor from
the authenticated installation and returns the canonical task-owned gate
snapshot, including criteria, typed evidence, and bounded blockers. Criteria
are limited to 64 entries. A plugin cannot supply the native human confirmation
used to remove or weaken an unmet criterion, and this extension does not expose
the one-move human completion override.
Criteria writes use the input-only `TaskCompletionCriterionInput` message;
server-owned verification and evidence fields are available only in returned
snapshots.

The Host stores command intent before the task service call. The task repository
stores the operation identity and gate snapshot in the same transaction as the
criteria or evidence change. A retry recovers that snapshot if the process stops
after the domain change but before completing the Host receipt. Stale task or
criteria revisions, evidence changes, and claim transfers return typed command
conflicts. Completion still rechecks live evidence in the final task transition
transaction.

```go
gates, supported := pluginsdk.HostTaskCompletionGates(host)
if !supported {
    return errors.New("task completion gates are unavailable")
}
result, gate, err := gates.SetCriteria(ctx, pluginsdk.ExactTaskCompletionCriteria{
    RequestID: "request-criteria-1", WorkspaceID: workspaceID, TaskID: task.ID,
    IdempotencyKey: "task-42-criteria-v1",
    ExpectedTaskResourceVersion: task.ResourceVersion,
    ExpectedRevision: 0,
    ApprovalRevision: capability.ApprovalRevision,
    ManifestDigest: capability.ManifestDigest,
    ManagementInstanceKey: instanceKey,
    ExpectedClaimGeneration: claim.Generation,
    Criteria: []pluginsdk.TaskCompletionCriterionInput{{
        ID: "tests-pass", Description: "Required checks pass",
        EvidenceSubject: pluginsdk.TaskCompletionEvidenceSubject{
            Kind: "artifact_revision", ID: "test-run-42",
        },
    }},
})
if err != nil {
    return err
}
_ = result.Status // APPLIED, ALREADY_APPLIED, CONFLICT, or DENIED
_ = gate.Blockers
```

The Host stores a command intent before the task service runs. The task service
commits its operation identity in the same transaction as the task change. A
retry after a host restart returns the original result or recovers a task commit
whose receipt was not yet completed. Reusing an idempotency key with another
payload or using a stale resource version returns a typed conflict. Revocation
and command admission share one guard; a command admitted after revocation has
no task effect.

```go
if exact, ok := pluginsdk.HostV2(host); ok {
    capability, err := exact.GetCapabilityContext(ctx, workspaceID)
    if err != nil {
        return err
    }
    task, err := host.Tasks().Get(ctx, taskID)
    if err != nil {
        return err
    }
    result, _, err := exact.UpdateTaskExact(ctx, pluginsdk.ExactTaskUpdate{
        RequestID: "request-1", WorkspaceID: workspaceID, TaskID: task.ID,
        IdempotencyKey: "stable-operation-key",
        ExpectedResourceVersion: task.ResourceVersion,
        ApprovalRevision: capability.ApprovalRevision,
        ManifestDigest: capability.ManifestDigest,
        Title: &newTitle,
    })
    if err != nil {
        return err
    }
    _ = result.Status // inspect APPLIED, ALREADY_APPLIED, CONFLICT, or DENIED
}
```

The Go SDK exposes typed `CapabilityContext`, `CommandResult`, `CommandReceipt`,
`ExactTaskUpdate`, `ExactTaskLabels`, `ExactTaskAssignment`, `ExactTaskMove`,
`ExactTaskArchive`, `ExactTaskCreate`, and `ExactTaskMessage` DTOs. The runtime-free frontend SDK exports matching
projection types for a plugin's own UI/backend boundary; browser bundles do not
receive the privileged gRPC Host or invoke exact backend commands directly.
An absent `ExactHost` means unsupported. Do not silently fall back to a v1
mutation when a caller requires exact versioning or idempotency.

Exact task creation, label replacement, human assignment, workflow moves, and
archive use
the additive task-command manager. The idempotency key must remain stable for a
retry. Source identity is workspace-scoped and includes archived tasks.

`SendMessage` on that manager requires `api_write:messages`, an active
`host.v2.write:messages` workspace grant, task and session resource versions,
and a stable idempotency key. It appends to the durable FIFO and returns the
accepted queue receipt ID. `APPLIED` confirms admission only; it does not mean
the agent received or acted on the message. A retry with the same key recovers
the same accepted receipt even if the task changes after admission.

```go
accepted, err := commands.SendMessage(ctx, pluginsdk.ExactTaskMessage{
    RequestID: "request-7", WorkspaceID: workspaceID, TaskID: task.ID,
    SessionID: session.ID, IdempotencyKey: "proposal-17-follow-up-v1",
    ExpectedTaskResourceVersion: task.ResourceVersion,
    ExpectedSessionResourceVersion: session.ResourceVersion,
    ApprovalRevision: capability.ApprovalRevision,
    ManifestDigest: capability.ManifestDigest,
    Content: "Implement the accepted proposal and report the resulting task state.",
})
if err != nil {
    return err
}
if accepted.Status != pluginsdk.CommandApplied && accepted.Status != pluginsdk.CommandAlreadyApplied {
    return fmt.Errorf("message was not accepted: %s", accepted.Status)
}
queueReceiptID := accepted.Receipt.TargetID
_ = queueReceiptID
```

```go
commands, supported := pluginsdk.HostTaskCommands(host)
if !supported {
    return errors.New("exact task commands are unavailable")
}
created, task, err := commands.CreateTask(ctx, pluginsdk.ExactTaskCreate{
    RequestID: "request-2", WorkspaceID: workspaceID,
    IdempotencyKey: "proposal-17-create", ExternalID: "coordinator:proposal-17",
    ApprovalRevision: capability.ApprovalRevision,
    ManifestDigest: capability.ManifestDigest,
    Task: pluginsdk.CreateTaskInput{
        WorkspaceID: workspaceID, WorkflowID: workflowID,
        Title: "Add coordinator API",
    },
})
if err != nil {
    return err
}
_ = created.Status // APPLIED or ALREADY_APPLIED

labelsResult, labeledTask, err := commands.SetLabels(ctx, pluginsdk.ExactTaskLabels{
    RequestID: "request-3", WorkspaceID: workspaceID, TaskID: task.ID,
    IdempotencyKey: "task-42-labels-v2",
    ExpectedResourceVersion: task.ResourceVersion,
    ApprovalRevision: capability.ApprovalRevision,
    ManifestDigest: capability.ManifestDigest,
    Labels: []string{"coordination", "urgent"},
})
if err != nil {
    return err
}
_ = labelsResult.Status

assigned, assignedTask, err := commands.Assign(ctx, pluginsdk.ExactTaskAssignment{
    RequestID: "request-4", WorkspaceID: workspaceID, TaskID: task.ID,
    IdempotencyKey: "task-42-human-owner-v1",
    ExpectedResourceVersion: labeledTask.ResourceVersion,
    ApprovalRevision: capability.ApprovalRevision,
    ManifestDigest: capability.ManifestDigest,
    AssigneeUserID: userID,
})
if err != nil {
    return err
}
_ = assigned.Status

moved, movedTask, err := commands.Move(ctx, pluginsdk.ExactTaskMove{
    RequestID: "request-5", WorkspaceID: workspaceID, TaskID: task.ID,
    IdempotencyKey: "task-42-move-review-v1",
    ExpectedResourceVersion: assignedTask.ResourceVersion,
    WorkflowID: workflowID, WorkflowStepID: reviewStepID, Position: 0,
    ApprovalRevision: capability.ApprovalRevision,
    ManifestDigest: capability.ManifestDigest,
})
if err != nil {
    return err
}
_ = moved.Status
_ = movedTask.ResourceVersion

archived, archivedTask, err := commands.Archive(ctx, pluginsdk.ExactTaskArchive{
    RequestID: "request-6", WorkspaceID: workspaceID, TaskID: task.ID,
    IdempotencyKey: "task-42-archive-v1",
    ExpectedResourceVersion: movedTask.ResourceVersion,
    ApprovalRevision: capability.ApprovalRevision,
    ManifestDigest: capability.ManifestDigest,
})
if err != nil {
    return err
}
_ = archived.Status
_ = archivedTask.ArchivedAt
```

### Host v2 execution controls and human responses

`pluginsdk.HostExecutionCommands(host)` exposes exact run and mode operations.
The Host checks installation identity, manifest declaration, workspace grant,
approval revision, and manifest digest on every call. Run-changing commands use
`host.v2.write:execution`; provider mode reads use
`host.v2.read:sessions`.

- `EnsureTaskRunExact` requires the observed task resource version and a stable
  idempotency key, then enters through normal orchestrator launch admission.
- `StopTaskRunExact` requires the observed session resource version and active
  execution ID. It stops only that ID, so a replacement execution is not
  affected.
- `RecoverSessionExact` currently accepts `resume` only. It requires the exact
  failed session revision and previous execution ID, no live replacement, and
  normal launch policy. It does not repair credentials or bypass restricted
  tool policy.
- `CancelPendingTaskTransitionExact` compares the exact transition ID and
  resource version and removes only the inspected queue record.
- `GetSessionModeContextExact` lists provider-advertised modes. The Host filters
  permission bypass and credential-repair modes.
- `SetSessionModeExact` checks the observed session and execution versions and
  accepts only a mode the current provider advertises.

Commands return typed Host results. `UNAVAILABLE` and `UNSUPPORTED` are not
permission to retry under a new key; reconcile the current resource before
starting another command.

`ListSessionsExact` returns each session's resource version, state, and opaque
current `execution_id`. Read it under `host.v2.read:sessions`; use the version
and execution ID together when building an exact stop or recovery request. The
Host rechecks both before applying the operation.

`RespondPermissionExact` and `AnswerClarificationExact` require
`api_write: interactions`, the `host.v2.write:interactions` workspace grant, an
exact pending interaction resource version, and a Host-issued one-use human
response receipt. The authenticated receipt endpoint is
`POST /api/plugins/host/interactions/response-receipts`. A user with native
`session.control` access issues a receipt for one exact permission option or
clarification answer. The receipt expires after five minutes and binds the
workspace, interaction, revision, kind, and response payload. First valid
response wins.

The legacy v1 `RespondToPermission`, `AnswerClarification`, and
`CancelClarification` RPCs remain in the wire schema for compatibility, but
the Host returns `PermissionDenied`; they cannot prove a current human
decision. Plugin authors must use the exact response commands. Native UI
interaction handling remains unchanged.

### Host v2 managed conversation lifetime

`ExactHost.ManagedAgentConversations()` provides an installation-owned
conversation manager. Its identity is the current installation, workspace, and
opaque instance key. `Ensure` creates or updates an idle conversation at its
expected revision; `Get` and `List` read only the current installation's
conversations; `SetPaused` changes desired pause state; and `Delete` removes one
conversation. Mutations require stable request and idempotency keys plus the
current approval revision and manifest digest. Every write returns a durable
Host command receipt.

Read calls require a grant for
`host.v2.read:managed_agent_conversations`. Ensure, pause, and delete require a
separate `host.v2.write:managed_agent_conversations` grant. The plugin manifest
declares both resources with `api_read` and `api_write`; approval is still
specific to the installation and workspace. The Host rejects calls from a
disabled, failed, uninstalled, or replaced installation.

Managed conversations use hidden task/session runtime storage and remain
separate from v1 `AgentConversations`. Disable closes Host admission, pauses
retained conversations, stops active runs, and preserves history; enabling does
not automatically unpause them. Host restart preserves their identity and uses
normal session reconciliation for interrupted runs. Uninstall revokes the old
identity, stops active runs, and pauses and detaches each transcript from plugin
access. A reinstall has a new installation identity and cannot adopt the old
rows. Detached transcripts are reserved for the host-owned read-only surface.
Do not use these lifecycle calls as a replacement for the v1 dispatch API;
managed input and restricted execution are separate Host extensions.

### Host v2 managed conversation inputs

`ExactHost.ManagedAgentConversations()` also exposes `EnqueueInput`, `GetInput`,
`ListInputs`, `CancelInput`, and `Dispatch`. These methods require the same
installation identity, workspace grant, approval revision, and manifest digest
as the managed conversation methods. Reads require
`host.v2.read:managed_agent_conversations`. Enqueue, cancel, and immediate
dispatch require `host.v2.write:managed_agent_conversations`.

`EnqueueInput` commits a receipt and FIFO row together before delivery. Its
occurrence key deduplicates retries within one conversation. A retry with the
same occurrence and payload returns the original receipt. A changed payload
returns a conflict. `ListInputs` uses an exclusive sequence cursor and a bounded
page. The default limit is 50 and the maximum is 200.

Receipts expose `accepted`, `running`, `completed`, `failed`, `cancelled`, and
`uncertain` states. `uncertain` means that the host cannot prove whether the
runtime started or completed the input. The host does not replay that input
automatically. Plugins must keep this result visible and reconcile through
their normal source-of-truth workflow.

`CancelInput` removes an accepted input from the queue. To cancel a running
input, pass the exact observed execution ID. The host stops only that execution
generation and marks the receipt cancelled after the stop succeeds. A stale ID
does not stop the current execution.

Only an input with `periodic` origin and a non-empty coalesce key can replace an
unreserved pending periodic input. The new input moves to the FIFO tail and
receives a new sequence. The replaced receipt is returned as cancelled with
`superseded_by_id` set. Pause blocks queue drain without removing accepted
inputs. Resume wakes the queue.

`Dispatch` is a separate immediate operation. It returns `busy`, `started`, or
`sent`. A `busy` result never means that the host queued the payload. Use
`EnqueueInput` when the caller needs durable delivery while a turn is active.

### Host v2 managed conversation schedules

`ExactHost.ManagedConversationSchedules()` provides read, create, update,
enable/disable, and delete operations for schedules owned by the current plugin
installation. Reads require `api_read: automations` and the workspace grant
`host.v2.read:automations`. Mutations require `api_write: automations` and
`host.v2.write:automations`. Each write carries the current approval revision,
manifest digest, a stable idempotency key, and the expected schedule revision
when updating an existing schedule.

Schedule DTOs carry the logical plugin and instance key for the managed
conversation destination. The Host resolves installation and conversation IDs
privately, validates that the target belongs to the schedule's current
installation, and rechecks its current approval when delivering each firing.
Delivery uses the firing's run ID as the occurrence key, so recovery reads or
retries the same durable input receipt instead of adding another input. Schedule
cleanup never deletes the retained conversation. Exported YAML carries only the
portable plugin and instance identity; import requires an explicit destination
selection in the receiving workspace.

### Managed agent tool policy

An `agent_tools[].surfaces` entry may include `managed-conversation`. Such a
declaration requires `capabilities.api_write: [managed_agent_tools]` and the
workspace grant `host.v2.write:managed_agent_tools`. Each retained conversation
selects at most 16 declared tools through `ManagedAgentConversationSpec.AgentToolNames`.
The broker lists and invokes only that installation's selected tools. It rejects
native tools, ambient MCP servers, and unrelated plugin tools for a restricted
turn.

Kandev supplies the agent-tool callback's `AgentToolContext` from the authenticated
broker context. In managed mode it includes the execution ID, installation ID,
conversation revision, approval revision, manifest digest, and selected names,
along with the task, session, workspace, and surface fields. Agent-supplied tool
arguments cannot set this context. The host checks that the execution is still
current and that the retained conversation has the same identity and policy before
each broker call. Approval changes invalidate the retained policy and request a
normal runtime stop. The plugin must refresh the conversation policy before the
host will launch another managed turn.

No current agent adapter is advertised as supporting this restriction. Managed
turn launch fails before an execution is created until an adapter passes the
launch, resume, recovery, native-tool-denial, and real provider smoke checks.

The wire descriptors expose opaque task/session handles, launch profile and
executor IDs, a revision, desired pause, and retention/detached state. They do
not expose task internals. A missing `Get` target returns `NotFound`, and a stale
revision returns a typed conflict through the command result.

### Host v2 exact workspace observations

The optional `pluginsdk.HostExactQueries(host)` extension exposes snapshot-bound
workspace reads. It is separate from the legacy `Host` readers, whose unversioned
pagination and cross-workspace behavior remain unchanged. Each exact query names
one authorized `workspace_id`; the Host derives the installation identity from
the connected plugin and rechecks the active workspace grant on every page.

The exact query manager provides:

- `ListWorkspaces`, `ListWorkflows`, and `ListWorkflowSteps`;
- `ListTasks` and `GetTask`, with canonical stored task status and the bounded
  task-status summary for activity, execution, and blockers;
- `ListSessions`, `ListPendingInteractions`, and `GetInteraction`;
- `ListSanitizedMessages`, `ListTaskInbox`, and task relation reads;
- `ListPendingTaskTransitions`, `ListChangeRequestEvidence`, and `ListTaskUsage`.

Every list accepts an `ExactReadPage`. The first request returns an opaque
`SnapshotVersion`, observation time, and receipt. Continue with both that version
and the returned cursor. The Host binds the cursor to installation, workspace,
method, filters, and approval revision. Cursors cannot widen scope or mix filters.
Snapshots expire after five minutes; each page is at most 200 rows, and one
snapshot is limited to 5,000 rows and 8 MiB. Expired or evicted snapshots return
`FailedPrecondition`; malformed cursors return `InvalidArgument`. Events remain
wake hints, so a consumer can discard its cache and read a new snapshot after a
gap or reconnect.

Task `ResourceVersion` identifies the task row used for the canonical status.
Semantic activity, last meaningful activity, execution state, and blockers come
from `internal/task/statussummary`; absent summary data is reported as unknown.
The task inbox currently combines durable pending interactions and queued
workflow moves. Pending transition versions include the current task version and
requested target. `GetCapabilityContext` marks task directive reads unsupported
until a durable directive source is available; a plugin must not infer support
from the presence of an RPC.

Change-request evidence contains Kandev's cached GitHub pull-request aggregate:
repository identity, PR head, review and check states, passing/total check counts,
unresolved threads, and last sync time. Its resource version changes with the PR
head or cached update. Missing evidence, an unsupported provider, and a failed
provider read have distinct states. This read does not grant merge, comment, or
CI-rerun authority.

Usage is an aggregate over the existing task-cost ledger. Token totals and event
counts retain the ledger's nullable-output and estimated/unpriced distinctions.
`cost_subcents` is an integer in hundredths of a cent; `currency` is `USD`.
`CostComplete` is false when an event is unpriced. `no_observations` with a null
cost means no ledger rows exist; it is not a measured free run. There is no
float-dollar conversion in the Host contract.

```go
queries, ok := pluginsdk.HostExactQueries(host)
if !ok {
    return errors.New("exact workspace observations are unsupported")
}
page, err := queries.ListTasks(ctx, pluginsdk.ExactTaskQuery{
    RequestID: "reconcile-2026-09-26-01", WorkspaceID: workspaceID,
    Filter: pluginsdk.TaskFilter{IncludeArchived: true},
    Page: pluginsdk.ExactReadPage{Limit: 100},
})
if err != nil {
    return err
}
for page.PageInfo.HasMore {
    page, err = queries.ListTasks(ctx, pluginsdk.ExactTaskQuery{
        RequestID: "reconcile-2026-09-26-02", WorkspaceID: workspaceID,
        Filter: pluginsdk.TaskFilter{IncludeArchived: true},
        Page: pluginsdk.ExactReadPage{
            Limit: 100, Cursor: page.PageInfo.NextCursor,
            SnapshotVersion: page.PageInfo.SnapshotVersion,
        },
    })
    if err != nil {
        return err
    }
}
```

Browser plugins do not receive this privileged gRPC connection. A plugin may
project observations from its Go backend to its own UI, but the TypeScript SDK
does not add a browser path for exact Host queries.

### Host v2 exact workspace administration

`pluginsdk.HostWorkspaceAdministration(host)` exposes one typed exact command
manager. It calls `ApplyWorkspaceAdministrationExact`. Each request contains
one operation from the protobuf `oneof`; the Host rejects empty or multi-field
commands.

Every command requires a manifest declaration and an active workspace grant:

- Workspace defaults require `capabilities.api_write: workspaces` and
  `host.v2.write:workspaces`.
- Workflow and workflow-step changes require `capabilities.api_write:
  workflows` and `host.v2.write:workflows`.
- Repository registration and updates require `capabilities.api_write:
  repositories` and `host.v2.write:repositories`.

Each command carries a request ID, idempotency key, approval revision, manifest
digest, and observed resource version. Reorder commands also carry the version
of every listed workflow or step. The Host stores command intent before it
calls the task or workflow service. The service write checks each version at
the database write boundary. A stale version returns `CONFLICT`. A retry with
the same key and payload returns the saved result or `ALREADY_APPLIED` after a
completed write.

The command set updates workspace defaults, creates and updates manual
workflows, reorders complete workflow lists, and creates, updates, or reorders
workflow steps. It also registers a workspace repository and updates its name,
default branch, branch prefix, branch template, or pull-before-worktree value.
The Host checks target workspace ownership and uses existing profile, workflow,
repository, and branch validation. Synchronized workflows and resources in the
reserved Improve Kandev workspace cannot be changed.

The API has no workflow, step, or repository delete operation. It does not
write repository scripts, copy-file rules, or secret bindings. Use native UI
actions for these settings. The typed step configuration supports selected
`on_enter` actions and transition moves. An update preserves native actions
that the plugin type cannot represent.

```go
exact, ok := pluginsdk.HostV2(host)
if !ok {
    return errors.New("exact Host commands are unavailable")
}
admin, ok := pluginsdk.HostWorkspaceAdministration(host)
if !ok {
    return errors.New("workspace administration is unavailable")
}
capability, err := exact.GetCapabilityContext(ctx, workspaceID)
if err != nil {
    return err
}
result, err := admin.Apply(ctx, pluginsdk.WorkspaceAdminCommand{
    RequestID: "request-workflow-1", WorkspaceID: workspaceID,
    IdempotencyKey: "workspace-workflow-build-v1",
    ApprovalRevision: capability.ApprovalRevision,
    ManifestDigest: capability.ManifestDigest,
    CreateWorkflow: &pluginsdk.WorkspaceWorkflowCreate{
        ExpectedWorkspaceResourceVersion: workspaceVersion,
        Name: "Build",
    },
})
if err != nil {
    return err
}
_ = result.Status // APPLIED, ALREADY_APPLIED, or a typed non-success result
```

### Host data API accessors (`apps/backend/pkg/pluginsdk/host.go`, `data_types.go`)

Each `Host.<Resource>()` call above returns a small reader interface. All
methods take `context.Context`; list methods take a Go-native `Page{Limit
int32; Cursor string}` and return `(items []T, *PageInfo, error)` where
`PageInfo{NextCursor string; HasMore bool}` — the Go-native mirror of the wire
`Page`/`PageInfo` messages (§3a). A resource whose capability isn't declared
still returns a non-nil reader; every method on it returns a gRPC
`PermissionDenied` error instead of a zero value.

```go
type TaskReader interface {
    List(ctx context.Context, filter TaskFilter, page Page) ([]Task, *PageInfo, error)
    Get(ctx context.Context, id string) (*Task, error)
}

type SessionReader interface {
    List(ctx context.Context, filter SessionFilter, page Page) ([]Session, *PageInfo, error)
    CodeStats(ctx context.Context, filter SessionFilter, page Page) ([]SessionCodeStats, *PageInfo, error)
}

type WorkspaceReader interface {
    List(ctx context.Context, page Page) ([]Workspace, *PageInfo, error)
}

type WorkflowReader interface {
    List(ctx context.Context, workspaceID string, page Page) ([]Workflow, *PageInfo, error)
    ListSteps(ctx context.Context, workflowID string) ([]WorkflowStep, error)
}

type AgentProfileReader interface {
    List(ctx context.Context, page Page) ([]AgentProfile, *PageInfo, error)
}

type ExecutorProfileReader interface {
    List(ctx context.Context, page Page) ([]ExecutorProfile, *PageInfo, error)
}

type RepositoryReader interface {
    List(ctx context.Context, workspaceID string, page Page) ([]Repository, *PageInfo, error)
}
```

`Task`, `Session`, `SessionCodeStats`, `Workspace`, `Workflow`, `WorkflowStep`,
`AgentProfile`, `Repository`, `TaskFilter`, `SessionFilter` are Go-native
structs in `pluginsdk` (field-for-field mirrors of the proto messages, PascalCase
Go names for the proto's snake_case fields, `*string` for `optional string`) —
authors never see the generated `pluginv1.*` types.

`Repository` additionally carries credential-free provider origin identity:
`source_type`, `provider_id`, `provider_repository_id`, `provider_host`, `provider_scope`,
`owner_or_project`, `provider_name`, and `remote_url`. The Host never exposes a
local checkout path, scripts, or credentials through this DTO.

`provider_scope` is opaque and credential-free. For provider-backed repositories,
the strong identity is workspace + provider + scope + immutable provider repository
ID. Host/name/owner fields remain routing and display metadata; scoped descriptors do
not adopt legacy unscoped rows.

**Authoring example** — a plugin declaring `api_read: ["sessions"]` and reading
computed per-session code stats instead of opening the kandev database:

```yaml
# manifest.yaml
capabilities:
  api_read: ["sessions"]
```

```go
func (p *statsPlugin) OnEvent(ctx context.Context, e *pluginsdk.Event) error {
    stats, pageInfo, err := p.host.Sessions().CodeStats(ctx, pluginsdk.SessionFilter{
        WorkspaceIDs: []string{e.WorkspaceID},
    }, pluginsdk.Page{Limit: 100})
    if err != nil {
        return err // e.g. gRPC PermissionDenied if api_read:sessions isn't declared
    }
    for _, s := range stats {
        log.Printf("session %s: +%d/-%d committed, +%d/-%d peak pending",
            s.SessionID, s.LinesAddedCommitted, s.LinesDeletedCommitted,
            s.LinesAddedPeakPending, s.LinesDeletedPeakPending)
    }
    _ = pageInfo.HasMore // paginate via pageInfo.NextCursor when true
    return nil
}
```

`kandev-plugin-agent-stats` is the plugin ADR 0043 was written for: it
originally opened `~/.kandev/data/kandev.db` read-only and hand-aggregated
`task_session_commits`/`task_session_git_snapshots` to get exactly the numbers
`Sessions().CodeStats(...)` now returns as a stable, computed DTO — read via
the API, never the DB.

## 5. Delivery / webhooks semantics (unchanged from HTTP era)

- **DeliverEvent**: unary. Per-plugin sequential queue, 10s timeout, 3 retries
  (5s/15s/45s, injectable), ring buffer 100/5min while plugin unhealthy, flush
  in order on recovery. Non-nil error or timeout counts as failure.
- **HandleWebhook**: kandev's HTTP endpoint `POST /api/plugins/{id}/webhooks/{key}`
  converts the HTTP request to WebhookRequest and relays the WebhookResponse.
- **Health**: go-plugin client `Ping()` every 30s (injectable), 3 consecutive
  failures → status `error` (+ restart attempt with backoff), recovery → `active`
  - delivery flush. Crash (process exit) → immediate restart with backoff
    (max 5 attempts, then `error`).
- **Capability gating**: each Host RPC checks the plugin's manifest capabilities
  before doing any work — `state` for `GetState`/`SetState`/`DeleteState`/
  `ListState`, `secrets` for `RevealSecret`, `api_read:<resource>` for each Host
  data API read RPC (§3a), `api_write:tasks` for `CreateTask`/`UpdateTask` and
  `api_write:messages` for `SendMessage` — and returns PermissionDenied with
  `capability '<name>' not declared` on a miss. `EmitEvent` is ungated. Reads
  and writes gate independently on the same resource, so a plugin can declare
  `api_read:tasks` without `api_write:tasks` (or vice versa), and likewise for
  `messages`. Exact workspace administration also requires the matching
  manifest `api_write` resource, an active `host.v2.write:<resource>` workspace
  grant, and the current approval revision and manifest digest. A denied exact
  command returns a typed `DENIED` result.

## 6. Package format (`<id>-<version>.tar.gz`)

```
manifest.yaml                      # authoritative; read BEFORE any code runs
server/plugin-<goos>-<goarch>[.exe]  # any subset; host platform key required at install
ui/bundle.js                       # optional (frontend half)
ui/*.css / assets/icon.svg         # optional
checksums.txt                      # "sha256  path" for every other file
checksums.txt.sig                  # OPTIONAL ed25519 signature (unsigned → warn)
```

Manifest additions (replaces base_url; endpoints block is REMOVED):

```yaml
runtime:
  type: binary
  executables:
    linux-amd64: server/plugin-linux-amd64
    darwin-arm64: server/plugin-darwin-arm64
    # ... any subset
min_kandev_version: "0.78.0" # optional
```

Install pipeline: `POST /api/plugins/install` with JSON `{"url": "..."}` OR
multipart field `package` → verify checksums.txt covers all files & hashes match
→ parse+validate manifest (host platform key present; id pattern; capabilities)
→ extract to `~/.kandev/plugins/<id>/<version>/` → write record → status
`registered` → spawn → handshake OK → `active`. Record keeps `version` and
`install_path`. Uninstall stops the process and removes record + versions + data
(24h grace not required for v1). `POST /api/plugins/register` is REMOVED.

## 7. Frontend deltas (PLUGIN-API.md otherwise unchanged)

- `GET /api/plugins/{id}/bundle` and `/api/plugins/{id}/ui/*` are served by
  kandev **from the extracted package dir** (no reverse proxy, no upstream).
- Management page: "Register plugin" (manifest paste) is replaced by "Install
  plugin" (URL input + file upload). No credentials are ever displayed.
- Boot payload `plugins: [{id,name,bundleUrl,styleUrls,repositoryProviderIds?}]`.
  `repositoryProviderIds` is JSON camelCase copied from manifest
  `repository_providers`; frontend loader records it before bundle initialization so
  provider/review registration can enforce declared ownership. Omission remains
  compatible with older payloads. Failed or timed-out initialization rolls back partial
  registrations and fences late callbacks from the expired activation attempt.

```

```
