# ADR-2026-09-01-passthrough-initial-prompt-turn-boundary: Keep Passthrough Initial Prompt State in Lifecycle

**Status:** accepted
**Date:** 2026-09-01
**Area:** backend, workflow

## Context

CLI passthrough uses the first PTY idle window as the readiness signal for
injecting an initial task prompt. The interactive runner also reports every
idle window as a completed turn. On a fresh process, the first callback can
therefore publish `agent.ready` before prompt injection. This event can place
the task in review and evaluate completion consumers before the agent receives
work.

The raw detector cannot determine whether Kandev still owes the process an
initial prompt. That delivery context belongs to lifecycle and differs across
fresh stdin injection, prompt-flag launch, promptless launch, successful resume,
and fresh-start fallback.

## Decision

The lifecycle manager owns an in-memory initial-prompt marker bound to the
active passthrough process identity. Fresh launches and fresh-start fallbacks
install the marker only when a non-empty task prompt must be delivered through
PTY stdin.

Passthrough completion handling ignores every raw completion callback from the
marked process until prompt injection finishes or aborts. Prompt injection
clears only the marker whose process identity it captured. Prompt-flag,
promptless, and resumed processes do not install a marker and retain ordinary
completion behavior.

Agentctl remains a raw signal source. It does not receive task-description or
prompt-delivery policy, and it does not reinterpret the first idle globally.

## Consequences

Startup idle, prompt-pattern, and status signals cannot move a task or trigger
workflow completion before the initial prompt is submitted. Process identity
prevents delayed cleanup or callbacks from affecting a replacement process.

The marker is runtime-only and must be cleared on successful submission,
timeout, shutdown, write failure, and process replacement. Mid-turn idle
accuracy is unchanged and remains a separate concern.

## Alternatives Considered

- Suppress the first idle inside `InteractiveRunner`. The runner lacks prompt
  delivery context, and first idle is a real completion for prompt-flag agents.
  Suppressing one callback also does not cover a concurrent prompt-pattern or
  status callback before injection.
- Publish `agent.boot_ready` for the first idle. Boot-ready handling can make
  the session promptable and drain queued work while the initial prompt is
  still pending.
- Force an `agent.running` event during injection. That repairs state only
  after a false completion and retains a visible state flicker and completion
  side effects that cannot be undone.
- Increase the idle timeout. Cold-start duration is unbounded, and timeout
  tuning cannot distinguish startup readiness from turn completion.
