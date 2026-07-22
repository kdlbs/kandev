# 0049: Fine-grained foreground-idle busy signal

**Status:** accepted (amended 2026-07-21)
**Date:** 2026-07-11
**Area:** backend, frontend, protocol

## Context

A session's durable state is a single scalar (`RUNNING`, `WAITING_FOR_INPUT`, …). That scalar cannot distinguish three independent lifecycle facts:

1. the foreground agent is actively generating output,
2. the foreground is idle while spawned background work is still live, or
3. the foreground turn has completed while detached background work continues.

Because both read as `RUNNING`, a session that kicked off a long background job rejected every new operator message as "agent is already running" for the entire duration of that job — locking the operator out of the conversation with no recovery but a restart.

Upstream idle-turn completion narrows but does not close this window: a synthetic turn-complete only fires after async content has been idle for a debounce interval, it never arms while the foreground prompt exchange is still in flight (a genuinely held-open subagent turn), and a chatty Monitor re-extends the debounce on every event burst. The residual lockout windows are real.

Mid-turn steering — delivering a message *into* a turn while the model is actively generating — is explicitly out of scope here; it needs ACP concurrent-prompt support and per-agent capability gating and is tracked separately.

## Decision

Track, per session and **in memory**, foreground ownership and background
liveness independently, and narrow the prompt gate (`checkSessionPromptable`)
to the foreground only:

- Foreground activity has absolute display and admission precedence. While a
  prompt is claimed, dispatched, or producing top-level output, the session is
  generating regardless of background activity. When the foreground is idle,
  outstanding recognized work produces the background-running state; only the
  absence of both produces done/idle.
- Foreground turn completion clears foreground ownership only. It does not
  clear background registrations. Each background registration remains live
  until a terminal signal for that workload arrives or the owning agent
  execution is explicitly torn down.
- A tool result that reports successful asynchronous launch is terminal for the
  launch tool card but not for the launched workload. The orchestrator retains
  the originating tool-call registration and clears workload liveness separately
  when the provider reports a
  task-notification result. Claude ACP currently exposes the notification's
  origin but not its task ID; one notification therefore retires one session
  background registration, while an ambiguous remainder stays background-live
  rather than guessing done. Prompt completion and prompt-end tool sweeps do not
  synthesize workload completion for detached shell work.
- Claude ACP usage updates expose the origin of each model cycle. Completion of
  the human-origin cycle is the explicit foreground-yield boundary even when
  the ACP prompt RPC remains held open for spawned subagents; a later
  task-notification-origin cycle may temporarily take foreground precedence
  while it generates the completion summary.

- A `RUNNING` session accepts a new prompt when its foreground turn is idle and at least one recognized background task is outstanding; otherwise it keeps rejecting input exactly as before.
- Recognition keys off the **normalized shape** of the work (subagent task / `run_in_background` shell / active Monitor), not tool-name string matching. The ACP normalizer is corrected to recognize Claude's `run_in_background:true` shell shape (a normalizer bug fix in its own right).
- Monitor recognition rests on **adapter attestation, not payload shape**. A Monitor normalizes to a Generic payload, and that payload's `Output` is assigned the agent's raw tool result verbatim — so *nothing carried inside `Output` can vouch for its own origin*, however many fields a classifier demands. The ACP adapter therefore stamps a typed `MonitorPayload` as a **sibling** of the Generic payload, on the path already gated by ACP `_meta.claudeCode.toolName` (metadata the claude-agent-acp wrapper sets, which model tool output cannot reach). `IsActiveMonitor` classifies on that attestation alone. `Output` keeps carrying the view the frontend card renders — it is a presentation contract, not a trust one. (The payload's `Name` is likewise unusable as a discriminator: it carries the ACP tool *kind*, which is `"other"` for Monitor.)
- The default is **busy**: any agent whose in-flight frames are not recognized as background work (Codex, OpenCode, and any future agent) preserves today's exact reject-while-`RUNNING` contract. The narrowing is capability-gated, not assumed — and, per the point above, an unrecognized agent cannot relax its own gate by shaping its tool output.
- Admission is **check-and-claim, not check-then-act**. The gate is a pure read, so `PromptTask` follows a passing read with an atomic `claimForegroundTurn` before it drives the turn: the check and the flip back to foreground-generating happen under one lock. Without this, two prompts landing in the background-idle window together (a double-send, two tabs) would both pass the read — the window spans a session reload, `ensureSessionRunning`, and a possibly network-bound model switch — and both reach `executor.Prompt`, starting overlapping turns on one ACP session. Exactly one prompt wins; the losers are rejected with `ErrAgentPromptInProgress` just as they were before this ADR.
- The claim is **held, not merely taken**, and is tracked independently of background-idle activity. It survives until agentctl accepts the prompt, so a background tool call landing while the prompt is in preflight or queued cannot reopen the gate underneath it. The lifecycle adapter reports that exact dispatch boundary before waiting for turn completion. Claim tokens bind the activity record and a monotonically increasing admission generation, so a delayed completion or release cannot mutate a newer claim for the same session. A failed pre-dispatch prompt reopens the gate only if no newer foreground output invalidated its captured foreground epoch. Every transition that changes the substate is broadcast, including a release and background work that becomes visible when a claim completes, so clients cannot remain stranded on the admission-time value.
- Lifecycle prompt delivery is serialized per agent execution. Agentctl acknowledges transport dispatch before the adapter's prompt RPC completes, so adapter-level queuing alone does not protect lifecycle's shared completion channel and response buffers from concurrent callers. An execution-scoped mutex gives each prompt one waiter and one buffer set; dispatch-only sends leave a pending-completion barrier that the next send must consume before resetting those buffers.
- Any top-level non-background tool activity marks the foreground as generating again, just like message and thinking frames. This closes the gate as soon as foreground execution resumes even when the next frame is a tool call rather than text.

The distinction is surfaced to the operator as a fine-grained substate (`foreground_activity`: `generating` vs `background`) so the UI can communicate two independent facts — "you may type" *and* "work is still in progress" — instead of collapsing them into one busy/done bit:

- The composer gates on foreground-generating rather than coarse `RUNNING`.
- A tri-state status indicator distinguishes generating / working-in-background / done; the established "running" affordance is unchanged and a distinct indicator is *added* for the background-idle substate — it never reads as "done" while background work runs.
- The substate is delivered live over a `session.activity_changed` WS event and
  carried on `session.state_changed`. It is also read from the in-memory tracker
  into the boot payload and the session REST/WS DTOs. Generating is meaningful
  for a `RUNNING` session; background may also be carried after the coarse state
  settles when detached work remains live.

## Consequences

The operator is no longer falsely locked out while background work runs, and the UI truthfully shows "working in the background" as distinct from both "generating" and "done". Accepting input earlier does not make delivery earlier for a held-open subagent turn — that message still waits behind the open exchange, as the queue does today — but out-of-turn work (a Monitor burst, a backgrounded shell after the main turn yielded) is forwarded promptly with no false "already running" rejection.

The signal is best-effort across a backend or agent-execution restart. Connected
executions retain background liveness across foreground turns, but detached
work that survives a restart cannot be reconstructed without an agent-side
liveness API. Recognition is deliberately Claude-shaped today (subagent task,
`run_in_background` shell, active Monitor are Claude features); other agents
keep today's behavior.

## Alternatives Considered

- **Rely on upstream synthetic idle-turn completion alone.** Rejected: it structurally cannot arm while the foreground prompt exchange is open, and its debounce re-extends under event bursts, so the held-open and chained-burst windows remain. A falsifiable acceptance test drives a chatty Monitor and confirms a prompt sent during a burst is accepted only with the fine-grained gate.
- **Drop the `RUNNING` gate / always accept input.** Rejected: it would let a new message race a genuinely-generating foreground turn, risking dropped or reordered messages and regressing non-steering agents.
- **Persist the foreground/background distinction to the database (like the coarse states).** Rejected: persistence without an agent-side reconciliation API becomes a second source of truth and can survive restart as a false live value after the workload has died. Connected-execution tracking is kept in memory and read at serialization boundaries; turn close no longer destroys it, while execution teardown does.
- **Recognize background work by tool-name string matching.** Rejected as brittle across agents and updates; recognition keys on the normalized payload shape so producer and consumer share one contract.
