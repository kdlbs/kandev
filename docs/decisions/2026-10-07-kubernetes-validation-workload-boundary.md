# ADR-2026-10-07-kubernetes-validation-workload-boundary: Isolate worker validation from agent control

**Status:** accepted
**Date:** 2026-10-07
**Area:** backend, infra, workflow

## Context

Concurrent full lint and browser checks exhausted an 8 GiB agent container. Its
OOM group killed agentctl and the native provider along with test processes.
Per-browser worker limits did not budget a concurrently started linter. Native
providers execute shell commands directly, bypassing agentctl's shell API.
The full-worker recipe already has an explicitly privileged task-owned Docker
companion with workspace access and nested cgroup accounting acceptance.

## Decision

Supported heavy validation entry points on an explicitly enabled full worker
execute in one bounded Docker workload under the existing companion. All sibling
sessions share the companion's single admission slot and conservative memory
budget. Resource accounting must be demonstrated on the selected runtime;
isolation failure cannot silently fall back to the agent container.

Keep the agent/control container independent of validation descendants. Preserve
existing workspace, exact cleanup ownership, immutable image policy and trust
boundaries. Separately repair restart-safe stop authentication so a control-server
restart remains recoverable regardless of its cause. No new public endpoint,
persistence table, privileged companion, or automatic production rollout is added.

## Consequences

A validator OOM fails its check without intentionally killing agent control.
Checks require workspace-only paths and explicit test inputs; daemon-dependent
and agent-only callback fixtures need separate policy. Serial admission trades
throughput for predictable capacity. Arbitrary direct provider shell commands
remain outside entry-point enforcement, so guidance must name covered commands.
Node-level memory pressure remains an operator concern; this does not promise
that a Pod can never restart or that trusted users cannot bypass the runner.

## Alternatives Considered

- Increasing agent memory only postpones failure and retains the shared OOM fate.
- Go heap/concurrency and browser-worker settings alone are cooperative tuning,
  not a hard descendant memory boundary or shared admission policy.
- Wrapping agentctl shell misses native-provider subprocesses.
- Delegated host cgroups require privilege/delegation not present in the current
  non-root agent recipe and add another operator compatibility boundary.
- A new companion/Job per check adds bootstrap, ownership, credentials and cleanup
  machinery already supplied by the task-owned Docker companion.

The [requirement](../specs/executors/requirements/kubernetes-validation-isolation.md)
and [design](../specs/executors/system-design/kubernetes-validation-isolation.md)
own the observable and technical contracts.
