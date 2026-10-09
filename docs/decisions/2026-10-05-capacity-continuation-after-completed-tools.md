# ADR-2026-10-05-capacity-continuation-after-completed-tools: Continue capacity failures after completed tools

**Status:** accepted
**Date:** 2026-10-05
**Area:** backend, protocol

## Context

The reported Codex ACP task stopped with `model_capacity`, `runtime_retained=true`, `recovery_disposition=refused`, and zero started retries.
The turn contained output and completed tool calls.
The existing replay fence rejects such turns before the exponential backoff scheduler can own recovery.
Existing native continuation supports transport loss with verified read-only work.

The user selected same-conversation continuation after completed tools.
The chosen policy preserves completed actions and stops when tools, permissions, or outcomes remain unresolved.
Platform owns shared recovery admission and runtime lifetime.

## Decision

Permit a narrow capacity continuation policy on the exact same settled, usable ACP runtime.
Automation-origin tasks retain their existing failure policy and are excluded from this after-effects continuation.
Completed shell, write, and MCP calls do not alone prohibit this policy.
Every observed tool must have an authoritative completed outcome.
Unknown outcomes, pending permissions, and unaccounted background work prohibit automatic continuation.

Adapters attest current-generation evidence through a versioned typed contract.
Initial production support covers tested Codex ACP shapes.
Orchestration consumes capability evidence and the shared semantic classifier without provider-name branches.
Original-prompt replay keeps its existing evidence fence.

The retry sends a new instruction to continue unfinished work from existing history.
It instructs the agent not to repeat completed actions and to ask about uncertain outcomes.
It preserves the process, connection, native conversation, configuration, and transcript.
Runtime loss stops this policy. It never restores or creates a conversation automatically after effects.

The existing scheduler owns five attempts with 5, 10, 20, 40, and 60-second delays.
The capacity policy does not depend on the experimental transport-loss continuation toggle.
It does not broaden that toggle's provider support, tool policy, or restoration contract.

This amends the completed-effect restriction in [the prior continuation decision](2026-10-02-safe-interrupted-conversation-continuation.md) only for this live capacity policy.
The [runtime lifetime decision](2026-10-03-transient-turn-runtime-lifetime.md) remains authoritative for runtime retention.

## Consequences

Capacity errors after completed work can recover without another user message or a process restart.
The model must interpret unfinished work from conversation history. The policy does not guarantee exactly-once actions.
Unknown tool outcomes remain manual even when the process stays usable.
Older remote components cannot authorize the new policy without its typed evidence.

## Alternatives considered

- Replay the original prompt: rejected because it can repeat completed actions.
- Keep the read-only restriction: rejected because it leaves the reported shell-tool case without automatic recovery.
- Restore a saved conversation after runtime loss: excluded because this policy depends on the exact live conversation after effects.
- Retry every transient error after tools: excluded because the user requested capacity recovery and other error owners remain distinct.
