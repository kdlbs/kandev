# 0049: Fine-grained foreground-idle busy signal

**Status:** accepted
**Date:** 2026-07-11
**Area:** backend, frontend, protocol

## Context

A session's durable state is a single scalar (`RUNNING`, `WAITING_FOR_INPUT`, …). While a turn is `RUNNING`, that scalar cannot distinguish two very different situations:

1. the foreground agent is actively generating output, versus
2. the foreground turn is idle and only held open by spawned background work — a subagent `Task`, a `run_in_background` shell, or an active Claude `Monitor` watch.

Because both read as `RUNNING`, a session that kicked off a long background job rejected every new operator message as "agent is already running" for the entire duration of that job — locking the operator out of the conversation with no recovery but a restart.

Upstream idle-turn completion narrows but does not close this window: a synthetic turn-complete only fires after async content has been idle for a debounce interval, it never arms while the foreground prompt exchange is still in flight (a genuinely held-open subagent turn), and a chatty Monitor re-extends the debounce on every event burst. The residual lockout windows are real.

Mid-turn steering — delivering a message *into* a turn while the model is actively generating — is explicitly out of scope here; it needs ACP concurrent-prompt support and per-agent capability gating and is tracked separately.

## Decision

Track, per session and **in memory**, whether the open turn is foreground-driven or has yielded to outstanding background work, and narrow the prompt gate (`checkSessionPromptable`) to the foreground only:

- A `RUNNING` session accepts a new prompt when its foreground turn is idle and at least one recognized background task is outstanding; otherwise it keeps rejecting input exactly as before.
- Recognition keys off the **normalized shape** of the work (subagent task / `run_in_background` shell / active Monitor), not tool-name string matching. The ACP normalizer is corrected to recognize Claude's `run_in_background:true` shell shape (a normalizer bug fix in its own right).
- Monitor recognition rests on **adapter attestation, not payload shape**. A Monitor normalizes to a Generic payload, and that payload's `Output` is assigned the agent's raw tool result verbatim — so *nothing carried inside `Output` can vouch for its own origin*, however many fields a classifier demands. The ACP adapter therefore stamps a typed `MonitorPayload` as a **sibling** of the Generic payload, on the path already gated by ACP `_meta.claudeCode.toolName` (metadata the claude-agent-acp wrapper sets, which model tool output cannot reach). `IsActiveMonitor` classifies on that attestation alone. `Output` keeps carrying the view the frontend card renders — it is a presentation contract, not a trust one. (The payload's `Name` is likewise unusable as a discriminator: it carries the ACP tool *kind*, which is `"other"` for Monitor.)
- The default is **busy**: any agent whose in-flight frames are not recognized as background work (Codex, OpenCode, and any future agent) preserves today's exact reject-while-`RUNNING` contract. The narrowing is capability-gated, not assumed — and, per the point above, an unrecognized agent cannot relax its own gate by shaping its tool output.
- Admission is **check-and-claim, not check-then-act**. The gate is a pure read, so `PromptTask` follows a passing read with an atomic `claimForegroundTurn` before it drives the turn: the check and the flip back to foreground-generating happen under one lock. Without this, two prompts landing in the background-idle window together (a double-send, two tabs) would both pass the read — the window spans a session reload, `ensureSessionRunning`, and a possibly network-bound model switch — and both reach `executor.Prompt`, starting overlapping turns on one ACP session. Exactly one prompt wins; the losers are rejected with `ErrAgentPromptInProgress` just as they were before this ADR.
- The claim is **held, not merely taken**, and is tracked independently of background-idle activity. It survives until the prompt is dispatched to the agent, so a background tool call landing mid-admission cannot reopen the gate underneath an in-flight prompt. It is released if the prompt fails before reaching the agent — but only when the claim is still current: if the agent's foreground began generating for real during preflight, releasing would hand a live turn back to "background-idle" and let a second prompt overlap it, so a stale claim declines to reopen the gate. The window must close *before* `executor.Prompt`, which blocks for the whole turn; a claim held across it would keep the session un-promptable for the entire turn and defeat the feature outright. Every transition that changes the substate is broadcast, including a release, so a client that loaded the page mid-admission is not stranded with a stale composer.

The distinction is surfaced to the operator as a fine-grained substate (`foreground_activity`: `generating` vs `background`) so the UI can communicate two independent facts — "you may type" *and* "work is still in progress" — instead of collapsing them into one busy/done bit:

- The composer gates on foreground-generating rather than coarse `RUNNING`.
- A tri-state status indicator distinguishes generating / working-in-background / done; the established "running" affordance is unchanged and a distinct indicator is *added* for the background-idle substate — it never reads as "done" while background work runs.
- The substate is delivered live over a `session.activity_changed` WS event (and carried on `session.state_changed` to reset on every coarse transition), and is also read from the in-memory tracker into the boot payload and the session REST/WS DTOs (`RUNNING`-only) so a fresh page-load or a second tab is immediately correct rather than showing the coarse busy signal until the next flip.

## Consequences

The operator is no longer falsely locked out while background work runs, and the UI truthfully shows "working in the background" as distinct from both "generating" and "done". Accepting input earlier does not make delivery earlier for a held-open subagent turn — that message still waits behind the open exchange, as the queue does today — but out-of-turn work (a Monitor burst, a backgrounded shell after the main turn yielded) is forwarded promptly with no false "already running" rejection.

The signal is best-effort across a backend restart: a restart ends the turn, so the in-memory tracker resets to the safe "generating" default and no stale "you may type" is ever surfaced. Correctness is never traded — only the optimization. Recognition is deliberately Claude-shaped today (subagent task, `run_in_background` shell, active Monitor are Claude features); other agents keep today's behavior.

## Alternatives Considered

- **Rely on upstream synthetic idle-turn completion alone.** Rejected: it structurally cannot arm while the foreground prompt exchange is open, and its debounce re-extends under event bursts, so the held-open and chained-burst windows remain. A falsifiable acceptance test drives a chatty Monitor and confirms a prompt sent during a burst is accepted only with the fine-grained gate.
- **Drop the `RUNNING` gate / always accept input.** Rejected: it would let a new message race a genuinely-generating foreground turn, risking dropped or reordered messages and regressing non-steering agents.
- **Persist the foreground/background distinction to the database (like the coarse states).** Rejected: it only matters for a live in-flight turn, so a persisted value becomes a second source of truth that can drift from the gate, survives a restart as a false "you may type" for a session whose turn is gone (needing extra reconciliation to stay safe), and adds write churn to the hot streaming path. In-memory tracking that resets to the safe default on every turn-close is sufficient and simpler; the same in-memory value is read at the serialization boundary to keep fresh page-loads correct without persistence.
- **Recognize background work by tool-name string matching.** Rejected as brittle across agents and updates; recognition keys on the normalized payload shape so producer and consumer share one contract.
