# ADR-2026-09-29-coordinator-phase-3-autonomy: Coordinator phase 3, autonomy

**Status:** proposed
**Date:** 2026-09-29
**Area:** backend, frontend, security, workflow

## Context

[ADR-2026-09-26-workspace-coordinator](2026-09-26-workspace-coordinator.md)
put the workspace coordinator in core behind `features.coordinator` and cut
the product into phases. Phase 1 (core flow) is attended only: a coordinator
acts when a manager sends it a message, and every write is a proposal a
person decides. Phase 2 (control) adds D17's per-coordinator permission
settings, the "What it did" log with undo, Watches, May do and Standing
orders; it is specified in parallel with this document and is referenced
here by those names only.

Phase 3 lets a coordinator act while nobody is watching. That ADR's gate G3
opens WP-11 and WP-12 only when this ADR records four decisions: the wake
design (a level-triggered backstop, no replacement fork), enforced
containment for unattended turns, the cost-ceiling model, and the first
`automatic` class. This ADR records them. The requirements and system designs
are the six phase 3 pairs under
[`docs/specs/coordinator/`](../specs/coordinator/README.md): wake,
containment, spend, relay, automatic and improvements. The delivery plan is
[`docs/plans/workspace-coordinator-p3/plan.md`](../plans/workspace-coordinator-p3/plan.md).

Two findings of the solution analysis that preceded phase 1 bind the wake
design:

- **Finding 5.2, the replacement fork.** An automation that reuses one thread
  as a coordinating parent forks it: the concurrency cap counts open run rows,
  turns started by a child, a question or a person create none, so a
  scheduled firing is admitted during such a turn; `continuationSession`
  treats any session that is not idle as unavailable, and
  `prepareAutomationTask` creates a replacement task and repoints the
  continuation to it. The result is two coordinators, one with the history
  and one with the schedule, and a fixed-string reason in the run row. The
  defect is still present on main
  (`orchestrator/event_handlers_automation.go`).
- **Finding 6.3, wake.** Every wake path in Kandev (child completion, Office
  reactivity, plugin hints, the shared runs subsystem of
  [ADR-2026-08-01](2026-08-01-global-run-scheduler-ownership.md)) can drop an
  event, so any design needs a level-triggered backstop. The task-level
  `task.stalled` event is itself deduplicated in memory only and is emitted
  again after a restart.

And one finding of phase 1 binds containment: with `features.auth` off, the
agent's own shell can call Kandev's REST API and external `/mcp` as the
synthetic admin identity, whatever the coordinator guard says about its
Kandev tools. Phase 1 accepted that for attended turns only and blocked
unattended turns on this decision.

## Decision

### Flag

Phase 3 ships behind a new release toggle, `features.coordinatorPhase3`
(`KANDEV_FEATURES_COORDINATOR_PHASE3`), `prod: "false"`, `dev: "false"`,
`e2e: "true"`, restart required. It is effective only while
`features.coordinator` is also on. Off, no wake is stored, no backstop runs,
no unattended turn starts, and every phase 3 route returns 404; stored
settings, wakes, turn rows, reviews and pending changes are kept.

### Autonomy is per coordinator, off by default, and needs a ceiling

A manager turns autonomy on for one coordinator in its settings. Turning it
on without a declared cost ceiling is refused. Readers and a coordinator
principal can never change it. Turning it off supersedes every pending wake;
a running unattended turn finishes.

### G3 decision 1: the wake design

This answers findings 5.2 and 6.3.

- **What wakes it.** Only the coordinator's own tasks: tasks created from its
  approved proposals, not archived and not ephemeral. Five conditions wake it,
  each with an episode key read from stored state: a question (the pending
  clarification's `pending_id`), a permission (the pending permission's
  `pending_id`), a stall (the stall row's `last_event_at`), an error (the
  session error's `stamp`), and completion (a constant). A Watch, a schedule
  or a heartbeat does not wake it in phase 3.
- **One wake per episode.** Wakes are durable rows, unique on (coordinator,
  task, kind, episode key). The event recorder and the backstop call one
  insert-or-nothing function with the same keys, so a redelivered event, a
  restart re-emitting `task.stalled`, and a backstop pass converge on one row.
  At most 200 pending wakes are held per coordinator; beyond that the episode
  waits for the backstop.
- **The backstop is level-triggered.** Every 60 seconds, for each coordinator
  with autonomy on, the backstop re-derives every current episode of its own
  tasks from stored task, session and stall state and records the missing
  ones. No event path is trusted to arrive; stored state is the source of
  truth (finding 6.3).
- **Busy is not unavailable; there is no replacement fork.** Wakes are
  delivered into the coordinator's one existing conversation, never into a
  new one. Admission distinguishes busy (`conversation_busy`: the session is
  starting, running, has a pending action or a queued message) from
  unavailable (`no_conversation`, `conversation_unavailable`). Busy holds the
  wakes and retries when the session goes idle or at the next backstop pass.
  Unavailable holds them until a manager opens a usable conversation. No
  wake, backstop pass or delivery creates, archives or repoints a
  conversation task, or starts any other session (finding 5.2).
- **Not the automation runner, the Office wakeup dispatcher or the shared
  runs queue.** Those serve other principals and carry the fork. The
  coordinator package owns a small store and a keyed delivery loop instead.
- **One unattended turn at a time.** Admission runs in a fixed order:
  autonomy on, containment, spend measurable, spend below the ceiling, a
  conversation, a usable session, an idle session, and five minutes since the
  last unattended turn ended. Delivery marks up to 20 still-holding wakes
  delivered and starts one unattended turn whose message lists them. A
  partial unique index on open turns makes the single open turn a database
  invariant, not a lock convention. A send failure or a crash between marking
  and sending returns the wakes to pending, so none is lost or delivered
  twice.
- **Visible.** The transcript shows an unattended turn's message as "Woken
  by N events", distinct from a manager's message. Needs you shows an
  autonomy strip (Active or Held with a reason, last turn, pending wakes,
  spend) and, for persistent hold reasons, one `autonomy` item that counts
  toward the coordinator's attention count and not the Inbox's.

Phase 1's rule that only a manager's message starts a turn
(`AC-COORDINATOR-COPILOT-002.1`) is amended to allow a delivery admitted by
`REQ-COORDINATOR-WAKE-004`, and nothing else.

### G3 decision 2: containment for unattended turns

What bounds an unattended turn is a containment check, run fresh at every
admission, whose four conditions must all hold. It adds no sandboxing; it
refuses to run unattended where the existing isolation cannot be shown.

1. **`executor_isolated`.** The coordinator's executor is `local_docker`,
   `remote_docker`, `sprites` or `k8s` (and `mock_remote` only under the e2e
   profile). `local` and `worktree` share the backend's host, filesystem and
   loopback; `ssh` can target the backend's own host; `plugin_remote`
   placement is a plugin's choice. All are refused.
2. **`auth_enabled`.** Kandev authentication is in mode `enabled`. With auth
   off or in setup, REST and external `/mcp` accept any caller that reaches
   the port, including a Docker container through `host.docker.internal`.
3. **`no_kandev_credential`.** The session's resolved launch environment holds
   no `KANDEV_API_KEY`, no `KANDEV_RUN_TOKEN`, and no value beginning with
   `kandev_pat_`.
4. **`no_extra_tools`.** The coordinator's agent profile configures no MCP
   server. Kandev's own tools reach the session through agentctl's local
   `/mcp`, which carries the server-derived coordinator principal and the
   phase 1 guard.

**What phase 3 refuses (fail closed).**

- A condition whose input cannot be read is not met. An unreadable executor
  profile, secret or MCP configuration holds the coordinator.
- Nothing caches the result, so a profile edit between admissions takes
  effect on the next one.
- Unmeasurable spend holds admission, and stops an open unattended turn.
- A permission request that is not auto-approved by phase 1's exact-name rule
  is denied at once during an unattended turn, with actor kind
  `coordinator_unattended`, and counted on the turn. Nobody is there to
  answer it.
- An improvement is never approved automatically, and only `create_task` can
  ever be raised to `automatic`.
- The coordinator has no tool that writes its own settings, autonomy,
  ceiling, reviews or pending changes; the guard refuses those routes to a
  coordinator principal on every transport.

Attended turns are unchanged: phase 1's residual (the agent's shell can reach
REST when auth is off) stays accepted for them, because a manager is present.

### G3 decision 3: the cost-ceiling model

- **What it is.** A per-coordinator ceiling in US dollars per rolling 24
  hours, from 0.01 to 10,000.00, stored in integer subcents and parsed without
  floats. It is set through the coordinator PATCH and does not reset the
  conversation.
- **Measured from.** Core's `task_usage_events` ledger, which exists with or
  without Office: the sum over `[now - 24h, now)` of every usage row of every
  conversation task the coordinator has had, current or archived, attended or
  unattended. Not Office's cost events or budgets. A seven-day daily mean is
  shown for context only.
- **Unmeasurable is not zero.** An unpriced usage row in the window, or a
  read error, makes spend unmeasurable; admission holds with
  `spend_unmeasured`.
- **What happens at it.** Admission refuses a new unattended turn at or above
  the ceiling (`ceiling_reached`). An open unattended turn is stopped when a
  recorded usage event takes the window to the ceiling: the usage writer's
  post-commit observer settles the turn `stopped_at_ceiling` and cancels it
  through the path the panel's Stop uses; the backstop re-checks every 60
  seconds in case the observer's queue dropped the call. Attended turns are
  never stopped. The overshoot is bounded by one usage report, because Kandev
  does not sit between the agent CLI and its model.

### G3 decision 4: the first automatic class

- **The class is `create_task`**, the phase 1 proposal kind: it creates a
  task and never starts an agent (D15), approval is idempotent through the
  reserved external id, and phase 2's undo can archive the created task. The
  set of raisable classes is a closed allowlist of one; every other class,
  including any phase 2 or later adds, is refused.
- **Chosen from the log, per coordinator.** A manager raises it with a D17
  setting on one coordinator, and the raise is refused unless, from that
  coordinator's own "What it did" log: the first `create_task` decision is at
  least 30 days old; the last 30 days hold at least 20 decisions; at least 90%
  of them were approved without edits; none of the tasks created from its
  proposals in that window was undone; and a manager recorded a review of
  that window in the last 7 days. Any read error refuses the raise.
- **What automatic does.** Inside `propose_task_kandev`, a `create_task`
  proposal of a raised coordinator is approved through the phase 1 approve
  service with `decided_automatically` set, so the log names the decider
  `automatic` and the manager who raised the setting (stored as
  `decided_by`, whose identity creates the task), and every phase 1
  guarantee intact. At most 10
  per coordinator per 24 hours; beyond that the proposal waits for a person.
- **Lowering is immediate.** A manager can lower it at any time with no
  check. Undoing a task that an automatic approval created lowers the class
  to `requires_approval` at once.
- **The evidence gate.** The class choice rests on reasoning, not yet on log
  rows, because phase 2's log does not exist yet. G3 is not met until this
  ADR's [G3 Status](#g3-status) records a review of at least 30 days of
  phase 2 log rows from real use that supports `create_task` (or names a
  different class, which re-plans the automatic work order).

### Answering in place, replying with a condition, improvements

- **Questions and permissions are answered on the Needs you card** with the
  same clarification bundle and resolver the Inbox uses (D14: the Inbox keeps
  its rows, count and tabs) and the same permission response the task chat
  sends. The card reads them through one coordinator relay route. No new
  answer contract exists.
- **Reply with a condition** settles a proposal as `returned` with the
  manager's text and delivers that text into the coordinator's conversation
  as the manager's own message, an attended turn. The coordinator may propose
  again with `in_reply_to`.
- **Improvement proposals** are a second proposal kind, proposed with
  `propose_improvement_kandev`, that suggests a replacement for the
  coordinator's context and cites 1 to 10 pieces of evidence, at least one an
  unattended turn. Approval stores a pending change and applies nothing; a
  manager applies it in settings, where Apply is refused if the context has
  changed since the proposal. Never automatic, never edited, and never a
  target other than the context.

## Residual risk

Accepted for phase 3, each named so a later phase can close it:

- **Board content steering.** Task titles, descriptions, questions and agent
  output are inputs the coordinator reads; a task's content can try to steer
  it. Every write it can make is still a proposal, except automatic
  `create_task`, which creates a task that starts no agent, capped at 10 a day
  and undoable.
- **Non-Kandev credentials in the executor environment.** A git or cloud
  token in the executor profile is not a Kandev credential and is not
  checked. Containment bounds what the agent can do to Kandev, not what it can
  do with the credentials a manager gave its executor.
- **`ssh` and `plugin_remote` are excluded**, even where a particular host is
  isolated, because nothing in their configuration proves it.
- **Clarification answers record no answerer.** Answering in place inherits
  that; permission answers keep their audit.
- **Auth-off REST during attended turns** remains phase 1's accepted
  residual.
- **The pending-wake cap can overshoot by concurrent inserts** (201 rows), by
  design; it bounds growth, not an exact count.
- **Spend overshoot of one usage report**, as above.

## G3 Status

**Status:** pending.

G3 is met when all four conditions of the phase plan hold: phase 2 is merged
with its exit met and its mockup ports pass in CI; this package is reviewed;
the phase 3 requirements and this ADR are posted on
[#3752](https://github.com/kdlbs/kandev/issues/3752) with screenshots, and a
kdlbs maintainer replies without objecting or 10 working days pass (recorded
here with the date); and the automatic class evidence above is recorded here.
Until then no phase 3 work order is built.

## Prior art

- **Conductor loop (author's notes, `concepts/conductor-loop.md`).** Separates
  admission, progress, learning and economy, and warns against an outer loop
  without an inner one. Adopted: admission is a fixed, ordered, deterministic
  gate in front of every unattended turn; the stall signal is progress; the
  ceiling is economy; improvements are learning, and a person applies them.
  The notes were cited in phase 1; this pass could not re-read the vault
  (access refused), so no new claim rests on them.
- **Paperclip heartbeats and routines**
  (`Paperclip/Documentation/guides_projects-workflow_routines.md`). Agents
  wake on assignment, comment, demand or a routine; timer heartbeats are off
  by default because they spend on nothing; a cooldown sets the minimum gap
  between runs; a routine's concurrency policy coalesces an overlapping tick
  into one queued follow-up, and catch-up is capped after downtime. Adopted:
  event wakes only, no heartbeat, a cooldown, coalescing into one pending
  batch, and a cap on held wakes. Differs: Paperclip starts a run per wake;
  the coordinator batches wakes into its one conversation and never forks it.
- **Claude Managed Agents session budgets**
  (`Claude Developer Platform/Documentation/docs_en_managed-agents_budgets.md`).
  A hard cap in integer cents sent as a string so no float rounding applies;
  enforced between model requests, so the overshoot is one request; a capped
  session goes idle, not terminated, and accepts only events that settle work
  in progress; usage that cannot be priced makes the budget unable to
  measure spend. Adopted: an integer, string-parsed ceiling; a bounded,
  documented overshoot; stopping a turn without ending the conversation; and
  unpriced usage treated as unmeasurable, which here fails closed. Differs:
  the window is a rolling 24 hours per coordinator rather than a lifetime cap
  per session, and answering a pending question stays possible because
  attended turns are never stopped.
- **Office.** Its wake path coalesces with an idempotency key and has a
  backstop; its budgets act at run admission only, and its budget alert
  handler is inert. Adopted: admission-time refusal plus an in-turn stop.
  Not reused, because Office's runs serve Office agents.
- What Kandev adds: a wake is a durable episode row whose key comes from
  stored state, so the event path and the backstop cannot disagree; and busy
  never becomes a reason to create a second conversation.

## Consequences

- The coordinator package gains four tables (`coordinator_wakes`,
  `coordinator_unattended_turns`, `coordinator_class_reviews`,
  `coordinator_pending_changes`), columns on `coordinators` and
  `coordinator_proposals`, a recorder, a 60-second backstop and a delivery
  loop, all built only when phase 3 is effective.
- The task usage writer gains an optional post-commit observer; the
  orchestrator's permission handling gains a coordinator hook at the seam
  phase 1's exact-name auto-approve already uses. Neither changes behaviour
  when nothing registers.
- The permission response builder moves from the chat hook to a shared module
  so the Needs you card and the chat send the same request.
- Autonomy is usable only on isolated executors with authentication on. A
  workspace on `local` or `worktree` executors keeps phase 1 and 2 behaviour
  and sees why in settings.
- The replacement fork in the automation runner is not fixed here; the
  coordinator does not use that runner.

## Alternatives considered

- **Run wakes through the automation runner.** Rejected: it carries the
  replacement fork and has no task or session trigger.
- **Run wakes through Office's wakeup dispatcher or the shared runs queue.**
  Rejected: both serve a different principal and would give the coordinator a
  run model beside its conversation.
- **A timer heartbeat.** Rejected: it spends while nothing happened, and the
  backstop already gives level-triggered recovery without starting a turn.
- **A coordinator-scoped token instead of the containment check.** Kandev's
  PATs are per user and carry no scope, so a scoped token would be a new
  authorisation model. Deferred; containment uses what exists and refuses the
  rest.
- **A ceiling from Office budgets.** Rejected: they exist only with Office and
  act only at admission.
- **A lifetime or per-turn ceiling.** Rejected: a lifetime cap needs manual
  resets to stay useful, and a per-turn cap does not bound a coordinator woken
  many times a day.
- **Raise any class from the start.** Rejected: only `create_task` starts no
  agent and is undone by archiving.
