---
status: shipped
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
five-member enum — `end_turn`, `max_tokens`, `max_turn_requests`, `refusal`, `cancelled` — with
no member meaning "turn over, work continues, I will come back". Sweeping the vendored schema at
version `1.20.0` for the terms that would express one returns **zero occurrences in both tiers**
(`background`, `async`, `detach`, `parked`, `long_running`).

*(Citation corrected 2026-08-09. An earlier revision cited `types_gen.go:6631-6637` as though it
were an in-tree file. **There is no `types_gen.go` anywhere in this repository** — the type comes
from the external ACP Go SDK module, so the line reference is not resolvable from a checkout and
a reader following it finds nothing. The claim itself is unchanged and independently checkable:
`acp.StopReasonEndTurn` and its siblings are referenced in-tree from `cmd/mock-agent/main.go:288`
and `.../transport/acp/prompt_handoff_test.go:55`, and the enum is closed at the SDK.)*

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
- **The agent process is launched by `process.Manager`, NOT by `ProcessRunner` — CORRECTED
  2026-08-09, and the distinction is load-bearing.** The agent is spawned at
  `internal/agentctl/server/process/manager.go` as the manager's own single `m.cmd`
  (`exec.Command(m.cfg.AgentArgs[0], …)`), immediately followed by `setAgentProcGroup(m.cmd)`,
  which resolves to `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}` in
  `procattr_unix.go` / `procattr_linux.go`. So the agent process **is** a process group
  leader, and its pid is `m.cmd.Process.Pid`, already exposed internally as the unexported
  accessor `Manager.agentPID()`.
  **`ProcessRunner` (`server/process/runner.go`) is a different component**: it manages
  *workspace* processes — user-launched commands, interactive shells, VS Code, git polling —
  and its `List(sessionID)` filters by `ProcessInfo.SessionID`, a Kandev task-session id, not
  the ACP session. Its `proc.pgid = cmd.Process.Pid` line records the pgid of a **workspace**
  process and has nothing to do with the agent.
  *(Two prior revisions got this wrong in the same sentence. The first attributed `Setpgid`
  to `runner.go`; that half was corrected. The second kept `runner.go` as the source of the
  agent's recorded pgid, which is still the wrong component — a builder following it roots the
  probe at a workspace process, or matches nothing and returns `unknown` forever. The
  *Background-workload liveness probe* section now names the accessor on the right component.)*
  §L measures what the agent's process group actually contains, and the answer invalidates the
  obvious design.
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
assume one. *(There are in fact **three**, and the third is the one the sidebar consumes; this
sentence is about these two surfaces only. §O is the complete map, added 2026-08-09.)*

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

**§M maps the RESOLVERS. It does not map the DATA that reaches them — see §O**, which was added
2026-08-09 for the same reason §M itself was: a revision named the consumer precisely and left the
producer to be guessed.

### N. What a Claude self-resume actually does to the turn boundary — READ FROM THE TREE

The PID-free probe rests on attestation and the probe baseline being scoped to *the same
turn*. §J's observation is an agent that **self-resumed with no operator input**, so whether
that produces a turn boundary is load-bearing, and an earlier revision simply asserted it did
"by construction". It was never checked. It is now:

- **A Claude self-resume goes through a real `session/prompt`.** The Claude Agent SDK exposes
  `ScheduleWakeup`; when its timer fires the SDK queues a turn on its async iterator, and the
  bridge only drains that iterator inside its `prompt()` handler. Kandev's adapter closes the
  gap explicitly: `wakeupScheduler` (`.../transport/acp/wakeup.go:11-46`) records the pending
  wakeup and, at fire time, `fireWakeup` (`.../transport/acp/adapter_prompt.go:434`) issues a
  **synthetic `session/prompt`** through `sendPrompt` (`adapter_prompt.go:71`), serialized behind
  the same `promptGate` as a human prompt. *(Citation normalised 2026-08-09: this line previously
  cited `:388-403`, the doc comment and declaration, while three other places in this spec cite
  `:434`, the `sendPrompt` call. The spec's convention elsewhere — `Prompt (:31)`,
  `PromptSteer (:60)` — is the dispatch line, so `:434` is used consistently now.)*
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

### O. The frontend DATA chain — DERIVED FROM THE TREE, 2026-08-09

**This section exists for exactly the reason §M does, one layer down, and it is the third time
this spec has been wrong about the frontend in the same shape.** §M mapped which *resolver* each
surface uses. It did not map which *object* each resolver is handed, or who builds it. An earlier
revision asserted that "Kandev carries **two** different `Task` types". **Measured false: there are
three, they are fed by one store shape, and that store shape has four producers — of which the spec
named three.** Every line below was read from the tree. A reviewer should re-derive all of it.

**The three `Task`-ish shapes, and who consumes each:**

| # | Shape | Declared in | Consumed by | Casing |
|---|---|---|---|---|
| 1 | Wire/DTO `Task`, `TaskSession` | `lib/types/backend.ts`, `lib/types/http.ts` | `rich-task-list-row.tsx:38`, the session surfaces | **snake_case** |
| 2 | Board/card `Task` | `components/kanban-card.tsx:40-65` | `kanban-card-content.tsx` (resolver B) | **camelCase** |
| 3 | **`TaskSwitcherItem`** | `components/task/task-switcher-types.ts:14` | `task-switcher-row.tsx:143` → `TaskItem` → `TaskStateIcon` (**resolver A**) | **camelCase** |

Shape 3 was named in **no** revision of this spec, and it is the one AC-23's surface consumes.

**Shapes 2 and 3 are both derived from ONE store shape**, `KanbanState["tasks"][number]`
(`lib/state/slices/kanban/types.ts:84`, which already declares `foregroundActivity`). So the field
has to reach that store shape first, and then be forwarded twice.

**The store shape has FOUR producers, not three.** The first three are already named under
*Data model → Revision epoch*; the fourth was not, and it is the one that runs first:

| Producer | File | What it feeds | Named before? |
|---|---|---|---|
| `toKanbanTask` | `lib/kanban/map-task.ts:152` | REST + WS task records | yes |
| `kanban.update` projection A | `lib/ws/handlers/kanban.ts:83` (`existing?.foregroundActivity`) | `state.kanban.tasks` | yes (AC-58a) |
| `kanban.update` projection B | `lib/ws/handlers/kanban.ts:121-124` (`undefined ? fallback : t`) | `state.kanbanMulti.snapshots[…].tasks` | yes (AC-58a) |
| **`snapshotToState`** | **`lib/ssr/mapper.ts:51`** (`foregroundActivity: task.foreground_activity ?? undefined`) | the **Go boot snapshot**, on first paint | **NO** |

`snapshotToState` hand-builds `KanbanTask` field by field and imports only `pickPendingAction` and
`workspaceModeFromMetadata` from `map-task.ts` — it does **not** route through `toKanbanTask`, so
adding the field there does not reach it. Left alone, a parked task renders unparked on first paint
on every surface and only lights up when the next `kanban.update` happens to arrive. AC-58a cannot
catch this: its GIVEN is *"a subsequent `kanban.update` arrives"*.

**`TaskSwitcherItem` has TWO producers, and both were unnamed.** Resolver A is reached from desktop
*and* mobile:

| Producer | File | Surface |
|---|---|---|
| `buildSidebarItem` | `components/task/task-session-sidebar-item.ts`, called at `task-session-sidebar.tsx:92` | desktop sidebar task list |
| `toSheetItem` | `components/task/mobile/session-task-switcher-sheet-hooks.ts` | mobile task switcher, which renders the **same** `TaskSwitcher` (`session-task-switcher-sheet.tsx:9`) and therefore the same `TaskItem` |

Both take `KanbanState["tasks"][number]`. Both already carry `interrupted` and `foregroundActivity`
forward explicitly, and `toSheetItem`'s own comment says the mobile row "shares `TaskItem`
rendering". So a builder who wires only `buildSidebarItem` ships a parked affordance that appears on
desktop and not on mobile, from one component.

**And there is a fourth carrier in play: `TaskStatusSummary`** (`lib/types/task-status-summary.ts`),
which is *not* a `Task` shape but which both sidebar producers prefer over the task record:

```
foregroundActivity: hasSummary ? summary?.foreground_activity : task.foregroundActivity
```

— identical in `sidebarSessionStatus` and in `sheetStatus`. So for `foregroundActivity` the task
record is a **fallback**, not the source, whenever a summary exists. That precedence is deliberate
and documented in `sidebarSessionStatus`'s own comment (ADR-0049: the summary carries a fresher
per-session view for the task in focus). **Whether the parked bit follows that precedence is a real
decision with an inert-but-green failure on one side**, and it is made under *API surface → The
frontend property names*, not left to the builder.

Note for that decision: `TaskStatusSummary` already declares its **own** `revision: number`
(`task-status-summary.ts:3`) with an unrelated meaning.

### Why this was invisible to every criterion

AC-23's home is `task-item.test.tsx` (*Notes for implementation* says so), which renders `TaskItem`
with props passed directly. AC-58 likewise asserts the render *given* the prop. So the entire chain
above — three shapes, six producers — is downstream of nothing any criterion looked at, and an
implementation that wires the resolver and none of the producers passes AC-23, AC-34, AC-58, AC-58b,
AC-59, AC-59a and AC-73a while every live surface stays dark. AC-58a was added in an earlier round
for exactly this reason and covers **one** producer of **one** shape. AC-83 and AC-84 cover the rest.

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
- **Attestation SHALL be scoped to detached background work of kind `shell`.** A detached launch
  of any other kind — today, `Kind=subagent`, which the shipped normalizer already stamps for both
  Claude and the mock agent — is not an attestation and never parks a session. The registry alone
  does not gate this; the kind filter is the other half.
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
  turn_marker           uint64  monotonic per session; increments on every
                                `turn_started` received for it (D2, D3)
  last_sample           enum    live | settled | unknown
```

**`turn_marker` — ADDED 2026-08-09, because the revalidation rule cannot be implemented without
it.** D2 requires a completed sample to be discarded if a new turn began while it was in flight,
and an earlier revision asserted that check "reads only values already in this table plus the
`turn_started` boundary D3 defines". It cannot. `turn_started` carries no timestamp and is not
retained; `revision` moves only on a transition of `parked`, so it is unchanged across a turn
boundary that does not flip the projection; and **`observed_detached` cannot witness the event**
— a new turn that clears it and then re-attests a detached launch inside the sample window leaves
it `true`, and the stale sample applies. So the marker is stated explicitly rather than left to be
invented:

- **Type** `uint64`, **default `0`**, per session, held in the same process memory as the rest of
  the row and read under the same session-level lock.
- It **increments on every `turn_started` received for that session**, and on nothing else. It is
  not a wall clock, is never compared across sessions, and never travels on any carrier — it is
  purely local to the revalidation check.
- It is **not persisted** and **resets to `0`** with the rest of `parkedState` on a backend
  restart, which is safe because every in-flight sample dies with the process.
- A `turn_started` for a session with no `parkedState` entry creates none (see *Entry lifecycle*);
  there is no sample to invalidate.
- It is **distinct from `parked_epoch`**: the epoch orders frames across backend processes for
  consumers, the marker orders samples against turns inside one process. They are never compared
  and never combined.

**Entry lifecycle — STATED 2026-08-09.** An earlier revision described the fields but never said
when a row exists, so a long-running backend would accumulate one per session ever attested.

- **Created** when the ordered consumer first sets `observed_detached` for a session — i.e. on the
  first attested detached launch. Nothing else creates a row: not a `turn_started`, not a settle,
  not a read. A `turn_started` or a settle for a session with no row is a no-op.
- **Revived**, rather than re-created, when an attested detached launch arrives for a session that
  holds a **reduced** entry (see eviction, below): `observed_detached` is set, `parked` is `false`,
  `turn_marker` restarts at `0` and `last_sample` at `unknown` — **but `revision` continues from the
  retained value instead of restarting at `0`**. Restarting it would move the revision backward for
  any consumer still holding the pre-eviction value, which is the defect the retention exists to
  prevent. Nothing else revives a reduced entry: a `turn_started`, a settle, or a read against one is
  a no-op, exactly as against no entry at all.
- **Evicted** on the first of: the session reaching a terminal state (`COMPLETED`, `FAILED`,
  `CANCELLED`); the session being deleted; its execution ending; an agent-context reset for that
  session; or backend shutdown.
- **EVICTION UN-PARKS FIRST — CORRECTED 2026-08-09, and the rule it replaces
  was false.** The previous revision said *"Eviction publishes nothing on its own — the projection
  has already gone `false` through the session-state term by the time any of these is reached
  (D8)."* **That premise does not hold for two of the five causes.** A session's **execution can
  end** — agentctl idle-reaped at `KANDEV_ACP_IDLE_TIMEOUT` (default `1h`), or crashed — while the
  session is still sitting at `WAITING_FOR_INPUT`, which is the ordinary long-idle shape in Kandev
  and precisely the twelve-minute-and-longer window §J measures. An **agent-context reset** likewise
  writes `STARTING` and restores, without the session ever having left `WAITING_FOR_INPUT` as far as
  a parked row is concerned. In both, no term-3 flip occurs, so under the old rule the row vanished
  with `parked` still `true` and **no carrier ever said otherwise**:

  > Because a rowless session serializes D9's defaults `(parked=false, revision 0, parked_epoch E)`,
  > and the consumer's field-scoped rule discards a strictly lower `(epoch, revision)` pair, a client
  > holding `(E, N≥1, true)` **rejects every later frame's parked triple and keeps showing the
  > affordance** — inside one epoch, so only a backend restart clears it. That is the "card stuck"
  > failure *Revision epoch* exists to make impossible, reached through an ordinary idle reap. The
  > publish-failure rule's self-healing claim — "the revision only ever moves forward" — is false on
  > this path, because eviction moves it **backward to `0`**.

  So the rule is stated positively:

  > **Before an entry is evicted, if its `parked` is currently `true`, the eviction performs the
  > `true → false` transition exactly as any other cause does** — under the session-level lock, it
  > flips `parked`, **increments `revision`**, and publishes `session.activity_changed` carrying
  > `false`, the new `revision` and `parked_epoch`; then it takes the per-task lock and applies the
  > `members` removal with the same recompute-compare-publish steps already specified under
  > *Task-level projection*. Only then is the entry **reduced** — see the next bullet, which is what
  > "eviction" leaves behind. If `parked` is already `false`, eviction publishes nothing, which is
  > what the old sentence was right about.
  >
  > **The one exception is backend shutdown**, which publishes nothing in either case: there is no
  > consumer left to inform, every client reconnects against a strictly higher `parked_epoch`, and
  > AC-77 already covers that reset. A builder MUST NOT make shutdown eviction publish.

  This also removes a race the old wording created with *Failure modes*: whichever of the failed
  probe and the execution-end bookkeeping happens first, the affordance now clears exactly once,
  through the sample term or through eviction, and the other becomes a no-op because `parked` is
  already `false`. AC-85 observes it.
- **EVICTION REDUCES THE ROW, IT DOES NOT DELETE THE REVISION — and this is what keeps the
  publish-failure rule true.** *API surface → What happens when the publish itself fails* says a
  dropped frame is corrected because "the next carrier for that entity re-serializes the current
  triple and the consumer's `(parked_epoch, revision)` comparison accepts it, because the revision
  only ever moves forward". **Deleting the row outright breaks that sentence**: a rowless session
  serializes D9's `revision 0`, which moves the revision *backward*, so a single dropped eviction
  frame would be uncorrectable for the life of the process — the same stuck affordance by a
  different route. So:

  > On eviction the entry is **reduced, not removed**: `parked`, `observed_detached`, `last_sample`
  > and `turn_marker` are cleared, and **`revision` is retained** at its last published value. A
  > reduced entry is inert — it creates nothing, is never sampled, never parks, and answers every
  > read as `parked = false` — but a session carrier for it serializes `(false, retained_revision,
  > parked_epoch)` rather than `(false, 0, …)`, so a consumer accepts it and the eviction publish
  > becomes belt-and-braces rather than load-bearing.
  >
  > The reduced entry is dropped for good when the session is **deleted**, or at backend shutdown.
  > Its cost is one session id and one `uint64` per session ever parked in one process lifetime,
  > which is the same order as every other per-session runtime map here, and it is never persisted.

  Two consequences fall out rather than needing their own rules. **Idempotency:** a second eviction
  cause firing for the same session finds `parked` already `false` and publishes nothing, so
  concurrent causes produce exactly one un-park. **A sample completing after eviction** is discarded
  by D2's existing revalidation with no new clause, because `observed_detached` was cleared by the
  reduction and therefore no longer matches the value captured at issue.

  D9 tabulates all of this. Note that the retained revision does **not** change D9's
  "`0` for a session with no recorded transition" — a session that never transitioned has nothing to
  retain, and `0` remains correct for it.
- A session that is attested but never settles keeps its row until one of the above fires. That is
  bounded by the session's own lifetime, which is the same bound every other per-session runtime
  map in the orchestrator carries.

**`last_sampled_at` is REMOVED.** It was declared in an earlier revision and read by no rule, no
failure mode and no acceptance criterion. That is the same defect the removed `sampling` field was
rejected for — a field with no consumer is a second thing that can disagree with the three terms.
Whether and when a session was last sampled is bookkeeping internal to the loop, bounded by AC-53
and AC-54.

**There is deliberately no field for "a sample is in flight" either.** The revalidation check
compares a snapshot the sampler captured at issue time — (`observed_detached`, session state,
`turn_marker`) — against the same three values at completion. Storing a redundant in-flight copy
would create a fourth thing that can disagree with the three terms.

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

**The OR is computed from a TASK-OWNED member cache, not by reading the sessions — SPECIFIED
2026-08-09.** An earlier revision said the OR is "recomputed across all of the task's sessions"
inside a per-task lock, while also mandating the lock order **session first, then task**. Those
two cannot both hold: reading each session's `parked` under the task lock means taking session
locks *after* the task lock — the forbidden order — and reading them unlocked races the very
concurrent transition the rule was added for. So the data is restated:

```
taskParkedState
  task_id            string            PK
  members            map[sessionID]bool   last value each session PUBLISHED
  parked             bool                 == OR over members
  parked_revision    uint64               increments on each change of `parked`
```

`members` is **owned by the per-task lock** and by nothing else. A session never reads another
session's state, and the task lock never reaches into a session:

1. The session completes its own transition under its **session-level** lock — it flips its
   `parked`, increments its `revision`, and releases that lock.
2. It then takes the **per-task** lock and, under it, writes its own entry in `members`,
   recomputes `parked` as the OR over `members`, compares with the previous value, increments
   `parked_revision` **only on a change**, and enqueues `task.updated` **only on a change**.
3. It releases the task lock before the event-bus send.

No path holds both locks in the opposite order, because step 2 takes no session lock at all. Two
sessions of one task transitioning concurrently serialize at step 2, so their `members` writes and
the resulting `parked_revision` values are in the same order as the OR actually changed — which is
the unorderable-carrier defect the `max()` rule was rejected for. Session transitions on
*different* tasks never contend.

- A session's entry is **removed from `members`** when its `parkedState` row is evicted
  (*Entry lifecycle*), under the same per-task lock and with the same recompute-compare-publish
  steps — so a task whose last parked session ends publishes the `true → false` flip exactly once.
  **This runs at step 2 of the sequence above, after the session side has released its own lock**:
  per *Entry lifecycle*, an eviction of a row whose `parked` is `true` performs the session-level
  `true → false` transition first and only then applies this removal, so the lock order is exactly
  the ordinary one and no new path holds both locks. *(Clarified 2026-08-09. This sentence was
  already correct and is what showed the old "eviction publishes nothing" rule to be wrong — if the
  projection were always already `false` at eviction, the flip it promises could never fire.)*
- The boolean and the counter are read for serialization in **one critical section** under the
  same per-task lock, so they cannot come from different instants.
- AC-78's "no `task.updated` is published" is the observable consequence for the non-flipping
  case; AC-49's strict increase is the consequence for the flipping one; **AC-49a** observes the
  concurrent case directly.
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

**"Discard" is FIELD-SCOPED, not frame-scoped — SPECIFIED 2026-08-09.** An earlier revision said
a consumer "discards the update", which is ambiguous and dangerous in one direction: the same
carriers transport unrelated state, so rejecting a whole `session.state_changed` or `task.updated`
frame because its parked triple is stale would drop a legitimate session-state or title change
and produce a *worse* bug than the one the revision guard exists to prevent.

> A stale pair causes the consumer to **keep its existing `parked_on_background_work`,
> `revision` and `parked_epoch`** and to **apply every other field of the frame normally**. Only
> the three parked fields are held back. Conversely, a frame carrying no parked fields at all
> leaves the three untouched — it is not read as `false`, which is what reconciles this rule with
> D9's "absent ≡ `false`" (that default applies to a *first* observation of an entity, never to a
> partial update of one already held).

**This is not a new pattern, and BOTH halves already exist — CORRECTED 2026-08-09.** An earlier
revision named "the equivalent merge helper on the task slice" as the task-side chokepoint.
**There is no task slice**: `apps/web/lib/state/slices/` contains `session/`, `kanban/`, `office/`
and others, and no `task/`. A builder following that citation has nowhere to put the code. The two
real chokepoints, both already implementing the exact merge discipline this rule needs:

| Scope | Chokepoint | Existing precedent in it | Carriers it covers |
|---|---|---|---|
| Session | `mergeCancellationProjection` in `apps/web/lib/state/slices/session/session-slice.ts` | returns a `Pick<TaskSession, "cancellation_pending" \| "cancellation_revision">` — two fields merged by revision while the surrounding merge applies the rest of the session | `session.state_changed`, `session.activity_changed`, session REST/boot records |
| Task | `mergeTaskUpdate` in `apps/web/lib/ws/handlers/tasks.ts` | already distinguishes "the payload omitted this field" from "the payload carried an explicit value", via `hasPayloadField` + `preserveOmittedField`, for `interrupted`, `parent_id`, `foreground_activity` and others | `task.updated`, task REST/boot records |

The parked triple follows each verbatim, with the epoch added to the comparison. `mergeTaskUpdate`
is a particularly close fit: its `foreground_activity` branch already encodes "preserve when the
event omits it entirely; an explicit value wins", which is precisely the omitted-vs-stale
distinction *this* rule needs, and its comment already explains why.

Every WS handler that upserts a session or task — including `agent-session.ts`, which registers
`"session.activity_changed"` — routes through those two functions rather than comparing revisions
inline. The **boot reset** (AC-77) is applied by the session slice and by the task-update path when
they ingest a boot payload; it is a store-level concern, not a per-handler one, which is what makes
it cover the reconnect-without-boot path the epoch alone must handle.

**The board `Task` needs a named PRODUCER, not just a field name — ADDED 2026-08-09.** Naming
`parkedOnBackgroundWork` on the board shape (*API surface → Parked projection*) says what the
property is called but not who sets it, and the board task is not one object.
`apps/web/lib/ws/handlers/kanban.ts` **hand-rebuilds it field by field into two separate stores** —
`state.kanban.tasks` and `state.kanbanMulti.snapshots[workflowId].tasks` — and each must carry the
value forward explicitly or it is dropped on the next `kanban.update`. Both already do this for
`foregroundActivity` and `interrupted`: one projection reads `existing?.foregroundActivity`, the
other falls back with `t.foregroundActivity === undefined ? fallback?.foregroundActivity : …`.

> `parkedOnBackgroundWork` MUST be carried forward in **both** `kanban.ts` projections, following
> whichever of those two patterns each site already uses for `foregroundActivity`, and MUST be
> projected from the snake_case wire field in `apps/web/lib/kanban/map-task.ts`, alongside its
> existing `foregroundActivity: pickForegroundActivity(source.foreground_activity)` line.

Without this, AC-58 passes in a unit test that renders the card with a prop and fails on a live
board that has since received a `kanban.update` — the same class of defect §M exists to prevent,
one layer down. §M mapped the *resolvers*; this maps the *data*. AC-58a observes it.

**THOSE THREE ARE NOT THE WHOLE PRODUCER SET — CORRECTED 2026-08-09.** The paragraph above names
`map-task.ts` and the two `kanban.ts` projections, which is what an earlier revision believed to be
exhaustive. §O measures a **fourth** producer of the same store shape, `snapshotToState`
(`lib/ssr/mapper.ts:51`), which hand-builds `KanbanTask` from the **Go boot snapshot** and does not
route through `toKanbanTask` — so the field must be added there too, beside its existing
`foregroundActivity: task.foreground_activity ?? undefined` line. This is the producer that runs
**first**, on page load, and AC-58a cannot observe it because AC-58a's GIVEN is a *subsequent*
`kanban.update`. AC-84 observes it. Two further producers, `buildSidebarItem` and `toSheetItem`,
build `TaskSwitcherItem` from that store shape for resolver A; they are specified under
*API surface → The frontend property names* and observed by AC-83. The complete six-producer list is
tabulated there.

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

**The frontend property names, per `Task` shape — NAMED 2026-08-09, and CORRECTED 2026-08-09 from
two shapes to three.** An earlier revision said §M measured that Kandev carries "**two** different
`Task` types". §O measures that there are **three**, and that the third is the one resolver A — the
sidebar, AC-23's surface — actually consumes. The rule is unchanged: *follow each shape's existing
convention rather than impose one*.

| Shape | Declared in | Existing precedent | New fields |
|---|---|---|---|
| Wire/DTO `Task`, `TaskSession` | `apps/web/lib/types/backend.ts`, `lib/types/http.ts` | `foreground_activity` (`backend.ts:106`, `:289`, `:307`), `cancellation_pending` (`:292`) | `parked_on_background_work`, `parked_revision` (task) / `revision` (session), `parked_epoch` — **snake_case, unchanged from the wire** |
| Board/card `Task` | `apps/web/components/kanban-card.tsx:40-65` | `foregroundActivity` (`:65`) | `parkedOnBackgroundWork` — **camelCase**; carries no revision fields and gains none |
| **`TaskSwitcherItem`** | **`apps/web/components/task/task-switcher-types.ts:14`** | `foregroundActivity` (`:20`), `interrupted` (`:22`) | **`parkedOnBackgroundWork` — camelCase**; carries no revision fields and gains none |
| **Store shape** `KanbanState["tasks"][number]` | **`apps/web/lib/state/slices/kanban/types.ts:84`** | `foregroundActivity` (`:84`) | **`parkedOnBackgroundWork` — camelCase**; this is the shape shapes 2 and 3 are both derived from, so it must carry the field or neither can |

Consequences that are contract, because each is a place two shapes meet:
`rich-task-list-row.tsx:38` reads the **snake_case** shape and passes
`task.parked_on_background_work`; `kanban-card-content.tsx` and `task-switcher-row.tsx:143` read
**camelCase** shapes and pass `task.parkedOnBackgroundWork`. The revision triple is consumed by the
**store slices** (*Revision epoch*), never by a card or row component, which is why neither camelCase
shape needs a revision field and neither may grow one.

**On all three camelCase shapes the field is OPTIONAL and `undefined` is equivalent to `false`**,
exactly as `foregroundActivity` already is on each of them and exactly as D9's absent-≡-`false` rule
states for the wire. So a producer that has not been updated yet, a boot snapshot from a backend
that predates this feature, and a task the backend has never projected all render as today rather
than as an error. No shape declares the field non-optional and no consumer coerces `undefined` to
anything but `false`.

**Every producer that must carry the field, named — because the field is dropped by omission at each
one.** §O derives this; the list is contract, and AC-83 and AC-84 observe it.

| # | Producer | File | Pattern to follow |
|---|---|---|---|
| 1 | `toKanbanTask` | `lib/kanban/map-task.ts` | beside `foregroundActivity: pickForegroundActivity(source.foreground_activity)` |
| 2 | `kanban.update` projection A | `lib/ws/handlers/kanban.ts:83` | `existing?.foregroundActivity` |
| 3 | `kanban.update` projection B | `lib/ws/handlers/kanban.ts:121-124` | `t.x === undefined ? fallback?.x : t.x` |
| 4 | **`snapshotToState`** | **`lib/ssr/mapper.ts:51`** | beside `foregroundActivity: task.foreground_activity ?? undefined` |
| 5 | **`buildSidebarItem` / `sidebarSessionStatus`** | **`components/task/task-session-sidebar-item.ts`** | beside `foregroundActivity`, but see the precedence rule below |
| 6 | **`toSheetItem` / `sheetStatus`** | **`components/task/mobile/session-task-switcher-sheet-hooks.ts`** | same rule as 5 — mobile renders the same `TaskItem` |

1–3 were already named; **4, 5 and 6 are added 2026-08-09** and are the close for the round-5
finding. Producer 4 is the boot path, so omitting it means the affordance is absent on first paint
and appears only when a later `kanban.update` arrives. Producers 5 and 6 are the only route to
resolver A at all, so omitting them leaves AC-23 green in `task-item.test.tsx` and the live sidebar
dark.

**THE STATUS-SUMMARY PRECEDENCE — DECIDED HERE, because both sidebar producers make the task record
a fallback rather than the source.** §O measures that `sidebarSessionStatus` and `sheetStatus` both
resolve `foregroundActivity` as `hasSummary ? summary?.foreground_activity : task.foregroundActivity`,
so a builder mirroring the neighbouring line would need `TaskStatusSummary` to carry the parked bit
— and if it does not, the sidebar reads `undefined` for every task that *has* a summary, which is the
common case for an open task. That is an inert-but-green failure, so the decision is stated rather
than left to the neighbouring line:

> **`parkedOnBackgroundWork` is read from the TASK RECORD unconditionally, on both producers 5 and
> 6. It does NOT follow `foregroundActivity`'s summary-first precedence, and `TaskStatusSummary`
> gains no parked field of any kind.** In `sidebarSessionStatus` and `sheetStatus` the line is
> `parkedOnBackgroundWork: task.parkedOnBackgroundWork`, with **no `hasSummary` ternary**.

Three reasons, each checkable:

1. **`TaskStatusSummary` already declares its own `revision: number`**
   (`lib/types/task-status-summary.ts:3`) with an unrelated meaning. The session carriers' parked
   counter is also called `revision` (*API surface* table above), so putting the triple on the
   summary either collides on that name or introduces a fourth revision namespace.
2. **The summary is not one of the carriers the discard rule covers.** *Revision epoch* names exactly
   two chokepoints, `mergeCancellationProjection` and `mergeTaskUpdate`, and the summary routes
   through neither. A summary carrying a parked value could therefore overwrite a newer `task.updated`
   with no `(parked_epoch, revision)` pair to order the two — the unorderable-carrier defect the
   `max()` rule was rejected for, reintroduced on a different carrier.
3. **There is nothing for the summary to refine.** The precedence exists because the summary carries
   a fresher *per-session* view; `parked_on_background_work` at task level is already the OR across
   the task's sessions by construction (*Data model → Task-level projection*), so a summary could
   only ever restate it.

The cost is named and accepted: the sidebar's parked bit and its `foregroundActivity` come from
different sources and can therefore be from different instants. That is harmless in the only
direction it can differ — both are advisory affordances on the same row, neither gates input, and
the parked bit has its own revision guard on the store shape while `foregroundActivity` does not.

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

**What happens when the publish itself fails — STATED 2026-08-09.** The revision is incremented in
memory before the event-bus send, and an unchanged re-derivation publishes nothing, so a single
failed enqueue would otherwise strand every connected client on the stale value with no later
event able to correct it — the "card stuck" failure this feature exists to prevent, reached
through a dropped frame rather than a reordered one.

> A failed publish is **not** retried and the projection is **not** rolled back. The in-memory
> value and its revision stand — they are the truth, and a rollback would make the next legitimate
> transition unorderable. Instead the failure is logged at warn, and correctness is restored by
> the **next carrier for that entity**, whatever it is: any subsequent
> `session.activity_changed`, `session.state_changed`, `task.updated`, REST read, or boot payload
> re-serializes the current triple, and the consumer's `(parked_epoch, revision)` comparison
> accepts it because the revision only ever moves forward. The affordance is therefore stale at
> most until the session's next state change, and never permanently.

This is the cheap direction and it is the same one D9 takes elsewhere: a lost frame degrades to a
stale spinner, never to a stuck notification, because this spec withholds none.

**The whole argument rests on the revision never moving backward, which is why eviction REDUCES the
`parkedState` entry rather than deleting it — ADDED 2026-08-09.** A deleted entry makes its session
serialize D9's `revision 0`, and a consumer holding `(E, N)` then discards every later frame's
parked triple for the rest of the epoch. That is the one uncorrectable case, and *Data model →
Entry lifecycle* removes it by retaining `revision` on the reduced entry. AC-85's last clause
observes it.

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
`apps/web/lib/ui/state-icons.tsx` and export it — same `IconCircleDashed`, same violet, same
`data-testid`, same `aria-label` and tooltip from `task:backgroundWorkIsRunning`.
`task-item.tsx` imports it instead of declaring it.

**It does NOT move byte-for-byte, and an earlier revision saying "unchanged" contradicted the
precedent it cites — CORRECTED 2026-08-09.** `InterruptedTaskIcon` accepts a `className` and
merges it, which is what lets `getTaskStateIcon` render it as
`<InterruptedTaskIcon className={cn("h-4 w-4", className)} />`. `BackgroundWorkTaskIcon` today
takes **no props** and hardcodes `h-3.5 w-3.5` plus a sidebar-tuned `mt-[1px]`. Promoted verbatim,
it would silently swallow every shared-resolver caller's class — `h-4 w-4` on the board,
`h-4 w-4 shrink-0` on `/tasks` (losing `shrink-0`, which is layout-affecting), `h-3 w-3` on the
graph nodes. No criterion would catch it: AC-58 and AC-73a assert only the `data-testid`, and
AC-59 asserts only the *not*-parked baseline.

> On promotion `BackgroundWorkTaskIcon` gains `className?: string`, merged onto the inner icon
> exactly as `InterruptedTaskIcon` does, and the size classes move out of the component into the
> call sites. The sidebar keeps its current appearance by passing `h-3.5 w-3.5 mt-[1px]` from
> `task-item.tsx`; the `mt-[1px]` nudge is **sidebar-specific and MUST NOT** become a default,
> since the board and `/tasks` rows do not share that alignment. AC-59a pins the sidebar's
> rendered classes as unchanged.

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

**Where that prop COMES FROM is contract too, and it is four hops — ADDED 2026-08-09.** Naming the
prop and its call site says what `TaskStateIcon` reads and nothing about what reaches `TaskItem`.
Per §O the chain is:

```
KanbanState["tasks"][number].parkedOnBackgroundWork      <- producers 1-4 (API surface table)
  -> buildSidebarItem / toSheetItem                      <- producers 5-6, task record, no ternary
    -> TaskSwitcherItem.parkedOnBackgroundWork           <- new field on that shape
      -> task-switcher-row.tsx:143  <TaskItem parkedOnBackgroundWork={task.parkedOnBackgroundWork}/>
        -> TaskItemProps            <- new optional prop, defaulting to false
          -> task-item.tsx:483      <TaskStateIcon parkedOnBackgroundWork={…}/>
```

Every hop drops the value by omission if it is missed, and **only the last two are visible to
`task-item.test.tsx`**, which is AC-23's home and passes props directly. **The mobile task switcher
is in scope**, not an exclusion: it renders the same `TaskSwitcher` and therefore the same
`TaskItem` (§O), so wiring `buildSidebarItem` alone ships one component that lights up on desktop and
not on mobile. AC-83 asserts hops 1–3 on both producers.

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
It gains a `parkedOnBackgroundWork` input, defaulting to `false`, and resolves:

1. `canRequestInput && hasPendingPermission` → `PENDING_PERMISSION_ICON` (`:304`)
2. `canRequestInput && hasPendingClarification` → `SESSION_STATE_ICONS.WAITING_FOR_INPUT` (`:305`)
3. `canRequestInput && foregroundActivity === "background"` → `SESSION_BACKGROUND_ICON` (`:308`)
4. **new:** `canRequestInput && parkedOnBackgroundWork` → `SESSION_BACKGROUND_ICON`
5. coarse `SESSION_STATE_ICONS[state]` (`:310`)

**The TOOLTIP must move with the icon — ADDED 2026-08-09.** `sessions-dropdown.tsx` renders the
icon inside a `Tooltip` whose content is `sessionStatusTooltip(session.state, pending,
session.foreground_activity)`. That helper takes no parked input, so changing only the icon
resolution ships a parked session showing the violet background spinner next to tooltip text
reading *"Waiting for input"* — the two affordances contradicting each other on the same element,
which is worse than the ambiguity this feature exists to remove.

> `sessionStatusTooltip` gains the same `parkedOnBackgroundWork` input and resolves it in the
> **same order as the icon ladder** — after both pending-input cases, before the coarse state — so
> icon and text can never disagree. It reuses `task:backgroundWorkIsRunning`, the string
> `BackgroundWorkTaskIcon` already carries, so this adds no i18n key and AC-82 still holds.
> AC-51a observes that the icon and the tooltip agree.

Mirroring the task row exactly: after the flagged experiment, and after both pending-input
branches so a real question always outranks parked. Three call sites pass it —
`sessions-dropdown.tsx:475`, `session-reopen-menu.tsx:204`,
`mobile/mobile-sessions-section.tsx:132` — and any that does not keeps today's behaviour via
the `false` default (AC-52).

**The signature becomes an options object, and this is contract rather than taste.**
`getSessionStateIcon` (`state-icons.tsx:313-319`) already takes **five positional parameters**
(`state`, `className`, `foregroundActivity`, `hasPendingClarification`, `hasPendingPermission`).
`apps/web/AGENTS.md` caps parameters at **≤5** and `apps/web/eslint.config.mjs:42` sets
`max-params: ["warn", 5]`, so a sixth positional argument is over the documented limit. An
earlier revision said only that the function "gains a parameter", which left the builder to
choose between breaking a stated limit and performing an unauthorised refactor.

> The two option-shaped inputs move into a trailing options object:
> `getSessionStateIcon(state, className, foregroundActivity, options)`, where `options` is
> `{ hasPendingClarification?, hasPendingPermission?, parkedOnBackgroundWork? }`, each
> defaulting to `false`. This lands at four parameters, mirrors the shape
> `getTaskStateIcon(state, className, options)` already uses (`state-icons.tsx:283-287` taking
> `TaskStateIconOptions`), and leaves room for the next flag without another signature change.
> `getSessionStateIconConfig` follows the same shape.

**AC-52's baseline is therefore re-expressed, not invalidated**, and this is the one place this
spec knowingly rewrites an existing test. The existing session-icon matrix in
`state-icons.test.tsx` calls the function **positionally** (e.g. `:335`, `:341`, `:349`, `:357`),
so those call sites are mechanically rewritten to the options shape. What AC-52 pins is the
**resolved icon for every row of that matrix**, which must be byte-identical before and after;
the call syntax is not part of the assertion. A row whose icon changes is a regression
regardless of how the arguments were passed. Rewriting the calls without changing a single
expected icon is exactly the evidence AC-52 asks for.

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
| **Payload** | `session_id`, plus the **`PromptGeneration`** agentctl holds at the emission point. `ExecutionID` is **not** supplied by agentctl — the lifecycle relay stamps it, as it does for every other agent stream event (below). **No timestamp** — the turn start stays in agentctl's own memory and clock (*Background-workload liveness probe*), and nothing about it crosses the process boundary |
| **Consumed by** | `handleAgentStreamEvent` (`internal/orchestrator/event_handlers_streaming.go:24`), the single ordered consumer D3 requires |

**The ownership filter's drop direction — CORRECTED 2026-08-09, and the previous revision had it
exactly backwards.** That revision claimed an event carrying no execution identity and generation
`0` was *"liable to be discarded precisely while a cancellation is reconciling"*, and required the
payload to carry both fields in order to survive. **Measured false, and the requirement it
justified was therefore resting on a hazard that does not exist.**
`cancellationOwnsStreamEvent` (`internal/orchestrator/task_operations.go:4615-4634`) returns
`true` outright when no cancellation is in flight, and each of its two rejection clauses is
guarded on **both** sides being present:

```go
if identity.executionID != "" && executionID != "" && identity.executionID != executionID { return false }
if identity.promptGeneration != 0 && promptGeneration != 0 && identity.promptGeneration != promptGeneration { return false }
```

So an identity-free, generation-`0` event is **never rejected *by this filter***. The filter drops
only on a *mismatch of two present values*. The real relationship is the opposite of the one that
was written down: carrying a **non-zero** identity is what makes an event droppable by the filter;
carrying none is what makes it always pass it.

**THE OWNERSHIP FILTER IS NOT THE ONLY GATE, AND THE EARLIER WORDING "UNCONDITIONALLY ADMITTED"
OVERSTATED WHAT IT PROVES — CORRECTED 2026-08-09 on the round-4 bounce.** `handleAgentStreamEvent`
applies a **second, independent** drop after the ownership filter: for every event whose type is
not `complete`, `shouldDropCompletedExecutionStreamEvent`
(`internal/orchestrator/event_handlers_streaming.go:768-780`) drops it when its
`(sessionID, executionID)` pair has been marked terminal. And cancellation marks the **owning**
execution terminal before teardown — *"Cancellation takes effect before detached runtime teardown.
Tombstone the execution immediately so buffered agent frames cannot recreate session output after
the coordinator has acknowledged the stop"* → `s.markExecutionFailed(sessionID, result.ExecutionID)`
(`task_operations.go:2681-2685` → `markTerminalExecution`, `event_handlers_streaming.go:1259-1271`).

> A `turn_started` relayed **after** its own execution has been tombstoned is therefore dropped at
> that second gate, even though the ownership filter would have admitted it. This is an **accepted**
> drop path, not a defect to engineer around: the execution is ending, the session's `parkedState`
> row is evicted on execution end anyway (*Data model → Entry lifecycle*), and the consequence is
> the cheap one already specified — `observed_detached` is simply not cleared, so the bias is toward
> a stale affordance rather than a wrongly-appearing one. A builder MUST NOT special-case
> `turn_started` past this gate.

So the accurate statement, and the one the criteria now use, is that generation `0` and a missing
identity are **never rejected by the ownership filter** — not that such an event is admitted
unconditionally by the consumer as a whole. AC-41a and AC-79a scope their admission clauses to an
execution that has not yet been tombstoned, which is what makes them deterministic rather than a
race against cancellation's own bookkeeping.

Three consequences of the ownership filter's shape, each contract:

- **`ExecutionID` is relay-stamped, not agentctl-supplied.** agentctl's own `AgentEvent` carries no
  execution id at all; the lifecycle relay stamps `ExecutionID` onto every payload it forwards. A
  `turn_started` relayed like any other stream event therefore cannot reach the consumer
  identity-free, and no requirement on agentctl can or should ask it to supply one.
- **`turn_started` carries the prompt generation agentctl holds at the emission point**, obtained
  the same way `notifWork` obtains it (`a.currentPromptGeneration()`). This is a
  carry-what-you-have rule, not a guarantee that the value is non-zero — see the next paragraph.
- **A `turn_started` from a stale execution is rejected during reconciliation, and that is
  correct.** Once the relay has stamped a real `ExecutionID`, an event from a superseded execution
  mismatches the cancellation identity and is dropped — which is the behaviour we want, since a
  stale execution's turn boundary must not clear the owner's `observed_detached`. AC-79a's second
  clause observes exactly this, and nothing else.

**Prompt generation `0` is VALID on the synthetic path, and this is stated rather than left to be
invented — ADDED 2026-08-09.** The previous revision's MUST ("carry the same prompt generation
every other agent stream event carries") is unachievable-as-intended on the one path §N and AC-41
make load-bearing. Read from the tree: `fireWakeup` dispatches
`a.sendPrompt(ctx, prompt, nil, sessionID, 0, false)` (`adapter_prompt.go:434`) — the generation
argument is the **literal `0`**. `sendPrompt` passes it to `newPromptTurnState(ctx,
promptGeneration, …)` (`adapter_prompt_cancel.go:22-38`), which stores it on the turn;
`acquirePromptTurn` installs that turn as `a.promptTurn` (`:57-60`); so at `beginPromptTurn`
(`adapter_prompt.go:140`) `currentPromptGeneration()` (`adapter_prompt_cancel.go:203-208`) returns
**`0`**. There is no non-zero generation in existence to carry.

> A `turn_started` emitted on the synthetic `ScheduleWakeup` path carries `prompt_generation: 0`,
> and that is **correct and sufficient**, not a defect to be worked around. A builder MUST NOT
> synthesize a generation for it, and MUST NOT skip the emission because it has none — skipping is
> the one choice that silently breaks AC-41's synthetic half while passing every other criterion.
> Per the filter semantics above, generation `0` is **never rejected by the ownership filter**,
> including mid-cancellation. It remains subject to the completed-execution gate like any other
> event, which is the accepted drop path named above.

Admitting a generation-`0` `turn_started` fails in the **cheap** direction, which is why no fence
is specified: the event clears `observed_detached` and increments `turn_marker`, and both of those
bias the projection *away* from parking. The worst case is an affordance that does not appear, not
one that appears wrongly or sticks. AC-41a's third clause observes the generation-`0` path.

**Which `session_id` — the namespace is contract.** The value in hand at `beginPromptTurn` is the
**ACP** session id; `parkedState` is keyed by the **Kandev task-session** id. Both names appear on
lifecycle payloads (`SessionID` and `ACPSessionID` are separate fields on several of them, and one
is annotated `// Task session ID`), so "carrying that `session_id`" is ambiguous enough that a test
can assert the wrong one and pass.

> agentctl emits the **ACP** session id, as every other stream event does. The lifecycle relay
> translates it the same way it already does for the rest of the agent stream, and the backend
> consumer keys `parkedState` off the **Kandev** `payload.SessionID` it receives — never off an
> `ACPSessionID` field. AC-41a asserts the backend end of this, not the wire end.

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

**It must be emitted through the ORDERED notification path, not straight onto `updatesCh` —
ADDED 2026-08-09.** agentctl does not deliver agent events in call order by default. ACP
notifications go through `enqueueACPUpdate`, which posts onto a 4096-slot `notifQueue`; a single
worker (`runUpdateWorker`) drains it and only then calls `sendUpdate` → `updatesCh`. The comments
on those functions state the reason explicitly: "a single worker preserves FIFO ordering across
notifications". `beginPromptTurn` runs on the *prompt* goroutine, so an emission that called
`sendUpdate` directly would **bypass the queue and overtake still-queued notifications from the
previous turn**.

That is not a theoretical reordering. The concrete failure: turn N backgrounds a shell, its
`tool_call` attestation is sitting in `notifQueue`, turn N+1 dispatches, `turn_started` jumps the
queue and clears `observed_detached` (which is not yet set), and *then* the turn-N attestation
drains and sets `observed_detached` — which the backend now reads as belonging to turn N+1. The
projection marks the wrong turn, and the affordance can appear on a turn that launched nothing.

> `turn_started` MUST be enqueued on the **same `notifQueue` FIFO** as tool-call notifications, so
> that every frame the previous turn produced is delivered to `updatesCh` before it. agentctl
> already owns both primitives needed: `notifWork` carries a `promptGeneration` alongside the
> notification, and `syncNotifQueueThen(afterBarrier)` runs a callback **on the worker at the FIFO
> barrier boundary**.

**WHICH THREAD STAMPS THE TURN START, AND WHETHER THE PROMPT BLOCKS — PINNED 2026-08-09, because
the previous revision specified this twice and differently.** *Background-workload liveness probe*
said the stamp is taken *"at `beginPromptTurn` (`adapter_prompt.go:140`)"* — the **prompt
goroutine**, before `conn.Prompt`. This section said the stamp *"must be stamped at the same
ordered instant the event is emitted"* via `syncNotifQueueThen` — the **update worker**, at the
FIFO barrier. Those are two different threads, and three implementations satisfy every criterion
as previously written. One of them is wrong in the expensive direction:

| Reading | Stamp lands | Verdict |
|---|---|---|
| (a) `sendPrompt` calls `syncNotifQueueThen(afterBarrier)` and **blocks**; `afterBarrier` stamps *and* emits, on the worker, before `conn.Prompt` | before dispatch | **REQUIRED** |
| (b) stamp on the prompt goroutine at `:140`, enqueue the event non-blocking | before dispatch | rejected — harmless but leaves the stamp and the emission on different threads, which is the drift D3 names one boundary to prevent |
| (c) enqueue non-blocking, stamp inside the worker callback | **possibly after `conn.Prompt`** | **FORBIDDEN** |

> The contract is **(a)**. `sendPrompt` calls `syncNotifQueueThen(afterBarrier)` at its
> `beginPromptTurn` call site — **after that call has returned**, per the locking rule below — and
> **blocks on the barrier**, and the `afterBarrier` callback — running on
> the update worker, after every frame the previous turn queued — performs **both** writes: it
> stamps agentctl's recorded turn start and emits `turn_started`. Because `syncNotifQueueThen`
> does not return until the callback has run **when it runs at all**, the stamp is guaranteed to
> precede `conn.Prompt`; the two cases where the callback does not run are specified under *When
> the barrier fails*.

**THE BARRIER MUST BE AWAITED WITH NO ADAPTER MUTEX HELD, AND THIS IS THE ONE RULE THAT MAKES (a)
SAFE RATHER THAN A HANG — ADDED 2026-08-09.** The previous revision said the call happens "at
`a.beginPromptTurn(sessionID)`" and asserted, as its whole safety argument, that blocking there
"is **not** a deadlock: the worker's only blocking call is the downstream stream send."
**That claim is false against the tree, and the placement it invites deadlocks.**

`beginPromptTurn` (`adapter_async_complete.go:150-155`) takes `a.asyncTurnMu` at `:151` and holds
it for its **whole body** (`defer a.asyncTurnMu.Unlock()` at `:152`). The update worker acquires
**that same mutex**: `handleACPUpdate` → `maybeScheduleAsyncTurnComplete`
(`adapter_updates.go:201`, `:215`, `:595`) → `a.asyncTurnMu.Lock()`
(`adapter_async_complete.go:57`). So the worker has a second blocking call, and it is one the
prompt goroutine can be holding.

The reachable interleaving, which is why this is stated as a rule rather than a caution:
`maybeScheduleAsyncTurnComplete` returns early at `:52` when `a.currentPromptTurn() != nil`. A
worker draining a frame between turns evaluates that check while no turn is installed and
proceeds. The prompt goroutine then passes the gate, `acquirePromptTurn` installs the turn, and
`beginPromptTurn` takes `asyncTurnMu` and waits on the barrier. The worker reaches `:57`, blocks
on `asyncTurnMu`, and neither side can advance. `syncNotifQueueThen` honours no caller context
(`adapter_updates.go:56-59`), so **only adapter shutdown releases it** — every `session/prompt`
on that adapter hangs, human, steer and synthetic alike.

> `syncNotifQueueThen` MUST be awaited with **no adapter mutex held**. In particular the barrier
> wait MUST NOT be placed inside `beginPromptTurn`'s `asyncTurnMu` critical section:
> `beginPromptTurn` runs to completion and releases that mutex **first**, and only then does
> `sendPrompt` post and await the barrier. "At `beginPromptTurn`" means *at that call site in
> `sendPrompt`*, immediately after it returns — never *inside* it.
>
> The no-deadlock property comes from **holding no lock across the barrier**, not from the worker
> having a single blocking call. The worker has two: the downstream stream send, and `asyncTurnMu`
> via `maybeScheduleAsyncTurnComplete`. Any future callback the barrier runs is subject to the
> same rule.

AC-41b's stamp-before-dispatch clause is unchanged by this; the rule constrains *where the wait
sits*, not what the callback writes.

**Reading (c) is called out explicitly because it fails in the expensive direction and no
criterion caught it.** If the stamp lands after `conn.Prompt`, a workload the new turn spawns in
the gap has a start time *earlier* than the recorded turn start, sorts before the baseline, and the
probe answers `settled` — the exact direction D5's truncate-down and inclusive `>=` rules exist to
avoid. AC-80 pins the truncation, not the stamp placement, so nothing else guards this. AC-41b now
does.

**The cost of (a) is real and is recorded here rather than discovered later**, in the same spirit
as D2's second-cost note. `syncNotifQueueThen` states *"No caller-context honored … Only adapter
shutdown via lifetimeCtx can release the wait early"* (`adapter_updates.go:64-80`), so a prompt
dispatch now waits for the notification queue to drain, and a slow `updatesCh` consumer delays
**every** `session/prompt` — human, steer and synthetic alike. This is accepted: the queue is
drained by a worker whose fast path is "nanoseconds-to-microseconds" per item by its own
documentation, agentctl already blocks on this same primitive before every `EventTypeComplete`
emit and on the session-load path, and the alternative readings are (b) drift or (c) a wrong
answer. It is bounded by adapter lifetime, not by the probe budget.

**It is not a deadlock *provided the rule above is followed*** — the wait holds no adapter mutex,
so the worker can always reach the barrier item. The earlier, unconditional version of this
sentence ("the worker's only blocking call is the downstream stream send") was wrong on its facts
and is corrected above: the worker also acquires `asyncTurnMu`. The freedom from deadlock is a
consequence of the locking rule, not a property of the primitive.

**When the barrier fails — STATED 2026-08-09.** `syncNotifQueueThen` returns `bool`, and returns
**`false`** when `lifetimeCtx` is done, i.e. the adapter is shutting down. The previous revision
named the primitive without naming its failure return, leaving three observably different choices
at teardown, one of which (stamp locally, skip the emission) desynchronises the two sides D3 names
one boundary to keep together.

> A `false` return means **neither** write happened: no turn start is stamped and no `turn_started`
> is emitted. The prompt dispatch proceeds or fails on its own terms — this feature does not gate
> `conn.Prompt` on the barrier's success — and the session is tearing down regardless. The
> consequences are already specified and both are cheap: an unstamped turn start yields `unknown`
> from the probe (*Failure modes*, "agentctl holds no recorded turn start"), and an unsent
> `turn_started` degrades to today's behaviour (the fourth bullet below). A builder MUST NOT stamp
> the turn start when the barrier returns `false`, because a stamp without its matching event is
> the one combination that lets the two sides drift.

**A `true` RETURN DOES NOT PROVE THE CALLBACK RAN, AND THE RULE IS THEREFORE STATED ON THE
CALLBACK RATHER THAN ON THE RETURN VALUE — CORRECTED 2026-08-09.** The paragraph above keys
everything on the `false` return, which reads as though `false` were the only way the writes can
be skipped. It is not. `runUpdateWorker` skips the callback when shutdown wins **after** dequeue
but closes the sync channel regardless:

```go
case item := <-a.notifQueue:
    if item.sync != nil {
        if item.afterBarrier != nil && a.lifetimeCtx.Err() == nil {
            item.afterBarrier()
        }
        close(item.sync)
```

The waiter then selects between `<-a.lifetimeCtx.Done()` (→ `false`) and `<-ch` (→ `true`) with
**both cases ready**, and Go chooses among ready cases at random. So a `true` return is
reachable with `afterBarrier` never having run.

> The contract is on the **callback**, not on the return value: **both writes live inside
> `afterBarrier` and neither is ever performed outside it.** A skipped callback is therefore
> indistinguishable from a `false` return — no turn start is stamped, no `turn_started` is
> emitted — and a later probe answers `unknown`. A builder MUST NOT read a `true` return as
> "the stamp is now written", and MUST NOT stamp outside the callback on the strength of it.

This is why the rule is expressed that way rather than as "on `false`, do neither": stating it on
the return value leaves the reachable skipped-callback path unspecified, and the one
implementation it invites — stamp outside the callback when the barrier reports success — is
exactly the stamp-without-event drift D3 names one boundary to prevent.

AC-79a observes the ordering directly, with the attestation queued before the dispatch; AC-41b
observes the stamp thread, the stamp-before-dispatch guarantee, and the barrier rule in both its
forms — a `false` return and a callback skipped at shutdown.

Four further properties are contract:

- **It is emitted at the same point agentctl stamps its own recorded turn start**, so the
  attestation-clearing boundary and the probe's baseline cannot drift apart. That is the whole
  reason D3 names one boundary rather than two.
- **It is emitted on the human and the synthetic path alike.** `sendPrompt` distinguishes them
  via `humanPrompt := expectSession == ""` (`adapter_prompt.go:79`); this event does **not**.
  AC-41 asserts both halves.
- **Repeats are idempotent IN `observed_detached` AND NOT IN `turn_marker`, and the distinction is
  contract — CORRECTED 2026-08-09.** The previous revision said flatly that "two `turn_started`
  events for one session leave the same state as one", which **contradicts *Data model →
  `turn_marker`***: that field "increments on every `turn_started` received for that session, and
  on nothing else". Both cannot hold, and the difference is observable rather than academic —
  D2's revalidation discards any in-flight sample whose captured `turn_marker` no longer matches at
  completion, so a duplicated or redelivered `turn_started` throws away a completed probe. Against
  §J's measured twelve-minute parked window that is a real un-park delay of up to one sampling
  interval, not a theoretical one. The rule, stated once:
  > Clearing `observed_detached` is a set-to-false and **is** idempotent: two events leave it
  > exactly as one does. `turn_marker` is **not** idempotent and **increments on every**
  > `turn_started`, duplicates included. That is deliberate — the marker's whole job is to answer
  > "did a turn boundary occur while this sample was in flight", and a duplicate event is
  > indistinguishable from a genuine second boundary at the point where the question is asked.
  > Discarding a sample is the cheap outcome (nothing is written, no transition occurs, no revision
  > moves — *D2*), so erring toward discard is the safe direction.

  An event for a session the backend has no `parkedState` for is ignored, not an error, and creates
  no entry — no marker exists to increment. AC-41a's fourth clause pins both halves.
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
  Probe(ctx, kandevSessionID) (live | settled | unknown, error)
```

**WHICH SESSION ID — NAMED 2026-08-09, and it is the one namespace question this spec pinned
everywhere else and missed here.** The previous revision wrote a bare `sessionID` on the port, on
`Client.ProbeBackgroundWorkloads`, and on the wire, while three facts pull in different
directions: `parkedState` and the sampling loop — the port's only callers — are keyed by the
**Kandev task-session** id; agentctl's `ProbeBackgroundWorkloads` is explicitly the **ACP**
session id, and *Background-workload liveness probe* warns in its own words that *"a builder who
passes the Kandev session id gets `ok == false` for every call, which is the inert-but-green
failure this paragraph exists to prevent"*; and the existing `Client` actions split both ways —
`LoadSession` and `SetMode` carry an **ACP** `session_id`, while `Cancel` and `GetAgentStderr`
carry **none at all**, because the client is per-execution. Nothing said who translates. The
answer is stated so nobody guesses:

> **The port takes the KANDEV task-session id**, because its only callers hold that and nothing
> else. **The translation to the ACP session id is performed by `lifecycle.Manager`** — the
> component that already owns the Kandev-session→execution mapping and already applies the
> session-access guard this action reaches agentctl through (*Permissions*). It is **not**
> performed by the projection, the sampling loop, agentctl, or the agentctl `Client`.
>
> **WHERE THE NAMESPACE CHANGES — the three layers, named separately, CORRECTED 2026-08-09.** The
> previous revision put `kandevSessionID` on `Client.ProbeBackgroundWorkloads` *and* required
> `lifecycle.Manager` to translate "before the frame is built". Those cannot both hold, because
> **the `Client` is what builds the frame**: `Client.LoadSession` marshals `session_id` into the
> payload inline and hands it to `sendStreamRequest` (`internal/agent/runtime/agentctl/agent.go:126-132`,
> `client_stream.go:26`), and the `Client` is per-execution with no session mapping of its own.
> `lifecycle.Manager` sits *above* it and calls `execution.agentctl.<Method>(ctx, …)` after
> resolving the execution — the `RespondToPermission` precedent this action's guard follows
> (`manager_interaction.go:1520-1552`; note that `Client` method carries no session id at all).
> So the layers are:
>
> | Layer | Takes | Why |
> |---|---|---|
> | `BackgroundProbe.Probe(ctx, kandevSessionID)` — the port | **Kandev** task-session id | its only callers are `parkedState` and the sampling loop, which hold nothing else |
> | `lifecycle.Manager`'s probe method | **Kandev** task-session id | it owns the Kandev-session→execution mapping and the session-access guard; it resolves the execution and reads `execution.ACPSessionID` |
> | `Client.ProbeBackgroundWorkloads(ctx, acpSessionID)` | **ACP** session id | it builds the frame, exactly as `LoadSession(ctx, sessionID, …)` and `SetMode(ctx, sessionID, …)` already do |
>
> The `Client` method therefore takes the **already-translated ACP id**, and its parameter is named
> `acpSessionID`. The production implementation of the port is `lifecycle.Manager`'s method — not
> the `Client` method directly — because the translation and the guard are the Manager's.
>

> The `agent.background.probe` request therefore carries the **ACP** `session_id` on the wire, as
> every other session-carrying agentctl action does, and agentctl's handler passes it to
> `AgentPID(sessionID)` unchanged — which is what makes that accessor's stated ACP-only contract
> reachable rather than a trap.
>
> **When the Kandev session cannot be translated** — no execution, no live agentctl attachment, or
> the session is unknown — the result is **`unknown`**, never `settled`, and **no request is put on
> the wire**. This is the same direction as the `AgentPID` → `ok == false` row and is covered by
> the *Probe transport* failure table's `ErrAgentStreamNotConnected` row.

This is why every parameter above is named for its namespace rather than a bare `sessionID`: an
unqualified name is exactly what let two readings coexist for three revisions, and naming only the
port's left the `Client`'s in the same trap one layer down. AC-45 asserts the wire end and AC-46
asserts the untranslatable case.

- The parked projection and the sampling loop depend on **this port**, never on the agentctl
  `Client` directly. The production implementation is `lifecycle.Manager`'s probe method, which
  translates and then calls `Client.ProbeBackgroundWorkloads` (*Probe transport*); a test
  implementation returns a scripted sequence of the three literals.
- The port's contract is the probe's contract: exactly three result values, and **every** error
  resolves to `unknown` (*Probe transport*, failure table).
- **A non-nil error resolves to `unknown` regardless of the value returned beside it**, and the
  caller MUST check the error first. Saying only that such a pair is "outside the contract"
  leaves the caller's behaviour undefined at exactly the point where getting it wrong parks a
  session on a failed sample — the one direction this spec never takes. The caller therefore
  never reads the value when the error is non-nil, and a result outside the three literals is
  `unknown` even with a nil error.
- **A nil error and a well-formed response return that exact literal, unchanged.** The port never
  downgrades a successful `live` or `settled` to `unknown`. This is stated because the failure
  mapping alone is satisfiable by a port that answers `unknown` to everything — the inert-but-green
  direction — and AC-45's success clause is what observes it.
- **One invocation, at most one wire request.** A `Probe` call performs **no retry** and **no
  coalescing**: two concurrent calls for the same session are independent invocations, each
  emitting at most one frame, and each resolving on its own. Serialization of the resulting *walks*
  is agentctl's business (D6, AC-81b), not the port's. An implementation that retried would
  multiply the probe budget it is explicitly not allowed to extend.
- **Empty and already-cancelled inputs resolve to `unknown` with nothing on the wire.** An empty
  Kandev task-session id, and a caller context already done on entry, both yield `unknown` before
  any frame is built — the same direction as an untranslatable session. AC-46's ninth and tenth
  conditions observe them.
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

**WHICH PROCESS OWNS THE REGISTRY — ADDED 2026-08-09, and it is not the obvious one.** An
earlier revision defined the seam without saying where it lives or how its key is obtained.
Both answers are forced by evidence, and the obvious reading of "the backend keys a registry by
agent ID" is **prohibited** by a shipped ADR.

> **The registry lives in agentctl. The backend never keys anything by vendor.**

- **agentctl side — the recogniser.** The registry sits at the point that already dispatches on
  vendor: `stampBackgroundShellWork` (`.../transport/acp/normalize.go:304-307`), called from
  `normalize.go:131` and `:243`. Its key is the **agentctl-internal agent id** the normalizer
  already holds as `n.agentID` — for Claude, the package constant
  `claudeAgentID = "claude-acp"` (`normalize.go:47`). The Claude recogniser's body is today's
  condition, unchanged: `payload.ShellExec() != nil && payload.ShellExec().Background`
  (`normalize.go:305`). A recogniser's only effect is to call
  `payload.SetBackgroundWorkIdentity(BackgroundWorkKindShell, "", /*detached=*/true, false)`.
- **The wire — a typed, vendor-neutral attestation.** The stamp travels as
  `BackgroundWorkPayload` on the normalized payload, and **it survives the process boundary**:
  `NormalizedPayload` has custom marshalling and `background_work` is carried in **both**
  directions (`tool_payload.go:101` and `:117` for `MarshalJSON`, `:138` and `:158` for
  `UnmarshalJSON`). This was verified rather than assumed, because the struct's fields are
  unexported and a reader would reasonably expect them to be dropped. Had they been, the feature
  would have been dead in exactly the Docker/SSH deployments the *Timing* section targets.
- **Backend side — no vendor knowledge at all, but the KIND is filtered.** On the single ordered
  consumer the backend reads the existing vendor-neutral predicate
  **`NormalizedPayload.IsDetachedBackgroundLaunch()`** (`types/streams/background_work.go`,
  which is `IsActiveBackgroundWork() && backgroundWork.Detached`) **and additionally requires
  `BackgroundWork().Kind == streams.BackgroundWorkKindShell`** before setting
  `observed_detached`. It does **not** look at any agent name, agent id, or profile.

> **The `Kind == shell` filter is CONTRACT, added 2026-08-09, and without it this spec is wrong
> against the tree it ships into.**

An earlier revision told the backend to read `IsDetachedBackgroundLaunch()` unfiltered and claimed
the recogniser registry gated it — that an agent with no registered recogniser "produces no
attestation … **by construction**". **Measured false.** `stampBackgroundShellWork` is not the only
producer of `Detached=true`. `stampSubagentBackgroundWork`
(`.../transport/acp/normalize.go`) is a **separate** vendor-dispatch site: it sets
`Detached = subagent.IsAsync` with `Kind = BackgroundWorkKindSubagent`, and its guard admits
**`mockAgentID` ("mock-agent") as well as `claudeAgentID`** — the agent every `dev` and `e2e`
profile runs, and the one `/detached-background` in `cmd/mock-agent` drives through
`launchAsyncSubagentTool`. So `IsDetachedBackgroundLaunch()` is **already true today** for an async
subagent launch under an agent this spec never registers a recogniser for.

Three concrete consequences of leaving it unfiltered, all of which the filter removes:

- **AC-37 would be false against the shipped tree.** Its "never probed" clause fails for
  mock-agent: `observed_detached` would be set, so a probe *is* taken on every such settle.
- **D7's "by construction" guarantee would be false**, so the seam's central extensibility claim
  would rest on a mechanism that does not gate what it claims to gate.
- **AC-73's end-to-end sequence and AC-37 would assert opposite things about the same profile.**

**Why shell and not subagent — the decision, with its reasoning, so it is reviewable.** This is a
scope decision the spec makes rather than defers, because the evidence settles it:

1. The *Why* section scopes this feature to "parked on a background **shell** it will come back
   to". Subagents were never the indistinguishable pair.
2. **The probe cannot see a subagent.** The liveness predicate is a transitive-descendant walk
   over OS processes; an async subagent is an in-agent construct with no process of its own, so
   every probe for one answers `settled` (or `unknown`). Attesting subagents would buy a
   cross-process probe per settle and could never produce `live` — cost with no signal.
3. **Subagents already have a working completion signal, and shells do not.** That asymmetry is
   the whole reason §H exists: `stampSubagentBackgroundWork` sets `Ended` from
   `subagentStatusTerminal(subagent.Status)`, so subagent completion *is* observable from frames.
   §H's objection — an edge-driven refcount that only increments reliably — is specific to
   shells, whose completion frame carries no correlatable work id. Different problem, different
   mechanism; this spec is the mechanism for the one that has no frame.

`BackgroundWorkKindShell` is a **kind constant in the shared `streams` package**, not an agent
identity, so reading it does not reintroduce the vendor coupling ADR-0049 forbids and does not
weaken AC-69a — the backend still compares no agent name, agent id, or profile. Extending the
projection to a second *kind* later is a deliberate contract change, not a silent one.

**Why the backend must not key on agent identity.** `AgentStreamEventPayload.AgentID` is
documented *"Historical: execution.ID. Prefer ExecutionID"*
(`internal/agent/runtime/lifecycle/event_types.go:225`) — it is an execution id, not an
agent-type slug, so a registry keyed on it would never match and AC-37 would pass **vacuously**
while AC-21 failed. The available alternative, `session.AgentProfileSnapshot["agent_name"]`
(`workflow_session_config.go:214-219`), is a **central agent-name whitelist**, and
**ADR-0049 rejects exactly that**: *"an agent identity does not prove the installed provider
version advertises what Kandev expects"* (`turn_activity.go:1154-1157`). The same repository
convention is stated in `internal/agentctl/AGENTS.md` — prompt handoff and steering gate on a
negotiated advertisement, *"never on `agentID`"*. Putting the registry in agentctl keeps the
backend on the right side of that decision, because agentctl is the one process that legitimately
knows which vendor's wire format it is parsing.

```
BackgroundLaunchRecognizer            (registered in agentctl)
  AgentID() string                    // matched against the normalizer's n.agentID
  RecognizesDetachedLaunch(payload *streams.NormalizedPayload) bool
```

- Recognisers are held in a **registry keyed by agent ID**, exposing a public registration
  function. At ship time exactly **one** is registered: Claude, as above.
- An agent with **no registered recogniser** produces no *shell* attestation: nothing stamps
  `Kind=shell, Detached=true`, so the backend's filtered predicate stays false, the session is
  never probed (AC-54, AC-40a) and never parked (AC-37). **This holds by the combination of the
  registry miss AND the `Kind == shell` filter — not by the registry alone.** The registry-only
  claim was measured false: `stampSubagentBackgroundWork` stamps `Detached=true` with
  `Kind=subagent` for `mock-agent`, which has no recogniser. Both halves are load-bearing and
  both must survive any future registration.
- **Adding a vendor is registering a recogniser in agentctl and nothing else.** It MUST NOT
  require changing the probe, the parked projection, the backend consumer, or any icon call site.
  AC-69 asserts this by registering a second recogniser **through the public registration API,
  from test code only**.
- The registration function MUST be reachable from a test package without importing anything
  under the probe, projection, or rendering paths. That reachability is what makes AC-69
  mechanically checkable rather than a claim about a diff.
- **Registration is process-start and single-valued** (D7): at most one recogniser per agent id;
  a duplicate registration for the same id is a programming error, not a runtime merge; a nil
  recogniser or an empty `AgentID()` is rejected at registration; a recogniser that panics is
  treated as "did not recognise".
- The seam is the natural home for a future PID-carrying attestation. §I records that
  `BackgroundWorkPayload` carries no PID today; if a vendor can supply one, it enters here and
  narrows the probe from a time-scoped predicate to an exact lookup **without** changing the
  probe's contract or its three result values.

Why this is a small change and not a rewrite: per §L the probe is already entirely
provider-agnostic — it walks a process tree that exists for every executor and every agent —
and the Claude-specific part is one predicate over one payload, at a call site that already
exists and already dispatches on exactly this key.

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

**Where the walk is rooted — RE-SITED 2026-08-09 onto the right component.** The predicate above
says the walk starts at "the agent process", which is not something the probe's caller holds.
An earlier revision pointed at `server/process/runner.go` and its `proc.pgid = cmd.Process.Pid`
line. **That is `ProcessRunner`, which manages *workspace* processes, not the agent** (§I). A
builder following it roots the walk at a user-launched shell or VS Code process, or — more likely,
since `ProcessRunner.List(sessionID)` filters on a Kandev task-session id that will not match —
finds nothing and returns `unknown` forever, leaving the feature inert with a green build.

The agent process belongs to **`process.Manager`**, which spawns it as its own single `m.cmd`
and already exposes the pid internally as the unexported `Manager.agentPID()` (§I).

> The probe resolves its root through a **named accessor on `process.Manager`** — the component
> that launched the agent — of the shape `AgentPID(sessionID) (pid int, ok bool)`. It returns the
> pid of **that manager's own agent process** (`m.cmd.Process.Pid`, the value `agentPID()` already
> yields), and `ok == false` yields **`unknown`**, never `settled`, matching the *Failure modes*
> row for an exited agent process.

**Which `sessionID` it takes is contract, because two namespaces are in play.** The argument is
the **ACP session id** — the value the adapter holds and the one `Manager.GetSessionID()` returns
— **not** the Kandev task-session id that `ProcessRunner.List(sessionID)` filters on. The
accessor returns `ok == false` when the requested ACP session is not the one this manager's agent
is currently serving (per D6, one agent process serves one active session at a time), when the
manager has no agent running, or when the process has already been reaped. A builder who passes
the Kandev session id gets `ok == false` for every call, which is the inert-but-green failure this
paragraph exists to prevent.

The accessor returns a **pid only**. It deliberately does not return a process group, so the
forbidden predicate is not reachable through this seam at all.

**Turn start is recorded by agentctl, in agentctl's own clock.** agentctl stamps the current
turn's start from `beginPromptTurn` (`adapter_prompt.go:140`) — after the prompt gate and after
the pinned-session drop check, and therefore on all three of `sendPrompt`'s callers including the
synthetic `ScheduleWakeup` one — and clears it on session teardown. Both sides of the comparison
are therefore read on the **same host and the same clock**, so no cross-process skew exists and no
timestamp travels on the wire. If no turn start is recorded (agentctl restarted mid-turn), the
result is `unknown`.

**The stamp is written on the UPDATE WORKER, inside the same `syncNotifQueueThen` barrier callback
that emits `turn_started`, and `sendPrompt` blocks until it has run — CLARIFIED 2026-08-09.** An
earlier revision said the stamp happened "at `beginPromptTurn` … the same instant that emits
`turn_started`", which reads as the *prompt* goroutine and contradicted the ordering rule stated
under *API surface → Turn-boundary stream event*. The two sentences named two different threads
and a builder could satisfy both readings three ways, one of which stamps **after** `conn.Prompt`
and makes the probe answer `settled` for a workload the new turn spawned — the expensive
direction. That section is now authoritative and names the thread, the blocking behaviour, and the
barrier-failure rule; this paragraph defers to it. What matters here is only the guarantee it
buys: **the recorded turn start is always written before `conn.Prompt`**, so no in-turn workload
can sort before the baseline.

**Start-time source and resolution — named per platform, because they are not interchangeable.**
An earlier revision offered two Darwin sources as if equivalent. They are not, and the
difference breaks the predicate's own stated failure direction:

| Platform | Required source | Resolution | Result |
|---|---|---|---|
| Linux | `/proc/<pid>/stat` — field 4 `ppid`, field 22 `starttime` (clock ticks since boot, resolved against `/proc/uptime` or `sysconf(_SC_CLK_TCK)`) | ≥ 10 ms | `live` / `settled` |
| Darwin/BSD | `sysctl KERN_PROC_ALL` → `kinfo_proc.kp_proc.p_starttime`, a `timeval` | 1 µs | `live` / `settled` |
| Windows | Not implemented | — | always `unknown` |

**Which of those rows CI actually enforces — STATED 2026-08-09.** The *Notes for
implementation* require AC-70, AC-70a, AC-71, AC-72 and AC-80 to exercise a real process tree,
and AC-80 names Linux **and** Darwin. Verified: `.github/workflows/backend-tests.yml` runs
`ubuntu-latest` and `windows-latest` and has **no macOS runner**, so absent a decision the
Darwin half — the `sysctl` path, which is the more fragile of the two and the one
`ps -eo lstart` would silently substitute for — would ship enforced by nothing. Adding a macOS
CI job is out of scope for this card. So the contract is:

- **Linux** — the real-process-tree criteria are **CI-enforced** on the existing
  `ubuntu-latest` backend job. This is the gate.
- **Darwin** — the same criteria run as a **host-gated** test: skipped when `runtime.GOOS` is
  not `darwin`, and therefore green-by-skip in CI and genuinely executed on a maintainer's
  machine. The skip MUST be an explicit `t.Skip` naming the platform, never a silent pass, so
  a reader of the CI log can tell the difference between "ran and passed" and "did not run".
- **Windows** — always `unknown`; no process-tree test.

AC-80 is restated below against this split so its Darwin half is a real, locatable obligation
rather than an unenforceable sentence. The residual risk is named and accepted: a Darwin-only
regression can reach `main` and is caught on first local run, not in CI.

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

**The clock DOMAIN, stated per platform — ADDED 2026-08-09.** D5 says both timestamps are read
"on agentctl's host in agentctl's clock", which is true of the *host* but not of the *domain*:

| Platform | Process start time is in | Turn start is in | Bridging |
|---|---|---|---|
| Linux | ticks since **boot** (`/proc/<pid>/stat` field 22) | wall clock (a Go `time.Time`) | needs a boot-time anchor |
| Darwin/BSD | wall clock (`p_starttime`, a `timeval`) | wall clock | none needed — same domain |

Darwin is already same-domain and needs no rule. **Linux is not**, and the bridge is where a
builder would otherwise invent a policy:

> On Linux the comparison is performed **entirely in the boot domain**. The recorded turn start is
> converted *into* boot-relative ticks once, at the moment agentctl stamps it, using the boot
> anchor read at that same moment; the probe then compares two boot-domain values and never
> converts a process start time into wall time. This is the direction that is immune to a clock
> adjustment between boot and the sample: an NTP step moves the wall clock and the anchor
> together, and a comparison already expressed in ticks is unaffected. Converting the other way —
> process ticks into wall time at probe time — re-reads the anchor after the step and shifts every
> process's apparent start, which can push an in-turn process before the turn start and yield
> `settled`, the expensive direction D5 exists to avoid.

If the boot anchor cannot be read, the result is **`unknown`**, never `settled`. The tick
frequency itself (`sysconf(_SC_CLK_TCK)`) supplies only the unit and is not an anchor; both are
required.

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
| **Request** | `{ "session_id": "…" }` — the **ACP** session id, translated from the Kandev task-session id by `lifecycle.Manager` before the frame is built (*Probe port*) |
| **Response** | `{ "result": "live" \| "settled" \| "unknown" }` |
| **Dispatch** | a new `case` in the `server/api/agent.go` action switch |
| **Backend entry point** | Two layers, and the namespace changes between them (*Probe port*). `lifecycle.Manager`'s probe method takes the **Kandev** task-session id, applies the session-access guard, resolves the execution, and reads `execution.ACPSessionID`; it then calls `Client.ProbeBackgroundWorkloads(ctx, acpSessionID) (ProbeResult, error)`, which builds the frame via `sendStreamRequest`. The `Client` performs no translation and holds no session mapping |

The request carries no timestamp: agentctl holds the turn start itself, which is what removes
the clock-skew question entirely.

`sendStreamRequest` takes its deadline from the caller's `ctx`, so the probe budget is applied
by the backend as a `context.WithTimeout` around the call. No timeout is baked into the
transport.

**agentctl needs its OWN bound, because the backend's deadline does not reach it — ADDED
2026-08-09.** The backend's `context.WithTimeout` governs only its own wait: `sendStreamRequest`
registers a pending-response channel and `select`s on it against `ctx.Done()`, so a timeout
abandons the *wait*. The request itself is a WS frame carrying `{session_id}` and no deadline, so
agentctl's handler keeps running. Combined with D6's rule that concurrent probes for one session
are serialized by agentctl, an abandoned-but-still-running probe can hold that serialization and
make every subsequent probe for the session queue behind it and time out too — turning a transient
`unknown` into a permanent one, with the sampling loop dutifully re-issuing a probe that can never
answer.

> The agentctl handler for `agent.background.probe` applies its **own** bound, from the same
> `KANDEV_PARKED_PROBE_BUDGET` default, and abandons the walk when it elapses, answering
> `unknown`. It never holds the per-session probe serialization past that bound. The two bounds
> are independent by design — the backend's protects turn settlement, agentctl's protects the
> serialization — and neither is derived from the other, so a version-skewed pair still degrades
> to `unknown` rather than stalling.

Because the budget is now read by both processes, the *Timing* table's "Read by" column names
both for this key.

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
| **Probe budget** — D2's bound on the synchronous first sample | `KANDEV_PARKED_PROBE_BUDGET` | `250ms` | **backend AND agentctl**, independently (*Probe transport*) | **rejected — see below** |
| **Sampling interval** — how often a parked session is re-probed | `KANDEV_PARKED_PROBE_INTERVAL` | `30s` | **backend** | disables periodic sampling; the projection then never clears via the probe (see below) |

**A NEGATIVE sampling interval is rejected, exactly like a negative budget — ADDED 2026-08-09.**
`0` was defined (AC-74) and the budget's non-positive case was defined (AC-81), but the interval's
*negative* case was left silent between them, and the three readings a builder could pick differ
observably: follow the `0` row and disable; follow the budget row and reject-and-default; or pass
the value through to `time.NewTicker`, which **panics on a non-positive duration** and takes the
backend down at boot. So: a **negative** `KANDEV_PARKED_PROBE_INTERVAL` is rejected at config load,
logged at warn, and the `30s` default is used. **Exactly `0` keeps its documented meaning** —
disable periodic sampling — because that is a deliberate operator choice AC-74 already covers.
AC-81a observes both halves.

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
| the session is stopped or deleted, or reaches a terminal state | term 3 |
| **its execution ends, or its agent context is reset** | **neither — the row is EVICTED, and eviction performs the `true → false` transition itself before removing the row** (*Data model → Entry lifecycle*). Listing this as "term 3" was wrong: an execution can end while the session is still `WAITING_FOR_INPUT`, so no term flips. Corrected 2026-08-09; AC-85 observes it |
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
| agentctl crashes, disconnects, or is version-skewed while a session is parked | The affordance clears exactly once, by whichever of two paths fires first, and the other is then a no-op because `parked` is already `false`. **(a)** A sample completes and the probe call fails → `unknown` → un-parked through term 2. **(b)** The session's execution ends first, so no further sample is taken (AC-53) → the row is evicted, and **eviction performs the `true → false` transition itself** (*Entry lifecycle*). *Clarified 2026-08-09: this row previously named only (a), while AC-53 stops sampling on execution end — so which one obtained was a race between two spec-stated behaviours, and under the old eviction rule branch (b) cleared nothing at all.* |
| A session's execution ends, or its agent context is reset, while the session is still `WAITING_FOR_INPUT` and parked | The row is evicted, and eviction first publishes the `true → false` transition with a strictly higher `revision` (*Entry lifecycle*). The affordance clears. It is **not** left to the session-state term, which does not flip on either cause. |
| Probe reports `live` but the workload is a leaked orphan | The affordance persists until the session leaves `WAITING_FOR_INPUT`. Accepted: it costs a stale spinner, not a stuck notification, because this spec withholds nothing. |
| A long-lived process unrelated to the workload starts mid-turn (e.g. a lazily-connected stdio MCP server) | Counted as `live`. This is a **false "still busy"**, the cheap direction. Preferred over any command-name heuristic, which would be fragile and vendor-specific. §L measures that this is a real, common case, and **AC-70a observes it** rather than leaving it as prose. |
| Detached launch observed but the agent has no registered recogniser | `observed_detached` is false; behaviour unchanged, and no probe is taken. |
| A detached launch is stamped with a kind **other than** `shell` — today, `Kind=subagent` from `stampSubagentBackgroundWork`, which fires for `mock-agent` as well as Claude | `observed_detached` is false; no probe, never parked. The backend's predicate is `IsDetachedBackgroundLaunch() && Kind == BackgroundWorkKindShell`. |
| `turn_started` arrives for a session with no `parkedState` row | Ignored; no row is created and no marker exists to increment. |
| `turn_started` arrives after its own execution was tombstoned (cancellation marks the owner terminal before teardown) | Dropped by `shouldDropCompletedExecutionStreamEvent`, the consumer's second gate. `observed_detached` stays set; the row is evicted on execution end anyway. Accepted — the bias is toward a stale affordance, never a wrongly-appearing one. |
| agentctl's own probe budget elapses mid-walk | The walk is abandoned, the answer is `unknown`, and the per-session probe serialization is released so the next probe is admitted (AC-81b). |
| The event-bus publish of a parked transition fails | Value and revision stand; no retry, no rollback. The next carrier for that entity re-serializes the current triple and the consumer accepts it, since the revision only moves forward. |
| Linux boot-time anchor cannot be read | `unknown`, never `settled` — the tick source alone supplies the unit, not the origin. |
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
common turn end pays nothing (AC-40a).

#### The settle seam — RE-SEATED 2026-08-09 onto the state write itself

Two earlier revisions named the wrong thing. The first said only "synchronously during
turn-settle handling", which names no instant. The second named
**`setSessionWaitingForInput`** and claimed "all 44 production call sites funnel through this
one function". **That claim is false, and it was measured false three separate ways:**

- The count is **43**, not 44.
- **`event_handlers_workflow.go` bypasses it.** Three sites write `WAITING_FOR_INPUT` directly
  through `updateTaskSessionState` without ever calling `setSessionWaitingForInput`: the
  stale-`RUNNING` flip ("no active turn registered"), and **two step-transition settles** that
  fire when a workflow step moves a `RUNNING`/`STARTING` session. §J's own observation is a
  Kandev **Verify** step — the originating scenario is on the bypassing path.
- **A second, independent `setSessionWaitingForInput` exists**, as a method on `*Handlers` in
  `internal/mcp/handlers/handlers.go`. It writes the session through its own repository and
  publishes `events.TaskSessionStateChanged` itself, never entering the orchestrator.

So the seam is moved one level down, to the function that actually performs the transition:

> **The seam is `updateTaskSessionStateWithHook`**
> (`internal/orchestrator/event_handlers_streaming.go`), and the projection hook runs at the
> **end** of it, gated on `changed == true && nextState == TaskSessionStateWaitingForInput`.

**Every** `WAITING_FOR_INPUT` write in package `orchestrator` funnels here — `updateTaskSessionState`
is a thin wrapper that calls it with a `nil` hook. Read from the tree, there are exactly **six**
such writes, and naming this one function covers all six:

| # | Site | What settles |
|---|---|---|
| 1 | `event_handlers_streaming.go`, `setSessionWaitingForInput` error-fallback branch | session lookup failed; state still written |
| 2 | `event_handlers_streaming.go`, `setSessionWaitingForInput` normal branch | stream turn completion |
| 3 | `event_handlers_workflow.go`, stale-`RUNNING` flip | no active turn registered |
| 4 | `event_handlers_workflow.go`, step-transition settle | **bypasses `setSessionWaitingForInput`** |
| 5 | `event_handlers_workflow.go`, step-transition settle (second site) | **bypasses `setSessionWaitingForInput`** |
| 6 | `event_handlers_transient.go`, transient-retry park | turn failed, retry scheduled |

Four properties of that placement are contract:

1. **After the CAS, so term 3 is guaranteed.** `updateTaskSessionStateWithHook` only reaches its
   tail after `persistTaskSessionState` reports `changed == true`, which is a
   compare-and-set (`UpdateTaskSessionStateIfCurrent`) against the session's previous state. The
   session is therefore committed at `WAITING_FOR_INPUT` before the projection is computed. A
   hook placed before the write reads the pre-settle state and can never park — the failure the
   previous revision correctly identified and then mis-sited.
2. **Once per transition, guaranteed by the CAS rather than by a read.** The function returns
   early when `session.State == nextState`, and the CAS itself rejects a second writer, so two
   concurrent settle paths on one session produce **exactly one** `changed == true`. This is
   what makes "at most one synchronous probe per settle transition" a property of the code
   rather than an aspiration: the previous seam derived it from an unsynchronised
   `wasAlreadyWaiting` read that two racing callers could both observe as `false`. No additional
   single-flight lock is required, and none is specified. AC-40 observes the bound; AC-54's
   "zero probes" for a session merely sitting in `WAITING_FOR_INPUT` follows from the same CAS.
3. **After the state-change publish, not in the existing `onChanged` callback.** The `onChanged`
   hook already on this function runs *before* `publishTaskSessionStateChanged`. Putting a
   probe there would delay `session.state_changed` by up to the whole probe budget on every
   attested turn. The parked value has its own carrier — `session.activity_changed`, per
   *API surface* — so nothing is lost by sampling after the publish, and turn-end latency for
   every existing consumer is unchanged.
4. **The probe is still gated on `observed_detached`**, so of the six sites only those whose
   settling turn actually attested a detached launch pay anything (AC-40a).

**The two non-turn-end callers are analysed rather than assumed, and neither can probe.**
`GetTaskSessionStatus` reaches the seam only through `shouldHealStuckStartingSession`, and
`ResetAgentContext` reaches it only to restore state after itself writing `STARTING`. Both are
therefore `STARTING → WAITING_FOR_INPUT` transitions, and a `STARTING` session has run no turn,
so `observed_detached` is false and **no probe is taken**. Additionally, an agent-context reset
discards the session's `parkedState` entry outright (*Data model → Entry lifecycle*), because the
turn it described no longer exists. AC-40's bound is nonetheless restated below against *any*
CAS-confirmed transition into `WAITING_FOR_INPUT`, not merely a turn end, so the guarantee does
not rest on this analysis staying true.

**The transient-retry park (site 6) IS in scope, deliberately.** The turn failed and a retry is
scheduled, but if a detached shell that turn launched is genuinely still alive then "a machine is
working" is true, and the uniform three-term rule gives the right answer. No carve-out.

**The MCP clarification path is EXCLUDED, and this is a named exclusion rather than an
oversight.** `Handlers.setSessionWaitingForInput` does not enter package `orchestrator` and
therefore does not reach this seam. It is excluded because it settles a session **in order to ask
a human a structured question**: `pending_action` is set on that path, and a pending clarification
outranks parked on every surface (AC-34, AC-51). The observable outcome is consequently identical
whether or not that path is hooked — the session renders `task-state-waiting-for-input`, which is
what this feature wants for it. Hooking it would add a cross-process probe to the MCP request path
for no rendering difference. AC-40b observes that no probe is issued there.

#### A sample must be revalidated before it is applied

D1's revision **increments only on a transition of `parked`**, so it cannot order two samples of
the same session: a turn-N probe completing during turn N+1 carries the *same* revision whenever
`parked` has not flipped in between, and the earlier "discard a sample carrying an older
revision" rule could never fire. §J measures a twelve-minute parked window, so a sample
outliving its turn is ordinary, not exotic.

The rule is therefore stated positively, against state the spec already defines:

> The sampler captures a **snapshot** at the instant it issues the probe — the session's state,
> `observed_detached`, and **`turn_marker`** — and re-reads the same three at completion. A
> completed sample is **applied only if all three are unchanged**: the session is still
> `WAITING_FOR_INPUT`; `observed_detached` is still true; and **`turn_marker` still equals the
> value captured at issue**. If any differs, the result is **discarded** — not recorded as
> `last_sample`, and not published. Discarding is not `unknown`: nothing is written at all, so no
> transition occurs and no revision moves.

The third clause is the load-bearing one and is why the boundary is defined once in D3: a new turn
invalidates the *question* the sample was asked, not merely its answer.

**It is expressed as a counter and not as "no `turn_started` was received", and that is the fix
for a rule that could not previously be implemented.** An earlier revision phrased the clause as
the absence of an event and asserted the check needed no new field. It does. `turn_started` is not
retained; `revision` does not move across a turn boundary that leaves `parked` unflipped; and
`observed_detached` **cannot witness the event** — a new turn that clears it and then re-attests a
detached launch inside the sample window leaves it `true`, so the stale sample passes all three
clauses and is applied to the wrong turn. `turn_marker` (*Data model*) is the monotonic per-session
counter that makes the comparison possible; it increments on every `turn_started` and on nothing
else, so a clear-and-re-attest inside the window still moves it.

Concurrent samples for one session are serialized (D6); a second sample that starts after the
first completes observes the first's applied result.

#### The synchronous probe's second cost, stated so it is not discovered later

The 250 ms budget bounds turn settlement (AC-40), and that is the cost the default was chosen
against. It has a second, smaller effect worth naming: the call is issued while the backend is
on its ordered agent-event path for that session, so an attested turn end can also delay that
session's subsequent event processing — and a concurrent cancel — by up to the budget. This is
**not** a deadlock: agentctl's read loop remains free to demux the response, which is what lets
the call return at all. It is bounded by the same budget, it happens only on turns that actually
attested a detached launch, and it is accepted. It is recorded here because a reader who meets
it later would reasonably file it as a cancel-latency regression rather than a known cost.

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
- `observed_detached` MUST be cleared, **and `turn_marker` incremented**, on **that same
  boundary** — i.e. on the `turn_started` event that dispatch emits (*API surface →
  Turn-boundary stream event*), not on the backend's own prompt-admission path
  (`startTurnForSession`, `service.go:1278`), which a synthetic prompt does not reach. §N
  verifies that no pre-existing event carries this boundary, which is why this spec publishes one
  rather than subscribing to one. The two writes happen together, under the session-level lock, on
  the ordered consumer — so a sample cannot observe the flag cleared but the marker unmoved.
  AC-41, AC-41a, AC-79 and AC-79a observe the halves.
- **If a continuation produces no `session/prompt` at all**, neither side advances: the baseline
  stays at the last dispatch and the attestation is not cleared. That is deliberate and is the
  cheap direction — the probe keeps comparing against an *earlier* baseline, so more processes
  count as in-turn and the answer biases toward `live`, i.e. toward a stale affordance rather
  than a missed one. It is stated so a builder does not "fix" it into the expensive direction.
- **The same is true when the event is emitted but dropped before the consumer applies it**, which
  the consumer's two gates make reachable: a stale execution's `turn_started` is rejected by the
  ownership filter, and an event relayed after its own execution was tombstoned is dropped by
  `shouldDropCompletedExecutionStreamEvent` (*API surface → Turn-boundary stream event*). Neither
  is an error and neither is worked around. `observed_detached` stays set, the boundary is simply
  not observed, and the bias is the same cheap direction as the bullet above.

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
- Both timestamps are read on agentctl's host, and the comparison is performed in **one clock
  domain** — the boot domain on Linux, the wall domain on Darwin (*Start-time source and
  resolution*). No timestamp crosses a process boundary.

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

- **It lives in agentctl**, at the `stampBackgroundShellWork` dispatch point
  (`.../transport/acp/normalize.go:304-307`), and is keyed by the agentctl-internal agent id the
  normalizer already holds. The backend holds no registry and compares no agent identity — see
  *API surface → The launch-recogniser seam* for why ADR-0049 forbids the alternative.
- Keyed by agent ID; **at most one** recogniser per agent ID. Registering a second for the same ID
  is a programming error, not a runtime merge.
- **Registration input validation:** a nil recogniser, or one whose `AgentID()` is empty or
  whitespace, is rejected at registration rather than stored — an entry that can never match is a
  silent no-op, which is the failure mode this seam is least able to detect.
- Lookup miss ⇒ nothing is stamped with `Kind=shell` ⇒ the backend's filtered predicate
  (`IsDetachedBackgroundLaunch() && Kind == BackgroundWorkKindShell`) stays false ⇒ no
  attestation, no probe, no parking. This is the default for every agent. **It does NOT hold by
  the registry alone** — `stampSubagentBackgroundWork` is an independent producer of
  `Detached=true` that admits `mockAgentID`, so the kind filter is the other half of the
  guarantee. See *API surface → The launch-recogniser seam* for the measurement.
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

**A transition of a term is not the ONLY way the projection clears — ADDED 2026-08-09.** There is
exactly one other, and it is not a fourth term: **eviction of the `parkedState` row**. Two of the
five eviction causes — the session's **execution ending** and an **agent-context reset** — can fire
while the session is still `WAITING_FOR_INPUT`, so no term flips and there is nothing for the
three-term formula to observe. *Data model → Entry lifecycle* therefore makes eviction perform the
`true → false` transition itself before removing the row, publishing with a strictly higher
`revision` exactly as a term transition does. From a consumer's side the two are indistinguishable,
which is the point: there is still exactly one way the affordance clears on the wire.

This is stated here because D8 is where a builder looks for the exhaustive list, and the previous
revision's list was exhaustive only over the *terms*. The distinction matters in one direction only
— a rowless session serializes `revision 0`, which the discard rule rejects against any applied
`N ≥ 1`, so an eviction that publishes nothing strands the client permanently inside the epoch.

### D9 — Defaults and boundary values

| Field / input | Default / boundary behaviour |
|---|---|
| `parked_on_background_work` | `false` when absent; absent and `false` are equivalent |
| parked revision (session) | `0` for a session with no recorded transition |
| parked revision (task) | own per-task counter; `0` for a task with no sessions or no recorded transition. **Not** the maximum over sessions |
| `parked_epoch` | the backend process's start time in Unix nanoseconds; `0` only from a peer that does not implement this spec, which sorts below every real epoch |
| probe with no sample yet | `unknown` until the first sample completes |
| probe budget | `250ms` (`KANDEV_PARKED_PROBE_BUDGET`); **`0` or negative is rejected**, default used. Read independently by the backend and by agentctl |
| probe sampling interval | `30s` (`KANDEV_PARKED_PROBE_INTERVAL`); `0` disables periodic sampling, and the projection then clears only via the session-state or attestation terms |
| probe sampling interval, **negative** | **rejected** at config load, warn-logged, `30s` default used — distinct from `0`, which is a valid operator choice |
| `turn_marker` | `uint64`, `0` for a session with no `turn_started` yet; increments on every `turn_started`; never persisted, never on a carrier, never compared across sessions |
| `parkedState` row that does not exist | created only by the first attested detached launch; a `turn_started`, a settle, or a probe for a session with no row is a **no-op**, not an error and not a creation |
| attested launch whose `Kind` is not `shell` | **not** an attestation — `observed_detached` stays false, no probe, never parked (covers today's `Kind=subagent` stamps, incl. `mock-agent`) |
| **eviction of a `parkedState` entry whose `parked` is `true`** | the eviction performs the `true → false` transition **first** — flip, increment `revision`, publish `session.activity_changed` — then applies the `members` removal, then **reduces** the entry (`revision` retained). Never reduce or drop a `true` entry silently, and never discard its `revision`: an entry that lost its revision serializes `revision 0`, which the consumer discards against any applied `N ≥ 1`, stranding the affordance inside the epoch |
| eviction of a `parkedState` row whose `parked` is already `false` | nothing is published; the row is simply removed |
| **eviction caused by backend shutdown**, in either case | nothing is published — no consumer remains, and every client reconnects against a strictly higher `parked_epoch` (AC-77) |
| **a second eviction cause firing for an already-evicted session** | a **no-op**, not an error: the reduced entry reads `parked = false`, so nothing flips and nothing publishes. Concurrent causes therefore produce exactly one un-park |
| **a session carrier for a REDUCED entry** | serializes `(parked=false, revision = the retained value, parked_epoch)` — **not** `revision 0`, which would move the revision backward and make a dropped eviction frame uncorrectable |
| a sample completing after its session's entry was **reduced** | **discarded** by D2's revalidation with no extra rule — the reduction cleared `observed_detached`, so it no longer matches the value captured at issue |
| **an attested detached launch for a session holding a REDUCED entry** | the entry is **revived in place**: `observed_detached` set, `parked` `false`, `turn_marker` `0`, `last_sample` `unknown` — and **`revision` continues from the retained value, never restarting at `0`**. Only an attestation revives; a `turn_started`, a settle or a read against a reduced entry is a no-op |
| task `members` entry for an evicted session | removed under the per-task lock, with the same recompute-compare-publish steps, so the task's `true → false` flip publishes exactly once |
| event-bus publish failure | value and revision stand; no retry, no rollback; corrected by the next carrier for that entity |
| `turn_started` with prompt generation `0` (every synthetic `ScheduleWakeup` dispatch) | **never rejected by the ownership filter** — it rejects only on a mismatch of two *present* values. Never synthesize a generation and never skip the emission. Still subject to the completed-execution gate, below |
| `turn_started` relayed from a superseded execution during a cancellation reconcile | **rejected** by the ownership filter on the relay-stamped `ExecutionID`; `observed_detached` is left untouched, which is the intended outcome |
| `turn_started` relayed from its **own** execution after that execution was tombstoned by `markExecutionFailed` (cancellation tombstones the owner before teardown) | **dropped** by `shouldDropCompletedExecutionStreamEvent`; `observed_detached` is left set. An accepted second drop path — the execution is ending, the `parkedState` row is evicted on execution end, and the bias is toward a stale affordance, never a wrong one |
| `turn_started` delivered twice for one session | `observed_detached` unchanged (idempotent); `turn_marker` increments **again** (not idempotent). A sample in flight across the duplicate is discarded per D2 |
| `syncNotifQueueThen` returns `false` (adapter shutting down) | **neither** the turn-start stamp nor the `turn_started` emission happens; a later probe yields `unknown`. Never stamp without emitting |
| a queued-but-unadmitted operator prompt | the projection is **unchanged** — all three terms still hold |
| agent with no registered recogniser | never attested, never probed, never parked |
| process start time exactly equal to the truncated turn start | counts as **in-turn** (inclusive comparison) |
| agent process exited | `unknown`, never `settled` |
| `AgentPID(sessionID)` returns `ok == false` | `unknown` — no walk is attempted |
| Kandev task-session id cannot be translated to an ACP session id (no execution, no live agentctl attachment, unknown session) | `unknown`, never `settled`, and **no frame is put on the wire**. The translation is `lifecycle.Manager`'s, not the projection's and not agentctl's |
| a sample completing after its turn ended, or after the session left `WAITING_FOR_INPUT` | **discarded** — not recorded as `last_sample`, not published, no revision moves. Distinct from `unknown`, which *is* recorded |
| recogniser registered with a nil value or an empty `AgentID()` | rejected at registration, never stored |
| a session transition that does not flip its task's OR | `session.activity_changed` only; the task's `parked_revision` does not move and no `task.updated` is published |
| a carrier that omits the parked fields for an entity already held | the three fields are left untouched, **not** reset to `false` |
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
  **none of the symbols this feature introduces** — the parked projection and its revision
  accessors, the probe port and its production implementation, the sampling loop, the recogniser
  registry, and the settle hook — references the `claudeBackgroundPromptHandoff` flag key, the
  `Features.ClaudeBackgroundPromptHandoff` config field, or the
  `claudeBackgroundPromptHandoffEnabled` / `claudeBackgroundPromptHandoffEnabledForSession`
  accessors; **and** a parked session's projection is computed identically with the flag forced on
  and forced off, for the same inputs.
  *(Restated twice. It first read "the parked projection was produced without reading that flag",
  which asserts a property of a diff. The 2026-08-09 restatement made it an architecture test but
  scoped it to **packages**, which Spec Review round 1 showed is **false by construction**: the
  projection must live in package `orchestrator` — D1 names `CancellationPendingSnapshot`
  (`task_operations.go:4478`) as its structural precedent and the settle hook is
  `updateTaskSessionStateWithHook` (`event_handlers_streaming.go`) — and that package legitimately
  references the flag at `service.go:61` and `turn_activity.go:1194 / :1214 / :1328` for the
  unrelated `ForegroundActivity` gate. At package granularity the assertion could never pass; a
  builder would have silently reinterpreted it at symbol granularity and the test would have
  proved nothing. Narrowed here to **symbol** granularity, which is what the requirement always
  meant — "this feature's code does not read that flag" — plus a behavioural clause that holds
  whatever the granularity. The contract is unchanged: the parked path is still independent of the
  flag, and the flag still stays off.)*
- **AC-36** — **GIVEN** a parked session, **WHEN** the backend restarts and the boot payload is
  built, **THEN** `parked_on_background_work` is `false`.
- **AC-37** — **GIVEN** an ACP agent with **no registered launch recogniser**, **WHEN** it settles a
  session after backgrounding work, **THEN** `parked_on_background_work` is `false`, **zero probes
  are issued for that session**, and every icon surface renders exactly as it does today;
  **and GIVEN** specifically the `mock-agent` used by the `dev` and `e2e` profiles, driven through
  `/detached-background` so that `stampSubagentBackgroundWork` stamps
  `Kind=subagent, Detached=true` and `IsDetachedBackgroundLaunch()` returns **true**, **THEN** the
  same three outcomes still hold — because the backend additionally requires
  `Kind == BackgroundWorkKindShell`.
  *(The second GIVEN was added 2026-08-09 and it is the regression guard for a defect that was
  live in the previous revision. `stampSubagentBackgroundWork` is an independent producer of
  `Detached=true` that admits `mockAgentID` as well as `claudeAgentID`, so an unfiltered backend
  predicate made this criterion FALSE against the shipped tree for the agent every dev and e2e
  profile runs — `observed_detached` would be set and a probe taken. The kind filter is what
  restores it; this clause is what stops the filter being dropped later as redundant.)*
- **AC-38** — **GIVEN** a session whose parked value transitions `false → true → false`, **WHEN**
  each carrier is serialized, **THEN** the boolean, its revision and `parked_epoch` are read from
  one critical section, the revision strictly increases across the two transitions, and re-deriving
  an unchanged value publishes nothing (D1).
- **AC-39** — **GIVEN** a consumer that has applied `(epoch E, revision N)` for a session, **WHEN**
  an update carrying `(E, N-1)` arrives for that session, **THEN** the consumer discards it and the
  displayed value is unchanged (D1).
- **AC-40** — **GIVEN** a session whose probe cannot complete a sample within
  `KANDEV_PARKED_PROBE_BUDGET`, **WHEN** **any** CAS-confirmed transition into
  `WAITING_FOR_INPUT` computes the projection — not only a turn end — **THEN** the result is
  treated as `unknown`, the session is not parked, and that transition is not delayed beyond the
  budget; **and** at most **one** synchronous probe is issued per transition even when two settle
  paths race the same session, which the state CAS guarantees rather than an added lock (D2).
  *(Widened 2026-08-09. The bound previously named "turn settlement", but the seam is the state
  write, which is also reached by `GetTaskSessionStatus`'s stale-`STARTING` heal and by
  `ResetAgentContext`'s restore. Both are `STARTING → WAITING_FOR_INPUT`, so `observed_detached`
  is false and neither actually probes — but the bound no longer depends on that remaining
  true. The single-probe clause replaces the previous seam's unsynchronised `wasAlreadyWaiting`
  read, which two racing callers could both observe as `false`.)*
- **AC-40b** — **GIVEN** a session settled to `WAITING_FOR_INPUT` by the **MCP clarification
  path** (`Handlers.setSessionWaitingForInput` in `internal/mcp/handlers`), which does not enter
  package `orchestrator`, **WHEN** that settle occurs, **THEN** **zero** probes are issued for
  that session, `parked_on_background_work` is `false`, and the row renders
  `task-state-waiting-for-input`. This is the named exclusion in D2's settle seam, asserted so it
  is a decision rather than an omission.
- **AC-39a** — **GIVEN** a consumer that has applied `(epoch E, revision N)` for a session, **WHEN**
  a frame arrives carrying `(E, N-1)` **together with a newer unrelated field** — a changed
  session state, or for a task a changed title — **THEN** the three parked fields keep their
  existing values **and every other field of the frame is applied normally**; and **GIVEN** a frame
  that omits the parked fields entirely for an entity already held, **THEN** those three fields are
  left untouched rather than reset to `false`. *(Added 2026-08-09. The field-scoped discard rule
  exists precisely so a stale triple cannot reject the rest of a frame, but no criterion paired a
  stale triple with a newer unrelated field — so a frame-scoped implementation, which is the bug
  the rule was written against, passed every existing criterion. The omission half mirrors
  `mergeTaskUpdate`'s existing `hasPayloadField` discipline.)*
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
  distinguishes via `humanPrompt` (`:79`) and the `steer` argument; **and GIVEN** specifically the
  synthetic path, whose `sendPrompt` call passes prompt generation `0`, **THEN** the event is
  still emitted, carries `prompt_generation: 0`, and is **not rejected by**
  `handleAgentStreamEvent`'s cancellation-ownership filter — including while a cancellation for
  that session is reconciling, **provided its own execution has not yet been tombstoned by
  `markExecutionFailed`** — so `observed_detached` is cleared on that path too; **and GIVEN** the
  same event but relayed **after** its execution was tombstoned, **THEN** it is dropped by
  `shouldDropCompletedExecutionStreamEvent` and `observed_detached` is left set, which is the
  accepted cheap-direction outcome and not a defect; **and GIVEN**
  the backend receives it, **THEN** `observed_detached` is cleared on the same ordered consumer
  that applies the attestation (`event_handlers_streaming.go:24`); **and GIVEN** a **second**
  `turn_started` for the same session, **THEN** `observed_detached` is unchanged (still cleared)
  **and `turn_marker` has incremented again** — the two fields differ deliberately, and a sample
  in flight across that duplicate is discarded per D2. **And GIVEN** a `turn_started` for a
  session with no `parkedState` row, **THEN** it is ignored, no row is created, and no marker
  exists to increment.
  *(Added 2026-08-09 with the event itself. §N verifies no pre-existing stream event marks a turn
  start, so D3's clearing rule had no carrier and this criterion is what stops a builder inventing
  one. The three-caller clause was added after reading the tree: an earlier draft named two paths
  and would have silently omitted steering, which is a shipped path. **Two clauses were added
  2026-08-09 on the round-3 bounce.** The generation-`0` clause exists because `fireWakeup` passes
  the literal `0` and there is no non-zero generation in existence to carry, so the previous
  MUST was unachievable on exactly the path AC-41's second GIVEN depends on; a builder could have
  resolved it by skipping the emission, which passes every other criterion and silently breaks
  AC-41. The duplicate clause replaces "a second `turn_started` leaves the same state as the
  first", which contradicted `turn_marker`'s increment-on-every-event rule — observable, because
  D2 discards an in-flight sample whose marker moved.*
  ***The tombstone qualification was added on the round-4 bounce.*** *The generation-`0` clause
  previously said the event is "**admitted** … including while a cancellation for that session is
  reconciling", full stop. That is true of the ownership filter and false of the consumer:
  `shouldDropCompletedExecutionStreamEvent` is a second gate, and cancellation tombstones the
  **owning** execution before teardown, so whether the event lands depended on which of the two
  raced — a test written to the old clause passes or fails on timing. Both outcomes are now named,
  and the drop is recorded as accepted rather than left looking like a bug.)*
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
  **And GIVEN** a dispatch that does reach `beginPromptTurn`, **THEN** the recorded turn start and
  the `turn_started` emission are performed by the **same `syncNotifQueueThen` barrier callback on
  the update worker** — asserted by **holding the barrier callback before it executes** and
  requiring that **neither** the recorded turn start nor the emitted event has changed while it is
  held, then releasing it and requiring **both** to have changed, **before** `conn.Prompt` is
  called. The stamp-before-dispatch half is additionally asserted by starting a process after
  `conn.Prompt` is entered and requiring the probe to report `live` for it. **And GIVEN**
  `beginPromptTurn` has returned, **THEN** `asyncTurnMu` is **not held** while the barrier is
  awaited — asserted by requiring a `tool_call` frame queued behind the barrier, whose handling
  reaches `maybeScheduleAsyncTurnComplete`, to be drained and the dispatch to complete rather than
  both sides blocking until adapter shutdown. **And GIVEN** the barrier does not complete its
  callback — whether `syncNotifQueueThen` returns `false`, **or it returns `true` but the worker
  skipped the callback because `lifetimeCtx` was done at dequeue** — **THEN** **neither** write
  happens: no turn start is stamped and no `turn_started` is emitted, so a subsequent probe reports
  `unknown` rather than comparing against a stamp whose event never arrived.
  *(The stamp-thread and barrier clauses were added 2026-08-09 on the round-3 bounce and
  **sharpened on the round-4 bounce**. Round 3: the stamp instant was previously specified twice on
  two different threads, and of the three implementations that satisfied every criterion, one
  stamps after `conn.Prompt` — so a workload the new turn spawns in the gap sorts before the
  baseline and the probe answers `settled`, the expensive direction D5 exists to forbid. AC-80 pins
  the truncation rule, not the stamp placement, so nothing guarded it.*
  *Round 4 found three separate holes in what replaced it. **(i)** The same-callback clause was
  **unobservable**: "starting a process after `conn.Prompt` and requiring `live`" proves only that
  the stamp preceded dispatch, which reading (b) — stamp on the prompt goroutine, enqueue
  non-blocking — also satisfies, and (b) satisfies AC-79a's FIFO clause too. So the reading this
  spec explicitly REJECTS passed every criterion. The hold-the-callback form is what actually
  distinguishes them. **(ii)** Nothing asserted the locking rule, so the pinned design's one real
  hazard — awaiting the barrier while holding `asyncTurnMu`, which deadlocks against the worker's
  own `maybeScheduleAsyncTurnComplete` — had no guard at all. **(iii)** The barrier clause keyed on
  a `false` return, but the worker can skip the callback and still close the sync channel, so a
  `true` return with neither write is reachable and was untested.)*
- **AC-45** — **GIVEN** the backend needs a liveness sample for a session, **WHEN** it takes one,
  **THEN** the request travels as the WebSocket action `agent.background.probe` on the existing
  agent stream carrying `session_id`, the request carries **no timestamp**, and the response
  `result` is one of exactly `live`, `settled`, `unknown`; **and** the `session_id` on the wire is
  the **ACP** session id, translated from the **Kandev** task-session id the **port** was called
  with, by `lifecycle.Manager` — asserted by driving the port with a Kandev id and reading the ACP
  id off the emitted frame, which fails for an implementation that passes the Kandev id straight
  through; **and** `Client.ProbeBackgroundWorkloads` is called with the **already-translated ACP**
  id, so the `Client` performs no lookup of its own.
  **And GIVEN** a well-formed response and a **nil** error, **THEN** the port returns **that exact
  literal unchanged** — asserted separately for `live`, for `settled` and for `unknown`, so an
  implementation that emits a correct frame and then maps every successful response to `unknown`
  fails. **And GIVEN** one `Probe` invocation, **THEN** **at most one** `agent.background.probe`
  frame is put on the wire for it: no retry, and no coalescing with a concurrent invocation for the
  same session, which are independent calls.
  *(The namespace clause was added 2026-08-09 on the round-3 bounce, and **corrected plus extended
  on the round-4 bounce**. Round 3: the port, the `Client` method and the wire all previously said a
  bare `sessionID` while the callers hold a Kandev id and agentctl's `AgentPID` accepts only an ACP
  id — and the spec's own words for that accessor are that passing the wrong one yields
  `ok == false` for every call, i.e. the feature ships, CI passes, and nothing ever parks.*
  *Round 4 found two further holes. **(i)** The round-3 fix put the Kandev id on the `Client` method
  too, which contradicts the same sentence's requirement that `lifecycle.Manager` translate "before
  the frame is built" — the `Client` is what builds the frame and has no session mapping. The
  namespace boundary is now named per layer (*Probe port*) and this criterion asserts both ends.
  **(ii)** Nothing pinned the SUCCESS mapping: this criterion constrained the wire response and
  AC-46 constrained eight failures, so a port answering `unknown` to every successful probe passed
  both — the same inert-but-green shape as (i), reached from the other side. AC-21's "probe reports
  `live`" runs against the test port implementation, so it never covered the production mapping.
  The per-invocation clause closes the remaining silence about retry and coalescing.)*
- **AC-46** — **GIVEN** each of these **ten** conditions in turn — the agent stream is
  disconnected (`ErrAgentStreamNotConnected`); the probe budget elapses; agentctl replies
  `ErrorCodeUnknownAction`; the response body is unparseable; the response carries a `result`
  outside the three literals; **the port returns a non-nil error alongside a `live` value**;
  **the port implementation panics**; **the Kandev task-session id cannot be translated to an ACP
  session id** (no execution, no live agentctl attachment, or an unknown session); **the Kandev
  task-session id is the empty string**; **the caller's context is already done on entry**
  (cancelled or past its deadline before the call) — **WHEN** the
  backend resolves the probe, **THEN** in **every** case the result is `unknown` and the session
  reports `parked_on_background_work: false`; and in the **last three** cases **no
  `agent.background.probe` frame is put on the wire at all**. *(The panic and error-beside-a-value cases were added
  2026-08-09. The error-beside-a-value case is the one the *Probe port* section calls out as the
  direction that would otherwise park a session on a failed sample, and it had no criterion. The
  panic case is asserted by the *Failure modes* table — "Probe errors **or panics** → treated as
  `unknown`" — and likewise had none; a panic that escapes the probe would take down the settle
  path rather than degrade to today's rendering. The eighth condition was added on the round-3
  bounce alongside the namespace rule, so the translation step has a stated failure direction
  rather than an invented one. **The previous "and turn settlement completes normally" clause is
  removed**: settlement is the settle hook's behaviour, which is not built at the same time as the
  probe transport this criterion covers, so that clause was unobservable wherever it was asserted
  and was never re-asserted later. What it was reaching for — that a failed probe does not delay or
  break settlement — is owned by AC-40, which asserts it against the budget.
  **The ninth and tenth conditions were added on the round-4 bounce.** An empty Kandev id is
  arguably subsumed by "an unknown session" and an already-done context by "the probe budget
  elapses", but neither was explicit, and both are entry-time conditions where the choice between
  "translate and fail" and "emit a doomed frame" is observable on the wire. Naming them keeps the
  no-frame set exhaustive rather than inferred.)*
- **AC-49** — **GIVEN** a task with two sessions, where session S1 has recorded two transitions
  (revision `2`, currently `false`) and session S2 has recorded none (revision `0`, `false`),
  **WHEN** S2 transitions to parked, **THEN** the task's `parked_on_background_work` becomes `true`
  **and its `parked_revision` is strictly greater than the value the task carried immediately
  before** — proving the task counter is independent of the member sessions' revisions; and
  **WHEN** a task-level update carrying a lower `parked_revision` at the same `parked_epoch`
  arrives at a consumer that has already applied the higher one, **THEN** it is discarded and the
  displayed value is unchanged.
- **AC-49a** — **GIVEN** a task with two sessions S1 and S2, neither parked, **WHEN** both
  transition to parked **concurrently**, **THEN** the task's `parked_on_background_work` ends
  `true`, its `parked_revision` advances **exactly once** (only the first transition flips the OR),
  exactly **one** `task.updated` is published, and no `task.updated` carries a `parked_revision`
  lower than one already published for that task. Asserted with the two session transitions
  genuinely racing, since the serialization point is the per-task lock around
  `members`-write → recompute → compare → increment → enqueue. *(Added 2026-08-09. AC-49 and AC-78
  are both sequential; the writer-side rule they exercise was added for the concurrent case and no
  criterion observed it. This is also the criterion that fails if a builder recomputes the OR by
  reading the sessions under the task lock — the lock-order violation the task-owned `members`
  cache exists to remove.)*
- **AC-50** — **GIVEN** a task with no sessions, or whose sessions have recorded no transition,
  **WHEN** the task DTO is serialized, **THEN** `parked_on_background_work` is `false` and
  `parked_revision` is `0`.
- **AC-51** — **GIVEN** a parked session, **WHEN** **each of the three `getSessionStateIcon` call
  sites** renders it — `components/task/sessions-dropdown.tsx:475`,
  `components/task/session-reopen-menu.tsx:204`, and
  `components/task/mobile/mobile-sessions-section.tsx:132` — **THEN** the background icon is shown
  rather than the `WAITING_FOR_INPUT` question mark; and **GIVEN** the same session additionally
  has a pending clarification, **THEN** the question mark wins, on all three. *(Widened
  2026-08-09 from `sessions-dropdown.tsx` alone. All three are named in the *Rendering contract* as
  passing the new option, but only one had a positive criterion — AC-52 covers the other two solely
  in the NOT-parked case, so a builder could have wired one surface, satisfied every criterion, and
  left the session switcher inconsistent between desktop and mobile.)*
- **AC-52** — **GIVEN** a session that is **not** parked, **WHEN** **`getSessionStateIcon` itself**
  is called for every row of **the matrix already enumerated in
  `apps/web/lib/ui/state-icons.test.tsx`** (session-icon describe blocks), extended with the new
  option pinned `false`, **THEN** the resolved icon is byte-identical to today's for every row —
  the new option defaults to `false` and changes nothing on its own.
  *(WHEN corrected 2026-08-09. It previously named the three component call sites while the THEN
  named the `state-icons.test.tsx` matrix — but that matrix calls the **function** directly and
  renders none of those components, so the criterion was unsatisfiable by the evidence it cited,
  and a builder would have resolved the conflict by quietly dropping the call-site clause. That
  clause is exactly what AC-51 was widened in round 2 to own, so it belongs there and not here.
  **AC-51 owns the three call sites; AC-52 owns the resolver matrix.** No coverage is lost.)*
- **AC-51a** — **GIVEN** a parked session rendered in the session switcher
  (`components/task/sessions-dropdown.tsx`), **WHEN** its tooltip is opened, **THEN** the tooltip
  text resolves through `task:backgroundWorkIsRunning` and does **not** read as the coarse
  `WAITING_FOR_INPUT` text; and **GIVEN** the same session with a pending clarification, **THEN**
  both the icon and the tooltip resolve to the pending-input affordance. The icon and the tooltip
  MUST agree in every row of the matrix. *(Added 2026-08-09. `sessionStatusTooltip` derives its
  text from `state`, `pending` and `foreground_activity` only, so changing icon resolution alone
  ships the background spinner beside text saying "waiting for input".)*
- **AC-53** — **GIVEN** a session that becomes parked, **WHEN** the sampling loop runs, **THEN** it
  is the **backend** that samples on the configured interval, agentctl holds no timer, and sampling
  for that session **stops** on the first of: a probe result other than `live`; the session leaving
  `WAITING_FOR_INPUT`; the session being stopped or deleted; **its execution ending**; an
  agent-context reset for that session; or backend shutdown.
  **And GIVEN** the stop was caused by the execution ending or an agent-context reset **while the
  session was still `WAITING_FOR_INPUT` and parked**, **THEN** stopping the loop does **not** strand
  the affordance: the row's eviction publishes the `true → false` transition with a strictly higher
  `revision` (AC-85), so "no further sample" never means "no further update". *(That clause was
  added 2026-08-09. Without it this criterion and the *Failure modes* agentctl-crash row specified
  different outcomes for the same condition — one said the affordance clears via a failed probe, the
  other stopped the only thing that could take one.)* *(The execution-end and
  agent-context-reset conditions were added 2026-08-09: the *Timing* section's loop lifecycle
  listed execution end, and *Data model → Entry lifecycle* adds the reset, but this criterion
  enumerated neither — so a sampler leaking past either would have passed.)*
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
- **AC-58a** — **GIVEN** a parked task rendered on the board, **WHEN** a subsequent `kanban.update`
  arrives that does **not** carry the parked fields, **THEN** the card still renders
  `data-testid="task-state-background-running"` — i.e. `parkedOnBackgroundWork` survives the
  rebuild in **both** `apps/web/lib/ws/handlers/kanban.ts` projections (`state.kanban.tasks` and
  `state.kanbanMulti.snapshots[…].tasks`), exactly as `foregroundActivity` already does. *(Added
  2026-08-09. AC-58 asserts the render given the prop; nothing asserted that the prop is produced
  and carried, and the board task is hand-rebuilt field by field into two stores. Without this,
  AC-58 passes in a unit test and fails on a live board.)*
- **AC-58b** — **GIVEN** a parked task that **also** has a pending clarification, and separately
  one that has a pending permission, **WHEN** the row renders through the **shared** resolver on
  the board and in the `/tasks` list, **THEN** the pending-input affordance wins on both surfaces
  and `task-state-background-running` is absent. *(Added 2026-08-09. AC-34 asserts this precedence
  only for resolver A's private ladder in `task-item.tsx`; nothing pinned it for
  `getTaskStateIconConfig`, so a builder could order the parked branch first there, pass every
  criterion, and hide an actionable question behind the background affordance on two surfaces.)*
- **AC-59** — **GIVEN** a task that is **not** parked, **WHEN** each of the six production
  `getTaskStateIcon` call sites enumerated in §M renders it, **THEN** the icon is identical to
  today's for **the matrix already enumerated in `apps/web/lib/ui/state-icons.test.tsx`**
  (task-icon describe blocks), extended with the new option pinned `false`; and **GIVEN** a task
  that is not parked, **WHEN** the sidebar row renders, **THEN** the icon is identical to today's
  for **the matrix already enumerated in `apps/web/components/task/task-item.test.tsx`**. Two
  baselines because there are two resolvers. This covers the three call sites this feature
  deliberately does not update.
- **AC-59a** — **GIVEN** the sidebar task list rendering a parked task after
  `BackgroundWorkTaskIcon` has been promoted into `state-icons.tsx`, **WHEN** the icon renders,
  **THEN** its resolved class list is identical to today's (`h-3.5 w-3.5` plus the `mt-[1px]`
  alignment nudge, now passed from `task-item.tsx` rather than hardcoded in the component); **and**
  **GIVEN** the board, the `/tasks` row and the graph nodes, **THEN** each receives **its own**
  size classes (`h-4 w-4`, `h-4 w-4 shrink-0`, `h-3 w-3` respectively) rather than the sidebar's,
  and none receives `mt-[1px]`. *(Added 2026-08-09. The promotion was specified as "unchanged",
  but the component takes no props and hardcodes its size, so promoted verbatim it silently
  swallows every caller's className — including `shrink-0` on `/tasks`, which is layout-affecting.
  No existing criterion looks at anything but the `data-testid`.)*
- **AC-62** — **GIVEN** a parked session rendering the background affordance, **and it is the only
  parked session on its task**, **WHEN** the probe transitions `live → settled`, **THEN** a
  `session.activity_changed` carrying `parked_on_background_work: false`, a higher `revision` and
  the current `parked_epoch` is published for that session, a `task.updated` is published for its
  task **because the task-level OR also changed**, and the task row stops rendering
  `data-testid="task-state-background-running"`.
- **AC-68** — **GIVEN** a parked session rendering the background affordance whose probe last
  reported `live`, **and it is the only parked session on its task**, **WHEN** the agent resumes
  itself and the session enters `RUNNING` **with no further sample being taken** (AC-53 stopped the
  loop), **THEN** `parked_on_background_work` is `false`, a `session.activity_changed` carrying
  `false` and a higher `revision` is published, and the task row stops rendering
  `data-testid="task-state-background-running"`. This is the session-state term of the formula
  clearing it, because `last_sample` is still `live` and is never re-read.
  *(The sole-parked-session clause was added to both GIVENs on 2026-08-09. Neither excluded a
  second parked session on the same task, so their "the task row stops rendering" THEN
  contradicted *Task-level projection* and AC-78, under which the task OR stays `true` and the row
  keeps rendering. AC-62 partly self-qualified via its "because the task-level OR also changed"
  clause; AC-68 did not qualify at all. The multi-session case is covered positively by AC-78 and
  is unchanged.)*
- **AC-69** — **GIVEN** a **second** launch recogniser for a different agent ID, registered in
  **agentctl** through the registry's public registration API from a test package, **WHEN** that
  agent emits a tool call its recogniser recognises, **THEN** the resulting
  `streams.NormalizedPayload` reports `IsDetachedBackgroundLaunch() == true` and survives a
  `MarshalJSON` → `UnmarshalJSON` round trip still reporting `true`; **and** the test package
  imports nothing from the probe, the parked projection, or any rendering path — its only
  production-code interaction is the registration call and the payload assertion. The
  wire round trip is part of the criterion because the attestation must cross a process boundary
  in the Docker and SSH executors.
- **AC-69a** — **GIVEN** the backend's ordered consumer receives a normalized payload reporting
  `IsDetachedBackgroundLaunch() == true` for a session, **WHEN** that session settles and the
  probe reports `live`, **THEN** `parked_on_background_work` is `true` — **and the backend
  performs no comparison against any agent name, agent ID, or agent profile field in doing so**,
  which an architecture test asserts over the symbols introduced by this feature.
  *(AC-69 has now been restated twice, and this round split it. It previously required "the
  session and task DTOs are serialized **and the task row renders**" while simultaneously
  forbidding the test package to import the projection or rendering paths — two clauses that
  cannot both hold: observing the DTO requires the projection path, and no Go test package can
  render a TSX row. Spec Review round 1 judged it unwritable as one test. The seam guarantee now
  lives in AC-69, testable in one Go package with the import constraint intact and therefore
  mechanically checkable; the vendor-neutrality of the backend, which is the other half of the
  guarantee, is AC-69a. The rendering clause is **dropped, not lost** — AC-23, AC-58 and AC-73a
  already assert that a parked session renders `data-testid="task-state-background-running"` on
  all three surfaces, and none of them cares which recogniser produced the attestation. No
  coverage is removed; the contract is unchanged.)*
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
  handling; `Service.handleSemanticOccurrence` is reached with the **same `taskID`, `sessionID`,
  `occurrenceID` (the turn id) and `eventType`** as a fixture captured from the pre-change code for
  the same inputs; the resulting `notificationPayload` — `{TaskID, TaskSessionID, OccurrenceID,
  EventType, Title, Body, Payload}` — is **byte-identical** to that fixture; **exactly one**
  `InsertDelivery` occurs for that occurrence id; and **no notification is withheld, deferred,
  delayed, reordered or dropped by anything in this spec**. Asserted across the parked, un-parked,
  `unknown`-probe and no-recogniser cases.
  This is the criterion that makes this spec shippable independently of
  [`parked-notification-deferral`](../parked-notification-deferral/spec.md).
  *(Restated TWICE, and the timestamp clause is now GONE rather than reworded — 2026-08-09.*
  *Revision 1 required "the same … timestamp it carries today", which has no referent inside a
  test process. Revision 2 replaced that with "asserted through the injected clock", which Spec
  Review round 2 showed is equally unwritable: there is **no `Clock` symbol anywhere in
  `internal/notifications/`**, and there is nothing to inject.*
  *Investigating that turned up the fact both revisions had missed, and it removes the clause
  instead of re-phrasing it a third time: **the notification carries no timestamp at all.***
  *`handleSemanticOccurrence` builds `models.Delivery{UserID, ProviderID, EventType,
  TaskSessionID, OccurrenceID}` and dispatches `notificationPayload{TaskID, TaskSessionID,
  OccurrenceID, EventType, Title, Body, Payload}` — no time field on either. The only timestamp in
  the whole path is `deliveries.created_at`, stamped by `store/sqlite.go` at insert time, which is
  storage bookkeeping this spec neither reads nor influences.*
  *So the byte-identical payload assertion is now **total** — it covers every field the
  notification actually has — and asserting a timestamp was always asserting a field that does not
  exist. The claim is unchanged: this spec alters nothing about the notification.)*
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
- **AC-79a** — **GIVEN** a turn that backgrounds a detached shell, with that attestation still
  queued on agentctl's `notifQueue` when the next `session/prompt` is dispatched, **WHEN** both
  reach the backend, **THEN** the attestation is delivered **before** `turn_started`, so
  `observed_detached` is set for turn N and then cleared by turn N+1 — never the reverse;
  **and GIVEN** a cancellation is reconciling for that session with a captured identity, **WHEN** a
  `turn_started` relayed from a **superseded execution** arrives, **THEN**
  `handleAgentStreamEvent`'s cancellation-ownership filter **rejects** it and `observed_detached`
  is left untouched — while a `turn_started` from the **owning execution, not yet tombstoned by
  `markExecutionFailed`**, and one carrying `prompt_generation: 0` from the synthetic wakeup path,
  are both **admitted** and do clear it. **And GIVEN** the owning execution **has** been tombstoned
  by that point, **THEN** the event is dropped by `shouldDropCompletedExecutionStreamEvent`
  instead — an accepted second drop path, asserted so the criterion is deterministic rather than a
  race between the relay and cancellation's own bookkeeping.
  *(Added 2026-08-09. `enqueueACPUpdate` posts notifications onto a FIFO drained by a single
  worker, while `beginPromptTurn` runs on the prompt goroutine — so an emission that called
  `sendUpdate` directly would overtake the previous turn's queued attestation, and the backend
  would attribute it to the new turn. AC-79 pins the attestation/turn-completion adjacency but
  said nothing about `turn_started`'s ordering against either.*
  ***The second clause was REPLACED on the round-3 bounce, and the reason is that it observed
  nothing.*** *It previously read "a `turn_started` carrying no execution identity or prompt
  generation is not silently dropped … because the event carries both" — which is self-refuting as
  a test (it describes an event that carries neither while asserting it carries both) and, worse,
  true of every implementation: `cancellationOwnsStreamEvent` rejects only on a mismatch of two
  **present** values, so an identity-free, generation-`0` event passes that filter whether
  or not the requirement was implemented. A conforming test and a no-op test both passed. The
  replacement asserts the property that is actually load-bearing and actually falsifiable —
  stale-execution rejection versus owning-execution and generation-`0` admission. The first clause
  is unchanged; FIFO ordering was always this criterion's real content.*
  ***The round-4 bounce added the tombstone branch*** *for the same reason the round-3 replacement
  was needed: "the owning execution is admitted" is true only until cancellation tombstones that
  execution, after which the second gate drops it. Without the qualification the clause asserted an
  outcome that depends on which side won a race.)*
- **AC-80** — **GIVEN** a descendant whose process start time falls in the same source-resolution
  tick as the recorded turn start but strictly after it in nanoseconds, **WHEN** the probe is taken,
  **THEN** it reports `live`, because the turn start is truncated down to the source's resolution
  before the inclusive comparison; and **GIVEN** an implementation that reads Darwin start times
  from `ps -eo lstart`, **THEN** this criterion fails, which is the intended guard against that
  source. This criterion exists **twice**, once per platform, with the enforcement stated in
  *Start-time source and resolution*: the **Linux** instance runs on the `ubuntu-latest` backend
  job and is the CI gate; the **Darwin** instance is host-gated on `runtime.GOOS == "darwin"` and
  MUST `t.Skip` with an explicit platform reason when it does not run, so a green CI log never
  reads as Darwin coverage. An implementation that satisfies only the Linux instance has not
  satisfied AC-80. *(The single "on Linux and on Darwin" phrasing was unenforceable: verified
  2026-08-09, `.github/workflows/backend-tests.yml` has no macOS runner, so half the criterion was
  guarded by nothing and would have read as passing.)*
- **AC-81** — **GIVEN** `KANDEV_PARKED_PROBE_BUDGET` set to `0`, and separately to a negative
  duration, **WHEN** configuration is loaded, **THEN** in both cases the value is rejected, a
  warning is logged, and the effective budget is the `250ms` default — so no synchronous probe is
  ever issued without a deadline.
- **AC-81a** — **GIVEN** `KANDEV_PARKED_PROBE_INTERVAL` set to a **negative** duration, **WHEN**
  configuration is loaded, **THEN** the value is rejected, a warning is logged, and the effective
  interval is the `30s` default; **and GIVEN** it set to exactly `0`, **THEN** it is **accepted**
  and periodic sampling is disabled per AC-74. The two cases are asserted together because the
  distinction is the whole point. *(Added 2026-08-09. `0` and the budget's non-positive case were
  both defined; the interval's negative case was silent between them, and one plausible reading —
  passing it to `time.NewTicker` — panics and takes the backend down at boot.)*
- **AC-81b** — **GIVEN** a probe request that agentctl begins handling and that exceeds
  `KANDEV_PARKED_PROBE_BUDGET` **inside agentctl**, **WHEN** the backend has already abandoned its
  own wait, **THEN** agentctl abandons the walk, answers `unknown`, and releases its per-session
  probe serialization — so a **subsequent** probe for that session is admitted and can return
  `live` or `settled` rather than queueing behind the abandoned one and timing out. *(Added
  2026-08-09. The backend's `context.WithTimeout` bounds only its own wait in `sendStreamRequest`;
  the request carries no deadline, so without an agentctl-side bound a single stalled walk turns
  a transient `unknown` into a permanent one under D6's serialization rule.)*
- **AC-82** — **GIVEN** a parked session, **WHEN** the app is rendered under the pseudo-locale,
  **THEN** the background affordance's accessible label and tooltip resolve through
  `task:backgroundWorkIsRunning` and no new translation key is introduced by this feature.
- **AC-83** — **GIVEN** a store task record (`KanbanState["tasks"][number]`) carrying
  `parkedOnBackgroundWork: true`, **and a `statusSummary` PRESENT on that same record** — a
  `TaskStatusSummary`, which carries no parked field of any kind — **WHEN** `buildSidebarItem`
  (`components/task/task-session-sidebar-item.ts`) and `toSheetItem`
  (`components/task/mobile/session-task-switcher-sheet-hooks.ts`) each map it, **THEN** the
  `TaskSwitcherItem` each returns has `parkedOnBackgroundWork: true`. The summary-present GIVEN is
  the whole point: an implementation that mirrors the adjacent
  `hasSummary ? summary?.foreground_activity : task.foregroundActivity` line yields `undefined` here
  and fails, which is the decision recorded under *API surface → The frontend property names*.
  **And GIVEN** that item, **WHEN** it renders through `task-switcher-row.tsx` → `TaskItem`,
  **THEN** `data-testid="task-state-background-running"` is present. Asserted on **both** producers,
  because the mobile task switcher renders the same `TaskSwitcher` and therefore the same `TaskItem`
  (§O) — an implementation that wires only the desktop producer passes on desktop and fails on
  mobile. *(Added 2026-08-09. AC-23 asserts the render given the prop and its home is
  `task-item.test.tsx`, which passes props directly, so nothing observed that the prop is produced
  at all. This is AC-58a's guarantee for resolver A, which had none.)*
- **AC-84** — **GIVEN** a Go boot snapshot (`WorkflowSnapshot`) in which a task carries
  `parked_on_background_work: true`, **WHEN** `snapshotToState` (`apps/web/lib/ssr/mapper.ts`) maps
  it into `KanbanState.tasks` and the board card and the sidebar row render **before any
  `kanban.update` has arrived**, **THEN** `data-testid="task-state-background-running"` is present
  on both surfaces. An implementation that projects the field in `toKanbanTask` and the two
  `kanban.ts` projections but not in `ssr/mapper.ts` **fails this criterion and passes AC-58a**,
  whose GIVEN is a *subsequent* `kanban.update`. *(Added 2026-08-09. `snapshotToState` hand-builds
  `KanbanTask` field by field and does not route through `toKanbanTask`, so it is a fourth,
  independent producer — and it is the one that runs on first paint, which is exactly when an
  operator looks at the board.)*
- **AC-85** — **GIVEN** a parked session at `(parked_epoch E, revision N)` with `N ≥ 1`, **and it is
  the only parked session on its task**, **and the session is still `WAITING_FOR_INPUT`** — so no
  term of the formula flips — **WHEN** its **execution ends** (agentctl idle-reaped or crashed) and
  the `parkedState` row is evicted, **THEN** a `session.activity_changed` carrying
  `parked_on_background_work: false`, a **strictly higher** `revision` and the same `parked_epoch`
  is published **before the entry is reduced**; a `task.updated` is published for its task because the
  task-level OR also changed; and a consumer that had applied `(E, N)` **applies** this frame and
  stops rendering `data-testid="task-state-background-running"`. **And GIVEN** the same setup but an
  **agent-context reset** instead of an execution end, **THEN** the same holds. **And GIVEN** a row
  whose `parked` is already `false`, **WHEN** it is evicted, **THEN** nothing is published. **And
  GIVEN** eviction caused by **backend shutdown**, **THEN** nothing is published in either case.
  An implementation that removes a `true` row without publishing fails, and fails in the way that
  matters: a consumer holding `(E, N)` then discards the rowless `(E, 0)` on every later carrier and
  keeps the affordance for the life of the backend process.
  **And GIVEN** the same eviction but with its publish **dropped** (the event-bus enqueue fails, per
  *API surface → What happens when the publish itself fails*), **WHEN** any later session carrier is
  serialized for that session, **THEN** it carries `(false, N+1, E)` — the **retained** revision, not
  `0` — and the consumer applies it, so the affordance still clears. An implementation that deletes
  the entry outright instead of reducing it fails this clause, which is the one that makes the
  publish-failure rule's "the revision only ever moves forward" true across an eviction. **And
  GIVEN** a **second** eviction cause firing for the same session afterwards, **THEN** nothing
  further is published.
  *(Added 2026-08-09. The previous revision asserted eviction "publishes nothing on its own —
  the projection has already gone `false` through the session-state term by the time any of these is
  reached", which is false for exactly these two causes: an execution can end while the session sits
  at `WAITING_FOR_INPUT`, which §J measures as an ordinary multi-minute window and which an
  `KANDEV_ACP_IDLE_TIMEOUT` reap makes routine. It also contradicted *Task-level projection*, which
  says the `members` removal "publishes the `true → false` flip exactly once" — a sentence that could
  never fire if the value were always already `false`.)*

## Suggested delivery order — ADVISORY, NOT CONTRACT

> **THIS SECTION IS NOT CONTRACT. DEMOTED BY HUMAN DECISION, 2026-08-09.**
>
> Nothing in this section binds anyone. **Build owns sequencing and Build decides when a criterion
> is closed.** No ordering, no grouping, and no per-criterion assignment here is a completion
> checklist, and there is deliberately no criterion-to-wave mapping and no criterion accounting to
> check a wave against. The contract is `## Requirements`, `## Scenarios`, and the decisions in
> `## Determinism, concurrency, and boundaries` — nothing else.
>
> **Why it was demoted.** Earlier revisions made the wave table contract ("a wave is complete only
> when its criteria pass") with a per-criterion `Closes` column. That mapping produced a
> Spec Review finding in **three consecutive rounds** — six criteria misassigned at round 2, two
> more at round 3, two more at round 4, with one criterion (AC-41b) relocated once and still
> landing in a wave that could not close it. The mapping was never a property of the shipped
> feature; it was a scheduling guess given the authority of a contract, and it kept being wrong in
> the one direction that matters — a builder marking a criterion green having never exercised it.
> The judgement it encoded is preserved below as advice, where being imprecise costs nothing.

The work splits roughly as follows. Treat it as a starting point and re-plan freely.

- **Seams first, and they ship as a no-op.** `turn_started` end to end, including the lifecycle
  relay's payload field mapping (which is also what stamps `ExecutionID`), the `notifQueue`
  ordering, and the prompt-generation payload. The `agent.background.probe` action with an agentctl
  stub returning `unknown`; `lifecycle.Manager`'s probe method with its Kandev→ACP translation, the
  `Client` method that takes the translated ACP id, the exhaustive error→`unknown` mapping, and
  agentctl's own budget. The `BackgroundProbe` port. Every DTO, wire and TS field present but
  hardcoded `false`/`0`.
- **Then three genuinely disjoint file sets, which can run concurrently:** the agentctl probe
  (`internal/agentctl/**` — descendant walk, start-time predicate, per-platform sources and clock
  domains, `AgentPID` on `process.Manager`); the recogniser and attestation
  (`transport/acp/normalize.go`, `internal/orchestrator/**` — the registry, the Claude recogniser,
  the `Kind == shell` filter, `observed_detached` and `turn_marker` on the ordered consumer);
  and the frontend (`apps/web/**` — the `BackgroundWorkTaskIcon` promotion with `className`, both
  task resolvers, both board early returns, the `/tasks` row, the session resolver and its tooltip,
  **all six producers** (`map-task.ts`, both `kanban.ts` projections, `ssr/mapper.ts`,
  `buildSidebarItem`, `toSheetItem` — §O), the merge-helper discard rule and the boot reset).
  Within the frontend set the producers are the half that is easy to skip, because every icon
  criterion passes without them.
- **Then the backend projection**, best kept to a single owner: the three-term formula, the settle
  hook on `updateTaskSessionStateWithHook`, session revisions and the task-owned `members` cache
  with its lock order, the publish rules and the publish-failure rule, the synchronous first
  sample, revalidation, entry lifecycle, and the sampling loop. This is the largest piece; that is
  the honest shape of this feature.
- **Then the guards, the E2E sequence, and the docs**: the AC-35 architecture test, the AC-76
  notification guard, the §J end-to-end sequence, and the *Contract amendment* below.

**Three real dependencies, which are the part of this section actually worth reading**, because
each has bitten a previous revision of the wave table:

- **A criterion whose THEN names a `parked_on_background_work` value cannot be exercised before the
  projection exists.** Wave-0 hardcoded fields make such a criterion *vacuously* green. AC-36,
  AC-37, AC-41, AC-69a, AC-70a and AC-27's DTO clause are all this shape.
- **A criterion whose THEN says "the probe reports `live`" needs the real probe**, not the stub —
  which notably includes **AC-41b**, whose emission-point clauses are otherwise agentctl-adapter
  work. Its probe-observable clauses cannot close until the process-tree probe lands, and its
  barrier and locking clauses can close before that.
- **The backend projection does NOT depend on the real probe** — only on the `BackgroundProbe`
  port. That is what the port buys, and it is why the port **is** contract while this section is
  not: the projection and its tests exist before the process-tree walk does.

Also worth carrying: **AC-70, AC-70a, AC-71, AC-72 and AC-80 must exercise a real process tree**
rather than a stub, and AC-80's Darwin instance is host-gated (*Start-time source and resolution*).
AC-73, AC-62 and AC-68 drive the injectable port with a scripted sequence, so they need no real
wait.

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
- **Parking on any background-work kind other than `shell`** — in particular async **subagent**
  launches, which `stampSubagentBackgroundWork` already stamps `Detached=true` for both Claude and
  `mock-agent`. This is a named exclusion, not an oversight: see *Open questions* for the decision
  and *API surface → The launch-recogniser seam* for the measurement that forced it. AC-37's
  second GIVEN is the regression guard. Extending the projection to a second kind is a deliberate
  contract change and would need its own liveness signal, since the process-tree probe cannot
  observe an in-agent construct.
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
- *Should an async **subagent** launch park a session, or only a detached background shell?*
  Raised by Spec Review round 2 as a possible open question and **closed by decision on
  2026-08-09 rather than deferred**, because the evidence settles it three ways: the *Why* section
  scopes this feature to background shells; the liveness probe walks OS processes and an async
  subagent has none, so every probe for one would answer `settled`; and subagent completion is
  already observable from frames (`Ended` via `subagentStatusTerminal`), which is exactly the
  signal §H establishes that shells lack. The backend predicate is therefore
  `IsDetachedBackgroundLaunch() && Kind == BackgroundWorkKindShell`. Reasoning and the measurement
  that forced it are in *API surface → The launch-recogniser seam*; extending to a second kind
  later is a deliberate contract change.

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
- **The recogniser registry goes in agentctl, not the backend.** The backend reads one
  vendor-neutral predicate, `NormalizedPayload.IsDetachedBackgroundLaunch()`
  (`types/streams/background_work.go:62-64`). Do **not** key anything backend-side on
  `payload.AgentID` (it is an execution id — `lifecycle/event_types.go:225`) or on
  `session.AgentProfileSnapshot["agent_name"]` (a central agent-name whitelist, which **ADR-0049
  rejects** — `turn_activity.go:1154-1157`). The attestation is proven to survive the process
  boundary: `background_work` is carried in both `MarshalJSON` and `UnmarshalJSON`
  (`tool_payload.go:101`, `:117`, `:138`, `:158`).
- **The synchronous first sample hooks `updateTaskSessionStateWithHook`**
  (`event_handlers_streaming.go`), at the **end**, gated on `changed == true && nextState ==
  WaitingForInput`. Do **not** hook `setSessionWaitingForInput`: three `event_handlers_workflow.go`
  sites — including **both step-transition settles**, which is the path §J's own observation took —
  write `WAITING_FOR_INPUT` directly through `updateTaskSessionState` and never call it. Do **not**
  hook `handleCompleteStreamEvent` either; its own state write lands late in the handler, so a
  probe placed there reads the pre-settle state and nothing ever parks. Do **not** use the existing
  `onChanged` callback — it runs *before* `publishTaskSessionStateChanged` and would delay that
  publish by the probe budget. D2 names the seam, the six covered sites, and every rejected
  placement.
- **`internal/mcp/handlers` has its OWN `setSessionWaitingForInput`** that never enters package
  `orchestrator`. It is deliberately out of scope (D2, AC-40b) because it settles a session to ask
  a human a question, where a pending clarification outranks parked anyway. Do not "fix" it into
  scope.
- **The backend predicate is `IsDetachedBackgroundLaunch() && Kind == BackgroundWorkKindShell`.**
  The kind filter is not optional: `stampSubagentBackgroundWork` independently stamps
  `Detached=true` with `Kind=subagent` and matches `mockAgentID` as well as `claudeAgentID`, so an
  unfiltered predicate makes AC-37 false for the agent every `dev` and `e2e` profile runs.
  `BackgroundWorkKindShell` is a shared-package kind constant, not an agent identity, so this does
  not weaken AC-69a.
- **The probe roots its walk via an `AgentPID(sessionID)` accessor on `process.Manager`** — the
  component that spawns the agent as its own `m.cmd` and already exposes
  `Manager.agentPID()` — never on `ProcessRunner`, which manages *workspace* processes, and never
  via the process group. The argument is the **ACP** session id, not the Kandev task-session id
  that `ProcessRunner.List` filters on. The accessor returns a pid only, precisely so the forbidden
  predicate is not reachable through it.
- **`turn_started` goes on the `notifQueue` FIFO, not straight to `sendUpdate`**, or it overtakes
  the previous turn's still-queued attestation and the backend marks the wrong turn (AC-79a).
  `sendPrompt` calls `syncNotifQueueThen(afterBarrier)` at `beginPromptTurn` and **blocks**; the
  callback does BOTH writes on the worker — stamp the turn start, then emit — so the stamp is
  guaranteed to precede `conn.Prompt` (AC-41b). Do **not** stamp on the prompt goroutine and emit
  separately, and do **not** stamp inside the callback while enqueuing non-blocking: the second
  lets the stamp land *after* `conn.Prompt`, so a workload the new turn spawns sorts before the
  baseline and the probe answers `settled`. Both writes live **inside** the callback and neither
  happens outside it — so a barrier that returns `false`, **or returns `true` after the worker
  skipped the callback at shutdown**, leaves neither written. Do not treat a `true` return as proof
  the stamp landed.
- **AWAIT THE BARRIER WITH NO ADAPTER MUTEX HELD.** `beginPromptTurn` holds `asyncTurnMu` for its
  whole body, and the update worker takes that same mutex via `maybeScheduleAsyncTurnComplete`. Post
  and await the barrier from `sendPrompt` **after** `beginPromptTurn` has returned and released it.
  Awaiting inside that critical section deadlocks every `session/prompt` on the adapter until
  shutdown, and `syncNotifQueueThen` honours no caller context, so nothing times it out (AC-41b).
- **Do NOT try to make `turn_started` carry a non-zero prompt generation.** The synthetic
  `ScheduleWakeup` path genuinely has none — `fireWakeup` passes the literal `0` — and generation
  `0` is **never rejected by** `cancellationOwnsStreamEvent`, which rejects only on a
  mismatch of two *present* values. Carry what agentctl holds and move on. `ExecutionID` is stamped
  by the lifecycle relay, not by agentctl, which has none.
- **The consumer has a SECOND drop gate after the ownership filter.**
  `shouldDropCompletedExecutionStreamEvent` drops any non-`complete` event from a tombstoned
  execution, and cancellation tombstones the **owning** execution before teardown. A `turn_started`
  can therefore be dropped even though the ownership filter admitted it. That is accepted and
  specified — do **not** special-case `turn_started` past it (AC-41a, AC-79a).
- **The probe port and `lifecycle.Manager`'s method take the KANDEV task-session id; the agentctl
  `Client` method takes the already-translated ACP id**, like `LoadSession` does. The Manager
  resolves the execution, reads `execution.ACPSessionID`, and calls the `Client`; the `Client`
  builds the frame and translates nothing. Passing the Kandev id through to agentctl makes
  `AgentPID` return `ok == false` for every call — the feature ships, CI passes, and nothing ever
  parks (AC-45).
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
- **Wiring the resolvers is HALF the frontend job. The other half is the SIX producers.** §O is that
  map and *API surface → The frontend property names* tabulates them: `map-task.ts`, **both**
  `kanban.ts` projections, **`lib/ssr/mapper.ts`** (the Go boot snapshot — a fourth producer of the
  same store shape that does **not** route through `toKanbanTask`), and **`buildSidebarItem`** plus
  **`toSheetItem`**, which are the only route to resolver A and feed the desktop sidebar and the
  mobile task switcher respectively. Every hop drops the value by omission. Because AC-23 and AC-58
  assert the render *given* the prop, an implementation that wires the resolvers and none of the
  producers passes every icon criterion with all four live surfaces dark — AC-83 and AC-84 exist
  precisely to fail that build.
- **In `buildSidebarItem` / `toSheetItem`, read the parked bit from the TASK RECORD with no
  `hasSummary` ternary.** The adjacent `foregroundActivity` line is
  `hasSummary ? summary?.foreground_activity : task.foregroundActivity`, and copying that shape is
  the trap: `TaskStatusSummary` carries no parked field and gains none, so the mirrored line returns
  `undefined` for every task that has a summary — the common case for an open task. The reasoning
  (including that `TaskStatusSummary` already has its own unrelated `revision`) is under
  *API surface → The frontend property names*; AC-83's GIVEN pins a summary as present so the wrong
  copy fails.
- **Evicting a `parkedState` row whose `parked` is `true` must PUBLISH the un-park before removing
  the row** (*Data model → Entry lifecycle*, AC-85). Do not treat eviction as bookkeeping: an
  execution can end — idle reap at `KANDEV_ACP_IDLE_TIMEOUT`, or a crash — while the session is
  still `WAITING_FOR_INPUT`, so no term of the formula flips, and a rowless session then serializes
  `revision 0`, which every consumer discards against the revision it already applied. The card
  keeps the affordance until the backend restarts. Backend-shutdown eviction is the one case that
  publishes nothing.
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
