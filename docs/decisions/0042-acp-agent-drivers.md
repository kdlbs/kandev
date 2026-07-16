# ADR-0042: ACP Agent Drivers

**Status:** accepted
**Date:** 2026-07-16
**Area:** backend, protocol

## Context

ACP defines common transport and session methods, but individual agent CLIs can expose models, configuration, usage, and recovery through private metadata or non-standard method combinations. Embedding those differences directly in the shared ACP session and update paths adds agent checks to common code and does not scale when another CLI needs similar translation.

## Decision

The ACP adapter delegates agent-specific protocol behavior to an `ACPDriver` selected by agent ID.

- Shared adapter code owns JSON-RPC transport, session lifecycle, event delivery, and provider-neutral state.
- The standard driver preserves canonical ACP behavior without agent-specific branches.
- Agent drivers translate private session metadata, configuration changes, notifications, usage, and protocol errors into existing Kandev shapes.
- Grok-specific model, reasoning, usage, and incompatible-agent error handling lives in the Grok driver.
- A driver never replaces an active session to satisfy a model change. When an implementation requires another agent harness, Kandev returns its actionable error and the user starts a new session explicitly.
- Driver hooks remain narrow and are added only for observed protocol differences.

## Consequences

- New ACP CLI variants can add a driver without spreading agent checks through shared adapter files.
- Shared transport and lifecycle behavior remain reusable and independently testable.
- Drivers reuse adapter event operations when translation emits convergence events.
- A small delegation layer adds indirection to ACP session and update handling.
- No product spec covers ACP driver error handling; this decision remains the durable record.
- Model changes cannot silently discard upstream agent context by replacing the ACP session.

## Alternatives Considered

### Keep agent checks in shared ACP files

Rejected because each new non-standard implementation would increase branching in common session and notification paths.

### Create a complete adapter for every ACP CLI

Rejected because transport, lifecycle, permissions, tools, and event delivery are already shared correctly and would be duplicated.

### Name the abstraction a compatibility layer

Rejected because drivers represent each CLI's ACP implementation, including the standard implementation; they are not limited to repairing incompatibilities.
