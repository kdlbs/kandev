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
  rpc InvokeTool(ToolRequest) returns (ToolResponse);
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
}

message Event {
  string event_id = 1;                     // fresh uuid per delivery
  string event_type = 2;                   // bus subject, e.g. "task.created"
  string occurred_at = 3;                  // RFC3339 UTC
  string workspace_id = 4;                 // empty if not derivable
  google.protobuf.Struct payload = 5;      // marshaled bus event.Data
}
message EventAck {}

message ToolRequest {
  string tool_call_id = 1;
  string tool_name = 2;
  google.protobuf.Struct input = 3;
  ToolContext context = 4;
}
message ToolContext { string task_id = 1; string agent_instance_id = 2; string session_id = 3; }
message ToolResponse { google.protobuf.Struct output = 1; string error = 2; }

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

message RevealSecretRequest { string ref = 1; }
message RevealSecretResponse { string value = 1; }

message EmitEventRequest { string event_name = 1; google.protobuf.Struct payload = 2; }
message EmitEventResponse {}
```

Notes: scope ∈ instance|workspace|task|agent (empty scope_id for instance —
matches the state store). The plugin never passes its own id; the Host service
instance is bound to the plugin's record at spawn time.

## 4. SDK (`apps/backend/pkg/pluginsdk`)

Public Go module surface (authors import only this):

```go
type Plugin interface {
    OnEvent(ctx context.Context, e *Event) error            // return err → kandev retries
    InvokeTool(ctx context.Context, req *ToolRequest) (*ToolResponse, error)
    HandleWebhook(ctx context.Context, req *WebhookRequest) (*WebhookResponse, error)
}
type Host interface {                                        // injected before Serve returns
    GetState/SetState/DeleteState/ListState(...)
    RevealSecret(ctx, ref string) (string, error)
    EmitEvent(ctx, name string, payload map[string]any) error
}
func Serve(p Plugin, opts ...Option)     // blocks; wires go-plugin server + broker
// Optional embeddable no-op base: sdk.UnimplementedPlugin
```

SDK types mirror proto but use `map[string]any` for Struct fields. The SDK owns
all go-plugin/grpc plumbing (handshake, broker for Host, conversions).

## 5. Delivery / tools / webhooks semantics (unchanged from HTTP era)

- **DeliverEvent**: unary. Per-plugin sequential queue, 10s timeout, 3 retries
  (5s/15s/45s, injectable), ring buffer 100/5min while plugin unhealthy, flush
  in order on recovery. Non-nil error or timeout counts as failure.
- **InvokeTool**: 30s timeout.
- **HandleWebhook**: kandev's HTTP endpoint `POST /api/plugins/{id}/webhooks/{key}`
  converts the HTTP request to WebhookRequest and relays the WebhookResponse.
- **Health**: go-plugin client `Ping()` every 30s (injectable), 3 consecutive
  failures → status `error` (+ restart attempt with backoff), recovery → `active`
  + delivery flush. Crash (process exit) → immediate restart with backoff
  (max 5 attempts, then `error`).
- **Capability gating**: a unary server interceptor on Host checks the plugin's
  manifest capabilities (state, secrets; api_read/api_write reserved) and returns
  PermissionDenied with `capability '<name>' not declared`.

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
