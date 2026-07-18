# 0044: ACP Agent Compatibility Dialects

**Status:** accepted
**Date:** 2026-07-16
**Area:** backend, protocol

## Context

ACP defines common transport and session methods, but individual agent CLIs can expose models, configuration, usage, and recovery through private metadata or non-standard method combinations. Grok, for example, combines the legacy `models` response with model metadata for reasoning controls and uses `session/set_model` for both model and reasoning changes. The live adapter and the sessionless utility probe must interpret those capabilities consistently.

## Decision

The ACP adapter selects a package-private `acpDialect` function table by agent ID.

- Dialect hooks are optional, pure compatibility functions that return normalized data or request descriptions. They never receive `*Adapter`, execute RPCs, mutate session state, or emit events.
- Shared adapter code owns JSON-RPC transport, session lifecycle, serialized configuration changes, generation checks, state convergence, and event delivery.
- Canonical ACP behavior is the zero-value dialect; there is no standard implementation containing no-op methods.
- Capability normalization shared by live sessions and utility probes lives in `internal/agentctl/acpcompat/`.
- Grok-specific model, reasoning, usage, notification, and incompatible-agent error translation lives in `dialect_grok.go` and the shared compatibility package.
- A dialect never replaces an active session to satisfy a model change. When an implementation requires another agent harness, Kandev returns its actionable error and the user starts a new session explicitly.

## Consequences

- New ACP CLI variants can add narrowly scoped functions without spreading agent checks through shared adapter files.
- Provider-specific parsing is reusable outside the live adapter when capability discovery needs the same interpretation.
- Concurrent model and configuration requests converge on the latest generation and stale session completions cannot overwrite replacement state.
- A small function table adds indirection to ACP session and update handling.
- No product spec covers internal ACP compatibility handling; this decision remains the durable record.
- Model changes cannot silently discard upstream agent context by replacing the ACP session.

## Alternatives Considered

### Use a stateful ACP driver interface

Rejected because the hooks crossed ownership boundaries by receiving the concrete adapter, executing RPCs, mutating shared state, and emitting events. It also required a standard implementation made mostly of no-op methods.

### Keep agent checks in shared ACP files

Rejected because each new non-standard implementation would increase branching in common session and notification paths. Small optional dialect functions keep selection centralized without creating provider objects.

### Create a complete adapter for every ACP CLI

Rejected because transport, lifecycle, permissions, tools, and event delivery are already shared correctly and would be duplicated.
