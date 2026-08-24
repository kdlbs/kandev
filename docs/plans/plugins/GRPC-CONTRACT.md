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
}

// Implemented by KANDEV (served back over the go-plugin broker).
// Every RPC is capability-gated server-side (§5).
service Host {
  rpc GetState(GetStateRequest) returns (GetStateResponse);
  rpc SetState(SetStateRequest) returns (SetStateResponse);
  rpc DeleteState(DeleteStateRequest) returns (DeleteStateResponse);
  rpc ListState(ListStateRequest) returns (ListStateResponse);
  rpc RevealSecret(RevealSecretRequest) returns (RevealSecretResponse);
  rpc EmitEvent(EmitEventRequest) returns (EmitEventResponse);

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
```

Notes: scope ∈ instance|workspace|task|agent (empty scope_id for instance —
matches the state store). The plugin never passes its own id; the Host service
instance is bound to the plugin's record at spawn time.

`DeletePluginOwnedTaskTree` is partial-progress aware. A successful response
carries every deleted task ID. If deletion stops after removing descendants,
the non-OK status includes a `DeletePluginOwnedTaskTreeProgress` detail with
those IDs. The Go SDK preserves them in the returned `([]string, error)` so a
plugin can reconcile completed deletions before retrying idempotent cleanup;
callers must not discard progress merely because the error is non-nil. An absent root
is a successful no-op, so a retry after a completed or externally removed tree remains
safe.

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

**Readable resources.** Each read RPC requires `api_read:<resource>` in the
plugin's manifest:

| RPC                     | Capability                   | Resource          |
| ----------------------- | ---------------------------- | ----------------- |
| `ListTasks` / `GetTask` | `api_read:tasks`             | tasks             |
| `ListWorkspaces`        | `api_read:workspaces`        | workspaces        |
| `ListWorkflows`         | `api_read:workflows`         | workflows         |
| `ListWorkflowSteps`     | `api_read:workflows`         | workflows         |
| `ListAgentProfiles`     | `api_read:agent_profiles`    | agent_profiles    |
| `ListExecutorProfiles`  | `api_read:executor_profiles` | executor_profiles |
| `ListRepositories`      | `api_read:repositories`      | repositories      |
| `ListSessions`          | `api_read:sessions`          | sessions          |
| `ListSessionCodeStats`  | `api_read:sessions`          | sessions          |
| `ListMessages`          | `api_read:messages`          | messages          |
| `ListPendingInteractions` / `GetInteraction` | `api_read:interactions` | interactions |

An undeclared capability returns gRPC `PermissionDenied` with message
`capability 'api_read:tasks' not declared` (substituting the actual resource) —
identical in shape to the existing state/secrets gating. Declaring the resource
grants every RPC listed against it; there is no finer-grained gate within a
resource (e.g. `api_read:workflows` covers both `ListWorkflows` and
`ListWorkflowSteps`).

**Writes.** `CreateTask`, `UpdateTask`, and `SendMessage` are implemented.
`CreateTask`/`UpdateTask` are gated by `api_write:tasks` and route through
`internal/task/service` (never a repository), so `task.*` events fire and
WS-driven UI stays in sync. The server stamps `source = "plugin:<id>"` on the
created task's metadata — a plugin cannot set it itself. `CreateTask` resolves
sane placement defaults when the plugin omits them: an empty `workspace_id`
resolves to the single workspace (ambiguous otherwise → `InvalidArgument`), an
empty `workflow_id` to that workspace's first workflow. `UpdateTask` accepts a
conservative field mask — `title`, `description`, `state`, `workflow_step_id`
(each optional/leave-unset). `start_agent` best-effort auto-launches an agent
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

The three writes route through the first-party services the native UI drives:
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

SDK types mirror proto but use `map[string]any` for Struct fields. The SDK owns
all go-plugin/grpc plumbing (handshake, broker for Host, conversions).

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
  `messages`.

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
