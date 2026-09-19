---
status: current
system: ui
created: 2026-09-16
requirements:
  - REQ-UI-EMPTY-TURN-NOTICE-001
---

# Empty-turn notice

## Purpose and boundaries

The UI system owns the empty-turn notice: the inline chat feedback shown when an
agent turn completes without producing user-visible output. This design covers
the backend `had_output` signal that the notice depends on, the frontend
decision and rendering, and the recoverable-failure exception. Turn lifecycle
and message persistence stay with the task system. The recoverable-failure
mechanism that marks a failed turn as error-terminated is owned by the
[session-recovery-failures design](../../agents/system-design/session-recovery-failures.md);
this design consumes that outcome and does not define recovery behavior.

## Requirement mapping

| Criteria under REQ-UI-EMPTY-TURN-NOTICE-001 | Design section |
| --- | --- |
| AC-UI-EMPTY-TURN-NOTICE-001.1, .2 | Backend output signal |
| AC-UI-EMPTY-TURN-NOTICE-001.3 | Backend output signal |
| AC-UI-EMPTY-TURN-NOTICE-001.4 through .8 | Frontend notice |
| AC-UI-EMPTY-TURN-NOTICE-001.9, .10 | Recoverable-failure exception |

## Backend output signal

The `session.turn.completed` event carries a transient `had_output` boolean.
`Service.turnHadOutput` (`apps/backend/internal/task/service/service_turns.go`)
computes it at completion time and `publishTurnEvent` attaches it to the
`events.TurnCompleted` payload. There is no persisted column; the notice is
live-only.

`turnHadAgentOutput` is an allowlist over the turn's own persisted messages,
read via `ListMessagesByTurnID` (indexed by `turn_id`). A message counts as
output only when it is agent-authored and is a tool call, a native plan or todo,
a permission or clarification prompt, or a non-empty text response. Incidental
per-turn messages (lifecycle `status` and `script_execution` notices, `log`,
`progress`, and `thinking`) never count. A message read failure defaults to
`true`, so a transient database error cannot produce a spurious notice. Orphan
turns swept on session resume report `had_output=true` and never trigger the
notice.

## Frontend notice

`maybeEmitEmptyTurnNotice` and `computeEmptyTurnNotice`
(`apps/web/lib/ws/handlers/empty-turn-notice.ts`) decide whether to inject the
notice. It fires only when the completed turn reports `had_output=false`, on the
main task chat surface (quick-chat and config-chat are excluded), and when no
notice already exists for the turn. The notice is a synthetic status message
keyed by the deterministic id `empty-turn-${turn_id}`, so the store merges it
once per turn.

`emptyTurnNoticeText` tailors the copy to the triggering user message.
`parseSlashCommand` strips a leading `/` and takes the first whitespace-delimited
word; `isKnownCommand` matches it case-insensitively against the agent's
advertised commands (`availableCommands.bySessionId`, whose names carry no
leading slash). A plain message yields the no-output text; an unrecognized
`/command` yields the unknown-command hint; an advertised but empty `/command`
yields the ran-but-empty hint.

The notice renders through the shared `StatusMessage` component
(`apps/web/components/task/chat/messages/status-message.tsx`) as a non-alarming
`type:"status"` warning row, so it appears identically on desktop and mobile with
no layout change. `filterVisibleMessages`
(`apps/web/hooks/processed-message-filtering.ts`) hides the notice if later output
arrives for the same turn.

## Recoverable-failure exception

A turn that ends in a recoverable agent failure is not empty: its recovery or
error entry is the turn's outcome. `handleRecoverableFailureLockedState`
(`apps/backend/internal/orchestrator/event_handlers_agent.go`) attaches the
recovery status message to the turn that failed and marks that turn with
`models.TurnMetaKeyErrorTerminated`. `turnHadOutput` returns `true` for an
error-terminated turn even though the recovery status message is not itself in
the output allowlist. Because the recovery entry attaches to the failed turn, no
second synthetic turn is opened whose only content is the recovery message, so at
most one turn completes for the failure and no empty-turn notice appears. The
sanitized provider detail is surfaced separately in the recovery entry's
`error_output` disclosure, per the
[session-recovery-failures design](../../agents/system-design/session-recovery-failures.md).

## Compatibility and verification

No protocol, persistence, or runtime flag changes: `had_output` is a transient
event field. Backend coverage asserts the output allowlist and the
error-terminated exception in the task service and orchestrator packages.
Frontend coverage in `empty-turn-notice.test.ts` asserts the three notice
variants, the once-per-turn keying, and the ephemeral-surface exclusion.
