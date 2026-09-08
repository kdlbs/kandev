---
status: active
system: agents
created: 2026-07-29
updated: 2026-08-27
owners:
  - Kandev
---
# Agent Stall Recovery Requirements

## Overview

An agent turn can remain `RUNNING` after it stops emitting events. Silence alone is ambiguous because a legitimate long-running tool may also be quiet, but some agents know that their provider stream has failed and fail to close the ACP prompt. Users need Kandev to distinguish trustworthy terminal evidence from mere inactivity so a failed turn is not presented as healthy work.

## Requirements

### REQ-AGENTS-AGENT-STALL-RECOVERY-001: Agent Stall Recovery

**Intent:** An agent turn can remain `RUNNING` after it stops emitting events. Silence alone is ambiguous because a legitimate long-running tool may also be quiet, but some agents know that their provider stream has failed and fail to close the ACP prompt. Users need Kandev to distinguish trustworthy terminal evidence from mere inactivity so a failed turn is not presented as healthy work.

#### Acceptance criteria

- **AC-AGENTS-AGENT-STALL-RECOVERY-001.1:** Kandev checks running prompts for inactivity once per 60 seconds.
- **AC-AGENTS-AGENT-STALL-RECOVERY-001.2:** After five minutes without agent events, Kandev creates at most one user-visible notice for that prompt generation.
- **AC-AGENTS-AGENT-STALL-RECOVERY-001.3:** The notice says Kandev is still waiting and includes the active top-level tool's display title or name when available. It does not assert that the tool failed and does not include raw command arguments.
- **AC-AGENTS-AGENT-STALL-RECOVERY-001.4:** The notice is a single compact inline row with muted neutral copy and a neutral **Cancel turn** action. It has no warning/error colors, alert icon, tinted background, or alert-card treatment.
- **AC-AGENTS-AGENT-STALL-RECOVERY-001.5:** On phones, **Cancel turn** remains inline and content-width rather than becoming a full-width row, while retaining a minimum 44px touch height. Activating it uses the existing `agent.cancel` request.
- **AC-AGENTS-AGENT-STALL-RECOVERY-001.6:** The notice remains visible and actionable while the affected prompt's `turn_id` is the active turn in a `RUNNING` session, including after a page reload. It is hidden when that turn settles or a later turn becomes active.
- **AC-AGENTS-AGENT-STALL-RECOVERY-001.7:** Detection after genuine agent activity does not change task state, session state, prompt admission, or process liveness. A current five-minute snapshot with no genuine event since prompt dispatch is a launch failure and moves the session and task to `FAILED`.
- **AC-AGENTS-AGENT-STALL-RECOVERY-001.8:** The backend logs the first stall detected for a prompt generation and does not emit another notice or log entry on every watchdog check.

## Migrated source detail

Decisions:

- [ADR-2026-07-29-agent-stall-user-controlled-recovery](../../../decisions/2026-07-29-agent-stall-user-controlled-recovery.md)
- [ADR-2026-08-02-agent-terminal-diagnostics-over-stderr](../../../decisions/2026-08-02-agent-terminal-diagnostics-over-stderr.md)
- [ADR-2026-08-07-allowlisted-provider-action-links](../../../decisions/2026-08-07-allowlisted-provider-action-links.md)
- [ADR-2026-08-08-provider-neutral-agent-error-recovery](../../../decisions/2026-08-08-provider-neutral-agent-error-recovery.md)
- [ADR-2026-08-18-never-started-agent-stall-terminal](../../../decisions/2026-08-18-never-started-agent-stall-terminal.md)

Implementation plans:

- [Agent stall recovery](../../../plans/agent-stall-recovery/plan.md)
- [Lost turn-completion cancel recovery](../../../plans/lost-turn-completion-cancel-recovery/plan.md)
- [OpenCode terminal error surfacing](../../../plans/opencode-terminal-error-surfacing/plan.md)
- [OpenCode actionable error links](../../../plans/opencode-actionable-error-links/plan.md)

## Why

An agent turn can remain `RUNNING` after it stops emitting events. Silence alone
is ambiguous because a legitimate long-running tool may also be quiet, but some
agents know that their provider stream has failed and fail to close the ACP
prompt. Users need Kandev to distinguish trustworthy terminal evidence from
mere inactivity so a failed turn is not presented as healthy work.

## What

### Advisory inactivity recovery

- Kandev checks running prompts for inactivity once per 60 seconds.
- After five minutes without agent events, Kandev creates at most one
  user-visible notice for that prompt generation.
- The notice says Kandev is still waiting and includes the active top-level
  tool's display title or name when available. It does not assert that the tool
  failed and does not include raw command arguments.
- The notice is a single compact inline row with muted neutral copy and a
  neutral **Cancel turn** action. It has no warning/error colors, alert icon,
  tinted background, or alert-card treatment.
- On phones, **Cancel turn** remains inline and content-width rather than
  becoming a full-width row, while retaining a minimum 44px touch height.
  Activating it uses the existing `agent.cancel` request.
- The notice remains visible and actionable while the affected prompt's
  `turn_id` is the active turn in a `RUNNING` session, including after a page
  reload. It is hidden when that turn settles or a later turn becomes active.
- Detection after genuine agent activity does not change task state, session
  state, prompt admission, or process liveness. A current five-minute snapshot
  with no genuine event since prompt dispatch is a launch failure and moves the
  session and task to `FAILED`.
- The backend logs the first stall detected for a prompt generation and does
  not emit another notice or log entry on every watchdog check.

### Explicit completion signal recovery

- An accepted `step_complete_kandev` signal is stronger evidence than
  inactivity alone. It records that the agent requested the current step
  transition.
- If the signal is at least ten minutes old and the session is still
  `RUNNING` or `STARTING`, Kandev checks current prompt activity before it
  reclaims the session. It excludes Office and passthrough sessions.
- The watchdog cancels the captured execution, closes only the captured
  turn, changes the session to `WAITING_FOR_INPUT`, and uses the normal
  signal reconciler to apply the workflow transition.
- The watchdog validates the execution ID, prompt generation, and activity
  epoch that it captured. It does not cancel a replacement execution or
  close a successor turn.
- If cancellation finds no execution, Kandev may continue because no live
  process remains. Other lifecycle or storage errors stop the reclaim and
  leave the durable signal for a later retry.
- A failed reconciliation does not lose the signal. Later watchdog ticks
  retry a `WAITING_FOR_INPUT` session while the matching signal remains.

This is the only automatic state-changing exception to the advisory inactivity
rule. Silence without an accepted explicit completion signal remains advisory.

### Correlated terminal diagnostics

- When an agent emits a terminal provider diagnostic outside ACP, Kandev acts
  on it only when the diagnostic identifies the current agent type, active
  provider session, and active foreground prompt.
- OpenCode provider diagnostics are collected from the managed `opencode acp`
  process's error-only stderr stream. Kandev does not read OpenCode's private
  files under `~/.local/share/opencode/log`.
- A correlated OpenCode `stream error` ends the affected prompt through the
  existing agent-error lifecycle instead of waiting for the inactivity
  threshold. The session stops presenting itself as `RUNNING` and retains the
  existing recovery actions.
- Kandev preserves only allowlisted diagnostic fields needed for recovery:
  source agent, provider, model, sanitized message, occurrence time, and a
  reset time when one can be derived. Raw log lines, workspace URLs, account or
  workspace identifiers, paths, credentials, and unrelated stderr are not
  persisted or sent to the browser.
- A recognized OpenCode usage-limit message is classified as
  `quota_limited`. Its recovery surface names the affected model when known,
  explains when capacity resets, and keeps sanitized technical details
  collapsed by default.
- Kanban interactive recovery treats `quota_limited` as user-actionable. A reset
  time is informative and never schedules a Kanban retry. Office may consume the
  same classification for configured provider fallback and durable scheduler
  recovery under its separate routing policy.
- The user-facing provider-limit copy is localized. Desktop and phone layouts
  expose the same failure explanation, technical details, and recovery actions.
- A diagnostic for a background/title request, another OpenCode session, an
  earlier prompt generation, or a prompt that has already settled cannot fail
  the active turn.
- An unrecognized, malformed, or unavailable diagnostic stream does not become
  proof of failure. The prompt continues and the advisory inactivity recovery
  remains available.

### Allowlisted actionable diagnostics

- A validated OpenCode remediation URL may cross the agentctl boundary as a
  separate `remediation_url` field. It is not appended to the sanitized
  provider message and is never sourced from an arbitrary raw error string.
- The only accepted URL shape is HTTPS on the exact `opencode.ai` host with a
  path of `/workspace/<safe-workspace-id>/go`, with no userinfo, query, or
  fragment. The workspace identifier and complete URL are bounded before they
  are retained.
- Kandev extracts this field from a structured ACP error field when OpenCode
  supplies one, or from a correlated structured OpenCode stderr record. If the
  source does not carry the URL, Kandev keeps the existing short error and does
  not attempt to reconstruct it from private OpenCode files.
- Kanban recovery cards, the persisted last-agent-error notice, and Office's
  failed-session entry expose the same safe destination as a localized,
  keyboard- and touch-accessible external link. The sanitized message and
  collapsed technical details remain unchanged.
- A missing, malformed, untrusted, or unsupported URL produces no link and no
  URL-bearing fallback text. Existing recovery actions and generic error copy
  remain available.

## Diagnostic contract

The internal normalized provider diagnostic contains only:

| Field | Meaning |
| --- | --- |
| `source` | Stable diagnostic source identifier, initially `opencode_stderr` |
| `provider_id` | Provider identifier reported by the agent, when present |
| `model_id` | Model identifier reported by the agent, when present |
| `message` | Sanitized provider message with URLs and identifiers removed |
| `occurred_at` | Parsed diagnostic timestamp |
| `reset_at` | Derived by adding a relative reset duration to `occurred_at`; absolute reset timestamps are not accepted in this version |
| `remediation_url` | Optional, exact-host OpenCode action URL accepted by the adapter-specific validator |

The persisted recovery-message metadata uses
`failure_kind: provider_quota_limited`, plus the safe model and reset fields,
to select localized presentation. `error_output` may contain the same bounded,
sanitized diagnostic message for the collapsed technical-details surface.

## Failure modes

- If active tool identity is unavailable, the generic inactivity notice and
  cancel action still appear.
- If the agent acknowledges cancellation, normal cancellation reconciliation
  makes the session input-ready.
- If the agent does not acknowledge cancellation, the existing bounded
  cancel-escalation path releases the prompt and reconciles the session.
- Cancel escalation clears every prompt-admission barrier for the cancelled
  turn, including a pending dispatch-only completion. A later prompt can
  dispatch without a backend restart.
- If notice-message persistence fails, the failure is logged without changing
  or terminating the running session.
- If OpenCode does not support error-only stderr emission, changes its log
  format, or emits a line that fails validation, Kandev ignores the line and
  retains normal ACP plus inactivity behavior.
- If a validated terminal diagnostic races with an ACP prompt response, only
  one terminal event settles the prompt generation.
- If sanitization removes all usable provider text, Kandev surfaces a generic
  OpenCode provider failure without exposing the raw diagnostic.
- If an explicit completion signal remains after a reclaim, the session stays
  input-ready and the watchdog retries the normal reconciliation path on a
  later tick.

## Persistence guarantees

- Advisory stall notices retain their existing persisted-message behavior.
- Sanitized provider failures use the existing session error and recovery
  message persistence. They survive reloads like other recoverable failures.
- A validated `remediation_url` is persisted only in the structured recovery
  metadata and remains available after reloads and through the Office
  `session.state_changed` snapshot. It is omitted from generic error text and
  raw stderr.
- Raw agent stderr remains a bounded, executor-local in-memory diagnostic. It
  is not added to session metadata, messages, debug exports, or durable logs by
  this feature.

## Scenarios

- **GIVEN** a running prompt whose top-level shell tool is `in_progress`,
  **WHEN** no agent event arrives for five minutes, **THEN** one compact neutral
  notice names that tool and offers **Cancel turn** while the session remains
  `RUNNING`.
- **GIVEN** a running prompt with no known active tool, **WHEN** no agent event
  arrives for five minutes, **THEN** one generic notice offers **Cancel turn**
  while the session remains `RUNNING`.
- **GIVEN** a notice already exists for the active prompt generation,
  **WHEN** subsequent watchdog checks observe the same stall, **THEN** Kandev
  creates no duplicate notice and emits no repeated stall log.
- **GIVEN** a persisted notice belongs to an earlier prompt generation,
  **WHEN** a later prompt is `RUNNING`, **THEN** the earlier notice is not
  rendered and a delayed stall event cannot create a new notice for it.
- **GIVEN** a stall notice is visible, **WHEN** the user activates
  **Cancel turn**, **THEN** the existing cancellation path settles the turn and
  the session becomes available for new input without a backend restart.
- **GIVEN** a dispatch-only prompt never reports completion and cancellation
  escalates, **WHEN** the user sends a later prompt, **THEN** Kandev dispatches
  it without a backend restart.
- **GIVEN** a stall notice is visible on a phone viewport, **WHEN** the user taps
  **Cancel turn**, **THEN** the same cancellation outcome is reachable through
  an inline, content-width touch target of at least 44px.
- **GIVEN** a prompt has produced no genuine agent event since dispatch,
  **WHEN** the current five-minute watchdog snapshot is handled, **THEN** Kandev
  persists a terminal error, moves the session and task to `FAILED`, and does
  not render a running-only cancel action.
- **GIVEN** a quiet but legitimate long-running turn, **WHEN** the inactivity
  threshold passes and the user does not cancel, **THEN** Kandev leaves the
  turn and process running.
- **GIVEN** a current-step completion signal is at least ten minutes old and
  the tracked prompt has also been inactive for ten minutes, **WHEN** the
  session is still `RUNNING` or `STARTING`, **THEN** the watchdog settles the
  captured turn, moves the session to `WAITING_FOR_INPUT`, and applies the
  signal through the normal workflow engine path.
- **GIVEN** the watchdog moves a signalled session to `WAITING_FOR_INPUT` but
  a storage or workflow lookup fails, **WHEN** a later tick runs and the
  signal is still pending, **THEN** Kandev retries reconciliation and does not
  create a second turn completion.
- **GIVEN** a new execution or turn appears while the watchdog is cancelling
  the captured one, **WHEN** the watchdog resumes, **THEN** it does not cancel
  or close the successor and leaves the signal pending for a later retry.
- **GIVEN** the active OpenCode prompt emits a `stream error` for its own ACP
  session, **WHEN** the diagnostic says the model's five-hour usage limit was
  reached, **THEN** Kandev ends the running state and shows a localized
  quota-limit recovery message with the model and reset time.
- **GIVEN** that quota-limit recovery is shown, **WHEN** the user expands
  technical details, **THEN** the sanitized provider message is visible and no
  OpenCode workspace URL or identifier is present.
- **GIVEN** OpenCode emits a title-generation error or an error for another
  session, **WHEN** a foreground prompt is active, **THEN** Kandev ignores that
  diagnostic and does not settle the foreground prompt.
- **GIVEN** a correlated diagnostic and the ACP response arrive concurrently,
  **WHEN** the turn settles, **THEN** exactly one terminal outcome is emitted
  for that prompt generation.
- **GIVEN** an OpenCode build without usable diagnostic stderr, **WHEN** its ACP
  prompt hangs without further events, **THEN** the five-minute advisory notice
  remains the recovery path.
- **GIVEN** a correlated OpenCode diagnostic containing a valid remediation URL,
  **WHEN** the recovery surface settles, **THEN** Kanban and Office show a
  localized external link while the sanitized message contains no URL or
  workspace identifier.
- **GIVEN** a URL with the wrong host, scheme, path, query, fragment, or
  malformed workspace identifier, **WHEN** it crosses the adapter boundary,
  **THEN** Kandev drops the URL and shows the existing generic recovery copy.
- **GIVEN** OpenCode 1.18.5 returns only its current short ACP failure,
  **WHEN** Kandev handles the error, **THEN** it shows the short failure without
  pretending that the TUI-only URL was received.
- **GIVEN** the provider-limit recovery renders on a phone viewport, **WHEN**
  the user reads details or selects a recovery action, **THEN** the same outcome
  is available through touch targets of at least 44px with no horizontal page
  overflow.

## Out of scope

- Automatically timing out, cancelling, or killing a turn based only on
  inactivity.
- Making the inactivity threshold user-configurable.
- Reading, tailing, or exposing an agent vendor's private log files.
- Treating arbitrary stderr text as trusted terminal evidence.
- Persisting raw stderr or raw OpenCode log records.
- Following or exposing arbitrary provider URLs, account URLs, or balance URLs;
  only the exact allowlisted OpenCode remediation route is in scope.
- Recovering a URL that was never present in ACP or managed structured stderr,
  including by reading or tailing OpenCode's private log files.
- Automatically purchasing capacity, changing models, or scheduling a retry at
  the reset time.
- Repairing OpenCode's ACP implementation upstream; OpenCode should still be
  encouraged to return the provider failure directly from `session/prompt`.
