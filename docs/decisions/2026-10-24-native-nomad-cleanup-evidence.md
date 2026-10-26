# ADR-2026-10-24-native-nomad-cleanup-evidence: Built-in native Nomad execution and cooperative cleanup

**Status:** accepted
**Date:** 2026-10-24
**Area:** backend, infra

## Context

The initial validation target is Docker-free native execution on an existing
macOS worker; macOS is the first test platform, not the architecture's only
supported platform. The capability requires end-to-end runtime evidence. The
current ExecutorBackend seam supports independently managed runtimes and the
lifecycle package already owns SSH artifact transfer, pinned-host
authentication, forwarding and reverse API tunnels. The external provider SDK
offers seven lifecycle methods, but cannot reuse those internal helpers and
carries no authenticated completed-drain receipt. It would also require a
dedicated production repository, managed gRPC packaging and public
non-loopback HTTPS gateway for connection leases.

Nomad raw_exec supervises its leader process group; agentctl creates independent
managed ACP groups. Terminal allocation state is not their absence proof. The
current ownership/shutdown response precedes teardown. The instance manager
already owns admission closure, in-flight/provisional work and process teardown.

## Decision

Implement Nomad as a built-in ExecutorBackend for the smallest complete native
integration and fastest real-worker feedback. Keep Nomad vendor code cohesive
for upstream review. Reuse internal SSH
transport/artifact primitives without invoking SSHExecutor's process launcher;
Nomad alone launches and owns the job. Do not add a mandatory public HTTPS
gateway, Caddy, plugin repository or provider SDK extension.

Add a generic authenticated seal-and-drain agentctl capability backed by the
existing instance manager. It reports success only after managed teardown and
keeps its leader alive and permanently sealed. The built-in executor durably
binds that receipt to the exact resource, runtime ServerIdentity, environment
generation and cleanup claim before stopping the whole immutable Nomad job.

Confirmed absence requires both scoped successful drain and exact live-worker
job/allocation stop evidence. A non-expiring atomic worker-local start fence must precede every agentctl
launch and survives leader/backend failure. Only exact positive cleanup or
externally verified operator recovery releases it; an already-queued B cannot
launch while A may have surviving groups. This safety lock does not place work
or replace Nomad resource reservations. Failed/lost drain, changed runtime, missing history
or unreachable worker retains cleanup-pending state and worker quarantine.
This covers agentctl-managed process groups under trusted native workloads, not
hostile daemonization or a hard macOS sandbox.

Per-environment SQL metadata CAS owns that environment only. It is not a
cross-session NodeID claim. Existing executors_running metadata plus narrow
ownership/revision-checked checkpoint/release methods, the authoritative worker
start fence and CAS Nomad operation/quarantine interlocks suffice. Do not add a
SQL worker-claim table or capacity ledger for this implementation. Retain stopped
and cleanup-pending envelopes until exact positive resource release.

Unfinished pre-instance creation continues under the original operation and
normal manager admission, not initialized-session recovery. Original nonce and
post-handshake token references must checkpoint before their next external side
effect. The disposable fixture may receive an explicitly verified operator
credential-reference repair from its exact owned private bootstrap file; this
is never production automatic discovery or evidence that automatic recovery
passed. Missing consumed credentials retain quarantine and require exact owned
cleanup/external recovery before any fresh operation.

Profile workspace_root is a base, not a checkout shared by all tasks. Record a
separate exact environment/generation WorkspacePath in the immutable launch
state and use it across job/controller/instance/recovery. Nomad must include
its primary repository in existing agentctl materialization because it has no
SSH prepare script. Pre-release fixtures missing the exact path retain their
original resources for cleanup rather than being relocated in place.

Recovery compares the complete expected fixed job policy, not just ownership
metadata or fields surviving a small decoder. Original staged paths and a
versioned expected spec digest checkpoint before register; server defaults are
normalized only through a closed reviewed compatibility table. Creation and
definition indexes remain separate from volatile status indexes. The
[job identity contract](../specs/executors/system-design/nomad-native-job-identity.md)
defines that boundary; unknown behavior or missing pre-release evidence stays
unknown without job mutation or replacement.

A Created task session can already own a live Nomad allocation. Preserve its
unfinished operation independently of conversational resumability and continue
it under the original manager-owned guard after readiness. Publish uninitialized
workspace infrastructure before releasing that exact pending owner; only the
existing durable delivery owner can perform first ACP initialization/dispatch.
The [startup continuation contract](../specs/executors/system-design/nomad-native-startup-continuation.md)
also restricts reconstruction of accidentally pruned test inventory to explicit
operator cleanup, excluded from automatic recovery evidence.
Capture the original workspace digest input independently of the resolved
environment path, including explicit empty-input presence. Runtime projection
cannot change input identity; historical enrichment requires unique durable
provenance and an exact original digest match under the existing operation CAS.

## Consequences

No public plugin protocol changes are needed for the initial contribution.
Kandev does acquire a Nomad client dependency and built-in profile/runtime
registration. Internal helper reuse needs focused SSH regressions and must not
turn into a new generic executor framework. If the built-in boundary proves
unsuitable, moving cohesive vendor code to a dedicated plugin would require a
separate design, not a parallel implementation in this plan.

Normal cooperative cancellation can settle positively without a supervisor.
Crashes and ambiguous histories may require operator verification or a controlled
worker reboot; automatic cleanup cannot manufacture evidence. Dedicated-node
eligibility quarantine is worker-wide, so shared/general-purpose nodes are not
supported by the first profile.

## Alternatives Considered

- **External plugin first:** fits the existing extension model, but adds managed
  transport/package/HTTPS and SDK cleanup evidence work for this native target.
  A plugin remains a separate future option if the built-in boundary proves
  unsuitable.
- **Call SSHExecutor.CreateInstance after Nomad allocation:** would launch a
  second unmanaged agentctl and split process ownership. Reuse helpers only.
- **Accept terminal raw_exec status or ownership/shutdown:** neither proves
  separate managed ACP groups were reaped.
- **Keep every cleanup pending forever:** cannot complete the normal lifecycle.
- **Custom supervisor/scheduler:** duplicates existing ownership; excluded.
- **Docker/VM or mandatory Linux migration:** conflicts with the chosen target.
  Bare-metal Linux exec/cgroups remains optional future Docker-free support.

## Related contracts

- [Requirements](../specs/executors/requirements/nomad-native-executor.md)
- [System design](../specs/executors/system-design/nomad-native-executor.md)
- [Runtime inventory](0003-executors-running-as-execution-id-source-of-truth.md)
- [Cleanup inventory](0025-runtime-cleanup-uses-executors-running.md)
