# 0049: Fine-grained foreground-idle busy signal

**Status:** superseded by 2026-07-28-coarse-running-busy-signal
**Date:** 2026-07-11
**Area:** backend, frontend, protocol
**Related:** [Background work liveness spec](../specs/platform/requirements/background-work-liveness.md)

## Context

A session's durable state is a single scalar (`RUNNING`, `WAITING_FOR_INPUT`, …). That scalar cannot distinguish three independent lifecycle facts:

1. the foreground agent is actively generating output,
2. the foreground is idle while spawned background work is still live, or
3. the foreground turn has completed while detached background work continues.

Because both read as `RUNNING`, a session that kicked off a long background job rejected every new operator message as "agent is already running" for the entire duration of that job — locking the operator out of the conversation with no recovery but a restart.

Upstream idle-turn completion narrows but does not close this window: a synthetic turn-complete only fires after async content has been idle for a debounce interval, it never arms while the foreground prompt exchange is still in flight (a genuinely held-open subagent turn), and a chatty Monitor re-extends the debounce on every event burst. The residual lockout windows are real.

Mid-turn steering — delivering a message *into* a turn while the model is actively generating — is explicitly out of scope here; it needs ACP concurrent-prompt support and per-agent capability gating and is tracked separately. **(Superseded for the steer-capable case by [ADR-2026-08-04-mid-turn-steering](2026-08-04-mid-turn-steering.md): the concurrent-prompt precondition is satisfied by the shipped Claude bridge, and the negotiated capability gate is implemented there.)**

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
- A generation-bearing human-origin idle boundary may also attest transport
  handoff, but only for adapters whose provider bridge is fixture-tested to
  accept the next human prompt echo while the earlier RPC remains open. The
  adapter transfers its existing prompt-gate token directly to one human
  successor; it does not release the token into the general queue. Synthetic
  `ScheduleWakeup` prompts therefore remain serialized and cannot consume or
  misalign the human handoff.

- A `RUNNING` session accepts a new prompt when its foreground turn is idle and at least one recognized background task is outstanding; otherwise it keeps rejecting input exactly as before.
- Recognition rests on **adapter attestation, not normalized presentation
  shape**. A normalized subagent, shell, or Generic card describes what the UI
  renders; it does not prove that Kandev understands the provider's complete
  workload lifecycle. The adapter stamps typed background-work provenance only
  on a path whose launch, foreground-yield, and accountable terminal frames are
  covered by protocol fixtures. Agent-shaped input or output is never allowed
  to stamp that provenance.
- Initial capability support is deliberately narrow: Claude subagents,
  `run_in_background` shells, and Monitor watches retain their tested support;
  Codex, OpenCode, Cursor, Auggie, Copilot, Amp, generic ACP, and future agents
  remain foreground-busy until their full lifecycle is covered. In particular,
  Codex does not opt in until accountable terminal child activity is reliably
  emitted and fixture-tested. This is a typed adapter capability, not a central
  string whitelist of agent names.
- The mock provider is a dev/E2E-only exception that replays captured Claude
  lifecycle metadata through this trusted path; it does not expand production
  agent support.
- Codex child activity may use a different ACP tool-call ID from the original
  launch. Its child thread/session ID correlates launch and activity cards for
  presentation, but this correlation does not attest background liveness.
  Observed Codex ACP streams can omit accountable terminal child activity, so
  neither launch nor collaboration control cards create background
  registrations.
- Monitor remains the motivating trust example. It normalizes to a Generic
  payload whose `Output` is raw agent data, so the adapter-issued typed
  provenance remains a sibling of presentation data. The same trust rule now
  governs every background-work kind rather than Monitor alone.
- The default is **busy**: any agent whose in-flight frames are not attested as
  recognized background work preserves the reject-while-`RUNNING` contract. An
  unrecognized agent cannot relax its own gate by shaping a tool card or result.
- The live registry stores workload kind and provider-owned child identity per
  registration. `active_subagent_count` is derived by counting live subagent
  registrations, never maintained as an independent mutable counter. Session
  DTOs and activity events expose that count; task DTOs sum it across sessions.
  Shells and Monitor watches affect liveness but not the subagent count. This
  preserves identities for a future subagent list without committing the
  current UI to one.
- Admission is **check-and-claim, not check-then-act**. The gate is a pure read, so `PromptTask` follows a passing read with an atomic `claimForegroundTurn` before it drives the turn: the check and the flip back to foreground-generating happen under one lock. Without this, two prompts landing in the background-idle window together (a double-send, two tabs) would both pass the read — the window spans a session reload, `ensureSessionRunning`, and a possibly network-bound model switch — and both reach `executor.Prompt`, starting overlapping turns on one ACP session. Exactly one prompt wins; the losers are rejected with `ErrAgentPromptInProgress` just as they were before this ADR.
- The claim is **held, not merely taken**, and is tracked independently of background-idle activity. It survives until agentctl accepts the prompt, so a background tool call landing while the prompt is in preflight or queued cannot reopen the gate underneath it. The lifecycle adapter reports that exact dispatch boundary before waiting for turn completion. Claim tokens bind the activity record and a monotonically increasing admission generation, so a delayed completion or release cannot mutate a newer claim for the same session. A failed pre-dispatch prompt reopens the gate only if no newer foreground output invalidated its captured foreground epoch. Every transition that changes the substate is broadcast, including a release and background work that becomes visible when a claim completes, so clients cannot remain stranded on the admission-time value.
- Lifecycle prompt delivery is serialized per agent execution. Agentctl
  acknowledges transport dispatch before the adapter's prompt RPC completes,
  so adapter-level queuing alone does not protect lifecycle's shared completion
  channel and response buffers from concurrent callers. An execution-scoped
  mutex gives each prompt one waiter and one buffer set; dispatch-only sends
  leave a pending-completion barrier that the next send must consume before
  resetting those buffers. The sole exception is an adapter-attested,
  generation-matched human handoff: lifecycle flushes the earlier streaming
  boundary and releases its waiter before the successor can reset shared state.
  A delayed result from the earlier RPC is then stale by generation and is
  suppressed before prompt-end sweeps, usage consumption, trace clearing, or
  completion emission can affect the successor. The successor's own prompt-end
  sweeps also preserve typed background work that was live at handoff, including
  nested child-tool and Monitor lineage; explicit session cancellation or reset
  still owns full cleanup.
- Tool activity marks the foreground as generating only from positive ownership
  evidence. The initial tool call records whether it is top-level foreground,
  recognized background work, or a child of background work; subsequent
  updates inherit that ownership only within the execution that established it.
  Provider-local tool-call IDs may be reused after execution rotation; a stale
  predecessor entry is never ownership evidence for the successor. Missing
  parent metadata and update-only tool frames preserve the current activity
  rather than promoting unknown work to foreground. This is required for
  incremental ACP streams such as Claude's, which can temporarily omit
  `parentToolUseId` while the adapter still carries the child tool's cached
  normalized payload. A known top-level non-background tool still marks the
  foreground as generating, just like message and thinking frames.
- Activity records are execution-lifetime state. Each execution that dispatches
  a prompt or records tool/background activity claims the mapped session record.
  Terminal cleanup removes that execution's tool ownership, background work,
  and record claim. It detaches the whole record only when no other execution
  claim or in-flight prompt/dispatch token still uses it.
- Record lookup and retirement use one lock-coupled identity protocol: lookup
  locks the mapped record before releasing the map guard, while retirement
  validates and detaches that same identity under the guard. A successor
  therefore either mutates the retained record or creates a new record after
  retirement; it cannot mutate a detached predecessor pointer. Delayed
  publications validate both record identity and revision. Validation and
  synchronous event delivery are serialized by a stable, reference-counted
  per-session guard that survives record replacement; a predecessor cannot pass
  validation and then deliver after its successor. Foreground activity and
  active-subagent count are captured together under the record lock, so a
  predecessor value cannot be paired with successor-owned count state. Session
  deletion first quiesces any live lifecycle execution, marks its exact
  execution ID terminal, then forcibly detaches the identity and invalidates
  outstanding tokens.
- All activity-bearing lifecycle and activity-refresh events use one FIFO per
  task; different tasks may publish concurrently. Same-task reentrant
  publication enqueues and returns. Repository reads and synchronous callbacks
  occur outside the bookkeeping mutex. The last-delivered baseline advances
  only after a successful publish, deletion clears that task's queue and
  baseline state, and queued publication uses a bounded context independent of
  caller cancellation. This preserves a monotonic observer sequence when
  generating, background, and idle transitions arrive in quick succession.

The distinction is surfaced to the operator as a fine-grained substate (`foreground_activity`: `generating` vs `background`) so the UI can communicate two independent facts — "you may type" *and* "work is still in progress" — instead of collapsing them into one busy/done bit:

- The composer gates on foreground-generating rather than coarse `RUNNING`.
- A tri-state status indicator distinguishes generating / working-in-background / done; the established "running" affordance is unchanged and a distinct indicator is *added* for the background-idle substate — it never reads as "done" while background work runs.
- The compact sidebar deliberately reuses the dashed running spinner: yellow
  means foreground generation and violet means background work. A distinct
  tooltip and accessible label carry the textual meaning; richer status
  surfaces continue to show the background-work label.
- The substate is delivered live over a `session.activity_changed` WS event and
  carried on `session.state_changed`. It is also read from the in-memory tracker
  into the boot payload and the session REST/WS DTOs. Generating is meaningful
  for a `RUNNING` session; background may also be carried after the coarse state
  settles when detached work remains live.

## Consequences

The operator is no longer falsely locked out while background work runs, and
the UI truthfully shows "working in the background" as distinct from both
"generating" and "done". For an attested held-open Claude subagent turn,
acceptance now also means immediate provider delivery after the matching
human-origin idle boundary; detached child output may continue concurrently.
Without that authoritative, generation-matched handoff, the prompt remains
queued and the UI retains the foreground-busy state. Out-of-turn work such as a
Monitor burst or backgrounded shell keeps its existing behavior.

The signal and count are best-effort across a backend or agent-execution
restart. Connected executions retain background liveness and exact subagent
registrations across foreground turns, but work that survives a restart cannot
be reconstructed without an agent-side liveness API. Claude is the initial
production adapter attested for subagent liveness; all others keep the
conservative busy behavior.
Terminal execution teardown releases orphaned tool ownership and background
registrations, and releases the session activity record once no successor uses
it, so completed session history does not retain live-memory bookkeeping.

## Alternatives Considered

- **Rely on upstream synthetic idle-turn completion alone.** Rejected: it structurally cannot arm while the foreground prompt exchange is open, and its debounce re-extends under event bursts, so the held-open and chained-burst windows remain. A falsifiable acceptance test drives a chatty Monitor and confirms a prompt sent during a burst is accepted only with the fine-grained gate.
- **Drop the `RUNNING` gate / always accept input.** Rejected: it would let a new message race a genuinely-generating foreground turn, risking dropped or reordered messages and regressing non-steering agents.
- **Remove ACP prompt serialization after any foreground-idle event.** Rejected:
  synthetic wakeups and unsupported providers could race the held prompt,
  stealing or shifting prompt responses. Handoff is generation-matched,
  adapter-attested, and transferable only to a human successor.
- **Persist the foreground/background distinction to the database (like the coarse states).** Rejected: persistence without an agent-side reconciliation API becomes a second source of truth and can survive restart as a false live value after the workload has died. Connected-execution tracking is kept in memory and read at serialization boundaries; turn close no longer destroys it, while execution teardown does.
- **Recognize background work by normalized payload shape.** Rejected: the shape
  is shared presentation data and proves neither provider capability nor
  terminal lifecycle support. It caused Codex starts to register globally even
  though accountable terminal child activities were not reliably emitted in
  observed streams.
- **Maintain a central agent-name whitelist.** Rejected: an agent identity does
  not prove that the installed adapter/provider version emits the lifecycle
  frames Kandev expects. Capability is stamped at the adapter recognition point
  and locked by fixtures instead.
- **Maintain an increment/decrement subagent counter.** Rejected: duplicate or
  out-of-order terminal events can drift an independent counter. Count is
  derived from the idempotent live registration map.
- **Treat every parentless tool update as foreground activity.** Rejected:
  incremental providers may omit lineage metadata on an update after supplying
  it on the initial call. Reclassifying from each update makes missing metadata
  override known ownership and falsely closes the prompt gate. Unknown updates
  therefore preserve activity until positive foreground evidence arrives.
- **Load, check, and delete the session record without a shared identity
  protocol.** Rejected: a successor can load the predecessor's pointer before
  cleanup deletes the map entry, then mutate detached state that serializers no
  longer observe. Execution claims alone do not close that stale-pointer window;
  lookup, token use, publication, and retirement must validate the same mapped
  identity.
