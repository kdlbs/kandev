---
id: "02-confirm-session-mode"
title: "Confirm and attribute the applied session mode"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.5
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.6
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 02: Confirm and attribute the applied session mode

## Summary

Report a permission mode as applied only after the agent confirms it, surface a
clamped or unconfirmed mode as a session-visible warning, and record which of
the profile, the persisted session override, and the workflow step supplied the
effective mode.

## Scope

- `acp.Adapter.SetMode` returns a typed result (`requested`, `effective`,
  `confirmed`) and emits its `session_mode` event from the agent's reported
  current mode instead of the requested value. It waits a bounded settle window
  for a `current_mode_update`, reusing the convergence pattern already used by
  `emitSetModelEvent`; on expiry it emits the last known current mode marked
  unconfirmed.
- Propagate that result through the agentctl WebSocket `agent.session.set_mode`
  response and `runtime/agentctl.Client.SetMode`.
- `SessionManager.applyProfileSessionLayers` / `applyRuntimeSessionLayers` log
  the applied mode only for a confirmed exact match; a clamp or an unconfirmed
  result logs at Warn and writes a session-visible warning message.
- `Manager.effectiveSessionMode` returns the winning source
  (`agent_profile`, `session_override`, `workflow_step`) with the mode; persist
  it next to `session_mode` and include it as a structured log field.
- Raise the workflow `set_session_mode` apply failure in
  `applyStepSessionMode` from Debug to Warn and write the same session-visible
  warning.
- Frontend: the session mode selector renders the agent's reported mode, and the
  warning renders as described in UI-02.

## Exclusions

- No precedence change. `session_override` keeps winning over `agent_profile`.
- No change to the set-mode HTTP route contract beyond the added result fields.
- No change to passthrough session mode handling.

## ASCII UI preview

See [UI-02 in the plan](plan.md#ui-02-session-mode-warning-work-order-02). The
warning is a session message above the transcript; the mode selector in the
chat-input toolbar shows the reported mode. Phone composition matches, with the
warning full width and the selector in the mobile toolbar row.

## Acceptance

1. A mode the agent accepts and reports produces the applied log and no warning;
   a mode the agent clamps or does not report produces a Warn and a
   session-visible warning naming requested and effective modes.
2. The session's displayed mode equals the agent's reported current mode.
3. The effective mode carries its winning source on the session and in
   structured logs, for all three sources.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_session.go`
- `apps/backend/internal/agentctl/server/api/agent.go`
- `apps/backend/internal/agent/runtime/agentctl/agent.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_profile.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_interaction.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/web/components/task/mode-selector.tsx`
- `apps/web/src/locales/*/task.json` (six locales plus `pseudo`)

## TDD sequence

1. Add failing backend tests: a fake ACP connection that accepts
   `session/set_mode` but reports a different current mode yields
   `confirmed: false` and no applied log; a connection that reports the
   requested mode yields `confirmed: true`; `effectiveSessionMode` returns the
   correct source for each of the three inputs.
2. Add a failing frontend test that the mode selector renders the reported mode
   rather than the requested one.
3. Run the focused commands and confirm the expected failures.
4. Implement the typed result, the settle window, the source attribution, and
   the warning paths.
5. Add the locale entries in all six languages plus `pseudo`; run the i18n
   checks.
6. Re-run the focused commands; all pass.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/agentctl/server/adapter/transport/acp/... ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/... -race -count=1
cd "$(git rev-parse --show-toplevel)/apps/web" && pnpm vitest run components/task/mode-selector && pnpm run typecheck && pnpm run i18n:check
```

## Dependencies

None.

## Risks

The settle window must not add perceptible launch latency. Bound it and assert
the bound in a test rather than relying on a comment.

## Results

Done.

Implemented:
- `acp.Adapter` tracks the mode the agent reports (from `session/new`,
  `session/load` and `current_mode_update`). `SetMode` now returns a typed
  `streams.ModeResult` (`requested`, `effective`, `confirmed`) and waits a
  bounded 750 ms settle window before emitting, so a clamp published after the
  reply is observed rather than missed.
- The emitted `session_mode` event carries the agent's reported mode, and sets
  `requested_mode_id` only when the session is not in the requested mode.
- The result travels through the agentctl WS response and
  `runtime/agentctl.Client.SetMode`. An older agentctl that answers without the
  result body yields `confirmed: false`, which is correct: nothing observed it.
- `applyProfileSessionLayers` / `applyRuntimeSessionLayers` log the applied mode
  only on a confirmed exact match; a clamp or an unconfirmed result logs Warn
  with requested, effective and confirmed.
- `effectiveSessionModeWithSource` returns the winning layer
  (`agent_profile`, `session_override`, `none`) and it is logged on the
  context-reset restore path.
- The workflow `set_session_mode` apply failure moved from Debug to Warn and
  carries `mode_source: workflow_step`.
- Frontend: `requested_mode_id` flows into the session-runtime slice; the mode
  selector shows a warning glyph and a tooltip naming the requested and the
  effective mode. The selector already rendered the live reported mode, so
  showing the truth needed no change there — only the mismatch signal is new.
  New copy in all six locales plus a regenerated pseudo entry.

Verification (2026-09-22):

```
go test ./internal/agentctl/... ./internal/agent/runtime/... ./internal/orchestrator/... -count=1
golangci-lint run ./... --new-from-rev=8690df2f7
cd apps/web && npx tsc --noEmit && npx vitest run components/task lib/ws/handlers && pnpm run i18n:ratchet
```

Backend clean, lint `0 issues`, typecheck clean, frontend tests pass, ratchet
clean.

Pre-existing and untouched: `pnpm run i18n:check` reports 32 missing `ja` keys
for SSH reachability and launch warnings. Those catalogs come from
`c7cc92382` landing after the Japanese catalog in `cd08c50ca`; this work order
touches neither file, and the new key reports zero issues.
