# 0015: Explicit completion signal for auto-advance

**Status:** proposed
**Date:** 2026-06-04
**Area:** backend, frontend

## Context

Today, a workflow step's `on_turn_complete` actions (most commonly `move_to_next`, i.e. "auto-advance") fire whenever the agent's turn ends. The orchestrator's only signal that work is "done" is that the agent has stopped streaming and the ACP turn is complete. That signal is unreliable as a proxy for "user requirements satisfied":

- **Halts are overloaded.** An agent stops emitting for many reasons: it actually finished the requested work; it called `ask_user_question_kandev` (or any clarification mechanism) and is waiting on the user; it hit a provider rate limit or transient error and bailed; it terminated mid-thought due to a context budget; it shelled out and exited; a future agent CLI added a new halt class we don't know about yet. From the orchestrator's vantage point these are indistinguishable — all of them look like "turn complete."
- **The reasons evolve faster than our interpreter.** Each new agent type and each upstream CLI revision can introduce new termination modes. Treating every halt as completion guarantees false positives now and on each upgrade.
- **Auto-advance changes user-observed behavior.** Users expect the agent to behave the same whether auto-advance is on or off: ask questions and wait indefinitely, hold extended back-and-forth conversations, defer until the user replies. With auto-advance on, the very act of asking a question (which halts the turn) trips a step transition, dropping the conversation onto the next step before the human has answered.
- **Clarification waits are durable user-input barriers.** If `ask_user_question_kandev` times out, disconnects, or otherwise ends before the user answers, the workflow must stay on the same step. Running `on_turn_complete` or start-of-next-turn triggers in that state treats "awaiting user input" as "step finished" and can advance the task with missing requirements.
- **Working around it via "auto-advance off + side effect" is worse.** Disabling the checkbox and re-implementing advance as a hidden behavior reintroduces the same ambiguity at a different layer and loses the explicit UI affordance.
- **Tasks driving themselves through `update_task_kandev` / `move_task_kandev` is an alternative we rejected.** Letting an agent mutate the very task it's running on creates ordering and state-machine hazards (lifecycle races against the in-flight turn, ambiguous "which step's events fire," reconciliation against `task_repositories` and session state). Even if we hardened it, it would not address the underlying interpretation problem — we'd still be guessing what a halt meant.

## Decision

**Auto-advance is gated on an explicit, agent-emitted completion signal.** When auto-advance is enabled for a step, the orchestrator no longer treats turn-end as the trigger for `on_turn_complete` transitions. Instead, transitions fire only when the agent calls a dedicated MCP tool — `step_complete_kandev` — that semantically means "all user-stated requirements for this step are satisfied." (Named `step_*`, not `task_*`, because the signal is scoped to one workflow step; a task spans many steps and may be signaled-complete multiple times in its lifetime.) Naked halts surface a UI affordance instead of advancing silently.

### Mechanism

1. **MCP tool.** A new `step_complete_kandev` tool is registered alongside the existing kandev MCP handlers (`internal/mcp/handlers/`, exposed via the same WS-backed action plumbing as `add_branch_to_task_kandev`). Signature:

   ```jsonc
   {
     "name": "step_complete_kandev",
     "arguments": {
       "summary":  "string",         // required: one-paragraph "what was done"
       "handoff":  "string?",        // optional: context for the next step's agent
       "blockers": "string?"         // optional: known unresolved issues
     }
   }
   ```

   The tool is implemented as a small `ws.ActionMCPStepComplete`. The handler (`internal/mcp/handlers/handlers.go:handleStepComplete`) does two things in order: (a) writes a `PendingStepCompletionSignal` blob into `TaskSession.Metadata` and (b) emits a `workflow.step_completion_signaled` bus event carrying `{task_id, session_id, step_id, source, summary, signaled_at}` — `handoff` and `blockers` are persisted in the bag only, not on the wire. The MCP tool returns `{accepted: true}` once both the bag write and the publish succeed; on publish failure it returns an error so the agent can retry (the persisted bag is idempotent against the retry). The actual transition is driven by the orchestrator off the event, so the agent's call site stays decoupled from step-machine timing.

2. **Storage — `TaskSession.Metadata` bag, no new table.** The pending signal is small, short-lived, and read in exactly one place (`processOnTurnComplete`); a dedicated table would be overkill. The MCP handler (write site) and the orchestrator (read site) share a single decoder — `models.LoadPendingStepSignal` in `internal/task/models` — that handles both the in-process typed shape and the JSON-rehydrated `map[string]interface{}` shape produced by reloading the row after a restart. Schema:

   ```go
   // Persisted JSON under TaskSession.Metadata["pending_step_completion_signal"]
   type PendingStepCompletionSignal struct {
       StepID     string    `json:"step_id"`
       Source     string    `json:"source"`              // "agent" | "manual_fallback"
       Summary    string    `json:"summary"`
       Handoff    string    `json:"handoff,omitempty"`
       Blockers   string    `json:"blockers,omitempty"`
       SignaledAt time.Time `json:"signaled_at"`
   }
   ```

   - **Set** on `step_complete_kandev` MCP call or fallback button click — written into `Metadata` and saved via the existing session-update path.
   - **Read** by `processOnTurnComplete` and by the `onStepCompletionSignaled` subscriber. Stale entries whose `StepID` no longer matches the session's current step are treated as absent and cleared on read.
   - **Cleared** on (a) successful transition, (b) user message arriving before the transition runs (re-open semantics), (c) any other step change.
   - **Idempotent.** Repeated calls within the same step are no-ops: if `existing != nil && existing.StepID == currentStepID`, the MCP tool returns `{accepted: false, reason: "already_signaled"}` without overwriting.
   - **Audit (deferred).** Persisting consumed-signal context on the transition row (`SessionStepHistory.Metadata["signal_source"]` ∈ {`"agent"`, `"manual_fallback"`} plus `Metadata["signal_summary"]`) is the planned audit shape but is not yet implemented. Today the bag is cleared on transition and the only history is the chat transcript. Cancelled signals (overwritten by user reply) are deliberately not retained.
   - **Chat visibility (deferred).** Surfacing a `step_complete_signaled` system message in the existing message stream is planned but not yet emitted. Today the only user-visible cue is the next-step transition itself; the bag remains the only truth source.

3. **System-prompt injection.** When `auto_advance_requires_signal` is true on the current workflow step, `internal/sysprompt/` prepends a short instruction block to the agent's system prompt at launch / resume time, in the same path that already injects MCP tool descriptions. The text is fixed, terse, and points at the tool by name:

   > When auto-advance is enabled, you MUST call `step_complete_kandev` once — and only once — when every user request and follow-up requirement for the current workflow step is satisfied. Do not call it on partial progress, on a question you are about to ask, or after a failure. Calling it is the signal that triggers the workflow transition; halting without calling it leaves the step paused for the user.

   The instruction is unconditional — agents that never encounter a signal-gated step will simply never need to act on it (the tool no-ops without changing visible state).

4. **Orchestrator gating.** `processOnTurnComplete` (`internal/orchestrator/event_handlers_workflow.go`) splits into two paths:

   - **Step requires explicit signal** (auto-advance enabled and `auto_advance_requires_signal == true`): on bare turn-end, read `Metadata["pending_step_completion_signal"]`. If absent (or stale for a prior step), do **not** evaluate transition actions; call `setSessionWaitingForInput` and surface a "Completion pending" UI hint. If present and matches the current step, run the existing `processTurnCompleteActions` / `resolveTransitionTargetStep` / `executeStepTransition` pipeline and clear the bag entry. The same path is triggered out-of-band by a new bus subscriber `onStepCompletionSignaled` for the case where the signal lands *after* the turn already ended (signal-after-halt is the common shape).
   - **Step does not require explicit signal** (legacy behavior): unchanged.

5. **Clarification hard-pause barrier.** A pending durable clarification request blocks every `on_turn_complete` transition path, including the legacy path, the workflow-engine path, and the `workflow.step_completion_signaled` subscriber. This guard runs before explicit completion-signal evaluation, so even a previously written `step_complete_kandev` signal cannot advance while a user question is pending.

   When `ask_user_question_kandev` ends without an answer because the MCP wait timed out, the transport disconnected, or the handler context was cancelled, the MCP handler calls the orchestrator's clarification pause path. That path detaches the pending clarification for later answer handling, silently cancels the active agent turn when one exists, marks the session `WAITING_FOR_INPUT`, completes the active turn bookkeeping, and deliberately skips workflow stop triggers. When the user later answers the pending clarification, the answer resumes the current session as a continuation prompt and deliberately skips `on_turn_start` triggers; otherwise answering a question could immediately fire start-of-turn transitions for the same step.

6. **Halt-without-signal fallback.** When the agent halts without having called `step_complete_kandev`, the UI shows an inline "Mark complete & advance" action (rendered in the chat composer area, akin to the existing clarification overlay). One click writes the bag entry with `source: "manual_fallback"` and emits the same `workflow.step_completion_signaled` event. This is the safety net: false negatives (work done, agent forgot to call the tool) cost one user click; false positives (agent calls the tool prematurely) are caught because user can keep messaging and the next turn re-opens the step. The fallback is **disabled** during an active ask-clarification waiting window so the button doesn't compete with a pending user question.

7. **Re-open semantics.** If the user sends a new message *after* the signal has been written *but before* the transition has executed, the orchestrator clears the bag entry, cancels the pending transition, and treats the message as continued conversation. If the user messages *after* the transition has already moved to the next step, the message lands on the new step as normal — the prior step is not re-opened.

### Configuration surface

- **Per-step:** the existing `auto_advance` UI checkbox stays; a new sub-toggle "Wait for agent completion signal" appears underneath when an `on_turn_complete` transition is configured. Single switch; no profile/global flag.
- **Workflow step model:** add `auto_advance_requires_signal bool` to `wfmodels.WorkflowStep`. Migration adds the column with default `false`; existing steps keep legacy "any turn-end advances" behaviour until the user opts a step in.
- **Config MCP:** `create_workflow_step_kandev` and `update_workflow_step_kandev` accept an optional `auto_advance_requires_signal` boolean with the same omitted-versus-false semantics as the REST step endpoints. `list_workflow_steps_kandev` returns the field explicitly for every step so config agents can audit which steps are signal-gated.

## Consequences

- **Trades false positives for false negatives.** Today: silent wrong advances are the failure mode. New: stuck steps are the failure mode, recoverable in one click. We judge stuck > wrong: stuck is visible and fixable; wrong-advance corrupts downstream step inputs.
- **Compliance burden on the agent.** Whether the agent obeys the injected instruction is a function of (a) the model's instruction-following, and (b) the agent CLI's handling of system-prompt prepends. Both vary across Claude, Codex, Cursor, OpenCode. The fallback button is what makes this acceptable; without it, the design would brick non-Claude agents.
- **Adds a new MCP tool.** Increases the kandev MCP surface area; tool listing payload grows by one entry. The tool itself is small, but new agents and new CLIs need to be validated against it the same way ADR 0014's strategies were.
- **`step_complete_kandev` is fire-and-forget once it returns.** The MCP call only returns after the bag write + bus publish complete, but the actual transition runs asynchronously in the bus subscriber. An agent that calls the tool then immediately continues working may have trailing tokens race with the transition. Mitigation today is behavioural: the tool description in `kandev-context.md` instructs the agent to "call this as the LAST action of the step." The manual fallback button and re-open semantics are the safety nets if the agent gets it wrong.
- **Clarification timeout becomes a hard pause, not a completed turn.** A no-answer `ask_user_question_kandev` result cancels the current agent turn without running `on_turn_complete`. The user-visible state is a waiting session with the clarification still answerable; the next user answer resumes the same step instead of triggering turn-start automation.
- **Idempotency.** Multiple calls within the same step are deduped at the bag-write site (first call wins, subsequent calls return `{accepted: false, reason: "already_signaled"}` without error). Cross-step re-entry is allowed — a fresh step's `currentStepID` no longer matches the stale bag entry, so the next call writes through.
- **No new schema for the signal itself.** Storage piggybacks on the existing `task_sessions.metadata` JSON column. Audit of *consumed* signals piggybacks on the existing `session_step_history.metadata` JSON column. Cancelled signals (overwritten by a user reply) are not retained — chat already surfaces what the user needs.
- **Telemetry (deferred).** The plan is two `expvar` counters under `workflow_*` — `step_completion_signal_received_total` and `step_completion_signal_fallback_used_total` — whose ratio (fallback-used / received ≤ 10% per agent type) becomes the headline "is the explicit-signal flow working" metric. Neither counter exists yet; ship in a follow-up alongside the manual fallback button so both halves of the ratio can be measured at once.
- **No data-model coupling to `task_repositories` or `task_prs`.** The signal is per-session, per-step; it does not interact with the multi-branch work in ADR 0013.

## Alternatives considered

- **Naming: `task_complete_kandev`.** Rejected: signal is per-workflow-step, not per-task. A task may pass through `triage → in-progress → review → done`, with `step_complete_kandev` called at the end of each. `task_*` would either misname the per-step signal or imply a separate "the whole task is done" semantic we don't have.
- **Heuristic: classify turn-end reasons.** Pattern-match on the ACP stop reason, presence of recent `ask_user_question_kandev` calls, etc. Rejected: re-implements the interpretation problem we're trying to escape, and breaks on every new agent CLI revision.
- **Cancel immediately when `ask_user_question_kandev` is called.** Rejected as the default because a healthy agent connection can block inside the tool call until the user answers, preserving the normal conversational flow. We only hard-pause when the wait ends without an answer, or when the agentctl-side timeout event reports that the MCP wait failed. That gives the good path no extra turn churn while still failing closed on timeout/disconnect.
- **Dedicated `session_step_completion_signals` table.** Considered (typed columns, partial UNIQUE index for idempotency, `source` column for SQL-aggregated telemetry, full cancelled-signal audit trail). Rejected as overkill: at most one pending signal per session at a time, lifetime measured in seconds, single reader. The `TaskSession.Metadata` bag covers idempotency (existence check), audit (lifted onto the existing `SessionStepHistory.Metadata` on transition), telemetry (in-process `expvar`), and chat visibility (system message) with zero new schema.
- **Two-signal: require both halt and explicit tool call.** Effectively what we do, except we let the user provide the second signal via the fallback button when the agent doesn't. Pure "agent must do both" guarantees the stuck-task failure mode without the escape hatch.
- **Agent self-mutating via `update_task_kandev` / `move_task_kandev`.** Lets the agent move its own task between steps. Rejected up front: state-machine races (orchestrator step-transition pipeline vs in-flight turn), ambiguous event semantics (does `on_exit` for the old step fire mid-turn?), and conflicts with the existing `workflowStepGetter`-driven transition path. Even if hardened, it doesn't fix the halt-interpretation problem; the agent still has to *decide* when to call it, which is the same decision as calling `task_complete_kandev` — but on a tool that has many other side effects.
- **Disable auto-advance entirely, reintroduce as silent side-effect.** Rejected: hides a feature the user explicitly enabled via the checkbox; loses the UI affordance; doesn't reduce ambiguity, just moves it.
- **Inject the system prompt always, gate only the transition.** Considered. Mild win (signal exists everywhere; checkbox only gates whether kandev acts on it), but pollutes prompts for users who don't use auto-advance and burns a small amount of every agent's context. Deferred — easy to flip later by changing the injection predicate without schema changes.
- **Require a `summary` arg and surface it in the UI.** Kept (above) — the summary is cheap, the next-step agent benefits from a handoff blurb, and it gives the user a one-line "what changed" without scrubbing the transcript.

## Open questions

- Do we want a per-agent-type override for the system-prompt text? Codex and Cursor have noticeably different instruction-following profiles; a one-size-fits-all prompt may underperform on the weaker ones.
- Should the fallback button be exposed as a keyboard shortcut, or kept click-only to avoid accidental advances?
- Long-running passthrough sessions (ADR 0014) — verify the MCP tool is reachable via `connectMCPStream` for every passthrough-capable CLI before depending on this flow in passthrough mode.
- **Late-signal re-close race.** `ProcessOnTurnStart` clears the bag when the user replies, but the persisted signal carries only `step_id` / `signaled_at` — not a monotonic turn or user-reply marker. A `step_complete_kandev` call that was in-flight when the user replied can repopulate the bag *after* the clear, and the next turn-end will then auto-advance, violating the re-open guarantee. Today the window is narrow (the tool call has to land after the user reply but before the next turn ends) and the worst case is one wrong advance recoverable by the user dragging the task back. Proper fix is to tie each signal to either `session.LastUserReplyAt` or an active-turn ID and reject signals older than the latest user reply at the write site. Deferred — needs either a new session column or a session-scoped sequence number.

## Files this will touch (estimate)

Backend:
- `apps/backend/internal/mcp/handlers/handlers.go` *(new `step_complete_kandev`)*
- `apps/backend/internal/mcp/server/server.go` *(register tool)*
- `apps/backend/pkg/websocket/actions.go` *(new `ActionMCPStepComplete`)*
- `apps/backend/internal/task/models/models.go` *(new `PendingStepCompletionSignal` type; metadata key constant)*
- `apps/backend/internal/orchestrator/event_handlers_workflow.go` *(gating in `processOnTurnComplete`; read/clear bag entry)*
- `apps/backend/internal/orchestrator/event_handlers_step_completion.go` *(new — `onStepCompletionSignaled` subscriber: write bag + drive transition)*
- `apps/backend/config/prompts/kandev-context.md` *(unconditional tool description)*
- `apps/backend/internal/workflow/models/` *(`AutoAdvanceRequiresSignal bool`; `SessionStepHistory.Metadata` key constants for `signal_source` / `signal_summary` — deferred)*
- `apps/backend/internal/workflow/repository/sqlite/` *(idempotent migration for `workflow_steps.auto_advance_requires_signal` only — no new tables)*

Frontend:
- `apps/web/components/settings/workflow-pipeline-editor-step-actions.tsx` *(sub-toggle UI)*
- `apps/web/components/task/chat/` *(fallback "Mark complete & advance" affordance)*
- `apps/web/lib/types/http.ts` *(extend `WorkflowStep`)*
- `apps/web/lib/state/slices/` *(transitional state for "completion pending")*

Spec: `docs/specs/workflow/explicit-completion-signal/spec.md` *(to be written via `/spec` once this ADR is accepted)*
