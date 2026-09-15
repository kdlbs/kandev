# ADR-2026-09-14-explicit-plugin-utility-selection: Explicit plugin utility selection

**Status:** accepted
**Date:** 2026-09-14
**Area:** protocol

## Context

PR #2870 makes Host.InvokeUtilityAgent read the calling plugin's configuration to choose an execution profile.
The method arguments do not expose that selection. Settings > Utility Agents already provides a default profile.
The user rejected implicit configuration routing and requested an explicit override with default behavior when omitted.

## Decision

Plugins own persistence of their preferred profile and pass any override with the invocation.
The host uses the platform default only when the request omits a profile. It never reads plugin configuration for execution selection.
An invalid override fails without fallback. The existing agent_invoke permission remains required.
This supersedes ADR 0048's configuration-based selection and compatibility rules. Its sessionless execution boundary remains applicable.
The transport must reject clients requesting unsupported selection semantics before execution. Silent downgrade is forbidden.

## Consequences

Callers can reason about execution from arguments and the platform default. Settings and execution ownership are separate.
Old prompt-only clients use the default on the revised host. Clients that need their saved selection must send it explicitly.
The SDK interface changes for implementers. Updated clients require a host supporting the new wire method.
General agent execution with cwd remains outside this utility completion contract.

## Alternatives considered

- Implicit plugin configuration routing: rejected because the call hides its execution selection.
- Always use the default: rejected because plugins need explicit per-call overrides.
- Keep legacy configuration fallback: rejected because it preserves the hidden behavior the user removed.
- Add only an optional field to the old RPC: rejected because old servers can ignore that field and execute the wrong profile.

## Specifications

- [Requirements](../specs/plugins/requirements/plugin-explicit-utility-invocation.md)
- [System design](../specs/plugins/system-design/plugin-explicit-utility-invocation.md)
