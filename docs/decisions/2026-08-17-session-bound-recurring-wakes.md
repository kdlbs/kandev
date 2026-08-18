# ADR-2026-08-17-session-bound-recurring-wakes: Session-bound recurring wakes

**Status:** accepted
**Date:** 2026-08-17
**Area:** backend, protocol

## Context

Coordinating agents need a durable way to resume their current session on a
cadence. Existing Automations create a new task per firing, while peer message
delivery may interrupt an active turn; neither is a safe same-session wake.

## Decision

The backend owns durable wake schedules keyed by `(task_id, session_id,
marker)`. MCP servers inject their authenticated task and session identities,
so callers cannot target an arbitrary session. The shared in-process cron loop
claims and advances a due schedule before delivery, then queues its exact
prompt using a server-owned coalesce key. A busy session is never interrupted;
repeated fires replace its one pending wake message.

## Consequences

Schedules survive process restarts and remain inspectable after expiry. A
firing cannot create a task, automation run, Office run, or child session.
The backend must retain delivery state and terminal errors rather than relying
on a host cron daemon.

## Alternatives Considered

Automations were rejected because their established contract creates a task per
run. Host crontab was rejected because it cannot own session lifecycle,
database claims, or portable deployment behavior. Interrupting peer messages
was rejected because it can cancel current work.
