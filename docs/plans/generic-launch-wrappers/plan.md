---
created: 2026-10-14
updated: 2026-10-16
status: draft
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
system_design:
  - ../../specs/agents/system-design/profile-capability-discovery.md
legacy_specs: []
---

# Generic launch wrappers: investigation and design handoff

## Status and ownership

This is a docs-only investigation handoff, not an implementation-ready design package.
The linked requirements cover existing profile discovery behavior. They do not yet
specify generic wrapper admission, passthrough changes, or a sandbox plugin contract.
Complete the requirements, system design, and implementation work orders before
production changes. No permanent code or regression tests are included in this draft.

Current Kandev task: `06d00e9a-1f0d-4392-90ed-aa665334a592`.
Work continues here. The user deprecated task `6e852cad-2b53-47a8-a51a-89c8a31bb176`,
titled "Issue #3812: Sandbox providers for local executors".
Do not restart, modify, or move work into that task.
[Issue #3812](https://github.com/kdlbs/kandev/issues/3812) remains background material.

## Problem and evidence

Pi with a Greywall command prefix exposed inconsistent launch preparation:

- Ordinary ACP session construction prepends the configured prefix.
- Profile discovery and utility execution resolve that prefix through an
  agent-executable allowlist that rejects Greywall before it runs.
- Pi CLI passthrough constructs the native Pi command without the profile prefix.
  Its apparent success therefore does not establish sandbox execution.

A temporary build overlay admitted the exact `greywall` token to isolate the failure.
That exposed a separate policy problem: Greywall's private `/tmp` hides both Kandev's
managed npm project directory and the temporary probe working directory.
Explicit grants for those paths allowed the real adapter to start.

Real tests covered discovery, a provider round trip, file tools, and a normal Kandev
chat session with passthrough disabled. Pi wrote the expected result and replied
`VERIFIED:2910`. See [reproduction.md](reproduction.md) for versions and limitations.
The original reporter's exact command, platform, and policy remain unknown.

The user rejected the exact-name patch as a permanent fix. The overlay and its
machine-specific profile are diagnostic evidence only. Do not implement a new
allowlist entry for Greywall, nono, or another named wrapper as this task's solution.

## Current direction

### Generic wrapping in core

Basic wrapping does not require a provider-specific Go adapter or an installed plugin.
Investigate a shared launch-preparation contract that takes an operator-authorized
executable and structured arguments, then applies them consistently to each supported
process path. Preserve the trusted agent command, argument order, environment handling,
process ownership, and cancellation behavior.

Separate wrapper admission from agent-command admission. Do not simply remove the
utility allowlist and create an arbitrary-execution endpoint. Specify how authorized
profile settings reach the execution boundary, including draft discovery requests.
Keep the fixed deployment guard's identity and policy distinct from user configuration.
Wrapper failures must not trigger an unwrapped retry or a different launch context.

### Runtime context without machine-specific paths

Kandev knows its working directory and managed runtime paths. Investigate how to expose
the required values without shell interpolation, hard-coded temporary paths, secret
disclosure, or automatic permission expansion. Typed argument substitutions and
declarative configuration are candidates, not a selected public syntax.
Resolve directories before any wrapper needs to reference them.

An arbitrary executable has no universal filesystem or network-policy language.
Its mapping must come from operator configuration, a declarative preset, or optional
plugin behavior. Supplying a path is not permission to grant access to it.
Report policy conflicts instead of granting all of `/tmp` or retrying without confinement.
A generic command wrapper does not prove that the configured executable is a sandbox.

### Optional plugin integration

A plugin could provide configuration, availability checks, setup, supervisor integration,
approval requests, or denial reports. A Greywall plugin must not be required for basic
command-prefix compatibility.

The existing `ExecutorProviderPlugin` SDK interface provisions and attaches remote
environments. It does not expose a local process-wrapper hook. Reuse plugin packaging,
versioning, capability checks, and transport if a new capability is justified, but do
not model a local wrapper as a remote executor or use UI actions for launch authority.
Core should retain process/ACP ownership, cancellation, authorization, and session identity.
The exact plugin contract and whether the first increment needs it remain open.

## Decisions the next agent must resolve

1. **Authority and admission:** Define who can configure and execute wrappers,
   binary/path resolution, environment precedence, draft validation, guard composition,
   and the distinction between customization and enforced confinement. Reconcile the
   [launcher authority ADR](../../decisions/2026-07-24-operator-owned-agent-launcher-settings.md)
   with current authentication code; its historical description is not a current audit.
2. **Execution coverage and scope:** Inventory discovery, model-option resolution,
   secondary model commands, utility prompts, startup, continuation, restart, recovery,
   and passthrough. Specify supported wrapping or explicit refusal for each path.
   Host discovery must not claim to validate a selected executor's sandbox. Discovery
   must not create task sessions or send model prompts.
3. **Runtime inputs:** Define exposed paths and environment values, preparation order,
   argv boundaries, and generated-file ownership. Preserve managed npm isolation and
   do not automatically widen operator policy.
4. **Configuration and compatibility:** Decide whether existing profile prefixes are
   sufficient initially or require executor-level configuration. If override levels
   are added, define precedence, migration, snapshots, and whether a task can loosen
   executor policy. Do not treat #3812's sketch as an approved schema.
5. **Plugin scope:** Decide between declarative-only support and a new typed plugin
   capability. Define unavailable-provider behavior and lifecycle ownership. Approval
   transport and policy learning need separate runtime evidence; #3812's triage did
   not establish a usable nono approval interface.

## Source map

| Area | Starting point |
| --- | --- |
| Profile token validation | `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`, `resolveProfileLaunchTokens` |
| Session and continuation argv | `apps/backend/internal/agent/runtime/lifecycle/command.go` |
| Probe and utility admission | `apps/backend/internal/agentctl/server/utility/acp_executor.go`, `resolveACPCommandPrefix` |
| Host probe context | `apps/backend/internal/agent/hostutility/profile_probe.go` |
| Runtime temporary directory | `apps/backend/internal/agent/managedruntime/npm_project.go` |
| Pi ACP and passthrough | `apps/backend/internal/agent/agents/pi_acp.go` |
| Plugin extension pattern | `apps/backend/pkg/pluginsdk/executor_provider.go`, `apps/backend/proto/kandev/plugin/v1/plugin.proto` |
| Plugin trust and packaging | `docs/public/plugins-authoring.md` |

## Verification expected from the eventual implementation

Use wrapper fixtures with unrelated executable names and paths to prove support does
not depend on a provider name. Exercise every supported path, argument boundaries,
environment handling, unavailable executables, wrapper failure, cancellation, guard
composition, authority rejection, and sensitive-error sanitization. An unwrapped
fallback must never count as success.

Retain Pi/Greywall as a real-provider acceptance scenario, including a probe working
directory under `/tmp`. Keep discovery separate from a billable prompt. Generic wrapper
fixtures prove composition, not another sandbox's platform support. Verify provider
claims with the actual programs.

## Work orders and next action

- [ ] [Task 01: Complete the generic wrapper design](task-01-complete-design.md).

The next deliverable is a reviewed design package with bounded implementation work
orders. This draft preserves evidence and direction; it does not ship the fix.
Public docs should describe supported behavior when implementation is ready, not
present this investigation or its temporary overlay as released functionality.
