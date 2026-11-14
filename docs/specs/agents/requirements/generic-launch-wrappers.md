---
status: active
system: agents
created: 2026-10-16
owners:
  - kandev
---

# Generic launch wrapper requirements

## Overview

An operator can run an agent through a configured executable without installing a provider-specific plugin.
Agents owns this contract because it owns profile launch settings and their execution semantics.
A wrapper is a command customization. Its presence alone does not prove sandbox confinement.

[Profile discovery](profile-capability-discovery.md) retains ownership of discovery state and model-picker behavior.
[Managed npm recovery](managed-npm-runtime-recovery.md) retains ownership of package repair.
This contract adds consistent wrapper execution and explicit refusal where support is absent.

## Terms

- **Wrapper:** An executable and ordered arguments placed before a trusted agent command.
- **Execution host:** The machine or container that starts the agent process.
- **Runtime reference:** A wrapper argument that requests a specific Kandev-owned path on that execution host.
- **Deployment guard:** An external deployment restriction that profile settings cannot remove or impersonate.

## Requirements

### REQ-AGENTS-WRAPPERS-001: Authorized generic configuration

**Intent:** Operators can select wrappers without changing provider code or weakening command admission.

#### Acceptance criteria

- **AC-AGENTS-WRAPPERS-001.1:** An authorized profile prefix shall accept an installed executable by name or explicit path, independent of provider identity and plugin availability.
- **AC-AGENTS-WRAPPERS-001.2:** Saved and draft wrapper execution shall enforce the existing profile-execution authority and secret-access rules before starting a process.
- **AC-AGENTS-WRAPPERS-001.3:** Task inputs and agent credentials shall not authorize changes to wrapper settings or select an arbitrary inner agent command.
- **AC-AGENTS-WRAPPERS-001.4:** Authentication-disabled installations shall retain their existing customization behavior. Kandev shall not describe their synthetic administrator as authenticated operator confinement.
- **AC-AGENTS-WRAPPERS-001.5:** A profile wrapper shall not replace an enforced deployment guard. A matching basename shall not establish guard identity or privilege.

### REQ-AGENTS-WRAPPERS-002: Execution-host runtime context

**Intent:** Operators can reference required runtime directories without embedding one machine's temporary paths.

#### Acceptance criteria

- **AC-AGENTS-WRAPPERS-002.1:** A wrapper can reference the actual working directory and managed npm project directory on its execution host.
- **AC-AGENTS-WRAPPERS-002.2:** Referenced managed directories shall exist before wrapper startup. Unavailable or unknown references shall fail before startup.
- **AC-AGENTS-WRAPPERS-002.3:** Each reference shall produce one argument. Spaces, quotes, shell operators, and dollar signs in values shall remain data.
- **AC-AGENTS-WRAPPERS-002.4:** Path exposure shall not grant filesystem or network access, broaden policy, install a wrapper, or move managed npm state into the workspace.
- **AC-AGENTS-WRAPPERS-002.5:** The wrapper and its child shall receive the effective profile environment after secret resolution and final environment stripping. Missing secrets shall fail visibly.

### REQ-AGENTS-WRAPPERS-003: Consistent execution coverage

**Intent:** Every supported process launch uses the requested wrapper exactly once.

#### Acceptance criteria

- **AC-AGENTS-WRAPPERS-003.1:** ACP model discovery, model-option resolution, and applicable secondary discovery commands shall use the same authorized wrapper context.
- **AC-AGENTS-WRAPPERS-003.2:** Profile-backed ACP utility prompts shall apply the wrapper on both the host and the selected execution environment.
- **AC-AGENTS-WRAPPERS-003.3:** Initial sessions, one-shot continuations, restart, context reset, and replacement processes shall preserve the wrapper when these paths are supported.
- **AC-AGENTS-WRAPPERS-003.4:** Supported CLI passthrough paths shall apply the wrapper to initial, fresh, and resumed native commands, including Pi.
- **AC-AGENTS-WRAPPERS-003.5:** Host discovery shall remain prompt-free and session-free at the Kandev level. Its success shall not establish support on another executor.

### REQ-AGENTS-WRAPPERS-004: Failure, recovery, and compatibility

**Intent:** A wrapper failure remains visible without silently changing the execution policy.

#### Acceptance criteria

- **AC-AGENTS-WRAPPERS-004.1:** Missing executables, invalid arguments, policy failures, and unsupported transports shall stop the requested launch without an unwrapped retry.
- **AC-AGENTS-WRAPPERS-004.2:** Cancellation, timeout, and failed startup shall terminate owned wrapper descendants and release operation-owned resources within existing cleanup bounds.
- **AC-AGENTS-WRAPPERS-004.3:** Literal prefixes and profiles without prefixes shall retain argument order and empty-argument semantics. Structured arguments shall remain authoritative over display strings.
- **AC-AGENTS-WRAPPERS-004.4:** An incompatible execution host shall refuse a new wrapped launch. Recovery shall not adopt, replace, or reconfigure a process using ambiguous wrapper state.
- **AC-AGENTS-WRAPPERS-004.5:** Responses and logs shall identify the failure stage without exposing credentials, raw wrapper arguments, or unfiltered wrapper stderr.

### REQ-AGENTS-WRAPPERS-005: Optional sandbox integration

**Intent:** Basic wrapping remains independent of optional sandbox-specific services.

#### Acceptance criteria

- **AC-AGENTS-WRAPPERS-005.1:** Removing or disabling all sandbox plugins shall not prevent a valid generic prefix from running.
- **AC-AGENTS-WRAPPERS-005.2:** Generic wrapping shall not require provider setup, approval transport, policy learning, or sandbox-health services from a plugin.
- **AC-AGENTS-WRAPPERS-005.3:** Kandev shall distinguish successful wrapper execution from verified confinement. Generic success shall not claim another platform's sandbox support.

## Exclusions

No executor-level wrapper setting, task override, automatic permission grant, provider-name exception, or sandbox plugin API ships in this increment.
Native non-ACP utility transports may refuse wrapper contexts explicitly.
Package installation, login commands, repository setup, user shells, and MCP server processes are separate execution contracts.
No new rendered controls or layout changes are required. Existing desktop and phone profile workflows retain their state and recovery behavior.

## Implementation plans

- [Generic launch wrappers](../../../plans/generic-launch-wrappers/plan.md)
