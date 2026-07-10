---
status: shipped
created: 2026-05-30
owner: cfl
---

# Empty-Turn Notice and Slash-Command Hint

## Why

When a user sends a message to the agent chat and the turn completes with **no content and no tool calls** (a clean `end_turn`), kandev showed the user's message followed by dead air — no reply, no explanation. This is common when a user types a slash command (e.g. `/pr-fixup`) into the agent chat: the ACP agent treats it as a slash command and returns an empty turn, either because the command is unsupported or because it exists but no-ops. The user gets no feedback and assumes the app is broken.

## What

- When an agent turn completes having produced **no agent output**, the chat shows an unobtrusive inline status notice instead of nothing.
- "Output" means at least one agent message that is a tool call (`tool_call`/`tool_edit`/`tool_read`/`tool_execute`), a native plan/todo (`agent_plan`/`todo`), a permission or clarification prompt (`permission_request`/`clarification_request`), or a non-empty text response (`message`/`content`). Incidental per-turn messages — lifecycle `status`/`script_execution` notices, `log`, `progress`, and `thinking` — do **not** count as output.
- The backend is authoritative: the `session.turn.completed` event carries a transient `had_output` boolean computed from the turn's persisted messages at completion time (no DB column; the notice is live-only).
- The notice text adapts to the triggering user message:
  - **No leading `/`** → "The agent finished without producing any output."
  - **`/cmd` not in the agent's advertised commands** → "`/cmd` isn't a command this agent recognizes, so it returned no output. Try resending your request as a normal message, without the leading slash."
  - **`/cmd` is advertised but the turn was empty** → "`/cmd` ran but produced no output. Try resending your request as a normal message, without the leading slash."
- The `/command` token is matched case-insensitively against the agent's advertised commands (`availableCommands.bySessionId`), whose names carry no leading slash; the leading `/` is stripped and the first whitespace-delimited word is taken.
- The notice renders as a non-alarming `type:"status"` chat message (amber warning row via the shared `StatusMessage` renderer), so it appears on both desktop and mobile chat with no layout change.
- It fires once per turn, keyed by a deterministic message id `empty-turn-${turn_id}` (the store merges messages by id).
- It is scoped to the main task agent chat; quick-chat and config-chat surfaces are excluded.
- Orphan turns swept on session resume report `had_output=true`, so they never trigger the notice.

## Scenarios

- **GIVEN** a user sends `/pr-fixup` and the agent returns a clean empty turn, **WHEN** the turn completes and `/pr-fixup` is not an advertised command, **THEN** an inline notice says it isn't a recognized command and to resend without the leading slash.
- **GIVEN** a user sends `/commit` (an advertised command) and the turn produces no output, **WHEN** the turn completes, **THEN** the notice says the command ran but produced no output and to resend without the leading slash.
- **GIVEN** a user sends a normal prompt with no leading slash and the turn is empty, **WHEN** the turn completes, **THEN** the notice says the agent finished without producing any output.
- **GIVEN** a turn that produces a text response or a tool call, **WHEN** the turn completes, **THEN** no notice appears.
- **GIVEN** the same empty turn's `turn.completed` is processed more than once, **WHEN** the handler runs again, **THEN** only one notice exists for that turn.
- **GIVEN** an orphan turn swept by resume cleanup, **WHEN** its `turn.completed` is published, **THEN** no notice appears.
- **GIVEN** an empty turn on a quick-chat or config-chat surface, **WHEN** it completes, **THEN** no notice appears.

## Out of scope

- Persisting the notice: it is synthesized client-side from a live `turn.completed` event and is not stored, so it does not reappear on reload or in history.
- Auto-retrying the prompt without the slash, or any one-click "resend" action — the hint is advisory text only.
- Deriving the slash-command match on the backend; command cross-referencing happens entirely on the frontend where `availableCommands` lives.
- Office dashboard / live-run surfaces, which render their own run errors.

## Notes

- Backend: `had_output` is computed in `Service.CompleteTurn` via `turnHadAgentOutput` over the turn's persisted messages (`apps/backend/internal/task/service/service_turns.go`). Messages are fetched via `ListMessagesByTurnID` (indexed by `turn_id`), so the read is O(turn_messages) rather than O(session_messages). The result is added to the `turn.completed` payload in `publishTurnEvent`.
- Frontend: pure decision logic in `apps/web/lib/ws/handlers/empty-turn-notice.ts` (`computeEmptyTurnNotice`), wired from the `session.turn.completed` handler in `turns.ts`.
- E2E: the mock agent's `empty-turn` scenario (`apps/backend/cmd/mock-agent/scenarios.go`) emits a multi-second empty `end_turn`; specs in `apps/web/e2e/tests/chat/empty-turn.spec.ts` (desktop) and `mobile-empty-turn.spec.ts` seed it as the auto-started turn so the live completion is observed.
