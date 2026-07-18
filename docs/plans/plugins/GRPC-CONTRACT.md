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
  rpc ListRepositories(ListRepositoriesRequest) returns (ListRepositoriesResponse);
  rpc ListSessions(ListSessionsRequest) returns (ListSessionsResponse);
  rpc ListSessionCodeStats(ListSessionCodeStatsRequest) returns (ListSessionCodeStatsResponse);

  // Host data API — writes, capability api_write:<resource>. Declared in the
  // proto for a stable contract; handlers are NOT wired yet (§3a "Deferred
  // writes") — calling any of these three today returns gRPC Unimplemented
  // regardless of capabilities.
  rpc CreateTask(CreateTaskRequest) returns (Task);
  rpc UpdateTask(UpdateTaskRequest) returns (Task);
  rpc CreateComment(CreateCommentRequest) returns (Comment);
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

### 3a. Host data API (ADR 0043)

Read/write RPCs let plugins read (and, later, write) kandev's own domain data —
tasks, sessions, workspaces, workflows, agent profiles, repositories, comments —
over the same Host gRPC channel used for state/secrets, instead of opening the
kandev database file directly. Full message definitions (`Page`, `PageInfo`,
`Task`, `TaskFilter`, `Workspace`, `Workflow`, `WorkflowStep`, `AgentProfile`,
`Repository`, `Session`, `SessionFilter`, `SessionCodeStats`, and the deferred
`CreateTaskRequest`/`UpdateTaskRequest`/`Comment`/`CreateCommentRequest`) live in
the real proto — `apps/backend/proto/kandev/plugin/v1/plugin.proto` — and are not
duplicated here; this section covers the RPC list (added to `service Host` above),
capability gating, and cross-cutting conventions. See ADR 0043
(`docs/decisions/0043-plugin-host-data-api.md`) for the design rationale.

**Readable resources.** Each read RPC requires `api_read:<resource>` in the
plugin's manifest:

| RPC | Capability | Resource |
|---|---|---|
| `ListTasks` / `GetTask` | `api_read:tasks` | tasks |
| `ListWorkspaces` | `api_read:workspaces` | workspaces |
| `ListWorkflows` | `api_read:workflows` | workflows |
| `ListWorkflowSteps` | `api_read:workflows` | workflows |
| `ListAgentProfiles` | `api_read:agent_profiles` | agent_profiles |
| `ListRepositories` | `api_read:repositories` | repositories |
| `ListSessions` | `api_read:sessions` | sessions |
| `ListSessionCodeStats` | `api_read:sessions` | sessions |

An undeclared capability returns gRPC `PermissionDenied` with message
`capability 'api_read:tasks' not declared` (substituting the actual resource) —
identical in shape to the existing state/secrets gating. Declaring the resource
grants every RPC listed against it; there is no finer-grained gate within a
resource (e.g. `api_read:workflows` covers both `ListWorkflows` and
`ListWorkflowSteps`).

**Deferred writes.** `CreateTask`, `UpdateTask`, and `CreateComment` are declared
on `service Host` and in the proto (frozen shape for `api_write:tasks` /
`api_write:comments`) but have no server-side handler yet — calling any of them
returns gRPC `Unimplemented` today regardless of what `api_write` declares. When
implemented, writes will route through the task service's `CreateTask`/
`UpdateTask`/comment-create methods (never a repository) so the standard
`task.*` events fire, and the server will stamp `source = "plugin:<id>"` on the
created row — a plugin cannot set it itself.

**Reads go through the service layer, never a repository.** Each read handler
calls the relevant internal service (task service, workflow service, the
analytics service for `ListSessionCodeStats`) so derived fields and future
access rules stay centralized, exactly as writes will route through
event-publishing service methods once implemented.

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
func Serve(p Plugin, opts ...Option)     // blocks; wires go-plugin server + broker
// Optional embeddable no-op base: sdk.UnimplementedPlugin
// Optional embeddable no-op base for Host data accessors (every method
// PermissionDenied/Unimplemented): sdk.UnimplementedHostData — used on the
// kandev side, not by plugin authors.
```

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

type RepositoryReader interface {
    List(ctx context.Context, workspaceID string, page Page) ([]Repository, *PageInfo, error)
}
```

`Task`, `Session`, `SessionCodeStats`, `Workspace`, `Workflow`, `WorkflowStep`,
`AgentProfile`, `Repository`, `TaskFilter`, `SessionFilter` are Go-native
structs in `pluginsdk` (field-for-field mirrors of the proto messages, PascalCase
Go names for the proto's snake_case fields, `*string` for `optional string`) —
authors never see the generated `pluginv1.*` types.

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
  + delivery flush. Crash (process exit) → immediate restart with backoff
  (max 5 attempts, then `error`).
- **Capability gating**: each Host RPC checks the plugin's manifest capabilities
  before doing any work — `state` for `GetState`/`SetState`/`DeleteState`/
  `ListState`, `secrets` for `RevealSecret`, `api_read:<resource>` for each Host
  data API read RPC (§3a) — and returns PermissionDenied with
  `capability '<name>' not declared` on a miss. `EmitEvent` is ungated.
  `api_write:<resource>` is accepted in the manifest but the write RPCs
  (`CreateTask`/`UpdateTask`/`CreateComment`) aren't implemented yet, so they
  return gRPC `Unimplemented` regardless of capability.

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
min_kandev_version: "0.78.0"     # optional
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
- Boot payload `plugins: [{id,name,bundleUrl,styleUrls}]` unchanged.
```
