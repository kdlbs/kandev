# Task busy indicators reflect all in-flight work

> Scope note: this document refocuses REQUIREMENTS.md onto the **busy-indicator
> surfacing** problem — making every surface that shows a task's live state tell
> the truth about whether *anything* is still running. The prior contents of
> this file (the #1597 executor-row / pause-resume scope) are preserved in git
> history and are still documented in `SPEC.md`'s executor-row sections; that
> work is a separate, largely-landed effort. The busy signal that effort
> produced now needs to be surfaced correctly and consistently to the operator,
> which is what this document owns.

## Problem statement §req:problem-statement

An operator manages many Kandev tasks at once and relies on the at-a-glance
status indicators — in the sidebar, on kanban cards, in task-list rows, on
graph/swimlane nodes, in the open-task header, in session menus, and on
mobile/compact views — to know which tasks are still working and which are
finished or waiting on them.

Today those indicators lie. When a task's **foreground** agent turn has gone
idle but the agent still has **background work in flight** — a sub-agent session
it spawned, or a watch it set up — the surfaces that show any state at all show
the task as **complete**, and the coverage is **uneven**: some surfaces show a
state, others show nothing, and they don't agree with each other. Foreground
activity already reads clearly (a gold/yellow spinner); background activity is
effectively invisible. There is no reliable way to distinguish, at a glance:

1. **Foreground running** — the agent is actively generating a turn.
2. **Background running** — the foreground turn is idle, but agent sub-work it
   started is still running.
3. **Waiting for my input** — the agent is idle and waiting for the operator to
   reply (this reads correctly in at least the sidebar today, but not
   necessarily everywhere).
4. **Finished** — nothing is running and nothing is pending.

**Why it hurts.** The wrong "complete" has real cost, in the operator's words:

- **Missed work.** The operator believes a task is done, moves on, and misses
  that a sub-agent or watch is still churning — or is about to need them.
- **Lost trust in the whole overview.** Once the indicators are known to lie,
  the operator stops believing any of them and has to open each task to check,
  which defeats the point of an at-a-glance board.
- **Destroyed work.** Worst of all, the operator sometimes **archives or deletes
  a task that turns out to still be running**, losing work mid-flight — a
  destructive, hard-to-reverse consequence of trusting a false "complete."

The fix is not a new capability so much as making the visible state *true and
consistent everywhere*, distinguishing the four situations above, keeping the
signal live and durable, and preventing the destructive misstep the false signal
currently invites.

## Success criteria §req:success-criteria

All criteria are observable from the product's surfaces (and behavior) by an
operator, without inspecting internals.

1. **Background work reads as running, not complete.** When a task's foreground
   turn has gone idle but a sub-agent session or watch it spawned is still
   running, every status surface shows the task as *running*, never as
   *complete* / finished.
2. **Any activity, anywhere, counts.** A task reads as running whenever *any* of
   its sessions has *any* foreground or background work in flight; it reads as
   not-running only when every session on the task is idle with nothing pending.
3. **Four states are each distinguishable at a glance,** on every surface:
   foreground-running, background-running, waiting-for-my-input, and finished.
   "Waiting for my input" looks different from "finished," and
   "background-running" looks different from "foreground-running."
4. **Background is visually distinct from foreground,** and the distinction does
   not rely on color alone — it survives color-blindness, small/compact sizes,
   and a quick glance (e.g. differing shape, motion, or an accessible
   label/tooltip in addition to hue).
5. **Every status surface agrees.** For the same task at the same moment, the
   sidebar, kanban card, task-list row, graph/swimlane node, open-task header,
   session menus, and their mobile/compact equivalents all show the same state.
   Where surfaces disagree today, they are reconciled.
6. **It's live.** The indicator flips near-immediately (within a second or two,
   without a manual refresh) when background work starts and when it ends.
7. **It survives reload and restart.** The state stays correct after a page
   refresh, after reopening the task, and after a backend restart — it never
   shows a stale "complete" while work runs, and never a stale "running" after
   work has finished.
8. **Destructive actions are guarded.** Attempting to archive or delete a task
   that still has work running warns the operator before it proceeds, so a
   still-running task can't be discarded by accident.
9. **The existing foreground signal is preserved.** The gold/yellow
   foreground-running spinner keeps its current meaning and prominence; this work
   adds the missing states around it rather than changing what "foreground
   running" looks like.

## User stories §req:user-stories

- **As an operator scanning my board,** I want a task that has finished its
  foreground turn but still has background work running to clearly read as
  *running* on every surface, so I don't walk away thinking it's done and miss
  work still in progress. *(→ §req:success-criteria #1, #2)*

- **As an operator triaging many tasks,** I want to tell foreground-running,
  background-running, waiting-for-me, and finished apart at a glance from any
  surface, so I immediately know which tasks need me, which are still churning,
  and which are truly done. *(→ §req:success-criteria #3, #4)*

- **As an operator who trusts the overview,** I want every surface to agree and
  the signal to stay correct after refreshes, reopening tasks, and backend
  restarts, so I can rely on the at-a-glance view without opening each task to
  verify. *(→ §req:success-criteria #5, #6, #7)*

- **As an operator cleaning up my board,** I want a warning before I archive or
  delete a task that's still running, so I never destroy in-flight work by
  mistaking a false "complete" for a real one. *(→ §req:success-criteria #8)*

- **As an operator on mobile or a compact view,** I want the same correct,
  distinguishable state I see on desktop, so the overview is trustworthy
  everywhere I use it. *(→ §req:success-criteria #3, #5)*

## Quality attributes §req:quality-attributes

- **Truthfulness first — false "done" is the dangerous error.** The signal must
  never show "complete" while work is running. When the state is momentarily
  uncertain, erring toward "running" is safer than erring toward "done," because
  a false "done" is what leads to missed and destroyed work.
- **Consistency across surfaces.** All surfaces derive from one shared notion of
  task/session state so they cannot disagree; there is a single source of truth
  for "is this task running," not per-surface guesses.
- **Liveness and durability.** The signal is event-driven and near-immediate,
  and remains correct across page reloads, task reopen, and backend restart —
  no stale state that a refresh would reveal as wrong.
- **Accessibility.** The four states are distinguishable without relying on color
  alone and remain legible at small/compact sizes and under a quick glance.
- **Mobile parity.** Mobile/compact surfaces carry the same correctness and
  distinguishability guarantees as their desktop counterparts.
- **No regression.** The existing foreground gold/yellow spinner behavior, and
  the existing "waiting for my input" signal where it already works, are not
  weakened.

## Constraints §req:constraints

- **Background scope is agent sub-work.** "Background work" that keeps a task
  reading as running means agent-level constructs — spawned sub-agent sessions
  and watches. A plain backgrounded shell command the agent started does **not**
  need to count. *(Operator decision.)*
- **"Waiting for my input" already partly works.** This state reads correctly at
  least in the sidebar today; the job is to make it consistent across surfaces,
  not to invent it from scratch. *(Operator: "it seems to be working … if you
  find inconsistencies across surfaces, fix them.")*
- **Foreground stays as-is.** Foreground-running keeps the current gold/yellow
  spinner and its prominence. *(Operator decision.)*
- **Destructive-action guard is in scope.** Warning before archive/delete of a
  running task is part of this effort, not deferred. *(Operator chose "also warn
  on destroy.")*
- **Repo conventions apply.** UI work follows the frontend conventions and
  desktop/mobile parity expectations in the repo guides, and every changed
  behavior carries tests (`CLAUDE.md`, `apps/web/AGENTS.md`).

## Priorities §req:priorities

Ordered by operator impact.

**Must have**

1. **Background never masquerades as complete.** On every status surface, a task
   with in-flight background sub-work reads as running, not finished — the core
   defect. *(§req:success-criteria #1, #2)*
2. **One truthful state, consistent across all surfaces.** Sidebar, kanban card,
   list row, graph/swimlane node, header, session menus, and mobile/compact
   views all agree, driven from a single source of truth. *(§req:success-criteria
   #5, #9)*
3. **Live and durable.** The signal flips near-immediately and stays correct
   across reload, reopen, and backend restart — never stale.
   *(§req:success-criteria #6, #7)*
4. **Four distinguishable states, background distinct from foreground, not by
   color alone.** *(§req:success-criteria #3, #4)*
5. **Guard destructive actions.** Warn before archiving or deleting a task that
   still has work running, to prevent the lost-work outcome.
   *(§req:success-criteria #8)*

**Nice to have**

6. Strengthen "waiting for my input" wherever it's weak or missing beyond the
   sidebar, so "needs me" is as unmistakable as "running" and "done."
   *(§req:success-criteria #3)*
