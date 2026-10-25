---
status: draft
system: agents
requirements:
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003
  - REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004
---

# OpenAI-compatible AI providers System Design

## Purpose and boundaries

The agent system owns the agent profile and the per-agent capability surface,
so it owns the provider primitive and the translation of that primitive into
each CLI's configuration. This design uses, but does not own:

- **Secrets** (`internal/secrets`, `ProfileEnvVar.SecretID` machinery) for the
  API-key value.
- **agentctl process/adapter launch** and the **one-shot ACP inference/probe**
  path (`internal/agentctl/server/utility`, `internal/agentctl/server/process`)
  as the injection sites.
- **agentctl ACP adapter** (`internal/agentctl/server/adapter/transport/acp`) as
  the site that advertises the gateway auth capability and replays the profile's
  `authenticate` call.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001` | [Data and contracts](#data-and-contracts), [Persistence](#persistence) |
| `REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002` | [Provider injection](#provider-injection), [Control flow](#control-flow) |
| `REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003` | [Probe and inference reach](#probe-and-inference-reach) |
| `REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004` | [Credential delivery](#credential-delivery) |

## Components and responsibilities

- **`AgentProfile` model + settings repo** (`internal/agent/settings`): stores
  and validates the three new provider fields; persists them as three real
  columns via idempotent `ADD COLUMN` migrations (see [Persistence](#persistence)).
- **Agent capability**: a new optional method on the agent interface,
  `OpenAICompatibleProvider() *OpenAICompatibleProviderSpec` (nil = unsupported).
  `CodexACP` returns a non-nil spec; every other agent returns nil initially.
- **`acpprovider` package** (`internal/common/acpprovider/`): tier-neutral data
  primitive. `BuildGatewayAuth` turns a spec + base URL + revealed key into a
  `GatewayAuth{ MethodID, Meta }` value; `ValidateBaseURL` /
  `ValidateCredentialedBaseURL` are the shared save-time and launch-time URL
  checks. No I/O, no `*Manager`; neither the backend nor agentctl imports the
  other.
- **Profile resolver / lifecycle** (`internal/agent/runtime/lifecycle`):
  `Manager.resolveProviderGatewayAuth` reveals the key, validates the base URL,
  adapts a loopback host to the launching runtime, and produces the `GatewayAuth`
  that rides through `CreateInstanceRequest` to agentctl.
- **agentctl ACP adapter** (`internal/agentctl/server/adapter/transport/acp`):
  advertises `clientCapabilities.auth._meta.gateway=true` in `initialize` and,
  for an `openai_compatible` launch, issues the `authenticate` gateway call right
  after the `initialize` round-trip.
- **Utility path** (`internal/agentctl/server/utility` + the backend caller in
  `lifecycle/utility.go`): carries the same `GatewayAuth` through
  `InferenceConfigDTO` and applies the identical `authenticate` in
  `acp_executor.go` for the inference and probe subprocesses.
- **Frontend profile editor** (`apps/web`): provider-kind select, base-URL
  input, API-key secret picker, gated on the capability flag; client-side
  absolute-URL and no-slash-in-model validation.

## Provider injection

Verified against `@agentclientprotocol/codex-acp` 1.7.0 / `@openai/codex`
0.151.0: codex-acp does **not** parse or forward CLI arguments, so provider
configuration is delivered through codex-acp's first-class ACP **"gateway"
provider**, not `-c` overrides and not a `config.toml` fragment.

- `getCodexAuthMethods` adds `GatewayAuthMethod` (`id: "gateway"`) to the
  `initialize` response **only when** the client advertises
  `clientCapabilities.auth._meta.gateway === true`.
- Kandev then sends `authenticate({ methodId: "gateway", _meta: { gateway: {
  baseUrl, headers, providerName } } })`. `authenticateWithGateway` applies it
  live (`restartRequired: "false"`); `apiType` is fixed to `openai` →
  `wire_api: "responses"`.
- The **API key travels in `headers`** (`{"Authorization": "Bearer <key>"}`),
  never a file and never a `ps`-visible arg. `OPENAI_API_KEY` / `CODEX_API_KEY`
  in the process env stay a fallback codex-acp reads on its own.
- codex-acp also exposes runtime `providers/list` / `providers/set` /
  `providers/disable` RPCs over ACP for the same gateway config.

The provider primitive lives on the profile (task-01).
`acpprovider.BuildGatewayAuth` turns it into a `GatewayAuth{ MethodID, Meta }`
value. The agentctl ACP adapter advertises `auth._meta.gateway=true` and, after
`initialize`, issues the `authenticate` gateway call whenever the launching
profile is `openai_compatible`. No host file is materialized and the path is
identical under the standalone and Docker executors.

`OpenAICompatibleProviderSpec` (Codex value,
`internal/agent/agents/openai_compatible_provider.go`):

```go
OpenAICompatibleProviderSpec{
    AuthMethodID: "gateway",
    ProviderName: "Kandev",
    KeyEnvVar:    "OPENAI_API_KEY",
}
```

`acpprovider.BuildGatewayAuth(spec.AuthMethodID, spec.ProviderName, baseURL,
revealedKey)` returns `GatewayAuth{ MethodID: "gateway", Meta }` where `Meta` is
`{"gateway": {"baseUrl": B, "providerName": "Kandev", "headers":
{"Authorization": "Bearer <key>"}}}`. `headers` is omitted when the profile
references no key, and `providerName` when the spec leaves it blank. The model id
is applied through the existing profile `Model` / session-model path unchanged,
so no host file is written (satisfies AC-002.1, AC-002.3). `ProviderName` never
equals a reserved built-in id (AC-002.2).

Adding another agent later = implement `OpenAICompatibleProvider()` with its own
spec. A CLI that authenticates differently (env-only when it reads
`OPENAI_BASE_URL`, a generated config fragment when it requires a file) renders
from the same profile primitive; the file case uses the session's existing
isolated home dir (`SessionDirTemplate`), never `$HOME` (AC-002.4).

## Data and contracts

New `AgentProfile` fields, persisted as three real columns via idempotent
`ALTER TABLE ... ADD COLUMN` migrations following the `command_prefix` /
`fallback_model` precedent (added after the table-recreation block, mirrored in
`CREATE TABLE`, the recreation copy list, insert, update, and `scanAgentProfile`):

```go
// ProviderKind is "" / "native" (default) or "openai_compatible".
ProviderKind string `json:"provider_kind,omitempty"`
// ProviderBaseURL is the absolute http(s) endpoint root, e.g.
// "http://localhost:20128/v1". Required when ProviderKind == openai_compatible.
ProviderBaseURL string `json:"provider_base_url,omitempty"`
// ProviderAPIKeySecretID references a Kandev secret holding the bearer key.
ProviderAPIKeySecretID string `json:"provider_api_key_secret_id,omitempty"`
```

API projection (`pkg/api/v1`) adds `provider_kind`, `provider_base_url`,
`provider_api_key_secret_id` (id only, never the value) plus a read-only
`provider_supported bool` derived from the agent capability for the editor to
gate on.

Validation (settings service, shared helper so create/update/duplicate agree):

- `provider_kind == openai_compatible` requires `provider_supported`,
  a `provider_base_url` that `url.Parse`s to an absolute `http`/`https` URL, and
  a `Model` without `/` (AC-001.2, AC-001.5, AC-003 setup).
- When `provider_api_key_secret_id` is set, the base URL check is the stricter
  `acpprovider.ValidateCredentialedBaseURL`: `http` is allowed only to a
  loopback host, everything else must be `https`, so the bearer key is never
  sent in cleartext (AC-001.6). The runtime resolver
  (`Manager.resolveProviderGatewayAuth`) applies the same check before building
  the `authenticate` params, so a legacy row cannot bypass it.
- `provider_kind != openai_compatible` clears the other two fields on write so
  stored values cannot linger active (AC-001.4).

## Control flow

Live session start (`lifecycle` → agentctl ACP adapter):

1. Resolve profile. If `ProviderKind == openai_compatible` and the agent spec is
   non-nil, `Manager.resolveProviderGatewayAuth` reveals
   `ProviderAPIKeySecretID` via the existing global-secret reveal.
2. It validates the base URL (`ValidateCredentialedBaseURL` when a key is set,
   `ValidateBaseURL` otherwise) and adapts a loopback host to the launching
   runtime (see [Executor reachability](#executor-reachability)).
3. `acpprovider.BuildGatewayAuth(...)` → `GatewayAuth`, carried to agentctl in
   `CreateInstanceRequest.ProviderGatewayAuth`.
4. The ACP adapter advertises `auth._meta.gateway=true` in `initialize`, then
   issues `authenticate(gateway)` before the first `session/new`; auth failure
   aborts the launch.
5. Reveal failure, empty base URL, or an agent with no provider spec → abort
   start with a sanitized `PROVIDER_MISCONFIGURED` error; never continue to the
   vendor default (AC-002.5).

Data crossing boundaries: `GatewayAuth` travels to agentctl through the
`CreateInstanceRequest.ProviderGatewayAuth` field. The revealed key rides inside
its `Meta` headers and, as codex-acp's own fallback, in the subprocess env as
`OPENAI_API_KEY` exactly as for native Codex.

## Probe and inference reach

`InferenceConfigDTO` gains `ProviderGatewayAuth *acpprovider.GatewayAuth` and
keeps using `Env`. The profile-scoped backend caller
(`lifecycle/utility.go: ExecuteInferenceProfilePrompt`) calls the shared
`Manager.resolveProviderGatewayAuth` when the resolved profile is
`openai_compatible`, populating the field and exporting the revealed key as
`OPENAI_API_KEY`; an `openai_compatible` profile pointed at an agent with no
provider spec fails closed with `ErrProviderMisconfigured` (AC-003.2). The
utility ACP executor (`acp_executor.go`) advertises the gateway client
capability and sends `authenticate(gateway)` right after `initialize` in both
`executeACPSession` and `probeACPSessionWithContext`; a failure aborts rather
than falling back to the vendor endpoint (AC-003.1, AC-003.2). Provider profiles
take their model as free-text (AC-001.2), so there is no per-profile model probe
to wire and none is required for the model picker; the probe executor accepts
`ProviderGatewayAuth` and applies the same gateway `authenticate`, so a
profile-scoped probe caller added later needs no new plumbing (AC-003.1).

Upstream failure surfacing: `describeACPFailure` already special-cases
timeout/cancel; `withUpstreamHint` additionally appends a sanitized HTTP status
line (allowlisted `4xx`/`5xx` shapes only, tmp paths scrubbed) from the child's
stderr tail when the call was routed through a provider gateway, so a provider
`401` is legible instead of only "peer disconnected" (AC-003.3).

## Credential delivery

Two corrections in `internal/agent/runtime/lifecycle/profile_env.go`:

1. `resolveAgentProfileEnvVars`: on a single non-cancel reveal failure, log
   `warn` with the key and **skip that entry**, returning the partial map
   instead of `ErrProfileSecretUnavailable` for the whole set. Cancellation
   errors still propagate unchanged. The provider key is revealed on its own
   dedicated path (step 1 above), so a required-key failure still aborts the
   launch there (AC-004.1).
2. `mergeEnvFillMissing` gains a `reserved []string` parameter (the provider key
   env var from the spec). Keys in `reserved` are written even when already
   present in `dst`; all other keys keep fill-missing semantics. Only the
   provider injection passes a non-empty `reserved` set (AC-004.2, AC-004.3).

## Failure and recovery

| Condition | Behavior |
| --- | --- |
| Secret store absent / reveal fails for the provider key | `PROVIDER_MISCONFIGURED`, launch aborts, no vendor fallback |
| One unrelated profile env secret fails | that entry dropped + warn log; launch proceeds |
| Base URL empty or not absolute http(s) | rejected at save; at launch (legacy row) → `PROVIDER_MISCONFIGURED` |
| Cleartext `http` base URL to a non-loopback host **with** an API key | rejected at save; at launch (legacy row) → `PROVIDER_MISCONFIGURED` (bearer key would go in the clear) |
| Model contains `/` | rejected at save; at launch → `PROVIDER_MISCONFIGURED` |
| Provider returns 401/5xx during probe/inference | sanitized upstream status in the error, retryable |
| Agent has no `OpenAICompatibleProvider()` but row has stale fields | fields inert; `native` behavior |
| Loopback base URL, local Docker executor | base URL host rewritten to `host.docker.internal` at launch (the agent container also gets the `host.docker.internal:host-gateway` alias so this resolves on Linux) |
| Loopback base URL, remote Docker / Sprites executor | `PROVIDER_MISCONFIGURED` at launch: the developer's loopback is unreachable and there is no sane rewrite; the error tells the user to use a routable host |

## Executor reachability

The gateway base URL is dereferenced by the agent process, whose network
namespace depends on the executor. `Manager.resolveProviderGatewayAuth` adapts a
loopback URL (`localhost`, `127.0.0.0/8`, `::1`) to the launching runtime
(`internal/common/acpprovider`: `IsLoopbackBaseURL`,
`RewriteLoopbackHostForDocker`):

- **Standalone / SSH host runtimes:** unchanged, the agent shares the host's
  loopback.
- **Local Docker:** the host is rewritten to `host.docker.internal`, and every
  agent container is created with `--add-host host.docker.internal:host-gateway`
  (`docker.ContainerConfig.ExtraHosts`) so the alias resolves on Linux as it
  already does on Docker Desktop. Requires Docker Engine 20.10+.
- **Remote Docker / Sprites:** a loopback URL is rejected with
  `PROVIDER_MISCONFIGURED` rather than silently pointing the agent at the remote
  box's own loopback.

Non-loopback URLs are never modified.

## Persistence

Three idempotent `ADD COLUMN` migrations (`provider_kind`, `provider_base_url`,
`provider_api_key_secret_id`, all `TEXT NOT NULL DEFAULT ''`). Existing rows read
back as `''` = `native`. The API-key value is never stored on the profile; only
the secret id is. Restart behavior is unchanged — injection is recomputed from
the profile and the secret store on each launch. Postgres schema test
(`store/postgres_schema_test.go`) and SQLite migration/replay tests cover the
new columns.

## Security

- The API key is a Kandev secret, revealed only into the target subprocess env,
  never returned by any profile API, never written to a file on disk for the
  Codex path.
- **Transport for the credentialed gateway:** when an API key is referenced the
  base URL must be `https` or a loopback host, enforced at save
  (`normalizeProviderConfig`) and again at launch
  (`resolveProviderGatewayAuth`), both via
  `acpprovider.ValidateCredentialedBaseURL` so the two cannot drift. This stops
  the bearer key from being sent over cleartext `http`. Kandev validates the
  configured URL only; the agent's own HTTP client governs redirect following,
  which is why a loopback rewrite target (`host.docker.internal`) is not treated
  as credential-safe for a non-loopback original.
- Base URL is operator-supplied; it is not fetched or probed by the backend
  outside the agent subprocess, so it is not an SSRF vector beyond what the
  agent CLI already does with its own config.
- Error text reaching the client is sanitized (no key, no tmp paths), matching
  the existing utility-path redaction rules.
- No change to per-user profile scoping; provider fields follow the profile's
  existing workspace/owner scoping.

## Observability

- `warn` log on a dropped profile env secret, with the key name.
- `info` log at session start when provider injection is applied:
  `provider_kind`, `provider_id`, redacted base URL host, agent id. No key.
- Reuse existing ACP frame debug logging; the `authenticate` gateway frame (key
  redacted) rides the ACP frame log already emitted for agent subprocesses.

## Related decisions

- No ADR required: this extends the existing profile + capability pattern and
  adds no new architectural boundary. Record one only if a second agent needs a
  materially different injection contract.
