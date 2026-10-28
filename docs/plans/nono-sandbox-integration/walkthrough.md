# Sandbox walkthrough: Kandev and Nono

Read sections 1 through 4 for the main design. Continue with sections 5 through 10 for the operational details and future adapters.

This guide describes the implementation inspected on 2026-10-27:

- [Kandev host PR #4769](https://github.com/kdlbs/kandev/pull/4769), at [`f0a7e7f35a2d`](https://github.com/kdlbs/kandev/tree/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f).
- [Nono plugin PR #2](https://github.com/kdlbs/kandev-plugin-nono/pull/2), at [`8f68c6a10add`](https://github.com/kdlbs/kandev-plugin-nono/tree/8f68c6a10add119a1187c7428c5fad51aa6ca25a).
- Nono runtime 0.80.0, built from upstream commit [`8bb041356d48`](https://github.com/nolabs-ai/nono/tree/8bb041356d48f771a221e58c98d9b1ff442e76fe).

After the SDK decoupling rebase on 2026-10-28, the current SDK lives in `pluginsdk/`, and its wire contract lives in `pluginsdk/proto/`. The Go import is `github.com/kdlbs/kandev/pluginsdk`. The pinned links below retain the earlier source locations for historical context.

Source links below use these commits. They remain useful after the branches change. This guide explains the delivered design; it does not replace the requirements or acceptance record.

## 1. What this initiative adds

An operator can select a sandbox policy for an agent run, review requests for extra file access, and inspect the launch record. Kandev preserves the selected permissions across automatic restarts. An operator can publish a policy revision from selected denial records, then explicitly adopt it for a later launch.

The host implementation provides this common behavior. The Nono implementation makes one sandbox runtime work with it.

Three concepts have separate jobs:

- **Wrapper:** a command prefix applied by Kandev core. Basic command wrapping works without a provider plugin.
- **Policy:** a versioned set of filesystem, network, environment, and Kandev MCP permissions. Kandev owns its selection and authority.
- **Provider:** an optional plugin that translates the common policy into a runtime-specific launch configuration and interprets runtime records.

The Nono binary performs confinement. The plugin understands Nono's formats. Kandev manages the run and decides who can change its permissions.

## 2. What users see, and when

Agent settings have an Execution section for wrapper configuration and sandbox selection. Executor settings define eligible policies, a default, and a permission ceiling. Workflow steps and tasks can select a policy within those constraints.

Session execution details expose the captured configuration, launch history, approvals, reports, and supported policy-learning actions. The execution indicator reflects available evidence. It must not imply verified protection merely because a wrapper was configured.

The sandbox feature uses `features.sandboxPolicies`, with environment override `KANDEV_FEATURES_SANDBOX_POLICIES`. It remains off in the shipped prod, dev, and e2e profiles. The generic wrapper UI changes and generic startup-recovery fixes are outside that feature flag.

The Nono provider currently supports Linux amd64 with Local and Worktree executors. The host requires the process-containment support used by this implementation, including delegated cgroup v2 and clone3. Other operating systems and remote executors are outside the verified support matrix.

A selected, required sandbox that cannot be prepared causes launch to fail. Kandev does not silently launch the agent without that sandbox.

## 3. Follow one launch

1. **Resolve the policy.** Kandev checks task selection, workflow-step selection, agent default, then executor default. The executor's eligible policies and permission ceiling still apply. A more specific selection does not bypass them.
2. **Capture the decision.** Kandev records the policy revision, digest, provenance, provider package, and attempt identity. This becomes the basis for restart and audit decisions.
3. **Ask the provider to compile.** The host supplies canonical permissions and explicit runtime facts. The plugin returns a command-prefix template, profile artifacts, effective permissions, and required capabilities.
4. **Validate and prepare.** Kandev checks the result against its contracts. It creates private runtime files, callback channels, any required network relay, and owned process containment. Dynamic values replace declared runtime references.
5. **Launch and supervise.** Core launches the command through the existing execution path. Core retains supervisor and PTY ownership, records process identity, and handles cancellation and cleanup.

An **attempt** identifies one launch. A restarted session has a new attempt even when it uses the same frozen policy.

Start reading the code at [policy resolution](https://github.com/kdlbs/kandev/blob/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f/apps/backend/internal/agent/sandbox/resolution.go), then [runtime preparation](https://github.com/kdlbs/kandev/blob/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f/apps/backend/internal/agent/sandboxruntime/prepare.go) and [materialization](https://github.com/kdlbs/kandev/blob/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f/apps/backend/internal/agent/sandboxruntime/materialize.go).

## 4. The host-to-plugin protocol

The optional capability uses the existing supervised plugin process and Protobuf/gRPC transport through HashiCorp go-plugin. The plugin manifest declares managed plugin API version 2, `sandbox_provider: true`, and sandbox protocol major version 1. These two version numbers describe different contracts.

The sandbox contract has six RPCs:

1. `DescribeSandboxProvider`: declares supported hosts, runtime identity, capabilities, evidence sources, and approval response formats.
2. `CompileSandboxPolicy`: translates a policy snapshot into a bounded launch recipe.
3. `NormalizeSandboxApproval`: parses a provider-specific request into a common approval operation.
4. `NormalizeSandboxStatus`: interprets runtime identity or confinement evidence. The host validates the returned evidence.
5. `ReadSandboxReport`: converts bounded audit input into denial records and reports coverage or integrity limits.
6. `ProposeSandboxRevision`: proposes permission additions from selected denial records and compiles the candidate.

Requests carry operation, attempt, execution, generation, installation, and provider identity. The context also carries a protocol version and deadline. Policy snapshots include revision and permission digests, the ceiling digest, and source provenance. These fields bind results to the intended operation and policy.

The plugin does not receive a sandbox `Start`, `Stop`, or `SavePolicy` RPC. Core retains those responsibilities. The plugin returns data for core to validate and use.

The production plugin lives in its dedicated repository. The host contains the SDK, protocol, loader integration, policy system, and test fixtures. Installing a plugin still means trusting executable host code; the RPC boundary does not isolate a malicious plugin.

Read the [wire contract](https://github.com/kdlbs/kandev/blob/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f/apps/backend/proto/kandev/plugin/v1/plugin.proto) and the [Nono manifest](https://github.com/kdlbs/kandev-plugin-nono/blob/8f68c6a10add119a1187c7428c5fad51aa6ca25a/manifest.yaml).

## 5. What the Nono plugin implements

The plugin packages a pinned runtime and describes its supported behavior. Its compiler turns canonical permissions into a Nono JSON profile and a launch prefix equivalent to `nono run --silent --profile <private-profile> -- ...`.

The compiler validates filesystem paths and network destinations. It returns explicit artifact bindings for values that only the host can create, such as approval and egress endpoints. The host verifies and materializes those bindings in private attempt storage.

The provider also declares prohibited overrides. Agent arguments or environment settings must not bypass the selected profile or replace its enforcement controls.

There are runtime-specific details here. For example, Nono's domain rules do not preserve the exact-port restriction needed by the host policy. The plugin therefore requests the core egress proxy capability. That proxy enforces the approved TCP destinations.

This keeps Nono-specific profile syntax and workarounds in the plugin, while the exact-destination enforcement mechanism remains reusable in core.

Read [provider description](https://github.com/kdlbs/kandev-plugin-nono/blob/8f68c6a10add119a1187c7428c5fad51aa6ca25a/server/plugin.go), [compiler](https://github.com/kdlbs/kandev-plugin-nono/blob/8f68c6a10add119a1187c7428c5fad51aa6ca25a/server/compiler.go), and [network translation](https://github.com/kdlbs/kandev-plugin-nono/blob/8f68c6a10add119a1187c7428c5fad51aa6ca25a/server/network.go).

## 6. Approvals and policy learning

Nono's verified approval path uses its file-access webhook. `inspect --json` is not an approval request/decision protocol.

When a supported file operation needs approval, Nono sends a structured request to the host-owned callback channel. The plugin strictly parses its schema. The host binds the request to the live attempt and process ownership before presenting it to the operator.

The operator can deny the operation or allow it once. Agent tool auto-approval does not grant sandbox operator authority. One user-visible read can cause multiple low-level requests, such as a MIME probe followed by the actual read. Each request retains its own decision.

The host tracks whether the decision was delivered. An interrupted or expired callback must not appear as a successfully granted operation. Non-file approval requests are outside the current Nono support.

Policy learning is a separate flow:

1. The operator selects recorded denials.
2. The plugin proposes the smallest supported permission additions for those records.
3. The host validates the candidate against the ceiling and stores its provenance.
4. The operator explicitly publishes a new immutable revision.
5. A later selection or reviewed restart explicitly adopts that revision.

Publication does not change a running attempt or automatically select the new revision. The host persists policy revisions, attempt snapshots, approval delivery states, reports, and learning proposals. The plugin does not own the policy database.

Read the [Nono approval parser](https://github.com/kdlbs/kandev-plugin-nono/blob/8f68c6a10add119a1187c7428c5fad51aa6ca25a/server/approval.go), [host approval delivery](https://github.com/kdlbs/kandev/blob/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f/apps/backend/internal/agent/sandbox/approval_delivery.go), and [Nono revision proposal](https://github.com/kdlbs/kandev-plugin-nono/blob/8f68c6a10add119a1187c7428c5fad51aa6ca25a/server/proposal.go).

## 7. How sandboxed Kandev MCP works

Kandev MCP access is an explicit logical permission. It does not imply unrestricted network access.

For an eligible task attempt, core creates a private relay with a dynamic loopback endpoint. The relay forwards the task's MCP connection and checks the attempt's ownership. The host supplies its exact address and port as runtime facts, then adds only the required endpoint to the effective runtime permissions.

The MCP relay and egress proxy have different jobs. The relay limits access to the task's MCP connection. The egress proxy enforces exact approved TCP destinations. Neither requires a hard-coded backend path or an allow-all network rule.

Two integration defects needed distinct fixes:

- The tested Pi ACP adapter did not forward the supplied MCP parameters. The host's Pi strategy supplies private child-home configuration instead. It does not copy the operator's retained credentials or depend on project trust.
- Nono injected localhost proxy-bypass variables. For MCP-enabled launches, the Nono plugin clears those bypass variables inside confinement, with explicit permissions for the required utility.

Actual acceptance used unpredictable markers written through Kandev MCP. A successful chat response alone was not sufficient evidence of connectivity.

Read [host MCP preparation](https://github.com/kdlbs/kandev/blob/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f/apps/backend/internal/agent/sandboxruntime/mcp.go) alongside the plugin compiler's MCP handling.

## 8. Restart, cancellation, and trustworthy status

Automatic restart uses the captured policy. A manual restart can explicitly review and adopt the current selection. The review binds the decision to the relevant version and digest, and the executor ceiling remains in force.

Restart rotates attempt-specific endpoints. Cancellation closes owned channels and stops owned descendants. Linux cgroup containment covers detached children that can survive ordinary process-session termination. Cleanup removes scoped state only when the ownership and stop conditions permit it.

Configured policy, a running process, and verified current confinement are different facts. To show active confinement, core requires fresh evidence bound to the attempt, generation, policy digest, and process identity.

**Current Nono limitation:** the session registry provides process identity, but no accepted current-policy attestation. The plugin returns identity without claiming confinement evidence. Thus a running Nono attempt retains unknown confinement status. Successful enforcement tests do not become live attestation or justify an active-protection shield.

Nono audit reports also have partial coverage. Their validated hash chain does not prove that every denied operation was recorded. The report exposes that limit.

Read [reviewed restart](https://github.com/kdlbs/kandev/blob/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f/apps/backend/internal/agent/sandbox/restart_review.go), [ownership and cleanup](https://github.com/kdlbs/kandev/blob/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f/apps/backend/internal/agent/sandboxruntime/ownership.go), [host evidence checks](https://github.com/kdlbs/kandev/blob/f0a7e7f35a2d9ac1af9fed10f32556f86d60f21f/apps/backend/internal/agent/sandboxruntime/evidence.go), and [Nono status normalization](https://github.com/kdlbs/kandev-plugin-nono/blob/8f68c6a10add119a1187c7428c5fad51aa6ca25a/server/status.go).

## 9. What the investigation and tests established

The original Task 04 startup failure exposed generic recovery problems, including a missing executor record, stale error state, and a queued prompt. These belong to task lifecycle and bootstrap recovery. A successful replacement sales run did not prove recovery of the original session.

MCP forwarding, Nono proxy bypass, exact-port enforcement, and detached-child cleanup required separate integration work. The test driver also needed to wait for asynchronous stop completion before asserting process exit. That last change corrected the test harness.

Recorded real acceptance covers discovery, ACP chat and tools, native terminal tools and PTY resize, approvals, learning, restart, cancellation, and cleanup. It pairs permitted operations with denied filesystem and network operations. It also checks MCP access and isolation between attempts. UI fixtures cover desktop and phone layouts.

The supported result is bounded: Linux amd64 Local/Worktree, the pinned runtime and package, explicit credentials for the test fixture, and the documented capabilities. It does not establish support for other runtimes, platforms, or current-policy attestation.

Use the [acceptance record](acceptance.md) for exact receipts and the [delivery work order](task-09-acceptance-delivery.md) for scope. The tracked runner is [scripts/check-nono-real.py](../../../scripts/check-nono-real.py). An actual managed run requires an explicitly owned, isolated fixture. The walkthrough does not start an instance or alter the main instance at port 9998.

## 10. Standards and future adapters

There are standards and shared APIs at several layers. They solve different problems:

- [OCI Runtime Specification](https://github.com/opencontainers/runtime-spec): container configuration and lifecycle. It does not define Kandev's operator policy-selection or learning model.
- [OpenSandbox APIs](https://open-sandbox.ai/api/implementation-compatibility): sandbox service lifecycle, command/file operations, and egress APIs, with implementation-specific coverage.
- [Kubernetes Agent Sandbox runtime API](https://agent-sandbox.sigs.k8s.io/docs/api/runtime/): process and filesystem access to sandbox environments. Kubernetes also supplies resource lifecycle management.
- [ACP](https://agentclientprotocol.com/protocol/v1/overview): communication between an agent and its client.
- [MCP](https://modelcontextprotocol.io/specification/latest/architecture): communication with tools and context servers.

The pinned Nono integration uses its CLI, JSON profiles, audit data, and [approval webhook protocol](https://github.com/nolabs-ai/nono/blob/8bb041356d48f771a221e58c98d9b1ff442e76fe/docs/protocols/approval-webhook-v1.md). It does not implement the OCI, OpenSandbox, or Kubernetes runtime service APIs listed above. ACP and MCP traffic can run inside its confinement.

These layers can compose. The [Kubernetes Nono example](https://agent-sandbox.sigs.k8s.io/docs/use-cases/examples/nono-sandbox/) places Nono inside a managed sandbox pod.

The current Nono plugin is already an adapter between Kandev's policy contract and one runtime. A future local policy engine could use the same capability if it can meet the contract. A service that creates and destroys remote environments may fit the executor boundary instead, possibly with a separate sandbox provider.

The recommendation is to add those adapters when a concrete backend or consumer needs them. An adapter must declare missing guarantees. It cannot manufacture exact-port enforcement, operator approval, or policy attestation that the backend cannot supply. Some integrations may require a versioned contract extension.

No OCI, OpenSandbox, or Kubernetes adapter is delivered by these two PRs.

## Reference terms

- **Ceiling:** the maximum permissions an executor permits for a selected policy.
- **Revision:** one immutable published version of a policy.
- **Attempt:** one prepared launch, with its own identity and runtime resources.
- **Provider:** the plugin capability that compiles policies and interprets runtime records.
- **Executor:** the mechanism that supplies the execution environment.
- **Attestation:** evidence of the current confinement state, bound to the exact process and policy.

For durable contracts, read [sandbox policy design](../../specs/agents/system-design/sandbox-policies.md) and [sandbox provider design](../../specs/plugins/system-design/sandbox-provider-capability.md). For implementation history, use the [plan](plan.md).
