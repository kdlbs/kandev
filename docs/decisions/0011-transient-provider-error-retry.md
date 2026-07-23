# 0011: Transient provider errors (529 Overloaded) auto-retry with visible backoff

**Status:** accepted
**Date:** 2026-05-30
**Area:** backend, frontend

## Context

When the Anthropic API returns a transient `529 Overloaded` mid-turn, the agent
surfaces it over ACP as an error event that the backend turns into `agent.failed`.
Previously this was handled identically to a terminal failure: a red "Agent has
encountered an error" banner, the execution torn down, the task dropped to
`REVIEW`, and (in some paths) an instant silent re-resume — reading as a flickery
glitch rather than a deliberate retry. The classifier had no rule for 529, so it
fell through to `phase.poststart.unknown` → `CodeAgentRuntime` (`AutoRetryable=false`).

## Decision

Transient provider overload is now a first-class, auto-retryable condition with a
calm, paced, visible retry — separate from generic agent failures.

- **Classification** (`internal/agent/runtime/routingerr/`): new runtime rule
  `anthropic.overloaded.529.v1` matches the `529 … overloaded` / `overloaded_error`
  signature and maps it to a dedicated `CodeProviderOverloaded`
  (`AutoRetryable=true, FallbackAllowed=true`). Chose a dedicated code over reusing
  `CodeProviderUnavailable` (HTTP-503 semantics) for distinct UX copy and clean
  `routing_code: provider_overloaded` metrics. Exposes
  `IsTransientProviderError(msg) bool`, mirroring `IsResumeCorrupted`.
- **Orchestrator** (`internal/orchestrator/event_handlers_transient.go`):
  `handleAgentFailed` routes transient errors to `handleTransientFailure` before
  the red recoverable-failure path. It schedules a backoff retry (5s → 15s → 30s,
  max 3 attempts) via a Service-owned cancellable timer goroutine (mirroring the
  clarification-watchdog pattern; drained in `Stop()`), parks the session in
  `WAITING_FOR_INPUT` (calm, banner-less) without moving the task to `REVIEW` or
  cleaning up, and emits a `variant: "warning"` status message with a Cancel
  action. On each retry it tears down the failed execution and re-drives the
  cached prompt via `PromptTask` (resuming a fresh agent). The prompt is cached
  both in `PromptTask` (follow-ups) and in the `LaunchPreparedSession` initial
  paths (`startTask`, `StartCreatedSession`) so a 529 on the very first turn can
  retry too; if no prompt is cached, the retry clears itself and surfaces the
  recovery banner rather than parking. After the budget is exhausted, or on
  Cancel (`session.recover` action `cancel_retry`), it falls through to the
  existing red Resume / Start-fresh banner (with friendly "stayed overloaded
  after several retries" copy instead of the raw 529). Gated to non-office tasks
  (`isOfficeTask`, i.e. authoritative Office ownership plus an assigned runner)
  so genuine office tasks keep
  their structured error UI. The transient branch runs *before* run-mode
  automation finalization in `handleAgentFailed` — it's the only non-terminal
  failure path, so finalizing (which reaps the ephemeral worktree) must wait
  until the budget is actually spent. The synchronous prompt-error path
  (`handlePromptError`) also skips the `REVIEW` transition for transient errors.
  `httpStatusToCode` maps a structured `529` to `CodeProviderOverloaded` too,
  for adapters that surface the status separately from the body.
- **Frontend** (`components/task/chat/messages/action-message.tsx`): the existing
  `variant: "warning"` rendering (amber, not red) drives the retry card; the
  bottom `AgentStatus` banner is never `FAILED` during retry, so there is no red
  flicker. The Cancel button dispatches the `cancel_retry` `ws_request`.
- **Repro** (`cmd/mock-agent/`): a `/overloaded[:N]` scenario returns a real
  prompt-time ACP error carrying the production 529 string for the first N
  prompts (file-backed counter), then recovers.

## Consequences

- A 529 mid-turn now reads as "Provider overloaded — retrying in Ns (attempt X/Y)"
  with a Cancel button instead of a red error flash, and self-heals when the
  provider recovers, without user intervention.
- Adds a per-session retry-loop owner the Service must drain on shutdown
  (goleak-covered).
- Re-driving via `PromptTask` re-sends the user prompt to the resumed agent; for a
  real agent this could append a duplicate user turn in agent-side history — an
  accepted limitation for the transient-recovery case.

## Alternatives Considered

- **Reuse `CodeProviderUnavailable`** — rejected: conflates HTTP-503 "unavailable"
  with "temporarily overloaded, retry"; muddies metrics and UX copy.
- **Keep the agent process alive and re-prompt in place** — rejected: the lifecycle
  marks the execution FAILED on a prompt error and rejects further prompts, so the
  retry must tear down and resume a fresh execution.
- **Office-style FAILED state for transient errors** — rejected: the goal is an
  uninterrupted working→retrying→working chat, not a terminal failure surface.
