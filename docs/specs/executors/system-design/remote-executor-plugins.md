---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-002
  - REQ-EXECUTORS-PLUGIN-003
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-EXECUTORS-PLUGIN-005
  - REQ-EXECUTORS-PLUGIN-006
  - REQ-EXECUTORS-PLUGIN-007
---

# Remote executor plugins system design

## Purpose and boundaries

The executor system owns environment identity, resource inventory, admission, and recovery.
Plugin packaging and dispatch remain in `internal/plugins` and `pkg/pluginsdk`.
The boundary is recorded in the [ADR](../../../decisions/2026-09-26-remote-executor-plugin-boundary.md).
This document defines new contracts; names marked **new** do not describe existing APIs.

Core continues to own `agentctl.Client`, agent configuration, ACP, Git materialization,
task authorization, session state, and stop-reason policy.
The plugin provisions compute, installs the selected runtime, and returns connection leases.
It does not implement chat, terminal, files, Git, or another agent protocol.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-EXECUTORS-PLUGIN-001 | Registration and profiles; permanent availability |
| REQ-EXECUTORS-PLUGIN-002 | Provider RPCs; bootstrap; launch flow |
| REQ-EXECUTORS-PLUGIN-003 | Connection leases; security |
| REQ-EXECUTORS-PLUGIN-004 | Inventory; recovery and cleanup |
| REQ-EXECUTORS-PLUGIN-005 | Plugin lifecycle |
| REQ-EXECUTORS-PLUGIN-006 | Effective capabilities and expiry |
| REQ-EXECUTORS-PLUGIN-007 | User interfaces |

## Registration and profiles

Add **new** `executor_providers` manifest entries and `capabilities.executor_provider`.
Each entry contains a local key, display name, description, localized message references,
contract version, supported state versions, profile schema, resource-state schema, and capabilities.
The host derives the canonical identity `plugin:<plugin-id>:<key>`.
The manifest validator bounds keys to 64 characters and schema documents to 64 KiB.
Resource-state schemas are closed objects (`additionalProperties: false`). Before any callback or
provider response can update inventory, the host validates required fields, declared scalar types,
enums, numeric bounds, and the absence of undeclared or secret fields.
Reject collisions, unsupported required contract versions, and declarations without the capability.

Reuse the current managed binary and gRPC transport. Add optional SDK interfaces, as existing
Git credential extensions do. Old plugins remain source compatible and return `Unimplemented`.
A declared executor provider must implement every required lifecycle RPC before becoming available.
Frontend registrations cannot create executor authority.

Introduce **new** executor type and runtime `plugin_remote`, with one core backend adapter.
An `Executor` row contains immutable host-written config keys `plugin_id`, `provider_key`, and
`provider_contract_version`. Each provider owns one executor entry; operators create its profiles.
Caller-supplied executor JSON cannot override these fields or change provider ownership.
The stable executor ID survives disable and compatible upgrades.

Keep `ExecutorProfile.Config` as `map[string]string`. The initial schema accepts scalar fields,
enums, and secret references. Boolean and number values have canonical string encodings.
Nested provider objects and custom form JavaScript are excluded from this first contract.
Use existing plugin secret storage for secret fields. Persist references, never secret values.
Redacted reads return presence, not vault IDs or values; updates distinguish unchanged, replace, and clear.
References are bound to the owning plugin and profile; arbitrary global secret lookup is not allowed.

Extend existing executor/profile HTTP and WebSocket projections with a sanitized provider descriptor,
effective availability, configuration schema, and capabilities. Use one backend catalog for boot hydration,
settings, task creation, workflow selection, and agent-profile compatibility.
Do not add an executor union member for each vendor.

Audit both `executor.ExecutorTypeToBackend` and `models.ExecutorType.Runtime`, remote/container
predicates, resumability, and embedded-editor policy. Recognized `plugin_remote` routes only through
the adapter. Unknown or unavailable providers cannot reach standalone fallback or host filesystem paths.
Keep existing built-in aliases and their behavior covered by regression tests.

## Provider RPCs

Add these **new** methods to the existing `Plugin` protobuf service and optional SDK interface.
They use typed envelopes, not generic browser actions.

| Method | Inputs | Output and semantics |
| --- | --- | --- |
| `ValidateExecutorProfile` | Provider key, canonical config, profile-scoped secret values | Field errors and effective profile capabilities; no allocation |
| `ProvisionExecutorEnvironment` | Operation identity, profile snapshot, bootstrap descriptor | Exact resource handle, schema-valid bounded non-secret state, platform, retention and expiry |
| `RecoverExecutorOperation` | Original operation identity and profile snapshot | `found`, `absent`, or `unknown`; resolves lost provision responses |
| `AttachExecutorEnvironment` | Recorded handle/state, session identity, expected runtime identity | Same environment descriptor; never provisions a replacement |
| `InspectExecutorEnvironment` | Recorded handle/state | `running`, `suspended`, `terminated`, `absent`, or `unknown`, reason and expiry |
| `ResolveExecutorConnection` | Recorded handle, purpose `agentctl`, runtime port (control or instance) | Transient connection lease |
| `DestroyExecutorEnvironment` | Recorded handle or unresolved operation identity, cleanup reason and claim | Confirmed absent or retryable failure; idempotent |

Every call carries host-derived plugin installation identity, workspace/task/session/environment IDs,
execution ID, operation ID, deadline, and environment ownership generation when applicable.
The dispatch layer derives authority from the active host operation. Browser or plugin payload IDs
do not grant access. Same operation ID with a different canonical input digest returns conflict.

Error codes are `invalid_config`, `denied`, `unsupported`, `not_found`, `expired`, `conflict`,
`unavailable`, `rate_limited`, and `cleanup_pending`. Expose localized host messages and bounded
field references. Never expose arbitrary provider response bodies or wrap secret-bearing errors.
Rate-limit responses can provide a bounded retry delay.

Control calls use 30-second deadlines; connection resolution uses 15 seconds.
Provision and attach use the admitted launch deadline, capped at ten minutes in the first contract.
Cancellation propagates over gRPC, but cancellation is not proof that provider allocation stopped.
Cleanup uses a fresh bounded context after launch cancellation.

Add **new** Host callbacks `CheckpointExecutorResource` and `ReportExecutorProgress`.
Checkpoint validates the active operation lease, execution identity, plugin owner, and generation.
It persists the provider handle before subsequent provider steps, such as tagging or bootstrap, can fail.
Progress contains a closed stage code and bounded counts, not raw logs or credentials.
Callbacks cannot query or mutate another provider's inventory.

## Bootstrap and reverse connectivity

Add **new** Host `ReadExecutorRuntimeArtifact`, a streaming RPC over the broker connection.
The host's existing `AgentctlResolver` chooses the artifact for the declared platform.
An operation-bound descriptor includes artifact ID, version, size, and SHA-256 digest.
Chunks are at most 256 KiB and total bytes must match the descriptor.
The provider cannot ask the host to read a path or fetch an arbitrary URL.

The plugin transfers this artifact with its own provider mechanism. An AWS implementation can stage it
in object storage and create a short-lived signed URL. Core does not become an object-storage service.
The plugin verifies the digest before execution. The fixture transfers it directly to its test environment.

The bootstrap descriptor carries host-selected startup configuration, a one-time nonce, and runtime
port allocation constraints. It is transient and never included in checkpoints or progress messages.
The provider starts agentctl with its control server on `runtime_port` and the nonce as its bootstrap
nonce. agentctl serves each session instance on a separate port that it assigns, so the provider must
issue leases for the control port and for any instance port the host requests.
Core owns the existing handshake and encrypted agentctl token persistence. Agent credentials and
repository credentials follow existing host-to-agentctl setup after authenticated readiness.
Do not put session secrets into reusable image configuration.

The first version requires a Kandev runtime API URL reachable from the remote network.
Reuse the configured runtime API address and scoped task credentials; reject loopback-only or missing
addresses before provisioning. Structural validation is not a reachability guarantee.
Bootstrap performs a bounded authenticated callback probe; failure rolls back provisioning.
Use a fake non-loopback address and controlled transport in unit tests, not a production bypass.
No new public relay or tunnel broker is introduced.

## Connection leases

The **new** connection lease contains `base_url`, HTTP headers, WebSocket headers,
WebSocket subprotocols, `expires_at`, and a non-secret connection generation.
No function values, HTTP bodies, or Go transports cross gRPC.

Extend `agentctl.NewClient` with a separate validated endpoint constructor while preserving its
host/port signature and behavior. A host-owned lease manager provides request decoration and dialing.
Use `url.Parse` and explicit HTTP-to-WebSocket scheme conversion at every WebSocket call site.
Never mutate `websocket.DefaultDialer`; copy dialer configuration per connection.

Remote plugin endpoints require HTTPS with normal certificate verification. Reject userinfo,
fragments, credential-bearing queries, and loopback/link-local targets. Bind the validated authority
to the environment and check resolved addresses at dial time. The fixture uses a trusted test TLS server
through injected test dependencies. Do not add a shipping skip-verification setting.

Limit lease headers and subprotocols to 16 KiB combined. Reject provider changes to `Authorization`,
`X-Instance-ID`, `Host`, cookies, and hop-by-hop headers. Permit provider subprotocol additions only
through the dedicated list. Host-owned auth and execution headers are applied last.
Do not forward browser cookies or provider credentials to redirected authorities. Disable automatic
cross-origin redirects on authenticated upstream requests.

Cache leases in memory, scoped to installation, provider, environment, and connection generation.
Refresh through singleflight 30 seconds before expiry. Reject already expired or unusably short leases.
Every new HTTP request and WebSocket handshake obtains a valid lease. Do not reconnect healthy
streams merely to rotate credentials. On transport loss, existing reconnect policy obtains a new lease.
Do not replay mutating HTTP requests after an auth failure without their own idempotency contract.

Gateway `vscode_proxy.go`, `port_proxy.go`, and `port_tunnel.go` must obtain the same authenticated
upstream transport. They currently reconstruct transport from `BaseURL()` and `AuthToken()`.
Include WebSocket upgrades, preserve application subprotocols, and remove provider-only subprotocols
from browser-facing negotiation. Cache invalidation follows the connection generation, not just URL.
Agentctl remains the proxy to editor and preview ports inside the environment. No cloud token for
arbitrary application ports is sent to the browser.

## Inventory and launch flow

Reuse `executors_running` as the sole durable runtime inventory. Add one **new** typed metadata
envelope `plugin_executor`, bounded to 32 KiB. Extend the existing persistence allowlist explicitly.
It contains provider identity, installation identity, contract/state versions, operation ID and digest,
phase, resource handle, validated provider state, platform, capability snapshot, expiry, generation, and a
monotonic envelope revision used to reject stale provider observations.
State is non-secret; profile snapshots contain secret references, not their values.
The state schema cannot declare secret fields. The SDK contract forbids secrets in opaque handles.
Never project the envelope or provider state into ordinary browser DTOs.

No new inventory table or DDL migration is planned. Add narrow transactional repository methods
for provisional insert, execution-CAS checkpoint, and guarded completion. Each checkpoint compares the
caller-observed plugin-envelope revision and merges only that envelope into the latest metadata. Do not
use generic metadata replacement that can overwrite a resume token or unrelated metadata. SQLite and
PostgreSQL must exercise concurrency.

Launch phases include `allocating`, `artifact_staging`, `bootstrapping`, `provisioned`, `ready`,
`cleanup_pending`, `expired`, and `absent`.
Provider lifecycle and task-session state remain separate concepts.

1. Resolve the installed owner, profile, capabilities, cloneability, and remote API address.
2. Reject shared workspace mode. Use the canonical task-environment allocation path for an isolated
   session; never borrow another session's resource or overwrite a shared environment binding.
3. Claim the session launch and persist `allocating` with operation identity before any provider call.
4. Invoke provision. The provider checkpoints its handle immediately after allocation.
5. Transfer/bootstrap agentctl. Lease the control port, perform the handshake, and create the session
   instance. Record the instance port in inventory, lease it, and perform authenticated readiness.
   Recovery and retained reattach lease the recorded instance port.
6. Materialize credential-free repository locators through existing workspace APIs. Resolve credentials
   through existing secret/Git broker paths and preserve origin scrubbing.
7. Configure and start the agent through core. Commit ready inventory before publishing success.
8. On failure, preserve the original operation and perform authorized rollback. Mark absent only
   after confirmed deletion; otherwise retain cleanup_pending and a sanitized diagnostic.

One unresolved allocation blocks a later launch for that session. No new idempotency key is generated
to escape uncertainty. Provider implementations must support recovery by the original identity.

## Recovery and cleanup

Reuse the inventory and ownership contracts in ADR 0003, ADR 0025, and the generation-fencing ADR.
Read all owned resource records, including stopped sessions. Never use local PID checks for plugin rows.
Runtime metadata is session-scoped and cannot be copied to sibling sessions.

For `allocating`, call operation recovery. `found` checkpoints the exact handle; `absent` permits
settlement; `unknown` retains the record and blocks replacement. After a canceled launch, even a late
successful provision result enters cleanup rather than agent startup.

For `cleanup_pending` without a resource handle, recover the original operation by its recorded ID.
Destroy a recovered handle, settle `absent` only after provider confirmation, and retain `unknown` for
retry. Never treat a missing handle or unimplemented lookup as proof that allocation did not occur.

For `artifact_staging`, `bootstrapping`, and `provisioned`, attach only when the recorded resource and
persisted transient agentctl authorization are available. If bootstrap did not persist authorization,
run idempotent cleanup for the exact recorded resource. Mark confirmed absence; retain `cleanup_pending`
when the provider cannot confirm deletion. Never attach an unauthenticated partial bootstrap.

For retained resources, inspect and attach the exact handle. A profile edit cannot redirect recovery:
use the recorded non-secret configuration snapshot and current values of its same secret references.
Invalid credentials produce unavailable, not absent. Deleted profile credentials are blocked while referenced.
An unsupported state version requires a compatible plugin; the host does not reinterpret opaque state.

Ordinary agent Stop uses core agentctl behavior and detaches transient transport as appropriate.
Backend shutdown closes local connections and stops plugin processes without destroying remote compute.
Archive, delete, reset, and failed-launch rollback use host-authorized resource destruction.
Acquire the existing cleanup claim for environment owner and generation before external teardown.
Late callbacks and stale claims cannot mutate inventory or destroy a successor resource.
Task reset authorizes task-write scope before it looks up the environment or invokes provider cleanup.
Reset cannot provision its replacement until prior destruction is confirmed.

Confirmed compute absence preserves conversation resume information according to `RowMustBePreserved`.
It does not make the workspace resumable. Surface separate conversation and environment outcomes.
This contract inherits current remote event delivery; durable transcript replay remains separate work.

## Plugin lifecycle

Integrate with `Service.Disable`, `Service.Uninstall`, install/rollback, and runtime health changes.
Serialize admission and lifecycle changes using existing lifecycle/dispatch locks in their established order.
Do not hold an exclusive dispatch lock while calling an RPC that needs that same lock.

Disable closes admission, cancels and drains bounded in-flight calls, revokes operation callback leases,
and stops dispatch. Keep profiles, secret references, resource snapshots, and unresolved operations.
A late remote allocation remains recoverable by its operation ID after re-enable.
Existing remote compute can continue running; disable does not call destroy.

Uninstall first checks authoritative retained inventory under the admission barrier, before stopping
the process or deleting secrets. Reject if any resource or allocation is unresolved. Explain that the
operator must re-enable the provider and use normal task cleanup, then retry uninstall.
There is no force-forget API in this package. Out-of-band package deletion leaves visible unavailable records.

Before upgrading, check provider keys and supported state versions against retained records.
Drain dispatch, start the candidate, and require successful contract negotiation before activation.
On failure, restore the prior version through existing rollback. Fence callbacks from the old process.
Do not persist new state versions merely because the installed plugin changed.
Plugin health failure marks availability only; it never confirms resource deletion.

## Effective capabilities and expiry

The first contract supports isolated session environments, clone-required repositories, and managed agentctl.
Capabilities declare terminal, files, Git, embedded editor, preview, reattach, retention, and maximum lifetime.
Profile validation can narrow manifest support; instance inspection can narrow it further.
The host also applies platform and policy constraints. Missing optional capabilities are false.
Unknown retention remains unknown. Built-in capability decisions remain unchanged.

Bounded instances must return a parseable absolute provider-derived expiry no later than their declared
maximum lifetime. Reattachment cannot move that deadline unless the provider verifies an actual lifetime
change. Show the exact timestamp in the user's locale.
At the deadline, bounded inspection determines whether the resource expired. An unavailable inspection
shows deadline passed, state unknown. It does not trigger host deletion or a synthetic successful completion.
Confirmed expiry goes through existing environment-loss and interrupted-session settlement, with reason
`environment_expired`. Preserve task history, disable workspace resume, and require explicit reset.

## User interfaces

Reuse existing settings routes and profile save state. Provider fields render from the sanitized schema
with existing form controls. Validation errors bind to fields; unsupported schemas fail visibly.
Saving cannot silently drop unknown fields or replace a secret with its redaction marker.
Plugin display strings follow the existing plugin localization contract; host states use `t()`.

Desktop: provider cards lead to a profile page. The task selector shows availability and retention.
Session environment disclosure shows provider, current state, retention, expiry, and cleanup status.
Loading and unavailable catalogs preserve the selected ID and disable invalid actions.

Phone: use the same direct settings route with one-column fields and a safe-area Save action.
Reuse `executor-profiles-card.tsx` navigation. Use the existing `useTouchDrawer` pattern in environment
disclosure for status, and the mobile picker pattern for temporary profile selection.
The curated `mobile-menu-sheet.tsx` supplies fixed header and internal scroll geometry.
Deep forms use a page because profile editing is sustained work, not a short temporary choice.
The page or drawer has one vertical scroll owner; viewport-bound surfaces use `100dvh`.
Phone and coarse-pointer touch targets are at least 44px; fine-pointer desktop profile controls remain 28px.
Share validation and mutations across presentations. Preserve focus return and desktop preferences.

The [plan previews](../../../plans/remote-executor-plugins/plan.md#ascii-ui-preview) define UI-01 through UI-03.
Do not show a working countdown as evidence that workspace backup exists.

## Security and observability

Treat provider capability as permission to allocate compute and receive scoped bootstrap material.
Managed plugins are trusted binaries; endpoint validation is not an OS sandbox claim.
Reuse current installation/capability enforcement. Do not depend on unimplemented generic Host v2 APIs.
Host callbacks require the current dispatch generation and operation lease.
Artifact streams, secret reads, and inventory updates are scoped to that operation and provider.

Emit sanitized structured logs for allocation, checkpoint, attach, cleanup, provider absence, and expiry.
IDs may appear in logs; resource state, headers, signed URLs, tokens, and config secrets may not.
Add closed-label counters for operations (`provision`, `recover`, `attach`, `inspect`, `destroy`, `connect`)
and outcomes (`success`, `failed`, `unknown`, `fenced`). Plugin or task IDs are never metric labels.

## Permanent availability

The initial release used the restart-required `features.remoteExecutorPlugins` toggle. The host retired
its profile default, typed config field, active registry registration, service state and setter, frontend
default, and flag-only branches. Its key and environment variable are in the append-only retired runtime
flag identities. Existing SQLite overrides remain inert and are not deleted.

Provider discovery remains manifest-owned. With no installed plugin declaring `executor_providers`,
the catalog has no plugin remote entries and built-in executor profiles continue normally. An installed
but disabled, incompatible, or unavailable provider remains ineligible for new launches. Saved profiles
and retained inventory remain visible, and uninstall and upgrade guards still protect unresolved remote
resources. Dispatch requires an active installed plugin, a declared compatible provider, and a running
plugin process; these admission checks do not depend on a release toggle. Background recovery and
cleanup retain the same provider and resource identity checks.

The host provider API, packaged fixture, desktop and phone profile flows, retention disclosure, and
operator guidance remain available when an eligible plugin is installed. Public documentation describes
the provider contract and fixture, not a production cloud provider. A production cloud plugin requires
its own live validation.

## Implementation evidence

The packaged fixture verifies the provider SDK over the managed plugin gRPC process boundary. Its
conformance test provisions once, requests leases over a real HTTPS test endpoint, rotates and revokes
credentials, restarts the plugin process, recovers and attaches the same resource, resolves a fresh lease,
and destroys the resource. The test uses an explicit test-only trust root; production TLS verification
remains enabled. Fixture state checks confirm that bootstrap and profile secrets are not persisted.

Review remediation adds authenticated WebSocket coverage after the actual bootstrap handshake; retains
rotated vault credentials referenced by active inventory; reopens stopped sessions by exact-resource
attachment; and recovers each durable bootstrap phase according to persisted authorization. Inventory
checkpoints reject stale observed revisions and invalid phase transitions while merging only the plugin
envelope. A barrier test covers inspection racing with cleanup. Legacy `local_pc` routing remains mapped
to standalone while unknown executor values fail closed. The detailed regression results are in
[Task 10](../../../plans/remote-executor-plugins/task-10-review-remediations.md).

The managed Chromium E2E tests cover provider profile availability and retention disclosure with shipped
defaults. With no eligible provider installed, they verify that built-in profile selection remains
available. Separate profile and status E2Es cover desktop and phone flows. Agentctl, lifecycle, and gateway
tests cover authenticated request and WebSocket transport, recovery, and cleanup. The packaged browser
test does not start a full remote agent session or exercise every prompt, permission, terminal, file, Git,
editor, and preview action through the fixture.

The fixture and focused transport checks passed:

- `make -C apps/backend e2e-plugin-package`
- `(cd apps/backend && go test -race ./cmd/plugin-fixture ./pkg/pluginsdk ./internal/plugins/... -count=1)`
- `(cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/gateway/websocket -run 'TestPluginExecutor' -count=1)`
- `(cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/remote-executor-plugin.spec.ts)` — 2 passed.
- `node --test scripts/validate-public-docs.test.mjs` — 62 passed.
- `node scripts/validate-public-docs.mjs` — 47 public documents validated.
- `python3 scripts/list-docs.py validate` — decisions and specifications validated.
- `python3 scripts/lint-spec-files.test.py` — 36 tests passed.
- `python3 scripts/lint-spec-files.py --all` — all specifications passed.
- `git diff --check`.

No AWS account or production provider was used. Provider-specific cloud bootstrap, networking, expiry, and
deletion behavior remain outside this implementation and require a separate live validation.

## Related decisions and delivery

- [Runtime identity](../../../decisions/0003-executors-running-as-execution-id-source-of-truth.md)
- [Cleanup inventory](../../../decisions/0025-runtime-cleanup-uses-executors-running.md)
- [Ownership generations](../../../decisions/2026-09-04-generation-fenced-task-environment-ownership.md)
- [Embedded editor capabilities](../../../decisions/2026-07-30-embedded-editor-executor-capabilities.md)
- [Work package](../../../plans/remote-executor-plugins/plan.md)
