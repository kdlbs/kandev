---
status: draft
system: plugins
created: 2026-10-22
requirements:
  - REQ-PLUGINS-SANDBOX-001
---

# Sandbox provider capability system design

## Boundary

This optional contract extends `pkg/pluginsdk`, `proto/kandev/plugin/v1/plugin.proto`, manifest validation, and plugin dispatch.
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
| `DescribeSandboxProvider` | Host OS/architecture and execution transport | Provider/runtime versions, supported features and denial-report classes. 5 seconds. |
| `CompileSandboxPolicy` | Canonical grants, pinned source graph, ceiling hash, runtime references, logical endpoint descriptors | Prefix template, bounded profile artifacts, canonical effective grants, protected roots, required capabilities. 10 seconds. |
| `NormalizeSandboxApproval` | Verified provider event and bound attempt | Exact typed operation and supported decisions. No authority or grant. 2 seconds. |
| `ReadSandboxReport` | Core-authorized audit bytes and cursor | At most 1000 records and 256 KiB, continuation cursor, completeness and integrity state. 5 seconds. |
| `ProposeSandboxRevision` | Exact source revision and operator-selected report entries | Typed access diff, provenance, compiled candidate. No persistence. 10 seconds. |

Total request size is 1 MiB. Artifact output is at most 1 MiB and 32 files.
Names are relative, normalized, and free of traversal or symlinks.
Only declared UTF-8 profile/config artifacts are accepted. Providers cannot return arbitrary executable files through policy compilation.
Binary helpers must ship in the reviewed plugin package and match its digest.
Runtime binaries remain explicit verified installation dependencies. The plugin does not download an unpinned runtime during launch.

The host expands reserved runtime references and materializes artifacts privately.
It checks provider effective grants against the policy snapshot and executor ceiling.
It admits the original child separately, then composes the launch through core.
The plugin receives no operator token, provider login secret, or complete inherited environment.
Endpoint placeholders let core insert callback secrets after compilation, without persisting them in provider reports.

No `Start`, `Stop`, arbitrary command, or policy-save RPC belongs to the provider interface.
Core owns those operations. Normalized approvals remain data until the host verifies and authorizes a decision.

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
