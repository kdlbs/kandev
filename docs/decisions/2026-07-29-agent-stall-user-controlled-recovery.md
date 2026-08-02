# ADR-2026-07-29-agent-stall-user-controlled-recovery: Keep Agent Stall Recovery User Controlled

**Status:** accepted
**Date:** 2026-07-29
**Area:** backend, frontend, protocol

## Context

An ACP turn can stop emitting events while its process remains alive, most
concretely when a top-level shell tool starts a long-lived process and never
reaches a terminal status. Kandev already detects five minutes of inactivity,
but only logs the condition and leaves the session `RUNNING`. Automatically
timing out every quiet turn would also terminate legitimate long-running tools
whose providers emit no progress frames.

## Decision

Stall detection is advisory and recovery remains operator controlled:

- After five minutes without agent events, Kandev presents one neutral notice
  for the active prompt generation and keeps the session `RUNNING`.
- The notice identifies the active top-level tool when that information is
  available, without exposing raw command arguments, and provides a compact
  neutral **Cancel turn** action. Its wording and styling do not imply that a
  quiet long-running tool has failed.
- **Cancel turn** reuses the existing `agent.cancel` escalation path. That path
  bounds the ACP acknowledgement wait, releases a hung prompt, and reconciles
  the session to an input-ready state.
- Stall detection never automatically cancels, fails, or kills the agent
  process. `RUNNING` sessions retain the conservative prompt-admission behavior
  defined by
  [ADR-2026-07-28-coarse-running-busy-signal](2026-07-28-coarse-running-busy-signal.md).
- This advisory rule applies to inactivity without terminal evidence. A
  sanitized, session-correlated provider diagnostic follows
  [ADR-2026-08-02-agent-terminal-diagnostics-over-stderr](2026-08-02-agent-terminal-diagnostics-over-stderr.md)
  and may fail the prompt immediately.
- The watchdog checks once per 60 seconds and logs only the first detected stall
  for a prompt generation; it does not repeat a notice every check.

The observable behavior is specified in
[`docs/specs/agent-stall-recovery/spec.md`](../specs/agent-stall-recovery/spec.md).

## Consequences

Users get a reload-safe explanation and a direct recovery action without
Kandev guessing whether quiet work is expendable. Sessions do not become
promptable until the user cancels or the provider completes the turn, so queued
input and lifecycle ownership remain conservative. A genuinely abandoned turn
can remain `RUNNING` indefinitely if the user does not intervene.

Lifecycle must retain enough in-memory top-level tool identity to enrich the
notice, while the persisted notice message remains the durable UI record.

## Alternatives Considered

- **Automatic timeout.** Rejected because event silence is not proof that a
  command is safe to terminate.
- **Warn, then automatically timeout after a grace period.** Rejected for the
  same ambiguity; a longer delay reduces but does not remove destructive false
  positives.
- **Continue logging only.** Rejected because logs do not make the session
  recoverable or explain the blocked Resume action to the user.
