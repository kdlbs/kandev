---
status: draft
system: plugins
created: 2026-10-22
requirements:
  - REQ-PLUGINS-SANDBOX-001
---

# Sandbox provider capability system design

## Boundary

This optional contract extends `pluginsdk/`, `pluginsdk/proto/kandev/plugin/v1/plugin.proto`, manifest validation, and plugin dispatch.
It does not reuse `ExecutorProviderPlugin`, which provisions remote environments.
[Agents](../../agents/system-design/sandbox-policies.md) owns authorization, policy, process ownership, and user outcomes.
REQ-PLUGINS-SANDBOX-001 maps to negotiation, calls, lifecycle, and repository boundaries below.

## Negotiation

Propose manifest contribution `sandbox_providers` with stable provider ID and protocol major 1.
The optional SDK interface is `SandboxProviderPlugin`.
The host grants a new explicit `sandbox_provider` capability after operator installation review.
Negotiate methods and transport support before use. Unknown majors fail as unsupported.
Unknown required fields, duplicated provider IDs, and conflicting operation descriptors fail validation.
Existing plugins need no new methods. Unimplemented RPCs return typed unsupported results.

## Proposed typed calls

Every request contains protocol major, installation ID, operation ID, attempt identity, deadline, and a bounded immutable policy snapshot.
Core supplies host and transport facts. Agent arguments cannot supply this context.
All replies contain operation ID, schema version, and either a typed result or a sanitized error code.

| Call | Input | Result and limit |
| --- | --- | --- |
| `DescribeSandboxProvider` | Host OS/architecture and execution transport | Provider/runtime versions, display name, supported features, status-evidence schema and denial-report classes. 5 seconds. |
| `CompileSandboxPolicy` | Canonical grants, pinned source graph, ceiling hash, runtime references, logical endpoint descriptors | Prefix template, bounded profile artifacts, canonical effective grants, protected roots, required capabilities. 10 seconds. |
| `NormalizeSandboxApproval` | Verified provider event and bound attempt | Exact typed operation and supported decisions. No authority or grant. 2 seconds. |
| `NormalizeSandboxStatus` | Host-read private runtime evidence, bound attempt, expected policy digest, and process birth identity | Mechanism, observed process/policy binding, evidence schema/version and limitations. No authoritative active boolean. 5 seconds. |
| `ReadSandboxReport` | Core-authorized audit bytes and cursor | At most 1000 records and 256 KiB, continuation cursor, completeness and integrity state. 5 seconds. |
| `ProposeSandboxRevision` | Exact source revision and operator-selected report entries | Typed access diff, provenance, compiled candidate. No persistence. 10 seconds. |

Total request size is 1 MiB. Artifact output is at most 1 MiB and 32 files.
Names are relative, normalized, and free of traversal or symlinks.
Only declared UTF-8 profile/config artifacts are accepted. Providers cannot return arbitrary executable files through policy compilation.
Binary helpers must ship in the reviewed plugin package and match its digest.
Runtime binaries remain explicit verified installation dependencies. The plugin does not download an unpinned runtime during launch.
The description identifies a packaged runtime artifact by relative path and SHA-256. Core resolves it within the selected installation, checks the package checksum entry and bytes, and copies the verified executable into the owned attempt directory. The installation checksum manifest identifies package content in the attempt record. Plugin removal cannot remove that running attempt copy. No provider-specific executable name or host installation path is compiled into core.

The host expands reserved runtime references and materializes artifacts privately.
It checks provider effective grants against the policy snapshot and executor ceiling.
It admits the original child separately, then composes the launch through core.
The plugin receives no operator token, provider login secret, or complete inherited environment.
Artifact bindings identify an existing empty JSON string by an object-key path and a runtime reference. Core validates the destination, resolves the reference, JSON-escapes the value, and enforces the output bound. Private callback references are valid only in these artifact bindings. They cannot enter provider requests, prefix arguments, or public projections.

The provider description supplies bounded static JSON bodies for allow-once, deny, and timeout replies. Core selects a body only after its own decision checks. Normalized requests include the provider session ID and child PID for comparison with the owned attempt. Core does not embed provider-specific request parsing or decision JSON.

No `Start`, `Stop`, arbitrary command, or policy-save RPC belongs to the provider interface.
Core owns those operations. Normalized approvals remain data until the host verifies and authorizes a decision.

## Presentation and runtime evidence

The host owns localized group labels, common states, icons, and selection semantics.
Provider display names are bounded plain text. The contract cannot return HTML, UI components, or an active-shield instruction.
Use canonical grants for readable/writable paths, explicit network destinations, and logical Kandev MCP access.
Capability fields distinguish supported, unsupported, and unavailable operations. Missing fields cannot imply support.
Provider compilation declares prohibited overrides and protected artifacts as typed constraints for generic core enforcement.
Keep CLI syntax, profile parsing, evidence decoding, and runtime-version differences in the provider implementation.

Status normalization accepts host-read evidence only from declared protected artifacts or a bound runtime channel.
Artifact descriptors must use the same bounded, validated references as compilation output.
The provider cannot direct the host to read arbitrary paths or execute a new command to obtain status.
Core checks process ownership, attempt generation, policy digest, freshness, and the adapter's tested compatibility before projecting active status.
Missing or unverifiable evidence yields unknown. Availability and successful compilation cannot substitute for runtime evidence.
The [agent design](../../agents/system-design/sandbox-policies.md#projection-and-evidence) owns projection and indicator rules.

Exercise a differently named fixture provider with the same typed data and no Nono runtime.
This proves generic dispatch and rendering only. It does not establish real confinement acceptance.

## Lifecycle and placement

V1 supports Local and Worktree on the backend execution host only.
Use the existing supervised plugin process and Host gRPC connection.
Do not start a plugin-owned HTTP server. Core owns callback listeners and MCP relays.
Do not invoke a backend-local provider for a remote agentctl.

Pin installation digest and runtime version in each attempt.
Disable and uninstall block new calls. Pending decisions fail closed.
Existing processes retain frozen policy and core termination handles.
Core retains active artifacts until process cleanup completes, even if plugin data removal is requested.
Reports can show provider unavailable without deleting the attempt's evidence.
An upgrade requires a new successful probe for new attempts and does not reinterpret old reports silently.

## Dedicated Nono implementation

Proposed repository: `kdlbs/kandev-plugin-nono`, created or attached during later implementation.
Its existence is not assumed, and this design turn creates no external repository.
Use the official plugin template, package validation, and disposable installation smoke tests.
The monorepo keeps only a deterministic capability fixture under `cmd/plugin-fixture`.
The real plugin must use public SDK contracts, never Kandev internal packages or direct database access.

Nono 0.80.0 is the initial pinned evidence baseline.
The plugin compiles explicit network restrictions because Nono's default network mode is not deny-all.
File approval uses the verified webhook exchange. Other approval kinds remain unsupported until actual runtime tests pass.
The provider reports unsupported host/kernel combinations before launch.

## Verification

Test old host/new plugin, new host/old plugin, absent capability, malformed artifacts, deadline expiry, crash, disable, upgrade, and uninstall during execution.
Pair each refusal with an unwrapped generic-prefix control that still succeeds with plugins disabled.
Run the actual packaged Nono plugin against a disposable Kandev instance before claiming support.

## Related decision

- [Optional sandbox provider boundary](../../../decisions/2026-10-22-optional-sandbox-provider-boundary.md).

## Private runtime evidence sources

A provider can declare bounded identity, status, and audit sources below an attempt's private state root. Each source specifies a relative directory, filename suffix, maximum depth, file count, and byte limit. A filename suffix is a literal basename ending, not a glob. It can select a full filename such as `audit-events.ndjson` without matching a sibling ledger. Core rejects path escapes, symbolic links, oversized files, and excessive enumeration. The provider receives bounded file bytes through its existing normalization RPCs, never a host filesystem read capability.

Registry identity and confinement evidence are separate reply alternatives. A registry identity contains supervisor PID, child PID, and provider session ID. Core requires the supervisor to match its captured process, then verifies live birth identities and ancestry. Registry identity cannot set active confinement. The Nono registry has no policy digest, so its normalization returns identity only. Status sources must name a declared evidence schema. Core accepts evidence only for the current policy, attempt, generation, and supervisor birth identity, with a positive timestamp less than 30 seconds old. Verification has a five-second total deadline. A release must include adapter conformance checks for each supported host, mechanism, and transport.

## Exact TCP egress capability

The optional `egress_proxy` capability supplies a core-owned CONNECT relay for one immutable attempt. The relay validates an exact canonical host/port against that attempt's captured TCP grants before dialing. It accepts no arbitrary HTTP routes, enforces request and connection limits, and closes active tunnels on attempt cancellation. The capability is unnecessary for providers that independently enforce the same contract.

A provider can bind the reserved `sandbox.egress.endpoint` reference only into an existing empty JSON string in a private artifact. Core supplies the runtime address after compilation. The provider does not receive it in compilation requests or place it in public projections. Basic command wrapping does not use this capability.

Pinned Nono 0.80.0 requires this capability whenever network grants exist. Its plain `allow_domain` input removes port suffixes. The provider uses its verified `network.upstream_proxy` setting to delegate exact destination enforcement to core. Its child remains restricted to Nono's local proxy. The callback listener uses a distinct port outside all granted network ports, and both direct and proxied callback access must fail in real-runtime tests.
