---
status: draft
system: plugins
created: 2026-10-30
requirements:
  - REQ-PLUGINS-SANDBOX-002
---

# Sandbox provider protocol major 2

## Boundary and mapping

This extends the [major 1 contract](sandbox-provider-capability.md). REQ-PLUGINS-SANDBOX-002 maps to version dispatch, typed payloads, identity, and package compatibility below. Agents owns [evidence meaning](../../agents/system-design/sandbox-evidence.md) and [operation authority](../../agents/system-design/sandbox-operation-approvals.md). The [ADR](../../../decisions/2026-10-30-sandbox-evidence-and-operation-authority.md) explains the new major and separate evidence facts.

## Version dispatch

Keep the existing six RPC method names and gRPC service namespace. Extend generated messages with typed v2 fields. A manifest declares `protocol_major: 2` for the extended capability. Host dispatch supports majors 1 and 2 through explicit validators and serializers. It must never send new fields to a major 1 provider, or accept them in a major 1 reply. Unknown majors and unknown fields remain errors.

Protocol major and policy schema are separate. A major 2 call can carry a declared schema 1 policy using its unchanged legacy grant fields, or schema 2 using the new typed policy field. Reject mixed shapes. Schema 1 filesystem approval/report/proposal replies can retain their original fields when no runtime-service binding or typed policy applies. New operation classes and service actors always require typed fields. The response still uses protocol major 2; this does not downgrade the RPC or migrate a policy.

The SDK's optional complete interface remains `SandboxProviderPlugin`. SDK module SemVer, managed manifest API 2, sandbox protocol 2, policy schema 2, and runtime version are independent. Do not rename the protobuf service or silently reinterpret its old fields.

`internal/plugins/manifest/sandbox_provider.go`, binding selection, and dispatch admit the declared major. Freeze it in attempts and report provenance. A provider that implements only major 2 cannot service a captured major 1 attempt. Existing cancellation does not require a provider call. Upgrade can make old interpretation unavailable; never reinterpret old bytes with a new parser silently. Preserve the captured normalized reports and disclose service unavailability.

## Typed additions

| RPC | Major 2 additions |
| --- | --- |
| DescribeSandboxProvider | Supported operation kinds, actor forms, policy schemas, network enforcement mode, evidence classes, audit verification formats, and explicit limitations |
| CompileSandboxPolicy | Schema-tagged static grants/envelope/resource policies; typed compiled effective permissions; declared callback and route bindings |
| NormalizeSandboxApproval | Typed operation plus child-process or runtime-service actor; request scope and operation-specific one-shot semantics |
| NormalizeSandboxStatus | Optional typed runtime observations with their source/timestamp; existing identity and confinement evidence remain distinct |
| ReadSandboxReport | Typed event variants; separate per-class coverage/integrity/finalization; bounded chunk verification and expected public trust key |
| ProposeSandboxRevision | Exact typed additions and provenance, distinguishing static grants from approval eligibility |

Keep legacy fields and field numbers. Use oneof alternatives for new operations and actors, with host validation that exactly one form applies. A TCP operation contains canonical host/port/transport. A command contains identity, argv, caller edge, work directory, and resource-policy reference. An endpoint contains origin, method, path, and query scope. Do not encode these in the old filesystem target/access strings.

All calls retain installation/operation/attempt/generation binding, deadlines, 1 MiB message limits, cancellation, and sanitized errors. No provider result can confer authority. Capability declarations are promises to check, not acceptance receipts.

## Runtime-service identity

Core supplies opaque channel and compiled-route identifiers to normalization. It authenticates the actual private callback before dispatch and compares the normalized result afterward. A provider cannot assert that a channel was authenticated by filling an ID field.

A runtime-service actor permits PID absence only for a declared service on that attempt's owned channel. The host verifies supervisor birth identity and live containment and rejects route mismatches. A child actor still requires actual child/shim process evidence. No placeholder PID or shared provider session name can satisfy that check.

Nono decoding and its PID-zero/shared-label details remain in the dedicated plugin. A differently named fixture must exercise both actor forms through the same host paths.

## Evidence and artifact verification

Extend runtime source declarations only with bounded, relative sources and their semantic classes. Core owns reads and does not execute an arbitrary provider command to obtain evidence. Major 2 status requests include the already-read root-relative source path and expected provider session identity. A provider uses them to reject an audit stream from another runtime session, without receiving an arbitrary filesystem-read capability. Existing source isolation, no-symlink, enumeration, and output limits remain mandatory.

For final reports exceeding one message, `ReadSandboxReport` accepts a versioned chunk envelope: artifact-set digest, core-generated operation handle, source ID, chunk sequence, final marker, and bytes. Core owns offsets and the whole-file digest. The provider keeps a bounded verifier only for this operation and returns continuation state with no file access. Expiry or crash discards it. Restart rereads immutable input; persisted provider cursors never grant arbitrary host reads.

A final verification result includes verified input digests, verifier/schema version, final event root, session binding, coverage, ledger result when supplied, signature result, and expected-key fingerprint. Core compares these with the captured input before persistence. Unsupported algorithms and missing required fields fail verification. Missing optional signatures are unsigned. Never claim trusted signing from the artifact's own public key alone.

Private signing keys, callback credentials, and provider login material are excluded from RPC data. Public projections use host allowlists. Signature verification does not assert current confinement.

## Dedicated Nono consumer

Migrate imports from `github.com/kandev/kandev/pkg/pluginsdk` to `github.com/kdlbs/kandev/pluginsdk`. Remove the dependency on the backend module. Local development may replace only the standalone module with `../kdlbs-kandev/pluginsdk`. CI must pin an exact compatible SDK version/commit, build independently, and validate a packaged artifact.

The host `plugin-pack` executable remains a separate build tool dependency if needed. Do not confuse its source checkout with the SDK module dependency. Update Makefile naming, `.kandev-sdk-ref`, release workflows, and README consistently. Publishing a production plugin requires the compatible SDK to be publicly resolvable; a local replace alone is not a release contract.

Runtime 0.81.0 is an explicit evaluation candidate. No floating latest download is permitted at launch. Pin source, archive, binary, package, and license records. Keep runtime-specific network/profile/approval/audit handling in the plugin. Runtime feature support remains limited to passed Linux amd64 Local/Worktree checks.

## Compatibility tests

Test v1 host/provider exchanges, new host with old provider, v2 host/provider, unsupported-major refusal, unknown-field refusal, nil replies, cancelled dispatch, plugin loss, and downgrade attempts. A v2-only plugin must fail safely on an old host. Repeat generic wrapper behavior with all provider plugins absent.

Test both real Nono and a second deterministic provider. Fake evidence proves validator behavior only. Real runtime checks prove the provider's declared semantics. All delivery gates appear in the [expansion plan](../../../plans/nono-sandbox-expansion/plan.md).
