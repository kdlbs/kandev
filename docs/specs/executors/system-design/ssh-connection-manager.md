---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-003
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
---

# SSH connection manager system design

## Purpose and boundaries

Implement the [requirements](../requirements/ssh-connection-manager.md) under
[the custody decision](../../../decisions/2026-09-19-managed-ssh-credential-custody.md).
Executors owns connections and identity metadata. The existing secrets package
owns encryption, authz owns authorization, and task/runtime services retain
control of task admission, lifetime, and recovery.

Implementation status is tracked in the linked plans. Preserve the
[legacy SSH contract](ssh-executor.md) until a profile is explicitly converted.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-EXECUTORS-SSH-CONNECTIONS-001` | Identity custody; Authorization; Persistence |
| `REQ-EXECUTORS-SSH-CONNECTIONS-002` | Connection model; Trust and readiness; Runtime binding |
| `REQ-EXECUTORS-SSH-CONNECTIONS-003` | Enrollment protocol; Failure and recovery |
| `REQ-EXECUTORS-SSH-CONNECTIONS-004` | API contracts; Settings and mobile composition; Diagnostics |
| `REQ-EXECUTORS-SSH-CONNECTIONS-005` | Compatibility; Rollout; Audit and persistence |

## Original integration seams

These seams describe the pre-manager code that the extraction replaced.
The dependency direction below remains the intended architecture.

- `internal/agent/runtime/lifecycle/executor_ssh_connection.go` owned
  `SSHConnConfig`, `SSHTarget`, `ResolveSSHTarget`, `DialSSH`, agent/file auth,
  config expansion, and ProxyJump. Its connection/dial tests cover those rules.
- `internal/ssh/handlers.go` owns `/api/v1/ssh/test`, `ssh.test`, sessions,
  `resolveSSHTarget`, and readiness probes. It imported lifecycle before extraction.
- `SSHExecutor` launch/resume/status/cleanup consumes SSH credentials through
  config/metadata. `SSHPreparer` currently reports configuration validation
  without dialing. Keep preparation metadata validation consistent with the
  new resolver; do not add a second authentication path or redundant dial.
- `internal/secrets` supplies `SecretStore`, `MasterKeyProvider`,
  `UserVisibleStore`, and `IsInternalID`. Global user secrets are per-user under
  auth; that must not be mistaken for installation-owned credential storage.
- `internal/authz` supplies `RequireOrgScope`, `ScopeOrgConfigManage`, and the
  role resolver. Do not introduce parallel admin booleans or permission maps.
- `internal/backendapp/{storage,services,agents,helpers}.go` composes stores,
  runtime dependencies, and routes. The new packages are wired there.
- Web seams: `src/settings-routes.tsx`, `lib/settings-discovery/navigation.ts`,
  `components/settings/ssh-connection-card.tsx`, `lib/types/http-ssh.ts`,
  `lib/api/domains/ssh-api.ts`, and the settings state/domain hooks.

## Components and dependency direction

Introduce `internal/sshtransport` below lifecycle and `internal/sshconnections`
for the persistent connection service. Neither imports lifecycle or an executor.
The connection service depends on transport, its own store, and a narrow
internal-secret adapter. Lifecycle and existing SSH handlers receive that
service through backend composition; consumers do not retrieve raw key bytes.

Move the existing low-level target resolution, auth, and dial implementation
into sshtransport with behavior-preserving adapters at existing lifecycle
exports. Move internal transport tests with the implementation and retain
consumer regressions. Do not copy a second SSH stack. Agentctl upload, shell
probes, repository preparation, session lifetime, and credential-copy logic
stay in lifecycle. Reuse the existing single-hop tunnel machinery for managed
routes, with explicit independent bastion and destination authentication and
pinned host policies. Preserve the legacy known_hosts/unknown-host behavior
only for unconverted legacy profiles; managed routes never inherit its
unknown-bastion acceptance. No agent forwarding or key upload to the bastion.

Transport's proposed handshake contract takes an explicit resolved target,
host-key policy, and an auth source producing signers/auth methods plus a
cleanup callback. It supports agent/file auth and bounded in-memory managed
signers. An observation operation performs key exchange without credentials to the
hop being observed and captures its fingerprint, then closes without opening
a session on that hop. Destination observation through a bastion first
authenticates the separately trusted bastion and opens its TCP forwarding
channel; destination auth remains empty. Observed
keys are evidence, never implicitly trusted.

Connection service exposes distinct authorized operations: manage identity,
manage connection, observe/trust, preflight, authenticate, and enroll. Runtime
consumers use `AdmitRuntime` to validate the saved executor/session binding
and create or recover a protected runtime reference. `DialRuntime` revalidates
that reference, route stamp and credential generations before it dials. These
internal capabilities do not accept free-form caller routing overrides.
Handshake cleanup closes agent sockets and releases private buffers; it does not forward an agent, export a
key, or put secrets in subprocess environments.

## Identity custody

Proposed `ssh_identities`: UUID, label, kind (`managed`, `file`, `agent`),
public key/fingerprint where knowable, internal secret reference or file/socket
reference, immutable credential generation, row version, created/updated timestamps.
Managed generation is Ed25519 using crypto/rand. Imports use the SSH parser
already in the dependency graph; accept supported Ed25519/RSA/ECDSA encodings,
reject malformed/multi-key or oversized input (64 KiB), and reject certificate
or hardware-backed formats that cannot be represented as a local signer.
No new cryptographic implementation is introduced.

Import decodes with a transient passphrase when needed and stores canonical
private material encrypted under the existing master key. Request fields are
write-only, HTTP bodies are bounded, and request/response diagnostic logging
must redact the entire credential-bearing body. Frontend keeps input only in
component memory, never Zustand, local storage, URL, task plan, or analytics.
Server drops references and clears owned byte buffers after parsing/signing;
Go/string memory cannot promise comprehensive zeroization, so do not claim it.

Use a new reserved internal secret prefix `ssh-identity:` through a narrow
adapter. Extend `IsInternalID` and its existing list/reveal/copy/move/provider
exclusion tests. Identity requests never accept a secret ID; the service derives
it from the identity UUID. User APIs cannot read, overwrite, delete, bind, copy,
or move these backing secrets even when an ID is guessed.

The adapter performs purpose-restricted installation-secret operations after
service authorization, independent of the HTTP actor's per-user secret scope.
Do not clear the identity from a general request context or fabricate synthetic
admin authority. Add an explicit internal-secret transaction capability to
secrets, constrained to the reserved prefix, used only by backend composition.
This also avoids the auth setup claim of unowned secrets changing runtime
access. Test both auth setup and removal of the identity's creating admin.

File and agent identities are references, not copies. External encrypted files
still require an external agent unless explicitly imported as managed. Agent
preflight resolves the socket in the connection context and asks for signers;
zero signers is distinct from a missing or unreachable socket. Managed
identities have one fixed public key; agent identities may offer several and
cannot supply an enrollment destination key. Enrollment requires a managed or
parseable file identity with one public key. A bootstrap identity can be any
supported kind but cannot equal an unavailable destination credential.

## Connection model

Proposed `ssh_connections`: UUID, name, resolved host/port/user, optional source
alias for display, identity ID, host public key and SHA256 fingerprint, optional
`bastion_connection_id`, transport revision, row version,
trust generation, last authentication result/revision/time, timestamps.
The row version changes on every metadata write and controls conditional
updates. Transport revision changes only when endpoint, user, identity, trust or
bastion selection changes; it binds probes and runtimes. Labels cannot invalidate
runtime identity. Auth statuses
are derived as `draft`, `untrusted`, `unchecked`, `ready`, or `unavailable`.
Last success is historical evidence, not a liveness guarantee.

For new connections, aliases are an input convenience. Resolve and display
HostName, Port, User, IdentityAgent/IdentityFile and one ProxyJump before save.
A jump alias resolves its own user/identity settings. Where legacy behavior
would fall back to the destination identity, show that identity explicitly and
require confirmation for the bastion rather than silently sharing it. Persist the concrete
endpoint and selected identity; do not silently reinterpret an edited SSH config
as a different managed target later. Host/port/user, identity selection, or pin
changes increment revision and clear readiness. Labels do not change transport
identity; their row-version conditional updates still prevent lost edits.

A connection owns one target pin and may reference one direct connection as
its bastion. The referenced record owns the bastion's user, identity and pin,
and may be reused by several destination connections. It cannot itself have a
bastion. Selection lists ready direct connections, with a Create bastion path
through the existing direct setup flow. A draft may become unavailable if its
selected bastion loses readiness; no silent direct fallback is permitted.
Reject self-reference, nested/chained hops and cycles before network work,
including changing a connection already used as a bastion into a jumped route.

Readiness and trust receipts bind a route stamp: destination ID/revision plus
bastion ID/revision when present. Read and compare both records in the same
transaction; a bastion transport change invalidates dependent readiness even
if destination rows have not been rewritten. Avoid cached success determined
from only the destination revision. Locks/revision checks cover both IDs in
stable order. Deletion references include dependent connections, and runtime
references protect both identities and connections. There is no automatic pin
reset, first-use acceptance, host-name deduplication, or profile-to-connection merge. Referenced identities are immutable in credential
content; replace by creating/enrolling a new identity and changing selection.

## Authorization and API contracts

All new management endpoints require `org.config.manage` using the existing
resolver at both HTTP and service boundaries. Missing authenticated identity
fails closed; auth-disabled synthetic operator remains valid. Existing task
access controls govern runtime use, not credential management authority.
No new MCP/agent-tool mutation surface or public signer endpoint is added.

Managed HTTP endpoints under `/api/v1/ssh/managed`:

Current main owns `/api/v1/ssh/identities` for external identity discovery. The
managed namespace preserves that API and prevents route-registration conflicts.

| Endpoint | Request / result |
| --- | --- |
| `GET /identities`, `GET /identities/:id` | Metadata and public keys only |
| `POST /identities` | label plus generate/import/file/agent input; metadata |
| `PATCH /identities/:id` | conditional label edit only |
| `DELETE /identities/:id` | conditional, refuse referenced identity |
| `GET /connections`, `GET /connections/:id` | Admin connection metadata |
| `POST /connections`, `PATCH /connections/:id` | target/identity draft, expected row version |
| `DELETE /connections/:id` | conditional, refuse profile/runtime references |
| `POST /connections/:id/observe` | expected route stamp; destination host key and receipt; direct bastion observation uses its own ID |
| `POST /connections/:id/trust` | expected route stamp, receipt, verified fingerprint for this connection |
| `POST /connections/:id/preflight` | expected route stamp; non-mutating local checks for both hop identities |
| `POST /connections/:id/test` | expected route stamp; pinned authentication result for the whole route |
| `POST /connections/:id/enroll` | expected route stamp, request UUID, destination key fingerprint, bootstrap mode |
| `GET /connections/:id/enrollments/:request_id` | Admin sanitized operation result |
| `POST /executors/:id/adopt-connection` | expected executor config digest and ready connection ID/route stamp |

Create operations also carry a caller-generated idempotency UUID scoped to
actor and operation; repeated generation cannot accidentally issue a different
key. Reusing a UUID with a different non-secret input digest returns conflict.
Never persist credential inputs to implement idempotency.

Enrollment results retain a bounded `error_code` and an optional `hop`
(`bastion` or `destination`). Preserve an existing probe diagnostic when fresh
authentication fails. Do not replace it with a generic transport error.
Older persisted results without `hop` remain readable.

Trust receipt is a server-side random opaque token, five-minute expiry, scoped
to actor, connection route stamp, resolved host/port/user, and observed host key.
The stamp includes the bastion record/revision used to reach the destination.
Consume it once when trust is committed. Restart discards receipts. Changed
key replacement is a distinct confirmation showing old and new fingerprints,
not a test checkbox reset. Observation does not execute remote commands.

Success responses carry row version, transport revision, and the complete
route stamp. Hop-specific failures include `hop: bastion | destination`. Metadata writes
compare expected row version; probes/enrollment compare the complete route
stamp. Mutation responses include resolved bastion metadata for administrators.
Stale edits/tests yield 409.
Stable error codes carry safe typed fields, not raw SSH errors: 400 invalid,
401/403 access, 404 unavailable feature/entity, 409 stale/reference conflict,
422 prerequisite/authentication failure, 503 network/protection unavailable.
The frontend translates codes into localized guidance. Never claim that
`authentication_rejected` proves a missing authorized_keys entry.

Runtime endpoints accepting executor IDs resolve their stored managed binding;
requests mixing connection ID with host/user/file/pin overrides are rejected.
Legacy HTTP/WS `ssh.test` and readiness handlers must reject managed identity or
connection requests from callers lacking management authority, rather than
bypassing the new service through their existing free-form fields. Bound task
operations never expose connection enumeration or a generic remote probe.

## Trust and readiness

1. Save a draft target and identity reference, without dialing.
2. For a jumped route, select or create a ready direct bastion connection.
   Its own host observation/trust/authentication completes before it is used.
   Observe the destination host key with no destination authentication, over
   the already trusted/authenticated bastion if selected. Independently
   verify/trust the destination fingerprint. Observation failure remains a draft.
3. Local preflight checks identity/storage/agent readiness and selected trust.
4. Test uses only the pinned key and selected identities for both hops. No password fallback or direct-route fallback.
5. Commit readiness only if the entire route stamp and both selected identity
   generations still match; classify bastion auth/tunnel and destination failures.
6. Profile binding requires connection readiness plus its existing executor
   readiness checks; bind atomically against the tested route stamp.

Each remote operation rechecks each hop's pin during its real handshake,
including enrollment. A failed bastion pin/auth/tunnel prevents destination
authentication; a failed destination pin prevents destination credentials from
being sent even though trusted bastion authentication has already occurred. A spoofed observation cannot cause password release without the
explicit trust step. A changed host fails before an auth callback supplies a
password or signer. Backend restart invalidates in-memory receipts and pending
probes, not stored trusted keys or managed credentials.

## Enrollment protocol

Enrollment targets the selected login user's standard OpenSSH home layout on
supported Linux/macOS hosts, directly or over the approved single-hop route.
The bastion must already authenticate with its selected identity. To bootstrap
a new bastion, enroll its own direct connection first; never cascade remote
authorization mutations across both hops in one request. Password and existing-identity bootstrap are
separate inputs. No keyboard-interactive prompts, sudo, arbitrary command input,
custom AuthorizedKeysFile discovery, or cloud console access is implied.
Unsupported layouts use public-key copy and normal authentication checks.

Before networking, validate feature, administrator authority, expected revision,
route stamp and both trusted fingerprints, destination public key, and
bootstrap choice. Persist a
sanitized operation record with request UUID and `started`; fail closed if that
write fails. Serialize enrollment for a connection; duplicate UUID returns its
result and never repeats a password attempt. Limit password attempts to one
handshake per explicit request; no background retries. Bound each operation to
60 seconds, and only one active enrollment per connection.

Open the trusted bastion using its configured identity if present; then
authenticate to the destination with the bootstrap credential under the stored
destination pin. The destination handshake runs end-to-end over the forwarding
channel. Never use the destination bootstrap password to log into the bastion,
forward an agent, or install the destination key on the bastion.
Run a fixed, reviewed POSIX shell procedure, with the validated public key sent
through stdin, not command interpolation. The procedure uses the remote login
home, no caller-supplied path. Create missing .ssh and authorized_keys with
0700/0600; refuse symlink/non-regular targets, unsafe ownership, and writable
by-other directories/files. Existing safe permissions and contents are preserved.
Take an atomic per-user .ssh enrollment lock directory; refuse a busy/stale lock
with recovery guidance, never break it automatically. All error/success paths
attempt lock release. No password or private material enters that procedure.

Read existing entries as SSH authorized-key records and compare key blobs,
including entries with options/comments. If the same key exists, preserve its
options rather than appending an unrestricted copy. Otherwise append one
validated bounded public-key line, ensuring a preceding newline, using append
mode without truncation or replacement. Never rewrite the complete file or
chmod an existing unsafe file into apparent compliance. Serialize Kandev writers;
non-cooperating external editors and a malicious owner of the remote account
are not a filesystem transaction boundary Kandev can guarantee. The remote
account already controls its authorization; report conflicting/read failures
rather than attempting to repair its files.

Close bootstrap session, discard password, and independently dial with the
destination identity and pinned host key, through a fresh verified bastion
connection when applicable. Success records `verified` and marks
the unchanged route stamp ready. Failed fresh auth after a write records
`installed_unverified`; transport loss where the write outcome is unknown records
`outcome_unknown`. No automatic rollback: another actor may already rely on
that public key. Retry starts with a new ordinary destination-auth test; if that
fails, explicit re-enrollment can supply new bootstrap input and safely check
for the existing entry. On restart, `started` becomes `outcome_unknown`, never
replayed. User cancellation may stop transport but cannot promise remote rollback.

## Runtime binding and compatibility

Add `ssh_connection_id` as the exclusive connection selector in executor Config.
Existing profiles retain their inline fields. A managed binding cannot also
contain inline auth/target/pin values. Profile-specific shell, preparation,
agent selection, and workdir settings remain on their current objects.

All launch, preparation, test/readiness, status, resume, and cleanup paths use
one resolver. Recorded runtimes carry the complete route stamp, both identity IDs and
generations when jumped, and the recorded endpoint/pin metadata for each hop,
never key bytes.
Validate those records against current connection generation before reconnecting;
a mismatch produces `connection_changed` before dial, upload, or process signal.
Existing live transports may finish on their recorded target. Transport edits
to either destination or bastion are refused while live or resumable runtime
references depend on that record, including through another connection; complete their
existing stop/reset/archive lifecycle before changing the connection. Labels
remain editable. A revision mismatch from stale/corrupt records still fails
closed. Display dependent profiles and sessions, and offer their existing
management routes; no silent session teardown or reattachment occurs.

New admission and connection mutation share a serialization boundary covering
all route IDs, locked in stable ID order. A dial can be slow outside the lock, but revision is rechecked before
issuing remote workload commands or recording admission; discard a stale dial.
Identity deletion checks connection and retained-runtime references, including
older generations, so a changed selection cannot orphan a still-recorded key.
Deletion never cascades across profiles or remote machines.

Explicit adoption previews the legacy resolved endpoint, identity source, pin,
and affected profile. Create a separate connection per selected legacy profile;
keep legacy pin exactly, require trust when absent, and retest. Final adoption
atomically checks the original config digest and ready connection revision,
then replaces only inline connection fields. Failure leaves the profile intact;
new unused connection records can remain visible for retry or deletion.
For single-hop adoption, resolve the bastion separately using the existing
alias/literal rules, preview both targets and identities, then explicitly
observe/confirm the bastion pin and test the entire route. An existing
known_hosts match can be shown as evidence but is not silent managed approval;
legacy unknown-host acceptance is never copied. Create a distinct direct
bastion record unless the administrator explicitly selects an existing record
whose resolved endpoint, user and identity match. Bind only after both route
revisions and the unchanged original executor config digest are verified.
Chained/unsupported settings leave the original profile untouched.
Unconverted resumable runtimes keep their legacy metadata path.

Remote Docker now uses the shared transport. Preserve its external file/agent
behavior; managed Remote Docker binding is outside this package. External key
discovery never implies import. Reachability remains separate from authentication.

## Persistence, audit, and failure recovery

Use the existing backend writer/reader DB pools and dialect helpers for new
identity, connection, and enrollment/audit tables; support SQLite and PostgreSQL.
Metadata creation and backing internal-secret creation/deletion use the same
transaction through the purpose-specific secret capability. Failures roll back
both; do not rely on best-effort orphan cleanup for normal operation. Foreign
reference checks and revision compare-and-swap must share the mutation
transaction. No credential payload is placed in executor Config or JSON metadata.

The secrets package already protects values with AES-GCM and a local master.key.
Backups need both DB and matching key. The current MasterKeyProvider may create
a new key when its file is missing/invalid; the connection service must detect
unreadable existing identities and refuse automatic regeneration or overwrite
of identity rows. It cannot recover old ciphertext with a new master key.
A master-key loader hardening project is not a prerequisite of this feature;
no stronger recovery guarantee than the existing backup boundary is claimed.

Use a dedicated `ssh_connection_audit` table for administrator actions and
managed authentication failures: actor ID (or synthetic operator), operation,
entity IDs/revisions, timestamps, correlation/request UUID, outcome/error code,
and relevant public fingerprint changes. The enrollment row is durable before
remote mutation; finalize outcome after it. If final persistence fails after
remote work, leave the earlier started record to reconcile as outcome_unknown.
For local mutations write audit in the metadata transaction. Unavailable audit
storage rejects enrollment before sending credentials. No request bodies,
passwords, key bytes, raw remote output, or free-form socket errors are stored.
Records remain until installation data is explicitly removed/restored; initial
version adds no automatic audit purge. Document backup/storage impact.

Logs use the same sanitized fields, never value-bearing DTOs. Metrics, if added,
use only closed operation/outcome enums, not host/user/identity labels. No new
metrics dashboard is necessary. Operational errors distinguish missing
identity, credential_unavailable, agent_missing/unreachable/empty,
host_untrusted/changed, network_unreachable, authentication_rejected,
unsafe_authorized_keys, enrollment_busy, installed_unverified, and
connection_changed. Success time never substitutes for a fresh failed probe.

## Settings and mobile composition

The [SSH management interaction design](ssh-connection-management-ui.md) owns
navigation, setup continuity, collection/detail composition, and desktop/phone
presentation for this same executor-owned capability. It includes the proposed
2026-10-26 UX revision and preserves the existing security and runtime contracts.
The [navigation package](../../../plans/ssh-connections-executor-navigation/plan.md)
is complete; the [UX package](../../../plans/ssh-connections-ux/plan.md) is pending.

## Rollout

Proposed identity: `features.sshConnectionManager`, environment
`KANDEV_FEATURES_SSH_CONNECTION_MANAGER`, config field `SSHConnectionManager`,
JSON key `sshConnectionManager`. Keep the typed registry/config and frontend defaults;
prod/dev/e2e defaults are false. Metadata declares restart required because
startup wires services and recovery. Tests explicitly enable in disposable
instances; do not enable the whole E2E profile.

Gate new HTTP/service entry points, boot/settings metadata, executor binding
validation, runtime resolution (including background reconnect/probes), and
legacy HTTP/WS managed-selector adapters. When off, legacy behavior remains,
managed operations fail before reading secrets or dialing, and metadata is
retained. In-flight established transports are not retroactively revoked;
normal stop can close an already-held transport without resolving credentials.
After restart with the flag off, managed runtimes require re-enablement for
reconnect or remote cleanup; do not fall back to legacy fields. Backend shutdown
may close local resources. No disabled startup enrollment replay or probes.

Schema initialization is additive and safe with the gate off; migration must not
create identities or adopt profiles. Promotion and eventual retirement follow
the runtime-feature-flags skill. This package defines a default-off release,
not a release date or automatic promotion.

## Validation and delivery

The [plan and work orders](../../../plans/ssh-connection-manager/plan.md) map
all ACs to proposed tests and commands. Include SQLite/PostgreSQL transactions,
HTTP/service/WS authorization, auth-setup continuity, restart recovery without
SSH_AUTH_SOCK, host-key change at either hop before its password/signing, distinct identities,
bastion replacement after destination observation, chain/cycle rejection, duplicate/stale
requests, locked/unsafe authorized_keys, redaction, profile adoption, retained
runtime revision mismatches, and flag-disabled side-effect prevention.

Browser tests use the real backend. Identity/settings/mobile tests need no
Docker; real enrollment uses the existing isolated sshd fixture under the
containers project. Mobile network-free tests stay outside tests/ssh to avoid
accidentally inheriting the containers matcher. Public docs change alongside
implementation, not this draft design.
