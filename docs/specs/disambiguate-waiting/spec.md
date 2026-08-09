---
status: draft
created: 2026-08-08
updated: 2026-08-09
owner: kandev
---

# Waiting Attribution

> **Scope note — this spec has been narrowed TWICE.**
>
> **2026-08-09, round 3.** The elicitation half was cut to
> [`docs/specs/acp-elicitation/spec.md`](../acp-elicitation/spec.md).
>
> **2026-08-09, round 4.** At Spec Review's cap the human chose option (c) again and cut
> this card at the **notification boundary**: *"can u split into 2 task with 2 spec first
> … and we can review them independently."* The notification-deferral half is now
> [`docs/specs/parked-notification-deferral/spec.md`](../parked-notification-deferral/spec.md).
>
> **This spec is the ATTRIBUTION half only: observe it, project it, render it.
> Notification behaviour is UNCHANGED by this spec** — nothing is withheld, nothing is
> deferred, and `session.turn_finished` fires exactly when it fires today (AC-76). That is
> deliberate: it makes this half shippable on its own, and it satisfies the originating
> ticket's first acceptance bullet — *"an operator can tell, from the board and the task
> list alone, whether a task is blocked on them or on a machine"* — without taking on the
> timing surface.
>
> Acceptance-criterion identifiers are **deliberately not renumbered**, across both splits.
> AC-21…AC-72 keep the ids they had. Gaps are where the elicitation ACs and the deferral
> ACs went. New criteria continue from **AC-73**.
>
> Determinism decisions keep their round-3 identifiers D1–D9, minus **D4** (deferral),
> which moved to the sibling spec. The mapping to the combined spec's numbering, which the
> review record cites, is in *Determinism → identifier mapping*.

## Why

When a Kandev session stops producing output, an operator cannot tell whether the agent
needs an answer from them or is parked on work a machine will finish on its own. Both look
identical on the board and in the task list, so the operator learns to ignore the signal,
and a task that genuinely needs them waits longer than one that needs nobody.

Kandev already separates *"the agent asked a structured question"* from the rest, via
`pending_action`. The genuinely indistinguishable pair is **"finished the step"** versus
**"parked on a background shell it will come back to."** This spec makes that pair
distinguishable on every operator-facing surface.

It does **not** change when anyone is notified. Suppressing the ping is the sibling spec's
job and depends on this one.

## Verified inputs

Every claim below was read from the shipped artifact named, or measured on a running
instance. This section is contract input. Line numbers are as of `bf62f39b1`.

### G. What the board and the task list actually show — CORRECTED 2026-08-09

**An earlier revision of this section was wrong, and the error survived two review rounds
marked "citations verified".** It claimed the resolution for both surfaces lives in
`getTaskStateIconConfig`. It does not. The corrected map is §M; what follows is only the
consequence for the originating ticket's two live tasks (`state=REVIEW`,
`pending_action=null`, no `foreground_activity`, primary session `WAITING_FOR_INPUT`):

| Surface | What renders today | Why |
|---|---|---|
| Sidebar task list (`task-item.tsx`) | `data-testid="task-state-turn-finished"`, a green `IconProgressCheck` (`task-item.tsx:279-284`) | falls through the private ladder to `classifyTask(...) === "review"` and is not on the last workflow step |
| Board card (`kanban-card-content.tsx`) | **nothing at all** — no icon element is rendered | `renderTaskStatusIcon` returns `null` at `kanban-card-content.tsx:275`; see §M |
| `/tasks` list row (`rich-task-list-row.tsx:38`) | yellow `IconCheck` (`TASK_STATE_ICONS.REVIEW`, `state-icons.tsx:36`) | goes through the shared resolver, which has no `REVIEW`-specific branch |
| Session switcher (`sessions-dropdown.tsx:475`) | yellow `IconMessageQuestion` (`SESSION_STATE_ICONS.WAITING_FOR_INPUT`, `state-icons.tsx:56`) | coarse session state |

Three different affordances for one condition, and one surface showing none. The earlier
"both land on a yellow `IconCheck`" claim was true only of `rich-task-list-row.tsx`, the
one surface no revision of this spec had named.

A background affordance already exists and is already wired — see §M — but it never lights
up in a shipped profile, for the reason in §H.

### H. Why the existing background signal cannot carry this

`ForegroundActivity` collapses to `generating` unless
`features.claudeBackgroundPromptHandoff` is on
(`internal/orchestrator/turn_activity.go:1211-1219`), and that flag is `"false"` in `prod`,
`dev`, and `e2e` in `profiles.yaml`. The reason is recorded in
`docs/decisions/2026-07-28-coarse-running-busy-signal.md`: the tracker is an edge-driven
refcount, and per `turn_activity.go:83-90` a Claude task-notification frame "exposes only
origin.kind, so workID is often available at launch but absent at completion", forcing the
code to retire the oldest outstanding registration. A refcount that only increments
reliably shows "still working" forever.

Two things follow, and both are contract:

- This spec **derives liveness by sampling a level**, never by counting edges. An
  implementation MUST NOT introduce a refcount that can only be decremented on a frame that
  may not arrive.
- This spec is **independent of `features.claudeBackgroundPromptHandoff`**, which stays off
  and is never read. Verified achievable: background-work *registration*
  (`registerBackgroundWorkKind`, called from `event_handlers_streaming.go:381`, `:648`,
  `:695` and `turn_activity.go:449`) is **unflagged**; only the *exposure*
  (`Service.ForegroundActivity`) is gated.

The protocol will not supply the missing signal either. `StopReason` is a closed
five-member enum — `end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled`
(`types_gen.go:6631-6637`) — with no member meaning "turn over, work continues, I will come
back". Sweeping the vendored schema at version `1.20.0` for the terms that would express
one returns **zero occurrences in both tiers** (`background`, `async`, `detach`, `parked`,
`long_running`).

`_meta.claudeCode.toolResponse.status: "async_launched"`
(`internal/agentctl/types/streams/tool_payload.go:296-302`) appears nowhere in the bridge's
`dist/` — it is opaque passthrough from the Claude Code CLI, not a bridge contract. **No
behaviour in this spec may depend on it.**

### I. What Kandev owns out of band

- `stampBackgroundShellWork` (`.../transport/acp/normalize.go:304-307`) stamps
  `BackgroundWorkPayload{Kind: shell, Detached: true, Ended: false}` when
  `agentID == claudeAgentID` **and** `ShellExec().Background` is true. Verified: the payload
  is exactly `{Kind, WorkID, Detached, Ended}`
  (`types/streams/background_work.go:17-22`) and shells are stamped with `workID: ""`.
  **The attestation carries no PID**, so it cannot name the process it attests to. This is
  why the probe below is defined as a time-scoped predicate rather than a lookup.
- Every agent is launched with `Setpgid: true` and `proc.pgid = cmd.Process.Pid`
  (`internal/agentctl/server/process/runner.go:388`), so the agent process is a process
  **group leader**. §L measures what that group actually contains, and the answer
  invalidates the obvious design.
- The backend's own PID handle is *not* usable. `Manager.resolveLocalPID`
  (`internal/agent/runtime/lifecycle/persistence.go:150`) returns the standalone agentctl
  control-server PID and **returns 0 for every non-standalone runtime**; `RowProcessLiveness`
  (`liveness.go:24`) returns `Unknown` for anything that is not `RuntimeStandalone`. The
  sample must be taken where the agent runs, i.e. in agentctl.
- **The agent stream is a single ordered consumer.** `handleAgentStreamEvent`
  (`internal/orchestrator/event_handlers_streaming.go:24`) is the sole entry point for
  agent events; both the background-work attestation path (`handleToolCallEvent` at `:309`
  and `trackBackgroundToolUpdate` at `:633`, which call `registerBackgroundWorkKind`) and
  the turn-completion path (`publishAgentTurnComplete` at `:436`) are dispatched from it.
  This is the ordering guarantee the attestation depends on; see D3 and AC-79.

### J. The one live observation of the parked condition

Observed on a running instance on 2026-08-08. A Kandev Verify step parked on a background
e2e shell held `WAITING_FOR_INPUT` for **≈12 minutes** with a live process tree throughout,
then **self-resumed** with no operator input.

Two things follow, and both constrain this spec:

1. **The parked condition is real, and it is long.** Twelve minutes is long enough for an
   operator to see the card, act on it, and be wrong. That is the cost this spec removes.
2. **A self-resume must clear the affordance promptly and without a probe.** The agent came
   back on its own; the card must stop reading "a machine is working" the moment the session
   leaves `WAITING_FOR_INPUT`, whatever the last sample said. That is the third term of the
   formula and AC-68.

The third consequence the combined spec drew from §J — that self-resume latency calibrates
a notification backstop — belongs to the sibling spec and has moved there.

### L. What the agent's process group actually contains — MEASURED, and it changes the design

**This is the decisive input and it was measured, not read.** The combined spec asserted,
from `Setpgid: true`, that "the agent and everything it spawns share one process group", and
defined the probe as *"the group contains at least one non-zombie member other than the agent
process itself"*. Both halves of that are wrong on a real session.

Measured 2026-08-09 on macOS (darwin 25.6.0) against a **live Kandev Claude ACP session**, by
enumerating the agent's process group and walking the parent chain of a process the agent had
backgrounded. Re-runnable:

```sh
AGENT_PGID=$(ps -o pgid= -p "$(pgrep -f claude-agent-acp | head -1)" | tr -d ' ')
ps -eo pid,ppid,pgid,stat,comm | awk -v g=$AGENT_PGID '$3==g'
```

**Measurement 1 — the group is never empty, so `live` would be permanently true.** With the
session merely idle and **no background shell running at all**, the agent's process group
(`pgid 71794`) contained **five non-leader, non-zombie members**:

| pid | ppid | pgid | what it is |
|---|---|---|---|
| 71794 | 43844 | 71794 | `npm exec @agentclientprotocol/claude-agent-acp` — the **group leader**, the process agentctl launched |
| 71842 | 71794 | 71794 | `node .../bin/claude-agent-acp` — the ACP **bridge** |
| 71851 | 71842 | 71794 | `.../claude-agent-sdk-darwin-arm64/claude` — the **Claude CLI** the bridge spawns |
| 6455 | 71842 | 71794 | a second `claude` CLI process |
| 71868 | 71851 | 71794 | `python -m backend.mcp.sqlite_server …` — a **stdio MCP server** |
| 6480 | 6455 | 71794 | a second stdio MCP server |

The bridge and the CLI are **unconditional** — every Claude ACP session has them. The stdio
MCP servers appear whenever the user has configured one. Under the combined spec's definition
this session reports `live` forever.

**Note the pids.** The leader is `71794`; the second CLI is `6455` and the second stdio MCP
server is `6480`. Those are *wrapped-around* pids, so both were spawned **long after** the
session leader — they are not present at session start. That is the empirical basis for
AC-70a and for the *Failure modes* row on lazily-connected stdio MCP servers.

Note for the record: an earlier review round guessed this failure would come from Kandev's
**own** injected MCP servers. That guess was wrong and is refuted — `injectKandevMcpServers`
(`internal/agentctl/server/api/agent.go:333`) injects exactly two entries, both **URL-based**
(`mcpTransportHTTP` and `mcpTransportSSE`, pointing at `http://localhost:<port>`), and spawns
no child process. The real cause is the bridge and CLI themselves, which no configuration
removes.

**Measurement 2 — the thing we are looking for is not in the group at all.** A process
backgrounded from inside the agent session was launched, and its ancestry walked:

```
pid=5980  ppid=5962  pgid=5962   sleep          <- the backgrounded workload
pid=5962  ppid=6455  pgid=5962   /bin/zsh       <- the shell that launched it
pid=6455  ppid=71842 pgid=71794  claude         <- the CLI
pid=71842 ppid=71794 pgid=71794  node           <- the bridge
pid=71794 ppid=43844 pgid=71794  npm exec …     <- the agent, group leader
```

The backgrounded process has **`pgid 5962`, not `71794`**. The Claude CLI puts each shell it
spawns into its **own** process group, as job control requires. So a background shell is
**never** a member of the agent's process group — but it **is** a transitive descendant: the
parent chain reaches the agent leader in four hops.

**Both conclusions are contract:**

> Process-**group membership** answers a question whose answer is permanently "yes" *and*
> excludes the only thing worth detecting. The evidence source is the **descendant tree**,
> scoped by **process start time**, never process-group membership.

### M. The frontend resolver map — DERIVED FROM THE TREE, 2026-08-09

**This section exists because the Rendering contract was wrong in two consecutive revisions
and both times the error passed review.** Rounds 1 and 2 grepped that each symbol *exists*;
neither checked that the symbols live in the *same* resolver. Every line below was read from
the tree at `bf62f39b1`. A reviewer should re-derive all of it.

**There are TWO independent task-icon resolvers, not one.**

**Resolver A — private to the sidebar task list.** `TaskStateIcon`, a local component at
`apps/web/components/task/task-item.tsx:187-292`, rendered once at `task-item.tsx:483`.
`task-item.tsx` imports **only** `shouldUseQuestionTaskIcon` and `shouldUsePermissionTaskIcon`
from `state-icons` (`task-item.tsx:28-30`); it calls `getTaskStateIcon` **zero times**. Its
ladder, in order, with the `data-testid` each branch emits:

| # | condition | test id | line |
|---|---|---|---|
| 1 | `shouldUsePermissionTaskIcon(hasPendingPermission)` | `task-state-pending-permission` | `:207-213` |
| 2 | `hasPendingClarification` | `task-state-waiting-for-input` | `:215-221` |
| 3 | `foregroundActivity === "generating"` | `task-state-running` | `:223-230` |
| 4 | `foregroundActivity === "background"` | `task-state-background-running` (via `BackgroundWorkTaskIcon`) | `:232-234` |
| 5 | `shouldUseQuestionTaskIcon(state)` | `task-state-waiting-for-input` | `:235-242` |
| 6 | `computeIsPreparing(state, sessionState)` | `task-state-running` | `:243-251` |
| 7 | `isInProgress` | `task-state-running` | `:254-262` |
| 8 | `interrupted && !isTerminalInterruptedState(...)` | `task-state-interrupted` | `:267-269` |
| 9 | `classifyTask(...) === "review"` | `task-state-workflow-complete` / `task-state-turn-finished` | `:270-285` |
| 10 | fallback | `task-state-backlog` | `:286-291` |

**Resolver B — the shared one.** `getTaskStateIconConfig`
(`apps/web/lib/ui/state-icons.tsx:254-281`), reached through the exported `getTaskStateIcon`
(`:283-295`), taking `TaskStateIconOptions` (`:246-252`: `hasPendingClarification`,
`foregroundActivity`, `hasPendingPermission`, `interrupted`). Its ladder is *similar but not
identical* to A — it has an `interrupted` branch and none of A's branches 6, 7, 9, 10 — and,
critically:

> **`getTaskStateIcon` emits NO `data-testid` on any branch.** It returns
> `<config.Icon className={…}/>` (`state-icons.tsx:294`). The single exception is
> `TASK_INTERRUPTED_ICON`, which is special-cased at `state-icons.tsx:291-293` to render the
> exported `InterruptedTaskIcon` component (`:183-203`), and *that component* carries
> `data-testid="task-state-interrupted"` (`:195`) plus a tooltip and an accessible label.
> **That special case is the precedent this spec follows.**

Its background branch returns `TASK_BACKGROUND_ICON` (`:101-104`), a violet **`IconLoader`** —
a *different icon* from resolver A's `BackgroundWorkTaskIcon`, which is a violet
**`IconCircleDashed`** (`task-item.tsx:175-179`).

**`data-testid="task-state-background-running"` has exactly ONE producer in the whole app**:
`BackgroundWorkTaskIcon` at `task-item.tsx:165-185`, which is **not exported**. Verified by
`grep -rn 'task-state-background-running'` over `apps/web` — two hits, that line and
`task-item.test.tsx:11`.

**`getTaskStateIcon`'s production call sites — SIX, in five files:**

| file:line | surface |
|---|---|
| `apps/web/components/kanban-card-content.tsx:285` | board card |
| `apps/web/app/tasks/rich-task-list-row.tsx:38` | `/tasks` list row |
| `apps/web/components/kanban/swimlane-graph-content.tsx:84` | swimlane node |
| `apps/web/components/kanban/swimlane-graph-content.tsx:121` | swimlane node |
| `apps/web/components/kanban/graph2-step-node.tsx:149` | graph step node |
| `apps/web/components/task/task-state-actions.tsx:33` | task action affordance |

`rich-task-list-row.tsx` was named in **no** revision of this spec, in either the
"call sites that pass the option" list or the "call sites deliberately not updated" list. It
also reads `task.foreground_activity` (snake_case) where the board reads
`task.foregroundActivity` — two different `Task` shapes; a builder touching both must not
assume one.

**The board has TWO early returns before the resolver, not one.**
`renderTaskStatusIcon` (`kanban-card-content.tsx:263-291`):

```
:275   if (!showRunningSpinner && !needsMe && !hasActivity && !showInterrupted) return null;
:282   if (showRunningSpinner && !needsMe && task.foregroundActivity !== "background")
           return <IconLoader2 … />;
:285   return getTaskStateIcon(task.state, "h-4 w-4", { … });
```

An earlier revision named only `:282`. **`:275` is the one a parked task actually hits.** For
a settled session, `shouldShowTaskRunningSpinner` (`state-icons.tsx:225-236`) returns `false`
— `WAITING_FOR_INPUT` is not in `ACTIVE_SESSION_STATES` (`:145-148`) — so `showRunningSpinner`
is false; `needsMe`, `hasActivity` and `showInterrupted` are all false too. The function
returns `null` and **the board renders no icon at all**. Both conditions must exclude a parked
task, and `:275` is the load-bearing one. AC-58 asserts this.

**Session-level resolution — the spec's earlier description of this half was CORRECT.**
`getSessionStateIconConfig` (`state-icons.tsx:297-311`) resolves
`canRequestInput && hasPendingPermission` → `canRequestInput && hasPendingClarification` →
`canRequestInput && foregroundActivity === "background"` → coarse `SESSION_STATE_ICONS[state]`.
Its three production call sites are `sessions-dropdown.tsx:475`,
`session-reopen-menu.tsx:204`, `mobile/mobile-sessions-section.tsx:132`. Like the task
resolver, it emits no `data-testid`; the switcher surfaces assert on the icon component.

### N. What a Claude self-resume actually does to the turn boundary — READ FROM THE TREE

The PID-free probe rests on attestation and the probe baseline being scoped to *the same
turn*. §J's observation is an agent that **self-resumed with no operator input**, so whether
that produces a turn boundary is load-bearing, and an earlier revision simply asserted it did
"by construction". It was never checked. It is now:

- **A Claude self-resume goes through a real `session/prompt`.** The Claude Agent SDK exposes
  `ScheduleWakeup`; when its timer fires the SDK queues a turn on its async iterator, and the
  bridge only drains that iterator inside its `prompt()` handler. Kandev's adapter closes the
  gap explicitly: `wakeupScheduler` (`.../transport/acp/wakeup.go:11-46`) records the pending
  wakeup and, at fire time, `fireWakeup` (`.../transport/acp/adapter_prompt.go:388-403`)
  issues a **synthetic `session/prompt`** through `sendPrompt` (`adapter_prompt.go:71`),
  serialized behind the same `promptGate` as a human prompt.
- **So agentctl's baseline does advance on a self-resume.** A synthetic prompt is a
  `session/prompt` dispatch like any other.
- **But it does not originate in the backend.** `sendPrompt` distinguishes them:
  `humanPrompt := expectSession == ""` (`adapter_prompt.go:79`), and the synthetic path passes
  a pinned `expectSession`. The backend's own turn-start entry point,
  `startTurnForSession` (`internal/orchestrator/service.go:1278`), is on the prompt-admission
  path and is not called by agentctl's synthetic dispatch.
- **And no stream event marks a turn start today — VERIFIED 2026-08-09.** An earlier revision
  wrote D3's clearing rule as "keyed on the stream event caused by that dispatch", which reads
  as though a carrier already exists. It does not. The agent stream's event set
  (`internal/agentctl/types/streams/agent.go:6-80`) has **no** turn-start member — it is
  `message_chunk`, `reasoning`, `tool_call`, `tool_update`, `plan`, `agent_plan`, `complete`,
  `error`, `permission_request`, `permission_cancelled`, `session_status`, `context_window`,
  `foreground_idle`, `background_complete`, `available_commands`, `session_mode`, `rate_limit`,
  `agent_capabilities`, `session_models`, `session_info`, `auth_required`, `mcp_attachment`.
  The closest candidate, `session_status`, is emitted from exactly two sites
  (`.../transport/acp/adapter_session.go:132` and `:514`) and both are session create/resume,
  not prompt dispatch. So the boundary D3 requires has to be **published**, not observed. The
  *API surface → Turn-boundary stream event* section defines it.
- **`sendPrompt` has THREE callers and one natural dispatch instant — READ 2026-08-09.** An
  earlier draft of that section spoke of "the operator path and the synthetic path", which is
  not the shape of the code. `sendPrompt` (`adapter_prompt.go:71`) is a funnel for `Prompt`
  (`:31`), `PromptSteer` (`:60`) and `fireWakeup` (`:434`); steering is fully shipped, reached
  via `manager_interaction.go:127` → `session.go:1330` → `agent.go:787`. Inside the funnel the
  prompt passes the gate (`acquirePromptTurn`, `:99`), may be **dropped** without dispatching
  (`:120`, a pinned wakeup whose session changed), and only then calls
  `a.beginPromptTurn(sessionID)` (`:140`) immediately before `conn.Prompt`. `beginPromptTurn`
  already bumps a per-session turn epoch (`adapter_async_complete.go:150-155`), so a turn marker
  at that instant already exists and this feature stamps beside it rather than inventing a
  second one. That is what makes the emission point specifiable rather than a builder's guess.

The consequence is the opposite of what the review round hypothesised, and it is worse in the
other direction: **agentctl's baseline can advance while the backend's `observed_detached` is
not cleared**, so a stale attestation from turn N could mark turn N+1. D3 resolves this by
naming a single turn boundary for the whole feature.

This does not claim `ScheduleWakeup` is the *only* self-resume mechanism. D3 states what
happens when a continuation produces no `session/prompt` at all, so both branches are
covered.

## Requirements

- A session that has settled while an observed detached background workload is still alive
  SHALL be described as **parked**, and SHALL be distinguishable from a settled session with
  no live background work **on the board, in the task list, and in the session switcher**.
- Parked is a **projection**, not a lifecycle state. `TaskSessionState` gains no member; the
  state machine and prompt-admission rules are unchanged.
- Liveness SHALL be derived by **sampling a level** — is a workload started during this turn
  still running — never by counting edges.
- Suppression of *nothing*: **notification behaviour SHALL be byte-for-byte unchanged by this
  spec.** No notification is withheld, deferred, delayed, reordered, or dropped. AC-76 is the
  guard. Deferral is the sibling spec's contract and depends on this one.
- The projection SHALL be conservative: it is true only when Kandev both observed the detached
  launch **and** can positively sample a workload started during that turn as still alive. An
  indeterminate sample SHALL render exactly as today.
- **Detached-launch attestation SHALL be recognised per agent through a named seam**
  (*API surface → The launch-recogniser seam*). Claude is the only recogniser registered at
  ship time. Registering a second vendor's recogniser SHALL NOT require changing the probe,
  the projection, or any icon call site.
- An agent with **no registered recogniser** SHALL never be attested, never be probed, and
  never be parked — it behaves exactly as it does today.
- This path SHALL be independent of `features.claudeBackgroundPromptHandoff`, which remains
  off and is neither read nor weakened.

## Data model

No schema migration. Runtime-only projections over existing durable rows.

### Parked projection (backend process memory, per session)

```
parkedState
  session_id            string  PK
  parked                bool    projected on session and task DTOs
  revision              uint64  monotonic per session; increments on each
                                transition of `parked`, never on a read (D1)
  observed_detached     bool    a recogniser attested a detached launch during
                                the turn that settled (D3)
  last_sample           enum    live | settled | unknown
  last_sampled_at       timestamp
```

**`parked` is true when ALL THREE hold**, and false in every other combination:

1. `observed_detached` is true for the turn that settled, **and**
2. `last_sample == live`, **and**
3. the session's `TaskSessionState` is `WAITING_FOR_INPUT`.

The third term is not decoration. Without it the formula contradicts the state-machine
table's first row (`RUNNING` → always `false`) on the most common physical sequence there is:
the agent self-resumes while the tree is still warm, so `observed_detached` is true and
`last_sample` is frozen at `live` — and it stays frozen, because sampling stops when the
session leaves `WAITING_FOR_INPUT` (AC-53). A two-term formula would leave the card spinning
while the session is actively `RUNNING`, forever. AC-68 observes this.

**There is deliberately no `sampling` field.** An earlier revision carried one, documented as
"true exactly while parked". That invariant is false: at `KANDEV_PARKED_PROBE_INTERVAL = 0` a
session can be parked from the synchronous first sample with no loop running, so
`parked ∧ ¬sampling` is reachable. The field was derivable from the loop's own bookkeeping and
observed by no AC, so it is **removed** rather than restated — a field whose stated invariant
is false is worse than no field. Whether a given session is currently being sampled is an
implementation detail of the loop, bounded by AC-53 and AC-54.

### Task-level projection (derived, not stored)

`parked_on_background_work` at the task level is the **OR** across the task's sessions, paired
with `parked_revision`, a **per-task monotonic counter**.

**`parked_revision` is NOT the maximum of the member sessions' revisions.** That rule was
specified in an earlier revision and is provably defective; it is recorded here so it is not
reintroduced. Counterexample using only this spec's own definitions: session S1 has toggled
twice (revision `2`, currently `false`); session S2 has never transitioned (revision `0`,
`false`). Under the old rule the task projects `(false, 2)`. S2 now parks: its revision becomes
`1`, the task's OR flips to `true`, and `max(2, 1)` is **still `2`**. Two consecutive task
carriers then go out with contradictory values at the *identical* revision, and the consumer
rule cannot order them — `<` lets a reordered stale update win, `<=` drops the legitimate flip.
The claim that justified it — "any session transition that can flip the OR necessarily raises
the maximum" — is false whenever the flipping session is not the one holding the maximum.

*(The counterexample previously read "toggled twice (revision `4`)", which contradicts D1's
one-increment-per-transition rule and made AC-49's GIVEN unconstructible. Corrected to `2`.)*

So instead:

- The task keeps its **own** `uint64`, incremented **once per observed change of the
  task-level boolean**, never on a read and never on a session transition that does not flip
  the OR.
- The boolean and the counter are read in **one critical section** across all of the task's
  sessions, so they cannot come from different instants.
- A consumer discards a task-level update whose `(parked_epoch, parked_revision)` is lower
  than one it has already applied **for that task**, compared as an ordered pair (see
  *Revision epoch* below). Task revisions and session revisions are compared only against
  their own kind, never with each other.
- A task with no sessions, or no session that has ever recorded a transition, projects `false`
  with revision `0` (D9).
- Like the session value, it is published only on change and never persisted.

### Revision epoch — what makes the restart reset survivable

The discard rule and the restart contract contradict each other unless something on the wire
distinguishes "an older frame arrived late" from "the backend restarted and its counters began
again at `0`". Nothing did: after a restart a live browser tab holding revision `N > 0` would
discard every `revision: 0` frame for that session, leaving the affordance stuck exactly as it
was — the "card stuck" failure this feature exists to prevent, reachable through an ordinary
backend restart. The *Out of scope* section asserted a client "must not treat the reset as
stale" but named no mechanism.

The mechanism is an **epoch**:

- **`parked_epoch`** — a `uint64`, the backend process's start time in Unix nanoseconds, fixed
  for the life of the process and **identical on every carrier and every session and task**.
  It is not per-entity and never increments during a process's life.
- Every carrier that carries a `revision` or a `parked_revision` also carries `parked_epoch`.
- The consumer rule is a **lexicographic comparison of the ordered pair
  `(parked_epoch, revision)`**. A strictly higher epoch **always** wins, whatever the revision.
  Within one epoch the rule is exactly as before: discard a strictly lower revision.
- A client that applies a boot payload additionally resets its applied-revision map for every
  session and task the payload contains. This is belt-and-braces: the epoch alone is sufficient,
  and is what covers a client that reconnects the WebSocket **without** re-fetching boot.

AC-77 observes the reset end to end. Choosing process start time rather than a persisted
counter is deliberate: it needs no storage, it is monotonic across restarts on any sane clock,
and a backend that restarts twice within one nanosecond is not a case this feature owes an
answer to.

## API surface

### Parked projection

Additive fields, on:

- the session REST DTO and the boot payload's session records;
- `session.state_changed` and `session.activity_changed`;
- the task DTO, the boot payload's task records, and `task.updated`, where the task-level value
  is the OR across the task's sessions.

| Field | Type | Default | Where |
|---|---|---|---|
| `parked_on_background_work` | bool | `false` | session and task carriers |
| `revision` / `parked_revision` | uint64 | `0` | session carriers / task carriers |
| `parked_epoch` | uint64 | `0` | every carrier that carries either revision |

`foreground_activity` and `active_subagent_count` keep their current meaning and their current
gate. No existing field changes shape or value.

**What actually fires when the value changes.** "Published only on change" is a *necessary*
condition, not a trigger, and a parked transition has no natural one: an un-park at the
`live → settled` sample leaves the session state at `WAITING_FOR_INPUT` and
`foreground_activity` untouched, so none of the listed carriers has an independent reason to
emit. Therefore a transition of `parked_on_background_work` — **in either direction, and via
any of the three terms of the formula** — itself publishes:

- `session.activity_changed` for that session, carrying the new value, its `revision`, and
  `parked_epoch`;
- **`task.updated` for its task, if and only if the task-level OR also changed**, carrying the
  recomputed OR, the task's own `parked_revision`, and `parked_epoch`, read in the same
  critical section.

**The task-level emission is conditional, and this resolves a contradiction an earlier revision
carried.** The publish rule said a session transition publishes `task.updated` unconditionally;
the task-level rule said the counter increments and publishes only when the task boolean
changes. For a task where S1 is already parked and S2 then parks, those two rules demand
opposite things. **The task-level rule wins**: a session transition that does not flip the OR
publishes `session.activity_changed` **only**, and the task's counter does not move. Rationale:
the counter is what orders task carriers, so emitting a `task.updated` that cannot advance it
would put two frames on the wire that a consumer cannot order — the same defect the `max()`
rule was rejected for. AC-78 observes the negative case.

"Via any of the three terms" is load-bearing: the third term (session state) is how a
self-resume clears the affordance, and that is the common case — AC-68 observes exactly that.

### Rendering contract

**REWRITTEN 2026-08-09.** Two previous revisions of this section named
`getTaskStateIconConfig` as the resolver for both task surfaces. Per §M there are **two**
independent resolvers and the sidebar task list does not use the shared one. Every claim below
cites `file:line`; §M is the derivation.

The affordance reuses what already exists — **no new icon, no new `data-testid` string, no new
user-facing string, and therefore no new i18n surface.** What does change is *where* the
existing affordance lives, so that both resolvers can reach it.

#### 1. Promote `BackgroundWorkTaskIcon` to the shared module, following the `InterruptedTaskIcon` precedent

`BackgroundWorkTaskIcon` (`task-item.tsx:165-185`) is the app's only producer of
`data-testid="task-state-background-running"` and it is private. Move it to
`apps/web/lib/ui/state-icons.tsx` and export it, **unchanged** — same `IconCircleDashed`, same
violet, same `data-testid`, same `aria-label` and tooltip from `task:backgroundWorkIsRunning`.
`task-item.tsx` imports it instead of declaring it.

This is not a new pattern. `state-icons.tsx` already does exactly this for the interrupted
affordance: `InterruptedTaskIcon` is an exported component carrying its own `data-testid`
(`:195`), tooltip and label, and `getTaskStateIcon` special-cases its config to render the
component rather than a bare icon (`:291-293`). The parked affordance follows that path
verbatim.

Consequence, stated so it is not a surprise: on the surfaces that go through the shared
resolver, the **parked** affordance is `BackgroundWorkTaskIcon` (violet `IconCircleDashed`)
while the pre-existing `foreground_activity === "background"` affordance remains
`TASK_BACKGROUND_ICON` (violet `IconLoader`, `state-icons.tsx:101-104`). Two violet spinners
for two different signals. `TASK_BACKGROUND_ICON` and its branch are **not touched** — leaving
the flagged experiment byte-identical is what AC-59 pins.

#### 2. Resolver A — the sidebar task list (`task-item.tsx`)

Add one branch to the private `TaskStateIcon` ladder (§M), **between branch 4
(`foregroundActivity === "background"`, `task-item.tsx:232-234`) and branch 5
(`shouldUseQuestionTaskIcon(state)`, `:235-242`)**:

```
if (parkedOnBackgroundWork) return <BackgroundWorkTaskIcon />;
```

`TaskStateIcon`'s prop bag (`task-item.tsx:187-206`) gains `parkedOnBackgroundWork?: boolean`,
**defaulting to `false`**, threaded from the call site at `:483`.

Position is contract, and it is determined by the branches on either side rather than by an
absolute index: **after** the `foreground_activity` branches, so the flagged experiment stays
authoritative when it is ever enabled; **before** `shouldUseQuestionTaskIcon` and everything
below it, so a parked task does not read as the coarse review/question state. It therefore also
outranks branches 6–10, including `interrupted` (`:267-269`) and the `review` classification
(`:270-285`) — the branch a parked task hits today. That precedence is intentional: a live
machine is more informative than a coarse workflow state. Both pending-input branches (1 and 2)
still outrank parked, so a real question always wins — AC-34.

#### 3. Resolver B — the shared `getTaskStateIconConfig` (`state-icons.tsx`)

`TaskStateIconOptions` (`state-icons.tsx:246-252`) gains `parkedOnBackgroundWork?: boolean`,
**defaulting to `false`** so a call site that does not pass it keeps today's behaviour by
omission and cannot regress. A new branch is evaluated **between
`foregroundActivity === "background"` (`:271`) and `isWaitingForInputState(state)` (`:272`)**,
returning a new `IconConfig` sentinel, and `getTaskStateIcon` special-cases that sentinel to
render `<BackgroundWorkTaskIcon />` — exactly as `:291-293` already special-cases
`TASK_INTERRUPTED_ICON` to `<InterruptedTaskIcon />`. This is what makes
`data-testid="task-state-background-running"` reachable on the board at all; without it the
shared resolver emits no test id on any branch and AC-58 would be unsatisfiable.

#### 4. The board also needs both early returns changed (`kanban-card-content.tsx`)

Per §M, `renderTaskStatusIcon` has **two** early returns before it reaches the resolver:

- **`:275`** — `if (!showRunningSpinner && !needsMe && !hasActivity && !showInterrupted) return null;`
  A settled parked task satisfies every clause, so the board renders **no icon at all** today.
  **This condition must also exclude a parked task.** This is the load-bearing one; an earlier
  revision named only `:282` and would have left AC-58 failing.
- **`:282`** — `if (showRunningSpinner && !needsMe && task.foregroundActivity !== "background") return <IconLoader2 …/>;`
  Must exclude a parked task as well, mirroring how it already excludes
  `foregroundActivity === "background"`.

Passing the option through is necessary and not sufficient at either site, which is why AC-58
asserts the board separately from AC-23's task list.

#### 5. Which call sites pass it, and which deliberately do not

| Call site | Passes `parkedOnBackgroundWork`? | AC |
|---|---|---|
| `components/task/task-item.tsx:483` (resolver A) | **yes** | AC-23 |
| `components/kanban-card-content.tsx:285` + both early returns | **yes** | AC-58 |
| `app/tasks/rich-task-list-row.tsx:38` | **yes** | AC-73a |
| `components/kanban/swimlane-graph-content.tsx:84`, `:121` | no — `false` default | AC-59 |
| `components/kanban/graph2-step-node.tsx:149` | no — `false` default | AC-59 |
| `components/task/task-state-actions.tsx:33` | no — `false` default | AC-59 |

`rich-task-list-row.tsx` is **in scope**, newly. It is a task-list surface — the `/tasks` list
— and the requirement says "in the task list" without qualification. Leaving it out would mean
one of Kandev's two task lists silently kept today's ambiguity. It was omitted from every prior
revision because no revision had enumerated the shared resolver's call sites; §M does. Note it
reads `task.foreground_activity` (snake_case) where the board reads `task.foregroundActivity`.

The three graph/action affordances stay on the `false` default; see *Out of scope*.

#### 6. Session-level rendering — required, and it does not come for free

`getSessionStateIconConfig` (`state-icons.tsx:297-311`) reaches `SESSION_BACKGROUND_ICON` only
via `canRequestInput && foregroundActivity === "background"` (`:308`), and per §H that value
never appears in a shipped profile. Left alone, a parked session would render the ordinary
`WAITING_FOR_INPUT` question mark in the switcher and the requirement would be quietly unmet.
It gains a `parkedOnBackgroundWork` parameter, defaulting to `false`, and resolves:

1. `canRequestInput && hasPendingPermission` → `PENDING_PERMISSION_ICON` (`:304`)
2. `canRequestInput && hasPendingClarification` → `SESSION_STATE_ICONS.WAITING_FOR_INPUT` (`:305`)
3. `canRequestInput && foregroundActivity === "background"` → `SESSION_BACKGROUND_ICON` (`:308`)
4. **new:** `canRequestInput && parkedOnBackgroundWork` → `SESSION_BACKGROUND_ICON`
5. coarse `SESSION_STATE_ICONS[state]` (`:310`)

Mirroring the task row exactly: after the flagged experiment, and after both pending-input
branches so a real question always outranks parked. Three call sites pass it —
`sessions-dropdown.tsx:475`, `session-reopen-menu.tsx:204`,
`mobile/mobile-sessions-section.tsx:132` — and any that does not keeps today's behaviour via
the `false` default (AC-52).

### Turn-boundary stream event (agentctl → backend)

**ADDED 2026-08-09.** D3 names one turn boundary for the whole feature — agentctl's
`session/prompt` dispatch — and requires the backend to clear `observed_detached` on it. §N
verifies that **no existing stream event carries that boundary**, so this spec publishes one.
Without it D3 is unimplementable as written and a builder would have to invent the carrier.

| | |
|---|---|
| **Event type** | `turn_started` (a new `streams.EventTypeTurnStarted`) |
| **Direction** | agentctl → backend, on the existing agent stream |
| **Emitted** | at the single dispatch point defined below — **every** `session/prompt` that actually reaches the agent, including the synthetic one a `ScheduleWakeup` self-resume issues |
| **Payload** | `session_id` only. **No timestamp** — the turn start stays in agentctl's own memory and clock (*Background-workload liveness probe*), and nothing about it crosses the process boundary |
| **Consumed by** | `handleAgentStreamEvent` (`internal/orchestrator/event_handlers_streaming.go:24`), the single ordered consumer D3 requires |

**The emission point is named exactly, because the three obvious placements are all wrong.**
An earlier draft of this section said only "on every `session/prompt` dispatch" and enumerated
two paths, which left a builder to choose the site. Read from the tree 2026-08-09:

> The event is emitted **inside `sendPrompt`, at `a.beginPromptTurn(sessionID)`**
> (`adapter_prompt.go:140`) — the instant after the prompt gate is acquired and after the
> pinned-session drop check, immediately before `conn.Prompt`. agentctl stamps its recorded
> turn start at that same call. `beginPromptTurn` already exists and already bumps a
> per-session turn epoch (`adapter_async_complete.go:150-155`); this feature adds the stamp and
> the emission beside it rather than introducing a second notion of "turn start".

Each rejected placement fails in a named direction:

- **At `sendPrompt` entry (`:71`) — wrong, and expensive.** A queued prompt blocks in
  `acquirePromptTurn` (`:99`) while the *previous* turn is still running. Stamping there would
  advance the baseline mid-previous-turn, so a workload launched by that turn would sort
  *before* the baseline and the probe would answer `settled` — the expensive direction D5's
  inclusive comparison exists to avoid.
- **At the outer callers — wrong, and incomplete.** `sendPrompt` has **three** callers, not the
  two an earlier draft enumerated: `Prompt` (`:31`, operator), `PromptSteer` (`:60`, mid-turn
  steering), and `fireWakeup` (`:434`, synthetic wakeup). Placing the emission at the named two
  silently omits steering, which is a fully shipped path
  (`manager_interaction.go:127` → `session.go:1330` → `agent.go:787` → `PromptSteer`).
- **Before the drop check (`:120`) — wrong.** A pinned wakeup whose session changed while it
  queued returns `nil` without ever dispatching. It must emit **no** `turn_started`; nothing
  reached the agent, so no turn began.

**A steer emits `turn_started` like any other dispatch, and that is deliberate.** Per
`PromptSteer`'s own contract, whether the agent folds a steer into the running turn or runs it
as the next turn "is the agent's decision and is not advertised over the protocol, so both
outcomes must be correct" (`adapter_prompt.go:41-52`). The protocol therefore cannot tell us
which happened, so the uniform rule at the single funnel is the only implementable one. The
cost is bounded and falls in the safe direction: a steer is delivered while the session is
generating, so the session-state term is already false and no parked affordance is being
cleared; the only effect is that a workload launched before the steer stops counting as
in-turn, which can retire the affordance one turn early but can never invent one. A rule that
tried to exempt steering would need a signal the protocol does not carry.

Four further properties are contract:

- **It is emitted at the same point agentctl stamps its own recorded turn start**, so the
  attestation-clearing boundary and the probe's baseline cannot drift apart. That is the whole
  reason D3 names one boundary rather than two.
- **It is emitted on the human and the synthetic path alike.** `sendPrompt` distinguishes them
  via `humanPrompt := expectSession == ""` (`adapter_prompt.go:79`); this event does **not**.
  AC-41 asserts both halves.
- **Repeats are idempotent.** Clearing `observed_detached` is a set-to-false, so two
  `turn_started` events for one session leave the same state as one. An event for a session the
  backend has no `parkedState` for is ignored, not an error, and creates no entry.
- **A backend that never receives it degrades to today's behaviour**, not to a wrong answer:
  `observed_detached` stays set from the prior turn, the probe keeps comparing against an
  earlier baseline, and the bias is toward `live` — the cheap direction D3 already specifies
  for a continuation that produces no `session/prompt` at all.

It carries no session content and rides the existing stream, so it grants no new access.
AC-41a observes the event and its three paths; AC-41b observes the emission point by pinning
the two placements it excludes.

### Probe port (backend)

**PROMOTED FROM *Notes for implementation* 2026-08-09.** The injectable probe seam was named
only in an implementation note, but three acceptance criteria cannot be satisfied without it —
AC-62 and AC-68 are transition assertions that need the probe to change value mid-test, and
AC-73 is written against "three consecutive `live` samples then `settled`" precisely so it runs
without a real twelve-minute wait. A seam that load-bearing is contract, not advice.

```
BackgroundProbe
  Probe(ctx, sessionID) (live | settled | unknown, error)
```

- The parked projection and the sampling loop depend on **this port**, never on the agentctl
  `Client` directly. The production implementation is `Client.ProbeBackgroundWorkloads`
  (*Probe transport*); a test implementation returns a scripted sequence of the three literals.
- The port's contract is the probe's contract: exactly three result values, and **every** error
  resolves to `unknown` (*Probe transport*, failure table).
- **A non-nil error resolves to `unknown` regardless of the value returned beside it**, and the
  caller MUST check the error first. Saying only that such a pair is "outside the contract"
  leaves the caller's behaviour undefined at exactly the point where getting it wrong parks a
  session on a failed sample — the one direction this spec never takes. The caller therefore
  never reads the value when the error is non-nil, and a result outside the three literals is
  `unknown` even with a nil error.
- The port is **not** responsible for the probe budget. The caller applies it as a
  `context.WithTimeout` (D2); a port implementation neither imposes nor extends a deadline of
  its own.
- Because the projection depends only on the port, it is buildable and fully testable before
  the real process-tree probe exists — the substitution the plan relies on to run the backend
  and agentctl work in parallel.

### The launch-recogniser seam

Detached-launch attestation is **per agent, behind a named seam**. This replaces the earlier
contract, which excluded multi-vendor attestation outright; the exclusion is withdrawn by human
decision on 2026-08-09 and the staged shape below is contract.

```
BackgroundLaunchRecognizer
  AgentID() string
  RecognizesDetachedLaunch(payload *streams.NormalizedPayload) bool
```

- Recognisers are held in a **registry keyed by agent ID**, exposing a public registration
  function. At ship time exactly **one** is registered: Claude, whose implementation is today's
  condition — `payload.ShellExec() != nil && payload.ShellExec().Background`
  (`transport/acp/normalize.go:305`).
- An agent with **no registered recogniser** produces no attestation. It is never probed
  (AC-54, AC-40a) and never parked (AC-37). This is the guarantee that keeps every other ACP
  agent in the registry behaving exactly as it does today, and it must survive any future
  registration.
- **Adding a vendor is registering a recogniser and nothing else.** It MUST NOT require
  changing the probe, the parked projection, or any icon call site. AC-69 asserts this by
  registering a second recogniser **through the public registration API, from test code only**.
- The registration function MUST be reachable from a test package without importing anything
  under the probe, projection, or rendering paths. That reachability is what makes AC-69
  mechanically checkable rather than a claim about a diff.
- The seam is the natural home for a future PID-carrying attestation. §I records that
  `BackgroundWorkPayload` carries no PID today; if a vendor can supply one, it enters here and
  narrows the probe from a time-scoped predicate to an exact lookup **without** changing the
  probe's contract or its three result values.

Why this is a small change and not a rewrite: per §L the probe is already entirely
provider-agnostic — it walks a process tree that exists for every executor and every agent —
and the Claude-specific part is one predicate over one payload.

### Background-workload liveness probe (agentctl)

An internal seam, stated here because its outcomes are contractual and a conformance test must
be able to drive them:

```
ProbeBackgroundWorkloads(sessionID) -> live | settled | unknown
```

The probe enumerates the **transitive descendant set** of the agent process and applies a
**start-time predicate**:

- **`live`** — the descendant set was enumerated, and it contains at least one non-zombie
  process whose **start time is at or after the current turn's start time** as recorded by
  agentctl.
- **`settled`** — the descendant set was enumerated and contains no such process.
- **`unknown`** — the set could not be enumerated with start times on this platform; or
  agentctl holds no recorded turn-start for the session; or the agent process is gone.

**Process-group membership MUST NOT be used, in any form, as the liveness predicate.** §L
measures why: the group permanently contains the bridge, the CLI, and any stdio MCP servers, so
membership is always non-empty; and a backgrounded shell is placed in its own process group by
the CLI, so membership excludes the very process the probe exists to find. An implementation
that samples the group passes neither AC-70 nor AC-71.

**Turn start is recorded by agentctl, in agentctl's own clock.** agentctl stamps the current
turn's start at `beginPromptTurn` (`adapter_prompt.go:140`) — the same instant that emits
`turn_started`, after the prompt gate and after the pinned-session drop check, and therefore on
all three of `sendPrompt`'s callers including the synthetic `ScheduleWakeup` one (*API surface →
Turn-boundary stream event*, which names the site and the placements it rejects) — and clears it
on session teardown. Both sides of the comparison are
therefore read on the **same host and the same clock**, so no cross-process skew exists and no
timestamp travels on the wire. If no turn start is recorded (agentctl restarted mid-turn), the
result is `unknown`.

**Start-time source and resolution — named per platform, because they are not interchangeable.**
An earlier revision offered two Darwin sources as if equivalent. They are not, and the
difference breaks the predicate's own stated failure direction:

| Platform | Required source | Resolution | Result |
|---|---|---|---|
| Linux | `/proc/<pid>/stat` — field 4 `ppid`, field 22 `starttime` (clock ticks since boot, resolved against `/proc/uptime` or `sysconf(_SC_CLK_TCK)`) | ≥ 10 ms | `live` / `settled` |
| Darwin/BSD | `sysctl KERN_PROC_ALL` → `kinfo_proc.kp_proc.p_starttime`, a `timeval` | 1 µs | `live` / `settled` |
| Windows | Not implemented | — | always `unknown` |

**`ps -eo lstart` MUST NOT be used as the Darwin source**, even though §L's exploratory
measurement used `ps`. `lstart` renders a whole-second timestamp, while the recorded turn start
is a Go `time.Time` at nanosecond resolution. A workload spawned 400 ms after `session/prompt`
in the same wall-clock second would report a start time strictly *before* the turn start and
the probe would answer `settled` — the **expensive** direction, and precisely the one D5
promises the inclusive comparison avoids. AC-71 and AC-72 would become timing-dependent rather
than deterministic. Verified greenfield: `apps/backend/go.mod` carries no process-enumeration
dependency, so the builder is choosing this, not inheriting it.

**Comparison rule when the source is coarser than the turn stamp.** The recorded turn start is
**truncated down** to the enumeration source's resolution before comparing, so a process that
started in the same source-resolution tick as the turn counts as in-turn. Truncating the turn
start (rather than rounding the process start) keeps the error in the `live` direction on every
platform. AC-80 pins this.

**Determinism of the predicate**, so a builder invents none of it:

- A process is identified by the pair **(pid, start time)**, never by bare pid. PID reuse inside
  one turn would otherwise let a recycled pid inherit the wrong verdict, and it would bias
  toward `settled` — the expensive direction.
- The comparison is **inclusive** (`start_time >= truncated_turn_start`), which fails toward
  `live` — the cheap direction.
- **Zombies are excluded**, on every platform. A reaped-but-unwaited child is not work.
- The enumeration is **one snapshot**. A process that exits while the walk is in progress is
  treated as absent; the walk is never restarted, and a partial walk that cannot be completed
  yields `unknown` rather than a shortened set.
- No ordering rule is needed: the result is an existence predicate over the set, not a selection
  from it.
- The agent process itself is never a member of its own descendant set and is never counted.

Only `live` may park. Both `settled` and `unknown` render exactly as today, and `unknown` MUST
NOT be recorded, serialized, or rendered as `settled`.

### Probe transport (backend ↔ agentctl)

The probe **executes in agentctl** (§I: only agentctl owns the agent subprocess), while
`parkedState` lives in **backend** memory and D2 needs the first sample synchronously during
backend turn-settle. That is a cross-process call and it needs a named transport. There is no
existing seam to reuse: the backend→agentctl `Client`
(`internal/agent/runtime/agentctl/agent.go`) exposes `Initialize`, `NewSession`, `Prompt`,
`Cancel`, `GetAgentStderr`, `RespondToPermission` and ~15 more, and **none of them samples
process liveness**.

It reuses the same WS-action mechanism as `agent.permissions.respond` — no new listener, no new
port, no HTTP route:

| | |
|---|---|
| **Action** | `agent.background.probe` |
| **Direction** | backend → agentctl, over the already-open agent stream |
| **Request** | `{ "session_id": "…" }` |
| **Response** | `{ "result": "live" \| "settled" \| "unknown" }` |
| **Dispatch** | a new `case` in the `server/api/agent.go` action switch |
| **Backend entry point** | `Client.ProbeBackgroundWorkloads(ctx, sessionID) (ProbeResult, error)` via `sendStreamRequest` |

The request carries no timestamp: agentctl holds the turn start itself, which is what removes
the clock-skew question entirely.

`sendStreamRequest` takes its deadline from the caller's `ctx`, so the probe budget is applied
by the backend as a `context.WithTimeout` around the call. No timeout is baked into the
transport.

**Every failure of this call resolves to `unknown`**, never to `settled` and never to `live`.
Exhaustive by construction — the backend maps the union of outcomes, not a list of anticipated
errors:

| Outcome | Result |
|---|---|
| `ErrAgentStreamNotConnected` — agentctl gone, crashed, or not yet attached | `unknown` |
| context deadline exceeded (probe budget elapsed, D2) | `unknown` |
| WS error frame, including `ErrorCodeUnknownAction` from an agentctl too old to know the action | `unknown` |
| response body absent, unparseable, or carrying a `result` outside the three literals | `unknown` |
| any other transport or marshalling error | `unknown` |

The `ErrorCodeUnknownAction` row matters for mixed-version deployments: an older agentctl
answers an unknown action with an error rather than a hang, so a version-skewed pair degrades to
exactly today's behaviour instead of stalling turn settlement.

### Timing, configuration, and who owns the sampling loop

Two durations govern this spec. Each gets a **named key, a default, and an explicit statement
of which process reads it** — the last column exists because both are read by the backend while
the convention they follow lives in agentctl's config, and an operator who sets a backend key in
agentctl's environment would otherwise have it silently ignored in exactly the executors this
feature targets (Docker/SSH/Sprites, where the two run in different containers).

| Constant | Env var | Default | **Read by** | `0` means |
|---|---|---|---|---|
| **Probe budget** — D2's bound on the synchronous first sample | `KANDEV_PARKED_PROBE_BUDGET` | `250ms` | **backend** | **rejected — see below** |
| **Sampling interval** — how often a parked session is re-probed | `KANDEV_PARKED_PROBE_INTERVAL` | `30s` | **backend** | disables periodic sampling; the projection then never clears via the probe (see below) |

They follow the existing `getEnvDuration("KANDEV_ACP_IDLE_TIMEOUT", time.Hour)` convention
(`internal/agentctl/server/config/config.go:285`). These are plain env-sourced config, **not**
runtime feature toggles: they are operational tuning, not a release gate, so they do not go in
`runtimeflags/registry.go`.

The two notification windows the combined spec defined — `KANDEV_PARKED_DEFER_BACKSTOP` and
`KANDEV_PARKED_SETTLE_CONFIRM` — are **not read by this spec** and have moved to the sibling
spec with the deferral they bound.

**`KANDEV_PARKED_PROBE_BUDGET = 0` is REJECTED, and this is a deliberate exception to the
"`0` disables" idiom.** Every other `0` in Kandev's duration config disables a behaviour. Here
it would *enable* an unbounded blocking call: the transport bakes in no timeout, so with no
budget the synchronous first sample has no stated bound at all and a hung agentctl would wedge
turn settlement for every attested turn. That is the one place in this design where a failure is
not fail-open. So: a non-positive value is **rejected at config load**, logged at warn, and the
default is used. AC-81 observes it. The deviation from the idiom is called out here precisely
because a builder following the convention blindly would produce the hazard.

Defaults chosen, with the reasoning recorded so it is reviewable rather than arbitrary:

- **Probe budget `250ms`.** This sits on the synchronous path of *every* attested turn end, so
  it is a latency budget first and a correctness knob second. The sample is a local process-table
  walk reached through an already-open WS connection; 250ms is far above its expected cost and
  still below human perception of turn-end latency. Exceeding it yields `unknown`, which renders
  as today — the safe direction. No probe is taken at all when `observed_detached` is false (D3),
  so the common turn end pays nothing.
- **Sampling interval `30s`.** This bounds how long the affordance keeps showing after background
  work really finishes. 30s keeps that lag under a minute while costing one cheap local walk per
  *parked* session per half-minute. Only parked sessions are sampled (AC-54), so fleet cost scales
  with parked sessions, not all sessions.

**`KANDEV_PARKED_PROBE_INTERVAL = 0` — what it means for the projection.** No periodic sample is
ever taken, so a session parked from the synchronous first sample **stays** parked until it
leaves `WAITING_FOR_INPUT`. That is accepted and is stated rather than left to be discovered: at
`0` the affordance is explicitly best-effort, the operator has opted out of re-sampling, and no
notification is affected because this spec withholds none. A session that never leaves
`WAITING_FOR_INPUT` keeps the affordance indefinitely. AC-74 observes this. Operators who do not
want that should not set the interval to `0`.

**The sampling loop is owned by the BACKEND**, not agentctl. The backend holds `parkedState` and
knows the session's lifecycle state; agentctl only answers `agent.background.probe` when asked
and keeps no timer. One loop serves all parked sessions. Its lifecycle is a closed set, so no
session is sampled forever:

- **Starts** when a session enters the parked condition (`observed_detached`, a synchronous first
  sample of `live`, and state `WAITING_FOR_INPUT`).
- **Samples** each parked session every sampling interval. Concurrent samples for one session are
  serialized (D6).
- **Stops** for a session on the first of: the probe returns anything other than `live`; the
  session leaves `WAITING_FOR_INPUT`; the session is stopped, deleted, or its execution ends; or
  the backend shuts down.

### Runtime flag

**This feature ships unflagged.** It changes no admission rule, adds no state, alters no
notification, and fails closed to today's rendering on every indeterminate input. It neither
reads nor modifies `features.claudeBackgroundPromptHandoff`, which stays off.

## State machine

`TaskSessionState` is unchanged. The parked projection is orthogonal:

| Session state | `parked_on_background_work` |
|---|---|
| `RUNNING` | always `false` |
| `WAITING_FOR_INPUT`, recogniser attested a detached launch, probe `live` | `true` |
| `WAITING_FOR_INPUT`, probe `settled` or `unknown` | `false` |
| `WAITING_FOR_INPUT`, no attested detached launch | `false` |
| `COMPLETED`, `FAILED`, `CANCELLED`, `IDLE`, `CREATED`, `STARTING` | `false` |

There is no deferral state machine in this spec. The five-exit table the combined spec carried
governed the *notification*, and it has moved to the sibling spec in full.

**What clears the projection**, exhaustively — it is the negation of the three-term conjunction
and nothing else:

| Cause | Which term goes false |
|---|---|
| the probe returns `settled` or `unknown` at a sample | term 2 |
| the agent resumes itself, or the operator's prompt is admitted | term 3 |
| the session is stopped, deleted, or its execution ends | term 3 |
| a new turn starts (D3 clears the attestation) | term 1 |

**A queued-but-unadmitted operator prompt does NOT clear the projection.** All three terms
remain true: the session is still `WAITING_FOR_INPUT`, the attestation still describes the turn
that settled, and the workload is still alive. This is deliberate and is the one place where the
projection and the sibling spec's deferral intentionally diverge — the deferral asks *"does a
human still need to be told?"* and a queued prompt answers no, while the projection asks *"is a
machine still working?"* and a queued prompt says nothing about that. Making the affordance
vanish the instant an operator types would hide the very fact the operator needs. AC-75 observes
it. The sibling spec records the same divergence from its side.

## Permissions

Scoped by the existing rules; grants no new access. The parked projection carries a boolean and
no task content, and rides the existing workspace-scoped broadcast paths. The probe action
reaches agentctl only through `lifecycle.Manager`, which applies the session-access guard used by
`RespondToPermission`. Notification bodies and delivery are untouched.

## Failure modes

| Condition | Behaviour |
|---|---|
| Probe reports `unknown` | Never parked. Renders exactly as today. |
| Probe errors or panics | Treated as `unknown`. |
| Platform cannot enumerate descendants with start times (Windows) | `unknown`, always. Never `settled`, never `live`. |
| agentctl holds no recorded turn start (restarted mid-turn) | `unknown`. |
| The agent process has exited | `unknown`, not `settled` — Kandev has lost the ability to observe rather than observed completion. |
| agentctl crashes, disconnects, or is version-skewed while a session is parked | The probe call fails → `unknown` → the session un-parks and the affordance clears. |
| Probe reports `live` but the workload is a leaked orphan | The affordance persists until the session leaves `WAITING_FOR_INPUT`. Accepted: it costs a stale spinner, not a stuck notification, because this spec withholds nothing. |
| A long-lived process unrelated to the workload starts mid-turn (e.g. a lazily-connected stdio MCP server) | Counted as `live`. This is a **false "still busy"**, the cheap direction. Preferred over any command-name heuristic, which would be fragile and vendor-specific. §L measures that this is a real, common case, and **AC-70a observes it** rather than leaving it as prose. |
| Detached launch observed but the agent has no registered recogniser | `observed_detached` is false; behaviour unchanged, and no probe is taken. |
| Backend restarts while a session is parked | The projection is not reconstructed; the session reads as not parked. The new process's `parked_epoch` is strictly higher, so clients accept the reset (AC-77). |
| `KANDEV_PARKED_PROBE_BUDGET` set to `0` or negative | Rejected at config load, warn-logged, default used (AC-81). |

## Persistence guarantees

Nothing added by this feature survives a restart of either process.

- `parked_on_background_work` and its revisions are in backend memory only. After a backend
  restart a previously parked session reads as not parked, and `parked_epoch` changes.
- agentctl's recorded turn start is in agentctl memory only and dies with the process.
- No new column, table, or migration.

## Determinism, concurrency, and boundaries

Everything here is a decision this spec makes so that a builder does not have to invent one.
Where a decision is genuinely excluded it is named under *Out of scope*.

**Identifier mapping.** Four review rounds cite the combined spec's numbering. The mapping is:
**D1**←D6, **D2**←D7, **D3**←D8, **D9**←D10. D5–D8 were introduced in round 3. **D4** (deferral:
at most one, single-flighted, never queued) governed the notification and has moved to the
sibling spec; its identifier is left vacant here rather than reused, so a citation to "D4" is
never ambiguous. The combined spec's D1–D5 were elicitation-only and moved to
[`acp-elicitation`](../acp-elicitation/spec.md).

### D1 — The parked projection carries a revision and an epoch, and why

`parked_on_background_work` is a transient session-scoped boolean projected onto REST, boot, and
WS — structurally identical to `CancellationPending`, which the backend already guards with a
process-local revision (`orchestrator.Service.CancellationPendingSnapshot`,
`task_operations.go:4478`). Without one, a delayed `session.state_changed` can overwrite a newer
`session.activity_changed` and strand the wrong value on the card.

- Value, a monotonically increasing per-session `uint64`, and `parked_epoch` are read **in one
  critical section** and travel together on every carrier.
- The revision increments on each transition of the boolean, not on each read.
- A consumer compares the ordered pair `(parked_epoch, revision)` and **discards a strictly
  lower pair for that session**. A higher epoch always wins.
- Publication happens only on change.
- Never persisted; the epoch is the backend process's start time and changes on every restart.

**Where the precedent stops.** `CancellationPending` is stamped only onto `TaskSessionDTO` /
`TaskSessionSummaryDTO` (`internal/task/dto/cancellation_pending.go`) and has **no task-level
projection at all**, so it supplies no rule for the task carriers this feature also writes to,
and it has no epoch because it is never compared across a restart. The task-level counter is
specified under *Data model → Task-level projection*, including why the maximum-of-members rule
it replaces is defective; the epoch is specified under *Data model → Revision epoch*.

### D2 — When the first sample is taken

Deciding whether to park requires knowing the probe's answer, so the first sample is taken
**synchronously during turn-settle handling**, bounded by `KANDEV_PARKED_PROBE_BUDGET` applied as
a `context.WithTimeout` by the backend. If the sample cannot complete within it, the result is
`unknown`, which renders as today. The probe is never allowed to delay a turn's settlement beyond
that budget, and the budget can never be disabled (see *Timing*).

Taking it synchronously rather than on the next loop tick is what makes the affordance appear at
settle rather than up to one sampling interval later — the whole point being that an operator
looking at the board immediately after a turn ends sees the truth.

The sample is taken **only** when `observed_detached` is true for the settling turn (D3), so the
common turn end pays nothing (AC-40a). Concurrent samples for one session are serialized; a
sample that completes carrying a revision older than the current one is discarded under D1.

### D3 — The turn boundary, defined once for both processes

`observed_detached` describes **the turn that settled**, not the session's history. It is set
when a registered recogniser attests a detached launch, and cleared at the start of every turn.
An earlier revision left "the start of every turn" to mean a backend event while the probe's
baseline keyed off an agentctl event, and asserted the two coincided "by construction". §N shows
they do not.

**So the feature has ONE turn boundary: agentctl's `session/prompt` dispatch.**

- agentctl stamps its recorded turn start on **every** `session/prompt` dispatch that reaches the
  agent, including the synthetic one a `ScheduleWakeup` self-resume issues (§N) and the mid-turn
  steering one. The stamp and the event share one call site, `beginPromptTurn`
  (`adapter_prompt.go:140`); *API surface → Turn-boundary stream event* names it and the three
  placements it rejects. AC-41b pins it.
- `observed_detached` MUST be cleared on **that same boundary** — i.e. on the `turn_started`
  event that dispatch emits (*API surface → Turn-boundary stream event*), not on the backend's
  own prompt-admission path (`startTurnForSession`, `service.go:1278`), which a synthetic prompt
  does not reach. §N verifies that no pre-existing event carries this boundary, which is why
  this spec publishes one rather than subscribing to one. AC-41, AC-41a and AC-79 observe the
  halves.
- **If a continuation produces no `session/prompt` at all**, neither side advances: the baseline
  stays at the last dispatch and the attestation is not cleared. That is deliberate and is the
  cheap direction — the probe keeps comparing against an *earlier* baseline, so more processes
  count as in-turn and the answer biases toward `live`, i.e. toward a stale affordance rather
  than a missed one. It is stated so a builder does not "fix" it into the expensive direction.

This pairs exactly with the probe's start-time predicate: attestation says *a detached launch
happened during this turn*, and the probe asks *is anything started during this turn still
alive*. Both are scoped to the same dispatch, which is what lets the probe work without a PID.

**Ordering against turn-settle.** The attestation and the turn-completion event arrive on the
**same ordered stream consumer** — `handleAgentStreamEvent`
(`event_handlers_streaming.go:24`) dispatches both `handleToolCallEvent` / `trackBackgroundToolUpdate`
(which register the background work) and `publishAgentTurnComplete` (`:436`). `observed_detached`
MUST therefore be applied on that consumer, so a detached launch emitted before the turn-end
frame is guaranteed visible when the settle handler reads the flag. An implementation that
applies the attestation on a different goroutine or a separate queue reintroduces the race in
which a shell backgrounded late in a turn is invisible at settle and the card silently never
parks. AC-79 observes it.

### D5 — Process identity and the start-time comparison

- A process is identified by **(pid, start time)**, never bare pid.
- The comparison against the truncated turn start is **inclusive** (`>=`).
- The turn start is truncated down to the enumeration source's resolution before comparing, so
  the error always falls toward `live` (see *Start-time source and resolution*).
- Zombies are excluded on every platform.
- The enumeration is one snapshot; a process that exits mid-walk is absent; a walk that cannot be
  completed yields `unknown`, never a shortened set.
- Both timestamps are read on agentctl's host in agentctl's clock. No timestamp crosses a process
  boundary.

### D6 — Two callers, same session

- **Two concurrent probes for one session** are serialized by agentctl; the second observes the
  first's snapshot only if it starts after it completes, otherwise it takes its own. Both return
  a well-defined tri-state; neither blocks the other beyond the probe budget.
- **A probe racing a turn start** — agentctl's recorded turn start changes under a probe in
  flight — resolves against the turn start read at the **beginning** of that probe's enumeration,
  so one probe never mixes two turns' baselines.
- **Two sessions on one agentctl** are independent: each has its own recorded turn start, and the
  descendant walk is rooted at the agent process serving that session. Per the ACP adapter's
  single `sessionID` field and `NewSession`'s teardown of prior per-session state, one agent
  process serves one active session at a time.

### D7 — The recogniser registry

- Keyed by agent ID; **at most one** recogniser per agent ID. Registering a second for the same ID
  is a programming error, not a runtime merge.
- Lookup miss ⇒ no attestation, no probe, no parking. This is the default for every agent.
- The registry is fixed at process start; recognisers are not added or removed at runtime, except
  that the registration function is reachable from test code so AC-69 can exercise the seam.
- A recogniser that panics is treated as "did not recognise" — fail closed to today's behaviour.

### D8 — What clears the projection

`parked` is a three-term conjunction (*Data model*), and a transition of **any** term publishes.
In particular, a self-resume or an admitted prompt clears the affordance through the
**session-state** term, not through the sample: `last_sample` may still read `live` at that moment
and is not re-sampled, because AC-53 stops the loop when the session leaves `WAITING_FOR_INPUT`.
AC-68 observes this directly — it is the case an earlier revision left uncovered, and the one that
would otherwise leave a card spinning while the session is actively `RUNNING`.

### D9 — Defaults and boundary values

| Field / input | Default / boundary behaviour |
|---|---|
| `parked_on_background_work` | `false` when absent; absent and `false` are equivalent |
| parked revision (session) | `0` for a session with no recorded transition |
| parked revision (task) | own per-task counter; `0` for a task with no sessions or no recorded transition. **Not** the maximum over sessions |
| `parked_epoch` | the backend process's start time in Unix nanoseconds; `0` only from a peer that does not implement this spec, which sorts below every real epoch |
| probe with no sample yet | `unknown` until the first sample completes |
| probe budget | `250ms` (`KANDEV_PARKED_PROBE_BUDGET`); **`0` or negative is rejected**, default used |
| probe sampling interval | `30s` (`KANDEV_PARKED_PROBE_INTERVAL`); `0` disables periodic sampling, and the projection then clears only via the session-state or attestation terms |
| a queued-but-unadmitted operator prompt | the projection is **unchanged** — all three terms still hold |
| agent with no registered recogniser | never attested, never probed, never parked |
| process start time exactly equal to the truncated turn start | counts as **in-turn** (inclusive comparison) |
| agent process exited | `unknown`, never `settled` |
| notification behaviour | **unchanged in every case** — this spec never withholds, defers, or reorders one |

## Scenarios

- **AC-21** — **GIVEN** a session that settled to `WAITING_FOR_INPUT` after a turn in which a
  registered recogniser attested a `Detached=true` background-shell launch, and the probe reports
  `live`, **WHEN** the session DTO is serialized, **THEN** `parked_on_background_work` is `true`.
- **AC-22** — **GIVEN** the same session, **WHEN** the task DTO is serialized, **THEN** the task's
  `parked_on_background_work` is `true` and its `foreground_activity` is unchanged from today's
  value.
- **AC-23** — **GIVEN** a parked session, **WHEN** its task row renders in the **sidebar task
  list** (`components/task/task-item.tsx`), **THEN** `data-testid="task-state-background-running"`
  is present and neither `data-testid="task-state-waiting-for-input"` nor
  `data-testid="task-state-turn-finished"` is. *(The `turn-finished` clause is new: §G measures
  that a `state=REVIEW` task renders that id today, so asserting only the absence of
  `waiting-for-input` would have been vacuous for the originating ticket's own tasks.)*
- **AC-24** — **GIVEN** a session that settled with no attested detached launch, **WHEN** its DTO
  is serialized, **THEN** `parked_on_background_work` is `false` regardless of what the probe
  reports.
- **AC-25** — **GIVEN** a session with an attested detached launch and a probe result of `settled`,
  **WHEN** its DTO is serialized, **THEN** `parked_on_background_work` is `false`.
- **AC-26** — **GIVEN** a session with an attested detached launch and a probe result of `unknown`,
  **WHEN** its DTO is serialized, **THEN** `parked_on_background_work` is `false`.
- **AC-27** — **GIVEN** a platform that cannot enumerate the agent's descendants with their start
  times, **WHEN** the probe is taken for a session with an attested detached launch, **THEN** the
  probe's returned value is `unknown` — never `live`, never `settled` — and the session DTO
  reports `parked_on_background_work: false`. *(This criterion previously also asserted that the
  turn-finished notification was delivered rather than withheld. That clause moved to the sibling
  spec with the deferral; in this spec no notification is ever withheld, which AC-76 asserts once
  for all cases.)*
- **AC-27a** — **GIVEN** an agent whose only in-turn descendant is a zombie, **WHEN** the probe is
  taken, **THEN** it reports `settled`.
- **AC-27b** — **GIVEN** an agent with exactly one live descendant, **and that descendant's process
  start time is BEFORE the current turn's recorded start**, **WHEN** the probe is taken, **THEN**
  it reports `settled`. *(This criterion previously asserted `live` for any live child. It was
  corrected on 2026-08-09 under the human's authorisation to change the contract: §L measures that
  every idle Claude session has such children, so the old reading made `live` permanent. Upheld by
  Spec Review round 4.)*
- **AC-34** — **GIVEN** a parked session that also has a pending clarification, **WHEN** its task
  row renders, **THEN** `task-state-waiting-for-input` is present and
  `task-state-background-running` is not; and **GIVEN** the same session with a pending permission
  instead, **THEN** `task-state-pending-permission` is present and `task-state-background-running`
  is not.
- **AC-35** — **GIVEN** `features.claudeBackgroundPromptHandoff` is off in every profile, **WHEN** a
  session is parked and the operator submits a prompt, **THEN** the prompt follows exactly the
  admission path it follows today for that session state; **and** an architecture test asserts that
  no package on the parked-projection, probe, or rendering path references the
  `claudeBackgroundPromptHandoff` flag key or its accessor symbol. *(The second clause previously
  read "the parked projection was produced without reading that flag", which asserts a property of
  the diff and is not observable from a test process. Restated on 2026-08-09 as a named
  architecture-test assertion. The claim is unchanged; only its form is. Spec Review round 5 should
  judge this rather than assume it.)*
- **AC-36** — **GIVEN** a parked session, **WHEN** the backend restarts and the boot payload is
  built, **THEN** `parked_on_background_work` is `false`.
- **AC-37** — **GIVEN** an ACP agent with **no registered launch recogniser**, **WHEN** it settles a
  session after backgrounding work, **THEN** `parked_on_background_work` is `false` and every icon
  surface renders exactly as it does today.
- **AC-38** — **GIVEN** a session whose parked value transitions `false → true → false`, **WHEN**
  each carrier is serialized, **THEN** the boolean, its revision and `parked_epoch` are read from
  one critical section, the revision strictly increases across the two transitions, and re-deriving
  an unchanged value publishes nothing (D1).
- **AC-39** — **GIVEN** a consumer that has applied `(epoch E, revision N)` for a session, **WHEN**
  an update carrying `(E, N-1)` arrives for that session, **THEN** the consumer discards it and the
  displayed value is unchanged (D1).
- **AC-40** — **GIVEN** a turn settling on a session whose probe cannot complete a sample within
  `KANDEV_PARKED_PROBE_BUDGET`, **WHEN** the projection is computed, **THEN** the result is treated
  as `unknown`, the session is not parked, and turn settlement is not delayed beyond that budget
  (D2).
- **AC-40a** — **GIVEN** a turn settling on a session with **no** attested detached launch, **WHEN**
  the projection is computed, **THEN** no probe is taken at all and turn settlement incurs no probe
  latency (D2, D3).
- **AC-41** — **GIVEN** a detached launch attested during turn N, **WHEN** turn N+1 settles without
  its own attested detached launch, **THEN** `parked_on_background_work` is `false` for turn N+1;
  and **GIVEN** turn N+1 was begun by a **synthetic** `session/prompt` from a `ScheduleWakeup`
  self-resume rather than an operator prompt, **THEN** the same holds — the attestation is cleared
  on that dispatch too (D3, §N).
- **AC-41a** — **GIVEN** a session, **WHEN** agentctl dispatches `session/prompt` for it, **THEN**
  a `turn_started` event carrying that `session_id` and **no timestamp** is emitted on the agent
  stream; and it is emitted on **all three** of `sendPrompt`'s callers — the operator path
  (`Prompt`, `adapter_prompt.go:31`), the mid-turn steering path (`PromptSteer`, `:60`) and the
  synthetic `ScheduleWakeup` path (`fireWakeup`, `:434`) — which `sendPrompt` otherwise
  distinguishes via `humanPrompt` (`:79`) and the `steer` argument; and **GIVEN** the backend
  receives it, **THEN** `observed_detached` is cleared on the same ordered consumer that applies
  the attestation (`event_handlers_streaming.go:24`), and a second `turn_started` for the same
  session leaves the same state as the first. *(Added 2026-08-09 with the event itself. §N
  verifies no pre-existing stream event marks a turn start, so D3's clearing rule had no carrier
  and this criterion is what stops a builder inventing one. The three-caller clause was added
  after reading the tree: an earlier draft named two paths and would have silently omitted
  steering, which is a shipped path.)*
- **AC-41b** — **GIVEN** a queued operator prompt that is blocked on the prompt gate
  (`acquirePromptTurn`, `adapter_prompt.go:99`) while the previous turn is still running,
  **WHEN** it is waiting, **THEN** no `turn_started` has been emitted for it and agentctl's
  recorded turn start still holds the previous turn's value — so a workload launched by the
  running turn still counts as in-turn and the probe reports `live`; and **THEN** both are
  updated only once the prompt reaches `beginPromptTurn` (`:140`). And **GIVEN** a pinned
  `ScheduleWakeup` prompt whose session changed while it queued, so it is dropped at `:120` and
  returns without dispatching, **WHEN** the drop occurs, **THEN** **no** `turn_started` is
  emitted and the recorded turn start is unchanged. Together these pin the emission point
  between the gate and the dispatch; an implementation that emits at `sendPrompt` entry fails
  the first half and one that emits at the outer callers fails the second.
- **AC-45** — **GIVEN** the backend needs a liveness sample for a session, **WHEN** it takes one,
  **THEN** the request travels as the WebSocket action `agent.background.probe` on the existing
  agent stream carrying `session_id`, the request carries **no timestamp**, and the response
  `result` is one of exactly `live`, `settled`, `unknown`.
- **AC-46** — **GIVEN** each of these five conditions in turn — the agent stream is disconnected
  (`ErrAgentStreamNotConnected`); the probe budget elapses; agentctl replies
  `ErrorCodeUnknownAction`; the response body is unparseable; the response carries a `result`
  outside the three literals — **WHEN** the backend resolves the probe, **THEN** in **every** case
  the result is `unknown` and the session reports `parked_on_background_work: false`.
- **AC-49** — **GIVEN** a task with two sessions, where session S1 has recorded two transitions
  (revision `2`, currently `false`) and session S2 has recorded none (revision `0`, `false`),
  **WHEN** S2 transitions to parked, **THEN** the task's `parked_on_background_work` becomes `true`
  **and its `parked_revision` is strictly greater than the value the task carried immediately
  before** — proving the task counter is independent of the member sessions' revisions; and
  **WHEN** a task-level update carrying a lower `parked_revision` at the same `parked_epoch`
  arrives at a consumer that has already applied the higher one, **THEN** it is discarded and the
  displayed value is unchanged.
- **AC-50** — **GIVEN** a task with no sessions, or whose sessions have recorded no transition,
  **WHEN** the task DTO is serialized, **THEN** `parked_on_background_work` is `false` and
  `parked_revision` is `0`.
- **AC-51** — **GIVEN** a parked session, **WHEN** the session switcher
  (`components/task/sessions-dropdown.tsx:475`) renders it, **THEN** the background icon is shown
  rather than the `WAITING_FOR_INPUT` question mark; and **GIVEN** the same session additionally
  has a pending clarification, **THEN** the question mark wins.
- **AC-52** — **GIVEN** a session that is **not** parked, **WHEN** any of the three
  `getSessionStateIcon` call sites (`sessions-dropdown.tsx:475`, `session-reopen-menu.tsx:204`,
  `mobile/mobile-sessions-section.tsx:132`) renders it, **THEN** the icon is identical to today's
  for **the matrix already enumerated in `apps/web/lib/ui/state-icons.test.tsx`** (session-icon
  describe blocks), extended with the new parameter pinned `false` — the new parameter defaults to
  `false` and changes nothing on its own.
- **AC-53** — **GIVEN** a session that becomes parked, **WHEN** the sampling loop runs, **THEN** it
  is the **backend** that samples on the configured interval, agentctl holds no timer, and sampling
  for that session **stops** on the first of: a probe result other than `live`; the session leaving
  `WAITING_FOR_INPUT`; the session being stopped or deleted; or backend shutdown.
- **AC-54** — **GIVEN** a session that has never been parked and stays `WAITING_FOR_INPUT`
  indefinitely, **WHEN** the sampling loop runs, **THEN** zero probes are taken for it.
- **AC-58** — **GIVEN** a parked session at `state=REVIEW` with no pending input, no
  `foreground_activity` and not interrupted, **WHEN** its card renders on the **board**
  (`components/kanban-card-content.tsx`), **THEN** `data-testid="task-state-background-running"` is
  present. This must hold against **both** early returns: `renderTaskStatusIcon` currently returns
  `null` at `kanban-card-content.tsx:275` for exactly this input — so the board renders **no icon
  at all** today, not a spinner — and returns a bare `IconLoader2` at `:282` when a launch spinner
  is showing. An implementation that changes only one of the two fails this criterion. Asserted
  separately from AC-23's task list because the two surfaces use different resolvers (§M).
- **AC-59** — **GIVEN** a task that is **not** parked, **WHEN** each of the six production
  `getTaskStateIcon` call sites enumerated in §M renders it, **THEN** the icon is identical to
  today's for **the matrix already enumerated in `apps/web/lib/ui/state-icons.test.tsx`**
  (task-icon describe blocks), extended with the new option pinned `false`; and **GIVEN** a task
  that is not parked, **WHEN** the sidebar row renders, **THEN** the icon is identical to today's
  for **the matrix already enumerated in `apps/web/components/task/task-item.test.tsx`**. Two
  baselines because there are two resolvers. This covers the three call sites this feature
  deliberately does not update.
- **AC-62** — **GIVEN** a parked session rendering the background affordance, **WHEN** the probe
  transitions `live → settled`, **THEN** a `session.activity_changed` carrying
  `parked_on_background_work: false`, a higher `revision` and the current `parked_epoch` is
  published for that session, a `task.updated` is published for its task **because the task-level
  OR also changed**, and the task row stops rendering
  `data-testid="task-state-background-running"`.
- **AC-68** — **GIVEN** a parked session rendering the background affordance whose probe last
  reported `live`, **WHEN** the agent resumes itself and the session enters `RUNNING` **with no
  further sample being taken** (AC-53 stopped the loop), **THEN** `parked_on_background_work` is
  `false`, a `session.activity_changed` carrying `false` and a higher `revision` is published, and
  the task row stops rendering `data-testid="task-state-background-running"`. This is the
  session-state term of the formula clearing it, because `last_sample` is still `live` and is never
  re-read.
- **AC-69** — **GIVEN** a **second** launch recogniser for a different agent ID, registered
  **through the registry's public registration API from a test package**, and a session of that
  agent that settles after its recogniser attests a detached launch with the probe reporting
  `live`, **WHEN** the session and task DTOs are serialized and the task row renders, **THEN** the
  session is parked and renders `data-testid="task-state-background-running"` exactly as a Claude
  session does. The seam guarantee is asserted **by construction**: the test's only production-code
  interaction is the registration call, and the test package imports nothing from the probe,
  projection or rendering paths. *(The second clause previously read "the change required to
  achieve this was the registration alone — no edit to the probe, the parked projection, the
  notification state machine, or any icon call site", which asserts a property of the diff and is
  not observable from a test process. Restated on 2026-08-09 as a construction constraint on the
  test itself. The claim is unchanged; only its form is. Spec Review round 5 should judge this
  rather than assume it.)*
- **AC-70** — **GIVEN** a Claude ACP session that is idle with **no background workload running**,
  whose agent process group contains the bridge process, one or more CLI processes, and one or more
  stdio MCP server processes — the §L measurement, reproduced — **and every one of those
  descendants has a process start time strictly BEFORE the current turn's recorded start**,
  **WHEN** the probe is taken, **THEN** it reports **`settled`**. This is the regression guard for
  §L measurement 1: an implementation that samples process-group membership reports `live` here and
  fails. *(The start-time clause was added to the GIVEN on 2026-08-09. **This changes what AC-70
  asserts** and it is flagged for that reason. Without it the criterion contradicted both the probe
  predicate and the *Failure modes* row that explicitly accepts a mid-turn stdio MCP server as
  `live`: §L's own pid table shows the second CLI and second MCP server were spawned long after the
  session leader, so an unqualified §L session can legitimately contain an in-turn descendant and
  the criterion would have been nondeterministic against the real process tree it is required to
  run against. An additive fix was attempted first and rejected: adding a sibling criterion leaves
  the unqualified AC-70 still contradictory. The mid-turn case is now covered positively by
  AC-70a.)*
- **AC-70a** — **GIVEN** the same §L-shaped idle session, **but with one stdio MCP server whose
  start time is at or after the current turn's recorded start** — the lazily-connected case §L's
  wrapped-around pids show is real — **WHEN** the probe is taken, **THEN** it reports **`live`**,
  and the session is parked if it is also attested and `WAITING_FOR_INPUT`. This is the false
  "still busy" the *Failure modes* table accepts, now observed rather than only described. Together
  with AC-70 it pins that the predicate is the start time and not the process's identity or
  command name.
- **AC-71** — **GIVEN** a Claude ACP session in which the agent has backgrounded a shell during the
  current turn, and that shell has been placed in **its own process group**
  (`pgid != the agent's pgid`) as the CLI does — the §L measurement, reproduced — **WHEN** the probe
  is taken, **THEN** it reports **`live`**. This is the regression guard for §L measurement 2: an
  implementation that samples process-group membership reports `settled` here and fails.
- **AC-72** — **GIVEN** a session whose agent has a descendant that started **before** the current
  turn and is still alive, and no descendant started during the current turn, **WHEN** the probe is
  taken, **THEN** it reports `settled`; and **GIVEN** the same session after a descendant is started
  **during** the current turn, **WHEN** the probe is taken again, **THEN** it reports `live`. The
  pair pins the start-time predicate itself, independent of process groups.
- **AC-73** — **GIVEN** the scenario of §J, reproduced with a probe that returns `live` for at least
  three consecutive samples before returning `settled`, and an agent that resumes itself on that
  transition, **WHEN** the sequence runs, **THEN** at every sample point at which the probe returned
  `live`, `parked_on_background_work` was `true` and `data-testid="task-state-background-running"`
  was the rendered affordance; and while the session was parked, neither
  `task-state-waiting-for-input` nor `task-state-turn-finished` was rendered. *(This is the
  projection-and-rendering half of the former AC-30a. Its third clause — that zero
  `session.turn_finished` deliveries were recorded — was a notification assertion and stays with
  AC-30a in the sibling spec. The straddle is recorded in the task plan.)*
- **AC-73a** — **GIVEN** a parked session, **WHEN** its row renders in the `/tasks` list
  (`app/tasks/rich-task-list-row.tsx:38`), **THEN** `data-testid="task-state-background-running"` is
  present. This surface goes through the shared resolver and was named in no prior revision of this
  spec; it is the second of Kandev's two task lists.
- **AC-74** — **GIVEN** `KANDEV_PARKED_PROBE_INTERVAL` set to `0` and a session that settles into
  the parked condition on its synchronous first sample, **WHEN** the background workload
  subsequently exits and the session remains `WAITING_FOR_INPUT`, **THEN** no further probe is taken
  and `parked_on_background_work` remains `true` indefinitely; and **WHEN** the session then leaves
  `WAITING_FOR_INPUT`, **THEN** it becomes `false`. The stale affordance at `interval=0` is accepted
  and specified, not a defect.
- **AC-75** — **GIVEN** a parked session, **WHEN** the operator submits a prompt that is **queued
  but not admitted** (the session is still `WAITING_FOR_INPUT`), **THEN**
  `parked_on_background_work` is still `true` and the background affordance is still rendered; and
  **WHEN** that prompt is subsequently admitted and the session enters `RUNNING`, **THEN** it
  becomes `false`. The projection tracks the machine, not the operator.
- **AC-76** — **GIVEN** any session that this spec parks, un-parks, or leaves unparked, **WHEN** its
  turn completes, **THEN** `session.turn_finished` is delivered within the same turn-completion
  handling, with the same occurrence key, title, body and timestamp it carries today, and **no
  notification is withheld, deferred, delayed, reordered or dropped by anything in this spec**.
  Asserted across the parked, un-parked, `unknown`-probe and no-recogniser cases. This is the
  criterion that makes this spec shippable independently of
  [`parked-notification-deferral`](../parked-notification-deferral/spec.md).
- **AC-77** — **GIVEN** a consumer that has applied `(epoch E1, revision 7)` for a session, **WHEN**
  the backend restarts and publishes `(epoch E2, revision 0)` for that session with `E2 > E1`,
  **THEN** the consumer **applies** it and the affordance clears; and **GIVEN** the consumer applies
  a boot payload, **THEN** its applied-revision map is reset for every session and task in that
  payload. Asserted for a client that reconnects the WebSocket **without** re-fetching the boot
  payload, which the epoch alone must cover.
- **AC-78** — **GIVEN** a task with two sessions S1 and S2 where S1 is already parked, **WHEN** S2
  also becomes parked, **THEN** a `session.activity_changed` is published for S2, the task's
  `parked_on_background_work` stays `true`, its `parked_revision` does **not** change, and **no
  `task.updated` is published** for that transition.
- **AC-79** — **GIVEN** a turn in which a detached background shell is launched and its attestation
  frame is emitted on the agent stream immediately before the turn-completion frame, **WHEN** the
  turn settles, **THEN** `observed_detached` is already true when the projection is computed, a
  probe is taken, and the session parks. Asserted with the two frames adjacent on the stream, which
  is the ordering the shared consumer (`event_handlers_streaming.go:24`) guarantees and the case a
  separate-queue implementation loses.
- **AC-80** — **GIVEN** a descendant whose process start time falls in the same source-resolution
  tick as the recorded turn start but strictly after it in nanoseconds, **WHEN** the probe is taken
  on Linux and on Darwin, **THEN** both report `live`, because the turn start is truncated down to
  the source's resolution before the inclusive comparison; and **GIVEN** an implementation that
  reads Darwin start times from `ps -eo lstart`, **THEN** this criterion fails, which is the
  intended guard against that source.
- **AC-81** — **GIVEN** `KANDEV_PARKED_PROBE_BUDGET` set to `0`, and separately to a negative
  duration, **WHEN** configuration is loaded, **THEN** in both cases the value is rejected, a
  warning is logged, and the effective budget is the `250ms` default — so no synchronous probe is
  ever issued without a deadline.
- **AC-82** — **GIVEN** a parked session, **WHEN** the app is rendered under the pseudo-locale,
  **THEN** the background affordance's accessible label and tooltip resolve through
  `task:backgroundWorkIsRunning` and no new translation key is introduced by this feature.

## Contract amendment

`docs/specs/platform/background-work-liveness.md` (status `shipped`) currently states, at line 25:

> A settled session follows its coarse state and does not remain visually busy solely because
> detached work is still registered.

That sentence must be amended when this feature ships. Its rationale, recorded in
`docs/decisions/2026-07-28-coarse-running-busy-signal.md`, is that *registration* is an unreliable
edge-driven refcount. This feature does not relax that: it introduces a second, independent,
**sampled** level. The amended sentence should read that a settled session may remain visually busy
only when a positive out-of-band liveness sample supports it, never on registration alone.

`docs/specs/platform/notifications.md` is **not** amended by this spec — nothing here changes
notification behaviour. The sibling spec amends it.

Neither amendment relaxes prompt admission, and neither reads
`features.claudeBackgroundPromptHandoff`.

## Out of scope

- **All notification behaviour** — withholding, deferral, the five-exit state machine, the
  settle-confirmation window, the orphan backstop, the occurrence-ordering rule, and the
  `KANDEV_PARKED_DEFER_BACKSTOP` / `KANDEV_PARKED_SETTLE_CONFIRM` keys. These are
  [`docs/specs/parked-notification-deferral/spec.md`](../parked-notification-deferral/spec.md),
  which depends on this spec. AC-76 is the guard that this spec touches none of it.
- Changing `TaskSessionState`, prompt admission, or the queued-message path.
- Enabling, weakening, or removing `features.claudeBackgroundPromptHandoff`.
- Making the parked projection survive a restart, and persisting the parked revisions. Both are
  process-local exactly like `CancellationPending`. After a restart the counters begin at `0`, the
  epoch changes, and the session reads as not parked; the epoch is what tells a client this is a
  reset rather than a stale frame (AC-77).
- Distinguishing *kinds* of live background work in the UI; parked is one bit.
- Ordering of parked tasks relative to each other in the board or task list. This spec adds one
  boolean per task; sort order is owned by the existing list and board specs.
- Cross-session aggregation beyond a boolean OR. No count, no ranking, no per-session breakdown.
  This exclusion covers the *value* only — the task-level `parked_revision` needed to keep that
  value from going stale is **specified**, not excluded.
- **The parked affordance on the three secondary task-icon call sites** —
  `swimlane-graph-content.tsx:84` and `:121`, `graph2-step-node.tsx:149`,
  `task-state-actions.tsx:33`. AC-23, AC-58 and AC-73a name the two task lists and the board; these
  are graph and action affordances and keep today's behaviour through the `false` default, which
  AC-59 pins.
- **Registering a recogniser for any agent other than Claude.** The seam is required and asserted
  (AC-69); populating it for a second vendor is that vendor's own change. What is *not* excluded,
  and is now contract, is that doing so must not require touching this feature's core.
- **Excluding known-infrastructure processes by name or path.** The start-time predicate makes this
  unnecessary, and a command-name allowlist would be fragile and vendor-specific. A long-lived
  process that genuinely starts mid-turn is counted as `live`; see *Failure modes* and AC-70a.
- Everything in [`docs/specs/acp-elicitation/spec.md`](../acp-elicitation/spec.md).

## Open questions

**None.**

Two questions earlier revisions carried are now closed by evidence rather than assumption:

- *Is the agent's process group a usable liveness source?* Closed by measurement on 2026-08-09;
  see §L. The answer was "no", and the probe was redesigned.
- *Does a Claude self-resume produce a turn boundary?* Closed by reading the adapter on 2026-08-09;
  see §N. The answer is "yes, a synthetic `session/prompt`" — but it does not reach the backend's
  own turn-start path, which is why D3 names a single boundary for both sides.

## Notes for implementation

- `agent.background.probe` is a WS action on the **existing** agent stream: a `case` in the
  `server/api/agent.go` action switch and a `Client` method using `sendStreamRequest`. There is an
  existing completeness test over the action list (`agent_test.go` enumerates `agent.cancel`,
  `agent.permissions.respond`, `agent.stderr`), so the new action must be added to it. Do **not**
  add an `/api/v1/acp` HTTP group; none exists.
- **`turn_started` is a NEW stream event, not an existing one you can subscribe to.** §N
  verifies the current event set has no turn-start member and that `session_status` fires only
  on session create/resume. Adding it touches both processes — the emission sits at
  `beginPromptTurn` (`adapter_prompt.go:140`), inside `sendPrompt` and beside where agentctl
  stamps its own recorded turn start so the two cannot drift, which also means it covers all
  **three** `sendPrompt` callers (`Prompt` `:31`, `PromptSteer` `:60`, `fireWakeup` `:434`)
  without touching any of them; the consumption sits on `handleAgentStreamEvent`. Do **not**
  emit at `sendPrompt` entry or at the outer callers — AC-41b fails both. Land it before either the
  attestation work or the probe work starts; both depend on it and neither owns it. A new event
  type traverses four files, verified by tracing `EventTypeForegroundIdle`: the constant in
  `internal/agentctl/types/streams/agent.go`, the emission in
  `.../transport/acp/`, the relay in `internal/agent/runtime/lifecycle/manager_events.go`, and
  the consumer in `internal/orchestrator/event_handlers_streaming.go`. **The lifecycle relay is
  the step most easily missed** — an event added at both ends but not relayed is silently never
  delivered.
- **Build the projection against `BackgroundProbe`, never against the agentctl `Client`.** The
  port is what lets the projection, the sampling loop and their tests exist before the real
  process-tree probe does, and it is what AC-62, AC-68 and AC-73 drive.
- The two durations are plain env-sourced config, **not** runtime feature toggles. Follow
  `getEnvDuration("KANDEV_ACP_IDLE_TIMEOUT", time.Hour)`
  (`internal/agentctl/server/config/config.go:285`) — but note the probe budget deviates from that
  helper's "`0` disables" idiom and must reject non-positive values (AC-81). Both are read by the
  **backend**.
- **Frontend work touches two resolvers, not one.** §M is the map; re-derive it before starting.
  Promoting `BackgroundWorkTaskIcon` out of `task-item.tsx:165` into `state-icons.tsx` is the first
  step and everything else depends on it. `state-icons.test.tsx` is the home for the shared
  resolver's assertions (AC-52, AC-59 first half, AC-73a); `task-item.test.tsx` is the home for the
  private ladder's (AC-23, AC-34, AC-59 second half). It already defines
  `BACKGROUND_ICON_TEST_ID` at `task-item.test.tsx:11`.
- The board needs **both** early returns changed (`kanban-card-content.tsx:275` and `:282`), not
  just the spinner one. `:275` is the one a parked task hits.
- AC-52 and AC-59 are *no-change* assertions over **named existing matrices**, not invented ones.
  They are cheap to satisfy via the `false` defaults, and they are what stops this feature silently
  altering surfaces that were never parked.
- **E2E needs an injectable probe seam.** AC-73 is written against "three consecutive `live` samples
  then `settled`" so it runs without a real 12-minute wait, and AC-62/AC-68 are transition
  assertions that need the probe to change value mid-spec. **AC-70, AC-70a, AC-71, AC-72 and AC-80
  must exercise a REAL process tree** rather than a stub, since they exist precisely to catch an
  implementation that samples the wrong thing or reads start times from the wrong source.
- No new i18n surface: the affordance reuses `task-state-background-running` and
  `task:backgroundWorkIsRunning`, both of which already ship (AC-82). Moving
  `BackgroundWorkTaskIcon` between files does not change either.
- `docs/specs/INDEX.md` carries a row for this spec, one for the elicitation spec, and one for the
  deferral spec.
