# ADR-2026-07-18-turn-configuration-snapshots: Attribute Runtime Configuration to Turns

**Status:** accepted
**Date:** 2026-07-18
**Area:** backend, frontend

## Context

Task-session runtime configuration is mutable across a conversation, while agent messages are historical output. Rendering every message with the session's latest model-adjacent options can relabel old output after a user changes model, reasoning, collaboration, or other provider settings. Persisting the same full snapshot on every streamed message would duplicate data and still leave message bundles within one prompt cycle to coordinate.

## Decision

The task turn owns the immutable runtime-configuration attribution for one prompt/response cycle. When `internal/task/service` creates a turn, it captures the effective model, mode, ordered selected ACP option values and display names, and the task-session provider-default baseline in turn metadata. Message rendering reads this snapshot through the existing turn payload; it does not fall back to mutable task-session options for historical turns.

Provider-reported prompt metadata may add or refine actual execution attribution such as model and usage on the same turn. It must preserve the captured option values and baseline. No schema migration is required because `task_session_turns.metadata` is already durable JSON.

## Consequences

Historical message metadata remains stable when later turns change configuration, and all messages in one turn share one compact snapshot. Turn rows become slightly larger, but the snapshot stores only selected display values rather than the provider's complete option catalog. Legacy turns remain truthful but less detailed: they show available model attribution and do not infer options from current session state.

## Alternatives Considered

- **Read current session configuration at render time:** rejected because it rewrites historical attribution after later changes.
- **Snapshot configuration on every message:** rejected because streamed and tool messages duplicate identical turn-level data and complicate consistency.
- **Store only raw option maps on turns:** rejected because later UI state would still be needed for provider order and human-readable names.
