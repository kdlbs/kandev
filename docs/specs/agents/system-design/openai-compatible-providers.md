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
- **MCP passthrough `-c` override strategy** (`internal/agent/mcpconfig`) as the
  existing pattern for Codex command-line configuration overrides.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-001` | [Data and contracts](#data-and-contracts), [Persistence](#persistence) |
| `REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-002` | [Provider injection](#provider-injection), [Control flow](#control-flow) |
| `REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-003` | [Probe and inference reach](#probe-and-inference-reach) |
| `REQ-AGENTS-OPENAI-COMPATIBLE-PROVIDERS-004` | [Credential delivery](#credential-delivery) |

## Components and responsibilities

- **`AgentProfile` model + settings repo** (`internal/agent/settings`): stores
  and validates the three new provider fields; keeps them in the existing
  JSON-encoded profile blob so no column migration is needed.
- **Agent capability**: a new optional method on the agent interface,
  `OpenAICompatibleProvider() *OpenAICompatibleProviderSpec` (nil = unsupported).
  `CodexACP` returns a non-nil spec; every other agent returns nil initially.
- **`providerinject` package** (new, `internal/agent/providerinject/`): pure
  function `Build(spec, profileProvider, revealedKey) (Injection, error)` where
  `Injection` is `{ CLIArgs []string; Env map[string]string; ReservedKeys
  []string }`. No I/O, no `*Manager`. Mirrors the `mcpconfig` strategy shape.
- **Profile resolver / lifecycle** (`internal/agent/runtime/lifecycle`): calls
  `providerinject.Build`, merges `CLIArgs` into the agent argv (alongside the
  existing MCP `-c` args and `CLIFlags`), and merges `Env` with
  provider-key-wins precedence.
- **Utility path** (`internal/agentctl/server/utility` + the backend caller in
  `lifecycle/utility.go`): carries the same `CLIArgs` / `Env` through
  `InferenceConfigDTO` and applies them in `acp_executor.go`.
- **Frontend profile editor** (`apps/web`): provider-kind select, base-URL
  input, API-key secret picker, gated on the capability flag; client-side
  absolute-URL and no-slash-in-model validation.

## Provider injection

> **Design update (2026-08-31, verified against `@agentclientprotocol/codex-acp`
> 1.7.0 / `@openai/codex` 0.151.0):** codex-acp does **not** parse or forward
> CLI arguments, so the `-c model_provider*` override approach below is dead for
> the ACP path. Instead codex-acp exposes a **first-class ACP "gateway"
> provider**:
>
> - `getCodexAuthMethods` adds `GatewayAuthMethod` (`id: "gateway"`) to the
>   `initialize` response **only when** the client advertises
>   `clientCapabilities.auth._meta.gateway === true`.
> - Kandev then sends `authenticate({ methodId: "gateway", _meta: { gateway: {
>   baseUrl, headers, providerName } } })`. `authenticateWithGateway` applies it
>   live (`restartRequired: "false"`); `apiType` is fixed to `openai` →
>   `wire_api: "responses"`.
> - The **API key travels in `headers`** (`{"Authorization": "Bearer <key>"}`),
>   not a file and not `ps`-visible args. `OPENAI_API_KEY` / `CODEX_API_KEY` in
>   the process env remain a fallback codex-acp reads on its own.
> - codex-acp also exposes runtime `providers/list` / `providers/set` /
>   `providers/disable` RPCs over ACP for the same gateway config.
>
> **Revised injection model:** the primitive still lives on the profile
> (task-01, unchanged). `providerinject.Build` produces an
> `ACPGatewayAuth{ MethodID, Meta }` value (base URL + `Authorization` header +
> provider name), not CLI args. The agentctl ACP adapter advertises
> `auth._meta.gateway=true` and, after `initialize`, issues the `authenticate`
> gateway call whenever the launching profile is `openai_compatible`. This
> removes the isolated-home file-materialization work entirely and works
> identically under the standalone and Docker executors. task-02/03/04 are
> re-scoped around this; task-01 and task-05 are unaffected.

The historical CLI-override design is retained below for context only.

`OpenAICompatibleProviderSpec` (Codex value):

```go
OpenAICompatibleProviderSpec{
    // Fixed, non-reserved id used for the synthesized provider block.
    ProviderID: "kandev_openai_compat",
    WireAPI:    "responses", // current codex-acp only accepts "responses"
    KeyEnvVar:  "OPENAI_API_KEY",
    // How Build renders the injection.
    Render: RenderCodexConfigOverrides,
}
```

`RenderCodexConfigOverrides` produces, for base URL `B` and provider id `P`:

```
-c model_provider="P"
-c model_providers.P.name="kandev-openai-compat"
-c model_providers.P.base_url="B"
-c model_providers.P.wire_api="responses"
-c model_providers.P.env_key="OPENAI_API_KEY"
```

and `Env["OPENAI_API_KEY"] = revealedKey`, `ReservedKeys = ["OPENAI_API_KEY"]`.
The model id itself is applied through the existing profile `Model` /
session-model path unchanged. `-c` overrides sit above `config.toml` in Codex's
precedence and are the same mechanism `mcpconfig.CodexStrategy` already uses, so
no host file is written (satisfies AC-002.1, AC-002.3). The fixed `ProviderID`
is chosen to never equal a reserved built-in id (AC-002.2).

Adding another agent later = implement `OpenAICompatibleProvider()` with its own
`Render` (env-only for CLIs that read `OPENAI_BASE_URL`, a generated config
fragment in the isolated agent home for CLIs that require a file). The file case
uses the session's existing isolated home dir (`SessionDirTemplate`), never
`$HOME` (AC-002.4).

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
- `provider_kind != openai_compatible` clears the other two fields on write so
  stored values cannot linger active (AC-001.4).

## Control flow

Live session start (`lifecycle`):

1. Resolve profile. If `ProviderKind == openai_compatible` and the agent spec is
   non-nil, reveal `ProviderAPIKeySecretID` via the existing global-secret
   reveal.
2. `providerinject.Build(spec, profile, key)` → `Injection`.
3. Append `Injection.CLIArgs` to the agent argv after MCP `-c` args, before
   `CLIFlags`.
4. Merge `Injection.Env` into the subprocess env with `ReservedKeys` overriding
   any inherited value (see [Credential delivery](#credential-delivery)).
5. Reveal failure or empty base URL → abort start with a sanitized
   `PROVIDER_MISCONFIGURED` error; never continue to the vendor default
   (AC-002.5).

Data crossing boundaries: `Injection.CLIArgs` and `Injection.Env` travel to
agentctl through the existing `CreateInstanceRequest` argv/env fields (same
channel as MCP `-c` args today). The revealed key crosses process boundary in
the env map exactly as `OPENAI_API_KEY` does now for native Codex.

## Probe and inference reach

> **Design update (2026-08-31):** the CLI-args model below is superseded by the
> ACP gateway auth mechanism (see the Provider injection update). `providerinject`
> and `ProviderArgs` do not exist.

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
than falling back to the vendor endpoint (AC-003.1, AC-003.2). No separate
profile-scoped model probe exists today; provider profiles use free-text model
entry (AC-001.2), and the probe executor is ready for any caller that supplies
`ProviderGatewayAuth`.

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
2. `mergeEnvFillMissing` gains a `reserved []string` parameter (the
   `Injection.ReservedKeys`). Keys in `reserved` are written even when already
   present in `dst`; all other keys keep fill-missing semantics. Only the
   provider injection passes a non-empty `reserved` set (AC-004.2, AC-004.3).

## Failure and recovery

| Condition | Behavior |
| --- | --- |
| Secret store absent / reveal fails for the provider key | `PROVIDER_MISCONFIGURED`, launch aborts, no vendor fallback |
| One unrelated profile env secret fails | that entry dropped + warn log; launch proceeds |
| Base URL empty or not absolute http(s) | rejected at save; at launch (legacy row) → `PROVIDER_MISCONFIGURED` |
| Model contains `/` | rejected at save; at launch → `PROVIDER_MISCONFIGURED` |
| Provider returns 401/5xx during probe/inference | sanitized upstream status in the error, retryable |
| Agent has no `OpenAICompatibleProvider()` but row has stale fields | fields inert; `native` behavior |

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
- Reuse existing ACP frame debug logging; the injected `-c` args appear in the
  launch command log already emitted for agent subprocesses.

## Related decisions

- No ADR required: this extends the existing profile + capability pattern and
  adds no new architectural boundary. Record one only if a second agent needs a
  materially different injection contract.
